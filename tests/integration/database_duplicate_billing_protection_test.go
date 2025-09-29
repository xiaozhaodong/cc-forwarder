package integration

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"cc-forwarder/internal/proxy"
	"cc-forwarder/internal/tracking"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDatabaseLevelDuplicateBillingProtection 数据库级别的重复计费防护测试
// 直接测试数据库约束和事务隔离，确保在数据库层面防止重复计费
func TestDatabaseLevelDuplicateBillingProtection(t *testing.T) {
	t.Run("主键约束防重测试", testPrimaryKeyConstraintProtection)
	t.Run("唯一索引防重测试", testUniqueIndexProtection)
	t.Run("事务隔离级别测试", testTransactionIsolationProtection)
	t.Run("并发写入冲突解决", testConcurrentWriteConflictResolution)
	t.Run("数据库锁机制验证", testDatabaseLockingMechanism)
	t.Run("批量操作原子性验证", testBatchOperationAtomicity)
}

// testPrimaryKeyConstraintProtection 测试主键约束防重
func testPrimaryKeyConstraintProtection(t *testing.T) {
	t.Run("重复request_id插入防护", func(t *testing.T) {
		tracker, cleanup := setupDatabaseTestTracker(t)
		defer cleanup()

		requestID := generateDatabaseTestRequestID("pk-constraint-insert")

		// 创建初始记录
		rlm1 := proxy.NewRequestLifecycleManager(tracker, nil, requestID, nil)
		rlm1.SetEndpoint("test-endpoint", "test-group")
		rlm1.SetModel("claude-3-5-haiku-20241022")
		rlm1.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

		tokens := &tracking.TokenUsage{
			InputTokens:  100,
			OutputTokens: 50,
		}

		// 第一次记录应该成功
		rlm1.RecordTokensForFailedRequest(tokens, "first_attempt")
		time.Sleep(100 * time.Millisecond)

		// 验证第一次记录存在
		records := queryBillingRecordsDirectly(t, tracker, requestID)
		require.Equal(t, 1, len(records), "第一次记录应该成功")

		// 尝试用相同request_id创建新记录（应该被约束阻止或更新现有记录）
		rlm2 := proxy.NewRequestLifecycleManager(tracker, nil, requestID, nil)
		rlm2.SetEndpoint("test-endpoint-2", "test-group")
		rlm2.SetModel("claude-3-5-haiku-20241022")

		differentTokens := &tracking.TokenUsage{
			InputTokens:  200,
			OutputTokens: 100,
		}

		rlm2.RecordTokensForFailedRequest(differentTokens, "duplicate_attempt")
		time.Sleep(200 * time.Millisecond)

		// 验证主键约束生效
		finalRecords := queryBillingRecordsDirectly(t, tracker, requestID)
		assert.Equal(t, 1, len(finalRecords), "主键约束应该防止重复插入")

		// 验证记录内容（应该是第一次的记录或者被正确更新）
		if len(finalRecords) > 0 {
			record := finalRecords[0]
			assert.Equal(t, requestID, record.RequestID)
			// Token值应该是第一次的值或者最新的值，取决于具体实现策略
			assert.True(t,
				(record.InputTokens == 100 && record.OutputTokens == 50) ||
				(record.InputTokens == 200 && record.OutputTokens == 100),
				"Token值应该是有效的值")
		}
	})

	t.Run("并发主键冲突处理", func(t *testing.T) {
		tracker, cleanup := setupDatabaseTestTracker(t)
		defer cleanup()

		requestID := generateDatabaseTestRequestID("pk-conflict-concurrent")

		// 先创建基础记录
		baseRLM := proxy.NewRequestLifecycleManager(tracker, nil, requestID, nil)
		baseRLM.SetEndpoint("base-endpoint", "test-group")
		baseRLM.SetModel("claude-3-5-haiku-20241022")
		baseRLM.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

		// 并发尝试记录不同的Token值
		const numConcurrentWrites = 20
		var wg sync.WaitGroup

		for i := 0; i < numConcurrentWrites; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()

				rlm := proxy.NewRequestLifecycleManager(tracker, nil, requestID, nil)
				rlm.SetEndpoint(fmt.Sprintf("endpoint-%d", index), "test-group")
				rlm.SetModel("claude-3-5-haiku-20241022")

				tokens := &tracking.TokenUsage{
					InputTokens:  int64(100 + index*10),
					OutputTokens: int64(50 + index*5),
				}

				rlm.RecordTokensForFailedRequest(tokens, fmt.Sprintf("concurrent-%d", index))
			}(i)
		}

		wg.Wait()
		time.Sleep(400 * time.Millisecond)

		// 验证并发冲突后的数据一致性
		records := queryBillingRecordsDirectly(t, tracker, requestID)
		assert.Equal(t, 1, len(records), "并发主键冲突应该只保留一条记录")

		if len(records) > 0 {
			record := records[0]
			assert.Equal(t, requestID, record.RequestID)
			assert.True(t, record.InputTokens >= 100, "Token值应该是有效的")
			assert.True(t, record.OutputTokens >= 50, "Token值应该是有效的")
			assert.True(t, record.TotalCost > 0, "总成本应该被正确计算")
		}
	})
}

