package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/tracking"
)

// StreamingHandler 流式请求处理器
// 负责处理所有流式请求，包括错误恢复、重试机制和流式数据转发
type StreamingHandler struct {
	config                   *config.Config
	endpointManager          *endpoint.Manager
	forwarder                *Forwarder
	usageTracker             *tracking.UsageTracker
	tokenParserFactory       TokenParserFactory
	streamProcessorFactory   StreamProcessorFactory
	errorRecoveryFactory     ErrorRecoveryFactory
	retryHandlerFactory      RetryHandlerFactory
	suspensionManagerFactory SuspensionManagerFactory
	// 🔧 [修复] 共享SuspensionManager实例，确保全局挂起限制生效
	sharedSuspensionManager SuspensionManager
}

// NewStreamingHandler 创建新的StreamingHandler实例
func NewStreamingHandler(
	cfg *config.Config,
	endpointManager *endpoint.Manager,
	forwarder *Forwarder,
	usageTracker *tracking.UsageTracker,
	tokenParserFactory TokenParserFactory,
	streamProcessorFactory StreamProcessorFactory,
	errorRecoveryFactory ErrorRecoveryFactory,
	retryHandlerFactory RetryHandlerFactory,
	suspensionManagerFactory SuspensionManagerFactory,
	// 🔧 [Critical修复] 直接接受共享的SuspensionManager实例
	sharedSuspensionManager SuspensionManager,
) *StreamingHandler {
	return &StreamingHandler{
		config:                   cfg,
		endpointManager:          endpointManager,
		forwarder:                forwarder,
		usageTracker:             usageTracker,
		tokenParserFactory:       tokenParserFactory,
		streamProcessorFactory:   streamProcessorFactory,
		errorRecoveryFactory:     errorRecoveryFactory,
		retryHandlerFactory:      retryHandlerFactory,
		suspensionManagerFactory: suspensionManagerFactory,
		// 🔧 [Critical修复] 使用传入的共享SuspensionManager实例
		// 确保流式请求与常规请求共享同一个全局挂起计数器
		sharedSuspensionManager: sharedSuspensionManager,
	}
}

// noOpFlusher 是一个不执行实际flush操作的flusher实现
type noOpFlusher struct{}

func (f *noOpFlusher) Flush() {
	// 不执行任何操作，避免panic但保持流式处理逻辑
}

// HandleStreamingRequest 统一流式请求处理
// 使用V2架构整合错误恢复机制和生命周期管理的流式处理
func (sh *StreamingHandler) HandleStreamingRequest(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, lifecycleManager RequestLifecycleManager) {
	connID := lifecycleManager.GetRequestID()

	slog.Info(fmt.Sprintf("🌊 [流式架构] [%s] 使用streaming v2架构", connID))
	slog.Info(fmt.Sprintf("🌊 [流式处理] [%s] 开始流式请求处理", connID))
	sh.handleStreamingV2(ctx, w, r, bodyBytes, lifecycleManager)
}

// handleStreamingV2 流式处理（带错误恢复）
func (sh *StreamingHandler) handleStreamingV2(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, lifecycleManager RequestLifecycleManager) {
	connID := lifecycleManager.GetRequestID()

	// 设置流式响应头
	sh.setStreamingHeaders(w)

	// 获取Flusher - 如果不支持，使用无flush模式继续流式处理
	flusher, ok := w.(http.Flusher)
	if !ok {
		slog.Warn(fmt.Sprintf("🌊 [Flusher不支持] [%s] 将使用无flush模式的流式处理", connID))
		// 创建一个mock flusher，不执行实际flush操作
		flusher = &noOpFlusher{}
	}

	// 继续执行流式请求处理
	sh.executeStreamingWithRetry(ctx, w, r, bodyBytes, lifecycleManager, flusher)
}

// setStreamingHeaders 设置流式响应头
func (sh *StreamingHandler) setStreamingHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Cache-Control")
}

