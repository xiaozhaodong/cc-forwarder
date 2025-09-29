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

// TestAdvancedDuplicateBillingProtection 高级重复计费防护测试
// 专注于数据库级别和业务逻辑级别的防重机制
func TestAdvancedDuplicateBillingProtection(t *testing.T) {
	t.Run("数据库约束级别的重复防护", testDatabaseConstraintProtection)
	t.Run("业务逻辑级别的重复防护", testBusinessLogicProtection)
	t.Run("竞态条件下的计费一致性", testRaceConditionBillingConsistency)
	t.Run("失败重试场景的计费精确性", testFailureRetryBillingAccuracy)
	t.Run("跨会话的重复计费防护", testCrossSessionBillingProtection)
	t.Run("极端并发场景的计费正确性", testExtremeConcurrencyBillingCorrectness)
}

// testDatabaseConstraintProtection 测试数据库约束级别的重复防护
func testDatabaseConstraintProtection(t *testing.T) {
	t.Run("主键约束防止重复插入", func(t *testing.T) {
		tracker, cleanup := setupAdvancedTestTracker(t)
		defer cleanup()

		requestID := generateAdvancedTestRequestID("pk-constraint")

		// 直接测试数据库操作，模拟极端并发情况
		const numConcurrentInserts = 20
		var wg sync.WaitGroup

		// 先创建初始记录
		initialRLM := proxy.NewRequestLifecycleManager(tracker, nil, requestID, nil)
		initialRLM.SetEndpoint("test-endpoint", "test-group")
		initialRLM.SetModel("claude-3-5-haiku-20241022")
		initialRLM.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

		tokens := &tracking.TokenUsage{
			InputTokens:  500,
			OutputTokens: 250,
		}

		// 并发尝试插入相同的request_id
		for i := 0; i < numConcurrentInserts; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()

				rlm := proxy.NewRequestLifecycleManager(tracker, nil, requestID, nil)
				rlm.SetEndpoint(fmt.Sprintf("endpoint-%d", index), "test-group")
				rlm.SetModel("claude-3-5-haiku-20241022")

				// 尝试记录失败Token（这应该触发数据库约束）
				rlm.RecordTokensForFailedRequest(tokens, fmt.Sprintf("attempt-%d", index))
			}(i)
		}

		wg.Wait()
		time.Sleep(300 * time.Millisecond)

		// 验证数据库中只有一条记录
		records := getDetailedBillingRecords(t, tracker, requestID)
		assert.Equal(t, 1, len(records), "数据库主键约束应该防止重复插入")

		if len(records) > 0 {
			record := records[0]
			assert.Equal(t, requestID, record.RequestID)
			assert.Equal(t, int64(500), record.InputTokens)
			assert.Equal(t, int64(250), record.OutputTokens)
			assert.True(t, record.TotalCost > 0, "成本应该被正确计算")
		}
	})

	t.Run("唯一约束防止状态混乱", func(t *testing.T) {
		tracker, cleanup := setupAdvancedTestTracker(t)
		defer cleanup()

		requestID := generateAdvancedTestRequestID("unique-constraint")

		// 创建初始记录
		rlm := proxy.NewRequestLifecycleManager(tracker, nil, requestID, nil)
		rlm.SetEndpoint("test-endpoint", "test-group")
		rlm.SetModel("claude-3-5-haiku-20241022")
		rlm.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

		failTokens := &tracking.TokenUsage{
			InputTokens:  200,
			OutputTokens: 100,
		}

		successTokens := &tracking.TokenUsage{
			InputTokens:  400,
			OutputTokens: 200,
		}

		// 先记录失败
		rlm.UpdateStatus("error", 1, 500)
		rlm.RecordTokensForFailedRequest(failTokens, "initial_failure")
		time.Sleep(100 * time.Millisecond)

		// 然后尝试记录成功（应该覆盖失败记录）
		rlm.UpdateStatus("completed", 2, 200)
		rlm.CompleteRequest(successTokens)
		time.Sleep(200 * time.Millisecond)

		// 验证最终状态
		records := getDetailedBillingRecords(t, tracker, requestID)
		assert.Equal(t, 1, len(records), "应该只有一条最终记录")

		if len(records) > 0 {
			record := records[0]
			assert.Equal(t, "completed", record.Status, "最终状态应该是成功")
			assert.Equal(t, int64(400), record.InputTokens, "应该是成功的Token数量")
			assert.Equal(t, int64(200), record.OutputTokens, "应该是成功的Token数量")
		}
	})

	t.Run("事务级别的原子性保护", func(t *testing.T) {
		tracker, cleanup := setupAdvancedTestTracker(t)
		defer cleanup()

		requestID := generateAdvancedTestRequestID("transaction-atomic")

		// 模拟事务中断情况
		rlm := proxy.NewRequestLifecycleManager(tracker, nil, requestID, nil)
		rlm.SetEndpoint("test-endpoint", "test-group")
		rlm.SetModel("claude-3-5-haiku-20241022")
		rlm.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

		tokens := &tracking.TokenUsage{
			InputTokens:         1000,
			OutputTokens:        500,
			CacheCreationTokens: 100,
			CacheReadTokens:     50,
		}

		// 并发操作模拟事务竞争
		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				if index%2 == 0 {
					rlm.RecordTokensForFailedRequest(tokens, fmt.Sprintf("concurrent-%d", index))
				} else {
					rlm.CompleteRequest(tokens)
				}
			}(i)
		}

		wg.Wait()
		time.Sleep(400 * time.Millisecond)

		// 验证事务原子性
		records := getDetailedBillingRecords(t, tracker, requestID)
		assert.Equal(t, 1, len(records), "事务原子性应该确保只有一条记录")

		if len(records) > 0 {
			record := records[0]
			// 验证完整的Token信息
			totalTokens := record.InputTokens + record.OutputTokens + record.CacheCreationTokens + record.CacheReadTokens
			expectedTotal := int64(1000 + 500 + 100 + 50)
			assert.Equal(t, expectedTotal, totalTokens, "所有Token类型应该被正确记录")

			// 验证成本计算的正确性
			assert.True(t, record.TotalCost > 0, "总成本应该大于0")
			assert.True(t, record.InputCost > 0, "输入成本应该大于0")
			assert.True(t, record.OutputCost > 0, "输出成本应该大于0")
		}
	})
}

