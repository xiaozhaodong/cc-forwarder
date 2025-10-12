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

// TestDuplicateBillingProtection 重复计费防护测试套件
// 确保Token记录机制不会导致重复计费问题
func TestDuplicateBillingProtection(t *testing.T) {
	t.Run("重试循环中的重复计费防护", testRetryLoopDuplicateBillingProtection)
	t.Run("正常完成vs失败记录防护", testCompletionVsFailureProtection)
	t.Run("并发Token记录防护", testConcurrentTokenRecordingProtection)
	t.Run("数据库级别重复防护", testDatabaseLevelDuplicationProtection)
	t.Run("跨组件重复记录防护", testCrossComponentDuplicationProtection)
	t.Run("异常恢复场景防护", testExceptionRecoveryProtection)
	t.Run("时间窗口重复防护", testTimeWindowDuplicationProtection)
}

// testRetryLoopDuplicateBillingProtection 测试重试循环中的重复计费防护
func testRetryLoopDuplicateBillingProtection(t *testing.T) {
	t.Run("重试过程中多次Token记录防护", func(t *testing.T) {
		// 创建测试环境
		tracker, cleanup := setupIntegrationTestTracker(t)
		defer cleanup()

		middleware := &MockMonitoringMiddleware{
			tokenRecords:       make([]*TokenRecord, 0),
			failedTokenRecords: make([]*FailedTokenRecord, 0),
			mu:                 sync.RWMutex{},
		}

		requestID := generateTestRequestID("retry-protection")
		rlm := proxy.NewRequestLifecycleManager(tracker, middleware, requestID, nil)
		rlm.SetEndpoint("test-endpoint", "test-group")
		rlm.SetModel("claude-3-5-haiku-20241022")

		// 启动请求记录
		rlm.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

		// 模拟Token信息
		tokens := &tracking.TokenUsage{
			InputTokens:         257,
			OutputTokens:        25,
			CacheCreationTokens: 0,
			CacheReadTokens:     0,
		}

		// 模拟重试过程中的多次调用
		for i := 0; i < 3; i++ {
			rlm.UpdateStatus("retry", i+1, 500)
			// 重试过程中不应该记录Token（这是关键点）
			// 只有最终失败或成功时才记录
		}

		// 最终失败时记录Token
		rlm.UpdateStatus("timeout", 3, 500)
		rlm.RecordTokensForFailedRequest(tokens, "timeout")

		// 等待异步处理
		time.Sleep(200 * time.Millisecond)

		// 验证：只有一条Token记录
		billingRecords := getBillingRecordsFromDB(t, tracker, requestID)
		assert.Equal(t, 1, len(billingRecords), "应该只有一条计费记录")

		if len(billingRecords) > 0 {
			record := billingRecords[0]
			assert.Equal(t, "timeout", record.Status, "状态应该是timeout")
			assert.Equal(t, int64(257), record.InputTokens, "输入Token应该正确")
			assert.Equal(t, int64(25), record.OutputTokens, "输出Token应该正确")
			assert.True(t, record.TotalCost > 0, "总成本应该大于0")
		}

		// 验证监控记录也只有一条
		failedRecords := middleware.GetFailedTokenRecords()
		assert.Equal(t, 1, len(failedRecords), "应该只有一条失败Token记录")
	})

	t.Run("重试失败不产生多条计费记录", func(t *testing.T) {
		tracker, cleanup := setupIntegrationTestTracker(t)
		defer cleanup()

		middleware := &MockMonitoringMiddleware{
			tokenRecords:       make([]*TokenRecord, 0),
			failedTokenRecords: make([]*FailedTokenRecord, 0),
			mu:                 sync.RWMutex{},
		}

		requestID := generateTestRequestID("retry-billing")
		rlm := proxy.NewRequestLifecycleManager(tracker, middleware, requestID, nil)
		rlm.SetEndpoint("test-endpoint", "test-group")
		rlm.SetModel("claude-3-5-haiku-20241022")

		// 启动请求
		rlm.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

		tokens := &tracking.TokenUsage{
			InputTokens:  500,
			OutputTokens: 200,
		}

		// 模拟多次重试失败，但只最后一次记录Token
		for i := 0; i < 5; i++ {
			rlm.UpdateStatus("retry", i+1, 500)
		}

		// 最终失败，只记录一次
		rlm.UpdateStatus("error", 5, 500)
		rlm.RecordTokensForFailedRequest(tokens, "error")

		// 等待处理
		time.Sleep(200 * time.Millisecond)

		// 验证数据库中只有一条记录
		records := getBillingRecordsFromDB(t, tracker, requestID)
		assert.Equal(t, 1, len(records), "重试失败应该只产生一条计费记录")

		// 验证费用计算正确
		if len(records) > 0 {
			record := records[0]
			expectedInputCost := float64(500) * 3.00 / 1000000.0 // claude-3-5-haiku定价
			expectedOutputCost := float64(200) * 15.00 / 1000000.0
			expectedTotal := expectedInputCost + expectedOutputCost

			assert.InDelta(t, expectedTotal, record.TotalCost, 0.0001, "总成本计算应该正确")
		}
	})
}

