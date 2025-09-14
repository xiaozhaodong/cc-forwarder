package integration

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/events"
	"cc-forwarder/internal/middleware"
	"cc-forwarder/internal/proxy"
	"cc-forwarder/internal/tracking"
	"cc-forwarder/internal/web"
)

// PerformanceTestMetrics 性能测试指标
type PerformanceTestMetrics struct {
	TotalRequests     int64
	SuccessfulRequest int64
	FailedRequests    int64
	TotalLatency      time.Duration
	MinLatency        time.Duration
	MaxLatency        time.Duration
	StartTime         time.Time
	EndTime           time.Time
	MemoryStart       runtime.MemStats
	MemoryEnd         runtime.MemStats
}

// CalculateStats 计算统计数据
func (m *PerformanceTestMetrics) CalculateStats() map[string]interface{} {
	duration := m.EndTime.Sub(m.StartTime)
	var avgLatency time.Duration
	if m.TotalRequests > 0 {
		avgLatency = m.TotalLatency / time.Duration(m.TotalRequests)
	}

	return map[string]interface{}{
		"total_requests":     m.TotalRequests,
		"successful_requests": m.SuccessfulRequest,
		"failed_requests":    m.FailedRequests,
		"success_rate":       float64(m.SuccessfulRequest) / float64(m.TotalRequests) * 100,
		"duration_seconds":   duration.Seconds(),
		"qps":               float64(m.TotalRequests) / duration.Seconds(),
		"avg_latency_ms":    float64(avgLatency.Nanoseconds()) / 1e6,
		"min_latency_ms":    float64(m.MinLatency.Nanoseconds()) / 1e6,
		"max_latency_ms":    float64(m.MaxLatency.Nanoseconds()) / 1e6,
		"memory_alloc_mb":   float64(m.MemoryEnd.Alloc-m.MemoryStart.Alloc) / 1024 / 1024,
		"heap_objects":      int64(m.MemoryEnd.HeapObjects) - int64(m.MemoryStart.HeapObjects),
	}
}

// PerformanceTestSuite 性能测试套件
type PerformanceTestSuite struct {
	servers         map[string]*httptest.Server
	config          *config.Config
	endpointManager *endpoint.Manager
	proxyHandler    *proxy.Handler
	monitoring      *middleware.MonitoringMiddleware
	webServer       *web.WebServer
	requestCounts   map[string]*int64 // 用于跟踪各服务器请求数
}

// NewPerformanceTestSuite 创建性能测试套件
func NewPerformanceTestSuite() *PerformanceTestSuite {
	return &PerformanceTestSuite{
		servers:       make(map[string]*httptest.Server),
		requestCounts: make(map[string]*int64),
	}
}

// SetupPerformanceServers 设置性能测试服务器
func (suite *PerformanceTestSuite) SetupPerformanceServers() {
	// 快速响应服务器
	fastCount := int64(0)
	suite.requestCounts["fast"] = &fastCount
	suite.servers["fast"] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&fastCount, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		if strings.Contains(r.URL.Path, "/v1/models") {
			w.Write([]byte(`{"object":"list","data":[{"id":"gpt-3.5-turbo","object":"model"}]}`))
		} else {
			w.Write([]byte(`{"id":"fast","object":"chat.completion","choices":[{"message":{"content":"Fast response"}}]}`))
		}
	}))

	// 中等延迟服务器
	mediumCount := int64(0)
	suite.requestCounts["medium"] = &mediumCount
	suite.servers["medium"] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&mediumCount, 1)
		time.Sleep(50 * time.Millisecond) // 50ms延迟
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		if strings.Contains(r.URL.Path, "/v1/models") {
			w.Write([]byte(`{"object":"list","data":[{"id":"gpt-3.5-turbo","object":"model"}]}`))
		} else {
			w.Write([]byte(`{"id":"medium","object":"chat.completion","choices":[{"message":{"content":"Medium response"}}]}`))
		}
	}))

	// 故障服务器（用于测试故障转移性能）
	failureCount := int64(0)
	suite.requestCounts["failure"] = &failureCount
	suite.servers["failure"] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&failureCount, 1)
		http.Error(w, "Simulated failure", http.StatusInternalServerError)
	}))

	// 负载均衡测试服务器组
	for i := 1; i <= 3; i++ {
		name := fmt.Sprintf("balance%d", i)
		count := int64(0)
		suite.requestCounts[name] = &count
		
		suite.servers[name] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&count, 1)
			time.Sleep(10 * time.Millisecond) // 模拟处理时间
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			w.Write([]byte(fmt.Sprintf(`{"id":"%s","object":"chat.completion","choices":[{"message":{"content":"Response from %s"}}]}`, name, name)))
		}))
	}

	// 大响应数据服务器
	bigResponseCount := int64(0)
	suite.requestCounts["big_response"] = &bigResponseCount
	suite.servers["big_response"] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&bigResponseCount, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		
		// 生成大响应（模拟长文本生成）
		largeContent := strings.Repeat("This is a test response with a lot of content. ", 100)
		response := fmt.Sprintf(`{"id":"big","object":"chat.completion","choices":[{"message":{"content":"%s"}}]}`, largeContent)
		w.Write([]byte(response))
	}))
}

