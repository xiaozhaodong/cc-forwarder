package proxy

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/monitor"
	"cc-forwarder/internal/transport"
)

// handleSSERequest handles Server-Sent Events streaming requests
func (h *Handler) handleSSERequest(w http.ResponseWriter, r *http.Request, bodyBytes []byte) {
	slog.InfoContext(r.Context(), "🚀 [SSE Handler] 开始处理SSE流式请求", "method", r.Method, "path", r.URL.Path, "bodySize", len(bodyBytes))
	
	// Set SSE headers immediately
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Cache-Control")

	// Enable flushing
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Get connection ID from request context (set by logging middleware)
	connID := ""
	if connIDValue, ok := r.Context().Value("conn_id").(string); ok {
		connID = connIDValue
	}

	// Get healthy endpoints with fast testing if enabled
	ctx := r.Context()
	var endpoints []*endpoint.Endpoint
	if h.endpointManager.GetConfig().Strategy.Type == "fastest" && h.endpointManager.GetConfig().Strategy.FastTestEnabled {
		endpoints = h.endpointManager.GetFastestEndpointsWithRealTimeTest(ctx)
	} else {
		endpoints = h.endpointManager.GetHealthyEndpoints()
	}
	
	if len(endpoints) == 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		h.writeSSEError(w, "No healthy endpoints available", flusher)
		return
	}

	slog.InfoContext(ctx, fmt.Sprintf("🌊 [SSE 流式传输] 开始建立连接 - 客户端: %s, 路径: %s", 
		r.RemoteAddr, r.URL.Path))
	slog.InfoContext(ctx, fmt.Sprintf("🎯 [SSE 流式传输] 选择端点: %s (共%d个可用)", 
		endpoints[0].Config.Name, len(endpoints)))

	// Try endpoints in order until one succeeds
	for i, ep := range endpoints {
		// Update connection endpoint in monitoring
		if mm, ok := h.retryHandler.monitoringMiddleware.(interface{
			UpdateConnectionEndpoint(connID, endpoint string)
		}); ok && connID != "" {
			mm.UpdateConnectionEndpoint(connID, ep.Config.Name)
		}
		
		err := h.streamFromEndpoint(ctx, w, r, ep, bodyBytes, flusher, connID)
		if err == nil {
			// Success
			return
		}

		slog.ErrorContext(ctx, fmt.Sprintf("❌ [SSE 流式传输] 端点连接失败: %s - 错误: %s", ep.Config.Name, err.Error()))

		// If this isn't the last endpoint, try the next one
		if i < len(endpoints)-1 {
			h.writeSSEEvent(w, "retry", fmt.Sprintf("🔄 切换到备用端点: %s", endpoints[i+1].Config.Name), flusher)
			continue
		}

		// All endpoints failed
		h.writeSSEError(w, fmt.Sprintf("💥 所有端点连接失败，最后错误: %v", err), flusher)
		return
	}
}

