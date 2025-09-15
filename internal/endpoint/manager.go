package endpoint

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"sync"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/events"
	"cc-forwarder/internal/transport"
	"cc-forwarder/internal/utils"
)

// EndpointStatus represents the health status of an endpoint
type EndpointStatus struct {
	Healthy         bool
	LastCheck       time.Time
	ResponseTime    time.Duration
	ConsecutiveFails int
	NeverChecked    bool  // 表示从未被检测过
}

// Endpoint represents an endpoint with its configuration and status
type Endpoint struct {
	Config config.EndpointConfig
	Status EndpointStatus
	mutex  sync.RWMutex
}

// Manager manages endpoints and their health status
type Manager struct {
	endpoints    []*Endpoint
	config       *config.Config
	client       *http.Client
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	fastTester   *FastTester
	groupManager *GroupManager
	// EventBus for decoupled event publishing
	eventBus     events.EventBus
}


// NewManager creates a new endpoint manager
func NewManager(cfg *config.Config) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	
	// Create transport with proxy support
	httpTransport, err := transport.CreateTransport(cfg)
	if err != nil {
		slog.Error(fmt.Sprintf("❌ Failed to create HTTP transport with proxy: %s", err.Error()))
		// Fall back to default transport
		httpTransport = &http.Transport{}
	}
	
	
	manager := &Manager{
		config:       cfg,
		client: &http.Client{
			Timeout:   cfg.Health.Timeout,
			Transport: httpTransport,
		},
		ctx:          ctx,
		cancel:       cancel,
		fastTester:   NewFastTester(cfg),
		groupManager: NewGroupManager(cfg),
	}

	// Initialize endpoints
	for _, endpointCfg := range cfg.Endpoints {
		endpoint := &Endpoint{
			Config: endpointCfg,
			Status: EndpointStatus{
				Healthy:      false, // Start pessimistic, let health checks determine actual status
				LastCheck:    time.Now(),
				NeverChecked: true,  // 标记为未检测
			},
		}
		manager.endpoints = append(manager.endpoints, endpoint)
	}

	// Set manager reference in fast tester for dynamic token resolution
	manager.fastTester.SetManager(manager)

	// Initialize groups from endpoints
	manager.groupManager.UpdateGroups(manager.endpoints)

	return manager
}

// Start starts the health checking routine
func (m *Manager) Start() {
	m.wg.Add(1)
	go m.healthCheckLoop()
}

// Stop stops the health checking routine
func (m *Manager) Stop() {
	m.cancel()
	m.wg.Wait()
}

// UpdateConfig updates the manager configuration and recreates endpoints
func (m *Manager) UpdateConfig(cfg *config.Config) {
	m.config = cfg
	
	// Recreate endpoints with new configuration
	endpoints := make([]*Endpoint, len(cfg.Endpoints))
	for i, epCfg := range cfg.Endpoints {
		endpoints[i] = &Endpoint{
			Config: epCfg,
			Status: EndpointStatus{
				Healthy:      false, // Start pessimistic, let health checks determine actual status
				LastCheck:    time.Now(),
				NeverChecked: true,  // 标记为未检测
			},
		}
	}
	m.endpoints = endpoints
	
	// Update group manager with new config and endpoints
	m.groupManager.UpdateConfig(cfg)
	m.groupManager.UpdateGroups(m.endpoints)
	
	// Update fast tester with new config
	if m.fastTester != nil {
		m.fastTester.UpdateConfig(cfg)
	}
	
	// Recreate transport with new proxy configuration
	if transport, err := transport.CreateTransport(cfg); err == nil {
		m.client = &http.Client{
			Transport: transport,
			Timeout:   cfg.Health.Timeout,
		}
	}
}

// GetHealthyEndpoints returns a list of healthy endpoints from active groups based on strategy
func (m *Manager) GetHealthyEndpoints() []*Endpoint {
	// First filter by active groups
	activeEndpoints := m.groupManager.FilterEndpointsByActiveGroups(m.endpoints)
	
	// Then filter by health status
	var healthy []*Endpoint
	for _, endpoint := range activeEndpoints {
		endpoint.mutex.RLock()
		if endpoint.Status.Healthy {
			healthy = append(healthy, endpoint)
		}
		endpoint.mutex.RUnlock()
	}

	return m.sortHealthyEndpoints(healthy, true) // Show logs by default
}

