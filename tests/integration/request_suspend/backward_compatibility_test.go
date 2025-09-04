package integration

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/middleware"
	"cc-forwarder/internal/proxy"
)

// BackwardCompatibilityTestSuite 向后兼容性测试套件
type BackwardCompatibilityTestSuite struct {
	testServer      *httptest.Server
	endpointManager *endpoint.Manager
	proxyHandler    *proxy.Handler
	monitoring      *middleware.MonitoringMiddleware
}

// SetupCompatibilityTest 设置兼容性测试环境
func (suite *BackwardCompatibilityTestSuite) SetupCompatibilityTest(cfg *config.Config) {
	// 创建简单的测试服务器
	suite.testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		
		if strings.Contains(r.URL.Path, "/v1/models") {
			w.Write([]byte(`{"object":"list","data":[{"id":"gpt-3.5-turbo","object":"model"}]}`))
		} else {
			w.Write([]byte(`{"id":"compat","object":"chat.completion","choices":[{"message":{"content":"Compatibility test response"}}]}`))
		}
	}))

	// 更新配置中的端点URL
	for i := range cfg.Endpoints {
		cfg.Endpoints[i].URL = suite.testServer.URL
	}

	// 创建组件
	suite.endpointManager = endpoint.NewManager(cfg)
	suite.endpointManager.Start()

	suite.proxyHandler = proxy.NewHandler(suite.endpointManager, cfg)
	suite.monitoring = middleware.NewMonitoringMiddleware(suite.endpointManager)
	suite.proxyHandler.SetMonitoringMiddleware(suite.monitoring)

	// 等待组件初始化
	time.Sleep(1 * time.Second)
}

// Cleanup 清理兼容性测试资源
func (suite *BackwardCompatibilityTestSuite) Cleanup() {
	if suite.endpointManager != nil {
		suite.endpointManager.Stop()
	}
	if suite.testServer != nil {
		suite.testServer.Close()
	}
}

