package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/tracking"
)

// StreamingHandler 流式请求处理器
// 负责处理所有流式请求，包括错误恢复、重试机制和流式数据转发
type StreamingHandler struct {
	config                 *config.Config
	endpointManager        *endpoint.Manager
	forwarder              *Forwarder
	usageTracker           *tracking.UsageTracker
	tokenParserFactory     TokenParserFactory
	streamProcessorFactory StreamProcessorFactory
	errorRecoveryFactory   ErrorRecoveryFactory
	retryHandlerFactory    RetryHandlerFactory
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
) *StreamingHandler {
	return &StreamingHandler{
		config:                 cfg,
		endpointManager:        endpointManager,
		forwarder:              forwarder,
		usageTracker:           usageTracker,
		tokenParserFactory:     tokenParserFactory,
		streamProcessorFactory: streamProcessorFactory,
		errorRecoveryFactory:   errorRecoveryFactory,
		retryHandlerFactory:    retryHandlerFactory,
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
	var lastErr error  // 声明在外层作用域，供最终错误处理使用
	for i := 0; i < len(endpoints); i++ {
		ep := endpoints[i]
		// 更新生命周期管理器信息
		lifecycleManager.SetEndpoint(ep.Config.Name, ep.Config.Group)
		lifecycleManager.UpdateStatus("forwarding", i, 0)
		
		// ✅ [同端点重试] 对当前端点进行max_attempts次重试
		endpointSuccess := false
		
		for attempt := 1; attempt <= sh.config.Retry.MaxAttempts; attempt++ {
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
			resp, err := sh.forwarder.ForwardRequestToEndpoint(ctx, r, bodyBytes, ep)
			if err == nil {
				// ✅ 成功！开始处理响应
				endpointSuccess = true
				slog.Info(fmt.Sprintf("✅ [流式成功] [%s] 端点: %s (组: %s), 尝试次数: %d", 
					connID, ep.Config.Name, ep.Config.Group, attempt))
					
				lifecycleManager.UpdateStatus("processing", i+1, attempt)
				
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
					slog.Warn(fmt.Sprintf("🔄 [流式处理失败] [%s] 端点: %s, 错误: %v", 
						connID, ep.Config.Name, err))
					
					// 流式处理失败，但HTTP连接已成功建立，记录为processing状态
					lifecycleManager.UpdateStatus("error", i+1, resp.StatusCode)
					fmt.Fprintf(w, "data: error: 流式处理失败: %v\n\n", err)
					flusher.Flush()
					return
				}
				
				// ✅ 流式处理成功完成，使用生命周期管理器完成请求
				if finalTokenUsage != nil {
					// 设置模型名称并通过生命周期管理器完成请求
					lifecycleManager.SetModel(modelName)
					lifecycleManager.CompleteRequest(finalTokenUsage)
				} else {
					// 没有Token信息，使用HandleNonTokenResponse处理
					lifecycleManager.HandleNonTokenResponse("")
				}
				return
			}
			
			// ❌ 出现错误，进行错误分类
			lastErr = err
			errorRecovery := sh.errorRecoveryFactory.NewErrorRecoveryManager(sh.usageTracker)
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
				connID, ep.Config.Name, attempt, sh.config.Retry.MaxAttempts, err))
			
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
				connID, ep.Config.Name, sh.config.Retry.MaxAttempts))
			
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
	tempRetryHandler := sh.retryHandlerFactory.NewRetryHandler(sh.config)
	tempRetryHandler.SetEndpointManager(sh.endpointManager)
	tempRetryHandler.SetUsageTracker(sh.usageTracker)
	
	// 检查是否应该挂起请求
	if tempRetryHandler.ShouldSuspendRequest(ctx) {
		fmt.Fprintf(w, "data: suspend: 当前所有组均不可用，请求已挂起等待组切换...\n\n")
		flusher.Flush()
		
		slog.Info(fmt.Sprintf("⏸️ [流式挂起] [%s] 请求已挂起等待组切换", connID))
		
		// 等待组切换
		if tempRetryHandler.WaitForGroupSwitch(ctx, connID) {
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
	lifecycleManager.UpdateStatus("error", len(endpoints), http.StatusBadGateway)
	fmt.Fprintf(w, "data: error: All endpoints failed, last error: %v\n\n", lastErr)
	flusher.Flush()
	return
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
	if delay > maxDelay {
		delay = maxDelay
	}
	
	return delay
}