// sortHealthyEndpoints sorts healthy endpoints based on strategy with optional logging
func (m *Manager) sortHealthyEndpoints(healthy []*Endpoint, showLogs bool) []*Endpoint {
	// Sort based on strategy
	switch m.config.Strategy.Type {
	case "priority":
		sort.Slice(healthy, func(i, j int) bool {
			return healthy[i].Config.Priority < healthy[j].Config.Priority
		})
	case "fastest":
		// Log endpoint latencies for fastest strategy (only if showLogs is true)
		if len(healthy) > 1 && showLogs {
			slog.Info("📊 [Fastest Strategy] 基于健康检查的端点延迟排序:")
			for _, ep := range healthy {
				ep.mutex.RLock()
				responseTime := ep.Status.ResponseTime
				ep.mutex.RUnlock()
				slog.Info(fmt.Sprintf("  ⏱️ %s - 延迟: %dms (来源: 定期健康检查)", 
					ep.Config.Name, responseTime.Milliseconds()))
			}
		}
		
		sort.Slice(healthy, func(i, j int) bool {
			healthy[i].mutex.RLock()
			healthy[j].mutex.RLock()
			defer healthy[i].mutex.RUnlock()
			defer healthy[j].mutex.RUnlock()
			return healthy[i].Status.ResponseTime < healthy[j].Status.ResponseTime
		})
	}

	return healthy
}