// CreatePerformanceConfig 创建性能测试配置
func (suite *PerformanceTestSuite) CreatePerformanceConfig() {
	suite.config = &config.Config{
		Server: config.ServerConfig{
			Host: "localhost",
			Port: 0,
		},
		RequestSuspend: config.RequestSuspendConfig{
			Enabled:              true,
			Timeout:              30 * time.Second,
			MaxSuspendedRequests: 1000, // 大容量用于性能测试
		},
		Group: config.GroupConfig{
			AutoSwitchBetweenGroups: true, // 自动模式用于性能测试
			Cooldown:                2 * time.Second,
		},
		Retry: config.RetryConfig{
			MaxAttempts: 2,
			BaseDelay:   10 * time.Millisecond,
			MaxDelay:    100 * time.Millisecond,
			Multiplier:  2.0,
		},
		Health: config.HealthConfig{
			CheckInterval: 2 * time.Second,
			Timeout:       5 * time.Second,
			HealthPath:    "/v1/models",
		},
		Endpoints: []config.EndpointConfig{
			{
				Name:          "fast-primary",
				URL:           suite.servers["fast"].URL,
				Group:         "fast-group",
				GroupPriority: 1,
				Priority:      1,
				Timeout:       10 * time.Second,
				Token:         "test-token",
			},
			{
				Name:          "medium-primary",
				URL:           suite.servers["medium"].URL,
				Group:         "medium-group", 
				GroupPriority: 2,
				Priority:      1,
				Timeout:       10 * time.Second,
				Token:         "test-token",
			},
			{
				Name:          "failure-endpoint",
				URL:           suite.servers["failure"].URL,
				Group:         "failure-group",
				GroupPriority: 3,
				Priority:      1,
				Timeout:       10 * time.Second,
				Token:         "test-token",
			},
			// 负载均衡组
			{
				Name:          "balance1",
				URL:           suite.servers["balance1"].URL,
				Group:         "balance-group",
				GroupPriority: 4,
				Priority:      1,
				Timeout:       10 * time.Second,
				Token:         "test-token",
			},
			{
				Name:          "balance2", 
				URL:           suite.servers["balance2"].URL,
				Group:         "balance-group",
				GroupPriority: 4,
				Priority:      2,
				Timeout:       10 * time.Second,
				Token:         "test-token",
			},
			{
				Name:          "balance3",
				URL:           suite.servers["balance3"].URL,
				Group:         "balance-group",
				GroupPriority: 4,
				Priority:      3,
				Timeout:       10 * time.Second,
				Token:         "test-token",
			},
			{
				Name:          "big-response",
				URL:           suite.servers["big_response"].URL,
				Group:         "big-group",
				GroupPriority: 5,
				Priority:      1,
				Timeout:       15 * time.Second,
				Token:         "test-token",
			},
		},
	}
}

// SetupPerformanceComponents 设置性能测试组件
func (suite *PerformanceTestSuite) SetupPerformanceComponents() error {
	suite.endpointManager = endpoint.NewManager(suite.config)
	suite.endpointManager.Start()

	suite.proxyHandler = proxy.NewHandler(suite.endpointManager, suite.config)
	suite.monitoring = middleware.NewMonitoringMiddleware(suite.endpointManager)
	suite.proxyHandler.SetMonitoringMiddleware(suite.monitoring)

	// Web服务器可选
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	// 创建空的UsageTracker用于测试
	usageTracker, _ := tracking.NewUsageTracker(&tracking.Config{Enabled: false})
	// 创建EventBus
	eventBus := events.NewEventBus(logger)
	suite.webServer = web.NewWebServer(suite.config, suite.endpointManager, suite.monitoring, usageTracker, logger, time.Now(), "performance-test.yaml", eventBus)

	// 等待组件初始化
	time.Sleep(3 * time.Second)
	return nil
}

// Cleanup 清理性能测试资源
func (suite *PerformanceTestSuite) Cleanup() {
	if suite.endpointManager != nil {
		suite.endpointManager.Stop()
	}
	if suite.webServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		suite.webServer.Stop(ctx)
	}
	for _, server := range suite.servers {
		if server != nil {
			server.Close()
		}
	}
}