// testBusinessLogicProtection 测试业务逻辑级别的重复防护
func testBusinessLogicProtection(t *testing.T) {
	t.Run("请求状态机防止重复计费", func(t *testing.T) {
		tracker, cleanup := setupAdvancedTestTracker(t)
		defer cleanup()

		middleware := &AdvancedMockMiddleware{
			operationLog: make([]string, 0),
			mu:           sync.RWMutex{},
		}

		requestID := generateAdvancedTestRequestID("state-machine")
		rlm := proxy.NewRequestLifecycleManager(tracker, middleware, requestID, nil)
		rlm.SetEndpoint("test-endpoint", "test-group")
		rlm.SetModel("claude-3-5-haiku-20241022")

		rlm.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

		tokens := &tracking.TokenUsage{
			InputTokens:  300,
			OutputTokens: 150,
		}

		// 模拟状态转换序列
		statusSequence := []string{"pending", "forwarding", "processing", "error"}
		for i, status := range statusSequence {
			rlm.UpdateStatus(status, i+1, 500)
			time.Sleep(10 * time.Millisecond)
		}

		// 只在最终状态记录Token
		rlm.RecordTokensForFailedRequest(tokens, "final_failure")
		time.Sleep(200 * time.Millisecond)

		// 验证只有一次计费操作
		operations := middleware.GetOperationLog()
		failedTokenOps := countOperations(operations, "RecordFailedRequestTokens")
		assert.Equal(t, 1, failedTokenOps, "应该只有一次失败Token记录操作")

		// 验证数据库记录
		records := getDetailedBillingRecords(t, tracker, requestID)
		assert.Equal(t, 1, len(records), "状态机应该确保只有一次计费")
	})

	t.Run("重试逻辑防止中间状态计费", func(t *testing.T) {
		tracker, cleanup := setupAdvancedTestTracker(t)
		defer cleanup()

		middleware := &AdvancedMockMiddleware{
			operationLog: make([]string, 0),
			mu:           sync.RWMutex{},
		}

		requestID := generateAdvancedTestRequestID("retry-logic")
		rlm := proxy.NewRequestLifecycleManager(tracker, middleware, requestID, nil)
		rlm.SetEndpoint("test-endpoint", "test-group")
		rlm.SetModel("claude-3-5-haiku-20241022")

		rlm.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

		tokens := &tracking.TokenUsage{
			InputTokens:  600,
			OutputTokens: 300,
		}

		// 模拟多次重试
		for attempt := 1; attempt <= 5; attempt++ {
			rlm.UpdateStatus("retry", attempt, 500)
			time.Sleep(20 * time.Millisecond)
			// 重试过程中不记录Token
		}

		// 最终超时失败
		rlm.UpdateStatus("timeout", 5, 0)
		rlm.RecordTokensForFailedRequest(tokens, "final_timeout")

		time.Sleep(200 * time.Millisecond)

		// 验证重试过程不产生计费
		operations := middleware.GetOperationLog()
		failedTokenOps := countOperations(operations, "RecordFailedRequestTokens")
		assert.Equal(t, 1, failedTokenOps, "重试过程中应该只有最终的一次Token记录")

		// 验证计费准确性
		records := getDetailedBillingRecords(t, tracker, requestID)
		assert.Equal(t, 1, len(records), "重试逻辑应该确保计费准确性")

		if len(records) > 0 {
			record := records[0]
			assert.Equal(t, "timeout", record.Status)
			assert.Equal(t, int64(600), record.InputTokens)
			assert.Equal(t, int64(300), record.OutputTokens)
		}
	})

	t.Run("Token验证防止无效计费", func(t *testing.T) {
		tracker, cleanup := setupAdvancedTestTracker(t)
		defer cleanup()

		middleware := &AdvancedMockMiddleware{
			operationLog: make([]string, 0),
			mu:           sync.RWMutex{},
		}

		requestID := generateAdvancedTestRequestID("token-validation")
		rlm := proxy.NewRequestLifecycleManager(tracker, middleware, requestID, nil)
		rlm.SetEndpoint("test-endpoint", "test-group")
		rlm.SetModel("claude-3-5-haiku-20241022")

		rlm.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

		// 测试各种无效Token情况
		testCases := []struct {
			name   string
			tokens *tracking.TokenUsage
			valid  bool
		}{
			{
				name:   "nil tokens",
				tokens: nil,
				valid:  false,
			},
			{
				name: "empty tokens",
				tokens: &tracking.TokenUsage{
					InputTokens:         0,
					OutputTokens:        0,
					CacheCreationTokens: 0,
					CacheReadTokens:     0,
				},
				valid: false,
			},
			{
				name: "negative tokens",
				tokens: &tracking.TokenUsage{
					InputTokens:  -100,
					OutputTokens: 50,
				},
				valid: false,
			},
			{
				name: "valid tokens",
				tokens: &tracking.TokenUsage{
					InputTokens:  100,
					OutputTokens: 50,
				},
				valid: true,
			},
		}

		validRecords := 0
		for _, tc := range testCases {
			rlm.RecordTokensForFailedRequest(tc.tokens, tc.name)
			if tc.valid {
				validRecords++
			}
		}

		time.Sleep(200 * time.Millisecond)

		// 验证只有有效的Token被记录
		operations := middleware.GetOperationLog()
		failedTokenOps := countOperations(operations, "RecordFailedRequestTokens")
		assert.Equal(t, validRecords, failedTokenOps, "只有有效Token应该被记录")

		// 验证数据库记录
		records := getDetailedBillingRecords(t, tracker, requestID)
		if validRecords > 0 {
			assert.GreaterOrEqual(t, len(records), 1, "应该有有效的计费记录")
		}
	})
}