// GetFastestEndpointsWithRealTimeTest returns endpoints from active groups sorted by real-time testing
func (m *Manager) GetFastestEndpointsWithRealTimeTest(ctx context.Context) []*Endpoint {
	// First get endpoints from active groups and filter by health
	activeEndpoints := m.groupManager.FilterEndpointsByActiveGroups(m.endpoints)
	
	var healthy []*Endpoint
	for _, endpoint := range activeEndpoints {
		endpoint.mutex.RLock()
		if endpoint.Status.Healthy {
			healthy = append(healthy, endpoint)
		}
		endpoint.mutex.RUnlock()
	}
	
	if len(healthy) == 0 {
		return healthy
	}

	// If not using fastest strategy or fast test disabled, apply sorting with logging
	if m.config.Strategy.Type != "fastest" || !m.config.Strategy.FastTestEnabled {
		return m.sortHealthyEndpoints(healthy, true) // Show logs
	}

	// Check if we have cached fast test results first
	testResults, usedCache := m.fastTester.TestEndpointsParallel(ctx, healthy)
	
	// Only show health check sorting if we're NOT using cache
	if !usedCache && m.config.Strategy.Type == "fastest" && len(healthy) > 1 {
		slog.InfoContext(ctx, "📊 [Fastest Strategy] 基于健康检查的活跃组端点延迟排序:")
		for _, ep := range healthy {
			ep.mutex.RLock()
			responseTime := ep.Status.ResponseTime
			group := ep.Config.Group
			ep.mutex.RUnlock()
			slog.InfoContext(ctx, fmt.Sprintf("  ⏱️ %s (组: %s) - 延迟: %dms (来源: 定期健康检查)", 
				ep.Config.Name, group, responseTime.Milliseconds()))
		}
	}
	
	// Log ALL test results first (including failures) - but only if cache wasn't used
	if len(testResults) > 0 && !usedCache {
		slog.InfoContext(ctx, "🔍 [Fastest Response Mode] 活跃组端点性能测试结果:")
		successCount := 0
		for _, result := range testResults {
			group := result.Endpoint.Config.Group
			if result.Success {
				successCount++
				slog.InfoContext(ctx, fmt.Sprintf("  ✅ 健康 %s (组: %s) - 响应时间: %dms", 
					result.Endpoint.Config.Name, group,
					result.ResponseTime.Milliseconds()))
			} else {
				errorMsg := ""
				if result.Error != nil {
					errorMsg = fmt.Sprintf(" - 错误: %s", result.Error.Error())
				}
				slog.InfoContext(ctx, fmt.Sprintf("  ❌ 异常 %s (组: %s) - 响应时间: %dms%s", 
					result.Endpoint.Config.Name, group,
					result.ResponseTime.Milliseconds(),
					errorMsg))
			}
		}
		
		slog.InfoContext(ctx, fmt.Sprintf("📊 [测试摘要] 活跃组测试: %d个端点, 健康: %d个, 异常: %d个",
			len(testResults), successCount, len(testResults)-successCount))
	}
	
	// Sort by response time (only successful results)
	sortedResults := SortByResponseTime(testResults)
	
	if len(sortedResults) == 0 {
		slog.WarnContext(ctx, "⚠️ [Fastest Response Mode] 活跃组所有端点测试失败，回退到健康检查模式")
		return healthy // Fall back to health check results if no fast tests succeeded
	}
	
	// Convert back to endpoint slice
	endpoints := make([]*Endpoint, 0, len(sortedResults))
	for _, result := range sortedResults {
		endpoints = append(endpoints, result.Endpoint)
	}

	// Log the successful endpoint ranking
	if len(endpoints) > 0 {
		// Show the fastest endpoint selection
		fastestEndpoint := endpoints[0]
		var fastestTime int64
		var fastestGroup string
		for _, result := range sortedResults {
			if result.Endpoint == fastestEndpoint {
				fastestTime = result.ResponseTime.Milliseconds()
				fastestGroup = result.Endpoint.Config.Group
				break
			}
		}
		
		cacheIndicator := ""
		if usedCache {
			cacheIndicator = " (缓存)"
		}
		
		slog.InfoContext(ctx, fmt.Sprintf("🚀 [Fastest Response Mode] 选择最快端点: %s (组: %s, %dms)%s", 
			fastestEndpoint.Config.Name, fastestGroup, fastestTime, cacheIndicator))
		
		// Show other available endpoints if there are more than one
		if len(endpoints) > 1 && !usedCache {
			slog.InfoContext(ctx, "📋 [备用端点] 其他可用端点:")
			for i := 1; i < len(endpoints); i++ {
				ep := endpoints[i]
				var responseTime int64
				var epGroup string
				for _, result := range sortedResults {
					if result.Endpoint == ep {
						responseTime = result.ResponseTime.Milliseconds()
						epGroup = result.Endpoint.Config.Group
						break
					}
				}
				slog.InfoContext(ctx, fmt.Sprintf("  🔄 备用 %s (组: %s) - 响应时间: %dms", 
					ep.Config.Name, epGroup, responseTime))
			}
		}
	}

	return endpoints
}

// GetEndpointByName returns an endpoint by name, only from active groups
func (m *Manager) GetEndpointByName(name string) *Endpoint {
	// First filter by active groups
	activeEndpoints := m.groupManager.FilterEndpointsByActiveGroups(m.endpoints)
	
	// Then find by name
	for _, endpoint := range activeEndpoints {
		if endpoint.Config.Name == name {
			return endpoint
		}
	}
	return nil
}

// GetEndpointByNameAny returns an endpoint by name from all endpoints (ignoring group status)
func (m *Manager) GetEndpointByNameAny(name string) *Endpoint {
	for _, endpoint := range m.endpoints {
		if endpoint.Config.Name == name {
			return endpoint
		}
	}
	return nil
}

// GetAllEndpoints returns all endpoints
func (m *Manager) GetAllEndpoints() []*Endpoint {
	return m.endpoints
}

// GetTokenForEndpoint dynamically resolves the token for an endpoint
// If the endpoint has its own token, return it
// If not, find the first endpoint in the same group that has a token
func (m *Manager) GetTokenForEndpoint(ep *Endpoint) string {
	// 1. If endpoint has its own token, use it directly
	if ep.Config.Token != "" {
		return ep.Config.Token
	}
	
	// 2. Find the first endpoint in the same group that has a token
	groupName := ep.Config.Group
	if groupName == "" {
		groupName = "Default"
	}
	
	// Search through all endpoints for the same group
	for _, endpoint := range m.endpoints {
		endpointGroup := endpoint.Config.Group
		if endpointGroup == "" {
			endpointGroup = "Default"
		}
		
		// If same group and has token, return it
		if endpointGroup == groupName && endpoint.Config.Token != "" {
			return endpoint.Config.Token
		}
	}
	
	// 3. No token found in the group
	return ""
}

