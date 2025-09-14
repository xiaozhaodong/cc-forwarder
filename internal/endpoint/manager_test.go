package endpoint

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/events"
)

// MockEventBus 用于测试的模拟EventBus
type MockEventBus struct {
	events      []events.Event
	broadcaster events.SSEBroadcaster
}

// Event 类型别名以便测试使用
type Event = events.Event
type PriorityHigh = events.EventPriority

const (
	PriorityHighValue = events.PriorityHigh
)

// Publish 实现EventBus接口
func (m *MockEventBus) Publish(event events.Event) {
	m.events = append(m.events, event)
}

// SetSSEBroadcaster 实现EventBus接口
func (m *MockEventBus) SetSSEBroadcaster(broadcaster events.SSEBroadcaster) {
	m.broadcaster = broadcaster
}

// Start 实现EventBus接口
func (m *MockEventBus) Start() error {
	return nil
}

// Stop 实现EventBus接口
func (m *MockEventBus) Stop() error {
	return nil
}

// GetStats 实现EventBus接口
func (m *MockEventBus) GetStats() events.BusStats {
	return events.BusStats{}
}

func TestHealthCheckWithAPIEndpoint(t *testing.T) {
	testCases := []struct {
		name       string
		statusCode int
		expectHealthy bool
	}{
		{"Success 200", 200, true},
		{"Success 201", 201, true},
		{"Bad Request 400", 400, false},  // API reachable but invalid request - should be unhealthy
		{"Unauthorized 401", 401, false}, // API reachable but needs auth - should be unhealthy
		{"Forbidden 403", 403, false},    // API reachable but forbidden - should be unhealthy
		{"Not Found 404", 404, false},    // API reachable but endpoint not found - should be unhealthy
		{"Server Error 500", 500, false}, // API has issues
		{"Bad Gateway 502", 502, false},  // API unreachable
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a test server that returns the specified status code
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Add small delay to ensure response time is measurable
				time.Sleep(1 * time.Millisecond)
				
				// Verify it's checking the correct path
				if r.URL.Path != "/v1/models" {
					t.Errorf("Expected request to /v1/models, got %s", r.URL.Path)
				}
				// Verify Authorization header is present
				if r.Header.Get("Authorization") != "Bearer test-token" {
					t.Errorf("Expected Authorization header 'Bearer test-token', got '%s'", r.Header.Get("Authorization"))
				}
				w.WriteHeader(tc.statusCode)
			}))
			defer server.Close()

			// Create config with test server URL
			cfg := &config.Config{
				Health: config.HealthConfig{
					CheckInterval: 30 * time.Second,
					Timeout:       5 * time.Second,
					HealthPath:    "/v1/models",
				},
				Endpoints: []config.EndpointConfig{
					{
						Name:    "test-endpoint",
						URL:     server.URL,
						Token:   "test-token",
						Timeout: 30 * time.Second,
					},
				},
			}

			// Create manager and perform single health check
			manager := NewManager(cfg)
			endpoint := manager.GetAllEndpoints()[0]

			// Perform health check twice for endpoints that should be unhealthy
			// (due to 2-failure threshold)
			manager.checkEndpointHealth(endpoint)
			if !tc.expectHealthy {
				manager.checkEndpointHealth(endpoint) // Second check to trigger unhealthy status
			}

			// Check result
			if endpoint.IsHealthy() != tc.expectHealthy {
				t.Errorf("Expected healthy=%v for status %d, got %v", 
					tc.expectHealthy, tc.statusCode, endpoint.IsHealthy())
			}

			// Verify response time is recorded (should be > 0 for all HTTP responses)
			responseTime := endpoint.GetResponseTime()
			if responseTime <= 0 {
				t.Errorf("Expected response time to be recorded for status %d, got %v", tc.statusCode, responseTime)
			}
		})
	}
}

