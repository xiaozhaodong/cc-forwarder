package proxy

import (
	"testing"
	"time"
)

func TestRequestLifecycleManager_NewRequestLifecycleManager(t *testing.T) {
	requestID := "test-lifecycle-123"

	rlm := NewRequestLifecycleManager(nil, nil, requestID)

	if rlm.requestID != requestID {
		t.Error("RequestID not set correctly")
	}
	if rlm.lastStatus != "pending" {
		t.Error("Initial status should be 'pending'")
	}
}

func TestRequestLifecycleManager_SettersAndGetters(t *testing.T) {
	requestID := "test-getters-303"
	rlm := NewRequestLifecycleManager(nil, nil, requestID)

	// 测试设置方法
	rlm.SetEndpoint("test-endpoint", "test-group")
	rlm.SetModel("claude-3-sonnet")

	// 测试获取方法
	if rlm.GetRequestID() != requestID {
		t.Errorf("Expected RequestID %s, got %s", requestID, rlm.GetRequestID())
	}

	if rlm.GetEndpointName() != "test-endpoint" {
		t.Errorf("Expected endpoint 'test-endpoint', got '%s'", rlm.GetEndpointName())
	}

	if rlm.GetGroupName() != "test-group" {
		t.Errorf("Expected group 'test-group', got '%s'", rlm.GetGroupName())
	}

	if rlm.modelName != "claude-3-sonnet" {
		t.Errorf("Expected model 'claude-3-sonnet', got '%s'", rlm.modelName)
	}
}

func TestRequestLifecycleManager_UpdateStatus(t *testing.T) {
	requestID := "test-update-789"
	rlm := NewRequestLifecycleManager(nil, nil, requestID)

	// 设置端点信息
	rlm.SetEndpoint("test-endpoint", "test-group")

	// 更新状态
	rlm.UpdateStatus("processing", 1, 200)

	// 验证状态更新
	if rlm.lastStatus != "processing" {
		t.Errorf("Expected status 'processing', got '%s'", rlm.lastStatus)
	}
	if rlm.retryCount != 1 {
		t.Errorf("Expected retry count 1, got %d", rlm.retryCount)
	}
}

func TestRequestLifecycleManager_Duration(t *testing.T) {
	requestID := "test-duration-404"
	rlm := NewRequestLifecycleManager(nil, nil, requestID)

	// 等待一小段时间
	time.Sleep(10 * time.Millisecond)

	duration := rlm.GetDuration()
	if duration < 10*time.Millisecond {
		t.Error("Duration should be at least 10ms")
	}

	if duration > 100*time.Millisecond {
		t.Error("Duration should not be more than 100ms")
	}
}

func TestRequestLifecycleManager_IsCompleted(t *testing.T) {
	requestID := "test-completed-505"
	rlm := NewRequestLifecycleManager(nil, nil, requestID)

	// 初始状态不应该是完成
	if rlm.IsCompleted() {
		t.Error("Initial state should not be completed")
	}

	// 更新状态为处理中
	rlm.UpdateStatus("processing", 0, 200)
	if rlm.IsCompleted() {
		t.Error("Processing state should not be completed")
	}

	// 更新状态为完成
	rlm.UpdateStatus("completed", 0, 200)
	if !rlm.IsCompleted() {
		t.Error("Completed state should be completed")
	}
}

func TestRequestLifecycleManager_GetStats(t *testing.T) {
	requestID := "test-stats-606"
	rlm := NewRequestLifecycleManager(nil, nil, requestID)

	// 设置一些状态
	rlm.SetEndpoint("stats-endpoint", "stats-group")
	rlm.SetModel("claude-3-haiku")
	rlm.UpdateStatus("processing", 2, 200)

	// 获取统计信息
	stats := rlm.GetStats()

	// 验证统计信息
	expectedFields := []string{"request_id", "endpoint", "group", "model", "status", "retry_count", "duration_ms", "start_time"}
	for _, field := range expectedFields {
		if _, exists := stats[field]; !exists {
			t.Errorf("Stats should contain field: %s", field)
		}
	}

	if stats["request_id"] != requestID {
		t.Errorf("Expected request_id %s, got %v", requestID, stats["request_id"])
	}
	if stats["endpoint"] != "stats-endpoint" {
		t.Errorf("Expected endpoint 'stats-endpoint', got %v", stats["endpoint"])
	}
	if stats["group"] != "stats-group" {
		t.Errorf("Expected group 'stats-group', got %v", stats["group"])
	}
	if stats["model"] != "claude-3-haiku" {
		t.Errorf("Expected model 'claude-3-haiku', got %v", stats["model"])
	}
	if stats["status"] != "processing" {
		t.Errorf("Expected status 'processing', got %v", stats["status"])
	}
	if stats["retry_count"] != 2 {
		t.Errorf("Expected retry_count 2, got %v", stats["retry_count"])
	}
}

func TestRequestLifecycleManager_AnalyzeResponseType(t *testing.T) {
	requestID := "test-analyze-707"
	rlm := NewRequestLifecycleManager(nil, nil, requestID)

	testCases := []struct {
		response string
		expected string
	}{
		{"", "empty_response"},
		{"Internal Server Error occurred", "error_response"},
		{"ERROR: Invalid request", "error_response"},
		{`{"data":[{"id":"gpt-4","object":"model"}]}`, "models_list"},
		{`{"config":{"server":"1.0"}}`, "config_response"},
		{`{"version":"2.1.0"}`, "config_response"},
		{"Just some plain text", "non_token_response"},
		{"Normal API response without special markers", "non_token_response"},
	}

	for _, tc := range testCases {
		result := rlm.analyzeResponseType(tc.response)
		if result != tc.expected {
			t.Errorf("For response '%s', expected '%s', got '%s'",
				tc.response, tc.expected, result)
		}
	}
}