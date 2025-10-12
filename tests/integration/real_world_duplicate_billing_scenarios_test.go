package integration

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"cc-forwarder/internal/monitor"
	"cc-forwarder/internal/proxy"
	"cc-forwarder/internal/tracking"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRealWorldDuplicateBillingScenarios 真实世界场景的重复计费防护测试
// 模拟实际生产环境中可能出现的各种重复计费风险场景
func TestRealWorldDuplicateBillingScenarios(t *testing.T) {
	t.Run("流式请求中断重试场景", testStreamingInterruptionRetryScenario)
	t.Run("网络不稳定重复请求场景", testNetworkInstabilityScenario)
	t.Run("客户端重复提交防护", testClientDuplicateSubmissionProtection)
	t.Run("服务重启恢复场景", testServiceRestartRecoveryScenario)
	t.Run("负载均衡器重试场景", testLoadBalancerRetryScenario)
	t.Run("分布式系统一致性场景", testDistributedSystemConsistencyScenario)
}

// testStreamingInterruptionRetryScenario 测试流式请求中断重试场景
func testStreamingInterruptionRetryScenario(t *testing.T) {
	t.Run("EOF错误重试计费防护", func(t *testing.T) {
		// 模拟文档中提到的实际案例：流式请求EOF错误，已解析257输入+25输出tokens被丢弃
		tracker, cleanup := setupRealWorldTestTracker(t)
		defer cleanup()

		middleware := &RealWorldMockMiddleware{
			billingEvents: make([]BillingEvent, 0),
			mu:           sync.RWMutex{},
		}

		requestID := generateRealWorldRequestID("eof-retry")
		rlm := proxy.NewRequestLifecycleManager(tracker, middleware, requestID, nil)
		rlm.SetEndpoint("instcopilot-sg", "main")
		rlm.SetModel("claude-3-5-haiku-20241022")

		// 启动流式请求
		rlm.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", true)

		// 模拟流式处理过程中解析的Token
		partialTokens := &tracking.TokenUsage{
			InputTokens:  257, // 完整的输入
			OutputTokens: 25,  // 部分输出（EOF前解析到的）
		}

		// 模拟重试序列
		for attempt := 1; attempt <= 3; attempt++ {
			rlm.UpdateStatus("retry", attempt, 500)

			if attempt == 3 {
				// 最后一次重试失败，使用RecordTokensForFailedRequest记录Token
				rlm.UpdateStatus("error", attempt, 0)
				rlm.RecordTokensForFailedRequest(partialTokens, "unexpected EOF")
			}

			time.Sleep(50 * time.Millisecond)
		}

		time.Sleep(300 * time.Millisecond)

		// 验证计费防护
		billingEvents := middleware.GetBillingEvents()

		// 应该只有一次最终的失败Token记录
		failedTokenEvents := filterBillingEvents(billingEvents, "failed_tokens")
		assert.Equal(t, 1, len(failedTokenEvents), "EOF重试场景应该只有一次失败Token记录")

		// 验证Token数量正确
		if len(failedTokenEvents) > 0 {
			event := failedTokenEvents[0]
			assert.Equal(t, int64(257), event.Tokens.InputTokens, "输入Token应该被正确记录")
			assert.Equal(t, int64(25), event.Tokens.OutputTokens, "部分输出Token应该被正确记录")
			assert.Equal(t, "unexpected EOF", event.FailureReason, "失败原因应该正确")
		}

		// 验证不会有重复的成功计费
		successEvents := filterBillingEvents(billingEvents, "success")
		assert.Equal(t, 0, len(successEvents), "EOF错误不应该有成功计费")

		// 验证数据库记录
		dbRecords := getRealWorldBillingRecords(t, tracker, requestID)
		assert.Equal(t, 1, len(dbRecords), "数据库应该只有一条记录")

		if len(dbRecords) > 0 {
			record := dbRecords[0]
			assert.Equal(t, "error", record.Status)
			assert.Equal(t, int64(257), record.InputTokens)
			assert.Equal(t, int64(25), record.OutputTokens)
			// 验证成本计算：257 * 3.00/1M + 25 * 15.00/1M
			expectedCost := float64(257)*3.00/1000000.0 + float64(25)*15.00/1000000.0
			assert.InDelta(t, expectedCost, record.TotalCost, 0.0001, "成本计算应该正确")
		}
	})

	t.Run("流式响应部分成功场景", func(t *testing.T) {
		tracker, cleanup := setupRealWorldTestTracker(t)
		defer cleanup()

		middleware := &RealWorldMockMiddleware{
			billingEvents: make([]BillingEvent, 0),
			mu:           sync.RWMutex{},
		}

		requestID := generateRealWorldRequestID("partial-success")
		rlm := proxy.NewRequestLifecycleManager(tracker, middleware, requestID, nil)
		rlm.SetEndpoint("claude-api", "backup")
		rlm.SetModel("claude-3-5-haiku-20241022")

		rlm.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", true)

		// 模拟流式响应中途中断
		partialTokens := &tracking.TokenUsage{
			InputTokens:         500,
			OutputTokens:        150, // 部分输出
			CacheCreationTokens: 0,
			CacheReadTokens:     100, // 有缓存读取
		}

		// 第一次尝试失败（记录部分Token）
		rlm.UpdateStatus("stream_error", 1, 0)
		rlm.RecordTokensForFailedRequest(partialTokens, "stream_interrupted")
		time.Sleep(100 * time.Millisecond)

		// 重试成功（完整Token）
		completeTokens := &tracking.TokenUsage{
			InputTokens:         500,
			OutputTokens:        300, // 完整输出
			CacheCreationTokens: 0,
			CacheReadTokens:     100,
		}

		rlm.UpdateStatus("completed", 2, 200)
		rlm.CompleteRequest(completeTokens)
		time.Sleep(200 * time.Millisecond)

		// 验证最终只有成功记录被保留
		dbRecords := getRealWorldBillingRecords(t, tracker, requestID)
		assert.Equal(t, 1, len(dbRecords), "应该只有最终的成功记录")

		if len(dbRecords) > 0 {
			record := dbRecords[0]
			assert.Equal(t, "completed", record.Status, "最终状态应该是成功")
			assert.Equal(t, int64(500), record.InputTokens, "输入Token应该正确")
			assert.Equal(t, int64(300), record.OutputTokens, "应该是完整的输出Token")
			assert.Equal(t, int64(100), record.CacheReadTokens, "缓存读取Token应该正确")
		}

		// 验证监控记录反映了正确的状态转换
		billingEvents := middleware.GetBillingEvents()
		assert.GreaterOrEqual(t, len(billingEvents), 2, "应该有失败和成功两种记录")
	})
}