// GetApiKeyForEndpoint dynamically resolves the API key for an endpoint
// If the endpoint has its own api-key, return it
// If not, find the first endpoint in the same group that has an api-key
func (m *Manager) GetApiKeyForEndpoint(ep *Endpoint) string {
	// 1. If endpoint has its own api-key, use it directly
	if ep.Config.ApiKey != "" {
		return ep.Config.ApiKey
	}
	
	// 2. Find the first endpoint in the same group that has an api-key
	groupName := ep.Config.Group
	if groupName == "" {
		groupName = "Default"
	}
	
	// Search through all endpoints for the same group
	for _, endpoint := range m.endpoints {
		endpointGroup := endpoint.Config.Group
		if endpointGroup == "" {
			endpointGroup = "Default"
		}
		
		// If same group and has api-key, return it
		if endpointGroup == groupName && endpoint.Config.ApiKey != "" {
			return endpoint.Config.ApiKey
		}
	}
	
	// 3. No api-key found in the group
	return ""
}

// GetConfig returns the manager's configuration
func (m *Manager) GetConfig() *config.Config {
	return m.config
}

// GetGroupManager returns the group manager
func (m *Manager) GetGroupManager() *GroupManager {
	return m.groupManager
}


// SetEventBus 设置EventBus事件总线
func (m *Manager) SetEventBus(eventBus events.EventBus) {
	m.eventBus = eventBus
}

// notifyWebInterface 通过EventBus发布端点状态变化事件
func (m *Manager) notifyWebInterface(endpoint *Endpoint) {
	if m.eventBus == nil {
		return
	}
	
	endpoint.mutex.RLock()
	status := endpoint.Status
	endpoint.mutex.RUnlock()
	
	// 确定事件类型和优先级
	eventType := events.EventEndpointHealthy
	priority := events.PriorityHigh
	changeType := "status_changed"
	
	if !status.Healthy {
		eventType = events.EventEndpointUnhealthy
		priority = events.PriorityCritical
		changeType = "health_changed"
	}
	
	m.eventBus.Publish(events.Event{
		Type:     eventType,
		Source:   "endpoint_manager",
		Priority: priority,
		Data: map[string]interface{}{
			"endpoint":        endpoint.Config.Name,
			"healthy":         status.Healthy,
			"response_time":   utils.FormatResponseTime(status.ResponseTime),
			"last_check":      status.LastCheck.Format("2006-01-02 15:04:05"),
			"consecutive_fails": status.ConsecutiveFails,
			"change_type":     changeType,
		},
	})
}

// ManualActivateGroup manually activates a specific group via web interface
func (m *Manager) ManualActivateGroup(groupName string) error {
	err := m.groupManager.ManualActivateGroup(groupName)
	if err != nil {
		return err
	}
	
	// Notify web interface about group change
	go m.notifyWebGroupChange("group_manually_activated", groupName)
	
	return nil
}

// ManualPauseGroup manually pauses a group via web interface
func (m *Manager) ManualPauseGroup(groupName string, duration time.Duration) error {
	err := m.groupManager.ManualPauseGroup(groupName, duration)
	if err != nil {
		return err
	}
	
	// Notify web interface about group change
	go m.notifyWebGroupChange("group_manually_paused", groupName)
	
	return nil
}

// ManualResumeGroup manually resumes a paused group via web interface
func (m *Manager) ManualResumeGroup(groupName string) error {
	err := m.groupManager.ManualResumeGroup(groupName)
	if err != nil {
		return err
	}
	
	// Notify web interface about group change
	go m.notifyWebGroupChange("group_manually_resumed", groupName)
	
	return nil
}

// GetGroupDetails returns detailed information about all groups for web interface
func (m *Manager) GetGroupDetails() map[string]interface{} {
	return m.groupManager.GetGroupDetails()
}

