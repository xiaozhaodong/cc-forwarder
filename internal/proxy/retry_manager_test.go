package proxy

import (
	"context"
	"fmt"
	"testing"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/proxy/handlers"
	"cc-forwarder/internal/tracking"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestRetryManager 创建测试用的RetryManager
func createTestRetryManager() *RetryManager {
	cfg := &config.Config{
		Retry: config.RetryConfig{
			MaxAttempts: 3,
			BaseDelay:   100 * time.Millisecond,
			MaxDelay:    5 * time.Second,
			Multiplier:  2.0,
		},
		Strategy: config.StrategyConfig{
			Type:            "priority",
			FastTestEnabled: false,
		},
	}

	// 创建UsageTracker
	usageTracker, _ := tracking.NewUsageTracker(nil) // 测试中使用nil配置

	// 创建ErrorRecoveryManager
	errorRecovery := NewErrorRecoveryManager(usageTracker)

	// 创建EndpointManager
	endpointMgr := createTestEndpointManager(cfg)

	return NewRetryManager(cfg, errorRecovery, endpointMgr)
}

// createTestEndpointManager 创建测试用的EndpointManager
func createTestEndpointManager(cfg *config.Config) *endpoint.Manager {
	endpointConfigs := []config.EndpointConfig{
		{
			Name:     "test-endpoint-1",
			URL:      "http://test1.example.com",
			Group:    "test-group",
			Priority: 1,
		},
		{
			Name:     "test-endpoint-2",
			URL:      "http://test2.example.com",
			Group:    "test-group",
			Priority: 2,
		},
	}

	cfg.Endpoints = endpointConfigs
	mgr := endpoint.NewManager(cfg)

	// 手动设置端点为健康状态（用于测试）
	// 注意：这里不直接操作内部状态，而是通过公开方法
	return mgr
}

func TestRetryManager_NewRetryManager(t *testing.T) {
	cfg := &config.Config{}
	errorRecovery := NewErrorRecoveryManager(nil)
	endpointMgr := createTestEndpointManager(cfg)

	rm := NewRetryManager(cfg, errorRecovery, endpointMgr)

	assert.NotNil(t, rm)
	assert.Equal(t, cfg, rm.config)
	assert.Equal(t, errorRecovery, rm.errorRecovery)
	assert.Equal(t, endpointMgr, rm.endpointMgr)
}

func TestRetryManager_ShouldRetry_NetworkError(t *testing.T) {
	rm := createTestRetryManager()

	errorCtx := &handlers.ErrorContext{
		RequestID:     "test-req-001",
		EndpointName:  "test-endpoint",
		GroupName:     "test-group",
		AttemptCount:  1,
		ErrorType:     handlers.ErrorTypeNetwork,
		OriginalError: fmt.Errorf("connection refused"),
	}

	shouldRetry, delay := rm.ShouldRetry(errorCtx, 1)

	assert.True(t, shouldRetry, "网络错误应该可以重试")
	assert.Greater(t, delay, time.Duration(0), "重试延迟应该大于0")
	assert.Equal(t, 100*time.Millisecond, delay, "第一次重试延迟应该等于BaseDelay")
}

func TestRetryManager_ShouldRetry_TimeoutError(t *testing.T) {
	rm := createTestRetryManager()

	errorCtx := &handlers.ErrorContext{
		RequestID:     "test-req-002",
		EndpointName:  "test-endpoint",
		GroupName:     "test-group",
		AttemptCount:  1,
		ErrorType:     handlers.ErrorTypeTimeout,
		OriginalError: fmt.Errorf("context deadline exceeded"),
	}

	shouldRetry, delay := rm.ShouldRetry(errorCtx, 1)

	assert.True(t, shouldRetry, "超时错误应该可以重试")
	assert.Greater(t, delay, time.Duration(0), "重试延迟应该大于0")
}

func TestRetryManager_ShouldRetry_ServerError(t *testing.T) {
	rm := createTestRetryManager()

	errorCtx := &handlers.ErrorContext{
		RequestID:     "test-req-003",
		EndpointName:  "test-endpoint",
		GroupName:     "test-group",
		AttemptCount:  1,
		ErrorType:     handlers.ErrorTypeServerError, // 服务器错误（5xx）应该可重试
		OriginalError: fmt.Errorf("HTTP 502: Bad Gateway"),
	}

	shouldRetry, delay := rm.ShouldRetry(errorCtx, 1)

	assert.True(t, shouldRetry, "服务器错误（5xx）应该可以重试")
	assert.Greater(t, delay, time.Duration(0), "重试延迟应该大于0")
}

func TestRetryManager_ShouldRetry_HTTPError(t *testing.T) {
	rm := createTestRetryManager()

	errorCtx := &handlers.ErrorContext{
		RequestID:     "test-req-004",
		EndpointName:  "test-endpoint",
		GroupName:     "test-group",
		AttemptCount:  1,
		ErrorType:     handlers.ErrorTypeHTTP,
		OriginalError: fmt.Errorf("HTTP 404: Not Found"),
	}

	shouldRetry, delay := rm.ShouldRetry(errorCtx, 1)

	assert.False(t, shouldRetry, "HTTP错误（4xx）不应该重试")
	assert.Equal(t, time.Duration(0), delay, "不重试时延迟应该为0")
}

func TestRetryManager_ShouldRetry_AuthError(t *testing.T) {
	rm := createTestRetryManager()

	errorCtx := &handlers.ErrorContext{
		RequestID:     "test-req-005",
		EndpointName:  "test-endpoint",
		GroupName:     "test-group",
		AttemptCount:  1,
		ErrorType:     handlers.ErrorTypeAuth,
		OriginalError: fmt.Errorf("HTTP 401: Unauthorized"),
	}

	shouldRetry, delay := rm.ShouldRetry(errorCtx, 1)

	assert.False(t, shouldRetry, "认证错误不应该重试")
	assert.Equal(t, time.Duration(0), delay, "不重试时延迟应该为0")
}

func TestRetryManager_ShouldRetry_ClientCancelError(t *testing.T) {
	rm := createTestRetryManager()

	errorCtx := &handlers.ErrorContext{
		RequestID:     "test-req-006",
		EndpointName:  "test-endpoint",
		GroupName:     "test-group",
		AttemptCount:  1,
		ErrorType:     handlers.ErrorTypeClientCancel,
		OriginalError: fmt.Errorf("context canceled"),
	}

	shouldRetry, delay := rm.ShouldRetry(errorCtx, 1)

	assert.False(t, shouldRetry, "客户端取消错误不应该重试")
	assert.Equal(t, time.Duration(0), delay, "不重试时延迟应该为0")
}

func TestRetryManager_ShouldRetry_RateLimitError(t *testing.T) {
	rm := createTestRetryManager()

	errorCtx := &handlers.ErrorContext{
		RequestID:     "test-req-007",
		EndpointName:  "test-endpoint",
		GroupName:     "test-group",
		AttemptCount:  1,
		ErrorType:     handlers.ErrorTypeRateLimit,
		OriginalError: fmt.Errorf("HTTP 429: Too Many Requests"),
	}

	shouldRetry, delay := rm.ShouldRetry(errorCtx, 1)

	assert.True(t, shouldRetry, "限流错误应该可以重试")
	assert.Greater(t, delay, time.Duration(0), "重试延迟应该大于0")
	// 限流错误的延迟应该比普通错误更长
	assert.Greater(t, delay, 100*time.Millisecond, "限流错误延迟应该比BaseDelay更长")
}

func TestRetryManager_ShouldRetry_StreamError(t *testing.T) {
	rm := createTestRetryManager()

	errorCtx := &handlers.ErrorContext{
		RequestID:     "test-req-008",
		EndpointName:  "test-endpoint",
		GroupName:     "test-group",
		AttemptCount:  1,
		ErrorType:     handlers.ErrorTypeStream,
		OriginalError: fmt.Errorf("stream parsing error"),
	}

	shouldRetry, delay := rm.ShouldRetry(errorCtx, 1)

	assert.True(t, shouldRetry, "流处理错误应该可以重试")
	assert.Greater(t, delay, time.Duration(0), "重试延迟应该大于0")
}

func TestRetryManager_ShouldRetry_UnknownError(t *testing.T) {
	rm := createTestRetryManager()

	errorCtx := &handlers.ErrorContext{
		RequestID:     "test-req-009",
		EndpointName:  "test-endpoint",
		GroupName:     "test-group",
		AttemptCount:  1,
		ErrorType:     handlers.ErrorTypeUnknown,
		OriginalError: fmt.Errorf("unknown error"),
	}

	// 第一次尝试应该可以重试
	shouldRetry, delay := rm.ShouldRetry(errorCtx, 1)
	assert.True(t, shouldRetry, "未知错误第一次应该可以重试")
	assert.Greater(t, delay, time.Duration(0), "重试延迟应该大于0")

	// 第二次尝试也应该可以重试
	shouldRetry, delay = rm.ShouldRetry(errorCtx, 2)
	assert.False(t, shouldRetry, "未知错误第二次后不应该重试")
	assert.Equal(t, time.Duration(0), delay, "不重试时延迟应该为0")
}

func TestRetryManager_ShouldRetry_MaxAttemptsExceeded(t *testing.T) {
	rm := createTestRetryManager()

	errorCtx := &handlers.ErrorContext{
		RequestID:     "test-req-010",
		EndpointName:  "test-endpoint",
		GroupName:     "test-group",
		AttemptCount:  3,
		ErrorType:     handlers.ErrorTypeNetwork,
		OriginalError: fmt.Errorf("network error"),
	}

	// 第3次尝试已经达到MaxAttempts，不应该再重试
	shouldRetry, delay := rm.ShouldRetry(errorCtx, 3)

	assert.False(t, shouldRetry, "超过最大重试次数时不应该重试")
	assert.Equal(t, time.Duration(0), delay, "不重试时延迟应该为0")
}

func TestRetryManager_CalculateBackoff(t *testing.T) {
	rm := createTestRetryManager()

	testCases := []struct {
		attempt  int
		expected time.Duration
	}{
		{1, 100 * time.Millisecond}, // BaseDelay
		{2, 200 * time.Millisecond}, // BaseDelay * 2^1
		{3, 400 * time.Millisecond}, // BaseDelay * 2^2
		{4, 800 * time.Millisecond}, // BaseDelay * 2^3
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("attempt_%d", tc.attempt), func(t *testing.T) {
			delay := rm.calculateBackoff(tc.attempt)
			assert.Equal(t, tc.expected, delay)
		})
	}
}

