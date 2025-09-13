package handlers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/tracking"
	"cc-forwarder/internal/transport"
)

// RegularHandler 常规请求处理器
// 负责处理所有常规请求，包含错误恢复机制和生命周期管理
type RegularHandler struct {
	config               *config.Config
	endpointManager      *endpoint.Manager
	forwarder            *Forwarder
	usageTracker         *tracking.UsageTracker
	responseProcessor    ResponseProcessor
	tokenAnalyzer        TokenAnalyzer
	retryHandler         RetryHandler
	errorRecoveryFactory ErrorRecoveryFactory
}

// NewRegularHandler 创建新的RegularHandler实例
func NewRegularHandler(
	cfg *config.Config,
	endpointManager *endpoint.Manager,
	forwarder *Forwarder,
	usageTracker *tracking.UsageTracker,
	responseProcessor ResponseProcessor,
	tokenAnalyzer TokenAnalyzer,
	retryHandler RetryHandler,
	errorRecoveryFactory ErrorRecoveryFactory,
) *RegularHandler {
	return &RegularHandler{
		config:               cfg,
		endpointManager:      endpointManager,
		forwarder:           forwarder,
		usageTracker:        usageTracker,
		responseProcessor:   responseProcessor,
		tokenAnalyzer:       tokenAnalyzer,
		retryHandler:        retryHandler,
		errorRecoveryFactory: errorRecoveryFactory,
	}
}

// HandleRegularRequestUnified 统一常规请求处理
// 整合错误恢复机制和生命周期管理的常规请求处理
func (rh *RegularHandler) HandleRegularRequestUnified(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, lifecycleManager RequestLifecycleManager) {
	connID := lifecycleManager.GetRequestID()
	var selectedEndpointName string
	
	slog.Info(fmt.Sprintf("🔄 [常规架构] [%s] 使用unified v2架构", connID))
	
	// 创建错误恢复管理器
	errorRecovery := rh.errorRecoveryFactory.NewErrorRecoveryManager(rh.usageTracker)
	
	// 使用重试处理器执行请求
	operation := func(ep *endpoint.Endpoint, connectionID string) (*http.Response, error) {
		// 更新生命周期管理器信息
		selectedEndpointName = ep.Config.Name
		lifecycleManager.SetEndpoint(ep.Config.Name, ep.Config.Group)
		lifecycleManager.UpdateStatus("forwarding", 0, 0)
		
		// 更新监控中间件的连接端点信息
		if mm, ok := rh.retryHandler.(interface{
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
		rh.forwarder.CopyHeaders(r, req, ep)

		// 创建HTTP传输
		httpTransport, err := transport.CreateTransport(rh.config)
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
				"error_type", errorRecovery.GetErrorTypeName(errorCtx.ErrorType), "error", err)
			
			return nil, fmt.Errorf("request failed: %w", err)
		}

		return resp, nil
	}

	// 执行请求与重试逻辑
	finalResp, lastErr := rh.retryHandler.ExecuteWithContext(ctx, operation, connID)
	
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
	rh.responseProcessor.CopyResponseHeaders(finalResp, w)

	// 写入状态码
	w.WriteHeader(finalResp.StatusCode)

	// 读取并处理响应体
	responseBytes, err := rh.responseProcessor.ProcessResponseBody(finalResp)
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

	// ✅ 异步Token解析优化：不阻塞连接关闭
	go func() {
		slog.Debug(fmt.Sprintf("🔄 [异步Token解析] [%s] 开始后台Token解析", connID))
		
		// 对于常规请求，异步解析Token信息（如果存在）
		tokenUsage, modelName := rh.tokenAnalyzer.AnalyzeResponseForTokensUnified(responseBytes, connID, selectedEndpointName)
		
		// 使用生命周期管理器完成请求
		if tokenUsage != nil {
			// 设置模型名称并完成请求
			// 使用对比方法，检测并警告模型不一致情况
			if modelName != "unknown" && modelName != "" {
				lifecycleManager.SetModelWithComparison(modelName, "常规响应解析")
			}
			lifecycleManager.CompleteRequest(tokenUsage)
			slog.Info(fmt.Sprintf("✅ [常规请求Token完成] [%s] 端点: %s, 模型: %s, 输入: %d, 输出: %d", 
				connID, selectedEndpointName, modelName, tokenUsage.InputTokens, tokenUsage.OutputTokens))
		} else {
			// 处理非Token响应
			lifecycleManager.HandleNonTokenResponse(string(responseBytes))
			slog.Info(fmt.Sprintf("✅ [常规请求完成] [%s] 端点: %s, 响应类型: %s", 
				connID, selectedEndpointName, modelName))
		}
	}()
}


// HandleRegularRequest handles non-streaming requests
func (rh *RegularHandler) HandleRegularRequest(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte) {
	var selectedEndpointName string
	
	// Get connection ID from request context (set by logging middleware)
	connID := ""
	if connIDValue, ok := r.Context().Value("conn_id").(string); ok {
		connID = connIDValue
	}
	
	// TODO: 创建重试处理器
	
	operation := func(ep *endpoint.Endpoint, connectionID string) (*http.Response, error) {
		// Store the selected endpoint name for logging
		selectedEndpointName = ep.Config.Name
		
		// TODO: Update connection endpoint in monitoring (if we have a monitoring middleware)
		
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
		rh.forwarder.CopyHeaders(r, req, ep)

		// Create HTTP client with timeout and proxy support
		httpTransport, err := transport.CreateTransport(rh.config)
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

	// Execute with retry logic - 使用retryHandler
	finalResp, lastErr := rh.retryHandler.ExecuteWithContext(ctx, operation, connID)
	
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
	
	// 使用响应处理器读取响应
	bodyBytes, err := io.ReadAll(finalResp.Body)
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
	
	// TODO: Analyze the complete response for token usage
	slog.DebugContext(ctx, fmt.Sprintf("🔍 [开始Token分析] [%s] 端点: %s", connID, selectedEndpointName))
	slog.DebugContext(ctx, fmt.Sprintf("✅ [Token分析完成] [%s] 端点: %s", connID, selectedEndpointName))
	
	// Write the body to client
	_, writeErr := w.Write(bodyBytes)
	if writeErr != nil {
		// Log error but don't return error response as headers are already sent
		slog.Error("Failed to write response to client", "request_id", connID, "error", writeErr)
	}
}