// notifyWebGroupChange notifies the web interface about group management changes
func (m *Manager) notifyWebGroupChange(eventType, groupName string) {
	// 检查EventBus是否可用
	if m.eventBus == nil {
		slog.Debug("[组管理] EventBus未设置，跳过组状态变化通知")
		return
	}

	// 获取组详细信息
	groupDetails := m.GetGroupDetails()

	// 构建事件数据
	data := map[string]interface{}{
		"event":     eventType,
		"group":     groupName,
		"timestamp": time.Now().Format("2006-01-02 15:04:05"),
		"details":   groupDetails,
	}

	// 使用EventBus发布组状态变化事件
	m.eventBus.Publish(events.Event{
		Type:      events.EventGroupStatusChanged,
		Source:    "endpoint_manager",
		Timestamp: time.Now(),
		Priority:  events.PriorityHigh,
		Data:      data,
	})

	slog.Debug(fmt.Sprintf("📢 [组管理] 发布组状态变化事件: %s (组: %s)", eventType, groupName))
}

// notifyGroupHealthStats 通知组健康统计变化
func (m *Manager) notifyGroupHealthStats(groupName string) {
	// 检查EventBus是否可用
	if m.eventBus == nil {
		slog.Debug("[组健康统计] EventBus未设置，跳过组健康统计通知")
		return
	}

	// 处理空组名，默认为"Default"
	if groupName == "" {
		groupName = "Default"
	}

	// 获取组详细信息
	groupDetails := m.groupManager.GetGroupDetails()
	if groups, ok := groupDetails["groups"].([]map[string]interface{}); ok {
		// 查找目标组的健康统计
		for _, group := range groups {
			if groupNameStr, exists := group["name"]; exists && groupNameStr == groupName {
				// 发布组健康统计变化事件
				m.eventBus.Publish(events.Event{
					Type:     events.EventGroupHealthStatsChanged,
					Source:   "endpoint_manager",
					Priority: events.PriorityHigh,
					Data: map[string]interface{}{
						"group":               groupName,
						"healthy_endpoints":   group["healthy_endpoints"],
						"unhealthy_endpoints": group["unhealthy_endpoints"],
						"total_endpoints":     group["total_endpoints"],
						"is_active":           group["is_active"],
						"status":              group["status"],
						"change_type":         "health_stats_changed",
						"timestamp":           time.Now().Format("2006-01-02 15:04:05"),
					},
				})

				slog.Debug(fmt.Sprintf("📊 [组健康统计] 成功发布组健康统计变化事件: %s (健康: %v/%v)",
					groupName, group["healthy_endpoints"], group["total_endpoints"]))
				return
			}
		}
	}

	slog.Warn(fmt.Sprintf("📊 [组健康统计] 未找到组 %s 的健康统计信息，可用组: %v", groupName, groupDetails))
}

// healthCheckLoop runs the health check routine
func (m *Manager) healthCheckLoop() {
	defer m.wg.Done()
	
	ticker := time.NewTicker(m.config.Health.CheckInterval)
	defer ticker.Stop()

	// Initial health check
	m.performHealthChecks()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.performHealthChecks()
		}
	}
}

