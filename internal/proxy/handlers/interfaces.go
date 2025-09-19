package handlers

import (
	"context"
	"net/http"
	"time"

	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/monitor"
	"cc-forwarder/internal/tracking"
)

// RequestLifecycleManager 请求生命周期管理器接口
// 修改版本：添加CompleteRequest和HandleNonTokenResponse方法以支持生命周期管理器架构
type RequestLifecycleManager interface {
	GetRequestID() string
	SetEndpoint(name, group string)
	SetModel(modelName string)                               // 简单设置模型
	SetModelWithComparison(modelName, source string)        // 带对比的设置模型
	HasModel() bool                                          // 检查是否已有模型
	UpdateStatus(status string, endpointIndex, statusCode int)
	HandleError(err error)
	// 新增方法：统一的请求完成入口
	CompleteRequest(tokens *tracking.TokenUsage)
	HandleNonTokenResponse(responseContent string)
	// 失败请求Token记录方法：只记录Token统计，不改变请求状态
	RecordTokensForFailedRequest(tokens *tracking.TokenUsage, failureReason string)
	// 🔢 [语义修复] 新增尝试计数管理方法
	IncrementAttempt() int      // 线程安全地增加尝试计数，返回当前计数
	GetAttemptCount() int       // 线程安全地获取当前尝试次数
}

// ErrorRecoveryManager 错误恢复管理器接口
type ErrorRecoveryManager interface {
	ClassifyError(err error, connID, endpointName, groupName string, attemptCount int) ErrorContext
	HandleFinalFailure(errorCtx ErrorContext)
	GetErrorTypeName(errorType ErrorType) string
}

// ErrorContext 错误上下文信息
type ErrorContext struct {
	RequestID      string
	EndpointName   string 
	GroupName      string
	AttemptCount   int
	ErrorType      ErrorType
	OriginalError  error
	RetryableAfter time.Duration
	MaxRetries     int
}

// ErrorType 错误类型枚举
type ErrorType int

const (
	ErrorTypeUnknown ErrorType = iota
	ErrorTypeNetwork
	ErrorTypeTimeout
	ErrorTypeHTTP
	ErrorTypeServerError
	ErrorTypeStream
	ErrorTypeAuth
	ErrorTypeRateLimit
	ErrorTypeParsing
	ErrorTypeClientCancel
)

// TokenParser Token解析器接口
type TokenParser interface {
	ParseSSELine(line string) *monitor.TokenUsage // 返回TokenUsage类型
	SetModelName(model string)
}

// StreamProcessor 流式处理器接口
// 修改版本：返回Token使用信息和模型名称而非直接记录到usageTracker
type StreamProcessor interface {
	ProcessStreamWithRetry(ctx context.Context, resp *http.Response) (*tracking.TokenUsage, string, error)
}

// RetryHandler 重试处理器接口  
type RetryHandler interface {
	ExecuteWithContext(ctx context.Context, operation func(*endpoint.Endpoint, string) (*http.Response, error), connID string) (*http.Response, error)
	ShouldSuspendRequest(ctx context.Context) bool
	WaitForGroupSwitch(ctx context.Context, connID string) bool
	SetEndpointManager(manager interface{})
	SetUsageTracker(tracker *tracking.UsageTracker)
}

// TokenParserFactory Token解析器工厂接口
type TokenParserFactory interface {
	NewTokenParserWithUsageTracker(connID string, usageTracker *tracking.UsageTracker) TokenParser
}

// StreamProcessorFactory 流式处理器工厂接口
type StreamProcessorFactory interface {
	NewStreamProcessor(tokenParser TokenParser, usageTracker *tracking.UsageTracker, 
		w http.ResponseWriter, flusher http.Flusher, requestID, endpoint string) StreamProcessor
}

// ErrorRecoveryFactory 错误恢复管理器工厂接口
type ErrorRecoveryFactory interface {
	NewErrorRecoveryManager(usageTracker *tracking.UsageTracker) ErrorRecoveryManager
}

// RetryHandlerFactory 重试处理器工厂接口
type RetryHandlerFactory interface {
	NewRetryHandler(config interface{}) RetryHandler
}

// TokenAnalyzer Token分析器接口
type TokenAnalyzer interface {
	AnalyzeResponseForTokens(ctx context.Context, responseBody, endpointName string, r *http.Request)
	AnalyzeResponseForTokensUnified(responseBytes []byte, connID, endpointName string) (*tracking.TokenUsage, string)
}

// ResponseProcessor 响应处理器接口
type ResponseProcessor interface {
	CopyResponseHeaders(resp *http.Response, w http.ResponseWriter)
	ProcessResponseBody(resp *http.Response) ([]byte, error)
	ReadAndDecompressResponse(ctx context.Context, resp *http.Response, endpointName string) ([]byte, error)
}

// TokenAnalyzerFactory Token分析器工厂接口
type TokenAnalyzerFactory interface {
	NewTokenAnalyzer(usageTracker *tracking.UsageTracker) TokenAnalyzer
}

// ResponseProcessorFactory 响应处理器工厂接口
type ResponseProcessorFactory interface {
	NewResponseProcessor() ResponseProcessor
}

// RetryManagerFactory 重试管理器工厂接口
type RetryManagerFactory interface {
	NewRetryManager() RetryManager
}

// SuspensionManagerFactory 挂起管理器工厂接口
type SuspensionManagerFactory interface {
	NewSuspensionManager() SuspensionManager
}

// RetryDecision 统一重试决策结果
// 包含重试策略的完整决策信息，用于替代原有的复杂RetryController机制
type RetryDecision struct {
	RetrySameEndpoint bool          // 是否继续在当前端点重试
	SwitchEndpoint    bool          // 是否切换到下一端点
	SuspendRequest    bool          // 是否尝试挂起请求
	Delay             time.Duration // 重试延迟时间
	FinalStatus       string        // 若终止，应记录的最终状态
	Reason            string        // 决策原因（用于日志）
}

// RetryManager 重试管理器接口
type RetryManager interface {
	ShouldRetry(errorCtx *ErrorContext, attempt int) (bool, time.Duration)
	GetHealthyEndpoints(ctx context.Context) []*endpoint.Endpoint
	GetMaxAttempts() int
	// ShouldRetryWithDecision 统一重试决策方法
	// 完全复制retry/policy.go的决策逻辑，确保行为一致
	// errorCtx: 错误上下文信息
	// localAttempt: 当前端点的尝试次数（从1开始，用于退避计算）
	// globalAttempt: 全局尝试次数（用于限流策略）
	// isStreaming: 是否为流式请求
	ShouldRetryWithDecision(errorCtx *ErrorContext, localAttempt int, globalAttempt int, isStreaming bool) RetryDecision
}

// SuspensionManager 挂起管理器接口
type SuspensionManager interface {
	ShouldSuspend(ctx context.Context) bool
	WaitForGroupSwitch(ctx context.Context, connID string) bool
	GetSuspendedRequestsCount() int
}

// GetDefaultStatusCodeForFinalStatus 根据最终状态获取默认HTTP状态码
// 用于在RetryDecision中没有明确状态码时提供合理默认值
//
// 工具函数签名（应在具体实现中定义）:
// func GetDefaultStatusCodeForFinalStatus(finalStatus string) int