// RunConcurrentRequests 运行并发请求测试
func (suite *PerformanceTestSuite) RunConcurrentRequests(numGoroutines, requestsPerGoroutine int, targetGroup string) *PerformanceTestMetrics {
	metrics := &PerformanceTestMetrics{
		MinLatency: time.Hour, // 初始化为一个大值
	}

	// 激活目标组
	if targetGroup != "" {
		groupManager := suite.endpointManager.GetGroupManager()
		groupManager.ManualActivateGroup(targetGroup)
		time.Sleep(500 * time.Millisecond)
	}

	// 记录开始状态
	runtime.GC()
	runtime.ReadMemStats(&metrics.MemoryStart)
	metrics.StartTime = time.Now()

	var wg sync.WaitGroup
	var latencyMutex sync.Mutex

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			
			for j := 0; j < requestsPerGoroutine; j++ {
				requestStart := time.Now()
				
				body := fmt.Sprintf(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Performance test %d-%d"}]}`, goroutineID, j)
				req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte(body)))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer test-token")

				rr := httptest.NewRecorder()
				suite.proxyHandler.ServeHTTP(rr, req)

				latency := time.Since(requestStart)
				
				// 更新指标
				atomic.AddInt64(&metrics.TotalRequests, 1)
				if rr.Code >= 200 && rr.Code < 300 {
					atomic.AddInt64(&metrics.SuccessfulRequest, 1)
				} else {
					atomic.AddInt64(&metrics.FailedRequests, 1)
				}

				// 更新延迟统计（需要加锁）
				latencyMutex.Lock()
				metrics.TotalLatency += latency
				if latency < metrics.MinLatency {
					metrics.MinLatency = latency
				}
				if latency > metrics.MaxLatency {
					metrics.MaxLatency = latency
				}
				latencyMutex.Unlock()
			}
		}(i)
	}

	wg.Wait()

	// 记录结束状态
	metrics.EndTime = time.Now()
	runtime.GC()
	runtime.ReadMemStats(&metrics.MemoryEnd)

	return metrics
}