// streamFromEndpoint streams response from a specific endpoint
func (h *Handler) streamFromEndpoint(ctx context.Context, w http.ResponseWriter, r *http.Request, ep *endpoint.Endpoint, bodyBytes []byte, flusher http.Flusher, connID string) error {
	// Create request to target endpoint
	targetURL := ep.Config.URL + r.URL.Path
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	// Create a context without timeout for streaming requests
	streamCtx := context.WithoutCancel(ctx)
	req, err := http.NewRequestWithContext(streamCtx, r.Method, targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Copy headers
	h.forwarder.CopyHeaders(r, req, ep)

	// Create HTTP client optimized for real-time streaming with proxy support
	httpTransport, err := transport.CreateTransport(h.config)
	if err != nil {
		return fmt.Errorf("failed to create transport: %w", err)
	}
	
	// Optimize transport for streaming
	httpTransport.DisableKeepAlives = false
	httpTransport.MaxIdleConns = 10
	httpTransport.MaxIdleConnsPerHost = 2
	httpTransport.IdleConnTimeout = 0 // No idle timeout
	httpTransport.TLSHandshakeTimeout = 10 * time.Second
	httpTransport.ExpectContinueTimeout = 1 * time.Second

	// 从配置中读取响应头超时时间，默认60秒
	responseHeaderTimeout := h.config.Streaming.ResponseHeaderTimeout
	if responseHeaderTimeout == 0 {
		responseHeaderTimeout = 60 * time.Second
	}
	httpTransport.ResponseHeaderTimeout = responseHeaderTimeout

	// Critical: Disable compression to prevent buffering delays
	httpTransport.DisableCompression = true
	// Set smaller buffer sizes for lower latency
	httpTransport.WriteBufferSize = 4096 // Smaller write buffer
	httpTransport.ReadBufferSize = 4096  // Smaller read buffer
	
	client := &http.Client{
		Timeout:   0, // No timeout for streaming
		Transport: httpTransport,
	}

	// Make the request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check if response is successful
	if resp.StatusCode >= 400 {
		return fmt.Errorf("endpoint returned error: %d", resp.StatusCode)
	}

	// Start streaming the response - use ultra-simple copy first
	return h.streamResponseUltraSimple(ctx, w, resp, flusher, connID, ep.Config.Name)
}

// streamResponse streams the HTTP response to the client
func (h *Handler) streamResponse(ctx context.Context, w http.ResponseWriter, resp *http.Response, flusher http.Flusher) error {
	slog.InfoContext(ctx, "📡 Starting real-time stream forwarding",
		"content_type", resp.Header.Get("Content-Type"),
		"status_code", resp.StatusCode)

	// Copy response headers first
	for key, values := range resp.Header {
		// Skip hop-by-hop headers and headers we set manually
		if key == "Connection" || key == "Transfer-Encoding" || key == "Content-Length" {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Ensure SSE headers are set
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Write status code
	w.WriteHeader(resp.StatusCode)
	flusher.Flush()

	// Use a smaller buffer for more responsive streaming
	scanner := bufio.NewScanner(resp.Body)
	// Use a smaller buffer size for lower latency
	buf := make([]byte, 4096) // Reduced from 64KB to 4KB
	scanner.Buffer(buf, 4096)

	// Create a ticker for heartbeat to keep connection alive
	heartbeatTicker := time.NewTicker(h.config.Streaming.HeartbeatInterval)
	defer heartbeatTicker.Stop()

	lastActivity := time.Now()
	lineCount := 0
	
	for {
		select {
		case <-ctx.Done():
			slog.InfoContext(ctx, "🚫 Stream cancelled by client",
				"lines_sent", lineCount,
				"duration", time.Since(lastActivity))
			return ctx.Err()
		case <-heartbeatTicker.C:
			// Send heartbeat if no activity for configured max idle time
			if time.Since(lastActivity) >= h.config.Streaming.MaxIdleTime {
				// Send SSE heartbeat comment (ignored by clients)
				fmt.Fprintf(w, ": heartbeat %s\n\n", time.Now().Format(time.RFC3339))
				flusher.Flush()
			}
		default:
			// Use a very short read timeout for responsiveness
			if conn, ok := resp.Body.(interface{ SetReadDeadline(time.Time) error }); ok {
				// Use a shorter timeout for better responsiveness
				conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			}

			if !scanner.Scan() {
				if err := scanner.Err(); err != nil {
					// Check if it's a timeout error, which is expected
					if netErr, ok := err.(interface{ Timeout() bool }); ok && netErr.Timeout() {
						// Timeout is expected, just continue the loop
						continue
					}
					slog.ErrorContext(ctx, "❌ Stream read error",
						"error", err.Error(),
						"lines_sent", lineCount)
					return fmt.Errorf("error reading response: %w", err)
				}
				// End of stream
				slog.InfoContext(ctx, "✅ Stream completed normally",
					"lines_sent", lineCount,
					"duration", time.Since(lastActivity))
				return nil
			}

			line := scanner.Text()
			lastActivity = time.Now()
			lineCount++
			
			// Write the line to the client immediately
			_, err := fmt.Fprintf(w, "%s\n", line)
			if err != nil {
				slog.ErrorContext(ctx, "❌ Error writing to client",
					"error", err.Error(),
					"lines_sent", lineCount)
				return fmt.Errorf("error writing to client: %w", err)
			}

			// CRITICAL: Flush immediately after each line for real-time streaming
			flusher.Flush()

			// Log progress every 10 lines (for debugging)
			if lineCount%10 == 0 {
				slog.DebugContext(ctx, "📊 Stream progress",
					"lines_sent", lineCount,
					"last_line_length", len(line))
			}
		}
	}
}

// writeSSEEvent writes a Server-Sent Event to the client
func (h *Handler) writeSSEEvent(w http.ResponseWriter, eventType, data string, flusher http.Flusher) {
	if eventType != "" {
		fmt.Fprintf(w, "event: %s\n", eventType)
	}
	
	// Handle multiline data
	lines := strings.Split(data, "\n")
	for _, line := range lines {
		fmt.Fprintf(w, "data: %s\n", line)
	}
	
	fmt.Fprintf(w, "\n")
	flusher.Flush()
}

// writeSSEError writes an error event to the client
func (h *Handler) writeSSEError(w http.ResponseWriter, message string, flusher http.Flusher) {
	h.writeSSEEvent(w, "error", message, flusher)
}

// streamResponseByBytes streams the HTTP response byte-by-byte for maximum real-time performance
func (h *Handler) streamResponseByBytes(ctx context.Context, w http.ResponseWriter, resp *http.Response, flusher http.Flusher, connID, endpointName string) error {
	slog.InfoContext(ctx, fmt.Sprintf("🚀 [实时流传输] 开始字节级转发 - 状态码: %d, 内容类型: %s", 
		resp.StatusCode, resp.Header.Get("Content-Type")))

	// Copy response headers first, preserving original content type
	originalContentType := ""
	for key, values := range resp.Header {
		// Skip hop-by-hop headers and headers we set manually
		if key == "Connection" || key == "Transfer-Encoding" || key == "Content-Length" {
			continue
		}
		if key == "Content-Type" {
			originalContentType = values[0]
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Only override content type if the backend didn't provide SSE headers
	if !strings.Contains(originalContentType, "text/event-stream") {
		slog.DebugContext(ctx, "🔄 [流转发] 后端没有返回SSE content-type，设置为event-stream", "originalType", originalContentType)
		w.Header().Set("Content-Type", "text/event-stream")
	} else {
		slog.DebugContext(ctx, "✅ [流转发] 保持后端原始content-type", "contentType", originalContentType)
	}
	
	// Ensure other SSE headers are set
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Write status code
	w.WriteHeader(resp.StatusCode)
	flusher.Flush()

	// Create heartbeat ticker
	heartbeatTicker := time.NewTicker(h.config.Streaming.HeartbeatInterval)
	defer heartbeatTicker.Stop()

	lastActivity := time.Now()
	bytesTransferred := int64(0)
	lineBuffer := make([]byte, 0, 1024)

	// Initialize token parser for extracting usage statistics
	tokenParser := NewTokenParserWithUsageTracker(connID, h.usageTracker)
	slog.InfoContext(ctx, "🔧 [Token Parser] 初始化完成，准备解析Claude API的令牌使用统计", "endpoint", endpointName, "connID", connID)
	
	// Initialize debug accumulator for SSE events
	var accumulatedEvents strings.Builder
	eventCounter := 0

	// Create a small buffer for reading bytes
	buffer := make([]byte, 1024)

	for {
		select {
		case <-ctx.Done():
			slog.InfoContext(ctx, fmt.Sprintf("🚫 [实时流传输] 客户端断开连接 - 已传输: %d字节, 耗时: %v", 
				bytesTransferred, time.Since(lastActivity)))
			return ctx.Err()
			
		case <-heartbeatTicker.C:
			// Send heartbeat if no activity for configured max idle time
			if time.Since(lastActivity) >= h.config.Streaming.MaxIdleTime {
				fmt.Fprintf(w, ": heartbeat %s\n\n", time.Now().Format(time.RFC3339))
				flusher.Flush()
			}

		default:
			// Set a very short read deadline for responsiveness
			if conn, ok := resp.Body.(interface{ SetReadDeadline(time.Time) error }); ok {
				conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
			}

			n, err := resp.Body.Read(buffer)
			if n > 0 {
				lastActivity = time.Now()
				bytesTransferred += int64(n)

				// Process each byte to detect line endings and flush immediately
				for i := 0; i < n; i++ {
					b := buffer[i]
					lineBuffer = append(lineBuffer, b)

					// If we hit a newline or the buffer is getting large, flush
					if b == '\n' || len(lineBuffer) >= 512 {
						// Parse the line for token usage before writing to client
						line := string(lineBuffer)
						
						// Accumulate SSE events for debug logging
						eventCounter++
						accumulatedEvents.WriteString(line)
						if len(line) > 0 && line[len(line)-1] != '\n' {
							accumulatedEvents.WriteString("\n")
						}
						
						// Debug logging: log accumulated SSE events every 10 events or when reaching 500 chars
						accumulatedContent := accumulatedEvents.String()
						if eventCounter%10 == 0 || len(accumulatedContent) > 500 {
							debugContent := accumulatedContent
							if len(debugContent) > 500 {
								debugContent = debugContent[:500]
							}
							slog.InfoContext(ctx, fmt.Sprintf("🐛 [调试SSE] 端点: %s, 事件数: %d, 总长度: %d字节, 累积SSE事件前500字符: %s", 
								endpointName, eventCounter, len(accumulatedContent), debugContent))
							
							// Reset accumulator if it gets too large
							if len(accumulatedContent) > 1000 {
								accumulatedEvents.Reset()
							}
						}
						
						// Always try to parse each line, with detailed logging
						slog.Debug(fmt.Sprintf("🔍 [Stream Parser] Processing line - line: %s, lineLength: %d", line, len(line)))
						if tokenUsage := tokenParser.ParseSSELine(line); tokenUsage != nil {
							// Record token usage if we have monitoring middleware
							if mm, ok := h.retryHandler.monitoringMiddleware.(interface{
								RecordTokenUsage(connID string, endpoint string, tokens *monitor.TokenUsage)
							}); ok && connID != "" {
								mm.RecordTokenUsage(connID, endpointName, tokenUsage)
								slog.InfoContext(ctx, fmt.Sprintf("✅ [令牌统计] 记录令牌使用 - 端点: %s, 输入: %d, 输出: %d, 缓存创建: %d, 缓存读取: %d",
									endpointName, tokenUsage.InputTokens, tokenUsage.OutputTokens, tokenUsage.CacheCreationTokens, tokenUsage.CacheReadTokens))
							} else {
								slog.Debug(fmt.Sprintf("⚠️ [Token Parser] Monitoring middleware not available or no connID - connID: %s, hasMiddleware: %t", connID, h.retryHandler.monitoringMiddleware != nil))
							}
						}
						
						_, writeErr := w.Write(lineBuffer)
						if writeErr != nil {
						slog.ErrorContext(ctx, fmt.Sprintf("❌ [实时流传输] 写入客户端失败 - 错误: %s, 已传输: %d字节", 
							writeErr.Error(), bytesTransferred))
							return fmt.Errorf("error writing to client: %w", writeErr)
						}
						
						// CRITICAL: Flush after every line for real-time streaming
						flusher.Flush()
						
						// Reset the line buffer
						lineBuffer = lineBuffer[:0]
					}
				}

				// Log progress periodically
				if bytesTransferred%10240 == 0 { // Every 10KB
					slog.DebugContext(ctx, fmt.Sprintf("📈 [流传输进度] 已传输: %d字节, 缓冲区: %d字节", 
						bytesTransferred, len(lineBuffer)))
				}
			}

			if err != nil {
				// Handle different types of errors
				if netErr, ok := err.(interface{ Timeout() bool }); ok && netErr.Timeout() {
					// Timeout is expected due to our short deadline, continue
					continue
				}
				
				// Check for EOF (end of stream)
				if err.Error() == "EOF" {
					// Flush any remaining data in the line buffer and parse it
					if len(lineBuffer) > 0 {
						// Try to parse the final line for tokens
						line := string(lineBuffer)
						slog.Debug(fmt.Sprintf("🔍 [Stream Parser] Processing final line - line: %s, lineLength: %d", line, len(line)))
						
						// Add final line to accumulated events and log final summary
						eventCounter++
						accumulatedEvents.WriteString(line)
						finalAccumulatedContent := accumulatedEvents.String()
						if len(finalAccumulatedContent) > 0 {
							debugContent := finalAccumulatedContent
							if len(debugContent) > 200 {
								debugContent = debugContent[:200]
							}
							slog.InfoContext(ctx, fmt.Sprintf("🐛 [调试SSE最终] 端点: %s, 总事件数: %d, 总长度: %d字节, 最终累积SSE事件前200字符: %s", 
								endpointName, eventCounter, len(finalAccumulatedContent), debugContent))
						}
						
						if tokenUsage := tokenParser.ParseSSELine(line); tokenUsage != nil {
							// Record token usage if we have monitoring middleware
							if mm, ok := h.retryHandler.monitoringMiddleware.(interface{
								RecordTokenUsage(connID string, endpoint string, tokens *monitor.TokenUsage)
							}); ok && connID != "" {
								mm.RecordTokenUsage(connID, endpointName, tokenUsage)
								slog.InfoContext(ctx, fmt.Sprintf("✅ [令牌统计] 记录最终令牌使用 - 端点: %s, 输入: %d, 输出: %d",
									endpointName, tokenUsage.InputTokens, tokenUsage.OutputTokens))
							}
						}
						
						w.Write(lineBuffer)
						flusher.Flush()
					}
					
					slog.InfoContext(ctx, fmt.Sprintf("✅ [实时流传输] 传输完成 - 总计: %d字节, 耗时: %v", 
						bytesTransferred, time.Since(lastActivity)))
					return nil
				}
				
				slog.ErrorContext(ctx, fmt.Sprintf("❌ [实时流传输] 读取错误 - 错误: %s, 已传输: %d字节", 
					err.Error(), bytesTransferred))
				return fmt.Errorf("error reading response: %w", err)
			}
		}
	}
}

// streamResponseSimple provides a simple, reliable stream forwarding implementation
func (h *Handler) streamResponseSimple(ctx context.Context, w http.ResponseWriter, resp *http.Response, flusher http.Flusher, connID, endpointName string) error {
	slog.InfoContext(ctx, "🚀 [简单流转发] 开始转发", "statusCode", resp.StatusCode, "contentType", resp.Header.Get("Content-Type"))

	// Copy response headers
	for key, values := range resp.Header {
		// Skip hop-by-hop headers
		if key == "Connection" || key == "Transfer-Encoding" || key == "Content-Length" {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Write status code
	w.WriteHeader(resp.StatusCode)
	flusher.Flush()

	// Initialize token parser for background parsing
	tokenParser := NewTokenParserWithUsageTracker(connID, h.usageTracker)
	lineBuffer := make([]byte, 0, 4096)
	
	// Simple copy with line-by-line token parsing
	buffer := make([]byte, 4096)
	bytesTransferred := int64(0)
	
	for {
		select {
		case <-ctx.Done():
			slog.InfoContext(ctx, "🚫 [简单流转发] 客户端断开", "bytesTransferred", bytesTransferred)
			return ctx.Err()
		default:
			n, err := resp.Body.Read(buffer)
			if n > 0 {
				bytesTransferred += int64(n)
				
				// Write directly to client first (priority: fast forwarding)
				_, writeErr := w.Write(buffer[:n])
				if writeErr != nil {
					slog.ErrorContext(ctx, "❌ [简单流转发] 写入失败", "error", writeErr)
					return writeErr
				}
				flusher.Flush()
				
				// Background token parsing (non-blocking)
				go func(data []byte) {
					for _, b := range data {
						lineBuffer = append(lineBuffer, b)
						if b == '\n' {
							line := string(lineBuffer)
							if tokenUsage := tokenParser.ParseSSELine(line); tokenUsage != nil {
								if mm, ok := h.retryHandler.monitoringMiddleware.(interface{
									RecordTokenUsage(connID string, endpoint string, tokens *monitor.TokenUsage)
								}); ok && connID != "" {
									mm.RecordTokenUsage(connID, endpointName, tokenUsage)
									slog.InfoContext(context.Background(), "✅ [简单流转发] 记录令牌使用", "endpoint", endpointName, "inputTokens", tokenUsage.InputTokens, "outputTokens", tokenUsage.OutputTokens)
								}
							}
							lineBuffer = lineBuffer[:0]
						}
					}
				}(buffer[:n])
			}
			
			if err != nil {
				if err.Error() == "EOF" {
					slog.InfoContext(ctx, "✅ [简单流转发] 转发完成", "bytesTransferred", bytesTransferred)
					return nil
				}
				slog.ErrorContext(ctx, "❌ [简单流转发] 读取错误", "error", err)
				return err
			}
		}
	}
}

// streamResponseUltraSimple provides the most basic stream forwarding without any parsing
func (h *Handler) streamResponseUltraSimple(ctx context.Context, w http.ResponseWriter, resp *http.Response, flusher http.Flusher, connID, endpointName string) error {
	slog.InfoContext(ctx, "🚀 [超简单流转发] 开始纯转发", "statusCode", resp.StatusCode)

	// Copy response headers as-is
	for key, values := range resp.Header {
		// Skip hop-by-hop headers
		if key == "Connection" || key == "Transfer-Encoding" || key == "Content-Length" {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Write status code
	w.WriteHeader(resp.StatusCode)
	flusher.Flush()

	// Pure io.Copy
	slog.InfoContext(ctx, "📡 [超简单流转发] 开始io.Copy")
	_, err := io.Copy(w, resp.Body)
	
	if err != nil {
		slog.ErrorContext(ctx, "❌ [超简单流转发] 复制失败", "error", err)
		return err
	}
	
	slog.InfoContext(ctx, "✅ [超简单流转发] 复制完成")
	return nil
}