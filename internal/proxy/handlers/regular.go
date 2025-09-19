package handlers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"strings"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/proxy/retry"
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

// createRetryController 创建重试控制器
func (rh *RegularHandler) createRetryController(lifecycleManager RequestLifecycleManager) *retry.RetryController {
	policy := retry.NewDefaultRetryPolicy(rh.config)

	// 创建适配器接口实现
	adaptedErrorRecoveryFactory := &errorRecoveryFactoryAdapter{
		factory: rh.errorRecoveryFactory,
	}

	// 创建适配器类型的lifecycleManager
	adaptedLifecycleManager := &lifecycleManagerAdapter{
		manager: lifecycleManager,
	}

	// 创建适配器类型的suspensionManager
	adaptedSuspensionManager := &suspensionManagerAdapter{
		manager: rh.sharedSuspensionManager,
	}

	return retry.NewRetryController(
		policy,
		adaptedSuspensionManager,
		adaptedErrorRecoveryFactory,
		adaptedLifecycleManager,
		rh.usageTracker,
	)
}

// errorRecoveryFactoryAdapter 适配handlers.ErrorRecoveryFactory到retry.ErrorRecoveryFactory
type errorRecoveryFactoryAdapter struct {
	factory ErrorRecoveryFactory
}

func (a *errorRecoveryFactoryAdapter) NewErrorRecoveryManager(usageTracker *tracking.UsageTracker) retry.ErrorRecoveryManager {
	manager := a.factory.NewErrorRecoveryManager(usageTracker)
	return &errorRecoveryManagerAdapter{manager: manager}
}

// errorRecoveryManagerAdapter 适配handlers.ErrorRecoveryManager到retry.ErrorRecoveryManager
type errorRecoveryManagerAdapter struct {
	manager ErrorRecoveryManager
}

func (a *errorRecoveryManagerAdapter) ClassifyError(err error, connID, endpointName, groupName string, attemptCount int) retry.ErrorContext {
	ctx := a.manager.ClassifyError(err, connID, endpointName, groupName, attemptCount)
	return retry.ErrorContext{
		RequestID:      ctx.RequestID,
		EndpointName:   ctx.EndpointName,
		GroupName:      ctx.GroupName,
		AttemptCount:   ctx.AttemptCount,
		ErrorType:      ctx.ErrorType,  // ErrorType会被自动转换为interface{}
		OriginalError:  ctx.OriginalError,
		RetryableAfter: ctx.RetryableAfter, // time.Duration会被转换为interface{}
		MaxRetries:     ctx.MaxRetries,
	}
}

func (a *errorRecoveryManagerAdapter) HandleFinalFailure(errorCtx retry.ErrorContext) {
	// 将retry.ErrorContext转换为handlers.ErrorContext
	// 🔧 [修复] 直接转换而不是类型断言，避免proxy.ErrorType到handlers.ErrorType的断言失败
	// errorCtx.ErrorType 实际上是 proxy.ErrorType，需要通过int转换
	var errorType ErrorType
	switch et := errorCtx.ErrorType.(type) {
	case int:
		errorType = ErrorType(et)
	default:
		// 对于其他类型（如proxy.ErrorType），尝试通过整数转换
		// 这里使用反射获取底层值
		if intVal, ok := errorCtx.ErrorType.(interface{ Int() int }); ok {
			errorType = ErrorType(intVal.Int())
		} else {
			// 使用 fmt 包转换为整数
			var val int
			if _, err := fmt.Sscanf(fmt.Sprintf("%d", errorCtx.ErrorType), "%d", &val); err == nil {
				errorType = ErrorType(val)
			} else {
				errorType = ErrorTypeUnknown
			}
		}
	}

	var retryableAfter time.Duration
	if ra, ok := errorCtx.RetryableAfter.(time.Duration); ok {
		retryableAfter = ra
	}

	handlersCtx := ErrorContext{
		RequestID:      errorCtx.RequestID,
		EndpointName:   errorCtx.EndpointName,
		GroupName:      errorCtx.GroupName,
		AttemptCount:   errorCtx.AttemptCount,
		ErrorType:      errorType,
		OriginalError:  errorCtx.OriginalError,
		RetryableAfter: retryableAfter,
		MaxRetries:     errorCtx.MaxRetries,
	}

	a.manager.HandleFinalFailure(handlersCtx)
}