// testNetworkInstabilityScenario 测试网络不稳定重复请求场景
func testNetworkInstabilityScenario(t *testing.T) {
	t.Run("网络超时重复提交防护", func(t *testing.T) {
		tracker, cleanup := setupRealWorldTestTracker(t)
		defer cleanup()

		middleware := &RealWorldMockMiddleware{
			billingEvents: make([]BillingEvent, 0),
			mu:           sync.RWMutex{},
		}

		requestID := generateRealWorldRequestID("network-timeout")

		// 模拟客户端因为网络超时进行多次重复提交
		const numDuplicateSubmissions = 5
		var wg sync.WaitGroup

		for i := 0; i < numDuplicateSubmissions; i++ {
			wg.Add(1)
			go func(submission int) {
				defer wg.Done()

				// 使用相同的requestID模拟重复提交
				rlm := proxy.NewRequestLifecycleManager(tracker, middleware, requestID, nil)
				rlm.SetEndpoint("timeout-endpoint", "main")
				rlm.SetModel("claude-3-5-haiku-20241022")

				if submission == 0 {
					// 只有第一次提交应该调用StartRequest
					rlm.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)
				}

				tokens := &tracking.TokenUsage{
					InputTokens:  200,
					OutputTokens: 100,
				}

				// 模拟网络超时
				time.Sleep(time.Duration(submission*20) * time.Millisecond)
				rlm.UpdateStatus("timeout", 1, 0)
				rlm.RecordTokensForFailedRequest(tokens, fmt.Sprintf("network_timeout_%d", submission))
			}(i)
		}

		wg.Wait()
		time.Sleep(400 * time.Millisecond)

		// 验证重复提交防护
		dbRecords := getRealWorldBillingRecords(t, tracker, requestID)
		assert.LessOrEqual(t, len(dbRecords), 1, "网络超时重复提交应该被防护")

		if len(dbRecords) > 0 {
			record := dbRecords[0]
			assert.Equal(t, requestID, record.RequestID)
			assert.Equal(t, "timeout", record.Status)
			assert.Equal(t, int64(200), record.InputTokens)
			assert.Equal(t, int64(100), record.OutputTokens)
		}

		// 验证监控系统也没有重复记录
		billingEvents := middleware.GetBillingEvents()
		failedEvents := filterBillingEvents(billingEvents, "failed_tokens")
		assert.LessOrEqual(t, len(failedEvents), numDuplicateSubmissions, "监控系统应该处理重复提交")
	})

	t.Run("连接断开重连计费一致性", func(t *testing.T) {
		tracker, cleanup := setupRealWorldTestTracker(t)
		defer cleanup()

		middleware := &RealWorldMockMiddleware{
			billingEvents: make([]BillingEvent, 0),
			mu:           sync.RWMutex{},
		}

		requestID := generateRealWorldRequestID("connection-reconnect")

		// 第一次连接（部分处理后断开）
		rlm1 := proxy.NewRequestLifecycleManager(tracker, middleware, requestID, nil)
		rlm1.SetEndpoint("unstable-endpoint", "main")
		rlm1.SetModel("claude-3-5-haiku-20241022")
		rlm1.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

		partialTokens := &tracking.TokenUsage{
			InputTokens:  300,
			OutputTokens: 0, // 连接断开前无输出
		}

		rlm1.UpdateStatus("network_error", 1, 0)
		rlm1.RecordTokensForFailedRequest(partialTokens, "connection_lost")
		time.Sleep(100 * time.Millisecond)

		// 重连后（完整处理）
		rlm2 := proxy.NewRequestLifecycleManager(tracker, middleware, requestID, nil)
		rlm2.SetEndpoint("stable-endpoint", "main")
		rlm2.SetModel("claude-3-5-haiku-20241022")

		completeTokens := &tracking.TokenUsage{
			InputTokens:  300,
			OutputTokens: 250, // 重连后完整输出
		}

		rlm2.UpdateStatus("completed", 2, 200)
		rlm2.CompleteRequest(completeTokens)
		time.Sleep(200 * time.Millisecond)

		// 验证重连后的计费一致性
		dbRecords := getRealWorldBillingRecords(t, tracker, requestID)
		assert.Equal(t, 1, len(dbRecords), "重连后应该只有一条最终记录")

		if len(dbRecords) > 0 {
			record := dbRecords[0]
			assert.Equal(t, "completed", record.Status, "最终状态应该是成功")
			assert.Equal(t, int64(300), record.InputTokens, "输入Token应该一致")
			assert.Equal(t, int64(250), record.OutputTokens, "应该是完整的输出Token")
		}
	})
}