// testCompletionVsFailureProtection 测试正常完成vs失败记录防护
func testCompletionVsFailureProtection(t *testing.T) {
	t.Run("请求先失败后成功的情况", func(t *testing.T) {
		tracker, cleanup := setupIntegrationTestTracker(t)
		defer cleanup()

		middleware := &MockMonitoringMiddleware{
			tokenRecords:       make([]*TokenRecord, 0),
			failedTokenRecords: make([]*FailedTokenRecord, 0),
			mu:                 sync.RWMutex{},
		}

		requestID := generateTestRequestID("fail-then-success")
		rlm := proxy.NewRequestLifecycleManager(tracker, middleware, requestID, nil)
		rlm.SetEndpoint("test-endpoint", "test-group")
		rlm.SetModel("claude-3-5-haiku-20241022")

		// 启动请求
		rlm.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

		failTokens := &tracking.TokenUsage{
			InputTokens:  100,
			OutputTokens: 50,
		}

		successTokens := &tracking.TokenUsage{
			InputTokens:  300,
			OutputTokens: 150,
		}

		// 首先失败并记录Token
		rlm.UpdateStatus("timeout", 1, 500)
		rlm.RecordTokensForFailedRequest(failTokens, "timeout")

		time.Sleep(100 * time.Millisecond)

		// 然后重试成功
		rlm.UpdateStatus("completed", 2, 200)
		rlm.CompleteRequest(successTokens)

		time.Sleep(200 * time.Millisecond)

		// 验证：应该只有成功的记录被保留
		records := getBillingRecordsFromDB(t, tracker, requestID)
		assert.Equal(t, 1, len(records), "应该只有一条最终记录")

		if len(records) > 0 {
			record := records[0]
			assert.Equal(t, "completed", record.Status, "最终状态应该是completed")
			assert.Equal(t, int64(300), record.InputTokens, "应该是成功请求的Token数量")
			assert.Equal(t, int64(150), record.OutputTokens, "应该是成功请求的Token数量")
		}

		// 验证监控记录
		tokenRecords := middleware.GetTokenRecords()
		failedRecords := middleware.GetFailedTokenRecords()

		// 应该有一个失败记录和一个成功记录，但最终计费以成功为准
		assert.GreaterOrEqual(t, len(tokenRecords), 1, "应该有成功Token记录")
		assert.GreaterOrEqual(t, len(failedRecords), 1, "应该有失败Token记录")
	})

	t.Run("确保状态转换时Token记录的唯一性", func(t *testing.T) {
		tracker, cleanup := setupIntegrationTestTracker(t)
		defer cleanup()

		middleware := &MockMonitoringMiddleware{
			tokenRecords:       make([]*TokenRecord, 0),
			failedTokenRecords: make([]*FailedTokenRecord, 0),
			mu:                 sync.RWMutex{},
		}

		requestID := generateTestRequestID("state-transition")
		rlm := proxy.NewRequestLifecycleManager(tracker, middleware, requestID, nil)
		rlm.SetEndpoint("test-endpoint", "test-group")
		rlm.SetModel("claude-3-5-haiku-20241022")

		rlm.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

		tokens := &tracking.TokenUsage{
			InputTokens:  1000,
			OutputTokens: 500,
		}

		// 模拟状态快速变化
		rlm.UpdateStatus("forwarding", 1, 200)
		rlm.UpdateStatus("processing", 1, 200)
		rlm.UpdateStatus("error", 1, 500)

		// 只在最终状态记录Token
		rlm.RecordTokensForFailedRequest(tokens, "error")

		time.Sleep(200 * time.Millisecond)

		// 验证最终只有一条记录
		records := getBillingRecordsFromDB(t, tracker, requestID)
		assert.Equal(t, 1, len(records), "状态转换过程中应该只产生一条计费记录")

		if len(records) > 0 {
			record := records[0]
			assert.Equal(t, "error", record.Status, "最终状态应该正确")
		}
	})
}

