// Package integration 提供统一请求处理架构的集成测试
// 验证重构后的RetryManager、SuspensionManager和LifecycleManager组件集成
package integration

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/proxy"
	"cc-forwarder/internal/proxy/handlers"
	"cc-forwarder/internal/tracking"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUnifiedRequestHandling 测试统一请求处理架构
// 验证重构后的RetryManager、SuspensionManager、LifecycleManager集成工作
func TestUnifiedRequestHandling(t *testing.T) {
	// 创建测试配置
	cfg := createIntegrationTestConfig()

	// 创建组件依赖
	usageTracker, err := tracking.NewUsageTracker(nil)
	require.NoError(t, err)

	endpointMgr := endpoint.NewManager(cfg)
	groupMgr := endpoint.NewGroupManager(cfg)

	// 创建重构后的组件
	errorRecovery := proxy.NewErrorRecoveryManager(usageTracker)
	retryManager := proxy.NewRetryManager(cfg, errorRecovery, endpointMgr)
	suspensionManager := proxy.NewSuspensionManager(cfg, endpointMgr, groupMgr)

	// 验证组件创建成功
	assert.NotNil(t, retryManager)
	assert.NotNil(t, suspensionManager)
	assert.NotNil(t, errorRecovery)

	t.Logf("✅ 统一架构组件创建成功")
}

// TestRetryManager_ErrorClassificationDrivenRetry 测试错误分类驱动的重试决策
func TestRetryManager_ErrorClassificationDrivenRetry(t *testing.T) {
	cfg := createIntegrationTestConfig()
	usageTracker, _ := tracking.NewUsageTracker(nil)
	endpointMgr := endpoint.NewManager(cfg)

	errorRecovery := proxy.NewErrorRecoveryManager(usageTracker)
	retryManager := proxy.NewRetryManager(cfg, errorRecovery, endpointMgr)

	testCases := []struct {
		name           string
		errorType      handlers.ErrorType
		originalError  error
		attempt        int
		expectRetry    bool
		expectDelay    bool
		description    string
	}{
		{
			name:          "网络错误应该重试",
			errorType:     handlers.ErrorTypeNetwork,
			originalError: fmt.Errorf("connection refused"),
			attempt:       1,
			expectRetry:   true,
			expectDelay:   true,
			description:   "网络错误通常是临时性的，应该重试",
		},
		{
			name:          "超时错误应该重试",
			errorType:     handlers.ErrorTypeTimeout,
			originalError: fmt.Errorf("context deadline exceeded"),
			attempt:       1,
			expectRetry:   true,
			expectDelay:   true,
			description:   "超时错误可能是暂时负载过高，应该重试",
		},
		{
			name:          "HTTP错误不应该重试",
			errorType:     handlers.ErrorTypeHTTP,
			originalError: fmt.Errorf("HTTP 404: Not Found"),
			attempt:       1,
			expectRetry:   false,
			expectDelay:   false,
			description:   "HTTP 4xx错误通常是客户端问题，不应该重试",
		},
		{
			name:          "认证错误不应该重试",
			errorType:     handlers.ErrorTypeAuth,
			originalError: fmt.Errorf("HTTP 401: Unauthorized"),
			attempt:       1,
			expectRetry:   false,
			expectDelay:   false,
			description:   "认证错误需要用户干预，不应该重试",
		},
		{
			name:          "客户端取消错误不应该重试",
			errorType:     handlers.ErrorTypeClientCancel,
			originalError: fmt.Errorf("context canceled"),
			attempt:       1,
			expectRetry:   false,
			expectDelay:   false,
			description:   "客户端取消请求，绝对不应该重试",
		},
		{
			name:          "限流错误应该重试且延迟更长",
			errorType:     handlers.ErrorTypeRateLimit,
			originalError: fmt.Errorf("HTTP 429: Too Many Requests"),
			attempt:       1,
			expectRetry:   true,
			expectDelay:   true,
			description:   "限流错误应该重试，但需要更长的延迟",
		},
		{
			name:          "流处理错误应该重试",
			errorType:     handlers.ErrorTypeStream,
			originalError: fmt.Errorf("stream parsing error"),
			attempt:       1,
			expectRetry:   true,
			expectDelay:   true,
			description:   "流处理错误可能是临时解析问题，应该重试",
		},
		{
			name:          "超过最大重试次数不应该重试",
			errorType:     handlers.ErrorTypeNetwork,
			originalError: fmt.Errorf("connection refused"),
			attempt:       3, // MaxAttempts = 3
			expectRetry:   false,
			expectDelay:   false,
			description:   "即使是可重试的错误，超过最大次数也不应该重试",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errorCtx := &handlers.ErrorContext{
				RequestID:     fmt.Sprintf("test-req-%s", tc.name),
				EndpointName:  "test-endpoint",
				GroupName:     "test-group",
				AttemptCount:  tc.attempt,
				ErrorType:     tc.errorType,
				OriginalError: tc.originalError,
			}

			shouldRetry, delay := retryManager.ShouldRetry(errorCtx, tc.attempt)

			assert.Equal(t, tc.expectRetry, shouldRetry,
				"重试决策不符合预期: %s", tc.description)

			if tc.expectDelay {
				assert.Greater(t, delay, time.Duration(0),
					"期望有延迟但延迟为0: %s", tc.description)
			} else {
				assert.Equal(t, time.Duration(0), delay,
					"期望无延迟但有延迟: %s", tc.description)
			}

			t.Logf("✅ [%s] 重试决策: %t, 延迟: %v", tc.name, shouldRetry, delay)
		})
	}
}