func TestRetryManager_CalculateBackoff_MaxDelay(t *testing.T) {
	// 创建一个MaxDelay较小的配置来测试上限
	cfg := &config.Config{
		Retry: config.RetryConfig{
			MaxAttempts: 10,
			BaseDelay:   1 * time.Second,
			MaxDelay:    3 * time.Second,
			Multiplier:  2.0,
		},
	}

	rm := NewRetryManager(cfg, nil, nil)

	// 第3次尝试: 1s * 2^2 = 4s, 应该被限制为MaxDelay = 3s
	delay := rm.calculateBackoff(3)
	assert.Equal(t, 3*time.Second, delay, "延迟应该被限制为MaxDelay")
}

func TestRetryManager_CalculateRateLimitBackoff(t *testing.T) {
	rm := createTestRetryManager()

	// 限流错误的延迟应该比普通错误更长
	normalDelay := rm.calculateBackoff(1)
	rateLimitDelay := rm.calculateRateLimitBackoff(1)

	assert.Greater(t, rateLimitDelay, normalDelay, "限流错误延迟应该比普通错误更长")

	// 第一次限流错误延迟应该是BaseDelay * 3
	expectedFirstDelay := 100 * time.Millisecond * 3
	assert.Equal(t, expectedFirstDelay, rateLimitDelay)
}