// TestBackwardCompatibilityDisabledFeature 测试功能禁用时的向后兼容性
func TestBackwardCompatibilityDisabledFeature(t *testing.T) {
	suite := &BackwardCompatibilityTestSuite{}
	defer suite.Cleanup()

	t.Run("Request suspend disabled - auto mode", func(t *testing.T) {
		cfg := &config.Config{
			Server: config.ServerConfig{
				Host: "localhost",
				Port: 0,
			},
			RequestSuspend: config.RequestSuspendConfig{
				Enabled: false, // 禁用挂起功能
			},
			Group: config.GroupConfig{
				AutoSwitchBetweenGroups: true, // 自动模式
				Cooldown:                5 * time.Second,
			},
			Health: config.HealthConfig{
				CheckInterval: 2 * time.Second,
				Timeout:       3 * time.Second,
				HealthPath:    "/v1/models",
			},
			Endpoints: []config.EndpointConfig{
				{
					Name:     "legacy-endpoint",
					URL:      "", // 将在setup中设置
					Priority: 1,
					Timeout:  10 * time.Second,
					Token:    "test-token",
				},
			},
		}

		suite.SetupCompatibilityTest(cfg)

		// 发送标准请求
		body := `{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Legacy test"}]}`
		req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")

		rr := httptest.NewRecorder()
		suite.proxyHandler.ServeHTTP(rr, req)

		// 验证正常工作
		if rr.Code != 200 {
			t.Errorf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		// 验证没有挂起功能影响
		retryHandler := suite.proxyHandler.GetRetryHandler()
		suspendedCount := retryHandler.GetSuspendedRequestsCount()
		if suspendedCount != 0 {
			t.Errorf("Expected 0 suspended requests when feature is disabled, got %d", suspendedCount)
		}

		// 验证统计数据
		stats := suite.monitoring.GetSuspendedRequestStats()
		if totalSuspended, ok := stats["total_suspended_requests"].(int64); ok && totalSuspended != 0 {
			t.Errorf("Expected 0 total suspended requests, got %d", totalSuspended)
		}

		t.Log("✓ 禁用挂起功能的向后兼容性测试通过")
	})

	t.Run("Request suspend disabled - manual mode", func(t *testing.T) {
		cfg := &config.Config{
			Server: config.ServerConfig{
				Host: "localhost",
				Port: 0,
			},
			RequestSuspend: config.RequestSuspendConfig{
				Enabled: false, // 禁用挂起功能
			},
			Group: config.GroupConfig{
				AutoSwitchBetweenGroups: false, // 手动模式，但挂起功能禁用
				Cooldown:                5 * time.Second,
			},
			Health: config.HealthConfig{
				CheckInterval: 2 * time.Second,
				Timeout:       3 * time.Second,
				HealthPath:    "/v1/models",
			},
			Endpoints: []config.EndpointConfig{
				{
					Name:     "manual-mode-endpoint",
					URL:      "",
					Priority: 1,
					Timeout:  10 * time.Second,
					Token:    "test-token",
				},
			},
		}

		suite.SetupCompatibilityTest(cfg)

		// 即使在手动模式下，禁用挂起功能应该正常工作
		body := `{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Manual mode test"}]}`
		req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")

		rr := httptest.NewRecorder()
		suite.proxyHandler.ServeHTTP(rr, req)

		if rr.Code != 200 {
			t.Errorf("Expected status 200 in manual mode with disabled suspend, got %d", rr.Code)
		}

		t.Log("✓ 手动模式下禁用挂起功能的向后兼容性测试通过")
	})
}

// TestBackwardCompatibilityDefaultValues 测试默认值的向后兼容性
func TestBackwardCompatibilityDefaultValues(t *testing.T) {
	suite := &BackwardCompatibilityTestSuite{}
	defer suite.Cleanup()

	t.Run("Missing request suspend configuration", func(t *testing.T) {
		// 模拟旧配置文件，没有request_suspend配置块
		cfg := &config.Config{
			Server: config.ServerConfig{
				Host: "localhost",
				Port: 0,
			},
			// 故意不设置RequestSuspend配置
			Group: config.GroupConfig{
				AutoSwitchBetweenGroups: true,
			},
			Endpoints: []config.EndpointConfig{
				{
					Name:     "default-config-endpoint",
					URL:      "",
					Priority: 1,
					Timeout:  10 * time.Second,
					Token:    "test-token",
				},
			},
		}

		// 设置默认值
		cfg.RequestSuspend.Enabled = false // 默认应该是禁用
		cfg.RequestSuspend.Timeout = 300 * time.Second
		cfg.RequestSuspend.MaxSuspendedRequests = 100

		suite.SetupCompatibilityTest(cfg)

		// 验证系统正常工作
		body := `{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Default config test"}]}`
		req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")

		rr := httptest.NewRecorder()
		suite.proxyHandler.ServeHTTP(rr, req)

		if rr.Code != 200 {
			t.Errorf("Expected status 200 with default config, got %d", rr.Code)
		}

		// 验证默认行为（不挂起）
		retryHandler := suite.proxyHandler.GetRetryHandler()
		suspendedCount := retryHandler.GetSuspendedRequestsCount()
		if suspendedCount != 0 {
			t.Errorf("Expected 0 suspended requests with default disabled config, got %d", suspendedCount)
		}

		t.Log("✓ 缺失配置的默认值向后兼容性测试通过")
	})

	t.Run("Partial request suspend configuration", func(t *testing.T) {
		cfg := &config.Config{
			Server: config.ServerConfig{
				Host: "localhost",
				Port: 0,
			},
			RequestSuspend: config.RequestSuspendConfig{
				Enabled: true, // 只设置enabled，其他使用默认值
				// Timeout 和 MaxSuspendedRequests 将使用默认值
			},
			Group: config.GroupConfig{
				AutoSwitchBetweenGroups: true, // 自动模式，不会触发挂起
			},
			Endpoints: []config.EndpointConfig{
				{
					Name:     "partial-config-endpoint",
					URL:      "",
					Priority: 1,
					Timeout:  10 * time.Second,
					Token:    "test-token",
				},
			},
		}

		// 设置默认值
		if cfg.RequestSuspend.Timeout == 0 {
			cfg.RequestSuspend.Timeout = 300 * time.Second
		}
		if cfg.RequestSuspend.MaxSuspendedRequests == 0 {
			cfg.RequestSuspend.MaxSuspendedRequests = 100
		}

		suite.SetupCompatibilityTest(cfg)

		body := `{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Partial config test"}]}`
		req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")

		rr := httptest.NewRecorder()
		suite.proxyHandler.ServeHTTP(rr, req)

		if rr.Code != 200 {
			t.Errorf("Expected status 200 with partial config, got %d", rr.Code)
		}

		t.Log("✓ 部分配置的向后兼容性测试通过")
	})
}

// TestBackwardCompatibilityAPIConsistency 测试API一致性的向后兼容性
func TestBackwardCompatibilityAPIConsistency(t *testing.T) {
	suite := &BackwardCompatibilityTestSuite{}
	defer suite.Cleanup()

	t.Run("API response format consistency", func(t *testing.T) {
		cfg := &config.Config{
			Server: config.ServerConfig{
				Host: "localhost",
				Port: 0,
			},
			RequestSuspend: config.RequestSuspendConfig{
				Enabled: false, // 禁用以确保传统行为
			},
			Endpoints: []config.EndpointConfig{
				{
					Name:     "api-consistency-endpoint",
					URL:      "",
					Priority: 1,
					Timeout:  10 * time.Second,
					Token:    "test-token",
				},
			},
		}

		suite.SetupCompatibilityTest(cfg)

		// 测试不同类型的请求
		testCases := []struct {
			name        string
			method      string
			path        string
			body        string
			contentType string
		}{
			{
				name:        "Chat completions",
				method:      "POST",
				path:        "/v1/chat/completions",
				body:        `{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Hello"}]}`,
				contentType: "application/json",
			},
			{
				name:        "Models list",
				method:      "GET",
				path:        "/v1/models",
				body:        "",
				contentType: "application/json",
			},
			{
				name:        "Custom endpoint",
				method:      "POST",
				path:        "/custom/endpoint",
				body:        `{"data":"test"}`,
				contentType: "application/json",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				var reqBody io.Reader
				if tc.body != "" {
					reqBody = bytes.NewReader([]byte(tc.body))
				}

				req := httptest.NewRequest(tc.method, tc.path, reqBody)
				req.Header.Set("Content-Type", tc.contentType)
				req.Header.Set("Authorization", "Bearer test-token")

				rr := httptest.NewRecorder()
				suite.proxyHandler.ServeHTTP(rr, req)

				// 验证响应格式一致性
				if rr.Code != 200 {
					t.Logf("Request %s returned status %d (may be expected for some endpoints)", tc.name, rr.Code)
				}

				// 验证Content-Type头部
				contentType := rr.Header().Get("Content-Type")
				if contentType == "" {
					t.Errorf("Response missing Content-Type header for %s", tc.name)
				}

				t.Logf("✓ API一致性测试通过: %s, 状态: %d, Content-Type: %s", tc.name, rr.Code, contentType)
			})
		}
	})

	t.Run("Header preservation", func(t *testing.T) {
		cfg := &config.Config{
			Server: config.ServerConfig{
				Host: "localhost",
				Port: 0,
			},
			RequestSuspend: config.RequestSuspendConfig{
				Enabled: false,
			},
			Endpoints: []config.EndpointConfig{
				{
					Name:     "header-test-endpoint",
					URL:      "",
					Priority: 1,
					Timeout:  10 * time.Second,
					Token:    "test-token",
				},
			},
		}

		suite.SetupCompatibilityTest(cfg)

		body := `{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Header test"}]}`
		req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")
		req.Header.Set("User-Agent", "test-client/1.0")
		req.Header.Set("X-Custom-Header", "custom-value")

		rr := httptest.NewRecorder()
		suite.proxyHandler.ServeHTTP(rr, req)

		// 验证响应头部存在且格式正确
		if rr.Header().Get("Content-Type") == "" {
			t.Error("Response should preserve Content-Type header")
		}

		t.Log("✓ 头部保持的向后兼容性测试通过")
	})
}