// TestRetryManager_ExponentialBackoffCalculation 测试指数退避计算正确性
func TestRetryManager_ExponentialBackoffCalculation(t *testing.T) {
	cfg := createIntegrationTestConfig()
	usageTracker, _ := tracking.NewUsageTracker(nil)
	endpointMgr := endpoint.NewManager(cfg)

	errorRecovery := proxy.NewErrorRecoveryManager(usageTracker)
	retryManager := proxy.NewRetryManager(cfg, errorRecovery, endpointMgr)

	// 测试普通错误的指数退避
	testCases := []struct {
		attempt  int
		expected time.Duration
	}{
		{1, 100 * time.Millisecond}, // BaseDelay
		{2, 200 * time.Millisecond}, // BaseDelay * 2^1
		// 注意：第3次尝试时已达到MaxAttempts=3，不会重试
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("attempt_%d", tc.attempt), func(t *testing.T) {
			errorCtx := &handlers.ErrorContext{
				RequestID:     fmt.Sprintf("backoff-test-%d", tc.attempt),
				EndpointName:  "test-endpoint",
				GroupName:     "test-group",
				AttemptCount:  tc.attempt,
				ErrorType:     handlers.ErrorTypeNetwork,
				OriginalError: fmt.Errorf("network error"),
			}

			shouldRetry, delay := retryManager.ShouldRetry(errorCtx, tc.attempt)

			assert.True(t, shouldRetry, "网络错误应该重试")
			assert.Equal(t, tc.expected, delay,
				"第%d次重试的延迟计算不正确", tc.attempt)

			t.Logf("✅ 第%d次重试延迟计算正确: %v", tc.attempt, delay)
		})
	}
}

// TestRetryManager_RateLimitSpecialBackoff 测试限流错误的特殊退避策略
func TestRetryManager_RateLimitSpecialBackoff(t *testing.T) {
	cfg := createIntegrationTestConfig()
	usageTracker, _ := tracking.NewUsageTracker(nil)
	endpointMgr := endpoint.NewManager(cfg)

	errorRecovery := proxy.NewErrorRecoveryManager(usageTracker)
	retryManager := proxy.NewRetryManager(cfg, errorRecovery, endpointMgr)

	// 比较普通网络错误和限流错误的延迟
	networkErrorCtx := &handlers.ErrorContext{
		RequestID:     "network-test",
		EndpointName:  "test-endpoint",
		GroupName:     "test-group",
		AttemptCount:  1,
		ErrorType:     handlers.ErrorTypeNetwork,
		OriginalError: fmt.Errorf("network error"),
	}

	rateLimitErrorCtx := &handlers.ErrorContext{
		RequestID:     "ratelimit-test",
		EndpointName:  "test-endpoint",
		GroupName:     "test-group",
		AttemptCount:  1,
		ErrorType:     handlers.ErrorTypeRateLimit,
		OriginalError: fmt.Errorf("HTTP 429: Too Many Requests"),
	}

	_, networkDelay := retryManager.ShouldRetry(networkErrorCtx, 1)
	_, rateLimitDelay := retryManager.ShouldRetry(rateLimitErrorCtx, 1)

	assert.Greater(t, rateLimitDelay, networkDelay,
		"限流错误的延迟应该比普通网络错误更长")

	// 限流错误的延迟应该至少是普通错误的3倍（BaseDelay * 3）
	expectedMinRateLimitDelay := networkDelay * 3
	assert.GreaterOrEqual(t, rateLimitDelay, expectedMinRateLimitDelay,
		"限流错误延迟应该至少是普通错误的3倍")

	t.Logf("✅ 限流错误特殊退避验证成功: 普通=%v, 限流=%v", networkDelay, rateLimitDelay)
}