func TestRetryManager_GetHealthyEndpoints_Priority(t *testing.T) {
	rm := createTestRetryManager()
	ctx := context.Background()

	endpoints := rm.GetHealthyEndpoints(ctx)

	// 端点管理器应该返回端点slice（验证方法能够正常调用）
	assert.IsType(t, []*endpoint.Endpoint{}, endpoints, "应该返回正确的slice类型")
	// 注意：由于测试环境中端点未进行健康检查，返回的可能是空slice
}

func TestRetryManager_GetHealthyEndpoints_Fastest(t *testing.T) {
	cfg := &config.Config{
		Retry: config.RetryConfig{
			MaxAttempts: 3,
			BaseDelay:   100 * time.Millisecond,
			MaxDelay:    5 * time.Second,
			Multiplier:  2.0,
		},
		Strategy: config.StrategyConfig{
			Type:            "fastest",
			FastTestEnabled: true,
		},
	}

	endpointMgr := createTestEndpointManager(cfg)
	rm := NewRetryManager(cfg, nil, endpointMgr)
	ctx := context.Background()

	endpoints := rm.GetHealthyEndpoints(ctx)

	// 应该调用GetFastestEndpointsWithRealTimeTest，返回端点slice
	assert.IsType(t, []*endpoint.Endpoint{}, endpoints, "应该返回正确的slice类型")
}

