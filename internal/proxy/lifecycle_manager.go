package proxy

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/events"
	"cc-forwarder/internal/monitor"
	"cc-forwarder/internal/proxy/handlers"
	"cc-forwarder/internal/tracking"
)

// MonitoringMiddlewareInterface 定义监控中间件接口（扩展版）
type MonitoringMiddlewareInterface interface {
	RecordTokenUsage(connID string, endpoint string, tokens *monitor.TokenUsage)
	RecordFailedRequestTokens(connID, endpoint string, tokens *monitor.TokenUsage, failureReason string) // 新增方法
}

// RetryDecision 重试决策结果
type RetryDecision struct {
	RetrySameEndpoint bool   // 是否重试同一端点
	FinalStatus       string // 最终状态
	Reason            string // 决策原因
}

// RetryContext 重试上下文信息
type RetryContext struct {
	RequestID     string              // 请求ID
	Endpoint      *endpoint.Endpoint  // 端点信息
	Attempt       int                 // 当前尝试次数
	AttemptGlobal int                 // 全局尝试次数
	Error         *ErrorContext       // 错误上下文
	IsStreaming   bool                // 是否为流式请求
}

// RequestLifecycleManager 请求生命周期管理器
// 负责管理请求的完整生命周期，确保所有请求都有完整的跟踪记录
type RequestLifecycleManager struct {
	usageTracker        *tracking.UsageTracker        // 使用跟踪器
	monitoringMiddleware MonitoringMiddlewareInterface // 监控中间件
	errorRecovery       *ErrorRecoveryManager         // 错误恢复管理器
	eventBus            events.EventBus               // EventBus事件总线
	requestID           string                        // 请求唯一标识符
	startTime           time.Time                     // 请求开始时间
	modelMu             sync.RWMutex                  // 保护模型字段的读写锁
	modelName           string                        // 模型名称
	endpointName        string                        // 端点名称
	groupName           string                        // 组名称
	retryCount          int                           // 重试计数
	lastStatus          string                        // 最后状态
	lastError           error                         // 最后一次错误
	finalStatusCode     int                           // 最终状态码
	modelUpdatedInDB    bool                          // 标记是否已在数据库中更新过模型
	modelUpdateMu       sync.Mutex                    // 保护模型更新标记
	attemptCounter      int                           // 内部尝试计数器（语义修复：统一重试计数）
	attemptMu           sync.Mutex                    // 保护尝试计数器的互斥锁
	pendingErrorContext *ErrorContext                 // 预先计算的错误上下文，仅对下一个HandleError有效
	pendingErrorOriginal error                        // 预先计算上下文对应的原始错误，用于校验匹配
	pendingErrorMu      sync.Mutex                    // 保护预先计算错误上下文的互斥锁
}

// NewRequestLifecycleManager 创建新的请求生命周期管理器
func NewRequestLifecycleManager(usageTracker *tracking.UsageTracker, monitoringMiddleware MonitoringMiddlewareInterface, requestID string, eventBus events.EventBus) *RequestLifecycleManager {
	return &RequestLifecycleManager{
		usageTracker:        usageTracker,
		monitoringMiddleware: monitoringMiddleware,
		errorRecovery:       NewErrorRecoveryManager(usageTracker),
		eventBus:            eventBus,
		requestID:           requestID,
		startTime:           time.Now(),
		lastStatus:          "pending",
	}
}

// StartRequest 开始请求跟踪
// 调用 RecordRequestStart 记录请求开始，并发布请求开始事件
func (rlm *RequestLifecycleManager) StartRequest(clientIP, userAgent, method, path string, isStreaming bool) {
	// 原有的数据记录逻辑
	if rlm.usageTracker != nil && rlm.requestID != "" {
		rlm.usageTracker.RecordRequestStart(rlm.requestID, clientIP, userAgent, method, path, isStreaming)
		slog.Info(fmt.Sprintf("🚀 Request started [%s]", rlm.requestID))
	}
	
	// 发布请求开始事件
	if rlm.eventBus != nil {
		rlm.eventBus.Publish(events.Event{
			Type:     events.EventRequestStarted,
			Source:   "lifecycle_manager",
			Priority: events.PriorityNormal,
			Data: map[string]interface{}{
				"request_id":   rlm.requestID,
				"client_ip":    clientIP,
				"user_agent":   userAgent,
				"method":       method,
				"path":         path,
				"is_streaming": isStreaming,
				"change_type":  "request_started",
			},
		})
	}
}