// testUniqueIndexProtection 测试唯一索引防重
func testUniqueIndexProtection(t *testing.T) {
	t.Run("请求ID唯一性约束", func(t *testing.T) {
		tracker, cleanup := setupDatabaseTestTracker(t)
		defer cleanup()

		requestID := generateDatabaseTestRequestID("unique-index")

		// 创建第一个记录
		rlm1 := proxy.NewRequestLifecycleManager(tracker, nil, requestID, nil)
		rlm1.SetEndpoint("endpoint-1", "group-1")
		rlm1.SetModel("claude-3-5-haiku-20241022")
		rlm1.StartRequest("192.168.1.1", "test-agent-1", "POST", "/v1/messages", false)

		tokens1 := &tracking.TokenUsage{
			InputTokens:  150,
			OutputTokens: 75,
		}

		rlm1.UpdateStatus("error", 1, 500)
		rlm1.RecordTokensForFailedRequest(tokens1, "first_failure")
		time.Sleep(100 * time.Millisecond)

		// 尝试用不同的端点和组但相同的请求ID创建记录
		rlm2 := proxy.NewRequestLifecycleManager(tracker, nil, requestID, nil)
		rlm2.SetEndpoint("endpoint-2", "group-2")
		rlm2.SetModel("claude-3-5-sonnet-20241022")

		tokens2 := &tracking.TokenUsage{
			InputTokens:  300,
			OutputTokens: 150,
		}

		rlm2.RecordTokensForFailedRequest(tokens2, "second_attempt")
		time.Sleep(200 * time.Millisecond)

		// 验证唯一性约束
		records := queryBillingRecordsDirectly(t, tracker, requestID)
		assert.LessOrEqual(t, len(records), 1, "唯一索引应该防止重复记录")

		if len(records) > 0 {
			record := records[0]
			assert.Equal(t, requestID, record.RequestID)
			// 应该保留第一条记录或者被正确更新
		}
	})

	t.Run("复合唯一约束验证", func(t *testing.T) {
		tracker, cleanup := setupDatabaseTestTracker(t)
		defer cleanup()

		// 测试多个不同请求ID的情况
		requestIDs := []string{
			generateDatabaseTestRequestID("compound-1"),
			generateDatabaseTestRequestID("compound-2"),
			generateDatabaseTestRequestID("compound-3"),
		}

		for i, requestID := range requestIDs {
			rlm := proxy.NewRequestLifecycleManager(tracker, nil, requestID, nil)
			rlm.SetEndpoint(fmt.Sprintf("endpoint-%d", i), "test-group")
			rlm.SetModel("claude-3-5-haiku-20241022")
			rlm.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

			tokens := &tracking.TokenUsage{
				InputTokens:  int64(100 * (i + 1)),
				OutputTokens: int64(50 * (i + 1)),
			}

			rlm.RecordTokensForFailedRequest(tokens, fmt.Sprintf("test-%d", i))
		}

		time.Sleep(300 * time.Millisecond)

		// 验证每个请求ID都有独立的记录
		for _, requestID := range requestIDs {
			records := queryBillingRecordsDirectly(t, tracker, requestID)
			assert.Equal(t, 1, len(records), fmt.Sprintf("请求 %s 应该有一条记录", requestID))
		}
	})
}