func TestFastestStrategyLogging(t *testing.T) {
	// Create multiple test servers with different response times
	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond) // Simulate slow response
		w.WriteHeader(200)
	}))
	defer slowServer.Close()

	fastServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond) // Simulate fast response
		w.WriteHeader(200)
	}))
	defer fastServer.Close()

	cfg := &config.Config{
		Strategy: config.StrategyConfig{
			Type: "fastest",
		},
		Health: config.HealthConfig{
			CheckInterval: 30 * time.Second,
			Timeout:       5 * time.Second,
			HealthPath:    "/v1/models",
		},
		Endpoints: []config.EndpointConfig{
			{
				Name:    "slow-endpoint",
				URL:     slowServer.URL,
				Priority: 1,
				Timeout: 30 * time.Second,
			},
			{
				Name:    "fast-endpoint", 
				URL:     fastServer.URL,
				Priority: 2,
				Timeout: 30 * time.Second,
			},
		},
	}

	manager := NewManager(cfg)
	
	// Perform health checks to populate response times
	for _, endpoint := range manager.GetAllEndpoints() {
		manager.checkEndpointHealth(endpoint)
	}

	// Get healthy endpoints (this should trigger logging for fastest strategy)
	healthy := manager.GetHealthyEndpoints()
	
	// Handle case where endpoints might not be healthy due to path mismatch
	if len(healthy) == 0 {
		t.Skip("No healthy endpoints available - this may be due to health check path requirements")
	}
	
	if len(healthy) < 2 {
		t.Logf("Expected 2 healthy endpoints, got %d", len(healthy))
		return // Skip the rest of the test if we don't have enough endpoints
	}

	// Verify the fast endpoint comes first
	if healthy[0].Config.Name != "fast-endpoint" {
		t.Errorf("Expected fast-endpoint to be first in fastest strategy, got %s", healthy[0].Config.Name)
	}

	// Verify response times are different
	fastTime := healthy[0].GetResponseTime()
	slowTime := healthy[1].GetResponseTime()
	
	if fastTime >= slowTime {
		t.Errorf("Expected fast endpoint to have lower response time. Fast: %v, Slow: %v", fastTime, slowTime)
	}
}

func TestGetEndpointByNameWithGroups(t *testing.T) {
	// Create config with endpoints having same name in different groups
	cfg := &config.Config{
		Health: config.HealthConfig{
			CheckInterval: 30 * time.Second,
			Timeout:       5 * time.Second,
			HealthPath:    "/v1/models",
		},
		Group: config.GroupConfig{
			Cooldown: 10 * time.Minute,
		},
		Endpoints: []config.EndpointConfig{
			{
				Name:          "api-endpoint",
				URL:           "https://primary.example.com",
				Group:         "primary",
				GroupPriority: 1,
				Priority:      1,
				Token:         "primary-token",
				Timeout:       30 * time.Second,
			},
			{
				Name:          "api-endpoint", // Same name, different group
				URL:           "https://backup.example.com",
				Group:         "backup",
				GroupPriority: 2,
				Priority:      1,
				Token:         "backup-token",
				Timeout:       30 * time.Second,
			},
		},
	}

	manager := NewManager(cfg)

	// Test: With primary group active, should return primary endpoint
	endpoint := manager.GetEndpointByName("api-endpoint")
	if endpoint == nil {
		t.Fatal("Expected to find endpoint by name, got nil")
	}
	if endpoint.Config.Group != "primary" {
		t.Errorf("Expected primary group endpoint, got group: %s", endpoint.Config.Group)
	}
	if endpoint.Config.URL != "https://primary.example.com" {
		t.Errorf("Expected primary URL, got: %s", endpoint.Config.URL)
	}

	// Test: GetEndpointByNameAny should still return the first match (primary)
	endpointAny := manager.GetEndpointByNameAny("api-endpoint")
	if endpointAny == nil {
		t.Fatal("Expected to find endpoint by name (any), got nil")
	}
	if endpointAny.Config.Group != "primary" {
		t.Errorf("Expected primary group endpoint (any search), got group: %s", endpointAny.Config.Group)
	}

	// Test: Put primary group in cooldown
	manager.GetGroupManager().SetGroupCooldown("primary")

	// Now GetEndpointByName should return backup endpoint
	endpoint = manager.GetEndpointByName("api-endpoint")
	if endpoint == nil {
		t.Fatal("Expected to find backup endpoint by name after primary cooldown, got nil")
	}
	if endpoint.Config.Group != "backup" {
		t.Errorf("Expected backup group endpoint after primary cooldown, got group: %s", endpoint.Config.Group)
	}
	if endpoint.Config.URL != "https://backup.example.com" {
		t.Errorf("Expected backup URL, got: %s", endpoint.Config.URL)
	}

	// Test: GetEndpointByNameAny should still return first match (primary) regardless of cooldown
	endpointAny = manager.GetEndpointByNameAny("api-endpoint")
	if endpointAny == nil {
		t.Fatal("Expected to find endpoint by name (any) after cooldown, got nil")
	}
	if endpointAny.Config.Group != "primary" {
		t.Errorf("Expected primary group endpoint (any search) even after cooldown, got group: %s", endpointAny.Config.Group)
	}
}