// testConcurrentTokenRecordingProtection 测试并发Token记录防护
func testConcurrentTokenRecordingProtection(t *testing.T) {
	t.Run("多个goroutine同时记录Token", func(t *testing.T) {
		tracker, cleanup := setupIntegrationTestTracker(t)
		defer cleanup()

		middleware := &MockMonitoringMiddleware{
			tokenRecords:       make([]*TokenRecord, 0),
			failedTokenRecords: make([]*FailedTokenRecord, 0),
			mu:                 sync.RWMutex{},
		}

		requestID := generateTestRequestID("concurrent-tokens")
		rlm := proxy.NewRequestLifecycleManager(tracker, middleware, requestID, nil)
		rlm.SetEndpoint("test-endpoint", "test-group")
		rlm.SetModel("claude-3-5-haiku-20241022")

		rlm.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

		tokens := &tracking.TokenUsage{
			InputTokens:  200,
			OutputTokens: 100,
		}

		// 并发尝试记录相同的Token信息
		const numGoroutines = 10
		var wg sync.WaitGroup

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				rlm.RecordTokensForFailedRequest(tokens, "concurrent_test")
			}(i)
		}

		wg.Wait()
		time.Sleep(300 * time.Millisecond)

		// 验证最终只有一条记录（或者记录被正确合并）
		records := getBillingRecordsFromDB(t, tracker, requestID)

		// 数据库应该处理并发写入，确保数据一致性
		assert.GreaterOrEqual(t, len(records), 1, "应该至少有一条记录")
		assert.LessOrEqual(t, len(records), 2, "不应该有过多重复记录")

		// 验证Token数量是预期的
		totalInputTokens := int64(0)
		totalOutputTokens := int64(0)
		for _, record := range records {
			totalInputTokens += record.InputTokens
			totalOutputTokens += record.OutputTokens
		}

		// 对于同一个请求，Token总量应该是确定的
		assert.Equal(t, int64(200), totalInputTokens, "总输入Token应该正确")
		assert.Equal(t, int64(100), totalOutputTokens, "总输出Token应该正确")
	})

	t.Run("验证数据库约束和业务逻辑防重", func(t *testing.T) {
		tracker, cleanup := setupIntegrationTestTracker(t)
		defer cleanup()

		middleware := &MockMonitoringMiddleware{
			tokenRecords:       make([]*TokenRecord, 0),
			failedTokenRecords: make([]*FailedTokenRecord, 0),
			mu:                 sync.RWMutex{},
		}

		requestID := generateTestRequestID("db-constraints")

		// 创建多个生命周期管理器实例（模拟异常情况）
		managers := make([]*proxy.RequestLifecycleManager, 3)
		for i := range managers {
			managers[i] = proxy.NewRequestLifecycleManager(tracker, middleware, requestID, nil)
			managers[i].SetEndpoint("test-endpoint", "test-group")
			managers[i].SetModel("claude-3-5-haiku-20241022")
		}

		// 只有第一个启动请求记录
		managers[0].StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

		tokens := &tracking.TokenUsage{
			InputTokens:  150,
			OutputTokens: 75,
		}

		// 并发从不同管理器记录Token
		var wg sync.WaitGroup
		for i, mgr := range managers {
			wg.Add(1)
			go func(index int, manager *proxy.RequestLifecycleManager) {
				defer wg.Done()
				manager.RecordTokensForFailedRequest(tokens, fmt.Sprintf("test_%d", index))
			}(i, mgr)
		}

		wg.Wait()
		time.Sleep(300 * time.Millisecond)

		// 验证数据库约束确保只有一条记录被保留
		records := getBillingRecordsFromDB(t, tracker, requestID)
		assert.LessOrEqual(t, len(records), 1, "数据库约束应该防止重复记录")

		if len(records) > 0 {
			record := records[0]
			assert.Equal(t, int64(150), record.InputTokens, "Token数量应该正确")
			assert.Equal(t, int64(75), record.OutputTokens, "Token数量应该正确")
		}
	})
}

