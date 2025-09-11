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
	"math"
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

// Handler handles HTTP proxy requests
type Handler struct {
	endpointManager *endpoint.Manager
	config          *config.Config
	retryHandler    *RetryHandler
	usageTracker    *tracking.UsageTracker
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

// GetRetryHandler returns the retry handler for accessing suspended request counts
func (h *Handler) GetRetryHandler() *RetryHandler {
	return h.retryHandler
}

// ServeHTTP implements the http.Handler interface
// 统一请求分发逻辑 - 整合流式处理、错误恢复和生命周期管理
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 创建请求上下文
	ctx := r.Context()
	
	// 获取连接ID
	connID := ""
	if connIDValue, ok := r.Context().Value("conn_id").(string); ok {
		connID = connIDValue
	}
	
	// 创建统一的请求生命周期管理器
	lifecycleManager := NewRequestLifecycleManager(h.usageTracker, connID)
	
	// 开始请求跟踪
	clientIP := r.RemoteAddr
	userAgent := r.Header.Get("User-Agent")
	lifecycleManager.StartRequest(clientIP, userAgent)
	
	// 克隆请求体用于重试
	var bodyBytes []byte
	if r.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(r.Body)
		if err != nil {
			lifecycleManager.HandleError(err)
			http.Error(w, "Failed to read request body", http.StatusInternalServerError)
			return
		}
		r.Body.Close()
	}

	// 检测是否为SSE流式请求
	isSSE := h.detectSSERequest(r, bodyBytes)
	
	// 统一请求处理
	if isSSE {
		// 流式请求处理
		h.handleStreamingRequest(ctx, w, r, bodyBytes, lifecycleManager)
	} else {
		// 常规请求处理
		h.handleRegularRequestUnified(ctx, w, r, bodyBytes, lifecycleManager)
	}
}

// detectSSERequest 统一SSE请求检测逻辑
func (h *Handler) detectSSERequest(r *http.Request, bodyBytes []byte) bool {
	// 检查多种SSE请求模式:
	acceptHeader := r.Header.Get("Accept")
	cacheControlHeader := r.Header.Get("Cache-Control")
	streamHeader := r.Header.Get("stream")
	
	// 1. Accept头包含text/event-stream
	if strings.Contains(acceptHeader, "text/event-stream") {
		return true
	}
	
	// 2. Cache-Control头包含no-cache (常见于SSE)
	if strings.Contains(cacheControlHeader, "no-cache") {
		return true
	}
	
	// 3. stream头设置为true
	if streamHeader == "true" {
		return true
	}
	
	// 4. 请求体包含stream参数为true
	bodyStr := string(bodyBytes)
	if strings.Contains(bodyStr, `"stream":true`) || strings.Contains(bodyStr, `"stream": true`) {
		return true
	}
	
	return false
}

// handleStreamingRequest 统一流式请求处理
// 使用V2架构整合错误恢复机制和生命周期管理的流式处理
func (h *Handler) handleStreamingRequest(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, lifecycleManager *RequestLifecycleManager) {
	connID := lifecycleManager.GetRequestID()
	
	slog.Info(fmt.Sprintf("🌊 [流式架构] [%s] 使用streaming v2架构", connID))
	slog.Info(fmt.Sprintf("🌊 [流式处理] [%s] 开始流式请求处理", connID))
	h.handleStreamingV2(ctx, w, r, bodyBytes, lifecycleManager)
}

// handleStreamingV2 流式处理（带错误恢复）
func (h *Handler) handleStreamingV2(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, lifecycleManager *RequestLifecycleManager) {
	connID := lifecycleManager.GetRequestID()
	
	// 设置流式响应头
	h.setStreamingHeaders(w)
	
	// 获取Flusher - 如果不支持，使用无flush模式继续流式处理
	flusher, ok := w.(http.Flusher)
	if !ok {
		slog.Warn(fmt.Sprintf("🌊 [Flusher不支持] [%s] 将使用无flush模式的流式处理", connID))
		// 创建一个mock flusher，不执行实际flush操作
		flusher = &noOpFlusher{}
	}
	
	// 继续执行流式请求处理
	h.executeStreamingWithRetry(ctx, w, r, bodyBytes, lifecycleManager, flusher)
}