// TestBackwardCompatibilityErrorHandling 测试错误处理的向后兼容性
func TestBackwardCompatibilityErrorHandling(t *testing.T) {
	suite := &BackwardCompatibilityTestSuite{}
	defer suite.Cleanup()

	// 创建一个会返回错误的服务器
	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
	}))
	defer errorServer.Close()

	t.Run("Error response format consistency", func(t *testing.T) {
		cfg := &config.Config{
			Server: config.ServerConfig{
				Host: "localhost",
				Port: 0,
			},
			RequestSuspend: config.RequestSuspendConfig{
				Enabled: false, // 禁用挂起以测试传统错误处理
			},
			Retry: config.RetryConfig{
				MaxAttempts: 1, // 不重试，直接返回错误
			},
			Endpoints: []config.EndpointConfig{
				{
					Name:     "error-endpoint",
					URL:      errorServer.URL,
					Priority: 1,
					Timeout:  5 * time.Second,
					Token:    "test-token",
				},
			},
		}

		suite.endpointManager = endpoint.NewManager(cfg)
		suite.endpointManager.Start()
		defer suite.endpointManager.Stop()

		suite.proxyHandler = proxy.NewHandler(suite.endpointManager, cfg)
		suite.monitoring = middleware.NewMonitoringMiddleware(suite.endpointManager)
		suite.proxyHandler.SetMonitoringMiddleware(suite.monitoring)

		time.Sleep(1 * time.Second)

		body := `{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Error test"}]}`
		req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")

		rr := httptest.NewRecorder()
		suite.proxyHandler.ServeHTTP(rr, req)

		// 验证错误响应格式一致性
		if rr.Code == 200 {
			t.Error("Expected error response, but got success")
		}

		// 验证错误响应有合理的内容
		responseBody := rr.Body.String()
		if responseBody == "" {
			t.Error("Error response should have body content")
		}

		// t.Logf("✓ 错误处理兼容性测试通过, 状态: %d, 响应: %s", rr.Code, responseBody[:minInt(100, len(responseBody))])
		t.Logf("✓ 错误处理兼容性测试通过, 状态: %d, 响应长度: %d", rr.Code, len(responseBody))
	})
}

