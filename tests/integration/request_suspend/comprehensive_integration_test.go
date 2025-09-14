package integration

import (
	"bytes"
	"context"
	"encoding/json"
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

// ComprehensiveIntegrationTestSuite 综合集成测试套件
type ComprehensiveIntegrationTestSuite struct {
	primaryServer   *httptest.Server
	backupServer    *httptest.Server
	slowServer      *httptest.Server
	endpointManager *endpoint.Manager
	proxyHandler    *proxy.Handler
	monitoring      *middleware.MonitoringMiddleware
	webServer       *web.WebServer
	config          *config.Config
	cleanup         []func()
}

// SetupTestSuite 创建测试环境
func (suite *ComprehensiveIntegrationTestSuite) SetupTestSuite(t *testing.T) {
	// 创建主服务器（模拟故障）
	suite.primaryServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Primary server unavailable", http.StatusInternalServerError)
	}))

	// 创建备用服务器（正常）
	suite.backupServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		
		// 模拟不同的API响应
		if strings.Contains(r.URL.Path, "/v1/models") {
			w.Write([]byte(`{"object":"list","data":[{"id":"gpt-3.5-turbo","object":"model"}]}`))
		} else if strings.Contains(r.URL.Path, "/v1/chat/completions") {
			if strings.Contains(r.Header.Get("Accept"), "text/event-stream") || 
			   strings.Contains(r.Header.Get("Content-Type"), "stream") {
				// SSE 响应
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("Cache-Control", "no-cache")
				fmt.Fprintf(w, "data: {\"id\":\"1\",\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n")
				fmt.Fprintf(w, "data: {\"id\":\"2\",\"choices\":[{\"delta\":{\"content\":\" World\"}}]}\n\n")
				fmt.Fprintf(w, "data: [DONE]\n\n")
			} else {
				// 普通JSON响应
				w.Write([]byte(`{"id":"chatcmpl-123","object":"chat.completion","choices":[{"message":{"content":"Hello World"}}]}`))
			}
		} else {
			w.Write([]byte(`{"status":"ok"}`))
		}
	}))

	// 创建慢响应服务器（用于超时测试）
	suite.slowServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Second) // 故意延迟
		w.WriteHeader(200)
		w.Write([]byte(`{"status":"slow"}`))
	}))

	// 配置
	suite.config = &config.Config{
		Server: config.ServerConfig{
			Host: "localhost",
			Port: 0,
		},
		Web: config.WebConfig{
			Enabled: true,
			Host:    "localhost", 
			Port:    0,
		},
		RequestSuspend: config.RequestSuspendConfig{
			Enabled:              true,
			Timeout:              30 * time.Second,
			MaxSuspendedRequests: 100,
		},
		Group: config.GroupConfig{
			AutoSwitchBetweenGroups: false, // 手动模式以触发挂起
			Cooldown:                5 * time.Second,
		},
		Retry: config.RetryConfig{
			MaxAttempts: 2,
			BaseDelay:   100 * time.Millisecond,
			MaxDelay:    1 * time.Second,
			Multiplier:  2.0,
		},
		Health: config.HealthConfig{
			CheckInterval: 500 * time.Millisecond,
			Timeout:       2 * time.Second,
			HealthPath:    "/v1/models",
		},
		Endpoints: []config.EndpointConfig{
			{
				Name:          "primary",
				URL:           suite.primaryServer.URL,
				Group:         "main",
				GroupPriority: 1,
				Priority:      1,
				Timeout:       3 * time.Second,
				Token:         "test-token-primary",
			},
			{
				Name:          "backup",
				URL:           suite.backupServer.URL,
				Group:         "backup",
				GroupPriority: 2,
				Priority:      1,
				Timeout:       3 * time.Second,
				Token:         "test-token-backup",
			},
			{
				Name:          "slow",
				URL:           suite.slowServer.URL,
				Group:         "slow",
				GroupPriority: 3,
				Priority:      1,
				Timeout:       1 * time.Second, // 短超时用于测试
				Token:         "test-token-slow",
			},
		},
	}

	// 创建组件
	suite.endpointManager = endpoint.NewManager(suite.config)
	suite.endpointManager.Start()
	suite.cleanup = append(suite.cleanup, func() { suite.endpointManager.Stop() })

	suite.proxyHandler = proxy.NewHandler(suite.endpointManager, suite.config)
	suite.monitoring = middleware.NewMonitoringMiddleware(suite.endpointManager)
	suite.proxyHandler.SetMonitoringMiddleware(suite.monitoring)

	// 创建Web服务器
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	// 创建空的UsageTracker用于测试
	usageTracker, _ := tracking.NewUsageTracker(&tracking.Config{Enabled: false})
	// 创建EventBus
	eventBus := events.NewEventBus(logger)
	suite.webServer = web.NewWebServer(suite.config, suite.endpointManager, suite.monitoring, usageTracker, logger, time.Now(), "test-config.yaml", eventBus)

	err := suite.webServer.Start()
	if err != nil {
		t.Fatalf("Failed to start web server: %v", err)
	}
	suite.cleanup = append(suite.cleanup, func() { 
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		suite.webServer.Stop(ctx) 
	})

	// 等待组件初始化和健康检查
	time.Sleep(2 * time.Second)
}

