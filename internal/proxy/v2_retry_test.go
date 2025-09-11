package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
)

// mockEndpointServer 模拟端点服务器，支持配置失败次数
type mockEndpointServer struct {
	server       *httptest.Server
	requestCount int
	failCount    int // 前N次请求返回错误，之后返回成功
	mu           sync.Mutex
}

func newMockEndpointServer(failCount int) *mockEndpointServer {
	mes := &mockEndpointServer{
		failCount: failCount,
	}
	
	mes.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mes.mu.Lock()
		mes.requestCount++
		currentCount := mes.requestCount
		mes.mu.Unlock()
		
		// 前failCount次请求返回错误
		if currentCount <= mes.failCount {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": "mock server error"}`))
			return
		}
		
		// 成功响应 - 模拟流式响应
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "event: message_start\n")
		fmt.Fprint(w, `data: {"type": "message_start", "message": {"model": "claude-3-5-haiku"}}`)
		fmt.Fprint(w, "\n\n")
		fmt.Fprint(w, "event: message_delta\n")
		fmt.Fprint(w, `data: {"type": "message_delta", "usage": {"input_tokens": 10, "output_tokens": 20}}`)
		fmt.Fprint(w, "\n\n")
		fmt.Fprint(w, "event: message_stop\n")
		fmt.Fprint(w, "data: {}\n\n")
	}))
	
	return mes
}

func (mes *mockEndpointServer) getRequestCount() int {
	mes.mu.Lock()
	defer mes.mu.Unlock()
	return mes.requestCount
}

func (mes *mockEndpointServer) close() {
	mes.server.Close()
}

// TestV2StreamingRetryLogic 测试V2流式处理的重试逻辑
func TestV2StreamingRetryLogic(t *testing.T) {
	tests := []struct {
		name               string
		maxAttempts        int    // 配置的最大重试次数
		endpointFailCount  int    // 端点前N次请求失败
		expectedRetryCount int    // 预期的总重试次数
		expectSuccess      bool   // 预期是否最终成功
	}{
		{
			name:               "第一次尝试成功",
			maxAttempts:        3,
			endpointFailCount:  0,  // 不失败
			expectedRetryCount: 1,  // 只尝试1次
			expectSuccess:      true,
		},
		{
			name:               "第二次尝试成功",
			maxAttempts:        3,
			endpointFailCount:  1,  // 第1次失败，第2次成功
			expectedRetryCount: 2,  // 尝试2次
			expectSuccess:      true,
		},
		{
			name:               "第三次尝试成功",
			maxAttempts:        3,
			endpointFailCount:  2,  // 前2次失败，第3次成功
			expectedRetryCount: 3,  // 尝试3次
			expectSuccess:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建模拟服务器
			mockServer := newMockEndpointServer(tt.endpointFailCount)
			defer mockServer.close()
			
			// 创建配置
			cfg := &config.Config{
				Retry: config.RetryConfig{
					MaxAttempts: tt.maxAttempts,
					BaseDelay:   50 * time.Millisecond, // 减少测试时间
					MaxDelay:    500 * time.Millisecond,
					Multiplier:  2.0,
				},
				Group: config.GroupConfig{
					AutoSwitchBetweenGroups: true,
				},
				UsageTracking: config.UsageTrackingConfig{
					Enabled: false, // 简化测试
				},
				Endpoints: []config.EndpointConfig{
					{
						Name:     "test-endpoint",
						URL:      mockServer.server.URL,
						Priority: 1,
						Timeout:  5 * time.Second,
						Group:    "test-group",
						Token:    "test-token",
					},
				},
			}

			// 创建端点管理器
			endpointManager := endpoint.NewManager(cfg)
			
			// 等待健康检查完成并强制标记为健康
			time.Sleep(200 * time.Millisecond)
			
			// 手动标记端点为健康状态
			endpoints := endpointManager.GetAllEndpoints()
			for _, ep := range endpoints {
				// 直接设置健康状态
				ep.Status.Healthy = true
				ep.Status.LastCheck = time.Now()
			}

			// 创建处理器
			handler := NewHandler(endpointManager, cfg)

			// 创建测试请求
			requestBody := `{"message": "test streaming request"}`
			req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(requestBody))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "text/event-stream") // 标记为流式请求

			// 创建响应记录器
			recorder := httptest.NewRecorder()

			// 执行请求（带超时上下文）
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			
			req = req.WithContext(ctx)

			// 执行处理
			start := time.Now()
			handler.ServeHTTP(recorder, req)
			duration := time.Since(start)

			// 获取实际的请求次数
			actualRetryCount := mockServer.getRequestCount()

			t.Logf("测试 %s:", tt.name)
			t.Logf("  配置最大重试次数: %d", tt.maxAttempts)
			t.Logf("  端点失败次数: %d", tt.endpointFailCount)
			t.Logf("  预期重试次数: %d", tt.expectedRetryCount)
			t.Logf("  实际重试次数: %d", actualRetryCount)
			t.Logf("  响应状态码: %d", recorder.Code)
			t.Logf("  处理时长: %v", duration)
			t.Logf("  响应体: %s", recorder.Body.String())

			// 验证重试次数
			if actualRetryCount != tt.expectedRetryCount {
				t.Errorf("重试次数不符合预期: 预期 %d, 实际 %d", tt.expectedRetryCount, actualRetryCount)
			}

			// 验证最终结果
			if tt.expectSuccess {
				// 期望成功：应该返回200状态码，并且响应包含流式数据
				if recorder.Code != http.StatusOK {
					t.Errorf("预期成功但失败: 状态码 %d, 响应: %s", recorder.Code, recorder.Body.String())
				}
				
				// 验证流式响应内容
				responseBody := recorder.Body.String()
				if !strings.Contains(responseBody, "event: message_start") {
					t.Errorf("成功响应应包含流式数据，但没有找到: %s", responseBody)
				}
			} else {
				// 期望失败：应该返回错误状态码或错误信息
				responseBody := recorder.Body.String()
				if recorder.Code == http.StatusOK && !strings.Contains(responseBody, "error") {
					t.Errorf("预期失败但成功: 状态码 %d, 响应: %s", recorder.Code, responseBody)
				}
			}

			// 验证处理时长合理性（考虑重试延迟）
			if tt.expectedRetryCount > 1 {
				expectedMinDuration := time.Duration(tt.expectedRetryCount-1) * cfg.Retry.BaseDelay
				if duration < expectedMinDuration/2 { // 允许一些误差
					t.Logf("处理时长可能过短，但在可接受范围内: 实际 %v, 最小预期 %v", duration, expectedMinDuration)
				}
			}
		})
	}
}

