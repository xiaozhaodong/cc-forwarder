package handlers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/tracking"
	"cc-forwarder/internal/transport"
)

// RegularHandler 常规请求处理器
// 负责处理所有常规请求，包含错误恢复机制和生命周期管理
type RegularHandler struct {
	config                   *config.Config
	endpointManager          *endpoint.Manager
	forwarder                *Forwarder
	usageTracker             *tracking.UsageTracker
	responseProcessor        ResponseProcessor
	tokenAnalyzer            TokenAnalyzer
	retryHandler             RetryHandler
	errorRecoveryFactory     ErrorRecoveryFactory
	retryManagerFactory      RetryManagerFactory
	suspensionManagerFactory SuspensionManagerFactory
	// 🔧 [修复] 共享SuspensionManager实例，确保全局挂起限制生效
	sharedSuspensionManager  SuspensionManager
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
	retryManagerFactory RetryManagerFactory,
	suspensionManagerFactory SuspensionManagerFactory,
	// 🔧 [Critical修复] 直接接受共享的SuspensionManager实例
	sharedSuspensionManager SuspensionManager,
) *RegularHandler {
	return &RegularHandler{
		config:                   cfg,
		endpointManager:          endpointManager,
		forwarder:                forwarder,
		usageTracker:             usageTracker,
		responseProcessor:        responseProcessor,
		tokenAnalyzer:            tokenAnalyzer,
		retryHandler:             retryHandler,
		errorRecoveryFactory:     errorRecoveryFactory,
		retryManagerFactory:      retryManagerFactory,
		suspensionManagerFactory: suspensionManagerFactory,
		// 🔧 [Critical修复] 使用传入的共享SuspensionManager实例
		// 确保常规请求与流式请求共享同一个全局挂起计数器
		sharedSuspensionManager:  sharedSuspensionManager,
	}
}