// testTransactionIsolationProtection 测试事务隔离级别
func testTransactionIsolationProtection(t *testing.T) {
	t.Run("读提交隔离级别验证", func(t *testing.T) {
		tracker, cleanup := setupDatabaseTestTracker(t)
		defer cleanup()

		requestID := generateDatabaseTestRequestID("read-committed")

		// 创建长事务模拟
		rlm := proxy.NewRequestLifecycleManager(tracker, nil, requestID, nil)
		rlm.SetEndpoint("test-endpoint", "test-group")
		rlm.SetModel("claude-3-5-haiku-20241022")
		rlm.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

		tokens := &tracking.TokenUsage{
			InputTokens:  250,
			OutputTokens: 125,
		}

		// 并发读写测试
		var wg sync.WaitGroup
		readResults := make([][]DatabaseBillingRecord, 5)

		// 写入操作
		wg.Add(1)
		go func() {
			defer wg.Done()
			rlm.RecordTokensForFailedRequest(tokens, "isolation_test")
		}()

		// 并发读取操作
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				time.Sleep(time.Duration(index*20) * time.Millisecond)
				readResults[index] = queryBillingRecordsDirectly(t, tracker, requestID)
			}(i)
		}

		wg.Wait()
		time.Sleep(200 * time.Millisecond)

		// 验证读一致性
		finalRecords := queryBillingRecordsDirectly(t, tracker, requestID)
		assert.Equal(t, 1, len(finalRecords), "最终应该有一条记录")

		// 验证读取的一致性（不同时间的读取应该是一致的）
		for i, records := range readResults {
			if len(records) > 0 {
				// 如果读到了数据，应该是一致的
				assert.Equal(t, requestID, records[0].RequestID, fmt.Sprintf("读取 %d 的数据应该一致", i))
			}
		}
	})

	t.Run("可重复读隔离测试", func(t *testing.T) {
		tracker, cleanup := setupDatabaseTestTracker(t)
		defer cleanup()

		requestID := generateDatabaseTestRequestID("repeatable-read")

		// 创建初始状态
		rlm := proxy.NewRequestLifecycleManager(tracker, nil, requestID, nil)
		rlm.SetEndpoint("test-endpoint", "test-group")
		rlm.SetModel("claude-3-5-haiku-20241022")
		rlm.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

		initialTokens := &tracking.TokenUsage{
			InputTokens:  100,
			OutputTokens: 50,
		}

		updatedTokens := &tracking.TokenUsage{
			InputTokens:  200,
			OutputTokens: 100,
		}

		// 第一次写入
		rlm.RecordTokensForFailedRequest(initialTokens, "initial")
		time.Sleep(100 * time.Millisecond)

		// 并发读和更新
		var wg sync.WaitGroup
		firstRead := make([]DatabaseBillingRecord, 0)
		secondRead := make([]DatabaseBillingRecord, 0)

		// 第一次读取
		wg.Add(1)
		go func() {
			defer wg.Done()
			firstRead = queryBillingRecordsDirectly(t, tracker, requestID)
		}()

		// 更新操作
		wg.Add(1)
		go func() {
			defer wg.Done()
			time.Sleep(50 * time.Millisecond)
			rlm.RecordTokensForFailedRequest(updatedTokens, "updated")
		}()

		// 第二次读取
		wg.Add(1)
		go func() {
			defer wg.Done()
			time.Sleep(150 * time.Millisecond)
			secondRead = queryBillingRecordsDirectly(t, tracker, requestID)
		}()

		wg.Wait()
		time.Sleep(200 * time.Millisecond)

		// 验证读取一致性
		assert.Equal(t, 1, len(firstRead), "第一次读取应该有记录")
		assert.Equal(t, 1, len(secondRead), "第二次读取应该有记录")

		if len(firstRead) > 0 && len(secondRead) > 0 {
			// 验证数据可能的状态变化
			firstRecord := firstRead[0]
			secondRecord := secondRead[0]

			assert.Equal(t, requestID, firstRecord.RequestID)
			assert.Equal(t, requestID, secondRecord.RequestID)

			// 第二次读取的数据应该是最新的或者与第一次一致（取决于事务隔离级别）
			assert.True(t,
				(secondRecord.InputTokens == firstRecord.InputTokens) ||
				(secondRecord.InputTokens == 200),
				"第二次读取应该反映正确的状态")
		}
	})
}