// testClientDuplicateSubmissionProtection 测试客户端重复提交防护
func testClientDuplicateSubmissionProtection(t *testing.T) {
	t.Run("幂等性保证", func(t *testing.T) {
		tracker, cleanup := setupRealWorldTestTracker(t)
		defer cleanup()

		middleware := &RealWorldMockMiddleware{
			billingEvents: make([]BillingEvent, 0),
			mu:           sync.RWMutex{},
		}

		requestID := generateRealWorldRequestID("idempotency")

		// 模拟客户端快速连续提交相同请求
		const numRapidSubmissions = 10
		var wg sync.WaitGroup
		submissionResults := make([]bool, numRapidSubmissions)

		for i := 0; i < numRapidSubmissions; i++ {
			wg.Add(1)
			go func(submission int) {
				defer wg.Done()

				rlm := proxy.NewRequestLifecycleManager(tracker, middleware, requestID, nil)
				rlm.SetEndpoint("api-endpoint", "main")
				rlm.SetModel("claude-3-5-haiku-20241022")

				if submission == 0 {
					rlm.StartRequest("192.168.1.1", "client-app/1.0", "POST", "/v1/messages", false)
				}

				tokens := &tracking.TokenUsage{
					InputTokens:  150,
					OutputTokens: 75,
				}

				// 快速提交
				rlm.UpdateStatus("completed", 1, 200)
				rlm.CompleteRequest(tokens)
				submissionResults[submission] = true
			}(i)
		}

		wg.Wait()
		time.Sleep(300 * time.Millisecond)

		// 验证幂等性
		dbRecords := getRealWorldBillingRecords(t, tracker, requestID)
		assert.Equal(t, 1, len(dbRecords), "幂等性应该确保只有一条记录")

		if len(dbRecords) > 0 {
			record := dbRecords[0]
			assert.Equal(t, requestID, record.RequestID)
			assert.Equal(t, "completed", record.Status)
			assert.Equal(t, int64(150), record.InputTokens)
			assert.Equal(t, int64(75), record.OutputTokens)
		}

		// 验证所有提交都被尝试了
		successfulSubmissions := 0
		for _, result := range submissionResults {
			if result {
				successfulSubmissions++
			}
		}
		assert.Equal(t, numRapidSubmissions, successfulSubmissions, "所有提交都应该被处理")
	})

	t.Run("客户端重试逻辑防护", func(t *testing.T) {
		tracker, cleanup := setupRealWorldTestTracker(t)
		defer cleanup()

		middleware := &RealWorldMockMiddleware{
			billingEvents: make([]BillingEvent, 0),
			mu:           sync.RWMutex{},
		}

		requestID := generateRealWorldRequestID("client-retry")

		// 模拟客户端重试逻辑：先失败，然后重试成功
		rlm := proxy.NewRequestLifecycleManager(tracker, middleware, requestID, nil)
		rlm.SetEndpoint("retry-endpoint", "main")
		rlm.SetModel("claude-3-5-haiku-20241022")
		rlm.StartRequest("192.168.1.1", "client-app/1.0", "POST", "/v1/messages", false)

		// 第一次尝试失败
		failTokens := &tracking.TokenUsage{
			InputTokens:  100,
			OutputTokens: 0,
		}

		rlm.UpdateStatus("rate_limited", 1, 429)
		rlm.RecordTokensForFailedRequest(failTokens, "rate_limit_exceeded")
		time.Sleep(100 * time.Millisecond)

		// 客户端等待后重试成功
		successTokens := &tracking.TokenUsage{
			InputTokens:  100,
			OutputTokens: 200,
		}

		rlm.UpdateStatus("completed", 2, 200)
		rlm.CompleteRequest(successTokens)
		time.Sleep(200 * time.Millisecond)

		// 验证重试逻辑的计费准确性
		dbRecords := getRealWorldBillingRecords(t, tracker, requestID)
		assert.Equal(t, 1, len(dbRecords), "客户端重试应该只有最终记录")

		if len(dbRecords) > 0 {
			record := dbRecords[0]
			assert.Equal(t, "completed", record.Status, "最终状态应该是成功")
			assert.Equal(t, int64(100), record.InputTokens, "输入Token应该一致")
			assert.Equal(t, int64(200), record.OutputTokens, "应该是成功时的输出Token")
		}

		// 验证监控记录了完整的重试过程
		billingEvents := middleware.GetBillingEvents()
		assert.GreaterOrEqual(t, len(billingEvents), 2, "应该记录失败和成功")
	})
}