// TestRequestSuspendPerformance 请求挂起功能性能测试
func TestRequestSuspendPerformance(t *testing.T) {
	suite := NewPerformanceTestSuite()
	defer suite.Cleanup()

	suite.SetupPerformanceServers()
	suite.CreatePerformanceConfig()
	err := suite.SetupPerformanceComponents()
	if err != nil {
		t.Fatalf("Failed to setup performance test suite: %v", err)
	}

	t.Run("Baseline Performance Test", func(t *testing.T) {
		// 基线性能测试 - 正常情况下的性能
		metrics := suite.RunConcurrentRequests(10, 20, "fast-group")
		stats := metrics.CalculateStats()

		t.Logf("基线性能测试结果:")
		for key, value := range stats {
			t.Logf("  %s: %v", key, value)
		}

		// 性能基线检查
		if stats["qps"].(float64) < 10 {
			t.Errorf("QPS too low: %.2f", stats["qps"].(float64))
		}

		if stats["success_rate"].(float64) < 95 {
			t.Errorf("Success rate too low: %.2f%%", stats["success_rate"].(float64))
		}

		if stats["avg_latency_ms"].(float64) > 1000 {
			t.Errorf("Average latency too high: %.2f ms", stats["avg_latency_ms"].(float64))
		}
	})

	t.Run("High Concurrency Test", func(t *testing.T) {
		// 高并发测试
		metrics := suite.RunConcurrentRequests(50, 10, "balance-group")
		stats := metrics.CalculateStats()

		t.Logf("高并发测试结果 (50 goroutines × 10 requests):")
		for key, value := range stats {
			t.Logf("  %s: %v", key, value)
		}

		// 检查负载分布
		t.Logf("负载分布:")
		for name, count := range suite.requestCounts {
			if strings.HasPrefix(name, "balance") {
				t.Logf("  %s: %d requests", name, atomic.LoadInt64(count))
			}
		}

		// 高并发下的性能要求
		if stats["success_rate"].(float64) < 90 {
			t.Errorf("High concurrency success rate too low: %.2f%%", stats["success_rate"].(float64))
		}
	})

	t.Run("Suspend and Recovery Performance", func(t *testing.T) {
		// 挂起和恢复性能测试
		
		// 先暂停所有组，强制挂起
		groupManager := suite.endpointManager.GetGroupManager()
		groupManager.ManualPauseGroup("fast-group", time.Hour)
		groupManager.ManualPauseGroup("medium-group", time.Hour)
		groupManager.ManualPauseGroup("balance-group", time.Hour)

		var wg sync.WaitGroup
		numRequests := 20
		results := make([]time.Duration, numRequests)
		statuses := make([]int, numRequests)

		// 异步发送请求
		for i := 0; i < numRequests; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				
				start := time.Now()
				body := fmt.Sprintf(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Suspend test %d"}]}`, index)
				req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte(body)))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer test-token")

				rr := httptest.NewRecorder()
				suite.proxyHandler.ServeHTTP(rr, req)
				
				results[index] = time.Since(start)
				statuses[index] = rr.Code
			}(i)
		}

		// 等待1秒让请求被挂起
		time.Sleep(1 * time.Second)

		// 激活快速组来恢复挂起的请求
		err := groupManager.ManualActivateGroup("fast-group")
		if err != nil {
			t.Fatalf("Failed to activate fast-group: %v", err)
		}

		wg.Wait()

		// 分析结果
		successCount := 0
		var totalLatency time.Duration
		var minLatency, maxLatency time.Duration = time.Hour, 0

		for i, duration := range results {
			if statuses[i] >= 200 && statuses[i] < 300 {
				successCount++
			}
			
			totalLatency += duration
			if duration < minLatency {
				minLatency = duration
			}
			if duration > maxLatency {
				maxLatency = duration
			}
		}

		avgLatency := totalLatency / time.Duration(numRequests)
		successRate := float64(successCount) / float64(numRequests) * 100

		t.Logf("挂起和恢复性能测试结果:")
		t.Logf("  总请求数: %d", numRequests)
		t.Logf("  成功请求: %d", successCount)
		t.Logf("  成功率: %.2f%%", successRate)
		t.Logf("  平均延迟: %v", avgLatency)
		t.Logf("  最小延迟: %v", minLatency)
		t.Logf("  最大延迟: %v", maxLatency)

		// 检查挂起统计
		stats := suite.monitoring.GetSuspendedRequestStats()
		if totalSuspended, ok := stats["total_suspended_requests"].(int64); ok {
			t.Logf("  总挂起请求: %d", totalSuspended)
		}
		if successful, ok := stats["successful_suspended_requests"].(int64); ok {
			t.Logf("  成功恢复: %d", successful)
		}

		// 性能要求
		if successRate < 80 {
			t.Errorf("Suspend recovery success rate too low: %.2f%%", successRate)
		}
	})

	t.Run("Large Response Handling", func(t *testing.T) {
		// 大响应处理性能测试
		metrics := suite.RunConcurrentRequests(5, 10, "big-group")
		stats := metrics.CalculateStats()

		t.Logf("大响应处理测试结果:")
		for key, value := range stats {
			t.Logf("  %s: %v", key, value)
		}

		// 检查内存使用
		memoryUsage := stats["memory_alloc_mb"].(float64)
		if memoryUsage > 50 { // 50MB限制
			t.Logf("注意: 内存使用较高: %.2f MB", memoryUsage)
		}

		// 大响应的成功率应该保持
		if stats["success_rate"].(float64) < 90 {
			t.Errorf("Large response success rate too low: %.2f%%", stats["success_rate"].(float64))
		}
	})

	t.Run("Memory Leak Detection", func(t *testing.T) {
		// 内存泄漏检测测试
		
		// 记录初始内存
		runtime.GC()
		var initialMem runtime.MemStats
		runtime.ReadMemStats(&initialMem)

		// 执行多轮请求
		rounds := 5
		for round := 0; round < rounds; round++ {
			metrics := suite.RunConcurrentRequests(10, 10, "fast-group")
			t.Logf("轮次 %d 完成, QPS: %.2f", round+1, metrics.CalculateStats()["qps"].(float64))
			
			// 轮次间短暂休息
			time.Sleep(500 * time.Millisecond)
		}

		// 强制GC并检查最终内存
		runtime.GC()
		time.Sleep(100 * time.Millisecond)
		runtime.GC()
		
		var finalMem runtime.MemStats
		runtime.ReadMemStats(&finalMem)

		memoryIncrease := int64(finalMem.Alloc) - int64(initialMem.Alloc)
		objectIncrease := int64(finalMem.HeapObjects) - int64(initialMem.HeapObjects)

		t.Logf("内存泄漏检测结果:")
		t.Logf("  初始内存分配: %d bytes", initialMem.Alloc)
		t.Logf("  最终内存分配: %d bytes", finalMem.Alloc)
		t.Logf("  内存增加: %d bytes (%.2f MB)", memoryIncrease, float64(memoryIncrease)/1024/1024)
		t.Logf("  堆对象增加: %d", objectIncrease)

		// 内存泄漏检查
		reasonableLimit := int64(20 * 1024 * 1024) // 20MB
		if memoryIncrease > reasonableLimit {
			t.Errorf("可能存在内存泄漏: 内存增加 %d bytes 超过限制 %d bytes", memoryIncrease, reasonableLimit)
		}

		if objectIncrease > 10000 {
			t.Errorf("可能存在对象泄漏: 堆对象增加 %d 超过合理范围", objectIncrease)
		}
	})

	t.Run("Sustained Load Test", func(t *testing.T) {
		// 持续负载测试
		testDuration := 15 * time.Second
		requestInterval := 50 * time.Millisecond

		ctx, cancel := context.WithTimeout(context.Background(), testDuration)
		defer cancel()

		// 激活平衡组
		groupManager := suite.endpointManager.GetGroupManager()
		groupManager.ManualActivateGroup("balance-group")

		var requestCount, successCount int64
		ticker := time.NewTicker(requestInterval)
		defer ticker.Stop()

		startTime := time.Now()

		for {
			select {
			case <-ticker.C:
				go func() {
					atomic.AddInt64(&requestCount, 1)
					
					body := fmt.Sprintf(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Sustained test %d"}]}`, requestCount)
					req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte(body)))
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("Authorization", "Bearer test-token")

					rr := httptest.NewRecorder()
					suite.proxyHandler.ServeHTTP(rr, req)

					if rr.Code >= 200 && rr.Code < 300 {
						atomic.AddInt64(&successCount, 1)
					}
				}()

			case <-ctx.Done():
				goto sustained_done
			}
		}

	sustained_done:
		endTime := time.Now()
		finalRequestCount := atomic.LoadInt64(&requestCount)
		finalSuccessCount := atomic.LoadInt64(&successCount)
		
		duration := endTime.Sub(startTime)
		qps := float64(finalRequestCount) / duration.Seconds()
		successRate := float64(finalSuccessCount) / float64(finalRequestCount) * 100

		t.Logf("持续负载测试结果:")
		t.Logf("  测试时长: %v", testDuration)
		t.Logf("  总请求数: %d", finalRequestCount)
		t.Logf("  成功请求: %d", finalSuccessCount)
		t.Logf("  平均QPS: %.2f", qps)
		t.Logf("  成功率: %.2f%%", successRate)

		// 获取系统监控数据
		if systemStats := suite.monitoring.GetSuspendedRequestStats(); len(systemStats) > 0 {
			t.Logf("  系统统计:")
			for key, value := range systemStats {
				t.Logf("    %s: %v", key, value)
			}
		}

		// 持续负载性能要求
		if qps < 5 {
			t.Errorf("持续负载QPS太低: %.2f", qps)
		}

		if successRate < 95 {
			t.Errorf("持续负载成功率太低: %.2f%%", successRate)
		}
	})
}