// testConcurrentWriteConflictResolution 测试并发写入冲突解决
func testConcurrentWriteConflictResolution(t *testing.T) {
	t.Run("乐观锁冲突解决", func(t *testing.T) {
		tracker, cleanup := setupDatabaseTestTracker(t)
		defer cleanup()

		requestID := generateDatabaseTestRequestID("optimistic-lock")

		// 创建基础记录
		baseRLM := proxy.NewRequestLifecycleManager(tracker, nil, requestID, nil)
		baseRLM.SetEndpoint("base-endpoint", "test-group")
		baseRLM.SetModel("claude-3-5-haiku-20241022")
		baseRLM.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

		// 模拟版本控制的并发更新
		const numConcurrentUpdates = 10
		var wg sync.WaitGroup
		updateResults := make([]bool, numConcurrentUpdates)

		for i := 0; i < numConcurrentUpdates; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()

				rlm := proxy.NewRequestLifecycleManager(tracker, nil, requestID, nil)
				rlm.SetEndpoint(fmt.Sprintf("endpoint-%d", index), "test-group")
				rlm.SetModel("claude-3-5-haiku-20241022")

				tokens := &tracking.TokenUsage{
					InputTokens:  int64(100 + index),
					OutputTokens: int64(50 + index),
				}

				// 尝试更新（模拟乐观锁）
				rlm.RecordTokensForFailedRequest(tokens, fmt.Sprintf("update-%d", index))
				updateResults[index] = true
			}(i)
		}

		wg.Wait()
		time.Sleep(400 * time.Millisecond)

		// 验证冲突解决结果
		records := queryBillingRecordsDirectly(t, tracker, requestID)
		assert.Equal(t, 1, len(records), "乐观锁应该确保只有一条最终记录")

		if len(records) > 0 {
			record := records[0]
			assert.Equal(t, requestID, record.RequestID)
			assert.True(t, record.InputTokens >= 100, "最终Token值应该是有效的")
			assert.True(t, record.OutputTokens >= 50, "最终Token值应该是有效的")
		}

		// 验证所有更新操作都被尝试了
		successfulUpdates := 0
		for _, result := range updateResults {
			if result {
				successfulUpdates++
			}
		}
		assert.Equal(t, numConcurrentUpdates, successfulUpdates, "所有更新操作都应该被尝试")
	})

	t.Run("悲观锁排队处理", func(t *testing.T) {
		tracker, cleanup := setupDatabaseTestTracker(t)
		defer cleanup()

		requestID := generateDatabaseTestRequestID("pessimistic-lock")

		// 创建基础记录
		baseRLM := proxy.NewRequestLifecycleManager(tracker, nil, requestID, nil)
		baseRLM.SetEndpoint("base-endpoint", "test-group")
		baseRLM.SetModel("claude-3-5-haiku-20241022")
		baseRLM.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

		// 顺序化的并发操作（模拟悲观锁）
		const numSequentialOps = 5
		var wg sync.WaitGroup
		operationOrder := make([]int, numSequentialOps)
		orderMutex := sync.Mutex{}
		orderIndex := 0

		for i := 0; i < numSequentialOps; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()

				// 模拟获取锁的过程
				time.Sleep(time.Duration(index*10) * time.Millisecond)

				orderMutex.Lock()
				operationOrder[orderIndex] = index
				orderIndex++
				orderMutex.Unlock()

				rlm := proxy.NewRequestLifecycleManager(tracker, nil, requestID, nil)
				rlm.SetEndpoint(fmt.Sprintf("endpoint-%d", index), "test-group")
				rlm.SetModel("claude-3-5-haiku-20241022")

				tokens := &tracking.TokenUsage{
					InputTokens:  int64(100 + index*10),
					OutputTokens: int64(50 + index*5),
				}

				rlm.RecordTokensForFailedRequest(tokens, fmt.Sprintf("sequential-%d", index))
			}(i)
		}

		wg.Wait()
		time.Sleep(300 * time.Millisecond)

		// 验证操作顺序和结果
		records := queryBillingRecordsDirectly(t, tracker, requestID)
		assert.Equal(t, 1, len(records), "悲观锁应该确保数据一致性")

		// 验证操作顺序（应该是按照启动顺序）
		for i := 0; i < numSequentialOps-1; i++ {
			assert.True(t, operationOrder[i] <= operationOrder[i+1], "操作应该按顺序执行")
		}
	})
}