func TestRetryManager_GetMaxAttempts(t *testing.T) {
	rm := createTestRetryManager()

	maxAttempts := rm.GetMaxAttempts()
	assert.Equal(t, 3, maxAttempts, "应该返回配置中的最大重试次数")
}

func TestRetryManager_GetConfig(t *testing.T) {
	rm := createTestRetryManager()

	cfg := rm.GetConfig()
	assert.NotNil(t, cfg, "应该返回配置对象")
	assert.Equal(t, rm.config, cfg, "应该返回正确的配置对象")
}

func TestRetryManager_GetErrorRecoveryManager(t *testing.T) {
	rm := createTestRetryManager()

	errorRecovery := rm.GetErrorRecoveryManager()
	assert.NotNil(t, errorRecovery, "应该返回错误恢复管理器")
	assert.Equal(t, rm.errorRecovery, errorRecovery, "应该返回正确的错误恢复管理器")
}

func TestRetryManager_GetEndpointManager(t *testing.T) {
	rm := createTestRetryManager()

	endpointMgr := rm.GetEndpointManager()
	assert.NotNil(t, endpointMgr, "应该返回端点管理器")
	assert.Equal(t, rm.endpointMgr, endpointMgr, "应该返回正确的端点管理器")
}

// 集成测试：测试完整的重试决策流程
func TestRetryManager_Integration_RetryFlow(t *testing.T) {
	rm := createTestRetryManager()

	// 模拟一个网络错误的重试流程
	errorCtx := &handlers.ErrorContext{
		RequestID:     "test-integration-001",
		EndpointName:  "test-endpoint",
		GroupName:     "test-group",
		AttemptCount:  0,
		ErrorType:     handlers.ErrorTypeNetwork,
		OriginalError: fmt.Errorf("connection reset by peer"),
	}

	// 第一次重试
	shouldRetry, delay := rm.ShouldRetry(errorCtx, 1)
	require.True(t, shouldRetry)
	assert.Equal(t, 100*time.Millisecond, delay)

	// 第二次重试
	shouldRetry, delay = rm.ShouldRetry(errorCtx, 2)
	require.True(t, shouldRetry)
	assert.Equal(t, 200*time.Millisecond, delay)

	// 第三次重试（达到MaxAttempts）
	shouldRetry, delay = rm.ShouldRetry(errorCtx, 3)
	require.False(t, shouldRetry)
	assert.Equal(t, time.Duration(0), delay)
}

// 基准测试：测试重试决策的性能
func BenchmarkRetryManager_ShouldRetry(b *testing.B) {
	rm := createTestRetryManager()

	errorCtx := &handlers.ErrorContext{
		RequestID:     "bench-test",
		EndpointName:  "bench-endpoint",
		GroupName:     "bench-group",
		AttemptCount:  1,
		ErrorType:     handlers.ErrorTypeNetwork,
		OriginalError: fmt.Errorf("network error"),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rm.ShouldRetry(errorCtx, 1)
	}
}

func BenchmarkRetryManager_CalculateBackoff(b *testing.B) {
	rm := createTestRetryManager()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rm.calculateBackoff(i%10 + 1) // 测试1-10次尝试的计算性能
	}
}