// TeardownTestSuite 清理测试环境
func (suite *ComprehensiveIntegrationTestSuite) TeardownTestSuite() {
	// 执行清理函数
	for i := len(suite.cleanup) - 1; i >= 0; i-- {
		if suite.cleanup[i] != nil {
			suite.cleanup[i]()
		}
	}

	// 关闭测试服务器
	if suite.primaryServer != nil {
		suite.primaryServer.Close()
	}
	if suite.backupServer != nil {
		suite.backupServer.Close()
	}
	if suite.slowServer != nil {
		suite.slowServer.Close()
	}
}

// TestComprehensiveRequestSuspendFlow 综合请求挂起流程测试
func TestComprehensiveRequestSuspendFlow(t *testing.T) {
	suite := &ComprehensiveIntegrationTestSuite{}
	suite.SetupTestSuite(t)
	defer suite.TeardownTestSuite()

	t.Run("Complete suspend and resume flow", func(t *testing.T) {
		// 1. 发送会被挂起的请求
		body := `{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Test suspend flow"}]}`
		req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")

		rr := httptest.NewRecorder()
		requestDone := make(chan bool, 1)
		var finalStatus int
		var responseBody string

		// 异步处理请求
		go func() {
			suite.proxyHandler.ServeHTTP(rr, req)
			finalStatus = rr.Code
			responseBody = rr.Body.String()
			requestDone <- true
		}()

		// 2. 验证请求被挂起
		time.Sleep(1 * time.Second)
		select {
		case <-requestDone:
			t.Error("Request should be suspended, but completed immediately")
		default:
			// 请求正确地被挂起
		}

		// 3. 检查挂起状态
		retryHandler := suite.proxyHandler.GetRetryHandler()
		suspendedCount := retryHandler.GetSuspendedRequestsCount()
		if suspendedCount == 0 {
			t.Error("Expected suspended requests count > 0")
		}

		// 4. 检查监控统计
		stats := suite.monitoring.GetSuspendedRequestStats()
		if suspended, ok := stats["suspended_requests"].(int64); !ok || suspended == 0 {
			t.Error("Expected suspended requests in monitoring stats")
		}

		// 5. 通过Web API激活备用组
		groupManager := suite.endpointManager.GetGroupManager()
		err := groupManager.ManualActivateGroup("backup")
		if err != nil {
			t.Fatalf("Failed to activate backup group: %v", err)
		}

		// 6. 等待请求完成
		select {
		case <-requestDone:
			if finalStatus != 200 {
				t.Errorf("Expected final status 200, got %d. Body: %s", finalStatus, responseBody)
			}
		case <-time.After(5 * time.Second):
			t.Error("Request timed out even after group activation")
		}

		// 7. 验证请求已恢复
		finalSuspendedCount := retryHandler.GetSuspendedRequestsCount()
		if finalSuspendedCount > suspendedCount {
			t.Errorf("Suspended count should not increase after resume. Before: %d, After: %d", suspendedCount, finalSuspendedCount)
		}

		// 8. 检查成功恢复的统计
		finalStats := suite.monitoring.GetSuspendedRequestStats()
		if successful, ok := finalStats["successful_suspended_requests"].(int64); !ok || successful == 0 {
			t.Error("Expected successful suspended requests in final stats")
		}
	})
}

