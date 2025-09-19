package retry

import (
	"context"

	"cc-forwarder/internal/tracking"
)

// SuspensionManager 挂起管理器接口
// 避免导入循环依赖，在此处定义接口
type SuspensionManager interface {
	ShouldSuspend(ctx context.Context) bool
	WaitForGroupSwitch(ctx context.Context, connID string) bool
	GetSuspendedRequestsCount() int
}

// ErrorRecoveryFactory 错误恢复管理器工厂接口
// 避免导入循环依赖，在此处定义接口
type ErrorRecoveryFactory interface {
	NewErrorRecoveryManager(usageTracker *tracking.UsageTracker) ErrorRecoveryManager
}

// ErrorRecoveryManager 错误恢复管理器接口
// 避免导入循环依赖，在此处定义接口
type ErrorRecoveryManager interface {
	ClassifyError(err error, connID, endpointName, groupName string, attemptCount int) ErrorContext
	HandleFinalFailure(errorCtx ErrorContext)
	GetErrorTypeName(errorType interface{}) string
}

// RequestLifecycleManager 请求生命周期管理器接口
// 避免导入循环依赖，在此处定义接口
type RequestLifecycleManager interface {
	GetRequestID() string
	SetEndpoint(name, group string)
	SetModel(modelName string)
	SetModelWithComparison(modelName, source string)
	HasModel() bool
	UpdateStatus(status string, endpointIndex, statusCode int)
	HandleError(err error)
	CompleteRequest(tokens *tracking.TokenUsage)
	HandleNonTokenResponse(responseContent string)
	RecordTokensForFailedRequest(tokens *tracking.TokenUsage, failureReason string)
	IncrementAttempt() int
	GetAttemptCount() int
}