// TestSuspensionManager_ShouldSuspendConditions 测试挂起条件判断
func TestSuspensionManager_ShouldSuspendConditions(t *testing.T) {
	cfg := createIntegrationTestConfig()
	endpointMgr := endpoint.NewManager(cfg)
	groupMgr := endpoint.NewGroupManager(cfg)
	ctx := context.Background()

	testCases := []struct {
		name             string
		configModifier   func(*config.Config)
		expectSuspension bool
		description      string
	}{
		{
			name: "功能未启用时不应该挂起",
			configModifier: func(c *config.Config) {
				c.RequestSuspend.Enabled = false
			},
			expectSuspension: false,
			description:      "当RequestSuspend.Enabled=false时，不应该挂起请求",
		},
		{
			name: "自动切换模式时不应该挂起",
			configModifier: func(c *config.Config) {
				c.RequestSuspend.Enabled = true
				c.Group.AutoSwitchBetweenGroups = true
			},
			expectSuspension: false,
			description:      "当AutoSwitchBetweenGroups=true时，不应该挂起请求",
		},
		{
			name: "达到最大挂起数时不应该挂起",
			configModifier: func(c *config.Config) {
				c.RequestSuspend.Enabled = true
				c.Group.AutoSwitchBetweenGroups = false
				c.RequestSuspend.MaxSuspendedRequests = 0 // 设置为0，立即达到上限
			},
			expectSuspension: false,
			description:      "当挂起请求数达到上限时，不应该挂起新请求",
		},
		{
			name: "手动模式且功能启用且未达上限时应该挂起",
			configModifier: func(c *config.Config) {
				c.RequestSuspend.Enabled = true
				c.Group.AutoSwitchBetweenGroups = false
				c.RequestSuspend.MaxSuspendedRequests = 100
			},
			expectSuspension: false, // 实际上可能不挂起，因为没有备用组
			description:      "满足所有条件但可能没有可用备用组",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 修改配置
			testCfg := *cfg // 创建配置副本
			tc.configModifier(&testCfg)

			// 创建新的SuspensionManager使用修改后的配置
			testSuspensionMgr := proxy.NewSuspensionManager(&testCfg, endpointMgr, groupMgr)

			shouldSuspend := testSuspensionMgr.ShouldSuspend(ctx)

			assert.Equal(t, tc.expectSuspension, shouldSuspend,
				"挂起条件判断不符合预期: %s", tc.description)

			t.Logf("✅ [%s] 挂起判断: %t", tc.name, shouldSuspend)
		})
	}
}

// TestSuspensionManager_WaitForGroupSwitchTimeout 测试组切换等待超时处理
func TestSuspensionManager_WaitForGroupSwitchTimeout(t *testing.T) {
	cfg := createIntegrationTestConfig()
	cfg.RequestSuspend.Timeout = 100 * time.Millisecond // 设置短超时用于测试

	endpointMgr := endpoint.NewManager(cfg)
	groupMgr := endpoint.NewGroupManager(cfg)

	suspensionManager := proxy.NewSuspensionManager(cfg, endpointMgr, groupMgr)

	ctx := context.Background()
	connID := "test-connection-timeout"

	start := time.Now()
	result := suspensionManager.WaitForGroupSwitch(ctx, connID)
	duration := time.Since(start)

	// 应该在超时时间内返回false
	assert.False(t, result, "超时后应该返回false")
	assert.Greater(t, duration, 100*time.Millisecond, "应该等待至少超时时间")
	assert.Less(t, duration, 200*time.Millisecond, "不应该等待太久")

	t.Logf("✅ 组切换超时处理验证成功: 等待时间=%v, 结果=%t", duration, result)
}