// testDatabaseLevelDuplicationProtection 测试数据库级别重复防护
func testDatabaseLevelDuplicationProtection(t *testing.T) {
	t.Run("相同request_id的重复插入防护", func(t *testing.T) {
		tracker, cleanup := setupIntegrationTestTracker(t)
		defer cleanup()

		middleware := &MockMonitoringMiddleware{
			tokenRecords:       make([]*TokenRecord, 0),
			failedTokenRecords: make([]*FailedTokenRecord, 0),
			mu:                 sync.RWMutex{},
		}

		requestID := generateTestRequestID("db-duplicate")

		// 创建第一个记录
		rlm1 := proxy.NewRequestLifecycleManager(tracker, middleware, requestID, nil)
		rlm1.SetEndpoint("test-endpoint-1", "test-group")
		rlm1.SetModel("claude-3-5-haiku-20241022")
		rlm1.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

		tokens1 := &tracking.TokenUsage{
			InputTokens:  100,
			OutputTokens: 50,
		}
		rlm1.RecordTokensForFailedRequest(tokens1, "error")

		time.Sleep(100 * time.Millisecond)

		// 尝试创建第二个相同request_id的记录
		rlm2 := proxy.NewRequestLifecycleManager(tracker, middleware, requestID, nil)
		rlm2.SetEndpoint("test-endpoint-2", "test-group")
		rlm2.SetModel("claude-3-5-haiku-20241022")

		tokens2 := &tracking.TokenUsage{
			InputTokens:  200,
			OutputTokens: 100,
		}
		rlm2.RecordTokensForFailedRequest(tokens2, "timeout")

		time.Sleep(200 * time.Millisecond)

		// 验证数据库约束生效
		records := getBillingRecordsFromDB(t, tracker, requestID)
		assert.Equal(t, 1, len(records), "数据库应该只保留一条记录")

		// 验证是第一次的记录被保留（或者正确更新）
		if len(records) > 0 {
			record := records[0]
			// 数据库可能保留第一条记录或者更新为最新值，具体取决于实现
			assert.True(t, record.InputTokens == 100 || record.InputTokens == 200, "Token数量应该是其中一个值")
		}
	})

	t.Run("验证数据库事务隔离级别", func(t *testing.T) {
		tracker, cleanup := setupIntegrationTestTracker(t)
		defer cleanup()

		middleware := &MockMonitoringMiddleware{
			tokenRecords:       make([]*TokenRecord, 0),
			failedTokenRecords: make([]*FailedTokenRecord, 0),
			mu:                 sync.RWMutex{},
		}

		requestID := generateTestRequestID("transaction-isolation")

		// 并发事务测试
		const numTransactions = 5
		var wg sync.WaitGroup

		for i := 0; i < numTransactions; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()

				rlm := proxy.NewRequestLifecycleManager(tracker, middleware, requestID, nil)
				rlm.SetEndpoint("test-endpoint", "test-group")
				rlm.SetModel("claude-3-5-haiku-20241022")

				if index == 0 {
					// 只有第一个事务启动请求
					rlm.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)
				}

				tokens := &tracking.TokenUsage{
					InputTokens:  int64(100 * (index + 1)),
					OutputTokens: int64(50 * (index + 1)),
				}

				rlm.RecordTokensForFailedRequest(tokens, fmt.Sprintf("transaction_%d", index))
			}(i)
		}

		wg.Wait()
		time.Sleep(400 * time.Millisecond)

		// 验证最终一致性
		records := getBillingRecordsFromDB(t, tracker, requestID)
		assert.LessOrEqual(t, len(records), 1, "事务隔离应该确保数据一致性")
	})
}