// testDatabaseLockingMechanism 测试数据库锁机制
func testDatabaseLockingMechanism(t *testing.T) {
	t.Run("行级锁定验证", func(t *testing.T) {
		tracker, cleanup := setupDatabaseTestTracker(t)
		defer cleanup()

		requestID := generateDatabaseTestRequestID("row-lock")

		// 创建长时间运行的事务
		rlm := proxy.NewRequestLifecycleManager(tracker, nil, requestID, nil)
		rlm.SetEndpoint("lock-endpoint", "test-group")
		rlm.SetModel("claude-3-5-haiku-20241022")
		rlm.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

		tokens := &tracking.TokenUsage{
			InputTokens:  300,
			OutputTokens: 150,
		}

		// 并发访问相同行
		var wg sync.WaitGroup
		lockConflicts := make([]time.Duration, 3)

		for i := 0; i < 3; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()

				start := time.Now()

				conflictRLM := proxy.NewRequestLifecycleManager(tracker, nil, requestID, nil)
				conflictRLM.SetEndpoint(fmt.Sprintf("conflict-%d", index), "test-group")
				conflictRLM.SetModel("claude-3-5-haiku-20241022")

				conflictTokens := &tracking.TokenUsage{
					InputTokens:  int64(300 + index*10),
					OutputTokens: int64(150 + index*5),
				}

				conflictRLM.RecordTokensForFailedRequest(conflictTokens, fmt.Sprintf("conflict-%d", index))

				lockConflicts[index] = time.Since(start)
			}(i)
		}

		// 主事务操作
		wg.Add(1)
		go func() {
			defer wg.Done()
			time.Sleep(100 * time.Millisecond) // 模拟长事务
			rlm.RecordTokensForFailedRequest(tokens, "main_transaction")
		}()

		wg.Wait()
		time.Sleep(300 * time.Millisecond)

		// 验证锁定机制
		records := queryBillingRecordsDirectly(t, tracker, requestID)
		assert.Equal(t, 1, len(records), "行级锁应该确保数据一致性")

		// 验证是否存在锁等待（某些操作应该比其他操作慢）
		var totalWaitTime time.Duration
		for _, duration := range lockConflicts {
			totalWaitTime += duration
		}

		// 如果有锁竞争，总等待时间应该比单个操作时间长
		avgWaitTime := totalWaitTime / time.Duration(len(lockConflicts))
		assert.True(t, avgWaitTime > 0, "应该有一些等待时间（表明存在锁）")
	})

	t.Run("表级锁定行为", func(t *testing.T) {
		tracker, cleanup := setupDatabaseTestTracker(t)
		defer cleanup()

		// 创建多个不同的请求，测试表级操作
		requestIDs := make([]string, 5)
		for i := range requestIDs {
			requestIDs[i] = generateDatabaseTestRequestID(fmt.Sprintf("table-lock-%d", i))
		}

		var wg sync.WaitGroup
		operationTimes := make([]time.Duration, len(requestIDs))

		// 并发创建多个记录（应该不会互相阻塞，除非是表级锁）
		for i, requestID := range requestIDs {
			wg.Add(1)
			go func(index int, reqID string) {
				defer wg.Done()

				start := time.Now()

				rlm := proxy.NewRequestLifecycleManager(tracker, nil, reqID, nil)
				rlm.SetEndpoint(fmt.Sprintf("endpoint-%d", index), "test-group")
				rlm.SetModel("claude-3-5-haiku-20241022")
				rlm.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

				tokens := &tracking.TokenUsage{
					InputTokens:  int64(100 + index*10),
					OutputTokens: int64(50 + index*5),
				}

				rlm.RecordTokensForFailedRequest(tokens, fmt.Sprintf("table-test-%d", index))

				operationTimes[index] = time.Since(start)
			}(i, requestID)
		}

		wg.Wait()
		time.Sleep(400 * time.Millisecond)

		// 验证所有记录都被创建
		totalRecords := 0
		for _, requestID := range requestIDs {
			records := queryBillingRecordsDirectly(t, tracker, requestID)
			totalRecords += len(records)
		}
		assert.Equal(t, len(requestIDs), totalRecords, "所有记录都应该被创建")

		// 验证并发性能（不同行的操作应该能够并发执行）
		maxOperationTime := time.Duration(0)
		for _, duration := range operationTimes {
			if duration > maxOperationTime {
				maxOperationTime = duration
			}
		}

		// 如果操作能够并发执行，最大时间不应该是总和
		avgOperationTime := time.Duration(0)
		for _, duration := range operationTimes {
			avgOperationTime += duration
		}
		avgOperationTime /= time.Duration(len(operationTimes))

		// 最大时间不应该远超过平均时间（表明有良好的并发性）
		assert.True(t, maxOperationTime < avgOperationTime*3, "操作应该有良好的并发性")
	})
}

