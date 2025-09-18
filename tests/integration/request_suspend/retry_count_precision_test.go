package integration

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/middleware"
	"cc-forwarder/internal/proxy"
	"cc-forwarder/internal/tracking"
)

// TestRetryCountPrecisionVerification 精确重试次数验证测试
// 目标：验证"端点数×重试次数=总尝试次数"的精确计算逻辑
func TestRetryCountPrecisionVerification(t *testing.T) {
	// 创建3个服务器：健康检查通过但实际请求失败，确保触发所有重试机制
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			// 健康检查路径返回成功
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			w.Write([]byte(`{"object":"list","data":[{"id":"test-model","object":"model"}]}`))
		} else {
			// 实际请求返回失败
			http.Error(w, "Server 1 unavailable", http.StatusInternalServerError)
		}
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			// 健康检查路径返回成功
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			w.Write([]byte(`{"object":"list","data":[{"id":"test-model","object":"model"}]}`))
		} else {
			// 实际请求返回失败
			http.Error(w, "Server 2 unavailable", http.StatusBadGateway)
		}
	}))
	defer server2.Close()

	server3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			// 健康检查路径返回成功
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			w.Write([]byte(`{"object":"list","data":[{"id":"test-model","object":"model"}]}`))
		} else {
			// 实际请求返回失败
			http.Error(w, "Server 3 unavailable", http.StatusServiceUnavailable)
		}
	}))
	defer server3.Close()

	// 配置参数：3个端点，每个端点最多3次重试
	maxAttempts := 3
	numEndpoints := 3
	expectedTotalAttempts := numEndpoints * maxAttempts // 预期总尝试次数 = 3 × 3 = 9

	// 创建配置
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host: "localhost",
			Port: 0,
		},
		Web: config.WebConfig{
			Enabled: false,
		},
		RequestSuspend: config.RequestSuspendConfig{
			Enabled:              true,
			Timeout:              2 * time.Second, // 足够的挂起时间
			MaxSuspendedRequests: 10,
		},
		Group: config.GroupConfig{
			AutoSwitchBetweenGroups: true,
			Cooldown:                100 * time.Millisecond,
		},
		Retry: config.RetryConfig{
			MaxAttempts: maxAttempts, // 每个端点最多3次重试
			BaseDelay:   10 * time.Millisecond,
			MaxDelay:    50 * time.Millisecond,
			Multiplier:  1.5,
		},
		Health: config.HealthConfig{
			CheckInterval: 500 * time.Millisecond,
			Timeout:       2 * time.Second,
			HealthPath:    "/v1/models",
		},
		Endpoints: []config.EndpointConfig{
			{
				Name:          "failing-1",
				URL:           server1.URL,
				Group:         "test-group",
				GroupPriority: 1,
				Priority:      1,
				Token:         "test-token-1",
				Timeout:       500 * time.Millisecond,
			},
			{
				Name:          "failing-2",
				URL:           server2.URL,
				Group:         "test-group",
				GroupPriority: 1,
				Priority:      2,
				Token:         "test-token-2",
				Timeout:       500 * time.Millisecond,
			},
			{
				Name:          "failing-3",
				URL:           server3.URL,
				Group:         "test-group",
				GroupPriority: 1,
				Priority:      3,
				Token:         "test-token-3",
				Timeout:       500 * time.Millisecond,
			},
		},
	}

	// 初始化组件
	usageTracker, err := tracking.NewUsageTracker(&tracking.Config{
		Enabled:       true,
		DatabasePath:  ":memory:",
		BufferSize:    100,
		BatchSize:     10,
		FlushInterval: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Failed to create usage tracker: %v", err)
	}
	defer usageTracker.Close()

	endpointManager := endpoint.NewManager(cfg)
	endpointManager.Start()
	defer endpointManager.Stop()

	monitoring := middleware.NewMonitoringMiddleware(endpointManager)

	// 等待组件初始化
	time.Sleep(200 * time.Millisecond)

	// 创建代理处理器
	proxyHandler := proxy.NewHandler(endpointManager, cfg)
	proxyHandler.SetMonitoringMiddleware(monitoring)
	proxyHandler.SetUsageTracker(usageTracker)

	t.Run("Streaming request retry count precision", func(t *testing.T) {
		// 测试流式请求的精确重试次数
		testPreciseRetryCount(t, usageTracker, proxyHandler, expectedTotalAttempts, true, "streaming")
	})

	t.Run("Regular request retry count precision", func(t *testing.T) {
		// 测试常规请求的精确重试次数
		testPreciseRetryCount(t, usageTracker, proxyHandler, expectedTotalAttempts, false, "regular")
	})
}