// BenchmarkRequestSuspendComponents 组件性能基准测试
func BenchmarkRequestSuspendComponents(b *testing.B) {
	suite := NewPerformanceTestSuite()
	defer suite.Cleanup()

	suite.SetupPerformanceServers()
	suite.CreatePerformanceConfig()
	err := suite.SetupPerformanceComponents()
	if err != nil {
		b.Fatalf("Failed to setup benchmark suite: %v", err)
	}

	// 激活快速组
	groupManager := suite.endpointManager.GetGroupManager()
	groupManager.ManualActivateGroup("fast-group")
	time.Sleep(1 * time.Second)

	b.Run("ProxyHandler", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			requestID := 0
			for pb.Next() {
				body := fmt.Sprintf(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Benchmark %d"}]}`, requestID)
				req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte(body)))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer test-token")

				rr := httptest.NewRecorder()
				suite.proxyHandler.ServeHTTP(rr, req)
				requestID++
			}
		})
	})

	b.Run("SuspendedRequestsCount", func(b *testing.B) {
		retryHandler := suite.proxyHandler.GetRetryHandler()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = retryHandler.GetSuspendedRequestsCount()
		}
	})

	b.Run("MonitoringStats", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = suite.monitoring.GetSuspendedRequestStats()
		}
	})

	b.Run("GroupManagement", func(b *testing.B) {
		groupManager := suite.endpointManager.GetGroupManager()
		groups := []string{"fast-group", "medium-group", "balance-group"}
		b.ResetTimer()
		
		for i := 0; i < b.N; i++ {
			group := groups[i%len(groups)]
			_ = groupManager.ManualActivateGroup(group)
		}
	})
}