// noOpFlusher 是一个不执行实际flush操作的flusher实现
type noOpFlusher struct{}

func (f *noOpFlusher) Flush() {
	// 不执行任何操作，避免panic但保持流式处理逻辑
}


// setStreamingHeaders 设置流式响应头
func (h *Handler) setStreamingHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Cache-Control")
}

// executeStreamingWithRetry 执行带重试的流式处理
func (h *Handler) executeStreamingWithRetry(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, lifecycleManager *RequestLifecycleManager, flusher http.Flusher) {
	connID := lifecycleManager.GetRequestID()
	
	// 获取健康端点
	var endpoints []*endpoint.Endpoint
	if h.endpointManager.GetConfig().Strategy.Type == "fastest" && h.endpointManager.GetConfig().Strategy.FastTestEnabled {
		endpoints = h.endpointManager.GetFastestEndpointsWithRealTimeTest(ctx)
	} else {
		endpoints = h.endpointManager.GetHealthyEndpoints()
	}
	
	if len(endpoints) == 0 {
		lifecycleManager.HandleError(fmt.Errorf("no healthy endpoints available"))
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "data: error: No healthy endpoints available\n\n")
		flusher.Flush()
		return
	}
	
	slog.Info(fmt.Sprintf("🌊 [流式开始] [%s] 流式请求开始，端点数: %d", connID, len(endpoints)))
	
	// 🔧 [重试逻辑修复] 对每个端点进行max_attempts次重试，而不是只尝试一次
	// 尝试端点直到成功
	var lastErr error  // 声明在外层作用域，供最终错误处理使用
	for i := 0; i < len(endpoints); i++ {
		ep := endpoints[i]
		// 更新生命周期管理器信息
		lifecycleManager.SetEndpoint(ep.Config.Name, ep.Config.Group)
		lifecycleManager.UpdateStatus("forwarding", i, 0)
		
		// ✅ [同端点重试] 对当前端点进行max_attempts次重试
		endpointSuccess := false
		
		for attempt := 1; attempt <= h.config.Retry.MaxAttempts; attempt++ {
			// 检查是否被取消
			select {
			case <-ctx.Done():
				slog.Info(fmt.Sprintf("🚫 [客户端取消检测] [%s] 检测到客户端取消，立即停止重试", connID))
				lifecycleManager.UpdateStatus("cancelled", i+1, attempt-1)
				fmt.Fprintf(w, "data: cancelled: 客户端取消请求\n\n")
				flusher.Flush()
				return
			default:
			}
			
			// 尝试连接端点
			resp, err := h.forwardRequestToEndpoint(ctx, r, bodyBytes, ep)
			if err == nil {
				// ✅ 成功！开始处理响应
				endpointSuccess = true
				slog.Info(fmt.Sprintf("✅ [流式成功] [%s] 端点: %s (组: %s), 尝试次数: %d", 
					connID, ep.Config.Name, ep.Config.Group, attempt))
					
				lifecycleManager.UpdateStatus("processing", i+1, attempt)
				
				// 处理流式响应 - 使用现有的流式处理逻辑
				w.WriteHeader(resp.StatusCode)
				
				// 创建Token解析器和流式处理器
				tokenParser := NewTokenParserWithUsageTracker(connID, h.usageTracker)
				processor := NewStreamProcessor(tokenParser, h.usageTracker, w, flusher, connID, ep.Config.Name)
				
				slog.Info(fmt.Sprintf("🚀 [开始流式处理] [%s] 端点: %s", connID, ep.Config.Name))
				
				// 执行流式处理
				if err := processor.ProcessStreamWithRetry(ctx, resp); err != nil {
					slog.Warn(fmt.Sprintf("🔄 [流式处理失败] [%s] 端点: %s, 错误: %v", 
						connID, ep.Config.Name, err))
					
					// 流式处理失败，但HTTP连接已成功建立，记录为processing状态
					lifecycleManager.UpdateStatus("error", i+1, resp.StatusCode)
					fmt.Fprintf(w, "data: error: 流式处理失败: %v\n\n", err)
					flusher.Flush()
					return
				}
				
				// 处理成功完成
				lifecycleManager.UpdateStatus("completed", i+1, resp.StatusCode)
				return
			}
			
			// ❌ 出现错误，进行错误分类
			lastErr = err
			errorRecovery := NewErrorRecoveryManager(h.usageTracker)
			errorCtx := errorRecovery.ClassifyError(err, connID, ep.Config.Name, ep.Config.Group, attempt-1)
			
			// 检查是否为客户端取消错误
			if errorCtx.ErrorType == ErrorTypeClientCancel {
				slog.Info(fmt.Sprintf("🚫 [客户端取消检测] [%s] 检测到客户端取消，立即停止重试", connID))
				lifecycleManager.UpdateStatus("cancelled", i+1, attempt-1)
				fmt.Fprintf(w, "data: cancelled: 客户端取消请求\n\n")
				flusher.Flush()
				return
			}
			
			// 非取消错误：记录重试状态
			lifecycleManager.HandleError(err)
			lifecycleManager.UpdateStatus("retry", i+1, attempt-1)
			
			slog.Warn(fmt.Sprintf("🔄 [流式重试] [%s] 端点: %s, 尝试: %d/%d, 错误: %v", 
				connID, ep.Config.Name, attempt, h.config.Retry.MaxAttempts, err))
			
			// 如果不是最后一次尝试，等待重试延迟
			if attempt < h.config.Retry.MaxAttempts {
				// 计算重试延迟
				delay := h.calculateRetryDelay(attempt)
				slog.Info(fmt.Sprintf("⏳ [等待重试] [%s] 端点: %s, 延迟: %v", 
					connID, ep.Config.Name, delay))
				
				// 向客户端发送重试信息
				fmt.Fprintf(w, "data: retry: 重试端点 %s (尝试 %d/%d)，等待 %v...\n\n", 
					ep.Config.Name, attempt+1, h.config.Retry.MaxAttempts, delay)
				flusher.Flush()
				
				// 等待延迟，同时检查取消
				select {
				case <-ctx.Done():
					slog.Info(fmt.Sprintf("🚫 [重试取消] [%s] 等待重试期间检测到取消", connID))
					lifecycleManager.UpdateStatus("cancelled", i+1, attempt)
					fmt.Fprintf(w, "data: cancelled: 客户端取消请求\n\n")
					flusher.Flush()
					return
				case <-time.After(delay):
					// 继续下一次重试
				}
			}
		}
		
		// 🔧 当前端点所有重试都失败了
		if !endpointSuccess {
			slog.Warn(fmt.Sprintf("❌ [端点失败] [%s] 端点: %s 所有 %d 次重试均失败", 
				connID, ep.Config.Name, h.config.Retry.MaxAttempts))
			
			// 如果不是最后一个端点，尝试下一个端点
			if i < len(endpoints)-1 {
				fmt.Fprintf(w, "data: retry: 切换到备用端点: %s\n\n", endpoints[i+1].Config.Name)
				flusher.Flush()
				continue
			}
		}
	}
	
	// 🔧 所有当前端点都失败，检查是否应该挂起请求
	// 注意：客户端取消错误已在上面统一处理，这里不会执行到
	
	// 创建临时的RetryHandler来访问挂起逻辑
	tempRetryHandler := NewRetryHandler(h.config)
	tempRetryHandler.SetEndpointManager(h.endpointManager)
	tempRetryHandler.SetUsageTracker(h.usageTracker)
	
	// 检查是否应该挂起请求
	if tempRetryHandler.shouldSuspendRequest(ctx) {
		fmt.Fprintf(w, "data: suspend: 当前所有组均不可用，请求已挂起等待组切换...\n\n")
		flusher.Flush()
		
		slog.Info(fmt.Sprintf("⏸️ [流式挂起] [%s] 请求已挂起等待组切换", connID))
		
		// 等待组切换
		if tempRetryHandler.waitForGroupSwitch(ctx, connID) {
			slog.Info(fmt.Sprintf("🚀 [挂起恢复] [%s] 组切换完成，重新获取端点", connID))
			fmt.Fprintf(w, "data: resume: 组切换完成，恢复处理...\n\n")
			flusher.Flush()
			
			// 重新获取健康端点
			var newEndpoints []*endpoint.Endpoint
			if h.endpointManager.GetConfig().Strategy.Type == "fastest" && h.endpointManager.GetConfig().Strategy.FastTestEnabled {
				newEndpoints = h.endpointManager.GetFastestEndpointsWithRealTimeTest(ctx)
			} else {
				newEndpoints = h.endpointManager.GetHealthyEndpoints()
			}
			
			if len(newEndpoints) > 0 {
				// 更新端点列表，重新开始处理
				endpoints = newEndpoints
				slog.Info(fmt.Sprintf("🔄 [重新开始] [%s] 获取到 %d 个新端点，重新开始流式处理", connID, len(newEndpoints)))
				
				// 🔧 [生命周期修复] 恢复时必须更新生命周期管理器的端点信息
				// 设置第一个新端点的信息到生命周期管理器
				firstEndpoint := newEndpoints[0]
				lifecycleManager.SetEndpoint(firstEndpoint.Config.Name, firstEndpoint.Config.Group)
				
				// 重新获取健康端点并重新尝试（递归调用）
				h.executeStreamingWithRetry(ctx, w, r, bodyBytes, lifecycleManager, flusher)
				return
			}
		}
	}
	
	slog.Warn(fmt.Sprintf("⚠️ [挂起失败] [%s] 挂起等待超时或失败", connID))
	
	// 最终失败处理 - 生命周期管理器已处理错误分类
	lifecycleManager.UpdateStatus("error", len(endpoints), http.StatusBadGateway)
	fmt.Fprintf(w, "data: error: All endpoints failed, last error: %v\n\n", lastErr)
	flusher.Flush()
	return
}