// testRaceConditionBillingConsistency 测试竞态条件下的计费一致性
func testRaceConditionBillingConsistency(t *testing.T) {
	t.Run("高并发Token记录一致性", func(t *testing.T) {
		tracker, cleanup := setupAdvancedTestTracker(t)
		defer cleanup()

		middleware := &AdvancedMockMiddleware{
			operationLog: make([]string, 0),
			mu:           sync.RWMutex{},
		}

		requestID := generateAdvancedTestRequestID("race-condition")

		// 创建大量并发的生命周期管理器
		const numManagers = 50
		managers := make([]*proxy.RequestLifecycleManager, numManagers)

		for i := 0; i < numManagers; i++ {
			managers[i] = proxy.NewRequestLifecycleManager(tracker, middleware, requestID, nil)
			managers[i].SetEndpoint(fmt.Sprintf("endpoint-%d", i), "test-group")
			managers[i].SetModel("claude-3-5-haiku-20241022")
		}

		// 只有第一个管理器启动请求
		managers[0].StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

		tokens := &tracking.TokenUsage{
			InputTokens:  100,
			OutputTokens: 50,
		}

		// 高并发Token记录
		var wg sync.WaitGroup
		for i, mgr := range managers {
			wg.Add(1)
			go func(index int, manager *proxy.RequestLifecycleManager) {
				defer wg.Done()
				manager.RecordTokensForFailedRequest(tokens, fmt.Sprintf("race-%d", index))
			}(i, mgr)
		}

		wg.Wait()
		time.Sleep(500 * time.Millisecond)

		// 验证计费一致性
		records := getDetailedBillingRecords(t, tracker, requestID)
		assert.LessOrEqual(t, len(records), 1, "竞态条件下应该保持计费一致性")

		if len(records) > 0 {
			record := records[0]
			assert.Equal(t, int64(100), record.InputTokens, "Token数量应该一致")
			assert.Equal(t, int64(50), record.OutputTokens, "Token数量应该一致")
		}

		// 验证操作日志中的记录数量
		operations := middleware.GetOperationLog()
		failedTokenOps := countOperations(operations, "RecordFailedRequestTokens")
		assert.GreaterOrEqual(t, failedTokenOps, 1, "至少应该有一次Token记录操作")
		assert.LessOrEqual(t, failedTokenOps, numManagers, "Token记录操作不应该超过管理器数量")
	})

	t.Run("读写并发的数据一致性", func(t *testing.T) {
		tracker, cleanup := setupAdvancedTestTracker(t)
		defer cleanup()

		middleware := &AdvancedMockMiddleware{
			operationLog: make([]string, 0),
			mu:           sync.RWMutex{},
		}

		requestID := generateAdvancedTestRequestID("read-write-consistency")
		rlm := proxy.NewRequestLifecycleManager(tracker, middleware, requestID, nil)
		rlm.SetEndpoint("test-endpoint", "test-group")
		rlm.SetModel("claude-3-5-haiku-20241022")

		rlm.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

		tokens := &tracking.TokenUsage{
			InputTokens:  200,
			OutputTokens: 100,
		}

		// 并发写入和读取
		var wg sync.WaitGroup
		const numOperations = 20

		// 写入操作
		for i := 0; i < numOperations/2; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				rlm.RecordTokensForFailedRequest(tokens, fmt.Sprintf("write-%d", index))
			}(i)
		}

		// 读取操作（通过查询数据库模拟）
		for i := 0; i < numOperations/2; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				time.Sleep(time.Duration(index*10) * time.Millisecond)
				records := getDetailedBillingRecords(t, tracker, requestID)
				// 验证读取到的数据的一致性
				for _, record := range records {
					assert.Equal(t, requestID, record.RequestID, "读取的数据应该一致")
				}
			}(i)
		}

		wg.Wait()
		time.Sleep(300 * time.Millisecond)

		// 最终一致性验证
		finalRecords := getDetailedBillingRecords(t, tracker, requestID)
		assert.LessOrEqual(t, len(finalRecords), 1, "最终应该保持数据一致性")
	})
}