// TestRequestSuspendPerformanceAndStability 性能和稳定性测试
func TestRequestSuspendPerformanceAndStability(t *testing.T) {
	suite := &ComprehensiveIntegrationTestSuite{}
	suite.SetupTestSuite(t)
	defer suite.TeardownTestSuite()

	t.Run("High concurrency suspend test", func(t *testing.T) {
		numGoroutines := 50
		requestsPerGoroutine := 5
		totalRequests := numGoroutines * requestsPerGoroutine

		var wg sync.WaitGroup
		var successCount, errorCount int64
		var responseTime sync.Map

		startTime := time.Now()

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()
				
				for j := 0; j < requestsPerGoroutine; j++ {
					requestStart := time.Now()
					
					body := fmt.Sprintf(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Concurrency test %d-%d"}]}`, goroutineID, j)
					req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("Authorization", "Bearer test-token")

					rr := httptest.NewRecorder()
					suite.proxyHandler.ServeHTTP(rr, req)

					elapsed := time.Since(requestStart)
					responseTime.Store(fmt.Sprintf("%d-%d", goroutineID, j), elapsed)

					if rr.Code == 200 {
						atomic.AddInt64(&successCount, 1)
					} else {
						atomic.AddInt64(&errorCount, 1)
					}
				}
			}(i)
		}

		// 在并发请求运行期间激活备用组
		time.Sleep(2 * time.Second)
		groupManager := suite.endpointManager.GetGroupManager()
		groupManager.ManualActivateGroup("backup")

		wg.Wait()
		totalTime := time.Since(startTime)

		t.Logf("并发测试结果:")
		t.Logf("  总请求数: %d", totalRequests)
		t.Logf("  成功请求: %d", successCount)
		t.Logf("  失败请求: %d", errorCount)
		t.Logf("  总耗时: %v", totalTime)
		t.Logf("  平均QPS: %.2f", float64(totalRequests)/totalTime.Seconds())

		// 计算响应时间统计
		var totalResponseTime time.Duration
		var maxResponseTime time.Duration
		var minResponseTime = time.Hour
		requestCount := 0

		responseTime.Range(func(key, value interface{}) bool {
			duration := value.(time.Duration)
			totalResponseTime += duration
			requestCount++
			if duration > maxResponseTime {
				maxResponseTime = duration
			}
			if duration < minResponseTime {
				minResponseTime = duration
			}
			return true
		})

		avgResponseTime := totalResponseTime / time.Duration(requestCount)
		t.Logf("  平均响应时间: %v", avgResponseTime)
		t.Logf("  最大响应时间: %v", maxResponseTime)
		t.Logf("  最小响应时间: %v", minResponseTime)

		// 验证基本性能要求
		if successCount == 0 {
			t.Error("所有请求都失败了")
		}

		successRate := float64(successCount) / float64(totalRequests) * 100
		if successRate < 50 {
			t.Errorf("成功率太低: %.2f%%, 期望至少50%%", successRate)
		}

		// 检查最终挂起状态
		retryHandler := suite.proxyHandler.GetRetryHandler()
		finalSuspended := retryHandler.GetSuspendedRequestsCount()
		t.Logf("  最终挂起请求数: %d", finalSuspended)
	})

	t.Run("Memory leak test", func(t *testing.T) {
		// 记录初始内存
		runtime.GC()
		var m1 runtime.MemStats
		runtime.ReadMemStats(&m1)

		// 执行大量请求
		numRequests := 100
		for i := 0; i < numRequests; i++ {
			body := fmt.Sprintf(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Memory test %d"}]}`, i)
			req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer test-token")

			rr := httptest.NewRecorder()
			suite.proxyHandler.ServeHTTP(rr, req)

			if i == 50 {
				// 中途激活备用组
				groupManager := suite.endpointManager.GetGroupManager()
				groupManager.ManualActivateGroup("backup")
			}
		}

		// 等待所有请求处理完成
		time.Sleep(2 * time.Second)

		// 记录最终内存
		runtime.GC()
		var m2 runtime.MemStats
		runtime.ReadMemStats(&m2)

		memoryIncrease := int64(m2.Alloc) - int64(m1.Alloc)
		t.Logf("内存使用变化:")
		t.Logf("  初始分配: %d bytes", m1.Alloc)
		t.Logf("  最终分配: %d bytes", m2.Alloc) 
		t.Logf("  内存增加: %d bytes", memoryIncrease)
		t.Logf("  堆对象数: %d -> %d", m1.HeapObjects, m2.HeapObjects)

		// 检查是否有明显的内存泄漏（允许合理的增长）
		reasonableLimit := int64(10 * 1024 * 1024) // 10MB
		if memoryIncrease > reasonableLimit {
			t.Errorf("可能存在内存泄漏，内存增加了 %d bytes (超过限制 %d bytes)", memoryIncrease, reasonableLimit)
		}
	})

	t.Run("Long running stability test", func(t *testing.T) {
		testDuration := 10 * time.Second
		requestInterval := 100 * time.Millisecond
		
		ctx, cancel := context.WithTimeout(context.Background(), testDuration)
		defer cancel()

		var requestCount, successCount int64
		ticker := time.NewTicker(requestInterval)
		defer ticker.Stop()

		// 定期切换组状态
		groupSwitchTicker := time.NewTicker(2 * time.Second)
		defer groupSwitchTicker.Stop()
		
		activeGroup := "main"
		groupManager := suite.endpointManager.GetGroupManager()

		go func() {
			for {
				select {
				case <-groupSwitchTicker.C:
					if activeGroup == "main" {
						groupManager.ManualActivateGroup("backup")
						activeGroup = "backup"
					} else {
						groupManager.ManualActivateGroup("main")
						activeGroup = "main"
					}
				case <-ctx.Done():
					return
				}
			}
		}()

		for {
			select {
			case <-ticker.C:
				atomic.AddInt64(&requestCount, 1)
				
				body := fmt.Sprintf(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Stability test %d"}]}`, requestCount)
				req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer test-token")

				rr := httptest.NewRecorder()
				suite.proxyHandler.ServeHTTP(rr, req)

				if rr.Code == 200 {
					atomic.AddInt64(&successCount, 1)
				}

			case <-ctx.Done():
				goto done
			}
		}

	done:
		finalRequestCount := atomic.LoadInt64(&requestCount)
		finalSuccessCount := atomic.LoadInt64(&successCount)
		successRate := float64(finalSuccessCount) / float64(finalRequestCount) * 100

		t.Logf("稳定性测试结果:")
		t.Logf("  测试时长: %v", testDuration)
		t.Logf("  总请求数: %d", finalRequestCount)
		t.Logf("  成功请求: %d", finalSuccessCount)
		t.Logf("  成功率: %.2f%%", successRate)

		// 获取最终统计
		stats := suite.monitoring.GetSuspendedRequestStats()
		if totalSuspended, ok := stats["total_suspended_requests"].(int64); ok {
			t.Logf("  总挂起请求: %d", totalSuspended)
		}
		if successful, ok := stats["successful_suspended_requests"].(int64); ok {
			t.Logf("  成功恢复: %d", successful)
		}
		if timeout, ok := stats["timeout_suspended_requests"].(int64); ok {
			t.Logf("  超时请求: %d", timeout)
		}

		if finalRequestCount == 0 {
			t.Error("没有发送任何请求")
		} else if successRate < 30 {
			t.Errorf("长时间运行稳定性测试成功率过低: %.2f%%", successRate)
		}
	})
}

// TestRequestSuspendSSEAndEdgeCases 测试SSE和边界情况
func TestRequestSuspendSSEAndEdgeCases(t *testing.T) {
	suite := &ComprehensiveIntegrationTestSuite{}
	suite.SetupTestSuite(t)
	defer suite.TeardownTestSuite()

	t.Run("SSE request suspend and resume", func(t *testing.T) {
		body := `{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"SSE test"}],"stream":true}`
		req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")
		req.Header.Set("Accept", "text/event-stream")

		rr := httptest.NewRecorder()
		requestDone := make(chan bool, 1)
		var responseBody string

		go func() {
			suite.proxyHandler.ServeHTTP(rr, req)
			responseBody = rr.Body.String()
			requestDone <- true
		}()

		// 等待挂起
		time.Sleep(1 * time.Second)

		// 激活备用组
		groupManager := suite.endpointManager.GetGroupManager()
		err := groupManager.ManualActivateGroup("backup")
		if err != nil {
			t.Fatalf("Failed to activate backup group: %v", err)
		}

		// 等待完成
		select {
		case <-requestDone:
			if rr.Code != 200 {
				t.Errorf("Expected status 200 for SSE, got %d", rr.Code)
			}
			if !strings.Contains(responseBody, "data:") {
				t.Error("SSE response should contain 'data:' events")
			}
		case <-time.After(5 * time.Second):
			t.Error("SSE request timed out")
		}
	})

	t.Run("Maximum suspended requests limit", func(t *testing.T) {
		// 临时修改配置以测试限制
		originalMax := suite.config.RequestSuspend.MaxSuspendedRequests
		suite.config.RequestSuspend.MaxSuspendedRequests = 3
		suite.proxyHandler.UpdateConfig(suite.config)

		var wg sync.WaitGroup
		numRequests := 10
		results := make([]int, numRequests)

		// 发送超过限制的请求数
		for i := 0; i < numRequests; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				
				body := fmt.Sprintf(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Limit test %d"}]}`, index)
				req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer test-token")

				rr := httptest.NewRecorder()
				suite.proxyHandler.ServeHTTP(rr, req)
				results[index] = rr.Code
			}(i)
		}

		// 等待一段时间让请求被处理或挂起
		time.Sleep(2 * time.Second)

		// 激活备用组
		groupManager := suite.endpointManager.GetGroupManager()
		groupManager.ManualActivateGroup("backup")

		wg.Wait()

		// 检查结果
		successCount := 0
		for _, code := range results {
			if code == 200 {
				successCount++
			}
		}

		t.Logf("在挂起限制测试中，%d个请求中有%d个成功", numRequests, successCount)

		// 恢复原始配置
		suite.config.RequestSuspend.MaxSuspendedRequests = originalMax
		suite.proxyHandler.UpdateConfig(suite.config)
	})

	t.Run("Request suspend timeout", func(t *testing.T) {
		// 修改为短超时进行测试
		originalTimeout := suite.config.RequestSuspend.Timeout
		suite.config.RequestSuspend.Timeout = 2 * time.Second
		suite.proxyHandler.UpdateConfig(suite.config)

		body := `{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Timeout test"}]}`
		req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")

		rr := httptest.NewRecorder()
		start := time.Now()
		suite.proxyHandler.ServeHTTP(rr, req)
		elapsed := time.Since(start)

		// 验证超时行为
		if rr.Code == 200 {
			t.Error("请求应该因为挂起超时而失败")
		}

		// 验证响应时间接近超时设置
		if elapsed < time.Second || elapsed > 4*time.Second {
			t.Errorf("响应时间不符合预期，期望大约2秒，实际: %v", elapsed)
		}

		// 检查超时统计
		stats := suite.monitoring.GetSuspendedRequestStats()
		if timeout, ok := stats["timeout_suspended_requests"].(int64); !ok || timeout == 0 {
			t.Error("预期超时统计中应有记录")
		}

		// 恢复原始配置
		suite.config.RequestSuspend.Timeout = originalTimeout
		suite.proxyHandler.UpdateConfig(suite.config)
	})
}

