package proxy

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/lzw"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/monitor"
	"cc-forwarder/internal/tracking"
	"cc-forwarder/internal/transport"
	"github.com/andybalholm/brotli"
)

// Context key for endpoint information
type contextKey string

const EndpointContextKey = contextKey("endpoint")

// WebNotifier interface for Web interface notifications
type WebNotifier interface {
	BroadcastConnectionUpdateSmart(data map[string]interface{}, changeType string)
	IsEventManagerActive() bool
}

// Handler handles HTTP proxy requests
type Handler struct {
	endpointManager *endpoint.Manager
	config          *config.Config
	retryHandler    *RetryHandler
	usageTracker    *tracking.UsageTracker
	webNotifier     WebNotifier // 添加Web通知器
}

// NewHandler creates a new proxy handler
func NewHandler(endpointManager *endpoint.Manager, cfg *config.Config) *Handler {
	retryHandler := NewRetryHandler(cfg)
	retryHandler.SetEndpointManager(endpointManager)
	
	return &Handler{
		endpointManager: endpointManager,
		config:          cfg,
		retryHandler:    retryHandler,
	}
}

// SetMonitoringMiddleware sets the monitoring middleware for retry tracking
func (h *Handler) SetMonitoringMiddleware(mm interface{
	RecordRetry(connID string, endpoint string)
}) {
	h.retryHandler.SetMonitoringMiddleware(mm)
}

// SetUsageTracker sets the usage tracker for request tracking
func (h *Handler) SetUsageTracker(ut *tracking.UsageTracker) {
	h.usageTracker = ut
}

// SetWebNotifier sets the web notifier for real-time connection updates
func (h *Handler) SetWebNotifier(notifier WebNotifier) {
	h.webNotifier = notifier
}

// GetRetryHandler returns the retry handler for accessing suspended request counts
func (h *Handler) GetRetryHandler() *RetryHandler {
	return h.retryHandler
}

// ServeHTTP implements the http.Handler interface
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Create a context for this request
	ctx := r.Context()
	
	// Clone request body for potential retries
	var bodyBytes []byte
	if r.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusInternalServerError)
			return
		}
		r.Body.Close()
	}

	// Check if this is an SSE request - Claude API streaming patterns
	acceptHeader := r.Header.Get("Accept")
	cacheControlHeader := r.Header.Get("Cache-Control")
	streamHeader := r.Header.Get("stream")
	
	// Multiple ways to detect streaming requests:
	// 1. Accept header contains text/event-stream
	// 2. Cache-Control header contains no-cache
	// 3. stream header is set to true
	// 4. Request body contains "stream": true
	isSSE := strings.Contains(acceptHeader, "text/event-stream") || 
			 strings.Contains(cacheControlHeader, "no-cache") ||
			 streamHeader == "true" ||
			 strings.Contains(string(bodyBytes), `"stream":true`) ||
			 strings.Contains(string(bodyBytes), `"stream": true`)

	// TEMPORARILY DISABLE STREAMING - force all requests to use regular handler for debugging
	if false && isSSE {
		h.handleSSERequest(w, r, bodyBytes)
		return
	}
	// Handle all requests with regular handler (with token parsing)
	h.handleRegularRequest(ctx, w, r, bodyBytes)
}

