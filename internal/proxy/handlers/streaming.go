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
	retryManagerFactory      RetryManagerFactory
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
	retryManagerFactory RetryManagerFactory,
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
		retryManagerFactory:      retryManagerFactory,
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
	var lastFailedEndpoint string // 🚀 [端点自愈] 追踪最后失败的端点

	// 获取健康端点
	var endpoints []*endpoint.Endpoint
	if sh.endpointManager.GetConfig().Strategy.Type == "fastest" && sh.endpointManager.GetConfig().Strategy.FastTestEnabled {
		endpoints = sh.endpointManager.GetFastestEndpointsWithRealTimeTest(ctx)
	} else {
		endpoints = sh.endpointManager.GetHealthyEndpoints()
	}

	if len(endpoints) == 0 {
		// 创建特殊错误，交给错误分类和重试系统处理
		noHealthyErr := fmt.Errorf("no healthy endpoints available")
		errorRecovery := sh.errorRecoveryFactory.NewErrorRecoveryManager(sh.usageTracker)
		errorCtx := errorRecovery.ClassifyError(noHealthyErr, connID, "", "", 0)

		if errorCtx.ErrorType == ErrorTypeNoHealthyEndpoints {
			// 尝试获取所有活跃端点，忽略健康状态
			allActiveEndpoints := sh.endpointManager.GetGroupManager().FilterEndpointsByActiveGroups(
				sh.endpointManager.GetAllEndpoints())

			if len(allActiveEndpoints) > 0 {
				slog.InfoContext(ctx, fmt.Sprintf("🔄 [健康检查回退] [%s] 忽略健康状态，尝试 %d 个活跃端点",
					connID, len(allActiveEndpoints)))
				endpoints = allActiveEndpoints
				// 继续正常处理流程
			} else {
				// 真的没有端点
				lifecycleManager.HandleError(noHealthyErr)
				w.WriteHeader(http.StatusServiceUnavailable)
				fmt.Fprintf(w, "data: error: No endpoints available in active groups\n\n")
				flusher.Flush()
				return
			}
		} else {
			// 按原来逻辑处理
			lifecycleManager.HandleError(noHealthyErr)
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, "data: error: No healthy endpoints available\n\n")
			flusher.Flush()
			return
		}
	}

	slog.Info(fmt.Sprintf("🌊 [流式开始] [%s] 流式请求开始，端点数: %d", connID, len(endpoints)))

	// 🔧 [重试逻辑修复] 对每个端点进行max_attempts次重试，而不是只尝试一次
	// 尝试端点直到成功
	var lastErr error // 声明在外层作用域，供最终错误处理使用
	var lastResp *http.Response // 🔧 [修复] 添加lastResp变量，用于获取真实HTTP状态码
	// 🔢 [重构] 移除currentAttemptCount变量，统一由LifecycleManager管理计数
	for i := 0; i < len(endpoints); i++ {
		ep := endpoints[i]
		lastFailedEndpoint = ep.Config.Name // 🚀 [端点自愈] 记录当前尝试的端点
		// 更新生命周期管理器信息
		lifecycleManager.SetEndpoint(ep.Config.Name, ep.Config.Group)
		lifecycleManager.UpdateStatus("forwarding", i, 0)

		// 🔧 [端点上下文修复] 立即设置端点信息到请求上下文，确保所有分支（成功/失败/取消）的日志都能正确记录端点
		*r = *r.WithContext(context.WithValue(r.Context(), "selected_endpoint", ep.Config.Name))

		// ✅ [同端点重试] 对当前端点进行max_attempts次重试
		endpointSuccess := false
		var attempt int // 声明在外部，循环结束后仍可访问
		var lastDecision *RetryDecision // 保存最后的重试决策，用于外层逻辑

		for attempt = 1; attempt <= sh.config.Retry.MaxAttempts; attempt++ {
			// 检查是否被取消
			select {
			case <-ctx.Done():
				slog.Info(fmt.Sprintf("🚫 [客户端取消检测] [%s] 检测到客户端取消，立即停止重试", connID))
				lifecycleManager.CancelRequest("client disconnected", nil)

				// 🔧 [日志状态码] 设置真实错误码到上下文用于日志记录
				*r = *r.WithContext(context.WithValue(r.Context(), "final_status_code", 499))
				fmt.Fprintf(w, "data: cancelled: 客户端取消请求\n\n")
				flusher.Flush()
				return
			default:
			}

			// 尝试连接端点
			resp, err := sh.forwarder.ForwardRequestToEndpoint(ctx, r, bodyBytes, ep)
			// 🔧 [修复] 保存最后的响应，用于获取真实HTTP状态码
			lastResp = resp
			if err == nil && IsSuccessStatus(resp.StatusCode) {
				// 🔢 [成功计数] 成功的尝试记录到生命周期管理器
				lifecycleManager.IncrementAttempt()
				currentAttemptCount := lifecycleManager.GetAttemptCount()

				// ✅ [重试决策] 成功请求的决策日志 - 保持监控完整性
				slog.Info(fmt.Sprintf("✅ [重试决策] 请求成功完成 request_id=%s endpoint=%s attempt=%d reason=请求成功完成",
					connID, ep.Config.Name, currentAttemptCount))

				// ✅ 成功！开始处理响应
				endpointSuccess = true
				slog.Info(fmt.Sprintf("✅ [流式成功] [%s] 端点: %s (组: %s), 尝试次数: %d",
					connID, ep.Config.Name, ep.Config.Group, currentAttemptCount))

				lifecycleManager.UpdateStatus("processing", currentAttemptCount, resp.StatusCode)

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

					// 🚀 [状态机重构] Phase 4: 统一使用HandleError处理错误，遵循状态错误分离原则
					// 设置failure_reason，让错误分类器正确识别stream_status错误
					lifecycleManager.HandleError(err)

					// 🚀 [HTTP状态码修复] 流式API错误应该映射为207 Multi-Status
					statusCode := GetStatusCodeFromError(err, resp)
					if status == "stream_error" {
						statusCode = http.StatusMultiStatus // 207: HTTP连接成功，但API业务层面有错误
					} else if status == "cancelled" {
						statusCode = 499 // 客户端取消
					}

					// 🚀 [语义修复] 区分取消和失败的不同处理方式
					if status == "cancelled" {
						// 取消请求：直接传递Token信息给CancelRequest，保持语义一致性
						// 避免先调用RecordTokensForFailedRequest再CancelRequest的语义矛盾
						lifecycleManager.CancelRequest("stream processing cancelled", finalTokenUsage)
					} else {
						// 流式错误：先记录失败Token，再使用FailRequest设置最终状态
						if finalTokenUsage != nil {
							lifecycleManager.RecordTokensForFailedRequest(finalTokenUsage, status)
						} else {
							// 无Token信息，仅记录失败状态
							slog.Info(fmt.Sprintf("❌ [流式失败无Token] [%s] 端点: %s, 状态: %s, 无Token信息可保存",
								connID, ep.Config.Name, status))
						}
						// 使用FailRequest设置最终状态为failed
						// 这样status=failed, failure_reason=stream_error, http_status=207
						lifecycleManager.FailRequest(status, err.Error(), statusCode)
					}

					// 🔧 [日志状态码] 设置真实错误码到上下文用于日志记录
					*r = *r.WithContext(context.WithValue(r.Context(), "final_status_code", statusCode))

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

			// ❌ 出现错误，记录尝试次数
			globalAttemptCount := lifecycleManager.IncrementAttempt()
			lastErr = err

			// 错误处理 - 先构造HTTP状态码错误（保持现有逻辑）
			if err == nil && resp != nil && !IsSuccessStatus(resp.StatusCode) {
				closeErr := resp.Body.Close() // 立即关闭非成功响应体
				if closeErr != nil {
					slog.Warn(fmt.Sprintf("⚠️ [响应体关闭失败] [%s] 端点: %s, Close错误: %v", connID, ep.Config.Name, closeErr))
				}
				// 构造HTTP状态码错误，确保RetryManager能正确分类429等状态
				lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
			} else if err != nil && resp != nil {
				closeErr := resp.Body.Close()
				if closeErr != nil {
					slog.Warn(fmt.Sprintf("⚠️ [错误响应体关闭失败] [%s] 端点: %s, Close错误: %v", connID, ep.Config.Name, closeErr))
				}
			}

			// 🔧 使用增强的RetryManager进行统一决策
			errorRecovery := sh.errorRecoveryFactory.NewErrorRecoveryManager(sh.usageTracker)
			errorCtx := errorRecovery.ClassifyError(lastErr, connID, ep.Config.Name, ep.Config.Group, attempt-1)

			// 🚀 [状态机重构] Phase 4: 分离状态转换与失败原因记录
			// 预设错误上下文（避免重复分类），由HandleError统一记录失败原因
			lifecycleManager.PrepareErrorContext(&errorCtx)
			lifecycleManager.HandleError(lastErr)

			// 创建重试管理器
			retryMgr := sh.retryManagerFactory.NewRetryManager()
			// 🔢 [关键修复] 分离局部和全局计数语义
			// attempt: 当前端点内的尝试次数，用于退避计算
			// globalAttemptCount: 全局尝试次数，用于限流策略
			decision := retryMgr.ShouldRetryWithDecision(&errorCtx, attempt, globalAttemptCount, true) // 流式请求: isStreaming=true
			lastDecision = &decision // 保存决策，供外层逻辑使用

			// 检查决策结果
			if decision.FinalStatus == "cancelled" {
				// 🔧 [修复] 添加生命周期状态更新
				lifecycleManager.CancelRequest("client disconnected", nil)
				slog.Info(fmt.Sprintf("🚫 [客户端取消检测] [%s] 检测到客户端取消，立即停止重试", connID))

				// 🔧 [日志状态码] 设置真实错误码到上下文用于日志记录
				*r = *r.WithContext(context.WithValue(r.Context(), "final_status_code", 499))
				fmt.Fprintf(w, "data: cancelled: 客户端取消请求\n\n")
				flusher.Flush()
				return
			}

			// 处理挂起决策
			if decision.SuspendRequest {
				if sh.sharedSuspensionManager.ShouldSuspend(ctx) {
					// 🚀 [状态机重构] Phase 4: 挂起时更新状态
					lifecycleManager.UpdateStatus("suspended", -1, 0)
					slog.Info(fmt.Sprintf("⏸️ [流式挂起] [%s] 原因: %s，失败端点: %s", connID, decision.Reason, ep.Config.Name))
					fmt.Fprintf(w, "data: suspend: 请求已挂起，等待端点 %s 恢复或组切换...\n\n", ep.Config.Name)
					flusher.Flush()

					// 🚀 [端点自愈] 使用新的端点恢复等待方法，能区分成功/超时/取消
					result := sh.sharedSuspensionManager.WaitForEndpointRecoveryWithResult(ctx, connID, ep.Config.Name)
					switch result {
					case SuspensionSuccess:
						slog.Info(fmt.Sprintf("🎯 [恢复成功] [%s] 端点 %s 已恢复或组已切换，重新开始处理", connID, ep.Config.Name))
						fmt.Fprintf(w, "data: resume: 端点已恢复，重新开始处理...\n\n")
						flusher.Flush()
						// 重新开始executeStreamingWithRetry
						sh.executeStreamingWithRetry(ctx, w, r, bodyBytes, lifecycleManager, flusher)
						return
					case SuspensionCancelled:
						// 🎯 [挂起取消区分] 用户在挂起期间取消请求，应该记录为取消而非失败
						slog.Info(fmt.Sprintf("🚫 [挂起期间取消] [%s] 用户在挂起期间取消请求", connID))
						// 🔧 [状态码修复] 设置取消状态码到上下文用于日志记录
						*r = *r.WithContext(context.WithValue(r.Context(), "final_status_code", 499))
						lifecycleManager.CancelRequest("suspended then cancelled", nil)
						fmt.Fprintf(w, "data: cancelled: 客户端取消请求\n\n")
						flusher.Flush()
						return
					case SuspensionTimeout:
						// 🔧 [修复] 添加生命周期状态更新
						currentAttemptCount := lifecycleManager.GetAttemptCount()
						lifecycleManager.UpdateStatus("error", currentAttemptCount, http.StatusBadGateway)
						slog.Warn(fmt.Sprintf("⏰ [挂起超时] [%s] 等待端点恢复或组切换超时", connID))
						fmt.Fprintf(w, "data: error: 挂起等待超时\n\n")
						flusher.Flush()
						return
					}
				}
			}

			if !decision.RetrySameEndpoint {
				if decision.SwitchEndpoint {
					slog.Info(fmt.Sprintf("🔀 [切换端点] [%s] 当前端点: %s, 原因: %s",
						connID, ep.Config.Name, decision.Reason))
					break // 尝试下一个端点
				} else {
					// 🚀 [状态机重构] Phase 4: 最终失败处理
					// 获取失败原因
					failureReason := lifecycleManager.MapErrorTypeToFailureReason(errorCtx.ErrorType)

					// 使用GetStatusCodeFromError获取真实的HTTP状态码
					statusCode := GetStatusCodeFromError(lastErr, lastResp)

					// 如果无法获取状态码，使用合理的默认值
					if statusCode == 0 {
						switch decision.FinalStatus {
						case "cancelled":
							statusCode = 499 // nginx风格的客户端取消码
						case "auth_error":
							statusCode = http.StatusUnauthorized
						case "rate_limited":
							statusCode = http.StatusTooManyRequests
						default:
							statusCode = http.StatusBadGateway
						}
					}

					// 使用新的FailRequest方法标记最终失败（修复：使用计算好的statusCode而非lastResp.StatusCode）
					lifecycleManager.FailRequest(failureReason, lastErr.Error(), statusCode)

					// 终止重试
					slog.Info(fmt.Sprintf("🛑 [终止重试] [%s] 端点: %s, 状态: %s, 状态码: %d, 原因: %s",
						connID, ep.Config.Name, decision.FinalStatus, statusCode, decision.Reason))
					fmt.Fprintf(w, "data: error: %s\n\n", decision.Reason)
					flusher.Flush()
					return
				}
			}

			// 🚀 [状态机重构] Phase 4: 重试状态管理
			if decision.RetrySameEndpoint && attempt < sh.config.Retry.MaxAttempts {
				// 更新为重试状态
				lifecycleManager.UpdateStatus("retry", globalAttemptCount, 0)

				// 如果不是最后一次尝试，等待重试延迟
				slog.Info(fmt.Sprintf("⏳ [等待重试] [%s] 端点: %s, 延迟: %v, 原因: %s",
					connID, ep.Config.Name, decision.Delay, decision.Reason))

				// 向客户端发送重试信息
				fmt.Fprintf(w, "data: retry: 重试端点 %s (尝试 %d/%d)，等待 %v...\n\n",
					ep.Config.Name, attempt+1, sh.config.Retry.MaxAttempts, decision.Delay)
				flusher.Flush()

				// 等待延迟，同时检查取消
				select {
				case <-ctx.Done():
					slog.Info(fmt.Sprintf("🚫 [重试取消] [%s] 等待重试期间检测到取消", connID))
					lifecycleManager.CancelRequest("client disconnected during retry delay", nil)

					// 🔧 [日志状态码] 设置真实错误码到上下文用于日志记录
					*r = *r.WithContext(context.WithValue(r.Context(), "final_status_code", 499))
					fmt.Fprintf(w, "data: cancelled: 客户端取消请求\n\n")
					flusher.Flush()
					return
				case <-time.After(decision.Delay):
					// 继续下一次重试
				}
			}
		}

		// 🔧 当前端点所有重试都失败了
		if !endpointSuccess {
			// 修复计数逻辑：处理提前break和自然跑满两种情况
			actualAttempts := attempt
			if actualAttempts > sh.config.Retry.MaxAttempts {
				actualAttempts = sh.config.Retry.MaxAttempts
			}

			// 🚀 [改进版方案1] 使用已保存的重试决策，避免重复错误分类
			var willSwitchEndpoint bool = true
			if lastDecision != nil {
				willSwitchEndpoint = lastDecision.SwitchEndpoint

				// 对于不切换端点的决策（如HTTP错误、流式错误等），直接终止
				if !willSwitchEndpoint && lastDecision.FinalStatus != "" {
					slog.Info(fmt.Sprintf("❌ [决策终止] [%s] %s，不尝试其他端点", connID, lastDecision.Reason))
					// 🚀 [状态机重构] Phase 4: 使用FailRequest方法标记最终失败
					failureReason := "unknown_error"
					if lastErr != nil {
						// 重新分类错误以获取准确的失败原因
						errorRecovery := sh.errorRecoveryFactory.NewErrorRecoveryManager(sh.usageTracker)
						errorCtx := errorRecovery.ClassifyError(lastErr, connID, "", "", 0)
						failureReason = lifecycleManager.MapErrorTypeToFailureReason(errorCtx.ErrorType)
					}
					// 获取真实的HTTP状态码
					statusCode := GetStatusCodeFromError(lastErr, lastResp)
					if statusCode == 0 {
						// 根据决策状态设置合适的默认状态码
						if lastDecision != nil && lastDecision.FinalStatus != "" {
							switch lastDecision.FinalStatus {
							case "cancelled":
								statusCode = 499 // nginx风格的客户端取消码
							case "auth_error":
								statusCode = http.StatusUnauthorized
							case "rate_limited":
								statusCode = http.StatusTooManyRequests
							default:
								statusCode = http.StatusBadGateway
							}
						} else {
							statusCode = http.StatusBadGateway
						}
					}
					lifecycleManager.FailRequest(failureReason, lastDecision.Reason, statusCode)
					fmt.Fprintf(w, "data: error: %s\n\n", lastDecision.Reason)
					flusher.Flush()
					return
				}
			}

			// 根据是否会切换端点来显示不同的日志
			if actualAttempts == 1 {
				if willSwitchEndpoint {
					slog.Warn(fmt.Sprintf("❌ [端点失败] [%s] 端点: %s 第1次尝试失败，切换端点",
						connID, ep.Config.Name))
				} else {
					slog.Warn(fmt.Sprintf("❌ [端点失败] [%s] 端点: %s 第1次尝试失败，直接终止",
						connID, ep.Config.Name))
				}
			} else {
				slog.Warn(fmt.Sprintf("❌ [端点失败] [%s] 端点: %s 共尝试 %d 次均失败",
					connID, ep.Config.Name, actualAttempts))
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

		// 🚀 [状态机重构] Phase 4: 挂起时更新状态（移除重复的失败原因记录）
		lifecycleManager.UpdateStatus("suspended", -1, 0)
		fmt.Fprintf(w, "data: suspend: 当前所有组均不可用，请求已挂起等待组切换...\n\n")
		flusher.Flush()

		// 🔢 [语义修复] 在日志中记录端点数量信息，但不影响重试计数语义
		actualAttemptCount := lifecycleManager.GetAttemptCount()
		slog.Info(fmt.Sprintf("⏸️ [流式挂起] [%s] 请求已挂起，尝试次数: %d, 健康端点数: %d, 最后失败端点: %s",
			connID, actualAttemptCount, len(currentEndpoints), lastFailedEndpoint))

		// 🚀 [端点自愈] 等待端点恢复，能区分成功/超时/取消
		result := suspensionMgr.WaitForEndpointRecoveryWithResult(ctx, connID, lastFailedEndpoint)
		switch result {
		case SuspensionSuccess:
			slog.Info(fmt.Sprintf("🚀 [挂起恢复] [%s] 端点 %s 已恢复或组切换完成，重新获取端点", connID, lastFailedEndpoint))
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
		case SuspensionCancelled:
			// 🎯 [挂起取消区分] 用户在挂起期间取消请求，应该记录为取消而非失败
			slog.Info(fmt.Sprintf("🚫 [挂起期间取消] [%s] 用户在挂起期间取消请求", connID))
			// 🔧 [状态码修复] 设置取消状态码到上下文用于日志记录
			*r = *r.WithContext(context.WithValue(r.Context(), "final_status_code", 499))
			lifecycleManager.CancelRequest("suspended then cancelled", nil)
			fmt.Fprintf(w, "data: cancelled: 客户端取消请求\n\n")
			flusher.Flush()
			return
		case SuspensionTimeout:
			slog.Warn(fmt.Sprintf("⏰ [挂起超时] [%s] 挂起等待超时", connID))
			// 继续执行下面的失败处理逻辑
		}
	}

	// 🚀 [状态机重构] Phase 4: 最终失败处理
	// 所有端点都失败了，使用FailRequest方法标记最终失败（修复：使用GetStatusCodeFromError计算正确状态码）
	statusCode := GetStatusCodeFromError(lastErr, lastResp)
	if statusCode == 0 {
		statusCode = http.StatusBadGateway // 端点耗尽的默认状态码
	}

	// 🔧 [日志状态码] 设置真实错误码到上下文用于日志记录
	*r = *r.WithContext(context.WithValue(r.Context(), "final_status_code", statusCode))

	lifecycleManager.FailRequest("endpoint_exhausted", "All endpoints failed, last error: "+fmt.Sprintf("%v", lastErr), statusCode)
	fmt.Fprintf(w, "data: error: All endpoints failed, last error: %v\n\n", lastErr)
	flusher.Flush()
}

