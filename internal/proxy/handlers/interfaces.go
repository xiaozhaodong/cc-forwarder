package handlers

import (
	"context"
	"net/http"
	"time"

	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/tracking"
)

// RequestLifecycleManager 请求生命周期管理器接口
type RequestLifecycleManager interface {
	GetRequestID() string
	SetEndpoint(name, group string)
	UpdateStatus(status string, endpointIndex, statusCode int)
	HandleError(err error)
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
	ErrorTypeStream
	ErrorTypeAuth
	ErrorTypeRateLimit
	ErrorTypeParsing
	ErrorTypeClientCancel
)

// TokenParser Token解析器接口
type TokenParser interface {
	ParseSSELine(line string) interface{} // 返回TokenUsage类型
	SetModelName(model string)
}

// StreamProcessor 流式处理器接口
type StreamProcessor interface {
	ProcessStreamWithRetry(ctx context.Context, resp *http.Response) error
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
	AnalyzeResponseForTokensUnified(responseBytes []byte, connID, endpointName string, lifecycleManager RequestLifecycleManager)
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