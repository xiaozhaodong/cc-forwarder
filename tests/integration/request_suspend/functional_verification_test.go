package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

// FunctionalTestSuite 功能验证测试套件
type FunctionalTestSuite struct {
	servers map[string]*httptest.Server
	config  *config.Config
	endpointManager *endpoint.Manager
	proxyHandler    *proxy.Handler
	monitoring      *middleware.MonitoringMiddleware
	webServer       *web.WebServer
}

// NewFunctionalTestSuite 创建功能测试套件
func NewFunctionalTestSuite() *FunctionalTestSuite {
	return &FunctionalTestSuite{
		servers: make(map[string]*httptest.Server),
	}
}

// SetupServers 设置测试服务器
func (suite *FunctionalTestSuite) SetupServers() {
	// 健康服务器
	suite.servers["healthy"] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		if strings.Contains(r.URL.Path, "/v1/models") {
			w.Write([]byte(`{"object":"list","data":[{"id":"gpt-3.5-turbo","object":"model"}]}`))
		} else {
			w.Write([]byte(`{"id":"test","object":"chat.completion","choices":[{"message":{"content":"Success from healthy server"}}]}`))
		}
	}))

	// 故障服务器
	suite.servers["unhealthy"] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}))

	// 慢响应服务器
	suite.servers["slow"] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second) // 模拟慢响应
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"slow","object":"chat.completion","choices":[{"message":{"content":"Slow response"}}]}`))
	}))

	// SSE服务器
	suite.servers["sse"] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		
		// 发送SSE数据
		fmt.Fprintf(w, "data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n")
		w.(http.Flusher).Flush()
		
		time.Sleep(100 * time.Millisecond)
		fmt.Fprintf(w, "data: {\"id\":\"2\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\" World\"}}]}\n\n")
		w.(http.Flusher).Flush()
		
		time.Sleep(100 * time.Millisecond)
		fmt.Fprintf(w, "data: [DONE]\n\n")
		w.(http.Flusher).Flush()
	}))

	// 间歇性故障服务器（用于测试恢复）
	requestCount := 0
	var mu sync.Mutex
	suite.servers["intermittent"] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		shouldFail := requestCount%3 == 0 // 每3个请求失败一次
		mu.Unlock()

		if shouldFail {
			http.Error(w, "Intermittent failure", http.StatusInternalServerError)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			w.Write([]byte(`{"id":"intermittent","object":"chat.completion","choices":[{"message":{"content":"Intermittent success"}}]}`))
		}
	}))
}

// CreateConfig 创建测试配置
func (suite *FunctionalTestSuite) CreateConfig() {
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
			Timeout:              10 * time.Second,
			MaxSuspendedRequests: 50,
		},
		Group: config.GroupConfig{
			AutoSwitchBetweenGroups: false, // 手动模式
			Cooldown:                5 * time.Second,
		},
		Retry: config.RetryConfig{
			MaxAttempts: 3,
			BaseDelay:   200 * time.Millisecond,
			MaxDelay:    2 * time.Second,
			Multiplier:  2.0,
		},
		Health: config.HealthConfig{
			CheckInterval: 1 * time.Second,
			Timeout:       3 * time.Second,
			HealthPath:    "/v1/models",
		},
		Endpoints: []config.EndpointConfig{
			{
				Name:          "primary-healthy",
				URL:           suite.servers["healthy"].URL,
				Group:         "primary",
				GroupPriority: 1,
				Priority:      1,
				Timeout:       4 * time.Second,
				Token:         "test-token-primary",
			},
			{
				Name:          "primary-unhealthy",
				URL:           suite.servers["unhealthy"].URL,
				Group:         "primary",
				GroupPriority: 1,
				Priority:      2,
				Timeout:       4 * time.Second,
				Token:         "test-token-primary",
			},
			{
				Name:          "backup-healthy",
				URL:           suite.servers["healthy"].URL,
				Group:         "backup",
				GroupPriority: 2,
				Priority:      1,
				Timeout:       4 * time.Second,
				Token:         "test-token-backup",
			},
			{
				Name:          "slow-endpoint",
				URL:           suite.servers["slow"].URL,
				Group:         "slow",
				GroupPriority: 3,
				Priority:      1,
				Timeout:       2 * time.Second, // 短超时用于测试
				Token:         "test-token-slow",
			},
			{
				Name:          "sse-endpoint",
				URL:           suite.servers["sse"].URL,
				Group:         "sse",
				GroupPriority: 4,
				Priority:      1,
				Timeout:       4 * time.Second,
				Token:         "test-token-sse",
			},
			{
				Name:          "intermittent",
				URL:           suite.servers["intermittent"].URL,
				Group:         "intermittent",
				GroupPriority: 5,
				Priority:      1,
				Timeout:       4 * time.Second,
				Token:         "test-token-intermittent",
			},
		},
	}
}

// SetupComponents 设置系统组件
func (suite *FunctionalTestSuite) SetupComponents() error {
	// 创建端点管理器
	suite.endpointManager = endpoint.NewManager(suite.config)
	suite.endpointManager.Start()

	// 创建代理处理器
	suite.proxyHandler = proxy.NewHandler(suite.endpointManager, suite.config)
	suite.monitoring = middleware.NewMonitoringMiddleware(suite.endpointManager)
	suite.proxyHandler.SetMonitoringMiddleware(suite.monitoring)

	// 创建Web服务器
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	// 创建空的UsageTracker用于测试
	usageTracker, _ := tracking.NewUsageTracker(&tracking.Config{Enabled: false})
	// 创建EventBus
	eventBus := events.NewEventBus(logger)
	suite.webServer = web.NewWebServer(suite.config, suite.endpointManager, suite.monitoring, usageTracker, logger, time.Now(), "functional-test.yaml", eventBus)

	err := suite.webServer.Start()
	if err != nil {
		return fmt.Errorf("failed to start web server: %v", err)
	}

	// 等待组件初始化
	time.Sleep(3 * time.Second)
	return nil
}

// Cleanup 清理资源
func (suite *FunctionalTestSuite) Cleanup() {
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

// TestRequestSuspendFunctionalVerification 请求挂起功能验证测试
func TestRequestSuspendFunctionalVerification(t *testing.T) {
	suite := NewFunctionalTestSuite()
	defer suite.Cleanup()

	// 设置测试环境
	suite.SetupServers()
	suite.CreateConfig()
	err := suite.SetupComponents()
	if err != nil {
		t.Fatalf("Failed to setup test suite: %v", err)
	}

	t.Run("Manual mode suspend verification", func(t *testing.T) {
		// 1. 确认手动模式配置
		if suite.config.Group.AutoSwitchBetweenGroups {
			t.Error("Expected manual mode (AutoSwitchBetweenGroups=false)")
		}

		// 2. 手动激活一个故障组
		groupManager := suite.endpointManager.GetGroupManager()
		err := groupManager.ManualActivateGroup("primary") // 包含健康和不健康端点
		if err != nil {
			t.Fatalf("Failed to activate primary group: %v", err)
		}

		// 3. 发送请求，应该路由到健康端点并成功
		body := `{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Manual mode test"}]}`
		req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")

		rr := httptest.NewRecorder()
		suite.proxyHandler.ServeHTTP(rr, req)

		// 4. 验证请求成功（因为primary组有健康端点）
		if rr.Code != 200 {
			t.Errorf("Expected status 200, got %d. Response: %s", rr.Code, rr.Body.String())
		}

		// 5. 检查响应内容
		var response map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		if err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if choices, ok := response["choices"].([]interface{}); ok && len(choices) > 0 {
			choice := choices[0].(map[string]interface{})
			message := choice["message"].(map[string]interface{})
			content := message["content"].(string)
			if !strings.Contains(content, "Success from healthy server") {
				t.Errorf("Unexpected response content: %s", content)
			}
		}
	})

	t.Run("Group switching and suspend flow", func(t *testing.T) {
		// 1. 激活只有故障端点的组（如果存在的话，或模拟所有端点都故障）
		groupManager := suite.endpointManager.GetGroupManager()
		
		// 2. 创建一个会导致挂起的场景
		// 先暂停所有健康端点，只激活不健康端点
		err := groupManager.ManualPauseGroup("primary", time.Hour) // 暂停1小时
		if err != nil {
			t.Logf("Could not pause primary group: %v", err)
		}

		// 激活一个预期会失败的组，但为了测试，我们先发送请求看看情况
		body := `{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Suspend flow test"}]}`
		req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")

		rr := httptest.NewRecorder()
		requestDone := make(chan bool, 1)
		var responseStatus int

		// 异步处理请求
		go func() {
			suite.proxyHandler.ServeHTTP(rr, req)
			responseStatus = rr.Code
			requestDone <- true
		}()

		// 等待短时间
		select {
		case <-requestDone:
			t.Logf("Request completed with status: %d", responseStatus)
		case <-time.After(2 * time.Second):
			// 可能被挂起，激活健康组
			err = groupManager.ManualActivateGroup("backup")
			if err != nil {
				t.Fatalf("Failed to activate backup group: %v", err)
			}

			// 等待请求完成
			select {
			case <-requestDone:
				if responseStatus != 200 {
					t.Errorf("Expected status 200 after group activation, got %d", responseStatus)
				}
				t.Log("Request successfully resumed after group activation")
			case <-time.After(3 * time.Second):
				t.Error("Request timed out even after group activation")
			}
		}
	})

	t.Run("Token statistics accuracy", func(t *testing.T) {
		// 激活健康组
		groupManager := suite.endpointManager.GetGroupManager()
		err := groupManager.ManualActivateGroup("backup")
		if err != nil {
			t.Fatalf("Failed to activate backup group: %v", err)
		}

		// 获取初始统计
		initialStats := suite.monitoring.GetSuspendedRequestStats()
		initialTotal := int64(0)
		if val, ok := initialStats["total_suspended_requests"].(int64); ok {
			initialTotal = val
		}

		// 发送多个请求
		numRequests := 5
		for i := 0; i < numRequests; i++ {
			body := fmt.Sprintf(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Stats test %d"}]}`, i)
			req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer test-token")

			rr := httptest.NewRecorder()
			suite.proxyHandler.ServeHTTP(rr, req)

			if rr.Code != 200 {
				t.Logf("Request %d failed with status %d", i, rr.Code)
			}
		}

		// 获取最终统计
		finalStats := suite.monitoring.GetSuspendedRequestStats()
		
		t.Logf("Token statistics test results:")
		for key, value := range finalStats {
			t.Logf("  %s: %v", key, value)
		}

		// 验证统计数据的一致性
		if totalRequests, ok := finalStats["total_suspended_requests"].(int64); ok {
			if totalRequests < initialTotal {
				t.Errorf("Total suspended requests should not decrease: initial=%d, final=%d", initialTotal, totalRequests)
			}
		}

		// 验证成功率计算
		if successRate, ok := finalStats["success_rate"].(float64); ok {
			if successRate < 0 || successRate > 100 {
				t.Errorf("Success rate should be between 0 and 100, got: %.2f", successRate)
			}
		}
	})

	t.Run("SSE streaming suspend handling", func(t *testing.T) {
		// 激活SSE组
		groupManager := suite.endpointManager.GetGroupManager()
		err := groupManager.ManualActivateGroup("sse")
		if err != nil {
			t.Fatalf("Failed to activate SSE group: %v", err)
		}

		// 发送SSE请求
		body := `{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"SSE test"}],"stream":true}`
		req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")
		req.Header.Set("Accept", "text/event-stream")

		rr := httptest.NewRecorder()
		suite.proxyHandler.ServeHTTP(rr, req)

		// 验证SSE响应
		if rr.Code != 200 {
			t.Errorf("Expected status 200 for SSE, got %d", rr.Code)
		}

		responseBody := rr.Body.String()
		if !strings.Contains(responseBody, "data:") {
			t.Error("SSE response should contain 'data:' events")
		}

		if !strings.Contains(responseBody, "Hello") || !strings.Contains(responseBody, "World") {
			t.Error("SSE response should contain expected content")
		}

		if !strings.Contains(responseBody, "[DONE]") {
			t.Error("SSE response should contain [DONE] marker")
		}

		t.Logf("SSE response preview: %s", responseBody[:minInt(200, len(responseBody))])
	})

	t.Run("Backward compatibility verification", func(t *testing.T) {
		// 创建禁用挂起功能的配置
		disabledConfig := *suite.config
		disabledConfig.RequestSuspend.Enabled = false
		
		// 更新配置
		suite.proxyHandler.UpdateConfig(&disabledConfig)
		suite.endpointManager.UpdateConfig(&disabledConfig)

		// 激活健康组
		groupManager := suite.endpointManager.GetGroupManager()
		err := groupManager.ManualActivateGroup("backup")
		if err != nil {
			t.Fatalf("Failed to activate backup group: %v", err)
		}

		// 发送请求
		body := `{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Backward compatibility test"}]}`
		req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")

		rr := httptest.NewRecorder()
		suite.proxyHandler.ServeHTTP(rr, req)

		// 验证请求正常处理
		if rr.Code != 200 {
			t.Errorf("Expected status 200 with disabled suspend, got %d", rr.Code)
		}

		// 验证没有挂起请求
		retryHandler := suite.proxyHandler.GetRetryHandler()
		suspendedCount := retryHandler.GetSuspendedRequestsCount()
		if suspendedCount != 0 {
			t.Errorf("Expected 0 suspended requests when feature is disabled, got %d", suspendedCount)
		}

		// 恢复原始配置
		suite.proxyHandler.UpdateConfig(suite.config)
		suite.endpointManager.UpdateConfig(suite.config)
		
		t.Log("Backward compatibility test passed")
	})

	t.Run("Web API integration verification", func(t *testing.T) {
		webPort := suite.config.Web.Port
		if webPort == 0 {
			webPort = 8088
		}
		webAddr := fmt.Sprintf("http://localhost:%d", webPort)

		// 测试状态API
		resp, err := http.Get(webAddr + "/api/v1/status")
		if err != nil {
			t.Logf("Web API not accessible, skipping test: %v", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			t.Errorf("Expected status 200 from web API, got %d", resp.StatusCode)
		}

		var status map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&status)
		if err != nil {
			t.Fatalf("Failed to decode status response: %v", err)
		}

		// 验证状态包含必要字段
		requiredFields := []string{"uptime", "total_requests", "active_connections"}
		for _, field := range requiredFields {
			if _, exists := status[field]; !exists {
				t.Errorf("Status response missing required field: %s", field)
			}
		}

		// 验证包含挂起请求信息
		if _, exists := status["suspended_requests"]; !exists {
			t.Error("Status response should include suspended_requests field")
		}

		t.Logf("Web API verification passed. Status fields: %v", getKeys(status))

		// 测试组管理API
		groupsResp, err := http.Get(webAddr + "/api/v1/groups")
		if err != nil {
			t.Fatalf("Failed to get groups from web API: %v", err)
		}
		defer groupsResp.Body.Close()

		if groupsResp.StatusCode != 200 {
			t.Errorf("Expected status 200 from groups API, got %d", groupsResp.StatusCode)
		}

		var groups []map[string]interface{}
		err = json.NewDecoder(groupsResp.Body).Decode(&groups)
		if err != nil {
			t.Fatalf("Failed to decode groups response: %v", err)
		}

		if len(groups) == 0 {
			t.Error("Groups API should return at least one group")
		}

		t.Logf("Groups API verification passed. Found %d groups", len(groups))
	})
}