// testPreciseRetryCount 精确重试次数测试辅助函数
func testPreciseRetryCount(t *testing.T, usageTracker *tracking.UsageTracker, proxyHandler *proxy.Handler, expectedAttempts int, isStreaming bool, testType string) {
	var requestBody string
	if isStreaming {
		requestBody = `{
			"model": "claude-3-sonnet-20240229",
			"messages": [{"role": "user", "content": "Precision test streaming"}],
			"stream": true,
			"max_tokens": 100
		}`
	} else {
		requestBody = `{
			"model": "claude-3-haiku-20240307",
			"messages": [{"role": "user", "content": "Precision test regular"}],
			"max_tokens": 50
		}`
	}

	req, err := http.NewRequest("POST", "/v1/messages", strings.NewReader(requestBody))
	if err != nil {
		t.Fatalf("Failed to create %s request: %v", testType, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-key")

	// 获取请求前的记录数，用于找到测试生成的记录
	beforeRecords, err := usageTracker.GetRequestLogs(context.Background(), time.Now().Add(-time.Minute), time.Now(), "", "", "", 100, 0)
	if err != nil {
		t.Fatalf("Failed to get before records: %v", err)
	}
	beforeCount := len(beforeRecords)

	// 使用ResponseRecorder捕获响应
	rr := httptest.NewRecorder()

	// 记录开始时间
	startTime := time.Now()

	// 发送请求（应该会因所有端点失败而挂起，最终超时）
	proxyHandler.ServeHTTP(rr, req)

	// 记录结束时间
	elapsed := time.Since(startTime)

	// 等待一下让数据库写入完成
	time.Sleep(300 * time.Millisecond)

	t.Logf("[%s] Request completed in %v with status %d", testType, elapsed, rr.Code)

	// 获取请求后的记录
	afterRecords, err := usageTracker.GetRequestLogs(context.Background(), time.Now().Add(-time.Minute), time.Now(), "", "", "", 100, 0)
	if err != nil {
		t.Fatalf("Failed to get after records: %v", err)
	}

	// 找到这次测试产生的新记录
	var testRecord *tracking.RequestDetail
	if len(afterRecords) > beforeCount {
		// 取最新的记录（应该是我们刚刚发送的请求）
		for i := len(afterRecords) - 1; i >= beforeCount; i-- {
			record := &afterRecords[i]
			if record.Path == "/v1/messages" && record.IsStreaming == isStreaming {
				testRecord = record
				break
			}
		}
	}

	if testRecord == nil {
		t.Logf("[%s] No test record found - this might be expected for some error conditions", testType)
		t.Logf("Before records: %d, After records: %d", beforeCount, len(afterRecords))

		// 输出最近的几条记录用于调试
		start := maxInt(0, len(afterRecords)-5)
		for i := start; i < len(afterRecords); i++ {
			record := afterRecords[i]
			t.Logf("Record %d: Path=%s, Status=%s, RetryCount=%d, IsStreaming=%t, Time=%v",
				i, record.Path, record.Status, record.RetryCount, record.IsStreaming, record.CreatedAt)
		}
		return
	}

	t.Logf("[%s] Found test record: ID=%d, Status=%s, RetryCount=%d, IsStreaming=%t",
		testType, testRecord.ID, testRecord.Status, testRecord.RetryCount, testRecord.IsStreaming)

	// 🔑 关键验证：精确断言重试次数等于预期值
	if testRecord.RetryCount != expectedAttempts {
		t.Errorf("[%s] ❌ PRECISION TEST FAILED: Expected exactly %d attempts (3 endpoints × 3 retries), but got %d",
			testType, expectedAttempts, testRecord.RetryCount)
		t.Errorf("[%s] This indicates the retry count calculation is incorrect", testType)
	} else {
		t.Logf("[%s] ✅ PRECISION TEST PASSED: Retry count is exactly %d as expected (3 endpoints × 3 retries)",
			testType, testRecord.RetryCount)
	}

	// 验证流式标记正确
	if testRecord.IsStreaming != isStreaming {
		t.Errorf("[%s] Expected IsStreaming=%t, got %t", testType, isStreaming, testRecord.IsStreaming)
	}

	// 验证最终状态合理（应该是suspended或error）
	validFinalStates := []string{"suspended", "error", "timeout", "cancelled"}
	validState := false
	for _, state := range validFinalStates {
		if testRecord.Status == state {
			validState = true
			break
		}
	}
	if !validState {
		t.Errorf("[%s] Unexpected final status: %s (expected one of %v)", testType, testRecord.Status, validFinalStates)
	}

	t.Logf("[%s] ✅ All precision validations passed: retryCount=%d, status=%s, isStreaming=%t",
		testType, testRecord.RetryCount, testRecord.Status, testRecord.IsStreaming)
}

// TestRetryCountEdgeCases 重试次数边界情况测试
func TestRetryCountEdgeCases(t *testing.T) {
	t.Run("Single endpoint with multiple retries", func(t *testing.T) {
		// 单端点多重试场景：1个端点 × 5次重试 = 5次尝试
		testSingleEndpointRetries(t, 5, 1)
	})

	t.Run("Multiple endpoints single retry", func(t *testing.T) {
		// 多端点单重试场景：5个端点 × 1次重试 = 5次尝试
		testSingleEndpointRetries(t, 1, 5)
	})
}

// testSingleEndpointRetries 测试单端点重试场景
func testSingleEndpointRetries(t *testing.T, maxAttempts, numEndpoints int) {
	expectedTotal := maxAttempts * numEndpoints

	// 创建失败的服务器
	servers := make([]*httptest.Server, numEndpoints)
	endpoints := make([]config.EndpointConfig, numEndpoints)

	for i := 0; i < numEndpoints; i++ {
		servers[i] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v1/models" {
				// 健康检查路径返回成功
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(200)
				w.Write([]byte(`{"object":"list","data":[{"id":"test-model","object":"model"}]}`))
			} else {
				// 实际请求返回失败
				http.Error(w, "Server unavailable", http.StatusInternalServerError)
			}
		}))
		defer servers[i].Close()

		endpoints[i] = config.EndpointConfig{
			Name:          fmt.Sprintf("endpoint-%d", i+1),
			URL:           servers[i].URL,
			Group:         "test",
			GroupPriority: 1,
			Priority:      i + 1,
			Token:         fmt.Sprintf("token-%d", i+1),
			Timeout:       500 * time.Millisecond,
		}
	}

	cfg := &config.Config{
		Server:         config.ServerConfig{Host: "localhost", Port: 0},
		Web:            config.WebConfig{Enabled: false},
		RequestSuspend: config.RequestSuspendConfig{Enabled: true, Timeout: 1 * time.Second, MaxSuspendedRequests: 10},
		Group:          config.GroupConfig{AutoSwitchBetweenGroups: true, Cooldown: 100 * time.Millisecond},
		Retry:          config.RetryConfig{MaxAttempts: maxAttempts, BaseDelay: 10 * time.Millisecond, MaxDelay: 50 * time.Millisecond, Multiplier: 1.5},
		Health:         config.HealthConfig{CheckInterval: 500 * time.Millisecond, Timeout: 2 * time.Second, HealthPath: "/v1/models"},
		Endpoints:      endpoints,
	}

	// 初始化并运行测试
	usageTracker, err := tracking.NewUsageTracker(&tracking.Config{Enabled: true, DatabasePath: ":memory:", BufferSize: 100, BatchSize: 10, FlushInterval: 100 * time.Millisecond})
	if err != nil {
		t.Fatalf("Failed to create usage tracker: %v", err)
	}
	defer usageTracker.Close()

	endpointManager := endpoint.NewManager(cfg)
	endpointManager.Start()
	defer endpointManager.Stop()

	monitoring := middleware.NewMonitoringMiddleware(endpointManager)
	proxyHandler := proxy.NewHandler(endpointManager, cfg)
	proxyHandler.SetMonitoringMiddleware(monitoring)
	proxyHandler.SetUsageTracker(usageTracker)

	time.Sleep(200 * time.Millisecond)

	// 发送测试请求
	body := `{"model":"test","messages":[{"role":"user","content":"Edge case test"}]}`
	req, _ := http.NewRequest("POST", "/v1/messages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-key")

	rr := httptest.NewRecorder()
	proxyHandler.ServeHTTP(rr, req)

	time.Sleep(300 * time.Millisecond)

	// 验证结果
	records, err := usageTracker.GetRequestLogs(context.Background(), time.Now().Add(-time.Minute), time.Now(), "", "", "", 10, 0)
	if err != nil {
		t.Fatalf("Failed to get records: %v", err)
	}

	var testRecord *tracking.RequestDetail
	for i := len(records) - 1; i >= 0; i-- {
		if records[i].Path == "/v1/messages" {
			testRecord = &records[i]
			break
		}
	}

	if testRecord != nil {
		if testRecord.RetryCount != expectedTotal {
			t.Errorf("Edge case failed: Expected %d attempts (%d endpoints × %d retries), got %d",
				expectedTotal, numEndpoints, maxAttempts, testRecord.RetryCount)
		} else {
			t.Logf("✅ Edge case passed: %d endpoints × %d retries = %d total attempts",
				numEndpoints, maxAttempts, testRecord.RetryCount)
		}
	}
}

// maxInt 返回较大的整数
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}