// handleRegularRequest handles non-streaming requests
func (h *Handler) handleRegularRequest(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte) {
	var selectedEndpointName string
	
	// Get connection ID from request context (set by logging middleware)
	connID := ""
	if connIDValue, ok := r.Context().Value("conn_id").(string); ok {
		connID = connIDValue
	}
	
	operation := func(ep *endpoint.Endpoint, connectionID string) (*http.Response, error) {
		// Store the selected endpoint name for logging
		selectedEndpointName = ep.Config.Name
		
		// Update connection endpoint in monitoring (if we have a monitoring middleware)
		if mm, ok := h.retryHandler.monitoringMiddleware.(interface{
			UpdateConnectionEndpoint(connID, endpoint string)
		}); ok && connectionID != "" {
			mm.UpdateConnectionEndpoint(connectionID, ep.Config.Name)
		}
		
		// Create request to target endpoint
		targetURL := ep.Config.URL + r.URL.Path
		if r.URL.RawQuery != "" {
			targetURL += "?" + r.URL.RawQuery
		}

		req, err := http.NewRequestWithContext(ctx, r.Method, targetURL, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		// Copy headers from original request
		h.copyHeaders(r, req, ep)

		// Create HTTP client with timeout and proxy support
		httpTransport, err := transport.CreateTransport(h.config)
		if err != nil {
			return nil, fmt.Errorf("failed to create transport: %w", err)
		}
		
		client := &http.Client{
			Timeout:   ep.Config.Timeout,
			Transport: httpTransport,
		}

		// Make the request
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("request failed: %w", err)
		}

		// Return the response - retry logic will check status code
		return resp, nil
	}

	// Execute with retry logic
	finalResp, lastErr := h.retryHandler.ExecuteWithContext(ctx, operation, connID)
	
	// Store selected endpoint info in request context for logging
	if selectedEndpointName != "" {
		*r = *r.WithContext(context.WithValue(r.Context(), "selected_endpoint", selectedEndpointName))
	}
	
	if lastErr != nil {
		// 通知Web界面连接失败
		h.notifyConnectionEvent(connID, selectedEndpointName, "error_response", lastErr)
		
		// Check if the error is due to no healthy endpoints
		if strings.Contains(lastErr.Error(), "no healthy endpoints") {
			http.Error(w, "Service Unavailable: No healthy endpoints available", http.StatusServiceUnavailable)
		} else {
			// If all retries failed, return error
			http.Error(w, "All endpoints failed: "+lastErr.Error(), http.StatusBadGateway)
		}
		return
	}

	if finalResp == nil {
		// 通知Web界面无响应错误
		h.notifyConnectionEvent(connID, selectedEndpointName, "no_response", fmt.Errorf("no response received"))
		http.Error(w, "No response received from any endpoint", http.StatusBadGateway)
		return
	}

	defer finalResp.Body.Close()

	// Copy response headers (except Content-Encoding for gzip handling)
	for key, values := range finalResp.Header {
		// Skip Content-Encoding header as we handle gzip decompression ourselves
		if strings.ToLower(key) == "content-encoding" {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Set status code
	w.WriteHeader(finalResp.StatusCode)

	// Read and decompress response body if needed
	slog.DebugContext(ctx, fmt.Sprintf("🔄 [开始读取响应] [%s] 端点: %s, Content-Encoding: %s", 
		connID, selectedEndpointName, finalResp.Header.Get("Content-Encoding")))
	
	bodyBytes, err := h.readAndDecompressResponse(ctx, finalResp, selectedEndpointName)
	if err != nil {
		slog.ErrorContext(ctx, fmt.Sprintf("❌ [响应读取失败] [%s] 端点: %s, 错误: %v", connID, selectedEndpointName, err))
		http.Error(w, "Failed to read response: "+err.Error(), http.StatusInternalServerError)
		return
	}

	slog.DebugContext(ctx, fmt.Sprintf("✅ [响应读取成功] [%s] 端点: %s, 长度: %d字节", 
		connID, selectedEndpointName, len(bodyBytes)))

	bodyContent := string(bodyBytes)
	slog.DebugContext(ctx, fmt.Sprintf("🐛 [调试响应头] 端点: %s, 响应头: %v", selectedEndpointName, finalResp.Header))
	
	// Pass the complete response content to logger - let the logger decide how to handle truncation
	slog.DebugContext(ctx, fmt.Sprintf("🐛 [调试响应] 端点: %s, 状态码: %d, 长度: %d字节, 响应内容: %s", 
		selectedEndpointName, finalResp.StatusCode, len(bodyContent), bodyContent))
	
	// Analyze the complete response for token usage
	slog.DebugContext(ctx, fmt.Sprintf("🔍 [开始Token分析] [%s] 端点: %s", connID, selectedEndpointName))
	h.analyzeResponseForTokens(ctx, bodyContent, selectedEndpointName, r)
	slog.DebugContext(ctx, fmt.Sprintf("✅ [Token分析完成] [%s] 端点: %s", connID, selectedEndpointName))
	
	// Write the body to client
	_, writeErr := w.Write(bodyBytes)
	if writeErr != nil {
		// 通知Web界面写入失败
		h.notifyConnectionEvent(connID, selectedEndpointName, "write_error", writeErr)
	} else {
		// 通知Web界面请求成功完成
		h.notifyConnectionEvent(connID, selectedEndpointName, "request_completed", nil)
	}
}

// readAndDecompressResponse reads and decompresses the response body based on Content-Encoding
func (h *Handler) readAndDecompressResponse(ctx context.Context, resp *http.Response, endpointName string) ([]byte, error) {
	// Read the raw response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check Content-Encoding header
	contentEncoding := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding")))
	if contentEncoding == "" {
		// No encoding, return as is
		return bodyBytes, nil
	}

	// Handle different compression methods
	switch contentEncoding {
	case "gzip":
		return h.decompressGzip(ctx, bodyBytes, endpointName)
	case "deflate":
		return h.decompressDeflate(ctx, bodyBytes, endpointName)
	case "br":
		return h.decompressBrotli(ctx, bodyBytes, endpointName)
	case "compress":
		return h.decompressLZW(ctx, bodyBytes, endpointName)
	case "identity":
		// Identity means no encoding
		return bodyBytes, nil
	default:
		// Unknown encoding, log warning and return as is
		slog.WarnContext(ctx, fmt.Sprintf("⚠️ [压缩] 未知的编码方式，端点: %s, 编码: %s", endpointName, contentEncoding))
		return bodyBytes, nil
	}
}

// decompressGzip decompresses gzip encoded content
func (h *Handler) decompressGzip(ctx context.Context, bodyBytes []byte, endpointName string) ([]byte, error) {
	slog.DebugContext(ctx, fmt.Sprintf("🗜️ [GZIP] 检测到gzip编码响应，端点: %s, 压缩长度: %d字节", endpointName, len(bodyBytes)))
	
	gzipReader, err := gzip.NewReader(bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	decompressedBytes, err := io.ReadAll(gzipReader)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress gzip content: %w", err)
	}

	slog.DebugContext(ctx, fmt.Sprintf("🗜️ [GZIP] 解压完成，端点: %s, 解压后长度: %d字节", endpointName, len(decompressedBytes)))
	return decompressedBytes, nil
}

// decompressDeflate decompresses deflate encoded content
func (h *Handler) decompressDeflate(ctx context.Context, bodyBytes []byte, endpointName string) ([]byte, error) {
	slog.DebugContext(ctx, fmt.Sprintf("🗜️ [DEFLATE] 检测到deflate编码响应，端点: %s, 压缩长度: %d字节", endpointName, len(bodyBytes)))
	
	deflateReader := flate.NewReader(bytes.NewReader(bodyBytes))
	defer deflateReader.Close()

	decompressedBytes, err := io.ReadAll(deflateReader)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress deflate content: %w", err)
	}

	slog.DebugContext(ctx, fmt.Sprintf("🗜️ [DEFLATE] 解压完成，端点: %s, 解压后长度: %d字节", endpointName, len(decompressedBytes)))
	return decompressedBytes, nil
}

// decompressBrotli decompresses Brotli encoded content
func (h *Handler) decompressBrotli(ctx context.Context, bodyBytes []byte, endpointName string) ([]byte, error) {
	slog.DebugContext(ctx, fmt.Sprintf("🗜️ [BROTLI] 检测到br编码响应，端点: %s, 压缩长度: %d字节", endpointName, len(bodyBytes)))
	
	brotliReader := brotli.NewReader(bytes.NewReader(bodyBytes))

	decompressedBytes, err := io.ReadAll(brotliReader)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress brotli content: %w", err)
	}

	slog.DebugContext(ctx, fmt.Sprintf("🗜️ [BROTLI] 解压完成，端点: %s, 解压后长度: %d字节", endpointName, len(decompressedBytes)))
	return decompressedBytes, nil
}

