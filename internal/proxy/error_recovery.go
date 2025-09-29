package proxy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"

	"cc-forwarder/internal/tracking"
)

// ErrorType 错误类型枚举
type ErrorType int

const (
	ErrorTypeUnknown              ErrorType = iota
	ErrorTypeNetwork                      // 网络错误
	ErrorTypeTimeout                      // 超时错误
	ErrorTypeHTTP                         // HTTP错误
	ErrorTypeServerError                  // 服务器错误（5xx）
	ErrorTypeStream                       // 流式处理错误
	ErrorTypeAuth                         // 认证错误
	ErrorTypeRateLimit                    // 限流错误
	ErrorTypeParsing                      // 解析错误
	ErrorTypeClientCancel                 // 客户端取消错误
	ErrorTypeNoHealthyEndpoints           // 没有健康端点可用
)

// ErrorContext 错误上下文信息
type ErrorContext struct {
	RequestID      string
	EndpointName   string
	GroupName      string
	AttemptCount   int
	ErrorType      ErrorType
	OriginalError  error
	RetryableAfter time.Duration // 建议重试延迟
	MaxRetries     int
}

// ErrorRecoveryManager 错误恢复管理器
// 负责识别错误类型、制定恢复策略、执行恢复操作
type ErrorRecoveryManager struct {
	usageTracker  *tracking.UsageTracker
	maxRetries    int
	baseDelay     time.Duration
	maxDelay      time.Duration
	backoffFactor float64
}

// NewErrorRecoveryManager 创建错误恢复管理器
func NewErrorRecoveryManager(usageTracker *tracking.UsageTracker) *ErrorRecoveryManager {
	return &ErrorRecoveryManager{
		usageTracker:  usageTracker,
		maxRetries:    3,
		baseDelay:     time.Second,
		maxDelay:      30 * time.Second,
		backoffFactor: 2.0,
	}
}