// executeStreamingWithRetry 执行带重试的流式处理
func (sh *StreamingHandler) executeStreamingWithRetry(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, lifecycleManager RequestLifecycleManager, flusher http.Flusher) {
	connID := lifecycleManager.GetRequestID()

	// 获取健康端点
	var endpoints []*endpoint.Endpoint
	if sh.endpointManager.GetConfig().Strategy.Type == "fastest" && sh.endpointManager.GetConfig().Strategy.FastTestEnabled {
		endpoints = sh.endpointManager.GetFastestEndpointsWithRealTimeTest(ctx)
	} else {
		endpoints = sh.endpointManager.GetHealthyEndpoints()
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
	var lastErr error // 声明在外层作用域，供最终错误处理使用
	var currentAttemptCount int // 🔢 [语义修复] 声明在外层作用域，用于追踪真实尝试次数
	for i := 0; i < len(endpoints); i++ {
		ep := endpoints[i]
		// 更新生命周期管理器信息
		lifecycleManager.SetEndpoint(ep.Config.Name, ep.Config.Group)
		lifecycleManager.UpdateStatus("forwarding", i, 0)

		// ✅ [同端点重试] 对当前端点进行max_attempts次重试
		endpointSuccess := false

		for attempt := 1; attempt <= sh.config.Retry.MaxAttempts; attempt++ {
			// 🔢 [语义修复] 每次尝试端点时增加真实的尝试计数
			currentAttemptCount = lifecycleManager.IncrementAttempt()

			// 检查是否被取消
			select {
			case <-ctx.Done():
				slog.Info(fmt.Sprintf("🚫 [客户端取消检测] [%s] 检测到客户端取消，立即停止重试", connID))
				lifecycleManager.UpdateStatus("cancelled", currentAttemptCount, 0)
				fmt.Fprintf(w, "data: cancelled: 客户端取消请求\n\n")
				flusher.Flush()
				return
			default:
			}

			// 尝试连接端点
			resp, err := sh.forwarder.ForwardRequestToEndpoint(ctx, r, bodyBytes, ep)
			if err == nil && IsSuccessStatus(resp.StatusCode) {
				// ✅ 成功！开始处理响应
				endpointSuccess = true
				slog.Info(fmt.Sprintf("✅ [流式成功] [%s] 端点: %s (组: %s), 尝试次数: %d",
					connID, ep.Config.Name, ep.Config.Group, currentAttemptCount))

				lifecycleManager.UpdateStatus("processing", currentAttemptCount, resp.StatusCode)

				// 设置选中的端点到请求上下文，用于日志记录
				*r = *r.WithContext(context.WithValue(r.Context(), "selected_endpoint", ep.Config.Name))

				// 处理流式响应 - 使用现有的流式处理逻辑
				w.WriteHeader(resp.StatusCode)

				// 创建Token解析器和流式处理器
				tokenParser := sh.tokenParserFactory.NewTokenParserWithUsageTracker(connID, sh.usageTracker)
				processor := sh.streamProcessorFactory.NewStreamProcessor(tokenParser, sh.usageTracker, w, flusher, connID, ep.Config.Name)

				slog.Info(fmt.Sprintf("🚀 [开始流式处理] [%s] 端点: %s", connID, ep.Config.Name))

				// 执行流式处理并获取Token信息和模型名称
				finalTokenUsage, modelName, err := processor.ProcessStreamWithRetry(ctx, resp)
				if err != nil {
					var status, parsedModelName string = "error", "unknown"

					// ✅ 从错误信息中提取状态和模型信息
					if strings.HasPrefix(err.Error(), "stream_status:") {
						parts := strings.SplitN(err.Error(), ":", 5)
						if len(parts) >= 4 {
							status = parts[1] // 状态：cancelled, timeout, error
							if parts[2] == "model" && len(parts) > 3 && parts[3] != "" {
								parsedModelName = parts[3] // 模型：claude-sonnet-4-20250514
							}
						}
					}

					// ✅ 确保生命周期管理器获得正确的模型信息
					// 优先使用从错误包装器中解析的模型信息
					if parsedModelName != "unknown" && parsedModelName != "" {
						lifecycleManager.SetModelWithComparison(parsedModelName, "stream_status")
					} else if modelName != "unknown" && modelName != "" {
						// ✅ 如果错误包装器中没有模型信息，使用ProcessStreamWithRetry返回的模型信息
						lifecycleManager.SetModelWithComparison(modelName, "stream_processor")
					}

					// ✅ 使用正确的状态更新
					lifecycleManager.UpdateStatus(status, currentAttemptCount, resp.StatusCode)

					// ✅ 如果有token信息，使用失败Token记录方法，不改变请求状态
					if finalTokenUsage != nil {
						lifecycleManager.RecordTokensForFailedRequest(finalTokenUsage, status)
					} else {
						// 无Token信息，仅记录失败状态
						slog.Info(fmt.Sprintf("❌ [流式失败无Token] [%s] 端点: %s, 状态: %s, 无Token信息可保存",
							connID, ep.Config.Name, status))
					}

					slog.Warn(fmt.Sprintf("🔄 [流式处理失败] [%s] 端点: %s, 状态: %s, 模型: %s, 错误: %v",
						connID, ep.Config.Name, status, parsedModelName, err))

					// 根据状态决定是否发送错误信息
					if status == "cancelled" {
						fmt.Fprintf(w, "data: cancelled: 客户端取消请求\n\n")
					} else {
						fmt.Fprintf(w, "data: error: 流式处理失败: %v\n\n", err)
					}
					flusher.Flush()
					return
				}

				// ✅ 流式处理成功完成，使用生命周期管理器完成请求
				if finalTokenUsage != nil {
					// 设置模型名称并通过生命周期管理器完成请求
					// 使用对比方法，检测并警告模型不一致情况
					if modelName != "unknown" && modelName != "" {
						lifecycleManager.SetModelWithComparison(modelName, "流式响应解析")
					}
					lifecycleManager.CompleteRequest(finalTokenUsage)
				} else {
					// 没有Token信息，使用HandleNonTokenResponse处理
					lifecycleManager.HandleNonTokenResponse("")
				}
				return
			}

			// ❌ 出现错误，进行错误分类
			lastErr = err

			// ❌ 处理非成功HTTP状态码 - 修复响应体资源泄漏
			if err == nil && resp != nil && !IsSuccessStatus(resp.StatusCode) {
				closeErr := resp.Body.Close() // 立即关闭非成功响应体，避免连接池耗尽
				if closeErr != nil {
					// Close失败时记录日志但继续处理HTTP错误
					slog.Warn(fmt.Sprintf("⚠️ [响应体关闭失败] [%s] 端点: %s, Close错误: %v", connID, ep.Config.Name, closeErr))
				}
				lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
			} else if err != nil && resp != nil {
				// HTTP客户端在某些错误情况下仍会返回响应体，必须关闭避免泄漏
				closeErr := resp.Body.Close() // 立即关闭错误响应体
				if closeErr != nil {
					// Close失败时记录日志但继续处理原错误
					slog.Warn(fmt.Sprintf("⚠️ [错误响应体关闭失败] [%s] 端点: %s, Close错误: %v", connID, ep.Config.Name, closeErr))
				}
			}

			errorRecovery := sh.errorRecoveryFactory.NewErrorRecoveryManager(sh.usageTracker)
			errorCtx := errorRecovery.ClassifyError(lastErr, connID, ep.Config.Name, ep.Config.Group, attempt-1)

			// 检查是否为客户端取消错误
			if errorCtx.ErrorType == ErrorTypeClientCancel {
				slog.Info(fmt.Sprintf("🚫 [客户端取消检测] [%s] 检测到客户端取消，立即停止重试", connID))
				lifecycleManager.UpdateStatus("cancelled", currentAttemptCount, 0)
				fmt.Fprintf(w, "data: cancelled: 客户端取消请求\n\n")
				flusher.Flush()
				return
			}

			// 非取消错误：记录重试状态
			lifecycleManager.HandleError(lastErr)
			lifecycleManager.UpdateStatus("retry", currentAttemptCount, 0)

			slog.Warn(fmt.Sprintf("🔄 [流式重试] [%s] 端点: %s, 尝试: %d/%d, 错误: %v",
				connID, ep.Config.Name, attempt, sh.config.Retry.MaxAttempts, lastErr))

			// 如果不是最后一次尝试，等待重试延迟
			if attempt < sh.config.Retry.MaxAttempts {
				// 计算重试延迟
				delay := sh.calculateRetryDelay(attempt)
				slog.Info(fmt.Sprintf("⏳ [等待重试] [%s] 端点: %s, 延迟: %v",
					connID, ep.Config.Name, delay))

				// 向客户端发送重试信息
				fmt.Fprintf(w, "data: retry: 重试端点 %s (尝试 %d/%d)，等待 %v...\n\n",
					ep.Config.Name, attempt+1, sh.config.Retry.MaxAttempts, delay)
				flusher.Flush()

				// 等待延迟，同时检查取消
				select {
				case <-ctx.Done():
					slog.Info(fmt.Sprintf("🚫 [重试取消] [%s] 等待重试期间检测到取消", connID))
					lifecycleManager.UpdateStatus("cancelled", currentAttemptCount, 0)
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
				connID, ep.Config.Name, sh.config.Retry.MaxAttempts))

			// 检查最后的错误类型，决定是否尝试其他端点
			if lastErr != nil {
				errorRecovery := sh.errorRecoveryFactory.NewErrorRecoveryManager(sh.usageTracker)
				errorCtx := errorRecovery.ClassifyError(lastErr, connID, ep.Config.Name, ep.Config.Group, 0)

				// 对于HTTP错误（如404 Not Found），立即失败而不尝试其他端点
				// 因为这类错误与端点健康状况无关，资源不存在问题不会因为更换端点而解决
				if errorCtx.ErrorType == ErrorTypeHTTP {
					slog.Info(fmt.Sprintf("❌ [HTTP错误终止] [%s] HTTP错误不尝试其他端点: %v", connID, lastErr))
					// 🔧 [语义修复] 使用-1参数让内部计数器处理
					lifecycleManager.UpdateStatus("error", -1, 0)
					fmt.Fprintf(w, "data: error: HTTP错误，终止处理: %v\n\n", lastErr)
					flusher.Flush()
					return
				}
			}

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

	// 🔧 [修复] 使用共享的SuspensionManager实例，确保全局挂起限制生效
	suspensionMgr := sh.sharedSuspensionManager

	// 检查是否应该挂起请求
	if suspensionMgr.ShouldSuspend(ctx) {
		currentEndpoints := sh.endpointManager.GetHealthyEndpoints()
		if cfg := sh.endpointManager.GetConfig(); cfg != nil && cfg.Strategy.Type == "fastest" && cfg.Strategy.FastTestEnabled {
			currentEndpoints = sh.endpointManager.GetFastestEndpointsWithRealTimeTest(ctx)
		}

		// 🔧 [语义修复] 使用-1参数让内部计数器处理
		lifecycleManager.UpdateStatus("suspended", -1, 0)
		fmt.Fprintf(w, "data: suspend: 当前所有组均不可用，请求已挂起等待组切换...\n\n")
		flusher.Flush()

		// 🔢 [语义修复] 在日志中记录端点数量信息，但不影响重试计数语义
		actualAttemptCount := lifecycleManager.GetAttemptCount()
		slog.Info(fmt.Sprintf("⏸️ [流式挂起] [%s] 请求已挂起，尝试次数: %d, 健康端点数: %d",
			connID, actualAttemptCount, len(currentEndpoints)))

		// 等待组切换
		if suspensionMgr.WaitForGroupSwitch(ctx, connID) {
			slog.Info(fmt.Sprintf("🚀 [挂起恢复] [%s] 组切换完成，重新获取端点", connID))
			fmt.Fprintf(w, "data: resume: 组切换完成，恢复处理...\n\n")
			flusher.Flush()

			// 重新获取健康端点
			var newEndpoints []*endpoint.Endpoint
			if sh.endpointManager.GetConfig().Strategy.Type == "fastest" && sh.endpointManager.GetConfig().Strategy.FastTestEnabled {
				newEndpoints = sh.endpointManager.GetFastestEndpointsWithRealTimeTest(ctx)
			} else {
				newEndpoints = sh.endpointManager.GetHealthyEndpoints()
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
				sh.executeStreamingWithRetry(ctx, w, r, bodyBytes, lifecycleManager, flusher)
				return
			}
		}
	}

	slog.Warn(fmt.Sprintf("⚠️ [挂起失败] [%s] 挂起等待超时或失败", connID))

	// 最终失败处理 - 生命周期管理器已处理错误分类
	// 🔧 [语义修复] 使用-1参数让内部计数器处理
	lifecycleManager.UpdateStatus("error", -1, http.StatusBadGateway)
	fmt.Fprintf(w, "data: error: All endpoints failed, last error: %v\n\n", lastErr)
	flusher.Flush()
}

// calculateRetryDelay 计算重试延迟（指数退避算法）
// 与RetryHandler保持一致的计算逻辑
func (sh *StreamingHandler) calculateRetryDelay(attempt int) time.Duration {
	baseDelay := sh.config.Retry.BaseDelay
	maxDelay := sh.config.Retry.MaxDelay
	multiplier := sh.config.Retry.Multiplier

	// 计算指数延迟
	delay := time.Duration(float64(baseDelay) * float64(attempt) * multiplier)

	// 限制在最大延迟范围内
	delay = min(delay, maxDelay)

	return delay
}