// decompressLZW decompresses LZW (compress) encoded content
func (h *Handler) decompressLZW(ctx context.Context, bodyBytes []byte, endpointName string) ([]byte, error) {
	slog.DebugContext(ctx, fmt.Sprintf("🗜️ [LZW] 检测到compress编码响应，端点: %s, 压缩长度: %d字节", endpointName, len(bodyBytes)))
	
	// LZW reader with MSB order (standard for HTTP compress)
	lzwReader := lzw.NewReader(bytes.NewReader(bodyBytes), lzw.MSB, 8)
	defer lzwReader.Close()

	decompressedBytes, err := io.ReadAll(lzwReader)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress LZW content: %w", err)
	}

	slog.DebugContext(ctx, fmt.Sprintf("🗜️ [LZW] 解压完成，端点: %s, 解压后长度: %d字节", endpointName, len(decompressedBytes)))
	return decompressedBytes, nil
}

// analyzeResponseForTokens analyzes the complete response body for token usage information
func (h *Handler) analyzeResponseForTokens(ctx context.Context, responseBody, endpointName string, r *http.Request) {
	
	// Get connection ID from request context
	connID := ""
	if connIDValue, ok := r.Context().Value("conn_id").(string); ok {
		connID = connIDValue
	}
	
	// Add entry log for debugging
	slog.DebugContext(ctx, fmt.Sprintf("🎯 [Token分析入口] [%s] 端点: %s, 响应长度: %d字节", 
		connID, endpointName, len(responseBody)))
	
	// Method 1: Try to find SSE format in the response (for streaming responses that were buffered)
	// Check for error events first before checking for token events
	if strings.Contains(responseBody, "event:error") || strings.Contains(responseBody, "event: error") {
		h.parseSSETokens(ctx, responseBody, endpointName, connID)
		return
	}
	
	// Check for both message_start and message_delta events as token info can be in either
	if strings.Contains(responseBody, "event:message_start") || 
	   strings.Contains(responseBody, "event: message_start") ||
	   strings.Contains(responseBody, "event:message_delta") || 
	   strings.Contains(responseBody, "event: message_delta") {
		h.parseSSETokens(ctx, responseBody, endpointName, connID)
		return
	}
	
	// Method 2: Try to parse as single JSON response
	if strings.HasPrefix(strings.TrimSpace(responseBody), "{") && strings.Contains(responseBody, "usage") {
		h.parseJSONTokens(ctx, responseBody, endpointName, connID)
		return
	}

	// Fallback: No token information found, mark request as completed with non_token_response model
	slog.InfoContext(ctx, fmt.Sprintf("🎯 [无Token响应] 端点: %s, 连接: %s - 响应不包含token信息，标记为完成", endpointName, connID))
	
	// Update request status to completed and set model name to "non_token_response"
	if h.usageTracker != nil && connID != "" {
		// Create empty token usage for consistent completion tracking
		emptyTokens := &tracking.TokenUsage{
			InputTokens:         0,
			OutputTokens:        0,
			CacheCreationTokens: 0,
			CacheReadTokens:     0,
		}
		
		// Record completion with non_token_response model name and zero duration (since we don't track start time here)
		h.usageTracker.RecordRequestComplete(connID, "non_token_response", emptyTokens, 0)
		
		slog.InfoContext(ctx, fmt.Sprintf("✅ [无Token完成] 连接: %s 已标记为完成状态，模型: non_token_response", connID))
	}
}

