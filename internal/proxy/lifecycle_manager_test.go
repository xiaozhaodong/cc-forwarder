package proxy

import (
	"sync"
	"testing"
	"time"
)

func TestRequestLifecycleManager_NewRequestLifecycleManager(t *testing.T) {
	requestID := "test-lifecycle-123"

	rlm := NewRequestLifecycleManager(nil, nil, requestID, nil)

	if rlm.requestID != requestID {
		t.Error("RequestID not set correctly")
	}
	if rlm.lastStatus != "pending" {
		t.Error("Initial status should be 'pending'")
	}
}

func TestRequestLifecycleManager_SettersAndGetters(t *testing.T) {
	requestID := "test-getters-303"
	rlm := NewRequestLifecycleManager(nil, nil, requestID, nil)

	// 测试设置方法
	rlm.SetEndpoint("test-endpoint", "test-group")
	rlm.SetModel("claude-3-sonnet")
	rlm.SetFinalStatusCode(404)

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

	if rlm.GetFinalStatusCode() != 404 {
		t.Errorf("Expected final status code 404, got %d", rlm.GetFinalStatusCode())
	}
}

func TestRequestLifecycleManager_UpdateStatus(t *testing.T) {
	requestID := "test-update-789"
	rlm := NewRequestLifecycleManager(nil, nil, requestID, nil)

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
	rlm := NewRequestLifecycleManager(nil, nil, requestID, nil)

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
	rlm := NewRequestLifecycleManager(nil, nil, requestID, nil)

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
	rlm := NewRequestLifecycleManager(nil, nil, requestID, nil)

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
	rlm := NewRequestLifecycleManager(nil, nil, requestID, nil)

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

// TestRequestLifecycleManager_AttemptCounter 测试尝试计数器的核心功能（语义修复）
func TestRequestLifecycleManager_AttemptCounter(t *testing.T) {
	requestID := "test-attempt-808"
	rlm := NewRequestLifecycleManager(nil, nil, requestID)

	// 验证初始状态：attemptCounter应该为0
	initialCount := rlm.GetAttemptCount()
	if initialCount != 0 {
		t.Errorf("Expected initial attempt count 0, got %d", initialCount)
	}

	// 测试第一次增加尝试计数
	count1 := rlm.IncrementAttempt()
	if count1 != 1 {
		t.Errorf("Expected first increment to return 1, got %d", count1)
	}

	// 验证GetAttemptCount返回正确值
	currentCount := rlm.GetAttemptCount()
	if currentCount != 1 {
		t.Errorf("Expected current attempt count 1, got %d", currentCount)
	}

	// 测试多次增加
	for i := 2; i <= 5; i++ {
		count := rlm.IncrementAttempt()
		if count != i {
			t.Errorf("Expected increment %d to return %d, got %d", i, i, count)
		}
	}

	// 验证最终计数
	finalCount := rlm.GetAttemptCount()
	if finalCount != 5 {
		t.Errorf("Expected final attempt count 5, got %d", finalCount)
	}
}

// TestRequestLifecycleManager_UpdateStatusWithAttemptCounter 测试UpdateStatus与尝试计数器的集成
func TestRequestLifecycleManager_UpdateStatusWithAttemptCounter(t *testing.T) {
	requestID := "test-update-attempt-909"
	rlm := NewRequestLifecycleManager(nil, nil, requestID)

	// 设置端点信息
	rlm.SetEndpoint("test-endpoint", "test-group")

	// 增加尝试计数器
	rlm.IncrementAttempt()
	rlm.IncrementAttempt()
	rlm.IncrementAttempt()

	// 测试使用retryCount=-1时，应该使用内部计数器
	rlm.UpdateStatus("suspended", -1, 0)

	// 验证状态更新使用了内部计数器的值
	if rlm.lastStatus != "suspended" {
		t.Errorf("Expected status 'suspended', got '%s'", rlm.lastStatus)
	}
	if rlm.retryCount != 3 {
		t.Errorf("Expected retry count to use internal counter (3), got %d", rlm.retryCount)
	}

	// 测试使用正常retryCount时，应该使用提供的值
	rlm.UpdateStatus("processing", 5, 200)
	if rlm.retryCount != 5 {
		t.Errorf("Expected retry count to use provided value (5), got %d", rlm.retryCount)
	}
}

// TestRequestLifecycleManager_AttemptCounterThreadSafety 测试尝试计数器的线程安全性
func TestRequestLifecycleManager_AttemptCounterThreadSafety(t *testing.T) {
	requestID := "test-threadsafe-1010"
	rlm := NewRequestLifecycleManager(nil, nil, requestID)

	const numGoroutines = 100
	const incrementsPerGoroutine = 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// 并发增加计数器
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < incrementsPerGoroutine; j++ {
				rlm.IncrementAttempt()
			}
		}()
	}

	wg.Wait()

	// 验证最终计数是否正确
	expectedCount := numGoroutines * incrementsPerGoroutine
	actualCount := rlm.GetAttemptCount()
	if actualCount != expectedCount {
		t.Errorf("Expected final count %d, got %d", expectedCount, actualCount)
	}
}