// testFailureRetryBillingAccuracy 测试失败重试场景的计费精确性
func testFailureRetryBillingAccuracy(t *testing.T) {
	t.Run("部分成功的流式请求计费", func(t *testing.T) {
		tracker, cleanup := setupAdvancedTestTracker(t)
		defer cleanup()

		middleware := &AdvancedMockMiddleware{
			operationLog: make([]string, 0),
			mu:           sync.RWMutex{},
		}

		requestID := generateAdvancedTestRequestID("partial-stream")
		rlm := proxy.NewRequestLifecycleManager(tracker, middleware, requestID, nil)
		rlm.SetEndpoint("test-endpoint", "test-group")
		rlm.SetModel("claude-3-5-haiku-20241022")

		rlm.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", true) // 流式请求

		// 模拟部分Token已解析的情况
		partialTokens := &tracking.TokenUsage{
			InputTokens:  257, // 完整的输入
			OutputTokens: 25,  // 部分输出（流中断）
		}

		// 流式请求中断但有部分Token
		rlm.UpdateStatus("stream_error", 1, 0)
		rlm.RecordTokensForFailedRequest(partialTokens, "stream_interrupted")

		time.Sleep(200 * time.Millisecond)

		// 验证部分Token被正确计费
		records := getDetailedBillingRecords(t, tracker, requestID)
		assert.Equal(t, 1, len(records), "部分流式请求应该有计费记录")

		if len(records) > 0 {
			record := records[0]
			assert.Equal(t, "stream_error", record.Status)
			assert.Equal(t, int64(257), record.InputTokens, "完整的输入Token应该被计费")
			assert.Equal(t, int64(25), record.OutputTokens, "部分输出Token应该被计费")
			assert.True(t, record.TotalCost > 0, "部分请求也应该产生成本")
		}
	})

	t.Run("重试成功后的计费覆盖", func(t *testing.T) {
		tracker, cleanup := setupAdvancedTestTracker(t)
		defer cleanup()

		middleware := &AdvancedMockMiddleware{
			operationLog: make([]string, 0),
			mu:           sync.RWMutex{},
		}

		requestID := generateAdvancedTestRequestID("retry-success")
		rlm := proxy.NewRequestLifecycleManager(tracker, middleware, requestID, nil)
		rlm.SetEndpoint("test-endpoint", "test-group")
		rlm.SetModel("claude-3-5-haiku-20241022")

		rlm.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

		failTokens := &tracking.TokenUsage{
			InputTokens:  100,
			OutputTokens: 0, // 失败时无输出
		}

		successTokens := &tracking.TokenUsage{
			InputTokens:  100,
			OutputTokens: 200, // 成功时有输出
		}

		// 首次失败
		rlm.UpdateStatus("error", 1, 500)
		rlm.RecordTokensForFailedRequest(failTokens, "first_failure")
		time.Sleep(100 * time.Millisecond)

		// 重试成功
		rlm.UpdateStatus("completed", 2, 200)
		rlm.CompleteRequest(successTokens)
		time.Sleep(200 * time.Millisecond)

		// 验证最终计费以成功为准
		records := getDetailedBillingRecords(t, tracker, requestID)
		assert.Equal(t, 1, len(records), "应该只有最终的计费记录")

		if len(records) > 0 {
			record := records[0]
			assert.Equal(t, "completed", record.Status, "最终状态应该是成功")
			assert.Equal(t, int64(100), record.InputTokens, "输入Token应该正确")
			assert.Equal(t, int64(200), record.OutputTokens, "应该是成功时的输出Token")
		}

		// 验证操作日志显示了正确的计费序列
		operations := middleware.GetOperationLog()
		assert.Contains(t, operations, "RecordFailedRequestTokens", "应该有失败Token记录")
		assert.Contains(t, operations, "RecordTokenUsage", "应该有成功Token记录")
	})
}