// ClassifyError 分类错误类型并创建错误上下文
func (erm *ErrorRecoveryManager) ClassifyError(err error, requestID, endpoint, group string, attempt int) *ErrorContext {
	errorCtx := &ErrorContext{
		RequestID:     requestID,
		EndpointName:  endpoint,
		GroupName:     group,
		AttemptCount:  attempt,
		OriginalError: err,
		MaxRetries:    erm.maxRetries,
	}

	if err == nil {
		errorCtx.ErrorType = ErrorTypeUnknown
		return errorCtx
	}

	errStr := strings.ToLower(err.Error())

	// 首先检查客户端取消错误（最高优先级）
	if erm.isClientCancelError(err) {
		errorCtx.ErrorType = ErrorTypeClientCancel
		errorCtx.RetryableAfter = 0 // 客户端取消不可重试
		slog.Info(fmt.Sprintf("🚫 [客户端取消分类] [%s] 端点: %s, 尝试: %d, 错误: %v",
			requestID, endpoint, attempt, err))
		return errorCtx
	}

	// 其次检查超时错误（优先级高于网络错误）
	if erm.isTimeoutError(err) {
		errorCtx.ErrorType = ErrorTypeTimeout
		errorCtx.RetryableAfter = erm.calculateBackoffDelay(attempt)
		slog.Warn(fmt.Sprintf("⏰ [超时错误分类] [%s] 端点: %s, 尝试: %d, 错误: %v",
			requestID, endpoint, attempt, err))
		return errorCtx
	}

	// 网络错误分类（在超时错误之后检查）
	if erm.isNetworkError(err) {
		errorCtx.ErrorType = ErrorTypeNetwork
		errorCtx.RetryableAfter = erm.calculateBackoffDelay(attempt)
		slog.Warn(fmt.Sprintf("🌐 [网络错误分类] [%s] 端点: %s, 尝试: %d, 错误: %v",
			requestID, endpoint, attempt, err))
		return errorCtx
	}

	// 限流错误分类 - 高优先级，必须在服务器错误和HTTP通用检查之前
	// 现在包含400错误码，因为400有时表示请求频率过高或临时的请求格式问题
	if strings.Contains(errStr, "rate") || strings.Contains(errStr, "429") ||
		strings.Contains(errStr, "quota") || strings.Contains(errStr, "limit") ||
		strings.Contains(errStr, "endpoint returned error: 429") ||
		strings.Contains(errStr, "endpoint returned error: 400") ||
		strings.Contains(errStr, "400") ||
		strings.Contains(errStr, "too many requests") || strings.Contains(errStr, "rate_limit") ||
		strings.Contains(errStr, "throttle") || strings.Contains(errStr, "quota exceeded") {
		errorCtx.ErrorType = ErrorTypeRateLimit
		errorCtx.RetryableAfter = time.Minute // 限流错误建议等待1分钟
		slog.Warn(fmt.Sprintf("🚦 [限流错误分类] [%s] 端点: %s, 尝试: %d, 错误: %v",
			requestID, endpoint, attempt, err))
		return errorCtx
	}

	// 服务器错误分类（5xx）- 优先级高于通用HTTP错误
	if strings.Contains(errStr, "endpoint returned error: 5") ||
		strings.Contains(errStr, "500") || strings.Contains(errStr, "501") ||
		strings.Contains(errStr, "502") || strings.Contains(errStr, "503") ||
		strings.Contains(errStr, "504") || strings.Contains(errStr, "505") ||
		strings.Contains(errStr, "520") || strings.Contains(errStr, "521") ||
		strings.Contains(errStr, "522") || strings.Contains(errStr, "523") ||
		strings.Contains(errStr, "524") || strings.Contains(errStr, "525") {
		errorCtx.ErrorType = ErrorTypeServerError
		errorCtx.RetryableAfter = erm.calculateBackoffDelay(attempt)
		slog.Warn(fmt.Sprintf("🚨 [服务器错误分类] [%s] 端点: %s, 尝试: %d, 错误: %v",
			requestID, endpoint, attempt, err))
		return errorCtx
	}

	// 认证错误分类
	if strings.Contains(errStr, "auth") || strings.Contains(errStr, "unauthorized") || strings.Contains(errStr, "401") {
		errorCtx.ErrorType = ErrorTypeAuth
		// 认证错误通常不可重试
		errorCtx.RetryableAfter = 0
		slog.Error(fmt.Sprintf("🔐 [认证错误分类] [%s] 端点: %s, 尝试: %d, 错误: %v",
			requestID, endpoint, attempt, err))
		return errorCtx
	}

	// 流处理错误分类 - 高优先级，必须在HTTP错误检查之前
	// 使用精确匹配，避免误判普通网络错误（如"upstream connect error"）
	if strings.HasPrefix(errStr, "stream_status:") ||
		strings.Contains(errStr, "streaming not supported") ||
		strings.Contains(errStr, "stream_error") ||
		strings.Contains(errStr, "sse") ||
		strings.Contains(errStr, "event-stream") ||
		strings.Contains(errStr, "stream parsing") {

		if strings.Contains(errStr, "streaming not supported") {
			// 特殊处理：这不是流处理本身的错误，而是环境不支持
			errorCtx.ErrorType = ErrorTypeUnknown
			errorCtx.RetryableAfter = 0 // 不可重试
			slog.Warn(fmt.Sprintf("🌊 [环境不支持] [%s] 端点: %s, 尝试: %d, 错误: %v",
				requestID, endpoint, attempt, err))
		} else {
			errorCtx.ErrorType = ErrorTypeStream
			errorCtx.RetryableAfter = erm.calculateBackoffDelay(attempt)
			slog.Warn(fmt.Sprintf("🌊 [流处理错误分类] [%s] 端点: %s, 尝试: %d, 错误: %v",
				requestID, endpoint, attempt, err))
		}
		return errorCtx
	}

	// HTTP错误分类（非5xx，非429，非400）- 现在在限流和服务器错误检查之后，避免过早捕获特殊错误
	if (strings.Contains(errStr, "http") || strings.Contains(errStr, "status") ||
		strings.Contains(errStr, "endpoint returned error")) &&
		!strings.Contains(errStr, "endpoint returned error: 5") && // 排除5xx
		!strings.Contains(errStr, "429") && !strings.Contains(errStr, "rate") && // 排除429/限流
		!strings.Contains(errStr, "400") && !strings.Contains(errStr, "endpoint returned error: 400") { // 排除400
		errorCtx.ErrorType = ErrorTypeHTTP
		// 非5xx HTTP错误通常不可重试
		slog.Error(fmt.Sprintf("🔗 [HTTP错误分类] [%s] 端点: %s, 尝试: %d, 错误: %v",
			requestID, endpoint, attempt, err))
		return errorCtx
	}

	// 没有健康端点可用错误分类 - 在未知错误之前检查
	if strings.Contains(errStr, "no healthy endpoints available") {
		errorCtx.ErrorType = ErrorTypeNoHealthyEndpoints
		errorCtx.RetryableAfter = 0 // 立即重试，不需要退避
		slog.Warn(fmt.Sprintf("🏥 [健康检查限制] [%s] 端点: %s, 尝试: %d, 建议尝试实际转发, 错误: %v",
			requestID, endpoint, attempt, err))
		return errorCtx
	}

	// 默认为未知错误
	errorCtx.ErrorType = ErrorTypeUnknown
	errorCtx.RetryableAfter = erm.calculateBackoffDelay(attempt)
	slog.Error(fmt.Sprintf("❓ [未知错误分类] [%s] 端点: %s, 尝试: %d, 错误: %v",
		requestID, endpoint, attempt, err))

	return errorCtx
}