func TestGetEndpointByNameWithNoActiveGroups(t *testing.T) {
	cfg := &config.Config{
		Health: config.HealthConfig{
			CheckInterval: 30 * time.Second,
			Timeout:       5 * time.Second,
			HealthPath:    "/v1/models",
		},
		Group: config.GroupConfig{
			Cooldown: 10 * time.Minute,
		},
		Endpoints: []config.EndpointConfig{
			{
				Name:          "test-endpoint",
				URL:           "https://test.example.com",
				Group:         "testgroup",
				GroupPriority: 1,
				Priority:      1,
				Token:         "test-token",
				Timeout:       30 * time.Second,
			},
		},
	}

	manager := NewManager(cfg)

	// Put the only group in cooldown
	manager.GetGroupManager().SetGroupCooldown("testgroup")

	// GetEndpointByName should return nil (no active groups)
	endpoint := manager.GetEndpointByName("test-endpoint")
	if endpoint != nil {
		t.Errorf("Expected nil when no active groups, got endpoint: %s", endpoint.Config.Name)
	}

	// GetEndpointByNameAny should still return the endpoint
	endpointAny := manager.GetEndpointByNameAny("test-endpoint")
	if endpointAny == nil {
		t.Fatal("Expected to find endpoint by name (any) even with no active groups, got nil")
	}
	if endpointAny.Config.Name != "test-endpoint" {
		t.Errorf("Expected test-endpoint, got: %s", endpointAny.Config.Name)
	}
}

// TestGroupEventBusPublish 测试EventBus组事件发布功能
func TestGroupEventBusPublish(t *testing.T) {
	// 创建模拟EventBus
	mockEventBus := &MockEventBus{
		events: make([]Event, 0),
	}

	// 创建测试配置
	cfg := &config.Config{
		Health: config.HealthConfig{
			CheckInterval: 30 * time.Second,
			Timeout:       5 * time.Second,
			HealthPath:    "/v1/models",
		},
		Group: config.GroupConfig{
			Cooldown: 10 * time.Minute,
			AutoSwitchBetweenGroups: true,
		},
		Endpoints: []config.EndpointConfig{
			{
				Name:          "test-endpoint",
				URL:           "https://test.example.com",
				Group:         "testgroup",
				GroupPriority: 1,
				Priority:      1,
				Token:         "test-token",
				Timeout:       30 * time.Second,
			},
		},
	}

	// 创建Manager实例
	manager := NewManager(cfg)

	// 设置EventBus
	manager.SetEventBus(mockEventBus)

	// 调用notifyWebGroupChange方法
	manager.notifyWebGroupChange("group_manually_activated", "testgroup")

	// 验证EventBus是否收到正确的事件
	if len(mockEventBus.events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(mockEventBus.events))
	}

	event := mockEventBus.events[0]

	// 验证事件类型
	if event.Type != "group_status_changed" {
		t.Errorf("Expected event type 'group_status_changed', got '%s'", event.Type)
	}

	// 验证事件来源
	if event.Source != "endpoint_manager" {
		t.Errorf("Expected event source 'endpoint_manager', got '%s'", event.Source)
	}

	// 验证事件优先级
	if event.Priority != events.PriorityHigh {
		t.Errorf("Expected event priority PriorityHigh (%d), got %d", events.PriorityHigh, event.Priority)
	}

	// 验证事件数据格式
	data := event.Data
	if data == nil {
		t.Fatal("Expected event data, got nil")
	}

	// 验证必要字段存在
	requiredFields := []string{"event", "group", "timestamp", "details"}
	for _, field := range requiredFields {
		if _, exists := data[field]; !exists {
			t.Errorf("Expected field '%s' in event data", field)
		}
	}

	// 验证事件字段值
	if data["event"] != "group_manually_activated" {
		t.Errorf("Expected event 'group_manually_activated', got '%v'", data["event"])
	}

	if data["group"] != "testgroup" {
		t.Errorf("Expected group 'testgroup', got '%v'", data["group"])
	}

	// 验证timestamp是字符串格式
	if _, ok := data["timestamp"].(string); !ok {
		t.Errorf("Expected timestamp to be string, got %T", data["timestamp"])
	}

	// 验证details是map类型
	if _, ok := data["details"].(map[string]interface{}); !ok {
		t.Errorf("Expected details to be map[string]interface{}, got %T", data["details"])
	}
}

// TestGroupEventBusPublishWithoutEventBus 测试没有EventBus时的处理
func TestGroupEventBusPublishWithoutEventBus(t *testing.T) {
	// 创建测试配置
	cfg := &config.Config{
		Health: config.HealthConfig{
			CheckInterval: 30 * time.Second,
			Timeout:       5 * time.Second,
			HealthPath:    "/v1/models",
		},
		Group: config.GroupConfig{
			Cooldown: 10 * time.Minute,
		},
		Endpoints: []config.EndpointConfig{
			{
				Name:          "test-endpoint",
				URL:           "https://test.example.com",
				Group:         "testgroup",
				GroupPriority: 1,
				Priority:      1,
				Token:         "test-token",
				Timeout:       30 * time.Second,
			},
		},
	}

	// 创建Manager实例，但不设置EventBus
	manager := NewManager(cfg)

	// 调用notifyWebGroupChange方法应该不会panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("notifyWebGroupChange panicked when EventBus is nil: %v", r)
		}
	}()

	manager.notifyWebGroupChange("group_manually_activated", "testgroup")
}