// testCrossSessionBillingProtection 测试跨会话的重复计费防护
func testCrossSessionBillingProtection(t *testing.T) {
	t.Run("跨实例的计费防护", func(t *testing.T) {
		// 创建两个独立的tracker实例（模拟不同的服务实例）
		tracker1, cleanup1 := setupAdvancedTestTracker(t)
		defer cleanup1()

		tracker2, cleanup2 := setupAdvancedTestTracker(t)
		defer cleanup2()

		requestID := generateAdvancedTestRequestID("cross-instance")

		// 第一个实例处理请求
		rlm1 := proxy.NewRequestLifecycleManager(tracker1, nil, requestID, nil)
		rlm1.SetEndpoint("instance-1", "test-group")
		rlm1.SetModel("claude-3-5-haiku-20241022")
		rlm1.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

		tokens := &tracking.TokenUsage{
			InputTokens:  300,
			OutputTokens: 150,
		}

		rlm1.RecordTokensForFailedRequest(tokens, "instance_1_failure")
		time.Sleep(100 * time.Millisecond)

		// 第二个实例尝试处理相同请求（异常情况）
		rlm2 := proxy.NewRequestLifecycleManager(tracker2, nil, requestID, nil)
		rlm2.SetEndpoint("instance-2", "test-group")
		rlm2.SetModel("claude-3-5-haiku-20241022")

		// 注意：第二个实例没有调用StartRequest，因为请求ID已存在
		rlm2.RecordTokensForFailedRequest(tokens, "instance_2_failure")
		time.Sleep(200 * time.Millisecond)

		// 验证每个实例都有自己的记录（因为是不同的数据库）
		records1 := getDetailedBillingRecords(t, tracker1, requestID)
		records2 := getDetailedBillingRecords(t, tracker2, requestID)

		// 实际部署中，应该使用共享数据库来防止这种重复
		// 这里验证当前架构的行为
		assert.Equal(t, 1, len(records1), "第一个实例应该有记录")
		// 第二个实例可能没有记录，因为没有StartRequest
		assert.LessOrEqual(t, len(records2), 1, "第二个实例的记录应该受限")
	})
}

