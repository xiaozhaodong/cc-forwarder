package integration

import (
	"testing"
	"time"

	"cc-forwarder/internal/middleware"
	"cc-forwarder/internal/proxy"
	"cc-forwarder/internal/tracking"
)

// TestFailedRequestTokensIntegration 集成测试：验证失败请求Token记录的完整流程
func TestFailedRequestTokensIntegration(t *testing.T) {
	// 1. 创建UsageTracker
	trackerConfig := &tracking.Config{
		Enabled:       true,
		DatabasePath:  ":memory:",
		BufferSize:    50,
		BatchSize:     5,
		FlushInterval: 100 * time.Millisecond,
		MaxRetry:      3,
		DefaultPricing: tracking.ModelPricing{
			Input:  2.0,
			Output: 10.0,
		},
	}

	tracker, err := tracking.NewUsageTracker(trackerConfig)
	if err != nil {
		t.Fatalf("创建UsageTracker失败: %v", err)
	}
	defer tracker.Close()

	// 2. 创建MonitoringMiddleware
	monitoringMiddleware := middleware.NewMonitoringMiddleware(nil)

	// 3. 创建RequestLifecycleManager
	rlm := proxy.NewRequestLifecycleManager(tracker, monitoringMiddleware, "req-integration-test", nil)

	// 4. 设置请求信息
	rlm.SetEndpoint("integration-endpoint", "integration-group")
	rlm.SetModel("claude-3-5-sonnet")

	// 5. 模拟完整的失败请求流程
	// 开始请求
	rlm.StartRequest("192.168.1.100", "integration-test-client", "POST", "/v1/messages", false)

	// 更新状态为处理中
	rlm.UpdateStatus("processing", 0, 200)

	// 模拟处理过程中收到一些Token然后失败
	tokens := &tracking.TokenUsage{
		InputTokens:         200,
		OutputTokens:        50,
		CacheCreationTokens: 30,
		CacheReadTokens:     10,
	}

	// 6. 记录失败请求的Token
	rlm.RecordTokensForFailedRequest(tokens, "connection_timeout")

	// 7. 等待异步处理完成
	time.Sleep(300 * time.Millisecond)

	// 8. 验证结果
	// 验证监控指标已更新
	metrics := monitoringMiddleware.GetMetrics()
	if metrics.TotalTokenUsage.InputTokens != 200 {
		t.Errorf("期望监控InputTokens 200，实际 %d", metrics.TotalTokenUsage.InputTokens)
	}
	if metrics.TotalTokenUsage.OutputTokens != 50 {
		t.Errorf("期望监控OutputTokens 50，实际 %d", metrics.TotalTokenUsage.OutputTokens)
	}

	// 验证请求状态保持一致（未被失败Token记录影响）
	if rlm.IsCompleted() {
		t.Error("记录失败Token不应改变请求完成状态")
	}
	if rlm.GetLastStatus() != "processing" {
		t.Errorf("期望状态保持'processing'，实际 '%s'", rlm.GetLastStatus())
	}

	// 验证请求统计信息
	stats := rlm.GetStats()
	if stats["request_id"] != "req-integration-test" {
		t.Errorf("请求ID不匹配：期望 'req-integration-test'，实际 '%v'", stats["request_id"])
	}
	if stats["endpoint"] != "integration-endpoint" {
		t.Errorf("端点不匹配：期望 'integration-endpoint'，实际 '%v'", stats["endpoint"])
	}

	t.Log("集成测试成功：所有组件正常协作，失败Token记录功能工作正常")
}