// handleRegularRequestUnified 统一常规请求处理
// 整合错误恢复机制和生命周期管理的常规请求处理
func (h *Handler) handleRegularRequestUnified(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, lifecycleManager *RequestLifecycleManager) {
	connID := lifecycleManager.GetRequestID()
	var selectedEndpointName string
	
	slog.Info(fmt.Sprintf("🔄 [常规架构] [%s] 使用unified v2架构", connID))
	
	// 创建错误恢复管理器
	errorRecovery := NewErrorRecoveryManager(h.usageTracker)
	
	// 使用重试处理器执行请求
	operation := func(ep *endpoint.Endpoint, connectionID string) (*http.Response, error) {
		// 更新生命周期管理器信息
		selectedEndpointName = ep.Config.Name
		lifecycleManager.SetEndpoint(ep.Config.Name, ep.Config.Group)
		lifecycleManager.UpdateStatus("forwarding", 0, 0)
		
		// 更新监控中间件的连接端点信息
		if mm, ok := h.retryHandler.monitoringMiddleware.(interface{
			UpdateConnectionEndpoint(connID, endpoint string)
		}); ok && connectionID != "" {
			mm.UpdateConnectionEndpoint(connectionID, ep.Config.Name)
		}
		
		// 创建目标请求
		targetURL := ep.Config.URL + r.URL.Path
		if r.URL.RawQuery != "" {
			targetURL += "?" + r.URL.RawQuery
		}

		req, err := http.NewRequestWithContext(ctx, r.Method, targetURL, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		// 复制和修改头部
		h.copyHeaders(r, req, ep)

		// 创建HTTP传输
		httpTransport, err := transport.CreateTransport(h.config)
		if err != nil {
			return nil, fmt.Errorf("failed to create transport: %w", err)
		}
		
		client := &http.Client{
			Timeout:   ep.Config.Timeout,
			Transport: httpTransport,
		}

		// 执行请求
		resp, err := client.Do(req)
		if err != nil {
			// 分类错误并记录
			errorCtx := errorRecovery.ClassifyError(err, connID, ep.Config.Name, ep.Config.Group, 0)
			lifecycleManager.HandleError(err)
			
			slog.Warn("🔄 Regular request failed", "request_id", connID, "endpoint", ep.Config.Name, 
				"error_type", errorRecovery.getErrorTypeName(errorCtx.ErrorType), "error", err)
			
			return nil, fmt.Errorf("request failed: %w", err)
		}

		return resp, nil
	}

	// 执行请求与重试逻辑
	finalResp, lastErr := h.retryHandler.ExecuteWithContext(ctx, operation, connID)
	
	// 在上下文中存储选中的端点信息用于日志记录
	if selectedEndpointName != "" {
		*r = *r.WithContext(context.WithValue(r.Context(), "selected_endpoint", selectedEndpointName))
	}
	
	// 处理错误情况
	if lastErr != nil {
		errorCtx := errorRecovery.ClassifyError(lastErr, connID, selectedEndpointName, "", 0)
		lifecycleManager.HandleError(lastErr)
		errorRecovery.HandleFinalFailure(errorCtx)
		
		// 根据错误类型返回适当的状态码
		if strings.Contains(lastErr.Error(), "no healthy endpoints") {
			http.Error(w, "Service Unavailable: No healthy endpoints available", http.StatusServiceUnavailable)
		} else {
			http.Error(w, "All endpoints failed: "+lastErr.Error(), http.StatusBadGateway)
		}
		return
	}

	if finalResp == nil {
		err := fmt.Errorf("no response received from any endpoint")
		lifecycleManager.HandleError(err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	defer finalResp.Body.Close()

	// 更新状态为处理中
	lifecycleManager.UpdateStatus("processing", 0, finalResp.StatusCode)

	// 复制响应头（排除Content-Encoding用于gzip处理）
	h.copyResponseHeaders(finalResp, w)

	// 写入状态码
	w.WriteHeader(finalResp.StatusCode)

	// 读取并处理响应体
	responseBytes, err := h.processResponseBody(finalResp)
	if err != nil {
		lifecycleManager.HandleError(fmt.Errorf("failed to process response: %w", err))
		slog.Error("Failed to process response body", "request_id", connID, "error", err)
		return
	}

	// 写入响应体到客户端
	if _, err := w.Write(responseBytes); err != nil {
		lifecycleManager.HandleError(fmt.Errorf("failed to write response: %w", err))
		slog.Error("Failed to write response to client", "request_id", connID, "error", err)
		return
	}

	// 对于常规请求，尝试解析Token信息（如果存在）
	h.analyzeResponseForTokensUnified(responseBytes, connID, selectedEndpointName, lifecycleManager)

	// 完成请求
	lifecycleManager.UpdateStatus("completed", 0, finalResp.StatusCode)
	
	slog.Info(fmt.Sprintf("✅ [常规请求完成] [%s] 端点: %s, 状态码: %d, 响应大小: %d字节", 
		connID, selectedEndpointName, finalResp.StatusCode, len(responseBytes)))
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
	
	slog.InfoContext(ctx, fmt.Sprintf("🔍 [JSON解析] [%s] 尝试解析JSON响应", connID))
	
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
		slog.DebugContext(ctx, fmt.Sprintf("🚫 [JSON解析] [%s] JSON中未找到token usage信息", connID))
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

// forwardRequestToEndpoint 转发请求到指定端点
func (h *Handler) forwardRequestToEndpoint(ctx context.Context, r *http.Request, bodyBytes []byte, ep *endpoint.Endpoint) (*http.Response, error) {
	// 创建目标URL
	targetURL := ep.Config.URL + r.URL.Path
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	// 创建请求
	req, err := http.NewRequestWithContext(ctx, r.Method, targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 复制和修改头部
	h.copyHeaders(r, req, ep)

	// 创建HTTP传输
	httpTransport, err := transport.CreateTransport(h.config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport: %w", err)
	}
	
	// 优化传输设置用于流式处理
	httpTransport.DisableKeepAlives = false
	httpTransport.MaxIdleConns = 10
	httpTransport.MaxIdleConnsPerHost = 2
	httpTransport.IdleConnTimeout = 0 // 无空闲超时
	httpTransport.TLSHandshakeTimeout = 10 * time.Second
	httpTransport.ExpectContinueTimeout = 1 * time.Second
	httpTransport.ResponseHeaderTimeout = 15 * time.Second
	httpTransport.DisableCompression = true // 禁用压缩以防缓冲延迟
	httpTransport.WriteBufferSize = 4096    // 较小的写缓冲区
	httpTransport.ReadBufferSize = 4096     // 较小的读缓冲区
	
	client := &http.Client{
		Timeout:   0, // 流式请求无超时
		Transport: httpTransport,
	}

	// 执行请求
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	// 检查响应状态
	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, fmt.Errorf("endpoint returned error: %d", resp.StatusCode)
	}

	return resp, nil
}

// copyResponseHeaders 复制响应头到客户端
func (h *Handler) copyResponseHeaders(resp *http.Response, w http.ResponseWriter) {
	for key, values := range resp.Header {
		// 跳过一些不应该复制的头部
		switch key {
		case "Content-Length", "Transfer-Encoding", "Connection", "Content-Encoding":
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
}

// processResponseBody 处理响应体（包括解压缩）
func (h *Handler) processResponseBody(resp *http.Response) ([]byte, error) {
	var reader io.Reader = resp.Body
	
	// 检查内容编码并解压缩
	encoding := resp.Header.Get("Content-Encoding")
	switch encoding {
	case "gzip":
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gzReader.Close()
		reader = gzReader
		
	case "deflate":
		reader = flate.NewReader(resp.Body)
		
	case "br":
		reader = brotli.NewReader(resp.Body)
		
	case "compress":
		reader = lzw.NewReader(resp.Body, lzw.LSB, 8)
	}
	
	// 读取响应体
	responseBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	
	return responseBytes, nil
}

// analyzeResponseForTokensWithLifecycle analyzes response with accurate duration from lifecycle manager
func (h *Handler) analyzeResponseForTokensWithLifecycle(ctx context.Context, responseBody, endpointName string, r *http.Request, lifecycleManager *RequestLifecycleManager) {
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
	   strings.Contains(responseBody, "event:message_delta") ||
	   strings.Contains(responseBody, "event: message_start") ||
	   strings.Contains(responseBody, "event: message_delta") {
		h.parseSSETokens(ctx, responseBody, endpointName, connID)
		return
	}
	
	// Method 2: Direct JSON analysis for non-SSE responses
	slog.InfoContext(ctx, fmt.Sprintf("🔍 [JSON解析] [%s] 尝试解析JSON响应", connID))
	
	// Try to parse as JSON and extract model information
	var jsonData map[string]interface{}
	var model string
	
	if err := json.Unmarshal([]byte(responseBody), &jsonData); err == nil {
		// Extract model information if available
		if modelValue, exists := jsonData["model"]; exists {
			if modelStr, ok := modelValue.(string); ok {
				model = modelStr
				slog.InfoContext(ctx, "📋 [JSON解析] 提取到模型信息", "model", model)
			}
		}
	}
	
	// Wrap JSON as SSE message_delta event
	tokenParser := NewTokenParser()
	tokenParser.ParseSSELine("event: message_delta")
	tokenParser.ParseSSELine("data: " + responseBody)
	if tokenUsage := tokenParser.ParseSSELine(""); tokenUsage != nil {
		// Record token usage to monitoring middleware
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
		
		// 🔧 修复：同时保存到数据库，使用准确的处理时间
		if h.usageTracker != nil && connID != "" && lifecycleManager != nil {
			// 转换Token格式
			dbTokens := &tracking.TokenUsage{
				InputTokens:         tokenUsage.InputTokens,
				OutputTokens:        tokenUsage.OutputTokens,
				CacheCreationTokens: tokenUsage.CacheCreationTokens,
				CacheReadTokens:     tokenUsage.CacheReadTokens,
			}
			
			// 使用提取的模型名称，如果没有则使用default
			modelName := "default"
			if model != "" {
				modelName = model
			}
			
			// 🎯 使用lifecycleManager获取准确的处理时间
			duration := lifecycleManager.GetDuration()
			
			// 保存到数据库
			h.usageTracker.RecordRequestComplete(connID, modelName, dbTokens, duration)
			slog.InfoContext(ctx, "💾 [数据库保存] JSON解析的Token信息已保存到数据库",
				"request_id", connID, "model", modelName, 
				"inputTokens", dbTokens.InputTokens, "outputTokens", dbTokens.OutputTokens,
				"duration", duration)
		}
	} else {
		slog.DebugContext(ctx, fmt.Sprintf("🚫 [JSON解析] [%s] JSON中未找到token usage信息", connID))
		
		// Fallback: No token information found, mark request as completed with default model
		if h.usageTracker != nil && connID != "" && lifecycleManager != nil {
			emptyTokens := &tracking.TokenUsage{
				InputTokens: 0, OutputTokens: 0, 
				CacheCreationTokens: 0, CacheReadTokens: 0,
			}
			duration := lifecycleManager.GetDuration()
			h.usageTracker.RecordRequestComplete(connID, "non_token_response", emptyTokens, duration)
			slog.InfoContext(ctx, fmt.Sprintf("✅ [无Token完成] 连接: %s 已标记为完成状态，模型: non_token_response, 处理时间: %v", 
				connID, duration))
		}
	}
}

// analyzeResponseForTokensUnified 简化版本的Token分析（用于统一接口）
func (h *Handler) analyzeResponseForTokensUnified(responseBytes []byte, connID, endpointName string, lifecycleManager *RequestLifecycleManager) {
	if len(responseBytes) == 0 {
		return
	}
	
	responseStr := string(responseBytes)
	
	// 使用现有的Token分析方法（创建一个临时的Request对象）
	req := &http.Request{} // 创建一个空的request对象
	req = req.WithContext(context.WithValue(context.Background(), "conn_id", connID))
	
	// 调用现有的分析方法，传入lifecycleManager以获取准确的duration
	h.analyzeResponseForTokensWithLifecycle(req.Context(), responseStr, endpointName, req, lifecycleManager)
}

// calculateRetryDelay 计算重试延迟（指数退避算法）
func (h *Handler) calculateRetryDelay(attempt int) time.Duration {
	// 使用与RetryHandler相同的计算逻辑
	multiplier := math.Pow(h.config.Retry.Multiplier, float64(attempt-1))
	delay := time.Duration(float64(h.config.Retry.BaseDelay) * multiplier)
	
	// 限制在最大延迟范围内
	if delay > h.config.Retry.MaxDelay {
		delay = h.config.Retry.MaxDelay
	}
	
	return delay
}