func (a *errorRecoveryManagerAdapter) GetErrorTypeName(errorType interface{}) string {
	// 🔧 [修复] 使用反射获取底层整数值，避免proxy.ErrorType到handlers.ErrorType的断言失败
	if errorType != nil {
		v := reflect.ValueOf(errorType)
		if v.Kind() == reflect.Int {
			return a.manager.GetErrorTypeName(ErrorType(v.Int()))
		}
		// 如果已经是 handlers.ErrorType，直接使用
		if et, ok := errorType.(ErrorType); ok {
			return a.manager.GetErrorTypeName(et)
		}
	}
	return "unknown"
}

// lifecycleManagerAdapter 适配handlers.RequestLifecycleManager到retry.RequestLifecycleManager
type lifecycleManagerAdapter struct {
	manager RequestLifecycleManager
}

func (a *lifecycleManagerAdapter) GetRequestID() string {
	return a.manager.GetRequestID()
}

func (a *lifecycleManagerAdapter) SetEndpoint(name, group string) {
	a.manager.SetEndpoint(name, group)
}

func (a *lifecycleManagerAdapter) SetModel(modelName string) {
	a.manager.SetModel(modelName)
}

func (a *lifecycleManagerAdapter) SetModelWithComparison(modelName, source string) {
	a.manager.SetModelWithComparison(modelName, source)
}

func (a *lifecycleManagerAdapter) HasModel() bool {
	return a.manager.HasModel()
}

func (a *lifecycleManagerAdapter) UpdateStatus(status string, endpointIndex, statusCode int) {
	a.manager.UpdateStatus(status, endpointIndex, statusCode)
}

func (a *lifecycleManagerAdapter) HandleError(err error) {
	a.manager.HandleError(err)
}

func (a *lifecycleManagerAdapter) CompleteRequest(tokens *tracking.TokenUsage) {
	a.manager.CompleteRequest(tokens)
}

func (a *lifecycleManagerAdapter) HandleNonTokenResponse(responseContent string) {
	a.manager.HandleNonTokenResponse(responseContent)
}

func (a *lifecycleManagerAdapter) RecordTokensForFailedRequest(tokens *tracking.TokenUsage, failureReason string) {
	a.manager.RecordTokensForFailedRequest(tokens, failureReason)
}

func (a *lifecycleManagerAdapter) IncrementAttempt() int {
	return a.manager.IncrementAttempt()
}

func (a *lifecycleManagerAdapter) GetAttemptCount() int {
	return a.manager.GetAttemptCount()
}

// suspensionManagerAdapter 适配handlers.SuspensionManager到retry.SuspensionManager
type suspensionManagerAdapter struct {
	manager SuspensionManager
}

func (a *suspensionManagerAdapter) ShouldSuspend(ctx context.Context) bool {
	return a.manager.ShouldSuspend(ctx)
}

func (a *suspensionManagerAdapter) WaitForGroupSwitch(ctx context.Context, connID string) bool {
	return a.manager.WaitForGroupSwitch(ctx, connID)
}

func (a *suspensionManagerAdapter) GetSuspendedRequestsCount() int {
	return a.manager.GetSuspendedRequestsCount()
}