// TestWebAPIIntegrationWithSuspend 测试Web API与挂起功能的集成
func TestWebAPIIntegrationWithSuspend(t *testing.T) {
	suite := &ComprehensiveIntegrationTestSuite{}
	suite.SetupTestSuite(t)
	defer suite.TeardownTestSuite()

	webPort := suite.config.Web.Port
	if webPort == 0 {
		webPort = 8088 // 默认端口
	}
	webAddr := fmt.Sprintf("http://localhost:%d", webPort)

	t.Run("Web API status includes suspend info", func(t *testing.T) {
		// 先发送一个会被挂起的请求
		go func() {
			body := `{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Web API test"}]}`
			req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer test-token")
			
			rr := httptest.NewRecorder()
			suite.proxyHandler.ServeHTTP(rr, req)
		}()

		time.Sleep(1 * time.Second)

		// 测试状态API
		resp, err := http.Get(webAddr + "/api/v1/status")
		if err != nil {
			t.Fatalf("Failed to get status from web API: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		var status map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&status)
		if err != nil {
			t.Fatalf("Failed to decode status: %v", err)
		}

		// 验证包含挂起请求信息
		if _, ok := status["suspended_requests"]; !ok {
			t.Error("状态响应中应包含suspended_requests字段")
		}

		t.Logf("Web API状态响应: %+v", status)
	})

	t.Run("Web API group management", func(t *testing.T) {
		// 测试获取组状态
		resp, err := http.Get(webAddr + "/api/v1/groups")
		if err != nil {
			t.Fatalf("Failed to get groups: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			t.Errorf("Expected status 200 for groups API, got %d", resp.StatusCode)
		}

		var groups []map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&groups)
		if err != nil {
			t.Fatalf("Failed to decode groups: %v", err)
		}

		t.Logf("Groups API响应: %+v", groups)

		// 测试激活组
		activateResp, err := http.Post(webAddr + "/api/v1/groups/backup/activate", "application/json", nil)
		if err != nil {
			t.Fatalf("Failed to activate group: %v", err)
		}
		defer activateResp.Body.Close()

		if activateResp.StatusCode != 200 {
			body, _ := io.ReadAll(activateResp.Body)
			t.Errorf("Expected status 200 for group activation, got %d. Body: %s", activateResp.StatusCode, string(body))
		}
	})
}

// TestConfigurationHotReload 测试配置热重载与挂起功能的交互
func TestConfigurationHotReload(t *testing.T) {
	suite := &ComprehensiveIntegrationTestSuite{}
	suite.SetupTestSuite(t)
	defer suite.TeardownTestSuite()

	t.Run("Hot reload suspend configuration", func(t *testing.T) {
		// 获取当前挂起状态
		retryHandler := suite.proxyHandler.GetRetryHandler()
		initialCount := retryHandler.GetSuspendedRequestsCount()

		// 修改配置
		newConfig := *suite.config
		newConfig.RequestSuspend.MaxSuspendedRequests = 200
		newConfig.RequestSuspend.Timeout = 60 * time.Second

		// 应用新配置
		suite.proxyHandler.UpdateConfig(&newConfig)
		suite.endpointManager.UpdateConfig(&newConfig)

		// 验证配置已更新
		// 这里可以通过发送请求来间接验证配置变更是否生效
		
		body := `{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Hot reload test"}]}`
		req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")

		rr := httptest.NewRecorder()
		suite.proxyHandler.ServeHTTP(rr, req)

		newCount := retryHandler.GetSuspendedRequestsCount()
		t.Logf("配置热重载测试 - 初始挂起数: %d, 当前挂起数: %d", initialCount, newCount)

		// 验证系统仍能正常工作
		if rr.Code != 200 && rr.Code != 500 { // 500是期望的，因为主服务器失败
			t.Logf("Hot reload test response code: %d", rr.Code)
		}
	})
}

// BenchmarkRequestSuspendPerformance 性能基准测试
func BenchmarkRequestSuspendPerformance(b *testing.B) {
	suite := &ComprehensiveIntegrationTestSuite{}
	suite.SetupTestSuite(&testing.T{}) // 使用空的T用于设置
	defer suite.TeardownTestSuite()

	// 预先激活备用组以获得稳定的性能数据
	groupManager := suite.endpointManager.GetGroupManager()
	groupManager.ManualActivateGroup("backup")
	time.Sleep(1 * time.Second)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		requestID := 0
		for pb.Next() {
			body := fmt.Sprintf(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Benchmark %d"}]}`, requestID)
			req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer test-token")

			rr := httptest.NewRecorder()
			suite.proxyHandler.ServeHTTP(rr, req)
			requestID++
		}
	})
}

// TestComprehensiveErrorHandling 综合错误处理测试
func TestComprehensiveErrorHandling(t *testing.T) {
	suite := &ComprehensiveIntegrationTestSuite{}
	suite.SetupTestSuite(t)
	defer suite.TeardownTestSuite()

	testCases := []struct {
		name        string
		requestBody string
		headers     map[string]string
		expectCode  int
	}{
		{
			name:        "Invalid JSON",
			requestBody: `{"invalid": json}`,
			headers:     map[string]string{"Content-Type": "application/json", "Authorization": "Bearer test-token"},
			expectCode:  0, // 任何错误码都可以接受
		},
		{
			name:        "Missing Authorization",
			requestBody: `{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"test"}]}`,
			headers:     map[string]string{"Content-Type": "application/json"},
			expectCode:  0,
		},
		{
			name:        "Empty Body",
			requestBody: "",
			headers:     map[string]string{"Content-Type": "application/json", "Authorization": "Bearer test-token"},
			expectCode:  0,
		},
		{
			name:        "Large Request Body",
			requestBody: fmt.Sprintf(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"%s"}]}`, strings.Repeat("A", 10000)),
			headers:     map[string]string{"Content-Type": "application/json", "Authorization": "Bearer test-token"},
			expectCode:  0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte(tc.requestBody)))
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}

			rr := httptest.NewRecorder()
			suite.proxyHandler.ServeHTTP(rr, req)

			t.Logf("错误处理测试 '%s': 状态码=%d, 响应长度=%d", tc.name, rr.Code, len(rr.Body.String()))

			// 验证系统没有崩溃（任何HTTP状态码都可以接受）
			if rr.Code == 0 {
				t.Error("应该返回有效的HTTP状态码")
			}
		})
	}
}