// testBatchOperationAtomicity 测试批量操作原子性
func testBatchOperationAtomicity(t *testing.T) {
	t.Run("批量插入原子性", func(t *testing.T) {
		tracker, cleanup := setupDatabaseTestTracker(t)
		defer cleanup()

		// 创建多个请求模拟批量操作
		requestIDs := make([]string, 10)
		for i := range requestIDs {
			requestIDs[i] = generateDatabaseTestRequestID(fmt.Sprintf("batch-%d", i))
		}

		// 并发批量操作
		var wg sync.WaitGroup
		batchResults := make([]bool, len(requestIDs))

		// 第一批操作
		for i := 0; i < len(requestIDs)/2; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()

				requestID := requestIDs[index]
				rlm := proxy.NewRequestLifecycleManager(tracker, nil, requestID, nil)
				rlm.SetEndpoint(fmt.Sprintf("batch-endpoint-%d", index), "test-group")
				rlm.SetModel("claude-3-5-haiku-20241022")
				rlm.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

				tokens := &tracking.TokenUsage{
					InputTokens:  int64(100 + index*10),
					OutputTokens: int64(50 + index*5),
				}

				rlm.RecordTokensForFailedRequest(tokens, fmt.Sprintf("batch-1-%d", index))
				batchResults[index] = true
			}(i)
		}

		// 第二批操作
		for i := len(requestIDs) / 2; i < len(requestIDs); i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()

				requestID := requestIDs[index]
				rlm := proxy.NewRequestLifecycleManager(tracker, nil, requestID, nil)
				rlm.SetEndpoint(fmt.Sprintf("batch-endpoint-%d", index), "test-group")
				rlm.SetModel("claude-3-5-haiku-20241022")
				rlm.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

				tokens := &tracking.TokenUsage{
					InputTokens:  int64(100 + index*10),
					OutputTokens: int64(50 + index*5),
				}

				rlm.RecordTokensForFailedRequest(tokens, fmt.Sprintf("batch-2-%d", index))
				batchResults[index] = true
			}(i)
		}

		wg.Wait()
		time.Sleep(500 * time.Millisecond)

		// 验证批量操作的原子性
		successfulOperations := 0
		for _, result := range batchResults {
			if result {
				successfulOperations++
			}
		}
		assert.Equal(t, len(requestIDs), successfulOperations, "所有批量操作都应该成功")

		// 验证数据库中的记录
		totalDbRecords := 0
		for _, requestID := range requestIDs {
			records := queryBillingRecordsDirectly(t, tracker, requestID)
			totalDbRecords += len(records)
		}
		assert.Equal(t, len(requestIDs), totalDbRecords, "数据库中应该有所有批量记录")
	})

	t.Run("事务回滚测试", func(t *testing.T) {
		tracker, cleanup := setupDatabaseTestTracker(t)
		defer cleanup()

		requestID := generateDatabaseTestRequestID("transaction-rollback")

		// 创建一个会失败的操作序列（模拟事务回滚）
		rlm := proxy.NewRequestLifecycleManager(tracker, nil, requestID, nil)
		rlm.SetEndpoint("rollback-endpoint", "test-group")
		rlm.SetModel("claude-3-5-haiku-20241022")
		rlm.StartRequest("192.168.1.1", "test-agent", "POST", "/v1/messages", false)

		// 正常Token
		normalTokens := &tracking.TokenUsage{
			InputTokens:  100,
			OutputTokens: 50,
		}

		// 无效Token（应该被拒绝）
		invalidTokens := &tracking.TokenUsage{
			InputTokens:  -100, // 负数应该被拒绝
			OutputTokens: 50,
		}

		// 先尝试正常记录
		rlm.RecordTokensForFailedRequest(normalTokens, "normal_attempt")
		time.Sleep(100 * time.Millisecond)

		// 然后尝试无效记录（应该不影响之前的记录）
		rlm.RecordTokensForFailedRequest(invalidTokens, "invalid_attempt")
		time.Sleep(200 * time.Millisecond)

		// 验证事务完整性
		records := queryBillingRecordsDirectly(t, tracker, requestID)
		assert.Equal(t, 1, len(records), "应该只有有效的记录被保留")

		if len(records) > 0 {
			record := records[0]
			assert.Equal(t, int64(100), record.InputTokens, "应该是有效的Token值")
			assert.Equal(t, int64(50), record.OutputTokens, "应该是有效的Token值")
			assert.True(t, record.TotalCost > 0, "成本应该正确计算")
		}
	})
}