// TestFailedRequestTokensEdgeCases 边界情况集成测试
func TestFailedRequestTokensEdgeCases(t *testing.T) {
	trackerConfig := &tracking.Config{
		Enabled:       true,
		DatabasePath:  ":memory:",
		BufferSize:    20,
		BatchSize:     3,
		FlushInterval: 50 * time.Millisecond,
		MaxRetry:      2,
		DefaultPricing: tracking.ModelPricing{Input: 1.0, Output: 5.0},
	}

	tracker, err := tracking.NewUsageTracker(trackerConfig)
	if err != nil {
		t.Fatalf("创建UsageTracker失败: %v", err)
	}
	defer tracker.Close()

	monitoringMiddleware := middleware.NewMonitoringMiddleware(nil)
	rlm := proxy.NewRequestLifecycleManager(tracker, monitoringMiddleware, "req-edge-case", nil)

	// 测试用例1: 空Token不会被处理
	rlm.RecordTokensForFailedRequest(nil, "nil_tokens")
	time.Sleep(100 * time.Millisecond)

	metrics1 := monitoringMiddleware.GetMetrics()
	if metrics1.TotalTokenUsage.InputTokens != 0 {
		t.Error("空Token不应被记录到监控指标")
	}

	// 测试用例2: 零值Token不会被处理
	zeroTokens := &tracking.TokenUsage{
		InputTokens:         0,
		OutputTokens:        0,
		CacheCreationTokens: 0,
		CacheReadTokens:     0,
	}
	rlm.RecordTokensForFailedRequest(zeroTokens, "zero_tokens")
	time.Sleep(100 * time.Millisecond)

	metrics2 := monitoringMiddleware.GetMetrics()
	if metrics2.TotalTokenUsage.InputTokens != 0 {
		t.Error("零值Token不应被记录到监控指标")
	}

	// 测试用例3: 有效Token会被正确处理
	validTokens := &tracking.TokenUsage{
		InputTokens:  100,
		OutputTokens: 30,
	}
	rlm.RecordTokensForFailedRequest(validTokens, "valid_tokens")
	time.Sleep(100 * time.Millisecond)

	metrics3 := monitoringMiddleware.GetMetrics()
	if metrics3.TotalTokenUsage.InputTokens != 100 {
		t.Errorf("期望有效Token被记录：InputTokens 100，实际 %d", metrics3.TotalTokenUsage.InputTokens)
	}
	if metrics3.TotalTokenUsage.OutputTokens != 30 {
		t.Errorf("期望有效Token被记录：OutputTokens 30，实际 %d", metrics3.TotalTokenUsage.OutputTokens)
	}

	t.Log("边界情况集成测试成功：空Token和零值Token被正确跳过，有效Token被正确处理")
}

// TestFailedRequestTokensConcurrency 并发集成测试
func TestFailedRequestTokensConcurrency(t *testing.T) {
	trackerConfig := &tracking.Config{
		Enabled:       true,
		DatabasePath:  ":memory:",
		BufferSize:    100,
		BatchSize:     10,
		FlushInterval: 200 * time.Millisecond,
		MaxRetry:      3,
		DefaultPricing: tracking.ModelPricing{Input: 1.0, Output: 2.0},
	}

	tracker, err := tracking.NewUsageTracker(trackerConfig)
	if err != nil {
		t.Fatalf("创建UsageTracker失败: %v", err)
	}
	defer tracker.Close()

	monitoringMiddleware := middleware.NewMonitoringMiddleware(nil)

	// 创建多个RequestLifecycleManager并发记录失败Token
	numManagers := 5
	tokensPerManager := 10

	for i := 0; i < numManagers; i++ {
		go func(managerID int) {
			rlm := proxy.NewRequestLifecycleManager(tracker, monitoringMiddleware,
				"req-concurrent-"+string(rune('A'+managerID)), nil)

			rlm.SetEndpoint("concurrent-endpoint-"+string(rune('0'+managerID)), "concurrent-group")

			for j := 0; j < tokensPerManager; j++ {
				tokens := &tracking.TokenUsage{
					InputTokens:  int64(10 + j),
					OutputTokens: int64(5 + j),
				}
				rlm.RecordTokensForFailedRequest(tokens, "concurrent_test")
			}
		}(i)
	}

	// 等待所有goroutine完成
	time.Sleep(1 * time.Second)

	// 验证并发处理结果
	metrics := monitoringMiddleware.GetMetrics()

	// 计算期望的总Token数
	// 每个manager: 10次调用，每次(10+j)+(5+j) = 15+2j
	// j从0到9，总计每个manager: 15*10 + 2*(0+1+...+9) = 150 + 2*45 = 240
	expectedTotal := int64(numManagers * 240) // 5 * 240 = 1200

	actualTotal := metrics.TotalTokenUsage.InputTokens + metrics.TotalTokenUsage.OutputTokens
	if actualTotal != expectedTotal {
		t.Errorf("并发测试Token统计不正确：期望 %d，实际 %d", expectedTotal, actualTotal)
	}

	t.Logf("并发集成测试成功：%d个管理器并发处理，总Token数正确 (%d)", numManagers, actualTotal)
}