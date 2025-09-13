package proxy

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"cc-forwarder/internal/monitor"
	"cc-forwarder/internal/tracking"
)

// MonitoringMiddlewareInterface 定义监控中间件接口
type MonitoringMiddlewareInterface interface {
	RecordTokenUsage(connID string, endpoint string, tokens *monitor.TokenUsage)
}

// RequestLifecycleManager 请求生命周期管理器
// 负责管理请求的完整生命周期，确保所有请求都有完整的跟踪记录
type RequestLifecycleManager struct {
	usageTracker        *tracking.UsageTracker        // 使用跟踪器
	monitoringMiddleware MonitoringMiddlewareInterface // 监控中间件
	errorRecovery       *ErrorRecoveryManager         // 错误恢复管理器
	requestID           string                        // 请求唯一标识符
	startTime           time.Time                     // 请求开始时间
	modelName           string                        // 模型名称
	endpointName        string                        // 端点名称
	groupName           string                        // 组名称
	retryCount          int                           // 重试计数
	lastStatus          string                        // 最后状态
	lastError           error                         // 最后一次错误
}

// NewRequestLifecycleManager 创建新的请求生命周期管理器
func NewRequestLifecycleManager(usageTracker *tracking.UsageTracker, monitoringMiddleware MonitoringMiddlewareInterface, requestID string) *RequestLifecycleManager {
	return &RequestLifecycleManager{
		usageTracker:        usageTracker,
		monitoringMiddleware: monitoringMiddleware,
		errorRecovery:       NewErrorRecoveryManager(usageTracker),
		requestID:           requestID,
		startTime:           time.Now(),
		lastStatus:          "pending",
	}
}

// StartRequest 开始请求跟踪
// 调用 RecordRequestStart 记录请求开始
func (rlm *RequestLifecycleManager) StartRequest(clientIP, userAgent string) {
	if rlm.usageTracker != nil && rlm.requestID != "" {
		rlm.usageTracker.RecordRequestStart(rlm.requestID, clientIP, userAgent)
		slog.Info(fmt.Sprintf("🚀 Request started [%s]", rlm.requestID))
	}
}

// UpdateStatus 更新请求状态
// 调用 RecordRequestUpdate 记录状态变化
func (rlm *RequestLifecycleManager) UpdateStatus(status string, retryCount, httpStatus int) {
	// 更新内部状态 (总是更新，不管usageTracker是否为nil)
	rlm.retryCount = retryCount
	rlm.lastStatus = status
	
	if rlm.usageTracker != nil && rlm.requestID != "" {
		rlm.usageTracker.RecordRequestUpdate(rlm.requestID, rlm.endpointName, 
			rlm.groupName, status, retryCount, httpStatus)
	}
	
	// 记录状态变更日志
	switch status {
	case "forwarding":
		slog.Info(fmt.Sprintf("🎯 [请求转发] [%s] 选择端点: %s (组: %s)", 
			rlm.requestID, rlm.endpointName, rlm.groupName))
	case "retry":
		slog.Info(fmt.Sprintf("🔄 [需要重试] [%s] 端点: %s (重试次数: %d)", 
			rlm.requestID, rlm.endpointName, retryCount))
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
		
		// 使用Token中的模型信息，如果没有则使用默认值
		modelName := rlm.modelName
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
			
			slog.Info(fmt.Sprintf("✅ [请求成功] [%s] 端点: %s (组: %s), 状态码: 200 (总尝试 %d 个端点)", 
				rlm.requestID, rlm.endpointName, rlm.groupName, rlm.retryCount+1))
			slog.Info(fmt.Sprintf("📊 [Token统计] [%s] 模型: %s, 输入[%d] 输出[%d] 总计[%d] 缓存[%d], 耗时: %dms", 
				rlm.requestID, modelName, tokens.InputTokens, tokens.OutputTokens, 
				totalTokens, cacheTokens, duration.Milliseconds()))
		} else {
			slog.Info(fmt.Sprintf("✅ [请求成功] [%s] 端点: %s (组: %s), 模型: %s, 耗时: %dms (无Token统计)", 
				rlm.requestID, rlm.endpointName, rlm.groupName, modelName, duration.Milliseconds()))
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

// SetModel 设置模型名称
func (rlm *RequestLifecycleManager) SetModel(modelName string) {
	rlm.modelName = modelName
}

// SetModelName 设置模型名称
// 用于在流处理中动态设置正确的模型信息
func (rlm *RequestLifecycleManager) SetModelName(modelName string) {
	rlm.modelName = modelName
	slog.Debug(fmt.Sprintf("🏷️ [模型设置] [%s] 设置模型名称: %s", rlm.requestID, modelName))
}

// GetModelName 获取当前模型名称
func (rlm *RequestLifecycleManager) GetModelName() string {
	return rlm.modelName
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
		"model":         rlm.modelName,
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

// HandleError 处理请求过程中的错误
func (rlm *RequestLifecycleManager) HandleError(err error) {
	if err == nil {
		return
	}
	
	rlm.lastError = err
	
	// 使用错误恢复管理器分类错误
	errorCtx := rlm.errorRecovery.ClassifyError(err, rlm.requestID, rlm.endpointName, rlm.groupName, rlm.retryCount)
	
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

// ShouldRetry 判断是否应该重试
func (rlm *RequestLifecycleManager) ShouldRetry() bool {
	if rlm.lastError == nil {
		return false
	}
	
	errorCtx := rlm.errorRecovery.ClassifyError(rlm.lastError, rlm.requestID, rlm.endpointName, rlm.groupName, rlm.retryCount)
	return rlm.errorRecovery.ShouldRetry(errorCtx)
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