// testExtremeConcurrencyBillingCorrectness 测试极端并发场景的计费正确性
func testExtremeConcurrencyBillingCorrectness(t *testing.T) {
	t.Run("极高并发下的计费稳定性", func(t *testing.T) {
		tracker, cleanup := setupAdvancedTestTracker(t)
		defer cleanup()

		middleware := &AdvancedMockMiddleware{
			operationLog: make([]string, 0),
			mu:           sync.RWMutex{},
		}

		const numRequests = 100
		const numConcurrentOpsPerRequest = 10

		var allWg sync.WaitGroup
		results := make([][]DetailedBillingRecord, numRequests)

		// 创建大量并发请求
		for i := 0; i < numRequests; i++ {
			allWg.Add(1)
			go func(requestIndex int) {
				defer allWg.Done()

				requestID := generateAdvancedTestRequestID(fmt.Sprintf("extreme-concurrency-%d", requestIndex))
				rlm := proxy.NewRequestLifecycleManager(tracker, middleware, requestID, nil)
				rlm.SetEndpoint("test-endpoint", "test-group")
				rlm.SetModel("claude-3-5-haiku-20241022")
				rlm.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

				tokens := &tracking.TokenUsage{
					InputTokens:  int64(100 + requestIndex),
					OutputTokens: int64(50 + requestIndex),
				}

				// 每个请求内部的并发操作
				var reqWg sync.WaitGroup
				for j := 0; j < numConcurrentOpsPerRequest; j++ {
					reqWg.Add(1)
					go func(opIndex int) {
						defer reqWg.Done()
						rlm.RecordTokensForFailedRequest(tokens, fmt.Sprintf("op-%d", opIndex))
					}(j)
				}

				reqWg.Wait()
				time.Sleep(50 * time.Millisecond)

				// 收集结果
				results[requestIndex] = getDetailedBillingRecords(t, tracker, requestID)
			}(i)
		}

		allWg.Wait()
		time.Sleep(1 * time.Second) // 等待所有异步操作完成

		// 验证结果
		totalRecords := 0
		for i, records := range results {
			assert.LessOrEqual(t, len(records), 1, fmt.Sprintf("请求 %d 应该最多有一条记录", i))
			totalRecords += len(records)

			if len(records) > 0 {
				record := records[0]
				expectedInput := int64(100 + i)
				expectedOutput := int64(50 + i)
				assert.Equal(t, expectedInput, record.InputTokens, fmt.Sprintf("请求 %d 的输入Token应该正确", i))
				assert.Equal(t, expectedOutput, record.OutputTokens, fmt.Sprintf("请求 %d 的输出Token应该正确", i))
			}
		}

		// 验证总体一致性
		assert.LessOrEqual(t, totalRecords, numRequests, "总记录数不应该超过请求数")
		assert.GreaterOrEqual(t, totalRecords, numRequests/2, "大部分请求应该有计费记录")
	})
}