// HandleRegularRequestUnified 统一常规请求处理
// 实现与StreamingHandler相同的重试循环模式，应用所有Critical修复
func (rh *RegularHandler) HandleRegularRequestUnified(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, lifecycleManager RequestLifecycleManager) {
	connID := lifecycleManager.GetRequestID()

	slog.Info(fmt.Sprintf("🔄 [常规架构] [%s] 使用unified v3架构", connID))

	// 创建管理器 - 修复依赖注入
	errorRecovery := rh.errorRecoveryFactory.NewErrorRecoveryManager(rh.usageTracker)
	retryMgr := rh.retryManagerFactory.NewRetryManager()
	// 🔧 [修复] 使用共享的SuspensionManager实例，确保全局挂起限制生效
	suspensionMgr := rh.sharedSuspensionManager

	// 外层循环处理组切换恢复 - 修复递归调用栈问题
	for {
		// 获取端点列表
		endpoints := retryMgr.GetHealthyEndpoints(ctx)
		if len(endpoints) == 0 {
			lifecycleManager.HandleError(fmt.Errorf("no healthy endpoints available"))
			http.Error(w, "No healthy endpoints available", http.StatusServiceUnavailable)
			return
		}

		// ✅ 使用与流式请求相同的重试循环
		for i, endpoint := range endpoints {
			lifecycleManager.SetEndpoint(endpoint.Config.Name, endpoint.Config.Group)
			lifecycleManager.UpdateStatus("forwarding", i, 0)

			for attempt := 1; attempt <= retryMgr.GetMaxAttempts(); attempt++ {
				// 检查取消
				select {
				case <-ctx.Done():
					lifecycleManager.UpdateStatus("cancelled", i, 0)
					return
				default:
				}

				// 执行请求
				resp, err := rh.executeRequest(ctx, r, bodyBytes, endpoint)

				if err == nil && IsSuccessStatus(resp.StatusCode) {
					// ✅ 成功 - 响应体由processSuccessResponse管理
					lifecycleManager.UpdateStatus("processing", i+1, resp.StatusCode)
					rh.processSuccessResponse(ctx, w, resp, lifecycleManager, endpoint.Config.Name)
					return
				}

				// ❌ 错误处理 - 修复HTTP响应体资源泄漏问题
				// 对于非成功响应，必须立即关闭响应体（不能在循环中使用defer！）
				if err == nil && resp != nil {
					if !IsSuccessStatus(resp.StatusCode) {
						closeErr := resp.Body.Close()
						if closeErr != nil {
							// Close失败时记录日志但继续处理HTTP错误
							slog.Warn(fmt.Sprintf("⚠️ [响应体关闭失败] [%s] 端点: %s, Close错误: %v", connID, endpoint.Config.Name, closeErr))
						}
						// 将HTTP状态码错误赋给外层err变量，确保后续错误处理生效
						err = fmt.Errorf("HTTP %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
					}
					// 注意：成功响应的Body由processSuccessResponse管理，不在此关闭
				} else if err != nil && resp != nil {
					// HTTP客户端在某些错误情况下仍会返回响应体，必须关闭避免泄漏
					closeErr := resp.Body.Close()
					if closeErr != nil {
						// Close失败时记录日志但继续处理原错误
						slog.Warn(fmt.Sprintf("⚠️ [错误响应体关闭失败] [%s] 端点: %s, Close错误: %v", connID, endpoint.Config.Name, closeErr))
					}
					// 保持原错误不变，让原网络/超时错误得到正确处理
				}

				errorCtx := errorRecovery.ClassifyError(err, connID, endpoint.Config.Name, endpoint.Config.Group, attempt-1)
				lifecycleManager.HandleError(err)

				// 重试判断
				shouldRetry, delay := retryMgr.ShouldRetry(&errorCtx, attempt)
				statusCode := GetStatusCodeFromError(err, resp)

				if !shouldRetry {
					lifecycleManager.UpdateStatus("error", i+1, statusCode)

					// 对于HTTP错误（如404 Not Found），立即失败而不尝试其他端点
					// 因为这类错误与端点健康状况无关，资源不存在问题不会因为更换端点而解决
					if errorCtx.ErrorType == ErrorTypeHTTP {
						finalEndpoints := retryMgr.GetHealthyEndpoints(ctx)
						lifecycleManager.UpdateStatus("error", len(finalEndpoints), statusCode)
						http.Error(w, fmt.Sprintf("HTTP %d: %s", statusCode, http.StatusText(statusCode)), statusCode)
						return
					}

					break // 对于其他不可重试错误，尝试下一个端点
				}

				// 重试
				lifecycleManager.UpdateStatus("retry", i+1, statusCode)
				if attempt < retryMgr.GetMaxAttempts() {
					time.Sleep(delay)
				}
				// 注意：响应体已立即关闭（无defer），连接已释放可重用
			}
		}

		// 检查挂起 - 修复递归调用栈问题
		// 使用循环而非递归避免栈溢出
		if suspensionMgr.ShouldSuspend(ctx) {
			currentEndpoints := retryMgr.GetHealthyEndpoints(ctx)
			lifecycleManager.UpdateStatus("suspended", len(currentEndpoints), 0)
			if suspensionMgr.WaitForGroupSwitch(ctx, connID) {
				// 使用循环重入而非递归
				continue // 重新获取端点列表并继续处理
			}
		}

		// 无法恢复，退出
		break
	}

	// 最终失败 - 使用最后获取的端点数量
	lastEndpoints := retryMgr.GetHealthyEndpoints(ctx)
	lifecycleManager.UpdateStatus("error", len(lastEndpoints), http.StatusBadGateway)
	http.Error(w, "All endpoints failed", http.StatusBadGateway)
}

// executeRequest 执行单个请求
func (rh *RegularHandler) executeRequest(ctx context.Context, r *http.Request, bodyBytes []byte, endpoint *endpoint.Endpoint) (*http.Response, error) {
	// 创建目标请求
	targetURL := endpoint.Config.URL + r.URL.Path
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	req, err := http.NewRequestWithContext(ctx, r.Method, targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 复制和修改头部
	rh.forwarder.CopyHeaders(r, req, endpoint)

	// 创建HTTP传输
	httpTransport, err := transport.CreateTransport(rh.config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport: %w", err)
	}

	client := &http.Client{
		Timeout:   endpoint.Config.Timeout,
		Transport: httpTransport,
	}

	// 执行请求
	return client.Do(req)
}

// processSuccessResponse 处理成功响应
func (rh *RegularHandler) processSuccessResponse(ctx context.Context, w http.ResponseWriter, resp *http.Response, lifecycleManager RequestLifecycleManager, endpointName string) {
	defer resp.Body.Close()

	// 复制响应头（排除Content-Encoding用于gzip处理）
	rh.responseProcessor.CopyResponseHeaders(resp, w)

	// 写入状态码
	w.WriteHeader(resp.StatusCode)

	// 读取并处理响应体
	responseBytes, err := rh.responseProcessor.ProcessResponseBody(resp)
	if err != nil {
		connID := lifecycleManager.GetRequestID()
		lifecycleManager.HandleError(fmt.Errorf("failed to process response: %w", err))
		slog.Error("Failed to process response body", "request_id", connID, "error", err)
		return
	}

	// 写入响应体到客户端
	if _, err := w.Write(responseBytes); err != nil {
		connID := lifecycleManager.GetRequestID()
		lifecycleManager.HandleError(fmt.Errorf("failed to write response: %w", err))
		slog.Error("Failed to write response to client", "request_id", connID, "error", err)
		return
	}

	// ✅ 异步Token解析优化：不阻塞连接关闭
	go func() {
		// 检查context是否已取消
		select {
		case <-ctx.Done():
			// 如果请求已取消，不执行异步Token解析
			return
		default:
		}

		connID := lifecycleManager.GetRequestID()
		slog.Debug(fmt.Sprintf("🔄 [异步Token解析] [%s] 开始后台Token解析", connID))

		// 对于常规请求，异步解析Token信息（如果存在）
		tokenUsage, modelName := rh.tokenAnalyzer.AnalyzeResponseForTokensUnified(responseBytes, connID, endpointName)

		// 再次检查context状态
		select {
		case <-ctx.Done():
			return
		default:
		}

		// 使用生命周期管理器完成请求
		if tokenUsage != nil {
			// 设置模型名称并完成请求
			// 使用对比方法，检测并警告模型不一致情况
			if modelName != "unknown" && modelName != "" {
				lifecycleManager.SetModelWithComparison(modelName, "常规响应解析")
			}
			lifecycleManager.CompleteRequest(tokenUsage)
			slog.Info(fmt.Sprintf("✅ [常规请求Token完成] [%s] 端点: %s, 模型: %s, 输入: %d, 输出: %d",
				connID, endpointName, modelName, tokenUsage.InputTokens, tokenUsage.OutputTokens))
		} else {
			// 处理非Token响应
			lifecycleManager.HandleNonTokenResponse(string(responseBytes))
			slog.Info(fmt.Sprintf("✅ [常规请求完成] [%s] 端点: %s, 响应类型: %s",
				connID, endpointName, modelName))
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