// UpdateStatus 更新请求状态
// 调用 RecordRequestUpdate 记录状态变化，并实现模型信息搭便车更新机制
// 如果retryCount为-1，则使用内部attemptCounter
func (rlm *RequestLifecycleManager) UpdateStatus(status string, retryCount, httpStatus int) {
	// 处理特殊的-1标记，使用内部计数器
	actualRetryCount := retryCount
	if retryCount == -1 {
		actualRetryCount = rlm.GetAttemptCount()
	}

	// 更新内部状态 (总是更新，不管usageTracker是否为nil)
	rlm.retryCount = actualRetryCount
	rlm.lastStatus = status

	if rlm.usageTracker != nil && rlm.requestID != "" {
		// 获取当前的模型信息（线程安全）
		currentModel := rlm.GetModelName()

		// 搭便车机制：检查是否需要更新模型到数据库
		rlm.modelUpdateMu.Lock()
		shouldUpdateModel := currentModel != "" &&
							currentModel != "unknown" &&
							!rlm.modelUpdatedInDB
		if shouldUpdateModel {
			rlm.modelUpdatedInDB = true // 标记为已更新，避免重复
		}
		rlm.modelUpdateMu.Unlock()

		if shouldUpdateModel {
			// 第一次有模型信息时，执行带模型的更新
			rlm.usageTracker.RecordRequestUpdateWithModel(
				rlm.requestID, rlm.endpointName, rlm.groupName,
				status, actualRetryCount, httpStatus, currentModel)
		} else {
			// 正常状态更新（模型已更新过或尚未就绪）
			rlm.usageTracker.RecordRequestUpdate(rlm.requestID, rlm.endpointName,
				rlm.groupName, status, actualRetryCount, httpStatus)
		}
	}

	// 发布请求状态更新事件
	if rlm.eventBus != nil {
		// 根据状态确定优先级
		priority := events.PriorityNormal
		changeType := "status_changed"

		switch status {
		case "error", "timeout":
			priority = events.PriorityHigh
			changeType = "error_response"
		case "suspended":
			changeType = "suspended_change"
		case "retry":
			changeType = "retry_attempt"
		case "completed":
			changeType = "request_completed"
		}

		rlm.eventBus.Publish(events.Event{
			Type:     events.EventRequestUpdated,
			Source:   "lifecycle_manager",
			Priority: priority,
			Data: map[string]interface{}{
				"request_id":     rlm.requestID,
				"endpoint_name":  rlm.endpointName,
				"group_name":     rlm.groupName,
				"status":         status,
				"retry_count":    retryCount,
				"http_status":    httpStatus,
				"model_name":     rlm.GetModelName(),
				"change_type":    changeType,
			},
		})
	}

	// 记录状态变更日志
	switch status {
	case "forwarding":
		slog.Info(fmt.Sprintf("🎯 [请求转发] [%s] 选择端点: %s (组: %s)",
			rlm.requestID, rlm.endpointName, rlm.groupName))
	case "retry":
		slog.Info(fmt.Sprintf("🔄 [需要重试] [%s] 端点: %s (重试次数: %d)",
			rlm.requestID, rlm.endpointName, actualRetryCount))
	case "processing":
		slog.Info(fmt.Sprintf("⚙️ [请求处理] [%s] 端点: %s, 状态码: %d",
			rlm.requestID, rlm.endpointName, httpStatus))
	case "suspended":
		slog.Warn(fmt.Sprintf("⏸️ [请求挂起] [%s] 端点: %s (组: %s)",
			rlm.requestID, rlm.endpointName, rlm.groupName))
	case "cancelled":
		slog.Info(fmt.Sprintf("🚫 [请求取消] [%s] 端点: %s (组: %s)",
			rlm.requestID, rlm.endpointName, rlm.groupName))
	case "error":
		slog.Error(fmt.Sprintf("❌ [请求错误] [%s] 端点: %s, 状态码: %d",
			rlm.requestID, rlm.endpointName, httpStatus))
	case "timeout":
		slog.Error(fmt.Sprintf("⏰ [请求超时] [%s] 端点: %s",
			rlm.requestID, rlm.endpointName))
	}
}

