package tracking

import (
	"context"
	"testing"
	"time"
)

// TestPhase2_UpdateOptions 测试UpdateOptions新字段功能
func TestPhase2_UpdateOptions(t *testing.T) {
	config := &Config{
		Enabled:         true,
		DatabasePath:    ":memory:", // 使用内存数据库
		BufferSize:      100,
		BatchSize:       10,
		FlushInterval:   1 * time.Second,
		MaxRetry:        3,
		RetentionDays:   7,
		CleanupInterval: 24 * time.Hour,
		ModelPricing: map[string]ModelPricing{
			"gpt-4": {
				Input:  0.03,
				Output: 0.06,
			},
		},
		DefaultPricing: ModelPricing{
			Input:  0.01,
			Output: 0.02,
		},
	}

	tracker, err := NewUsageTracker(config)
	if err != nil {
		t.Fatalf("Failed to create usage tracker: %v", err)
	}
	defer tracker.Close()

	requestID := "test-req-001"

	// 测试失败原因更新（中间过程）
	failureReason := "rate_limited"
	opts := UpdateOptions{
		FailureReason: &failureReason,
	}
	tracker.RecordRequestUpdate(requestID, opts)

	// 测试取消完成（最终状态）
	tracker.RecordRequestFinalFailure(requestID, "cancelled", "client disconnected", "Connection closed by client", 0, 499, nil)

	// 等待一点时间让事件处理完成
	time.Sleep(100 * time.Millisecond)

	// 验证数据库是否能正常工作
	ctx := context.Background()
	err = tracker.HealthCheck(ctx)
	if err != nil {
		t.Errorf("Health check failed after unified update methods: %v", err)
	}

	t.Log("✅ 统一数据库更新架构测试通过")
}

// TestPhase2_UpdateOptionsFields 测试UpdateOptions所有字段
func TestPhase2_UpdateOptionsFields(t *testing.T) {
	// 创建包含所有字段的UpdateOptions
	endpoint := "test-endpoint"
	group := "test-group"
	status := "processing"
	retryCount := 1
	httpStatus := 200
	model := "gpt-4"
	endTime := time.Now()
	duration := 5 * time.Second
	failureReason := "rate_limited"

	opts := UpdateOptions{
		EndpointName:  &endpoint,
		GroupName:     &group,
		Status:        &status,
		RetryCount:    &retryCount,
		HttpStatus:    &httpStatus,
		ModelName:     &model,
		EndTime:       &endTime,
		Duration:      &duration,
		FailureReason: &failureReason,
	}

	// 验证字段存在且正确
	if *opts.FailureReason != "rate_limited" {
		t.Errorf("Expected FailureReason='rate_limited', got '%s'", *opts.FailureReason)
	}

	if *opts.EndpointName != "test-endpoint" {
		t.Errorf("Expected EndpointName='test-endpoint', got '%s'", *opts.EndpointName)
	}

	t.Log("✅ UpdateOptions字段测试通过")
}

// TestPhase2_FlexibleUpdateQuery 测试灵活更新查询构建
func TestPhase2_FlexibleUpdateQuery(t *testing.T) {
	config := &Config{
		Enabled:         true,
		DatabasePath:    ":memory:",
		BufferSize:      10,
		BatchSize:       5,
		FlushInterval:   500 * time.Millisecond,
		MaxRetry:        1,
		RetentionDays:   7,
		CleanupInterval: 24 * time.Hour,
	}

	tracker, err := NewUsageTracker(config)
	if err != nil {
		t.Fatalf("Failed to create usage tracker: %v", err)
	}
	defer tracker.Close()

	// 测试构建flexible_update事件查询
	requestID := "test-req-002"
	failureReason := "server_error"
	status := "retry"

	updateEvent := RequestEvent{
		Type:      "flexible_update",
		RequestID: requestID,
		Timestamp: time.Now(),
		Data: UpdateOptions{
			Status:        &status,
			FailureReason: &failureReason,
		},
	}

	query, args, err := tracker.buildWriteQuery(updateEvent)
	if err != nil {
		t.Errorf("Failed to build flexible_update query: %v", err)
	}
	if query == "" {
		t.Error("Expected non-empty query for flexible_update event")
	}
	// 应该有3个参数：status, failure_reason, request_id
	if len(args) != 3 {
		t.Errorf("Expected 3 args for flexible_update query, got %d", len(args))
	}

	t.Log("✅ 灵活更新查询构建测试通过")
}