// parseSSETokens parses SSE format response for token usage or error events
func (h *Handler) parseSSETokens(ctx context.Context, responseBody, endpointName, connID string) {
	tokenParser := NewTokenParserWithUsageTracker(connID, h.usageTracker)
	lines := strings.Split(responseBody, "\n")
	
	foundTokenUsage := false
	hasErrorEvent := false
	
	// Check if response contains error events first
	if strings.Contains(responseBody, "event:error") || strings.Contains(responseBody, "event: error") {
		hasErrorEvent = true
		slog.InfoContext(ctx, fmt.Sprintf("❌ [SSE错误检测] 端点: %s, 连接: %s - 检测到error事件", endpointName, connID))
	}
	
	for _, line := range lines {
		if tokenUsage := tokenParser.ParseSSELine(line); tokenUsage != nil {
			foundTokenUsage = true
			slog.InfoContext(ctx, fmt.Sprintf("✅ [SSE解析成功] 端点: %s, 连接: %s - 成功解析token信息", endpointName, connID))
			
			// Record token usage in monitoring middleware if available
			if mm, ok := h.retryHandler.monitoringMiddleware.(interface{
				RecordTokenUsage(connID string, endpoint string, tokens *monitor.TokenUsage)
			}); ok && connID != "" {
				mm.RecordTokenUsage(connID, endpointName, tokenUsage)
			}
			
			// Token usage has already been recorded in usage tracker by TokenParser
			// So we can return successfully here
			return
		}
	}
	
	// If we found an error event, the parseErrorEvent method would have already handled it
	if hasErrorEvent {
		slog.InfoContext(ctx, fmt.Sprintf("❌ [SSE错误处理] 端点: %s, 连接: %s - 错误事件已处理", endpointName, connID))
		return
	}
	
	if !foundTokenUsage {
		slog.InfoContext(ctx, fmt.Sprintf("🚫 [SSE解析] 端点: %s, 连接: %s - 未找到token usage信息", endpointName, connID))
	}
}