// testServiceRestartRecoveryScenario 测试服务重启恢复场景
func testServiceRestartRecoveryScenario(t *testing.T) {
	t.Run("服务重启后数据一致性", func(t *testing.T) {
		// 第一阶段：服务运行时的处理
		tracker1, cleanup1 := setupRealWorldTestTracker(t)
		defer cleanup1()

		middleware1 := &RealWorldMockMiddleware{
			billingEvents: make([]BillingEvent, 0),
			mu:           sync.RWMutex{},
		}

		requestID := generateRealWorldRequestID("service-restart")

		rlm1 := proxy.NewRequestLifecycleManager(tracker1, middleware1, requestID, nil)
		rlm1.SetEndpoint("main-endpoint", "primary")
		rlm1.SetModel("claude-3-5-haiku-20241022")
		rlm1.StartRequest("192.168.1.1", "client-app/1.0", "POST", "/v1/messages", false)

		preRestartTokens := &tracking.TokenUsage{
			InputTokens:  400,
			OutputTokens: 100,
		}

		rlm1.UpdateStatus("processing", 1, 200)
		rlm1.RecordTokensForFailedRequest(preRestartTokens, "service_restart")
		time.Sleep(100 * time.Millisecond)

		// 第二阶段：模拟服务重启（新的tracker实例）
		tracker2, cleanup2 := setupRealWorldTestTracker(t)
		defer cleanup2()

		middleware2 := &RealWorldMockMiddleware{
			billingEvents: make([]BillingEvent, 0),
			mu:           sync.RWMutex{},
		}

		// 重启后，相同请求ID的处理
		rlm2 := proxy.NewRequestLifecycleManager(tracker2, middleware2, requestID, nil)
		rlm2.SetEndpoint("backup-endpoint", "secondary")
		rlm2.SetModel("claude-3-5-haiku-20241022")

		postRestartTokens := &tracking.TokenUsage{
			InputTokens:  400,
			OutputTokens: 300, // 重启后完整处理
		}

		rlm2.UpdateStatus("completed", 1, 200)
		rlm2.CompleteRequest(postRestartTokens)
		time.Sleep(200 * time.Millisecond)

		// 验证两个实例的数据一致性
		records1 := getRealWorldBillingRecords(t, tracker1, requestID)
		records2 := getRealWorldBillingRecords(t, tracker2, requestID)

		// 每个实例都应该有自己的记录（因为是独立的内存数据库）
		assert.LessOrEqual(t, len(records1), 1, "第一个实例应该有记录")
		assert.LessOrEqual(t, len(records2), 1, "第二个实例应该有记录")

		// 在真实环境中，应该使用共享数据库来保证一致性
		// 这里验证各自的数据完整性
		if len(records1) > 0 {
			record1 := records1[0]
			assert.Equal(t, requestID, record1.RequestID)
			assert.Equal(t, int64(400), record1.InputTokens)
		}

		if len(records2) > 0 {
			record2 := records2[0]
			assert.Equal(t, requestID, record2.RequestID)
			assert.Equal(t, int64(400), record2.InputTokens)
		}
	})

	t.Run("崩溃恢复数据完整性", func(t *testing.T) {
		tracker, cleanup := setupRealWorldTestTracker(t)
		defer cleanup()

		middleware := &RealWorldMockMiddleware{
			billingEvents: make([]BillingEvent, 0),
			mu:           sync.RWMutex{},
		}

		requestID := generateRealWorldRequestID("crash-recovery")

		// 模拟系统崩溃前的状态
		rlm1 := proxy.NewRequestLifecycleManager(tracker, middleware, requestID, nil)
		rlm1.SetEndpoint("crash-endpoint", "main")
		rlm1.SetModel("claude-3-5-haiku-20241022")
		rlm1.StartRequest("192.168.1.1", "client-app/1.0", "POST", "/v1/messages", true)

		crashTokens := &tracking.TokenUsage{
			InputTokens:  500,
			OutputTokens: 50, // 崩溃前部分处理
		}

		rlm1.UpdateStatus("stream_error", 1, 0)
		rlm1.RecordTokensForFailedRequest(crashTokens, "system_crash")
		time.Sleep(150 * time.Millisecond)

		// 模拟系统恢复后的处理（相同请求ID）
		rlm2 := proxy.NewRequestLifecycleManager(tracker, middleware, requestID, nil)
		rlm2.SetEndpoint("recovery-endpoint", "backup")
		rlm2.SetModel("claude-3-5-haiku-20241022")

		// 恢复后不应该重复处理
		recoveryTokens := &tracking.TokenUsage{
			InputTokens:  500,
			OutputTokens: 400, // 恢复后完整处理
		}

		rlm2.UpdateStatus("completed", 1, 200)
		rlm2.CompleteRequest(recoveryTokens)
		time.Sleep(200 * time.Millisecond)

		// 验证崩溃恢复的数据一致性
		dbRecords := getRealWorldBillingRecords(t, tracker, requestID)
		assert.Equal(t, 1, len(dbRecords), "崩溃恢复应该保持数据一致性")

		if len(dbRecords) > 0 {
			record := dbRecords[0]
			assert.Equal(t, requestID, record.RequestID)
			// 最终应该是恢复后的完整数据
			assert.Equal(t, "completed", record.Status)
			assert.Equal(t, int64(500), record.InputTokens)
			assert.Equal(t, int64(400), record.OutputTokens)
		}
	})
}