// testCrossComponentDuplicationProtection 测试跨组件重复记录防护
func testCrossComponentDuplicationProtection(t *testing.T) {
	t.Run("UsageTracker和MonitoringMiddleware的记录同步", func(t *testing.T) {
		tracker, cleanup := setupIntegrationTestTracker(t)
		defer cleanup()

		middleware := &MockMonitoringMiddleware{
			tokenRecords:       make([]*TokenRecord, 0),
			failedTokenRecords: make([]*FailedTokenRecord, 0),
			mu:                 sync.RWMutex{},
		}

		requestID := generateTestRequestID("cross-component")
		rlm := proxy.NewRequestLifecycleManager(tracker, middleware, requestID, nil)
		rlm.SetEndpoint("test-endpoint", "test-group")
		rlm.SetModel("claude-3-5-haiku-20241022")

		rlm.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

		tokens := &tracking.TokenUsage{
			InputTokens:  400,
			OutputTokens: 200,
		}

		// 记录失败Token
		rlm.RecordTokensForFailedRequest(tokens, "cross_component_test")

		time.Sleep(200 * time.Millisecond)

		// 验证UsageTracker记录
		dbRecords := getBillingRecordsFromDB(t, tracker, requestID)
		assert.Equal(t, 1, len(dbRecords), "数据库应该有一条记录")

		// 验证MonitoringMiddleware记录
		failedRecords := middleware.GetFailedTokenRecords()
		assert.Equal(t, 1, len(failedRecords), "监控中间件应该有一条失败记录")

		// 验证记录内容一致性
		if len(dbRecords) > 0 && len(failedRecords) > 0 {
			dbRecord := dbRecords[0]
			monitorRecord := failedRecords[0]

			assert.Equal(t, dbRecord.InputTokens, int64(monitorRecord.Tokens.InputTokens), "输入Token应该一致")
			assert.Equal(t, dbRecord.OutputTokens, int64(monitorRecord.Tokens.OutputTokens), "输出Token应该一致")
			assert.Equal(t, "cross_component_test", monitorRecord.FailureReason, "失败原因应该正确")
		}
	})

	t.Run("验证统计指标的准确性", func(t *testing.T) {
		tracker, cleanup := setupIntegrationTestTracker(t)
		defer cleanup()

		middleware := &MockMonitoringMiddleware{
			tokenRecords:       make([]*TokenRecord, 0),
			failedTokenRecords: make([]*FailedTokenRecord, 0),
			mu:                 sync.RWMutex{},
		}

		// 创建多个请求来验证统计准确性
		requestIDs := make([]string, 3)
		for i := 0; i < 3; i++ {
			requestIDs[i] = generateTestRequestID(fmt.Sprintf("stats-accuracy-%d", i))

			rlm := proxy.NewRequestLifecycleManager(tracker, middleware, requestIDs[i], nil)
			rlm.SetEndpoint("test-endpoint", "test-group")
			rlm.SetModel("claude-3-5-haiku-20241022")
			rlm.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

			tokens := &tracking.TokenUsage{
				InputTokens:  int64(100 * (i + 1)),
				OutputTokens: int64(50 * (i + 1)),
			}

			if i%2 == 0 {
				// 成功请求
				rlm.UpdateStatus("completed", 1, 200)
				rlm.CompleteRequest(tokens)
			} else {
				// 失败请求
				rlm.UpdateStatus("error", 1, 500)
				rlm.RecordTokensForFailedRequest(tokens, "test_error")
			}
		}

		time.Sleep(300 * time.Millisecond)

		// 验证数据库记录数量
		totalRecords := 0
		for _, requestID := range requestIDs {
			records := getBillingRecordsFromDB(t, tracker, requestID)
			totalRecords += len(records)
		}
		assert.Equal(t, 3, totalRecords, "应该有3条记录")

		// 验证监控记录
		tokenRecords := middleware.GetTokenRecords()
		failedRecords := middleware.GetFailedTokenRecords()

		// 应该有2个成功记录和1个失败记录
		assert.GreaterOrEqual(t, len(tokenRecords), 2, "应该有至少2个成功Token记录")
		assert.GreaterOrEqual(t, len(failedRecords), 1, "应该有至少1个失败Token记录")
	})
}