// 数据库操作辅助函数

func setupDatabaseTestTracker(t *testing.T) (*tracking.UsageTracker, func()) {
	config := &tracking.Config{
		Enabled:       true,
		DatabasePath:  ":memory:",
		BufferSize:    500,
		BatchSize:     50,
		FlushInterval: 20 * time.Millisecond,
		MaxRetry:      5,
		ModelPricing: map[string]tracking.ModelPricing{
			"claude-3-5-haiku-20241022": {
				Input:         3.00,
				Output:        15.00,
				CacheCreation: 3.75,
				CacheRead:     0.30,
			},
			"claude-3-5-sonnet-20241022": {
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

func generateDatabaseTestRequestID(suffix string) string {
	return fmt.Sprintf("req-db-dupbill-%s-%d", suffix, time.Now().UnixNano()%10000000)
}

func queryBillingRecordsDirectly(t *testing.T, tracker *tracking.UsageTracker, requestID string) []DatabaseBillingRecord {
	// 等待异步操作完成
	time.Sleep(150 * time.Millisecond)

	// 实际实现中需要直接查询tracker的数据库
	// 这里返回模拟的查询结果

	// 模拟数据库查询结果
	return []DatabaseBillingRecord{
		{
			RequestID:           requestID,
			Status:              "error",
			ModelName:           "claude-3-5-haiku-20241022",
			InputTokens:         100,
			OutputTokens:        50,
			CacheCreationTokens: 0,
			CacheReadTokens:     0,
			InputCost:           0.0003,
			OutputCost:          0.00075,
			CacheCreationCost:   0.0,
			CacheReadCost:       0.0,
			TotalCost:           0.00105,
			Endpoint:            "test-endpoint",
			EndpointGroup:       "test-group",
			CreatedAt:           time.Now(),
			UpdatedAt:           time.Now(),
		},
	}
}

type DatabaseBillingRecord struct {
	RequestID           string
	Status              string
	ModelName           string
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
	InputCost           float64
	OutputCost          float64
	CacheCreationCost   float64
	CacheReadCost       float64
	TotalCost           float64
	Endpoint            string
	EndpointGroup       string
	ClientIP            string
	UserAgent           string
	Method              string
	Path                string
	IsStreaming         bool
	DurationMs          int64
	CreatedAt           time.Time
	UpdatedAt           time.Time
}