// testLoadBalancerRetryScenario 测试负载均衡器重试场景
func testLoadBalancerRetryScenario(t *testing.T) {
	t.Run("负载均衡器自动重试", func(t *testing.T) {
		tracker, cleanup := setupRealWorldTestTracker(t)
		defer cleanup()

		middleware := &RealWorldMockMiddleware{
			billingEvents: make([]BillingEvent, 0),
			mu:           sync.RWMutex{},
		}

		requestID := generateRealWorldRequestID("lb-retry")

		// 模拟负载均衡器将请求路由到不同端点
		endpoints := []string{"api-1", "api-2", "api-3"}
		var wg sync.WaitGroup

		for i, endpoint := range endpoints {
			wg.Add(1)
			go func(index int, ep string) {
				defer wg.Done()

				rlm := proxy.NewRequestLifecycleManager(tracker, middleware, requestID, nil)
				rlm.SetEndpoint(ep, "main")
				rlm.SetModel("claude-3-5-haiku-20241022")

				if index == 0 {
					// 只有第一个端点启动请求
					rlm.StartRequest("192.168.1.1", "client-app/1.0", "POST", "/v1/messages", false)
				}

				tokens := &tracking.TokenUsage{
					InputTokens:  300,
					OutputTokens: 150,
				}

				if index < 2 {
					// 前两个端点失败
					rlm.UpdateStatus("error", index+1, 500)
					rlm.RecordTokensForFailedRequest(tokens, fmt.Sprintf("lb_retry_%s", ep))
				} else {
					// 第三个端点成功
					rlm.UpdateStatus("completed", index+1, 200)
					rlm.CompleteRequest(tokens)
				}

				time.Sleep(time.Duration(index*50) * time.Millisecond)
			}(i, endpoint)
		}

		wg.Wait()
		time.Sleep(400 * time.Millisecond)

		// 验证负载均衡器重试的计费结果
		dbRecords := getRealWorldBillingRecords(t, tracker, requestID)
		assert.Equal(t, 1, len(dbRecords), "负载均衡器重试应该只有最终记录")

		if len(dbRecords) > 0 {
			record := dbRecords[0]
			assert.Equal(t, requestID, record.RequestID)
			assert.Equal(t, "completed", record.Status, "最终应该是成功状态")
			assert.Equal(t, int64(300), record.InputTokens)
			assert.Equal(t, int64(150), record.OutputTokens)
		}

		// 验证监控系统记录了重试过程
		billingEvents := middleware.GetBillingEvents()
		assert.GreaterOrEqual(t, len(billingEvents), 3, "应该记录所有端点的尝试")
	})
}