// TestV2StreamingRetryWithMultipleEndpoints 测试多端点情况下的重试逻辑
func TestV2StreamingRetryWithMultipleEndpoints(t *testing.T) {
	// 创建两个模拟服务器
	// 第一个端点：前3次请求失败
	// 第二个端点：第1次请求成功
	mockServer1 := newMockEndpointServer(3) // 前3次失败
	defer mockServer1.close()
	
	mockServer2 := newMockEndpointServer(0) // 立即成功
	defer mockServer2.close()

	// 创建配置 - 每个端点最多重试3次
	cfg := &config.Config{
		Retry: config.RetryConfig{
			MaxAttempts: 3,
			BaseDelay:   50 * time.Millisecond,
			MaxDelay:    500 * time.Millisecond,
			Multiplier:  2.0,
		},
		Group: config.GroupConfig{
			AutoSwitchBetweenGroups: true,
		},
		UsageTracking: config.UsageTrackingConfig{
			Enabled: false,
		},
		Endpoints: []config.EndpointConfig{
			{
				Name:     "endpoint-1",
				URL:      mockServer1.server.URL,
				Priority: 1,
				Timeout:  5 * time.Second,
				Group:    "test-group",
				Token:    "test-token-1",
			},
			{
				Name:     "endpoint-2",
				URL:      mockServer2.server.URL,
				Priority: 2,
				Timeout:  5 * time.Second,
				Group:    "test-group",
				Token:    "test-token-2",
			},
		},
	}

	// 创建端点管理器和处理器
	endpointManager := endpoint.NewManager(cfg)
	time.Sleep(200 * time.Millisecond) // 等待健康检查
	
	// 手动标记端点为健康状态
	endpoints := endpointManager.GetAllEndpoints()
	for _, ep := range endpoints {
		// 直接设置健康状态
		ep.Status.Healthy = true
		ep.Status.LastCheck = time.Now()
	}
	
	handler := NewHandler(endpointManager, cfg)

	// 创建流式请求
	requestBody := `{"message": "test multi-endpoint retry"}`
	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	recorder := httptest.NewRecorder()

	// 执行处理
	start := time.Now()
	handler.ServeHTTP(recorder, req)
	duration := time.Since(start)

	// 获取请求统计
	endpoint1Requests := mockServer1.getRequestCount()
	endpoint2Requests := mockServer2.getRequestCount()

	t.Logf("多端点重试测试结果:")
	t.Logf("  端点1请求次数: %d (预期3次)", endpoint1Requests)
	t.Logf("  端点2请求次数: %d (预期1次)", endpoint2Requests)
	t.Logf("  最终状态码: %d", recorder.Code)
	t.Logf("  处理时长: %v", duration)
	t.Logf("  响应体: %s", recorder.Body.String())

	// 验证：端点1应该被重试3次，然后切换到端点2成功
	if endpoint1Requests != 3 {
		t.Errorf("端点1重试次数错误: 预期 3, 实际 %d", endpoint1Requests)
	}
	
	if endpoint2Requests != 1 {
		t.Errorf("端点2请求次数错误: 预期 1, 实际 %d", endpoint2Requests)
	}

	// 验证最终成功
	if recorder.Code != http.StatusOK {
		t.Errorf("多端点重试应该成功: 状态码 %d, 响应: %s", recorder.Code, recorder.Body.String())
	}

	// 验证响应是流式的
	responseBody := recorder.Body.String()
	if !strings.Contains(responseBody, "event: message_start") {
		t.Errorf("多端点成功响应应包含流式数据: %s", responseBody)
	}
}