// TestEdgeCasesAndErrorConditions 边界情况和错误条件测试
func TestEdgeCasesAndErrorConditions(t *testing.T) {
	suite := NewFunctionalTestSuite()
	defer suite.Cleanup()

	suite.SetupServers()
	suite.CreateConfig()
	err := suite.SetupComponents()
	if err != nil {
		t.Fatalf("Failed to setup test suite: %v", err)
	}

	t.Run("Maximum suspended requests boundary", func(t *testing.T) {
		// 设置较小的限制用于测试
		testConfig := *suite.config
		testConfig.RequestSuspend.MaxSuspendedRequests = 2
		suite.proxyHandler.UpdateConfig(&testConfig)

		// 暂停所有组，强制挂起
		groupManager := suite.endpointManager.GetGroupManager()
		groups := []string{"primary", "backup", "slow", "sse", "intermittent"}
		for _, group := range groups {
			groupManager.ManualPauseGroup(group, time.Hour)
		}

		// 发送超过限制的请求数
		numRequests := 5
		results := make([]int, numRequests)
		var wg sync.WaitGroup

		for i := 0; i < numRequests; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				
				body := fmt.Sprintf(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Boundary test %d"}]}`, index)
				req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer test-token")

				rr := httptest.NewRecorder()
				suite.proxyHandler.ServeHTTP(rr, req)
				results[index] = rr.Code
			}(i)
		}

		// 等待一段时间让请求被处理或拒绝
		time.Sleep(2 * time.Second)

		// 激活一个健康组
		err := groupManager.ManualActivateGroup("backup")
		if err != nil {
			t.Fatalf("Failed to activate backup group: %v", err)
		}

		wg.Wait()

		// 分析结果
		successCount := 0
		errorCount := 0
		for _, code := range results {
			if code >= 200 && code < 300 {
				successCount++
			} else {
				errorCount++
			}
		}

		t.Logf("边界测试结果: %d成功, %d错误 (总共%d请求)", successCount, errorCount, numRequests)

		// 恢复原配置
		suite.proxyHandler.UpdateConfig(suite.config)
	})

	t.Run("Suspend timeout edge case", func(t *testing.T) {
		// 设置极短的超时时间
		testConfig := *suite.config
		testConfig.RequestSuspend.Timeout = 500 * time.Millisecond
		suite.proxyHandler.UpdateConfig(&testConfig)

		// 暂停所有组
		groupManager := suite.endpointManager.GetGroupManager()
		groupManager.ManualPauseGroup("primary", time.Hour)
		groupManager.ManualPauseGroup("backup", time.Hour)

		body := `{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Timeout edge test"}]}`
		req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")

		rr := httptest.NewRecorder()
		start := time.Now()
		suite.proxyHandler.ServeHTTP(rr, req)
		elapsed := time.Since(start)

		// 验证请求在超时时间附近返回
		if elapsed < 400*time.Millisecond || elapsed > 1500*time.Millisecond {
			t.Errorf("请求超时时间不符合预期: %v (期望大约500ms)", elapsed)
		}

		// 验证返回错误状态
		if rr.Code < 400 {
			t.Errorf("超时请求应返回错误状态，实际: %d", rr.Code)
		}

		t.Logf("超时测试通过: 耗时%v, 状态%d", elapsed, rr.Code)

		// 恢复原配置
		suite.proxyHandler.UpdateConfig(suite.config)
	})

	t.Run("Intermittent failure handling", func(t *testing.T) {
		// 激活间歇性故障组
		groupManager := suite.endpointManager.GetGroupManager()
		err := groupManager.ManualActivateGroup("intermittent")
		if err != nil {
			t.Fatalf("Failed to activate intermittent group: %v", err)
		}

		// 发送多个请求观察故障模式
		numRequests := 9 // 应该有3次失败（每3次失败1次）
		successCount := 0
		failureCount := 0

		for i := 0; i < numRequests; i++ {
			body := fmt.Sprintf(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Intermittent test %d"}]}`, i)
			req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer test-token")

			rr := httptest.NewRecorder()
			suite.proxyHandler.ServeHTTP(rr, req)

			if rr.Code == 200 {
				successCount++
			} else {
				failureCount++
			}

			// 短暂延迟以避免请求过快
			time.Sleep(100 * time.Millisecond)
		}

		t.Logf("间歇性故障测试: %d成功, %d失败 (总共%d请求)", successCount, failureCount, numRequests)

		// 验证有成功和失败的请求
		if successCount == 0 {
			t.Error("应该有一些成功的请求")
		}
		if failureCount == 0 {
			t.Error("应该有一些失败的请求（基于间歇性故障模式）")
		}
	})
}

// minInt 返回两个整数的最小值
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func getKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}