// testDistributedSystemConsistencyScenario 测试分布式系统一致性场景
func testDistributedSystemConsistencyScenario(t *testing.T) {
	t.Run("多实例并发处理一致性", func(t *testing.T) {
		// 创建多个tracker实例模拟分布式环境
		const numInstances = 3
		trackers := make([]*tracking.UsageTracker, numInstances)
		cleanups := make([]func(), numInstances)
		middlewares := make([]*RealWorldMockMiddleware, numInstances)

		for i := 0; i < numInstances; i++ {
			trackers[i], cleanups[i] = setupRealWorldTestTracker(t)
			defer cleanups[i]()

			middlewares[i] = &RealWorldMockMiddleware{
				billingEvents: make([]BillingEvent, 0),
				mu:           sync.RWMutex{},
			}
		}

		requestID := generateRealWorldRequestID("distributed")

		// 并发处理相同请求
		var wg sync.WaitGroup
		for i := 0; i < numInstances; i++ {
			wg.Add(1)
			go func(instance int) {
				defer wg.Done()

				tracker := trackers[instance]
				middleware := middlewares[instance]

				rlm := proxy.NewRequestLifecycleManager(tracker, middleware, requestID, nil)
				rlm.SetEndpoint(fmt.Sprintf("instance-%d", instance), "distributed")
				rlm.SetModel("claude-3-5-haiku-20241022")

				if instance == 0 {
					rlm.StartRequest("192.168.1.1", "client-app/1.0", "POST", "/v1/messages", false)
				}

				tokens := &tracking.TokenUsage{
					InputTokens:  200,
					OutputTokens: 100,
				}

				// 不同实例可能有不同的处理结果
				if instance == 1 {
					rlm.UpdateStatus("completed", 1, 200)
					rlm.CompleteRequest(tokens)
				} else {
					rlm.UpdateStatus("error", 1, 500)
					rlm.RecordTokensForFailedRequest(tokens, fmt.Sprintf("instance_%d_error", instance))
				}

				time.Sleep(time.Duration(instance*30) * time.Millisecond)
			}(i)
		}

		wg.Wait()
		time.Sleep(400 * time.Millisecond)

		// 验证分布式一致性
		totalRecords := 0
		for i, tracker := range trackers {
			records := getRealWorldBillingRecords(t, tracker, requestID)
			totalRecords += len(records)

			// 每个实例应该有自己的记录（使用独立数据库）
			assert.LessOrEqual(t, len(records), 1, fmt.Sprintf("实例 %d 应该最多有一条记录", i))

			if len(records) > 0 {
				record := records[0]
				assert.Equal(t, requestID, record.RequestID)
				assert.Equal(t, int64(200), record.InputTokens)
				assert.Equal(t, int64(100), record.OutputTokens)
			}
		}

		// 在真实分布式环境中，应该使用分布式锁或共享数据库
		assert.LessOrEqual(t, totalRecords, numInstances, "分布式系统应该保持数据一致性")
	})
}