// CompleteRequest 完成请求跟踪
// 调用 RecordRequestComplete 记录请求完成，包含Token使用信息和成本计算
// 这是所有请求完成的统一入口，确保架构一致性
func (rlm *RequestLifecycleManager) CompleteRequest(tokens *tracking.TokenUsage) {
	if rlm.usageTracker != nil && rlm.requestID != "" {
		duration := time.Since(rlm.startTime)
		
		// 使用线程安全的方式获取模型信息
		modelName := rlm.GetModelName()
		if modelName == "" {
			modelName = "unknown"
		}
		
		// 记录请求完成信息到使用跟踪器
		rlm.usageTracker.RecordRequestComplete(rlm.requestID, modelName, tokens, duration)
		
		// 同时记录到监控中间件（用于Web图表显示）
		if rlm.monitoringMiddleware != nil && tokens != nil {
			monitorTokens := &monitor.TokenUsage{
				InputTokens:         tokens.InputTokens,
				OutputTokens:        tokens.OutputTokens,
				CacheCreationTokens: tokens.CacheCreationTokens,
				CacheReadTokens:     tokens.CacheReadTokens,
			}
			rlm.monitoringMiddleware.RecordTokenUsage(rlm.requestID, rlm.endpointName, monitorTokens)
		}
		
		// 同时更新状态为完成
		rlm.UpdateStatus("completed", rlm.retryCount, 0)
		
		// 增强的完成日志，包含更详细信息
		if tokens != nil {
			totalTokens := tokens.InputTokens + tokens.OutputTokens
			cacheTokens := tokens.CacheCreationTokens + tokens.CacheReadTokens

			slog.Info(fmt.Sprintf("✅ [请求完成] [%s] 端点: %s (组: %s) (总尝试 %d 个端点)",
				rlm.requestID, rlm.endpointName, rlm.groupName, rlm.retryCount+1))
			slog.Info(fmt.Sprintf("📊 [Token统计] [%s] 模型: %s, 输入[%d] 输出[%d] 总计[%d] 缓存[%d], 耗时: %dms",
				rlm.requestID, modelName, tokens.InputTokens, tokens.OutputTokens,
				totalTokens, cacheTokens, duration.Milliseconds()))
		} else {
			slog.Info(fmt.Sprintf("✅ [请求完成] [%s] 端点: %s (组: %s), 模型: %s, 耗时: %dms (无Token统计)",
				rlm.requestID, rlm.endpointName, rlm.groupName, modelName, duration.Milliseconds()))
		}
		
		// 发布请求完成事件
		if rlm.eventBus != nil {
			duration := time.Since(rlm.startTime)
			modelName := rlm.GetModelName()
			if modelName == "" {
				modelName = "unknown"
			}
			
			// 判断是否为慢请求
			priority := events.PriorityNormal
			changeType := "request_completed"
			if duration > 10*time.Second {
				priority = events.PriorityHigh
				changeType = "slow_request_completed"
			}
			
			data := map[string]interface{}{
				"request_id":            rlm.requestID,
				"model_name":            modelName,
				"duration_ms":           duration.Milliseconds(),
				"endpoint_name":         rlm.endpointName,
				"group_name":            rlm.groupName,
				"change_type":           changeType,
			}
			
			if tokens != nil {
				data["input_tokens"] = tokens.InputTokens
				data["output_tokens"] = tokens.OutputTokens
				data["cache_creation_tokens"] = tokens.CacheCreationTokens
				data["cache_read_tokens"] = tokens.CacheReadTokens
				
				// 计算总成本（如果 tracker 有定价信息）
				if rlm.usageTracker != nil {
					pricing := rlm.usageTracker.GetPricing(modelName)
					totalCost := rlm.calculateCost(tokens, pricing)
					data["total_cost"] = totalCost
				}
			}
			
			rlm.eventBus.Publish(events.Event{
				Type:     events.EventRequestCompleted,
				Source:   "lifecycle_manager",
				Priority: priority,
				Data:     data,
			})
		}
		
		slog.Info(fmt.Sprintf("✅ Request completed [%s]", rlm.requestID))
	}
}