// TestRequestLifecycleManager_UpdateStatusSemanticFix 测试语义修复的完整流程
func TestRequestLifecycleManager_UpdateStatusSemanticFix(t *testing.T) {
	requestID := "test-semantic-1111"
	rlm := NewRequestLifecycleManager(nil, nil, requestID)

	rlm.SetEndpoint("endpoint1", "group1")

	// 模拟流式请求的挂起逻辑
	rlm.IncrementAttempt() // 第一次尝试
	rlm.IncrementAttempt() // 第二次尝试
	rlm.IncrementAttempt() // 第三次尝试

	// 获取真实尝试次数
	actualAttemptCount := rlm.GetAttemptCount()
	if actualAttemptCount != 3 {
		t.Errorf("Expected 3 attempts, got %d", actualAttemptCount)
	}

	// 使用 retryCount=-1 来使用内部计数器（这是修复的核心）
	rlm.UpdateStatus("suspended", -1, 0)

	// 验证数据库中记录的是真实尝试次数，不是端点数量
	if rlm.GetRetryCount() != 3 {
		t.Errorf("Expected retry count in status to be 3 (actual attempts), got %d", rlm.GetRetryCount())
	}

	// 验证状态正确设置为suspended而不是retry
	if rlm.GetLastStatus() != "suspended" {
		t.Errorf("Expected status 'suspended', got '%s'", rlm.GetLastStatus())
	}
}

// TestRequestLifecycleManager_AttemptCounterIndependentOfRetryCount 测试尝试计数器与retryCount的独立性
func TestRequestLifecycleManager_AttemptCounterIndependentOfRetryCount(t *testing.T) {
	requestID := "test-independent-1212"
	rlm := NewRequestLifecycleManager(nil, nil, requestID)

	// 增加尝试计数器
	rlm.IncrementAttempt()
	rlm.IncrementAttempt()

	// 手动设置retryCount（模拟其他路径的更新）
	rlm.UpdateStatus("processing", 5, 200)

	// 验证attemptCounter和retryCount是独立的
	attemptCount := rlm.GetAttemptCount()
	retryCount := rlm.GetRetryCount()

	if attemptCount != 2 {
		t.Errorf("Expected attempt count 2, got %d", attemptCount)
	}
	if retryCount != 5 {
		t.Errorf("Expected retry count 5, got %d", retryCount)
	}

	// 继续增加尝试计数器
	rlm.IncrementAttempt()

	// 验证attemptCounter增加了，但retryCount不变
	newAttemptCount := rlm.GetAttemptCount()
	sameRetryCount := rlm.GetRetryCount()

	if newAttemptCount != 3 {
		t.Errorf("Expected new attempt count 3, got %d", newAttemptCount)
	}
	if sameRetryCount != 5 {
		t.Errorf("Expected retry count unchanged (5), got %d", sameRetryCount)
	}
}