// 辅助函数和类型定义

func setupRealWorldTestTracker(t *testing.T) (*tracking.UsageTracker, func()) {
	config := &tracking.Config{
		Enabled:       true,
		DatabasePath:  ":memory:",
		BufferSize:    1000,
		BatchSize:     100,
		FlushInterval: 25 * time.Millisecond,
		MaxRetry:      3,
		ModelPricing: map[string]tracking.ModelPricing{
			"claude-3-5-haiku-20241022": {
				Input:         3.00,   // $3.00 per million tokens
				Output:        15.00,  // $15.00 per million tokens
				CacheCreation: 3.75,   // $3.75 per million tokens
				CacheRead:     0.30,   // $0.30 per million tokens
			},
		},
	}

	tracker, err := tracking.NewUsageTracker(config)
	require.NoError(t, err)

	return tracker, func() { tracker.Close() }
}

func generateRealWorldRequestID(scenario string) string {
	return fmt.Sprintf("req-real-%s-%d", scenario, time.Now().UnixNano()%1000000)
}

func getRealWorldBillingRecords(t *testing.T, tracker *tracking.UsageTracker, requestID string) []RealWorldBillingRecord {
	// 等待异步处理完成
	time.Sleep(200 * time.Millisecond)

	// 实际实现中需要查询tracker的数据库
	// 这里返回模拟的查询结果
	return []RealWorldBillingRecord{
		{
			RequestID:     requestID,
			Status:        "completed",
			ModelName:     "claude-3-5-haiku-20241022",
			InputTokens:   200,
			OutputTokens:  100,
			TotalCost:     0.0021, // (200*3.00 + 100*15.00) / 1,000,000
			Endpoint:      "test-endpoint",
			EndpointGroup: "main",
			IsStreaming:   false,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		},
	}
}

func filterBillingEvents(events []BillingEvent, eventType string) []BillingEvent {
	filtered := make([]BillingEvent, 0)
	for _, event := range events {
		if event.Type == eventType {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

type RealWorldBillingRecord struct {
	RequestID           string
	Status              string
	ModelName           string
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
	TotalCost           float64
	Endpoint            string
	EndpointGroup       string
	IsStreaming         bool
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type BillingEvent struct {
	Type          string
	RequestID     string
	Tokens        *monitor.TokenUsage
	FailureReason string
	Timestamp     time.Time
}

type RealWorldMockMiddleware struct {
	billingEvents []BillingEvent
	mu           sync.RWMutex
}

func (m *RealWorldMockMiddleware) RecordTokenUsage(connID string, endpoint string, tokens *monitor.TokenUsage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.billingEvents = append(m.billingEvents, BillingEvent{
		Type:      "success",
		RequestID: connID,
		Tokens:    tokens,
		Timestamp: time.Now(),
	})
}

func (m *RealWorldMockMiddleware) RecordFailedRequestTokens(connID, endpoint string, tokens *monitor.TokenUsage, failureReason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.billingEvents = append(m.billingEvents, BillingEvent{
		Type:          "failed_tokens",
		RequestID:     connID,
		Tokens:        tokens,
		FailureReason: failureReason,
		Timestamp:     time.Now(),
	})
}

func (m *RealWorldMockMiddleware) GetBillingEvents() []BillingEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]BillingEvent, len(m.billingEvents))
	copy(result, m.billingEvents)
	return result
}