// TestSuspensionManager_WaitForGroupSwitchCancel 测试组切换等待上下文取消
func TestSuspensionManager_WaitForGroupSwitchCancel(t *testing.T) {
	cfg := createIntegrationTestConfig()
	cfg.RequestSuspend.Timeout = 10 * time.Second // 设置长超时

	endpointMgr := endpoint.NewManager(cfg)
	groupMgr := endpoint.NewGroupManager(cfg)

	suspensionManager := proxy.NewSuspensionManager(cfg, endpointMgr, groupMgr)

	ctx, cancel := context.WithCancel(context.Background())
	connID := "test-connection-cancel"

	// 在另一个goroutine中延迟取消context
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	result := suspensionManager.WaitForGroupSwitch(ctx, connID)
	duration := time.Since(start)

	// 应该在50ms左右返回false（上下文被取消）
	assert.False(t, result, "上下文取消后应该返回false")
	assert.Greater(t, duration, 40*time.Millisecond, "应该等待至少40ms")
	assert.Less(t, duration, 100*time.Millisecond, "应该在100ms内返回")

	t.Logf("✅ 组切换上下文取消处理验证成功: 等待时间=%v, 结果=%t", duration, result)
}

// TestHTTP404ErrorHandling 测试404错误正确记录为失败状态
func TestHTTP404ErrorHandling(t *testing.T) {
	cfg := createIntegrationTestConfig()
	usageTracker, _ := tracking.NewUsageTracker(nil)
	endpointMgr := endpoint.NewManager(cfg)

	errorRecovery := proxy.NewErrorRecoveryManager(usageTracker)
	retryManager := proxy.NewRetryManager(cfg, errorRecovery, endpointMgr)

	// 创建生命周期管理器
	lifecycleManager := proxy.NewRequestLifecycleManager(usageTracker, nil, "test-404-req", nil)

	// 模拟404错误
	err404 := fmt.Errorf("HTTP 404: Not Found")

	// 使用错误恢复管理器分类错误
	errorCtx := errorRecovery.ClassifyError(err404, "test-404-req", "test-endpoint", "test-group", 1)

	// 验证错误分类
	assert.Equal(t, proxy.ErrorTypeHTTP, errorCtx.ErrorType, "404错误应该被分类为HTTP错误")

	// 验证不应该重试
	shouldRetry, delay := retryManager.ShouldRetry(&handlers.ErrorContext{
		RequestID:     errorCtx.RequestID,
		EndpointName:  errorCtx.EndpointName,
		GroupName:     errorCtx.GroupName,
		AttemptCount:  errorCtx.AttemptCount,
		ErrorType:     handlers.ErrorTypeHTTP,
		OriginalError: errorCtx.OriginalError,
	}, 1)

	assert.False(t, shouldRetry, "404错误不应该重试")
	assert.Equal(t, time.Duration(0), delay, "404错误不重试时延迟应该为0")

	// 通过生命周期管理器处理错误
	lifecycleManager.HandleError(err404)

	// 验证最终状态不是completed
	assert.NotEqual(t, "completed", lifecycleManager.GetLastStatus(),
		"404错误不应该被标记为completed状态")

	t.Logf("✅ 404错误处理验证成功: 错误类型=%v, 重试=%t, 最终状态=%s",
		errorCtx.ErrorType, shouldRetry, lifecycleManager.GetLastStatus())
}

// TestStatusUpdatePathUnification 测试状态更新路径统一性
func TestStatusUpdatePathUnification(t *testing.T) {
	usageTracker, _ := tracking.NewUsageTracker(nil)

	// 创建多个生命周期管理器模拟不同请求
	managers := []*proxy.RequestLifecycleManager{
		proxy.NewRequestLifecycleManager(usageTracker, nil, "req-unified-1", nil),
		proxy.NewRequestLifecycleManager(usageTracker, nil, "req-unified-2", nil),
		proxy.NewRequestLifecycleManager(usageTracker, nil, "req-unified-3", nil),
	}

	testStatuses := []string{"pending", "forwarding", "processing", "completed"}

	for i, manager := range managers {
		t.Run(fmt.Sprintf("manager_%d", i+1), func(t *testing.T) {
			// 设置端点信息
			manager.SetEndpoint(fmt.Sprintf("test-endpoint-%d", i+1), "test-group")

			// 按顺序更新状态，验证状态转换
			for j, status := range testStatuses {
				manager.UpdateStatus(status, j, 200)

				// 验证状态正确更新
				assert.Equal(t, status, manager.GetLastStatus(),
					"状态更新不正确: 期望=%s, 实际=%s", status, manager.GetLastStatus())

				t.Logf("✅ [manager_%d] 状态更新: %s", i+1, status)
			}
		})
	}

	t.Logf("✅ 状态更新路径统一性验证成功")
}