// ShouldRetry 判断是否应该重试
func (erm *ErrorRecoveryManager) ShouldRetry(errorCtx *ErrorContext) bool {
	// 超过最大重试次数
	if errorCtx.AttemptCount >= errorCtx.MaxRetries {
		slog.Info(fmt.Sprintf("🛑 [重试判断] [%s] 超过最大重试次数 %d, 不再重试",
			errorCtx.RequestID, errorCtx.MaxRetries))
		return false
	}

	// 根据错误类型判断是否可重试
	switch errorCtx.ErrorType {
	case ErrorTypeClientCancel:
		// 客户端取消错误绝对不可重试
		slog.Info(fmt.Sprintf("🚫 [重试判断] [%s] 客户端取消错误不可重试", errorCtx.RequestID))
		return false

	case ErrorTypeNetwork, ErrorTypeTimeout, ErrorTypeStream, ErrorTypeServerError:
		// 网络、超时、流处理、服务器错误通常可重试
		slog.Info(fmt.Sprintf("✅ [重试判断] [%s] %s错误可重试, 尝试: %d/%d",
			errorCtx.RequestID, erm.getErrorTypeName(errorCtx.ErrorType), errorCtx.AttemptCount, errorCtx.MaxRetries))
		return true

	case ErrorTypeHTTP:
		// 非5xx HTTP错误通常不可重试
		slog.Info(fmt.Sprintf("❌ [重试判断] [%s] 非5xx HTTP错误不可重试", errorCtx.RequestID))
		return false

	case ErrorTypeRateLimit:
		// 限流错误可重试，但需要更长的延迟
		slog.Info(fmt.Sprintf("✅ [重试判断] [%s] 限流错误可重试, 尝试: %d/%d, 建议延迟: %v",
			errorCtx.RequestID, errorCtx.AttemptCount, errorCtx.MaxRetries, errorCtx.RetryableAfter))
		return true

	case ErrorTypeAuth:
		// 认证错误通常不可重试
		slog.Info(fmt.Sprintf("❌ [重试判断] [%s] 认证错误不可重试", errorCtx.RequestID))
		return false

	case ErrorTypeParsing:
		// 解析错误可以尝试重试，可能是临时问题
		slog.Info(fmt.Sprintf("✅ [重试判断] [%s] 解析错误可重试, 尝试: %d/%d",
			errorCtx.RequestID, errorCtx.AttemptCount, errorCtx.MaxRetries))
		return true

	default:
		// 未知错误谨慎重试
		slog.Info(fmt.Sprintf("⚠️ [重试判断] [%s] 未知错误谨慎重试, 尝试: %d/%d",
			errorCtx.RequestID, errorCtx.AttemptCount, errorCtx.MaxRetries))
		return errorCtx.AttemptCount < 2 // 未知错误最多重试2次
	}
}