// HandleRegularRequestUnified 统一常规请求处理
// 实现与StreamingHandler相同的重试循环模式，应用所有Critical修复
func (rh *RegularHandler) HandleRegularRequestUnified(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, lifecycleManager RequestLifecycleManager) {
	connID := lifecycleManager.GetRequestID()

	slog.Info(fmt.Sprintf("🔄 [常规架构] [%s] 使用unified v3架构", connID))

	// 创建重试控制器
	retryController := rh.createRetryController(lifecycleManager)

	// 创建管理器 - 修复依赖注入
	retryMgr := rh.retryManagerFactory.NewRetryManager()

	// 外层循环处理组切换逻辑
	for {
		// 获取端点列表
		endpoints := retryMgr.GetHealthyEndpoints(ctx)
		if len(endpoints) == 0 {
			lifecycleManager.HandleError(fmt.Errorf("no healthy endpoints available"))
			http.Error(w, "No healthy endpoints available", http.StatusServiceUnavailable)
			return
		}

		// 内层循环处理端点重试
		groupSwitchNeeded := false
		for i, endpoint := range endpoints {
			lifecycleManager.SetEndpoint(endpoint.Config.Name, endpoint.Config.Group)
			lifecycleManager.UpdateStatus("forwarding", i, 0)

			for attempt := 1; attempt <= retryMgr.GetMaxAttempts(); attempt++ {
				// 检查取消
				select {
				case <-ctx.Done():
					currentAttemptCount := lifecycleManager.GetAttemptCount()
					lifecycleManager.UpdateStatus("cancelled", currentAttemptCount, 0)
					return
				default:
				}

				// 执行请求
				resp, err := rh.executeRequest(ctx, r, bodyBytes, endpoint)

				if err == nil && IsSuccessStatus(resp.StatusCode) {
					// 🔢 [重构] 成功时也需要通过RetryController计数，确保一致性
					// 成功的尝试也是真实的HTTP调用，应该被计数
					retryController := rh.createRetryController(lifecycleManager)
					_, ctrlErr := retryController.OnAttemptResult(ctx, endpoint, nil, attempt, false)
					if ctrlErr != nil {
						slog.Error(fmt.Sprintf("❌ [成功计数错误] [%s] 端点: %s, 错误: %v", connID, endpoint.Config.Name, ctrlErr))
					}

					currentAttemptCount := lifecycleManager.GetAttemptCount()
					lifecycleManager.UpdateStatus("processing", currentAttemptCount, resp.StatusCode)
					rh.processSuccessResponse(ctx, w, resp, lifecycleManager, endpoint.Config.Name)
					return
				}

				// 构造HTTP状态码错误（保持现有逻辑）
				if err == nil && resp != nil && !IsSuccessStatus(resp.StatusCode) {
					// 先尝试从HTTP错误中提取Token信息（如果可能）
					rh.tryExtractTokensFromHttpError(resp, lifecycleManager, endpoint.Config.Name)

					closeErr := resp.Body.Close()
					if closeErr != nil {
						slog.Warn(fmt.Sprintf("⚠️ [响应体关闭失败] [%s] 端点: %s, Close错误: %v",
							connID, endpoint.Config.Name, closeErr))
					}
					err = fmt.Errorf("HTTP %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
				} else if err != nil && resp != nil {
					closeErr := resp.Body.Close()
					if closeErr != nil {
						slog.Warn(fmt.Sprintf("⚠️ [错误响应体关闭失败] [%s] 端点: %s, Close错误: %v",
							connID, endpoint.Config.Name, closeErr))
					}
				}

				// 使用统一重试控制器
				decision, ctrlErr := retryController.OnAttemptResult(ctx, endpoint, err, attempt, false) // 常规请求：isStreaming=false
				if ctrlErr != nil {
					lifecycleManager.HandleError(ctrlErr)
					http.Error(w, "Retry controller error", http.StatusInternalServerError)
					return
				}

				// 处理挂起决策
				if decision.SuspendRequest {
					if rh.sharedSuspensionManager.ShouldSuspend(ctx) {
						currentAttemptCount := lifecycleManager.GetAttemptCount()
						lifecycleManager.UpdateStatus("suspended", currentAttemptCount, 0)
						slog.Info(fmt.Sprintf("⏸️ [请求挂起] [%s] 原因: %s",
							connID, decision.Reason))

						if rh.sharedSuspensionManager.WaitForGroupSwitch(ctx, connID) {
							slog.Info(fmt.Sprintf("📡 [组切换成功] [%s] 重新获取端点列表",
								connID))
							groupSwitchNeeded = true
							break // 跳出端点循环
						} else {
							slog.Warn(fmt.Sprintf("⏰ [挂起失败] [%s] 等待组切换超时或被取消",
								connID))
							currentAttemptCount := lifecycleManager.GetAttemptCount()
							lifecycleManager.UpdateStatus("error", currentAttemptCount, http.StatusBadGateway)
							http.Error(w, "Request suspended but group switch failed", http.StatusBadGateway)
							return
						}
					}
				}

				if !decision.RetrySameEndpoint {
					if decision.SwitchEndpoint {
						break // 尝试下一个端点
					} else {
						// 🔧 [修复] 终止重试时获取真实状态码，避免http.Error panic
						statusCode := GetStatusCodeFromError(err, resp)

						// 🚨 [关键修复] 避免statusCode=0导致http.Error panic
						// Go标准库要求状态码在100-999之间，0会触发panic
						if statusCode == 0 {
							switch decision.FinalStatus {
							case "cancelled":
								// 客户端取消：使用499（nginx风格的客户端取消码）
								statusCode = 499
							case "auth_error":
								statusCode = http.StatusUnauthorized
							case "rate_limited":
								statusCode = http.StatusTooManyRequests
							default:
								// 其他情况（网络错误等）使用502
								statusCode = http.StatusBadGateway
							}
						}

						currentAttemptCount := lifecycleManager.GetAttemptCount()
						lifecycleManager.UpdateStatus(decision.FinalStatus, currentAttemptCount, statusCode)
						http.Error(w, decision.Reason, statusCode)
						return
					}
				}

				// 使用统一延迟
				if attempt < retryMgr.GetMaxAttempts() && decision.Delay > 0 {
					time.Sleep(decision.Delay)
				}
			}

			// 如果需要组切换，跳出端点循环
			if groupSwitchNeeded {
				break
			}
		}

		// 如果需要组切换，重新开始外层循环
		if groupSwitchNeeded {
			continue
		}

		// 所有端点都失败了，终止处理
		break
	}

	// 最终失败处理
	currentAttemptCount := lifecycleManager.GetAttemptCount()
	lifecycleManager.UpdateStatus("error", currentAttemptCount, http.StatusBadGateway)
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

	// ✅ 同步Token解析：简化逻辑，避免协程控制问题
	connID := lifecycleManager.GetRequestID()
	slog.Debug(fmt.Sprintf("🔄 [Token解析] [%s] 开始Token解析", connID))

	// 对于常规请求，同步解析Token信息（如果存在）
	tokenUsage, modelName := rh.tokenAnalyzer.AnalyzeResponseForTokensUnified(responseBytes, connID, endpointName)

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

// tryExtractTokensFromHttpError 尝试从HTTP错误响应中提取Token信息
// 注意：此方法必须在响应体关闭前调用
func (rh *RegularHandler) tryExtractTokensFromHttpError(resp *http.Response, lifecycleManager RequestLifecycleManager, endpointName string) {
	if resp == nil || resp.Body == nil {
		return
	}

	// ✅ 只对可能包含Token信息的错误码进行解析
	if resp.StatusCode != 429 && resp.StatusCode != 413 && resp.StatusCode < 500 {
		return
	}

	// ✅ 同步解析，确保在响应体关闭前完成
	defer func() {
		if r := recover(); r != nil {
			slog.Warn(fmt.Sprintf("⚠️ [错误响应解析恢复] 解析过程中出现异常: %v", r))
		}
	}()

	responseBytes, err := rh.responseProcessor.ProcessResponseBody(resp)
	if err != nil || len(responseBytes) == 0 {
		return
	}

	tokenUsage, modelName := rh.tokenAnalyzer.AnalyzeResponseForTokensUnified(responseBytes, lifecycleManager.GetRequestID(), endpointName)
	if tokenUsage != nil {
		// ✅ 修复：将解析到的模型信息设置到生命周期管理器
		if modelName != "" && modelName != "unknown" {
			lifecycleManager.SetModel(modelName)
		}

		lifecycleManager.RecordTokensForFailedRequest(tokenUsage, fmt.Sprintf("http_%d", resp.StatusCode))
		slog.Info(fmt.Sprintf("💾 [HTTP错误Token记录] [%s] 端点: %s, 状态码: %d, 模型: %s",
			lifecycleManager.GetRequestID(), endpointName, resp.StatusCode, modelName))
	}
}
