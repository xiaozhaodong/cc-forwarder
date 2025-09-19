package retry

import (
	"cc-forwarder/internal/endpoint"
)

// ErrorContext 是从 proxy 包导入的错误上下文类型
// 由于在同一个模块内，这里通过导入路径引用
// 实际的 ErrorContext 定义在 internal/proxy/error_recovery.go 中
type ErrorContext struct {
	RequestID      string
	EndpointName   string
	GroupName      string
	AttemptCount   int
	ErrorType      interface{} // 避免导入 ErrorType 枚举
	OriginalError  error
	RetryableAfter interface{} // 建议重试延迟
	MaxRetries     int
}

// RetryContext 重试上下文信息
type RetryContext struct {
	RequestID     string              // 请求ID
	Endpoint      *endpoint.Endpoint  // 当前端点
	Attempt       int                 // 当前端点尝试次数（从1开始）
	AttemptGlobal int                 // 全局尝试次数
	Error         *ErrorContext       // 错误上下文（指针类型）
	IsStreaming   bool                // 是否为流式请求
}