// ExecuteRetry 执行重试操作
func (erm *ErrorRecoveryManager) ExecuteRetry(ctx context.Context, errorCtx *ErrorContext) error {
	if errorCtx.RetryableAfter > 0 {
		slog.Info(fmt.Sprintf("⏳ [重试延迟] [%s] 等待 %v 后重试",
			errorCtx.RequestID, errorCtx.RetryableAfter))

		select {
		case <-time.After(errorCtx.RetryableAfter):
			// 延迟完成，继续重试
		case <-ctx.Done():
			// 上下文取消，停止重试
			return ctx.Err()
		}
	}

	// 记录重试状态
	if erm.usageTracker != nil && errorCtx.RequestID != "" {
		opts := tracking.UpdateOptions{
			EndpointName: &errorCtx.EndpointName,
			GroupName:    &errorCtx.GroupName,
			Status:       stringPtr("retry"),
			RetryCount:   &errorCtx.AttemptCount,
			HttpStatus:   intPtr(0),
		}
		erm.usageTracker.RecordRequestUpdate(errorCtx.RequestID, opts)
	}

	slog.Info(fmt.Sprintf("🔄 [执行重试] [%s] 第 %d 次重试, 端点: %s",
		errorCtx.RequestID, errorCtx.AttemptCount+1, errorCtx.EndpointName))

	return nil
}

// HandleFinalFailure 处理最终失败情况
func (erm *ErrorRecoveryManager) HandleFinalFailure(errorCtx *ErrorContext) {
	// 记录最终失败状态
	if erm.usageTracker != nil && errorCtx.RequestID != "" {
		status := "error"
		switch errorCtx.ErrorType {
		case ErrorTypeClientCancel:
			status = "cancelled"
		case ErrorTypeTimeout:
			status = "timeout"
		case ErrorTypeAuth:
			status = "auth_error"
		case ErrorTypeRateLimit:
			status = "rate_limited"
		case ErrorTypeServerError:
			status = "server_error"
		}

		opts := tracking.UpdateOptions{
			EndpointName: &errorCtx.EndpointName,
			GroupName:    &errorCtx.GroupName,
			Status:       &status,
			RetryCount:   &errorCtx.AttemptCount,
			HttpStatus:   intPtr(0),
		}
		erm.usageTracker.RecordRequestUpdate(errorCtx.RequestID, opts)
	}

	slog.Error(fmt.Sprintf("💀 [最终失败] [%s] 错误类型: %s, 尝试次数: %d, 端点: %s, 原始错误: %v",
		errorCtx.RequestID, erm.getErrorTypeName(errorCtx.ErrorType),
		errorCtx.AttemptCount, errorCtx.EndpointName, errorCtx.OriginalError))
}

// RecoverFromPartialData 从部分数据中恢复
func (erm *ErrorRecoveryManager) RecoverFromPartialData(requestID string, partialData []byte, processingTime time.Duration) {
	if len(partialData) == 0 {
		slog.Warn(fmt.Sprintf("⚠️ [部分数据恢复] [%s] 无部分数据可恢复", requestID))
		return
	}

	// 尝试从部分数据中提取有用信息
	dataStr := string(partialData)

	// 检查是否包含部分Token信息
	if strings.Contains(dataStr, "usage") || strings.Contains(dataStr, "tokens") {
		slog.Info(fmt.Sprintf("💾 [部分数据恢复] [%s] 从部分数据中发现Token信息, 长度: %d字节",
			requestID, len(partialData)))

		// 可以在这里添加部分Token解析逻辑
		if erm.usageTracker != nil {
			// 记录部分数据恢复状态
			opts := tracking.UpdateOptions{
				Status: stringPtr("partial_recovery"),
			}
			erm.usageTracker.RecordRequestUpdate(requestID, opts)
		}
	} else {
		slog.Info(fmt.Sprintf("📝 [部分数据恢复] [%s] 保存部分响应数据, 长度: %d字节, 处理时间: %v",
			requestID, len(partialData), processingTime))
	}
}

// isNetworkError 判断是否为网络错误（增强版本）
func (erm *ErrorRecoveryManager) isNetworkError(err error) bool {
	if err == nil {
		return false
	}

	// 检查网络操作错误
	var netOpErr *net.OpError
	if errors.As(err, &netOpErr) {
		return true
	}

	// 检查DNS错误
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}

	// 检查系统调用错误
	var syscallErr *syscall.Errno
	if errors.As(err, &syscallErr) {
		switch *syscallErr {
		case syscall.ECONNREFUSED, syscall.ECONNRESET, syscall.ETIMEDOUT,
			syscall.ENETUNREACH, syscall.EHOSTUNREACH:
			return true
		}
	}

	// 字符串匹配（现有逻辑，但排除超时相关错误）
	errStr := strings.ToLower(err.Error())
	networkErrors := []string{
		"connection reset", "connection refused", "connection closed",
		"network is unreachable", "no route to host", "broken pipe",
		"eof", "unexpected eof",
		"upstream connect", "connect error", // 补充常见的upstream错误
		"stream reset", // 补充网络流重置错误
	}

	for _, netErr := range networkErrors {
		if strings.Contains(errStr, netErr) {
			return true
		}
	}

	return false
}