// performHealthChecks performs health checks on all endpoints
func (m *Manager) performHealthChecks() {
	// In auto mode: only check active group endpoints
	// In manual mode: check all endpoints so we can know their health for manual activation
	var endpointsToCheck []*Endpoint
	
	if m.config.Group.AutoSwitchBetweenGroups {
		// Auto mode: only check active group endpoints
		endpointsToCheck = m.groupManager.FilterEndpointsByActiveGroups(m.endpoints)
		
		if len(endpointsToCheck) == 0 {
			slog.Debug("🩺 [健康检查] 自动模式下没有活跃组中的端点，跳过健康检查")
			return
		}
		
		slog.Debug(fmt.Sprintf("🩺 [健康检查] 自动模式：开始检查 %d 个活跃组端点 (总共 %d 个端点)", 
			len(endpointsToCheck), len(m.endpoints)))
	} else {
		// Manual mode: check all endpoints to determine their health status
		endpointsToCheck = m.endpoints
		
		if len(endpointsToCheck) == 0 {
			slog.Debug("🩺 [健康检查] 没有配置的端点，跳过健康检查")
			return
		}
		
		slog.Debug(fmt.Sprintf("🩺 [健康检查] 手动模式：检查所有 %d 个端点的健康状态", 
			len(endpointsToCheck)))
	}
	
	var wg sync.WaitGroup
	
	// Check the determined endpoints based on mode
	for _, endpoint := range endpointsToCheck {
		wg.Add(1)
		go func(ep *Endpoint) {
			defer wg.Done()
			m.checkEndpointHealth(ep)
		}(endpoint)
	}
	
	wg.Wait()
	
	// Count healthy endpoints after checks
	healthyCount := 0
	for _, ep := range endpointsToCheck {
		if ep.IsHealthy() {
			healthyCount++
		}
	}
	
	if m.config.Group.AutoSwitchBetweenGroups {
		slog.Debug(fmt.Sprintf("🩺 [健康检查] 完成检查 - 活跃组健康: %d/%d", healthyCount, len(endpointsToCheck)))
	} else {
		slog.Debug(fmt.Sprintf("🩺 [健康检查] 完成检查 - 总体健康: %d/%d", healthyCount, len(endpointsToCheck)))
	}
}

// checkEndpointHealth checks the health of a single endpoint
func (m *Manager) checkEndpointHealth(endpoint *Endpoint) {
	start := time.Now()
	
	healthURL := endpoint.Config.URL + m.config.Health.HealthPath
	req, err := http.NewRequestWithContext(m.ctx, "GET", healthURL, nil)
	if err != nil {
		m.updateEndpointStatus(endpoint, false, 0)
		return
	}

	// Add authorization header with dynamically resolved token
	token := m.GetTokenForEndpoint(endpoint)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := m.client.Do(req)
	responseTime := time.Since(start)
	
	if err != nil {
		// Network or connection error
		slog.Warn(fmt.Sprintf("❌ [健康检查] 端点网络错误: %s - 错误: %s, 响应时间: %dms", 
			endpoint.Config.Name, err.Error(), responseTime.Milliseconds()))
		m.updateEndpointStatus(endpoint, false, responseTime)
		return
	}
	
	resp.Body.Close()
	
	// Only consider 2xx as healthy for API endpoints
	// 2xx: Success responses only
	// All other status codes (including 4xx, 5xx) are considered unhealthy
	healthy := (resp.StatusCode >= 200 && resp.StatusCode < 300)
	
	// Log health check results
	if healthy {
		slog.Debug(fmt.Sprintf("✅ [健康检查] 端点正常: %s - 状态码: %d, 响应时间: %dms",
			endpoint.Config.Name,
			resp.StatusCode,
			responseTime.Milliseconds()))
	} else {
		slog.Warn(fmt.Sprintf("⚠️ [健康检查] 端点异常: %s - 状态码: %d, 响应时间: %dms",
			endpoint.Config.Name,
			resp.StatusCode,
			responseTime.Milliseconds()))
	}
	
	m.updateEndpointStatus(endpoint, healthy, responseTime)
}

// updateEndpointStatus updates the health status of an endpoint
func (m *Manager) updateEndpointStatus(endpoint *Endpoint, healthy bool, responseTime time.Duration) {
	endpoint.mutex.Lock()
	defer endpoint.mutex.Unlock()

	endpoint.Status.LastCheck = time.Now()
	endpoint.Status.ResponseTime = responseTime
	endpoint.Status.NeverChecked = false // 标记为已检测

	if healthy {
		// Endpoint is healthy
		wasUnhealthy := !endpoint.Status.Healthy
		endpoint.Status.Healthy = true
		endpoint.Status.ConsecutiveFails = 0

		// Log recovery if endpoint was previously unhealthy
		if wasUnhealthy {
			slog.Info(fmt.Sprintf("✅ [健康检查] 端点恢复正常: %s - 响应时间: %dms",
				endpoint.Config.Name, responseTime.Milliseconds()))
		}
	} else {
		// Endpoint failed health check
		endpoint.Status.ConsecutiveFails++
		wasHealthy := endpoint.Status.Healthy

		// Mark as unhealthy immediately on any failure
		endpoint.Status.Healthy = false

		// Log the failure
		if wasHealthy {
			slog.Warn(fmt.Sprintf("❌ [健康检查] 端点标记为不可用: %s - 连续失败: %d次, 响应时间: %dms",
				endpoint.Config.Name, endpoint.Status.ConsecutiveFails, responseTime.Milliseconds()))
		} else {
			slog.Debug(fmt.Sprintf("❌ [健康检查] 端点仍然不可用: %s - 连续失败: %d次, 响应时间: %dms",
				endpoint.Config.Name, endpoint.Status.ConsecutiveFails, responseTime.Milliseconds()))
		}
	}

	// 通知Web界面端点状态变化
	go m.notifyWebInterface(endpoint)

	// 通知组健康统计变化
	go m.notifyGroupHealthStats(endpoint.Config.Group)
}