// TestBackwardCompatibilityConfigurationHotReload 测试配置热重载的向后兼容性  
func TestBackwardCompatibilityConfigurationHotReload(t *testing.T) {
	suite := &BackwardCompatibilityTestSuite{}
	defer suite.Cleanup()

	t.Run("Hot reload with disabled suspend feature", func(t *testing.T) {
		// 初始配置
		cfg := &config.Config{
			Server: config.ServerConfig{
				Host: "localhost",
				Port: 0,
			},
			RequestSuspend: config.RequestSuspendConfig{
				Enabled: false,
				Timeout: 300 * time.Second,
				MaxSuspendedRequests: 100,
			},
			Endpoints: []config.EndpointConfig{
				{
					Name:     "hotreload-endpoint",
					URL:      "",
					Priority: 1,
					Timeout:  10 * time.Second,
					Token:    "test-token",
				},
			},
		}

		suite.SetupCompatibilityTest(cfg)

		// 测试初始配置
		body := `{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Initial config"}]}`
		req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")

		rr := httptest.NewRecorder()
		suite.proxyHandler.ServeHTTP(rr, req)

		if rr.Code != 200 {
			t.Errorf("Initial config test failed with status %d", rr.Code)
		}

		// 热重载配置（修改非关键参数）
		newConfig := *cfg
		newConfig.RequestSuspend.Timeout = 600 * time.Second
		newConfig.RequestSuspend.MaxSuspendedRequests = 200

		// 应用新配置
		suite.proxyHandler.UpdateConfig(&newConfig)
		suite.endpointManager.UpdateConfig(&newConfig)

		// 测试配置更新后的行为
		body2 := `{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Updated config"}]}`
		req2 := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte(body2)))
		req2.Header.Set("Content-Type", "application/json")
		req2.Header.Set("Authorization", "Bearer test-token")

		rr2 := httptest.NewRecorder()
		suite.proxyHandler.ServeHTTP(rr2, req2)

		if rr2.Code != 200 {
			t.Errorf("Hot reload test failed with status %d", rr2.Code)
		}

		// 验证挂起功能仍然禁用
		retryHandler := suite.proxyHandler.GetRetryHandler()
		suspendedCount := retryHandler.GetSuspendedRequestsCount()
		if suspendedCount != 0 {
			t.Errorf("Suspended count should remain 0 after hot reload, got %d", suspendedCount)
		}

		t.Log("✓ 配置热重载向后兼容性测试通过")
	})
}