// HandleNonTokenResponse 处理非Token响应的Fallback机制
// 用于处理不包含Token信息的响应（如健康检查、配置查询等）
func (rlm *RequestLifecycleManager) HandleNonTokenResponse(responseContent string) {
	// 分析响应内容，确定合适的模型名
	modelName := rlm.analyzeResponseType(responseContent)
	
	// 创建空Token使用统计
	emptyTokens := &tracking.TokenUsage{
		InputTokens:         0,
		OutputTokens:        0,
		CacheCreationTokens: 0,
		CacheReadTokens:     0,
	}
	
	// 完成请求记录
	rlm.CompleteRequest(emptyTokens)
	
	slog.Info(fmt.Sprintf("🎯 [非Token响应] [%s] 模型: %s, 内容长度: %d字节", 
		rlm.requestID, modelName, len(responseContent)))
}

// analyzeResponseType 分析响应类型，返回合适的模型名
func (rlm *RequestLifecycleManager) analyzeResponseType(responseContent string) string {
	if len(responseContent) == 0 {
		return "empty_response"
	}
	
	// 检查是否为错误响应
	if strings.Contains(strings.ToLower(responseContent), "error") {
		return "error_response"
	}
	
	// 检查是否为模型列表响应（健康检查）
	if strings.Contains(responseContent, `"data"`) && 
	   strings.Contains(responseContent, `"id"`) {
		return "models_list"
	}
	
	// 检查是否为系统配置响应
	if strings.Contains(responseContent, `"config"`) || 
	   strings.Contains(responseContent, `"version"`) {
		return "config_response"
	}
	
	// 默认为非Token响应
	return "non_token_response"
}

// SetEndpoint 设置端点信息
func (rlm *RequestLifecycleManager) SetEndpoint(endpointName, groupName string) {
	rlm.endpointName = endpointName
	rlm.groupName = groupName
}

// SetModel 设置模型名称（线程安全）
// 简单版本，只在模型为空或unknown时设置
func (rlm *RequestLifecycleManager) SetModel(modelName string) {
	rlm.modelMu.Lock()
	defer rlm.modelMu.Unlock()
	
	// 只在当前模型为空或unknown时设置，避免覆盖更准确的模型信息
	if rlm.modelName == "" || rlm.modelName == "unknown" {
		rlm.modelName = modelName
		slog.Debug(fmt.Sprintf("🏷️ [模型提取] [%s] 从请求中获取模型名称: %s", rlm.requestID, modelName))
	}
}

// SetModelWithComparison 设置模型名称并进行对比检查（线程安全）
// 如果已有模型，会进行对比并在不一致时输出警告，最终以新模型为准
func (rlm *RequestLifecycleManager) SetModelWithComparison(newModelName, source string) {
	rlm.modelMu.Lock()
	defer rlm.modelMu.Unlock()
	
	// 如果新模型为空或unknown，不进行设置
	if newModelName == "" || newModelName == "unknown" {
		return
	}
	
	// 如果当前没有模型或为unknown，直接设置
	if rlm.modelName == "" || rlm.modelName == "unknown" {
		rlm.modelName = newModelName
		slog.Debug(fmt.Sprintf("🏷️ [模型提取] [%s] 从%s设置模型名称: %s", rlm.requestID, source, newModelName))
		return
	}
	
	// 如果两个模型都有值，进行对比
	if rlm.modelName != newModelName {
		slog.Warn(fmt.Sprintf("⚠️ [模型不一致] [%s] 请求体模型: %s, %s模型: %s - 以%s为准", 
			rlm.requestID, rlm.modelName, source, newModelName, source))
		
		// 以新模型（通常是message_start解析的）为准
		rlm.modelName = newModelName
	} else {
		slog.Debug(fmt.Sprintf("✅ [模型一致] [%s] 请求体与%s模型一致: %s", rlm.requestID, source, newModelName))
	}
}