// IsHealthy returns the health status of an endpoint
func (e *Endpoint) IsHealthy() bool {
	e.mutex.RLock()
	defer e.mutex.RUnlock()
	return e.Status.Healthy
}

// GetResponseTime returns the last response time of an endpoint
func (e *Endpoint) GetResponseTime() time.Duration {
	e.mutex.RLock()
	defer e.mutex.RUnlock()
	return e.Status.ResponseTime
}

// GetStatus returns a copy of the endpoint status
func (e *Endpoint) GetStatus() EndpointStatus {
	e.mutex.RLock()
	defer e.mutex.RUnlock()
	return e.Status
}

// GetEndpoints returns all endpoints for Web interface
func (m *Manager) GetEndpoints() []*Endpoint {
	return m.endpoints
}

// GetEndpointStatus returns the status of an endpoint by name
func (m *Manager) GetEndpointStatus(name string) EndpointStatus {
	for _, ep := range m.endpoints {
		if ep.Config.Name == name {
			ep.mutex.RLock()
			status := ep.Status
			ep.mutex.RUnlock()
			return status
		}
	}
	return EndpointStatus{}
}

// UpdateEndpointPriority updates the priority of an endpoint by name
func (m *Manager) UpdateEndpointPriority(name string, newPriority int) error {
	if newPriority < 1 {
		return fmt.Errorf("优先级必须大于等于1")
	}

	// Find the endpoint
	var targetEndpoint *Endpoint
	for _, ep := range m.endpoints {
		if ep.Config.Name == name {
			targetEndpoint = ep
			break
		}
	}

	if targetEndpoint == nil {
		return fmt.Errorf("端点 '%s' 未找到", name)
	}

	// Update the priority
	targetEndpoint.Config.Priority = newPriority

	// Update the config as well
	for i, epConfig := range m.config.Endpoints {
		if epConfig.Name == name {
			m.config.Endpoints[i].Priority = newPriority
			break
		}
	}

	slog.Info(fmt.Sprintf("🔄 端点优先级已更新: %s -> %d", name, newPriority))
	
	return nil
}

// ManualHealthCheck performs a manual health check on a specific endpoint by name
func (m *Manager) ManualHealthCheck(endpointName string) error {
	var targetEndpoint *Endpoint
	
	// Find the endpoint by name
	for _, endpoint := range m.endpoints {
		if endpoint.Config.Name == endpointName {
			targetEndpoint = endpoint
			break
		}
	}
	
	if targetEndpoint == nil {
		return fmt.Errorf("端点 '%s' 未找到", endpointName)
	}
	
	// Perform health check on the endpoint
	slog.Info(fmt.Sprintf("🔍 [手动检查] 开始检查端点: %s", endpointName))
	m.checkEndpointHealth(targetEndpoint)
	
	// Get status and log completion with response time
	status := targetEndpoint.Status
	healthStatus := "健康"
	if !status.Healthy {
		if status.NeverChecked {
			healthStatus = "未检测"
		} else {
			healthStatus = "不健康"
		}
	}
	
	slog.Info(fmt.Sprintf("🔍 [手动检查] 检查完成: %s - 状态: %s, 响应时间: %s", 
		endpointName, healthStatus, utils.FormatResponseTime(status.ResponseTime)))
	
	return nil
}