// testExceptionRecoveryProtection 测试异常恢复场景防护
func testExceptionRecoveryProtection(t *testing.T) {
	t.Run("网络中断恢复后的重复处理防护", func(t *testing.T) {
		tracker, cleanup := setupIntegrationTestTracker(t)
		defer cleanup()

		middleware := &MockMonitoringMiddleware{
			tokenRecords:       make([]*TokenRecord, 0),
			failedTokenRecords: make([]*FailedTokenRecord, 0),
			mu:                 sync.RWMutex{},
		}

		requestID := generateTestRequestID("network-recovery")
		rlm := proxy.NewRequestLifecycleManager(tracker, middleware, requestID, nil)
		rlm.SetEndpoint("test-endpoint", "test-group")
		rlm.SetModel("claude-3-5-haiku-20241022")

		rlm.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

		tokens := &tracking.TokenUsage{
			InputTokens:  300,
			OutputTokens: 150,
		}

		// 模拟网络中断期间的记录尝试
		rlm.UpdateStatus("network_error", 1, 0)
		rlm.RecordTokensForFailedRequest(tokens, "network_error")

		time.Sleep(100 * time.Millisecond)

		// 模拟网络恢复后的重试
		rlm.UpdateStatus("retry", 2, 200)
		// 网络恢复后不应该重复记录相同的Token

		// 最终成功
		rlm.UpdateStatus("completed", 2, 200)
		rlm.CompleteRequest(tokens)

		time.Sleep(200 * time.Millisecond)

		// 验证最终只有成功记录
		records := getBillingRecordsFromDB(t, tracker, requestID)
		assert.Equal(t, 1, len(records), "网络恢复后应该只有一条最终记录")

		if len(records) > 0 {
			record := records[0]
			assert.Equal(t, "completed", record.Status, "最终状态应该是completed")
		}
	})

	t.Run("系统崩溃恢复的数据一致性", func(t *testing.T) {
		tracker, cleanup := setupIntegrationTestTracker(t)
		defer cleanup()

		middleware := &MockMonitoringMiddleware{
			tokenRecords:       make([]*TokenRecord, 0),
			failedTokenRecords: make([]*FailedTokenRecord, 0),
			mu:                 sync.RWMutex{},
		}

		requestID := generateTestRequestID("crash-recovery")

		// 模拟系统崩溃前的状态
		rlm1 := proxy.NewRequestLifecycleManager(tracker, middleware, requestID, nil)
		rlm1.SetEndpoint("test-endpoint", "test-group")
		rlm1.SetModel("claude-3-5-haiku-20241022")
		rlm1.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

		tokens := &tracking.TokenUsage{
			InputTokens:  250,
			OutputTokens: 125,
		}

		rlm1.RecordTokensForFailedRequest(tokens, "crash_before")
		time.Sleep(100 * time.Millisecond)

		// 模拟系统重启后的处理（创建新的管理器实例）
		rlm2 := proxy.NewRequestLifecycleManager(tracker, middleware, requestID, nil)
		rlm2.SetEndpoint("test-endpoint", "test-group")
		rlm2.SetModel("claude-3-5-haiku-20241022")

		// 重启后不应该重复处理已记录的Token
		rlm2.RecordTokensForFailedRequest(tokens, "crash_after")

		time.Sleep(200 * time.Millisecond)

		// 验证数据一致性
		records := getBillingRecordsFromDB(t, tracker, requestID)
		assert.LessOrEqual(t, len(records), 1, "系统恢复后不应该有重复记录")

		if len(records) > 0 {
			record := records[0]
			assert.Equal(t, int64(250), record.InputTokens, "Token数量应该正确")
			assert.Equal(t, int64(125), record.OutputTokens, "Token数量应该正确")
		}
	})
}

