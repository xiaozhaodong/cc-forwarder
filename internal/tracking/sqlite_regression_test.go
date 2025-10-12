package tracking

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSQLiteDataPersistence 测试SQLite数据持久化完整性
// 防止INSERT OR REPLACE导致的数据丢失回归
func TestSQLiteDataPersistence(t *testing.T) {
	config := &Config{
		Enabled:         true,
		DatabasePath:    ":memory:",
		BufferSize:      10,
		BatchSize:       5,
		FlushInterval:   100 * time.Millisecond,
		MaxRetry:        3,
		CleanupInterval: 24 * time.Hour,
		RetentionDays:   30,
		ModelPricing: map[string]ModelPricing{
			"claude-3-5-haiku-20241022": {
				Input:         1.00,
				Output:        5.00,
				CacheCreation: 1.25,
				CacheRead:     0.10,
			},
		},
		DefaultPricing: ModelPricing{
			Input:         2.00,
			Output:        10.00,
			CacheCreation: 2.50,
			CacheRead:     0.20,
		},
	}

	tracker, err := NewUsageTracker(config)
	require.NoError(t, err)
	defer tracker.Close()

	ctx := context.Background()
	requestID := "req-test-persistence"

	// 第1步：发送开始事件
	t.Run("Step1_StartEvent", func(t *testing.T) {
		startEvents := []RequestEvent{
			{
				Type:      "start",
				RequestID: requestID,
				Timestamp: time.Now(),
				Data: RequestStartData{
					ClientIP:    "192.168.1.100",
					UserAgent:   "TestAgent/1.0",
					Method:      "POST",
					Path:        "/v1/messages",
					IsStreaming: true,
				},
			},
		}

		err := tracker.processBatch(startEvents)
		require.NoError(t, err)

		// 强制刷新等待异步处理完成
		time.Sleep(200 * time.Millisecond)

		// 验证开始事件数据存在 - 直接查询数据库
		db := tracker.GetReadDB()
		var req RequestDetail
		err = db.QueryRowContext(ctx, `SELECT id, request_id,
			COALESCE(client_ip, '') as client_ip,
			COALESCE(user_agent, '') as user_agent,
			method, path, start_time, end_time, duration_ms,
			COALESCE(endpoint_name, '') as endpoint_name,
			COALESCE(group_name, '') as group_name,
			COALESCE(model_name, '') as model_name,
			COALESCE(is_streaming, false) as is_streaming,
			status, http_status_code, retry_count,
			input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
			input_cost_usd, output_cost_usd, cache_creation_cost_usd,
			cache_read_cost_usd, total_cost_usd,
			created_at, updated_at
			FROM request_logs WHERE request_id = ?`, requestID).Scan(
			&req.ID, &req.RequestID,
			&req.ClientIP, &req.UserAgent,
			&req.Method, &req.Path, &req.StartTime, &req.EndTime, &req.DurationMs,
			&req.EndpointName, &req.GroupName, &req.ModelName, &req.IsStreaming,
			&req.Status, &req.HTTPStatusCode, &req.RetryCount,
			&req.InputTokens, &req.OutputTokens, &req.CacheCreationTokens, &req.CacheReadTokens,
			&req.InputCostUSD, &req.OutputCostUSD, &req.CacheCreationCostUSD,
			&req.CacheReadCostUSD, &req.TotalCostUSD,
			&req.CreatedAt, &req.UpdatedAt,
		)
		require.NoError(t, err)
		assert.Equal(t, requestID, req.RequestID)
		assert.Equal(t, "192.168.1.100", req.ClientIP)
		assert.Equal(t, "TestAgent/1.0", req.UserAgent)
		assert.Equal(t, "POST", req.Method)
		assert.Equal(t, "/v1/messages", req.Path)
		assert.Equal(t, "pending", req.Status)
		assert.True(t, req.IsStreaming)

		// 这些字段应该还是空的
		assert.Empty(t, req.EndpointName)
		assert.Empty(t, req.GroupName)
		assert.Empty(t, req.ModelName)
		assert.Equal(t, int64(0), req.InputTokens)
		assert.Equal(t, int64(0), req.OutputTokens)
	})

	// 第2步：发送更新事件 - 关键测试点：不应该清空已有数据
	t.Run("Step2_UpdateEvent", func(t *testing.T) {
		updateEvents := []RequestEvent{
			{
				Type:      "update",
				RequestID: requestID,
				Timestamp: time.Now(),
				Data: RequestUpdateData{
					EndpointName: "test-endpoint-001",
					GroupName:    "primary-group",
					Status:       "processing",
					RetryCount:   1,
					HTTPStatus:   200,
				},
			},
		}

		err := tracker.processBatch(updateEvents)
		require.NoError(t, err)

		// 强制刷新等待异步处理完成
		time.Sleep(200 * time.Millisecond)

		// 验证更新事件数据存在，且原有数据未丢失 - 直接查询数据库
		db := tracker.GetReadDB()
		var req RequestDetail
		err = db.QueryRowContext(ctx, `SELECT id, request_id,
			COALESCE(client_ip, '') as client_ip,
			COALESCE(user_agent, '') as user_agent,
			method, path, start_time, end_time, duration_ms,
			COALESCE(endpoint_name, '') as endpoint_name,
			COALESCE(group_name, '') as group_name,
			COALESCE(model_name, '') as model_name,
			COALESCE(is_streaming, false) as is_streaming,
			status, http_status_code, retry_count,
			input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
			input_cost_usd, output_cost_usd, cache_creation_cost_usd,
			cache_read_cost_usd, total_cost_usd,
			created_at, updated_at
			FROM request_logs WHERE request_id = ?`, requestID).Scan(
			&req.ID, &req.RequestID,
			&req.ClientIP, &req.UserAgent,
			&req.Method, &req.Path, &req.StartTime, &req.EndTime, &req.DurationMs,
			&req.EndpointName, &req.GroupName, &req.ModelName, &req.IsStreaming,
			&req.Status, &req.HTTPStatusCode, &req.RetryCount,
			&req.InputTokens, &req.OutputTokens, &req.CacheCreationTokens, &req.CacheReadTokens,
			&req.InputCostUSD, &req.OutputCostUSD, &req.CacheCreationCostUSD,
			&req.CacheReadCostUSD, &req.TotalCostUSD,
			&req.CreatedAt, &req.UpdatedAt,
		)
		require.NoError(t, err)
		// 原有的开始事件数据应该保留
		assert.Equal(t, requestID, req.RequestID)
		assert.Equal(t, "192.168.1.100", req.ClientIP, "开始事件的ClientIP不应该丢失")
		assert.Equal(t, "TestAgent/1.0", req.UserAgent, "开始事件的UserAgent不应该丢失")
		assert.Equal(t, "POST", req.Method, "开始事件的Method不应该丢失")
		assert.Equal(t, "/v1/messages", req.Path, "开始事件的Path不应该丢失")
		assert.True(t, req.IsStreaming, "开始事件的IsStreaming不应该丢失")

		// 更新事件的新数据应该存在
		assert.Equal(t, "test-endpoint-001", req.EndpointName)
		assert.Equal(t, "primary-group", req.GroupName)
		assert.Equal(t, "processing", req.Status)
		assert.Equal(t, 1, req.RetryCount)
		assert.Equal(t, 200, req.HTTPStatusCode)

		// Token相关字段应该还是空的
		assert.Empty(t, req.ModelName)
		assert.Equal(t, int64(0), req.InputTokens)
		assert.Equal(t, int64(0), req.OutputTokens)
		assert.Equal(t, float64(0), req.TotalCostUSD)
	})

	// 第3步：发送完成事件 - 关键测试点：不应该清空前面的数据
	t.Run("Step3_CompleteEvent", func(t *testing.T) {
		endTime := time.Now()
		completeEvents := []RequestEvent{
			{
				Type:      "complete",
				RequestID: requestID,
				Timestamp: endTime,
				Data: RequestCompleteData{
					ModelName:           "claude-3-5-haiku-20241022",
					InputTokens:         1500,
					OutputTokens:        800,
					CacheCreationTokens: 200,
					CacheReadTokens:     100,
					Duration:            2500 * time.Millisecond,
				},
			},
		}

		err := tracker.processBatch(completeEvents)
		require.NoError(t, err)

		// 强制刷新等待异步处理完成
		time.Sleep(200 * time.Millisecond)

		// 验证完成事件数据存在，且所有前面的数据都保留 - 直接查询数据库
		db := tracker.GetReadDB()
		var req RequestDetail
		err = db.QueryRowContext(ctx, `SELECT id, request_id,
			COALESCE(client_ip, '') as client_ip,
			COALESCE(user_agent, '') as user_agent,
			method, path, start_time, end_time, duration_ms,
			COALESCE(endpoint_name, '') as endpoint_name,
			COALESCE(group_name, '') as group_name,
			COALESCE(model_name, '') as model_name,
			COALESCE(is_streaming, false) as is_streaming,
			status, http_status_code, retry_count,
			input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
			input_cost_usd, output_cost_usd, cache_creation_cost_usd,
			cache_read_cost_usd, total_cost_usd,
			created_at, updated_at
			FROM request_logs WHERE request_id = ?`, requestID).Scan(
			&req.ID, &req.RequestID,
			&req.ClientIP, &req.UserAgent,
			&req.Method, &req.Path, &req.StartTime, &req.EndTime, &req.DurationMs,
			&req.EndpointName, &req.GroupName, &req.ModelName, &req.IsStreaming,
			&req.Status, &req.HTTPStatusCode, &req.RetryCount,
			&req.InputTokens, &req.OutputTokens, &req.CacheCreationTokens, &req.CacheReadTokens,
			&req.InputCostUSD, &req.OutputCostUSD, &req.CacheCreationCostUSD,
			&req.CacheReadCostUSD, &req.TotalCostUSD,
			&req.CreatedAt, &req.UpdatedAt,
		)
		require.NoError(t, err)

		// ===== 开始事件数据应该保留 =====
		assert.Equal(t, requestID, req.RequestID)
		assert.Equal(t, "192.168.1.100", req.ClientIP, "开始事件的ClientIP不应该丢失")
		assert.Equal(t, "TestAgent/1.0", req.UserAgent, "开始事件的UserAgent不应该丢失")
		assert.Equal(t, "POST", req.Method, "开始事件的Method不应该丢失")
		assert.Equal(t, "/v1/messages", req.Path, "开始事件的Path不应该丢失")
		assert.True(t, req.IsStreaming, "开始事件的IsStreaming不应该丢失")

		// ===== 更新事件数据应该保留 =====
		assert.Equal(t, "test-endpoint-001", req.EndpointName, "更新事件的EndpointName不应该丢失")
		assert.Equal(t, "primary-group", req.GroupName, "更新事件的GroupName不应该丢失")
		assert.Equal(t, 1, req.RetryCount, "更新事件的RetryCount不应该丢失")
		assert.Equal(t, 200, req.HTTPStatusCode, "更新事件的HTTPStatusCode不应该丢失")

		// ===== 完成事件数据应该存在 =====
		assert.Equal(t, "claude-3-5-haiku-20241022", req.ModelName)
		assert.Equal(t, int64(1500), req.InputTokens)
		assert.Equal(t, int64(800), req.OutputTokens)
		assert.Equal(t, int64(200), req.CacheCreationTokens)
		assert.Equal(t, int64(100), req.CacheReadTokens)
		assert.Equal(t, "completed", req.Status)
		assert.NotNil(t, req.EndTime)
		assert.Greater(t, req.DurationMs, int64(2000)) // 至少2.5秒

		// 验证成本计算
		assert.Greater(t, req.TotalCostUSD, float64(0))

		// 验证成本计算逻辑（基于配置的价格）
		// Input: 1500 tokens = 0.0015M * $1.00 = $0.0015
		// Output: 800 tokens = 0.0008M * $5.00 = $0.004
		// Cache Creation: 200 tokens = 0.0002M * $1.25 = $0.00025
		// Cache Read: 100 tokens = 0.0001M * $0.10 = $0.00001
		// Total: $0.00576
		expectedCost := 0.00576
		assert.InDelta(t, expectedCost, req.TotalCostUSD, 0.0001)
	})

	// 第4步：验证数据完整性统计
	t.Run("Step4_DataIntegrityVerification", func(t *testing.T) {
		stats, err := tracker.GetDatabaseStats(ctx)
		require.NoError(t, err)

		assert.Equal(t, int64(1), stats.TotalRequests)
		assert.Greater(t, stats.TotalCostUSD, float64(0))

		// 验证没有产生重复记录 - 直接查询数据库
		db := tracker.GetReadDB()
		var count int
		err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM request_logs").Scan(&count)
		require.NoError(t, err)

		// 应该只有一条记录
		assert.Equal(t, 1, count)

		// 验证这条记录包含所有阶段的数据
		var clientIP, endpointName, modelName string
		err = db.QueryRowContext(ctx, "SELECT COALESCE(client_ip, ''), COALESCE(endpoint_name, ''), COALESCE(model_name, '') FROM request_logs WHERE request_id = ?", requestID).Scan(&clientIP, &endpointName, &modelName)
		require.NoError(t, err)

		assert.NotEmpty(t, clientIP, "应该包含开始事件数据")
		assert.NotEmpty(t, endpointName, "应该包含更新事件数据")
		assert.NotEmpty(t, modelName, "应该包含完成事件数据")
	})
}

