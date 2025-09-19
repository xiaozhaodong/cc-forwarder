package retry

import (
	"context"
	"log/slog"

	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/tracking"
)


// RetryController 统一重试控制器
// 核心组件：负责根据错误结果做出重试决策
type RetryController struct {
	policy           RetryPolicy
	suspension       SuspensionManager
	errorFactory     ErrorRecoveryFactory
	lifecycle        RequestLifecycleManager
	usageTracker     *tracking.UsageTracker
	errorRecovery    ErrorRecoveryManager  // 缓存的错误恢复管理器
}

// NewRetryController 创建新的重试控制器
// 接收所有依赖项并创建控制器实例
func NewRetryController(
	policy RetryPolicy,
	suspension SuspensionManager,
	errorFactory ErrorRecoveryFactory,
	lifecycle RequestLifecycleManager,
	usageTracker *tracking.UsageTracker,
) *RetryController {
	return &RetryController{
		policy:       policy,
		suspension:   suspension,
		errorFactory: errorFactory,
		lifecycle:    lifecycle,
		usageTracker: usageTracker,
	}
}

// OnAttemptResult 处理尝试结果并返回重试决策
// 这是重试控制器的核心方法，负责：
// 1. 统一的尝试计数管理
// 2. 错误分类和处理
// 3. 策略决策
// 4. 状态更新
// 5. 日志记录
func (rc *RetryController) OnAttemptResult(
	ctx context.Context,
	endpoint *endpoint.Endpoint,
	err error,
	attempt int,
	isStreaming bool,
) (RetryDecision, error) {
	requestID := rc.lifecycle.GetRequestID()

	// 🔢 [关键修复] 每次尝试都要计数，不论成功失败或后续决策
	// 这确保了所有真实的HTTP尝试都被正确记录
	currentAttemptCount := rc.lifecycle.IncrementAttempt()

	// 如果没有错误，返回成功决策
	if err == nil {
		decision := RetryDecision{
			RetrySameEndpoint: false,
			SwitchEndpoint:    false,
			SuspendRequest:    false,
			FinalStatus:       "completed",
			Reason:           "请求成功完成",
		}
		rc.logDecision(requestID, decision, endpoint, attempt)
		return decision, nil
	}

	// 处理错误：调用lifecycle.HandleError
	rc.lifecycle.HandleError(err)

	// 懒加载错误恢复管理器
	if rc.errorRecovery == nil {
		rc.errorRecovery = rc.errorFactory.NewErrorRecoveryManager(rc.usageTracker)
	}

	// 分类错误 - 🔧 [修复] 保持与旧实现一致的退避延迟计算
	endpointName := "unknown"
	groupName := "unknown"
	if endpoint != nil {
		endpointName = endpoint.Config.Name
		groupName = endpoint.Config.Group
	}

	// ErrorRecoveryManager期望attempt从0开始计算退避延迟
	// 第一次重试(attempt=1)应该使用基础延迟，所以传入attempt-1
	attemptForRecovery := max(0, attempt-1)
	errorCtxFromRecovery := rc.errorRecovery.ClassifyError(err, requestID, endpointName, groupName, attemptForRecovery)

	// 转换为 retry 包的 ErrorContext 结构
	retryErrorCtx := ErrorContext{
		RequestID:      errorCtxFromRecovery.RequestID,
		EndpointName:   errorCtxFromRecovery.EndpointName,
		GroupName:      errorCtxFromRecovery.GroupName,
		AttemptCount:   errorCtxFromRecovery.AttemptCount,
		ErrorType:      errorCtxFromRecovery.ErrorType,  // 保持原类型
		OriginalError:  errorCtxFromRecovery.OriginalError,
		RetryableAfter: errorCtxFromRecovery.RetryableAfter,  // 转换为 interface{}
		MaxRetries:     errorCtxFromRecovery.MaxRetries,
	}

	// 构建重试上下文，使用统一管理的计数
	retryContext := RetryContext{
		RequestID:     requestID,
		Endpoint:      endpoint,
		Attempt:       attempt,
		AttemptGlobal: currentAttemptCount, // 🔢 [修复] 使用刚刚增加的计数
		Error:         &retryErrorCtx,
		IsStreaming:   isStreaming,
	}

	// 使用策略做决策
	decision := rc.policy.Decide(retryContext)

	// 🔧 [重构] 移除重复的计数逻辑，状态更新使用统一的计数
	if decision.RetrySameEndpoint {
		// 更新状态为重试，使用统一管理的计数
		rc.lifecycle.UpdateStatus("retry", currentAttemptCount, 0)
		slog.Info("重试状态更新",
			"request_id", requestID,
			"attempt", currentAttemptCount,
			"endpoint", endpointName)
	}

	// 记录决策日志
	rc.logDecision(requestID, decision, endpoint, attempt)

	return decision, nil
}

// logDecision 记录重试决策日志
// 提供清晰的决策过程日志记录
func (rc *RetryController) logDecision(
	requestID string,
	decision RetryDecision,
	endpoint *endpoint.Endpoint,
	attempt int,
) {
	endpointName := "unknown"
	if endpoint != nil {
		endpointName = endpoint.Config.Name
	}

	// 根据决策类型记录不同的日志
	if decision.RetrySameEndpoint {
		slog.Info("🔄 [重试决策] 在同一端点重试",
			"request_id", requestID,
			"endpoint", endpointName,
			"attempt", attempt,
			"delay", decision.Delay,
			"reason", decision.Reason)
	} else if decision.SwitchEndpoint {
		slog.Info("🔀 [重试决策] 切换到下一端点",
			"request_id", requestID,
			"current_endpoint", endpointName,
			"attempt", attempt,
			"delay", decision.Delay,
			"reason", decision.Reason)
	} else if decision.SuspendRequest {
		slog.Info("⏸️ [重试决策] 尝试挂起请求",
			"request_id", requestID,
			"endpoint", endpointName,
			"attempt", attempt,
			"final_status", decision.FinalStatus,
			"reason", decision.Reason)
	} else if decision.FinalStatus == "completed" {
		// 🔧 [修复] 成功请求不应被误报为"终止重试"
		// 成功完成的请求应该有专门的成功日志
		slog.Info("✅ [重试决策] 请求成功完成",
			"request_id", requestID,
			"endpoint", endpointName,
			"attempt", attempt,
			"reason", decision.Reason)
	} else {
		// 真正的终止重试（错误、认证失败等）
		slog.Info("❌ [重试决策] 终止重试",
			"request_id", requestID,
			"endpoint", endpointName,
			"attempt", attempt,
			"final_status", decision.FinalStatus,
			"reason", decision.Reason)
	}
}