// SetModelName 设置模型名称（兼容性方法，内部调用SetModel）
// 用于在流处理中动态设置正确的模型信息
func (rlm *RequestLifecycleManager) SetModelName(modelName string) {
	rlm.SetModel(modelName)
}

// GetModelName 获取当前模型名称（线程安全）
func (rlm *RequestLifecycleManager) GetModelName() string {
	rlm.modelMu.RLock()
	defer rlm.modelMu.RUnlock()
	return rlm.modelName
}

// HasModel 检查是否已有有效的模型名称（线程安全）
func (rlm *RequestLifecycleManager) HasModel() bool {
	rlm.modelMu.RLock()
	defer rlm.modelMu.RUnlock()
	return rlm.modelName != "" && rlm.modelName != "unknown"
}

// GetRequestID 获取请求ID
func (rlm *RequestLifecycleManager) GetRequestID() string {
	return rlm.requestID
}

// GetEndpointName 获取端点名称
func (rlm *RequestLifecycleManager) GetEndpointName() string {
	return rlm.endpointName
}

// GetGroupName 获取组名称  
func (rlm *RequestLifecycleManager) GetGroupName() string {
	return rlm.groupName
}

// GetDuration 获取请求持续时间
func (rlm *RequestLifecycleManager) GetDuration() time.Duration {
	return time.Since(rlm.startTime)
}

// GetLastStatus 获取最后状态
func (rlm *RequestLifecycleManager) GetLastStatus() string {
	return rlm.lastStatus
}

// GetRetryCount 获取重试次数
func (rlm *RequestLifecycleManager) GetRetryCount() int {
	return rlm.retryCount
}

// IsCompleted 检查请求是否已完成
func (rlm *RequestLifecycleManager) IsCompleted() bool {
	return rlm.lastStatus == "completed"
}

// GetStats 获取生命周期统计信息
func (rlm *RequestLifecycleManager) GetStats() map[string]any {
	stats := map[string]any{
		"request_id":    rlm.requestID,
		"endpoint":      rlm.endpointName,
		"group":         rlm.groupName,
		"model":         rlm.GetModelName(), // 线程安全获取
		"status":        rlm.lastStatus,
		"retry_count":   rlm.retryCount,
		"duration_ms":   time.Since(rlm.startTime).Milliseconds(),
		"start_time":    rlm.startTime.Format(time.RFC3339),
	}
	
	// 如果有错误信息，包含在统计中
	if rlm.lastError != nil {
		stats["last_error"] = rlm.lastError.Error()
		
		// 使用错误恢复管理器分析错误类型
		errorCtx := rlm.errorRecovery.ClassifyError(rlm.lastError, rlm.requestID, rlm.endpointName, rlm.groupName, rlm.retryCount)
		stats["error_type"] = rlm.errorRecovery.getErrorTypeName(errorCtx.ErrorType)
		stats["retryable"] = rlm.errorRecovery.ShouldRetry(errorCtx)
	}
	
	return stats
}

// PrepareErrorContext 预先注入错误上下文，在下次 HandleError 时复用
// 仅针对同一个错误对象有效，避免重复分类与重复日志
func (rlm *RequestLifecycleManager) PrepareErrorContext(errorCtx *handlers.ErrorContext) {
	rlm.pendingErrorMu.Lock()
	defer rlm.pendingErrorMu.Unlock()

	if errorCtx == nil {
		rlm.pendingErrorContext = nil
		rlm.pendingErrorOriginal = nil
		return
	}

	// 将 handlers.ErrorContext 转换为 proxy.ErrorContext，避免跨包指针依赖
	converted := &ErrorContext{
		RequestID:      errorCtx.RequestID,
		EndpointName:   errorCtx.EndpointName,
		GroupName:      errorCtx.GroupName,
		AttemptCount:   errorCtx.AttemptCount,
		ErrorType:      ErrorType(errorCtx.ErrorType),
		OriginalError:  errorCtx.OriginalError,
		RetryableAfter: errorCtx.RetryableAfter,
		MaxRetries:     errorCtx.MaxRetries,
	}

	rlm.pendingErrorContext = converted
	rlm.pendingErrorOriginal = errorCtx.OriginalError
}