// TestSQLiteUpsertBehavior 测试SQLite upsert行为
func TestSQLiteUpsertBehavior(t *testing.T) {
	config := &Config{
		Enabled:         true,
		DatabasePath:    ":memory:",
		BufferSize:      10,
		BatchSize:       5,
		FlushInterval:   100 * time.Millisecond,
		MaxRetry:        3,
		CleanupInterval: 24 * time.Hour,
		RetentionDays:   30,
	}

	tracker, err := NewUsageTracker(config)
	require.NoError(t, err)
	defer tracker.Close()

	// 测试adapter的BuildInsertOrReplaceQuery方法
	t.Run("TestBuildInsertOrReplaceQuery", func(t *testing.T) {
		adapter := tracker.adapter.(*SQLiteAdapter)

		// 测试包含多个字段的情况
		columns := []string{"request_id", "client_ip", "status", "updated_at"}
		values := []string{"?", "?", "?", "datetime('now')"}

		query := adapter.BuildInsertOrReplaceQuery("request_logs", columns, values)

		// 应该生成ON CONFLICT DO UPDATE语句
		assert.Contains(t, query, "INSERT INTO request_logs")
		assert.Contains(t, query, "ON CONFLICT(request_id) DO UPDATE SET")
		assert.Contains(t, query, "client_ip = EXCLUDED.client_ip")
		assert.Contains(t, query, "status = EXCLUDED.status")
		assert.NotContains(t, query, "request_id = EXCLUDED.request_id", "主键不应该在UPDATE部分")

		t.Logf("Generated query: %s", query)
	})

	// 测试只有主键的情况
	t.Run("TestBuildInsertOrReplaceQueryOnlyPrimaryKey", func(t *testing.T) {
		adapter := tracker.adapter.(*SQLiteAdapter)

		columns := []string{"request_id"}
		values := []string{"?"}

		query := adapter.BuildInsertOrReplaceQuery("request_logs", columns, values)

		// 应该生成INSERT OR IGNORE语句
		assert.Contains(t, query, "INSERT OR IGNORE INTO request_logs")
		assert.NotContains(t, query, "ON CONFLICT")

		t.Logf("Generated query for primary key only: %s", query)
	})
}