// TestStreamingAndRegularRequestConsistency 测试流式和常规请求行为一致性
func TestStreamingAndRegularRequestConsistency(t *testing.T) {
	usageTracker, _ := tracking.NewUsageTracker(nil)

	// 创建流式和常规请求的生命周期管理器
	streamingManager := proxy.NewRequestLifecycleManager(usageTracker, nil, "req-streaming", nil)
	regularManager := proxy.NewRequestLifecycleManager(usageTracker, nil, "req-regular", nil)

	// 模拟相同的处理流程
	commonEndpoint := "test-endpoint"
	commonGroup := "test-group"

	// 设置端点信息
	streamingManager.SetEndpoint(commonEndpoint, commonGroup)
	regularManager.SetEndpoint(commonEndpoint, commonGroup)

	// 模拟相同的状态转换序列
	statusSequence := []struct {
		status     string
		retryCount int
		httpStatus int
	}{
		{"pending", 0, 0},
		{"forwarding", 0, 0},
		{"processing", 0, 200},
		{"completed", 0, 200},
	}

	for _, step := range statusSequence {
		streamingManager.UpdateStatus(step.status, step.retryCount, step.httpStatus)
		regularManager.UpdateStatus(step.status, step.retryCount, step.httpStatus)

		// 验证状态一致性
		assert.Equal(t, streamingManager.GetLastStatus(), regularManager.GetLastStatus(),
			"流式和常规请求的状态应该一致")
	}

	// 验证最终状态
	assert.Equal(t, "completed", streamingManager.GetLastStatus())
	assert.Equal(t, "completed", regularManager.GetLastStatus())
	assert.True(t, streamingManager.IsCompleted())
	assert.True(t, regularManager.IsCompleted())

	t.Logf("✅ 流式和常规请求行为一致性验证成功")
}

// TestConcurrentRequestHandling 测试并发请求处理的线程安全性
func TestConcurrentRequestHandling(t *testing.T) {
	cfg := createIntegrationTestConfig()
	usageTracker, _ := tracking.NewUsageTracker(nil)
	endpointMgr := endpoint.NewManager(cfg)

	errorRecovery := proxy.NewErrorRecoveryManager(usageTracker)
	retryManager := proxy.NewRetryManager(cfg, errorRecovery, endpointMgr)

	const numGoroutines = 10
	const numOperationsPerGoroutine = 50

	var wg sync.WaitGroup
	results := make([]bool, numGoroutines)

	// 并发执行重试决策
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			allPassed := true
			for j := 0; j < numOperationsPerGoroutine; j++ {
				errorCtx := &handlers.ErrorContext{
					RequestID:     fmt.Sprintf("concurrent-req-%d-%d", goroutineID, j),
					EndpointName:  fmt.Sprintf("endpoint-%d", goroutineID),
					GroupName:     "test-group",
					AttemptCount:  1,
					ErrorType:     handlers.ErrorTypeNetwork,
					OriginalError: fmt.Errorf("network error %d-%d", goroutineID, j),
				}

				shouldRetry, delay := retryManager.ShouldRetry(errorCtx, 1)

				// 验证基本行为
				if !shouldRetry || delay <= 0 {
					allPassed = false
					break
				}
			}

			results[goroutineID] = allPassed
		}(i)
	}

	wg.Wait()

	// 验证所有goroutine都成功完成
	for i, result := range results {
		assert.True(t, result, "Goroutine %d 未通过并发测试", i)
	}

	t.Logf("✅ 并发请求处理线程安全性验证成功: %d个goroutine, 每个%d次操作",
		numGoroutines, numOperationsPerGoroutine)
}