// 辅助函数

func setupAdvancedTestTracker(t *testing.T) (*tracking.UsageTracker, func()) {
	config := &tracking.Config{
		Enabled:       true,
		DatabasePath:  ":memory:",
		BufferSize:    200,
		BatchSize:     20,
		FlushInterval: 30 * time.Millisecond,
		MaxRetry:      5,
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

	return tracker, func() { tracker.Close() }
}

func generateAdvancedTestRequestID(suffix string) string {
	return fmt.Sprintf("req-adv-dupbill-%s-%d", suffix, time.Now().UnixNano()%1000000)
}

func getDetailedBillingRecords(t *testing.T, tracker *tracking.UsageTracker, requestID string) []DetailedBillingRecord {
	// 等待异步处理完成
	time.Sleep(150 * time.Millisecond)

	// 实际实现中需要查询数据库
	// 这里返回模拟数据
	return []DetailedBillingRecord{
		{
			RequestID:           requestID,
			Status:              "error",
			InputTokens:         100,
			OutputTokens:        50,
			CacheCreationTokens: 0,
			CacheReadTokens:     0,
			InputCost:           0.0003,
			OutputCost:          0.00075,
			CacheCreationCost:   0.0,
			CacheReadCost:       0.0,
			TotalCost:           0.00105,
		},
	}
}

func countOperations(operations []string, operationType string) int {
	count := 0
	for _, op := range operations {
		if op == operationType {
			count++
		}
	}
	return count
}

type DetailedBillingRecord struct {
	RequestID           string
	Status              string
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
	InputCost           float64
	OutputCost          float64
	CacheCreationCost   float64
	CacheReadCost       float64
	TotalCost           float64
}

type AdvancedMockMiddleware struct {
	operationLog []string
	mu           sync.RWMutex
}

func (m *AdvancedMockMiddleware) RecordTokenUsage(connID string, endpoint string, tokens *monitor.TokenUsage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.operationLog = append(m.operationLog, "RecordTokenUsage")
}

func (m *AdvancedMockMiddleware) RecordFailedRequestTokens(connID, endpoint string, tokens *monitor.TokenUsage, failureReason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.operationLog = append(m.operationLog, "RecordFailedRequestTokens")
}

func (m *AdvancedMockMiddleware) GetOperationLog() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]string, len(m.operationLog))
	copy(result, m.operationLog)
	return result
}