// consumePreparedErrorContext 尝试取出与指定错误匹配的预计算上下文
func (rlm *RequestLifecycleManager) consumePreparedErrorContext(err error) *ErrorContext {
	rlm.pendingErrorMu.Lock()
	defer rlm.pendingErrorMu.Unlock()

	if rlm.pendingErrorContext == nil || err == nil {
		return nil
	}

	// 只有当错误对象匹配时才复用，确保不跨错误复用
	if rlm.pendingErrorOriginal != nil {
		if err == rlm.pendingErrorOriginal || errors.Is(err, rlm.pendingErrorOriginal) {
			ctx := rlm.pendingErrorContext
			rlm.pendingErrorContext = nil
			rlm.pendingErrorOriginal = nil
			return ctx
		}
	}

	// 不匹配则丢弃预计算结果，避免影响后续错误
	rlm.pendingErrorContext = nil
	rlm.pendingErrorOriginal = nil
	return nil
}

// HandleError 处理请求过程中的错误
func (rlm *RequestLifecycleManager) HandleError(err error) {
	if err == nil {
		return
	}
	
	rlm.lastError = err

	// 优先复用预计算的错误分类，避免重复日志
	errorCtx := rlm.consumePreparedErrorContext(err)
	if errorCtx == nil {
		errorCtx = rlm.errorRecovery.ClassifyError(err, rlm.requestID, rlm.endpointName, rlm.groupName, rlm.retryCount)
	}
	
	// 根据错误类型更新状态
	switch errorCtx.ErrorType {
	case ErrorTypeClientCancel:
		rlm.UpdateStatus("cancelled", rlm.retryCount, 0)
	case ErrorTypeNetwork:
		rlm.UpdateStatus("network_error", rlm.retryCount, 0)
	case ErrorTypeTimeout:
		rlm.UpdateStatus("timeout", rlm.retryCount, 0)
	case ErrorTypeAuth:
		rlm.UpdateStatus("auth_error", rlm.retryCount, 401)
	case ErrorTypeRateLimit:
		rlm.UpdateStatus("rate_limited", rlm.retryCount, 429)
	case ErrorTypeStream:
		rlm.UpdateStatus("stream_error", rlm.retryCount, 0)
	default:
		rlm.UpdateStatus("error", rlm.retryCount, 0)
	}
	
	slog.Error(fmt.Sprintf("⚠️ [生命周期错误] [%s] 错误类型: %s, 错误: %v",
		rlm.requestID, rlm.errorRecovery.getErrorTypeName(errorCtx.ErrorType), err))
}

// IncrementRetry 增加重试计数
func (rlm *RequestLifecycleManager) IncrementRetry() {
	rlm.retryCount++
	slog.Info(fmt.Sprintf("🔄 [重试计数] [%s] 重试次数: %d", rlm.requestID, rlm.retryCount))
}

// GetLastError 获取最后一次错误
func (rlm *RequestLifecycleManager) GetLastError() error {
	return rlm.lastError
}

// calculateCost 计算Token使用成本的辅助方法
func (rlm *RequestLifecycleManager) calculateCost(tokens *tracking.TokenUsage, pricing tracking.ModelPricing) float64 {
	if tokens == nil {
		return 0.0
	}

	inputCost := float64(tokens.InputTokens) * pricing.Input / 1000000
	outputCost := float64(tokens.OutputTokens) * pricing.Output / 1000000
	cacheCost := float64(tokens.CacheCreationTokens) * pricing.CacheCreation / 1000000

	return inputCost + outputCost + cacheCost
}

// SetFinalStatusCode 设置最终状态码
// 用于记录请求的实际HTTP状态码，替代硬编码的状态码
func (rlm *RequestLifecycleManager) SetFinalStatusCode(statusCode int) {
	rlm.finalStatusCode = statusCode
}

// GetFinalStatusCode 获取最终状态码
func (rlm *RequestLifecycleManager) GetFinalStatusCode() int {
	return rlm.finalStatusCode
}