// isTimeoutError 判断是否为超时错误
func (erm *ErrorRecoveryManager) isTimeoutError(err error) bool {
	if err == nil {
		return false
	}

	// 检查context.DeadlineExceeded
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// 检查http.Client超时
	if errors.Is(err, http.ErrHandlerTimeout) {
		return true
	}

	// 检查系统调用超时错误
	var syscallErr *syscall.Errno
	if errors.As(err, &syscallErr) {
		if errors.Is(*syscallErr, syscall.ETIMEDOUT) {
			return true
		}
	}

	// 字符串匹配
	errStr := strings.ToLower(err.Error())
	timeoutErrors := []string{
		"timeout", "deadline exceeded", "context deadline exceeded",
		"i/o timeout", "read timeout", "write timeout", "operation timed out",
	}

	for _, timeoutErr := range timeoutErrors {
		if strings.Contains(errStr, timeoutErr) {
			return true
		}
	}

	return false
}

// isClientCancelError 判断是否为客户端取消错误
func (erm *ErrorRecoveryManager) isClientCancelError(err error) bool {
	if err == nil {
		return false
	}

	// 检查context.Canceled
	if errors.Is(err, context.Canceled) {
		return true
	}

	// 字符串匹配客户端取消相关错误
	errStr := strings.ToLower(err.Error())
	cancelErrors := []string{
		"context canceled", "canceled", "client disconnected",
		"connection closed by client", "client gone away",
	}

	for _, cancelErr := range cancelErrors {
		if strings.Contains(errStr, cancelErr) {
			return true
		}
	}

	return false
}

// calculateBackoffDelay 计算指数退避延迟
func (erm *ErrorRecoveryManager) calculateBackoffDelay(attempt int) time.Duration {
	if attempt <= 0 {
		return erm.baseDelay
	}

	// 指数退避: baseDelay * (backoffFactor ^ attempt)
	delay := time.Duration(float64(erm.baseDelay) *
		func() float64 {
			result := 1.0
			for i := 0; i < attempt; i++ {
				result *= erm.backoffFactor
			}
			return result
		}())

	// 限制最大延迟
	if delay > erm.maxDelay {
		delay = erm.maxDelay
	}

	return delay
}

// String 实现 ErrorType 的字符串方法，用于与重试策略的类型断言兼容
func (et ErrorType) String() string {
	switch et {
	case ErrorTypeNetwork:
		return "网络"
	case ErrorTypeTimeout:
		return "超时"
	case ErrorTypeHTTP:
		return "HTTP"
	case ErrorTypeServerError:
		return "服务器"
	case ErrorTypeStream:
		return "流处理"
	case ErrorTypeAuth:
		return "认证"
	case ErrorTypeRateLimit:
		return "限流"
	case ErrorTypeParsing:
		return "解析"
	case ErrorTypeClientCancel:
		return "客户端取消"
	default:
		return "未知"
	}
}

// getErrorTypeName 获取错误类型名称（保持向后兼容）
func (erm *ErrorRecoveryManager) getErrorTypeName(errorType ErrorType) string {
	return errorType.String()
}

// SetRetryPolicy 设置重试策略
func (erm *ErrorRecoveryManager) SetRetryPolicy(maxRetries int, baseDelay, maxDelay time.Duration, backoffFactor float64) {
	erm.maxRetries = maxRetries
	erm.baseDelay = baseDelay
	erm.maxDelay = maxDelay
	erm.backoffFactor = backoffFactor

	slog.Info("⚙️ [重试策略] 已更新重试策略",
		"max_retries", maxRetries,
		"base_delay", baseDelay,
		"max_delay", maxDelay,
		"backoff_factor", backoffFactor)
}

// 辅助函数：创建string指针
func stringPtr(s string) *string {
	return &s
}

// 辅助函数：创建int指针
func intPtr(i int) *int {
	return &i
}