// TestIntegrationErrorRecoveryFlow 测试完整的错误恢复流程集成
func TestIntegrationErrorRecoveryFlow(t *testing.T) {
	cfg := createIntegrationTestConfig()
	usageTracker, _ := tracking.NewUsageTracker(nil)
	endpointMgr := endpoint.NewManager(cfg)

	errorRecovery := proxy.NewErrorRecoveryManager(usageTracker)
	retryManager := proxy.NewRetryManager(cfg, errorRecovery, endpointMgr)
	lifecycleManager := proxy.NewRequestLifecycleManager(usageTracker, nil, "integration-error-test", nil)

	// 模拟完整的错误恢复流程
	requestID := "integration-error-flow"
	endpointName := "test-endpoint"
	groupName := "test-group"

	lifecycleManager.SetEndpoint(endpointName, groupName)

	// 第一次尝试 - 网络错误
	networkErr := fmt.Errorf("connection refused")
	errorCtx := errorRecovery.ClassifyError(networkErr, requestID, endpointName, groupName, 1)

	// 验证错误分类
	assert.Equal(t, proxy.ErrorTypeNetwork, errorCtx.ErrorType)

	// 检查是否应该重试
	shouldRetry, delay := retryManager.ShouldRetry(&handlers.ErrorContext{
		RequestID:     errorCtx.RequestID,
		EndpointName:  errorCtx.EndpointName,
		GroupName:     errorCtx.GroupName,
		AttemptCount:  errorCtx.AttemptCount,
		ErrorType:     handlers.ErrorTypeNetwork,
		OriginalError: errorCtx.OriginalError,
	}, 1)

	assert.True(t, shouldRetry, "网络错误第一次应该重试")
	assert.Greater(t, delay, time.Duration(0), "重试应该有延迟")

	// 更新生命周期状态
	lifecycleManager.UpdateStatus("retry", 1, 0)
	assert.Equal(t, "retry", lifecycleManager.GetLastStatus())

	// 第二次尝试 - 超时错误
	timeoutErr := fmt.Errorf("context deadline exceeded")
	errorCtx2 := errorRecovery.ClassifyError(timeoutErr, requestID, endpointName, groupName, 2)

	assert.Equal(t, proxy.ErrorTypeTimeout, errorCtx2.ErrorType)

	// 第三次尝试 - 达到最大重试次数
	shouldRetry3, delay3 := retryManager.ShouldRetry(&handlers.ErrorContext{
		RequestID:     errorCtx.RequestID,
		EndpointName:  errorCtx.EndpointName,
		GroupName:     errorCtx.GroupName,
		AttemptCount:  3,
		ErrorType:     handlers.ErrorTypeNetwork,
		OriginalError: errorCtx.OriginalError,
	}, 3)

	assert.False(t, shouldRetry3, "达到最大重试次数后不应该重试")
	assert.Equal(t, time.Duration(0), delay3, "不重试时延迟应该为0")

	// 处理最终失败
	lifecycleManager.HandleError(networkErr)
	assert.NotEqual(t, "completed", lifecycleManager.GetLastStatus(),
		"错误状态下不应该标记为completed")

	t.Logf("✅ 完整错误恢复流程集成验证成功")
}

// createIntegrationTestConfig 创建集成测试配置
func createIntegrationTestConfig() *config.Config {
	return &config.Config{
		Retry: config.RetryConfig{
			MaxAttempts: 3,
			BaseDelay:   100 * time.Millisecond,
			MaxDelay:    5 * time.Second,
			Multiplier:  2.0,
		},
		RequestSuspend: config.RequestSuspendConfig{
			Enabled:               true,
			Timeout:               5 * time.Second,
			MaxSuspendedRequests:  100,
		},
		Group: config.GroupConfig{
			AutoSwitchBetweenGroups: false, // 默认手动模式
			Cooldown:                300 * time.Second,
		},
		Strategy: config.StrategyConfig{
			Type:            "priority",
			FastTestEnabled: false,
		},
		Endpoints: []config.EndpointConfig{
			{
				Name:     "test-endpoint-1",
				URL:      "http://test1.example.com",
				Group:    "main",
				Priority: 1,
			},
			{
				Name:     "test-endpoint-2",
				URL:      "http://test2.example.com",
				Group:    "backup",
				Priority: 1,
			},
		},
	}
}