// parseJSONTokens parses single JSON response for token usage
func (h *Handler) parseJSONTokens(ctx context.Context, responseBody, endpointName, connID string) {
	// Simulate SSE parsing for a single JSON response
	tokenParser := NewTokenParserWithUsageTracker(connID, h.usageTracker)
	
	slog.InfoContext(ctx, "🔍 [JSON解析] 尝试解析JSON响应")
	
	// 🆕 First extract model information directly from JSON
	var jsonResp map[string]interface{}
	if err := json.Unmarshal([]byte(responseBody), &jsonResp); err == nil {
		if model, ok := jsonResp["model"].(string); ok && model != "" {
			tokenParser.SetModelName(model)
			slog.InfoContext(ctx, "📋 [JSON解析] 提取到模型信息", "model", model)
		}
	}
	
	// Wrap JSON as SSE message_delta event
	tokenParser.ParseSSELine("event: message_delta")
	tokenParser.ParseSSELine("data: " + responseBody)
	if tokenUsage := tokenParser.ParseSSELine(""); tokenUsage != nil {
		// Record token usage
		if mm, ok := h.retryHandler.monitoringMiddleware.(interface{
			RecordTokenUsage(connID string, endpoint string, tokens *monitor.TokenUsage)
		}); ok && connID != "" {
			mm.RecordTokenUsage(connID, endpointName, tokenUsage)
			slog.InfoContext(ctx, "✅ [JSON解析] 成功记录token使用", 
				"endpoint", endpointName, 
				"inputTokens", tokenUsage.InputTokens, 
				"outputTokens", tokenUsage.OutputTokens,
				"cacheCreation", tokenUsage.CacheCreationTokens,
				"cacheRead", tokenUsage.CacheReadTokens)
		}
	} else {
		slog.DebugContext(ctx, "🚫 [JSON解析] JSON中未找到token usage信息")
	}
}

// copyHeaders copies headers from source to destination request
func (h *Handler) copyHeaders(src *http.Request, dst *http.Request, ep *endpoint.Endpoint) {
	// List of headers to skip/remove
	skipHeaders := map[string]bool{
		"host":          true, // We'll set this based on target endpoint
		"authorization": true, // We'll add our own if configured
		"x-api-key":     true, // Remove sensitive client API keys
	}
	
	// Copy all headers except those we want to skip
	for key, values := range src.Header {
		if skipHeaders[strings.ToLower(key)] {
			continue
		}
		
		for _, value := range values {
			dst.Header.Add(key, value)
		}
	}

	// Set Host header based on target endpoint URL
	if u, err := url.Parse(ep.Config.URL); err == nil {
		dst.Header.Set("Host", u.Host)
		// Also set the Host field directly on the request for proper HTTP/1.1 behavior
		dst.Host = u.Host
	}

	// Add or override Authorization header with dynamically resolved token
	token := h.endpointManager.GetTokenForEndpoint(ep)
	if token != "" {
		dst.Header.Set("Authorization", "Bearer "+token)
	}

	// Add or override X-Api-Key header with dynamically resolved api-key
	apiKey := h.endpointManager.GetApiKeyForEndpoint(ep)
	if apiKey != "" {
		dst.Header.Set("X-Api-Key", apiKey)
	}

	// Add custom headers from endpoint configuration
	for key, value := range ep.Config.Headers {
		dst.Header.Set(key, value)
	}

	// Remove hop-by-hop headers
	hopByHopHeaders := []string{
		"Connection",
		"Keep-Alive", 
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Te",
		"Trailers",
		"Transfer-Encoding",
		"Upgrade",
	}

	for _, header := range hopByHopHeaders {
		dst.Header.Del(header)
	}
}

// UpdateConfig updates the handler configuration
func (h *Handler) UpdateConfig(cfg *config.Config) {
	h.config = cfg
	
	// Update retry handler with new config
	h.retryHandler.UpdateConfig(cfg)
}

// notifyConnectionEvent notifies the web interface about connection events
func (h *Handler) notifyConnectionEvent(connID, endpoint, changeType string, err error) {
	if h.webNotifier == nil || !h.webNotifier.IsEventManagerActive() {
		return
	}
	
	// 构建事件数据
	data := map[string]interface{}{
		"connection_id": connID,
		"endpoint":      endpoint,
		"change_type":   changeType,
		"timestamp":     time.Now().Format("2006-01-02 15:04:05"),
	}
	
	if err != nil {
		data["error"] = err.Error()
	}
	
	// 异步发送通知，避免阻塞请求处理
	go h.webNotifier.BroadcastConnectionUpdateSmart(data, changeType)
}