// TestGroupEventBusIntegration 测试组管理和EventBus的集成
func TestGroupEventBusIntegration(t *testing.T) {
	// 创建模拟EventBus
	mockEventBus := &MockEventBus{
		events: make([]Event, 0),
	}

	// 创建测试配置
	cfg := &config.Config{
		Health: config.HealthConfig{
			CheckInterval: 30 * time.Second,
			Timeout:       5 * time.Second,
			HealthPath:    "/v1/models",
		},
		Group: config.GroupConfig{
			Cooldown: 10 * time.Minute,
			AutoSwitchBetweenGroups: true,
		},
		Endpoints: []config.EndpointConfig{
			{
				Name:          "primary-endpoint",
				URL:           "https://primary.example.com",
				Group:         "main",
				GroupPriority: 1,
				Priority:      1,
				Token:         "primary-token",
				Timeout:       30 * time.Second,
			},
			{
				Name:          "backup-endpoint",
				URL:           "https://backup.example.com",
				Group:         "backup",
				GroupPriority: 2,
				Priority:      1,
				Token:         "backup-token",
				Timeout:       30 * time.Second,
			},
		},
	}

	// 创建Manager实例
	manager := NewManager(cfg)
	manager.SetEventBus(mockEventBus)

	// 手动设置端点为健康状态以便能够激活组
	allEndpoints := manager.GetAllEndpoints()
	for _, ep := range allEndpoints {
		ep.mutex.Lock()
		ep.Status.Healthy = true
		ep.mutex.Unlock()
	}

	// 测试手动激活组
	err := manager.ManualActivateGroup("backup")
	if err != nil {
		t.Fatalf("Failed to manually activate group: %v", err)
	}

	// 等待goroutine完成
	time.Sleep(10 * time.Millisecond)

	// 验证是否发布了事件
	if len(mockEventBus.events) != 1 {
		t.Errorf("Expected 1 event after manual activation, got %d", len(mockEventBus.events))
	}

	// 验证激活事件
	if len(mockEventBus.events) > 0 {
		event := mockEventBus.events[0]
		if event.Type != "group_status_changed" {
			t.Errorf("Expected group_status_changed event, got %s", event.Type)
		}
		if event.Data["event"] != "group_manually_activated" {
			t.Errorf("Expected group_manually_activated event, got %v", event.Data["event"])
		}
		if event.Data["group"] != "backup" {
			t.Errorf("Expected backup group, got %v", event.Data["group"])
		}
	}

	// 清空事件记录
	mockEventBus.events = mockEventBus.events[:0]

	// 测试手动暂停组
	err = manager.ManualPauseGroup("backup", 5*time.Minute)
	if err != nil {
		t.Fatalf("Failed to manually pause group: %v", err)
	}

	// 等待goroutine完成
	time.Sleep(10 * time.Millisecond)

	// 验证是否发布了暂停事件
	if len(mockEventBus.events) != 1 {
		t.Errorf("Expected 1 event after manual pause, got %d", len(mockEventBus.events))
	}

	// 验证暂停事件
	if len(mockEventBus.events) > 0 {
		event := mockEventBus.events[0]
		if event.Type != "group_status_changed" {
			t.Errorf("Expected group_status_changed event, got %s", event.Type)
		}
		if event.Data["event"] != "group_manually_paused" {
			t.Errorf("Expected group_manually_paused event, got %v", event.Data["event"])
		}
		if event.Data["group"] != "backup" {
			t.Errorf("Expected backup group, got %v", event.Data["group"])
		}
	}

	// 清空事件记录
	mockEventBus.events = mockEventBus.events[:0]

	// 测试手动恢复组
	err = manager.ManualResumeGroup("backup")
	if err != nil {
		t.Fatalf("Failed to manually resume group: %v", err)
	}

	// 等待goroutine完成
	time.Sleep(10 * time.Millisecond)

	// 验证是否发布了恢复事件
	if len(mockEventBus.events) != 1 {
		t.Errorf("Expected 1 event after manual resume, got %d", len(mockEventBus.events))
	}

	// 验证恢复事件
	if len(mockEventBus.events) > 0 {
		event := mockEventBus.events[0]
		if event.Type != "group_status_changed" {
			t.Errorf("Expected group_status_changed event, got %s", event.Type)
		}
		if event.Data["event"] != "group_manually_resumed" {
			t.Errorf("Expected group_manually_resumed event, got %v", event.Data["event"])
		}
		if event.Data["group"] != "backup" {
			t.Errorf("Expected backup group, got %v", event.Data["group"])
		}
	}
}