// RecordTokensForFailedRequest 为失败请求记录Token信息
// 与 CompleteRequest 的区别：只记录Token统计，不改变请求状态
func (rlm *RequestLifecycleManager) RecordTokensForFailedRequest(tokens *tracking.TokenUsage, failureReason string) {
	if rlm.requestID != "" && tokens != nil {
		// ✅ 检查是否有真实的Token使用
		hasRealTokens := tokens.InputTokens > 0 || tokens.OutputTokens > 0 ||
			tokens.CacheCreationTokens > 0 || tokens.CacheReadTokens > 0

		if !hasRealTokens {
			// 空Token信息不记录
			slog.Debug(fmt.Sprintf("⏭️ [跳过空Token] [%s] 失败请求无实际Token消耗", rlm.requestID))
			return
		}

		duration := time.Since(rlm.startTime)
		modelName := rlm.GetModelName()
		if modelName == "" {
			modelName = "unknown"
		}

		// ✅ 只记录Token统计到UsageTracker，不调用 RecordRequestComplete
		if rlm.usageTracker != nil {
			rlm.usageTracker.RecordFailedRequestTokens(rlm.requestID, modelName, tokens, duration, failureReason)
		}

		// ✅ 记录到监控中间件（总是调用，即使usageTracker为nil）
		if rlm.monitoringMiddleware != nil {
			monitorTokens := &monitor.TokenUsage{
				InputTokens:         tokens.InputTokens,
				OutputTokens:        tokens.OutputTokens,
				CacheCreationTokens: tokens.CacheCreationTokens,
				CacheReadTokens:     tokens.CacheReadTokens,
			}
			// 新增失败请求Token记录方法
			rlm.monitoringMiddleware.RecordFailedRequestTokens(rlm.requestID, rlm.endpointName, monitorTokens, failureReason)
		}

		slog.Info(fmt.Sprintf("💾 [失败请求Token记录] [%s] 端点: %s, 原因: %s, 模型: %s, 输入: %d, 输出: %d",
			rlm.requestID, rlm.endpointName, failureReason, modelName, tokens.InputTokens, tokens.OutputTokens))
	}
}

// IncrementAttempt 线程安全地增加尝试计数
// 用于统一重试计数语义，每次端点切换或重试时调用
func (rlm *RequestLifecycleManager) IncrementAttempt() int {
	rlm.attemptMu.Lock()
	defer rlm.attemptMu.Unlock()
	rlm.attemptCounter++
	slog.Debug(fmt.Sprintf("🔢 [尝试计数] [%s] 当前尝试次数: %d", rlm.requestID, rlm.attemptCounter))
	return rlm.attemptCounter
}

// GetAttemptCount 线程安全地获取当前尝试次数
// 返回真实的尝试次数，用于数据库记录和监控
func (rlm *RequestLifecycleManager) GetAttemptCount() int {
	rlm.attemptMu.Lock()
	defer rlm.attemptMu.Unlock()
	return rlm.attemptCounter
}

// OnRetryDecision 处理重试决策结果
func (rlm *RequestLifecycleManager) OnRetryDecision(decision RetryDecision, httpStatus int) {
	actualRetryCount := rlm.GetAttemptCount()

	if decision.RetrySameEndpoint {
		rlm.UpdateStatus("retry", actualRetryCount, httpStatus)
	} else if decision.FinalStatus != "" {
		rlm.UpdateStatus(decision.FinalStatus, actualRetryCount, httpStatus)
	}

	// 记录决策原因
	slog.Debug(fmt.Sprintf("📋 [重试决策记录] [%s] 状态: %s, 原因: %s",
		rlm.requestID, decision.FinalStatus, decision.Reason))
}

// GetRetryContext 获取重试上下文信息
func (rlm *RequestLifecycleManager) GetRetryContext(endpoint *endpoint.Endpoint, err error, attempt int) RetryContext {
	errorRecovery := rlm.errorRecovery
	errorCtx := errorRecovery.ClassifyError(err, rlm.requestID, rlm.endpointName, rlm.groupName, attempt-1)

	return RetryContext{
		RequestID:     rlm.requestID,
		Endpoint:      endpoint,
		Attempt:       attempt,
		AttemptGlobal: rlm.GetAttemptCount(),
		Error:         errorCtx,
		IsStreaming:   false, // 由调用方设置
	}
}