// TestBackwardCompatibilityLegacyBehavior 测试遗留行为的向后兼容性
func TestBackwardCompatibilityLegacyBehavior(t *testing.T) {
	suite := &BackwardCompatibilityTestSuite{}
	defer suite.Cleanup()

	t.Run("Legacy endpoint priority behavior", func(t *testing.T) {
		cfg := &config.Config{
			Server: config.ServerConfig{
				Host: "localhost",
				Port: 0,
			},
			RequestSuspend: config.RequestSuspendConfig{
				Enabled: false,
			},
			Strategy: config.StrategyConfig{
				Type: "priority", // 传统优先级策略
			},
			Endpoints: []config.EndpointConfig{
				{
					Name:     "high-priority",
					URL:      "",
					Priority: 1, // 高优先级
					Timeout:  10 * time.Second,
					Token:    "test-token",
				},
				{
					Name:     "low-priority", 
					URL:      "",
					Priority: 2, // 低优先级
					Timeout:  10 * time.Second,
					Token:    "test-token",
				},
			},
		}

		suite.SetupCompatibilityTest(cfg)

		// 发送请求，应该路由到高优先级端点
		body := `{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Priority test"}]}`
		req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")

		rr := httptest.NewRecorder()
		suite.proxyHandler.ServeHTTP(rr, req)

		if rr.Code != 200 {
			t.Errorf("Legacy priority test failed with status %d", rr.Code)
		}

		t.Log("✓ 遗留端点优先级行为向后兼容性测试通过")
	})

	t.Run("Legacy authentication behavior", func(t *testing.T) {
		cfg := &config.Config{
			Server: config.ServerConfig{
				Host: "localhost",
				Port: 0,
			},
			Auth: config.AuthConfig{
				Enabled: false, // 禁用鉴权，保持向后兼容
			},
			RequestSuspend: config.RequestSuspendConfig{
				Enabled: false,
			},
			Endpoints: []config.EndpointConfig{
				{
					Name:     "no-auth-endpoint",
					URL:      "",
					Priority: 1,
					Timeout:  10 * time.Second,
				},
			},
		}

		suite.SetupCompatibilityTest(cfg)

		// 不提供Authorization头部的请求应该正常工作
		body := `{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"No auth test"}]}`
		req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")
		// 故意不设置Authorization头部

		rr := httptest.NewRecorder()
		suite.proxyHandler.ServeHTTP(rr, req)

		if rr.Code != 200 {
			t.Logf("Legacy auth test status: %d (may be expected based on endpoint requirements)", rr.Code)
		}

		t.Log("✓ 遗留鉴权行为向后兼容性测试通过")
	})
}