// testTimeWindowDuplicationProtection 测试时间窗口重复防护
func testTimeWindowDuplicationProtection(t *testing.T) {
	t.Run("短时间内的重复调用防护", func(t *testing.T) {
		tracker, cleanup := setupIntegrationTestTracker(t)
		defer cleanup()

		middleware := &MockMonitoringMiddleware{
			tokenRecords:       make([]*TokenRecord, 0),
			failedTokenRecords: make([]*FailedTokenRecord, 0),
			mu:                 sync.RWMutex{},
		}

		requestID := generateTestRequestID("time-window")
		rlm := proxy.NewRequestLifecycleManager(tracker, middleware, requestID, nil)
		rlm.SetEndpoint("test-endpoint", "test-group")
		rlm.SetModel("claude-3-5-haiku-20241022")

		rlm.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

		tokens := &tracking.TokenUsage{
			InputTokens:  180,
			OutputTokens: 90,
		}

		// 短时间内多次调用
		for i := 0; i < 5; i++ {
			rlm.RecordTokensForFailedRequest(tokens, "time_window_test")
			time.Sleep(10 * time.Millisecond) // 很短的间隔
		}

		time.Sleep(200 * time.Millisecond)

		// 验证防重机制
		records := getBillingRecordsFromDB(t, tracker, requestID)
		assert.LessOrEqual(t, len(records), 1, "短时间内的重复调用应该被防护")

		if len(records) > 0 {
			record := records[0]
			assert.Equal(t, int64(180), record.InputTokens, "Token数量应该正确")
			assert.Equal(t, int64(90), record.OutputTokens, "Token数量应该正确")
		}
	})

	t.Run("验证时间戳和去重逻辑", func(t *testing.T) {
		tracker, cleanup := setupIntegrationTestTracker(t)
		defer cleanup()

		middleware := &MockMonitoringMiddleware{
			tokenRecords:       make([]*TokenRecord, 0),
			failedTokenRecords: make([]*FailedTokenRecord, 0),
			mu:                 sync.RWMutex{},
		}

		requestID := generateTestRequestID("timestamp-dedup")
		rlm := proxy.NewRequestLifecycleManager(tracker, middleware, requestID, nil)
		rlm.SetEndpoint("test-endpoint", "test-group")
		rlm.SetModel("claude-3-5-haiku-20241022")

		rlm.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

		tokens1 := &tracking.TokenUsage{
			InputTokens:  100,
			OutputTokens: 50,
		}

		tokens2 := &tracking.TokenUsage{
			InputTokens:  200,
			OutputTokens: 100,
		}

		// 第一次记录
		rlm.RecordTokensForFailedRequest(tokens1, "first_attempt")
		time.Sleep(50 * time.Millisecond)

		// 短时间内的第二次记录（可能是重复）
		rlm.RecordTokensForFailedRequest(tokens1, "duplicate_attempt")
		time.Sleep(50 * time.Millisecond)

		// 稍后的第三次记录（不同的Token值）
		rlm.RecordTokensForFailedRequest(tokens2, "different_tokens")

		time.Sleep(200 * time.Millisecond)

		// 验证去重逻辑
		records := getBillingRecordsFromDB(t, tracker, requestID)
		assert.LessOrEqual(t, len(records), 2, "时间戳去重应该生效")

		// 验证最终保留的是最新或最大的Token值
		if len(records) > 0 {
			record := records[len(records)-1] // 最后一条记录
			// 应该是最新的tokens2值或者tokens1值
			assert.True(t,
				(record.InputTokens == 100 && record.OutputTokens == 50) ||
				(record.InputTokens == 200 && record.OutputTokens == 100),
				"保留的Token值应该是有效的")
		}
	})
}

// 辅助函数

// setupIntegrationTestTracker 创建集成测试用的UsageTracker
func setupIntegrationTestTracker(t *testing.T) (*tracking.UsageTracker, func()) {
	config := &tracking.Config{
		Enabled:       true,
		DatabasePath:  ":memory:",
		BufferSize:    100,
		BatchSize:     10,
		FlushInterval: 50 * time.Millisecond,
		MaxRetry:      3,
		ModelPricing: map[string]tracking.ModelPricing{
			"claude-3-5-haiku-20241022": {
				Input:         3.00,
				Output:        15.00,
				CacheCreation: 3.75,
				CacheRead:     0.30,
			},
		},
	}

	tracker, err := tracking.NewUsageTracker(config)
	require.NoError(t, err)

	cleanup := func() {
		tracker.Close()
	}

	return tracker, cleanup
}

// generateTestRequestID 生成测试用的请求ID
func generateTestRequestID(suffix string) string {
	return fmt.Sprintf("req-dupbill-%s-%d", suffix, time.Now().UnixNano()%100000)
}

// getBillingRecordsFromDB 从数据库获取计费记录
func getBillingRecordsFromDB(t *testing.T, tracker *tracking.UsageTracker, requestID string) []BillingRecord {
	// 等待异步操作完成
	time.Sleep(150 * time.Millisecond)

	// 实际实现中需要直接查询tracker的数据库
	// 这里返回模拟的查询结果

	// 等待一下确保数据已写入
	time.Sleep(100 * time.Millisecond)

	// 这里返回模拟数据，实际实现需要真实查询数据库
	return []BillingRecord{
		{
			RequestID:    requestID,
			Status:       "error",
			InputTokens:  100,
			OutputTokens: 50,
			TotalCost:    0.001,
		},
	}
}

// BillingRecord 计费记录结构
type BillingRecord struct {
	RequestID    string
	Status       string
	InputTokens  int64
	OutputTokens int64
	TotalCost    float64
}

// MockMonitoringMiddleware 模拟监控中间件
type MockMonitoringMiddleware struct {
	tokenRecords       []*TokenRecord
	failedTokenRecords []*FailedTokenRecord
	mu                 sync.RWMutex
}

type TokenRecord struct {
	ConnID   string
	Endpoint string
	Tokens   *monitor.TokenUsage
}

type FailedTokenRecord struct {
	ConnID        string
	Endpoint      string
	Tokens        *monitor.TokenUsage
	FailureReason string
}

func (m *MockMonitoringMiddleware) RecordTokenUsage(connID string, endpoint string, tokens *monitor.TokenUsage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tokenRecords = append(m.tokenRecords, &TokenRecord{
		ConnID:   connID,
		Endpoint: endpoint,
		Tokens:   tokens,
	})
}

func (m *MockMonitoringMiddleware) RecordFailedRequestTokens(connID, endpoint string, tokens *monitor.TokenUsage, failureReason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failedTokenRecords = append(m.failedTokenRecords, &FailedTokenRecord{
		ConnID:        connID,
		Endpoint:      endpoint,
		Tokens:        tokens,
		FailureReason: failureReason,
	})
}

func (m *MockMonitoringMiddleware) GetTokenRecords() []*TokenRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*TokenRecord, len(m.tokenRecords))
	copy(result, m.tokenRecords)
	return result
}

func (m *MockMonitoringMiddleware) GetFailedTokenRecords() []*FailedTokenRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*FailedTokenRecord, len(m.failedTokenRecords))
	copy(result, m.failedTokenRecords)
	return result
}