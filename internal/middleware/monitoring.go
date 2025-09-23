package middleware

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"time"

	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/events"
	"cc-forwarder/internal/monitor"
)

// MonitoringMiddleware provides health and metrics endpoints
type MonitoringMiddleware struct {
	endpointManager *endpoint.Manager
	metrics         *monitor.Metrics
	eventBus        events.EventBus
	lastBroadcast   map[string]time.Time
	startTime       time.Time
}

// NewMonitoringMiddleware creates a new monitoring middleware
func NewMonitoringMiddleware(endpointManager *endpoint.Manager) *MonitoringMiddleware {
	return &MonitoringMiddleware{
		endpointManager: endpointManager,
		metrics:         monitor.NewMetrics(),
		lastBroadcast:   make(map[string]time.Time),
		startTime:       time.Now(),
	}
}

// SetEventBus 设置EventBus事件总线
func (mm *MonitoringMiddleware) SetEventBus(eventBus events.EventBus) {
	mm.eventBus = eventBus
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status    string              `json:"status"`
	Timestamp string              `json:"timestamp"`
	Endpoints []EndpointHealth    `json:"endpoints"`
}

// EndpointHealth represents the health status of an endpoint
type EndpointHealth struct {
	Name             string `json:"name"`
	URL              string `json:"url"`
	Healthy          bool   `json:"healthy"`
	ResponseTimeMs   int64  `json:"response_time_ms"`
	LastCheckTime    string `json:"last_check_time"`
	ConsecutiveFails int    `json:"consecutive_fails"`
	Priority         int    `json:"priority"`
}

// RegisterHealthEndpoint registers health check endpoints
func (mm *MonitoringMiddleware) RegisterHealthEndpoint(mux *http.ServeMux) {
	mux.HandleFunc("/health", mm.handleHealth)
	mux.HandleFunc("/health/detailed", mm.handleDetailedHealth)
	mux.HandleFunc("/metrics", mm.handleMetrics)
}

// handleHealth handles basic health check
func (mm *MonitoringMiddleware) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	endpoints := mm.endpointManager.GetAllEndpoints()
	healthyCount := 0
	
	for _, ep := range endpoints {
		if ep.IsHealthy() {
			healthyCount++
		}
	}

	status := "healthy"
	statusCode := http.StatusOK
	
	if healthyCount == 0 {
		status = "unhealthy"
		statusCode = http.StatusServiceUnavailable
	} else if healthyCount < len(endpoints) {
		status = "degraded"
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	
	response := map[string]interface{}{
		"status": status,
		"healthy_endpoints": healthyCount,
		"total_endpoints": len(endpoints),
	}

	json.NewEncoder(w).Encode(response)
}

// handleDetailedHealth handles detailed health check
func (mm *MonitoringMiddleware) handleDetailedHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	endpoints := mm.endpointManager.GetAllEndpoints()
	healthyCount := 0
	endpointHealths := make([]EndpointHealth, 0, len(endpoints))
	
	for _, ep := range endpoints {
		status := ep.GetStatus()
		if status.Healthy {
			healthyCount++
		}
		
		endpointHealths = append(endpointHealths, EndpointHealth{
			Name:             ep.Config.Name,
			URL:              ep.Config.URL,
			Healthy:          status.Healthy,
			ResponseTimeMs:   status.ResponseTime.Milliseconds(),
			LastCheckTime:    status.LastCheck.Format("2006-01-02T15:04:05Z"),
			ConsecutiveFails: status.ConsecutiveFails,
			Priority:         ep.Config.Priority,
		})
	}

	overallStatus := "healthy"
	statusCode := http.StatusOK
	
	if healthyCount == 0 {
		overallStatus = "unhealthy"
		statusCode = http.StatusServiceUnavailable
	} else if healthyCount < len(endpoints) {
		overallStatus = "degraded"
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	
	response := HealthResponse{
		Status:    overallStatus,
		Timestamp: time.Now().Format("2006-01-02T15:04:05Z"),
		Endpoints: endpointHealths,
	}

	json.NewEncoder(w).Encode(response)
}

// handleMetrics handles metrics endpoint
func (mm *MonitoringMiddleware) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	endpoints := mm.endpointManager.GetAllEndpoints()
	
	w.Header().Set("Content-Type", "text/plain")
	
	// Basic Prometheus-style metrics
	fmt.Fprintf(w, "# HELP endpoint_forwarder_endpoints_total Total number of configured endpoints\n")
	fmt.Fprintf(w, "# TYPE endpoint_forwarder_endpoints_total gauge\n")
	fmt.Fprintf(w, "endpoint_forwarder_endpoints_total %d\n", len(endpoints))
	
	fmt.Fprintf(w, "# HELP endpoint_forwarder_endpoints_healthy Number of healthy endpoints\n")
	fmt.Fprintf(w, "# TYPE endpoint_forwarder_endpoints_healthy gauge\n")
	
	healthyCount := 0
	for _, ep := range endpoints {
		if ep.IsHealthy() {
			healthyCount++
		}
		
		// Individual endpoint metrics
		healthy := 0
		if ep.IsHealthy() {
			healthy = 1
		}
		
		fmt.Fprintf(w, "endpoint_forwarder_endpoint_healthy{name=\"%s\",url=\"%s\",priority=\"%d\"} %d\n",
			ep.Config.Name, ep.Config.URL, ep.Config.Priority, healthy)
		
		fmt.Fprintf(w, "endpoint_forwarder_endpoint_response_time_ms{name=\"%s\",url=\"%s\"} %d\n",
			ep.Config.Name, ep.Config.URL, ep.GetResponseTime().Milliseconds())
		
		status := ep.GetStatus()
		fmt.Fprintf(w, "endpoint_forwarder_endpoint_consecutive_fails{name=\"%s\",url=\"%s\"} %d\n",
			ep.Config.Name, ep.Config.URL, status.ConsecutiveFails)
	}
	
	fmt.Fprintf(w, "endpoint_forwarder_endpoints_healthy %d\n", healthyCount)
}

// GetMetrics returns the metrics instance for TUI access
func (mm *MonitoringMiddleware) GetMetrics() *monitor.Metrics {
	return mm.metrics
}

// RecordRequest records a new request in metrics
func (mm *MonitoringMiddleware) RecordRequest(endpoint, clientIP, userAgent, method, path string) string {
	return mm.metrics.RecordRequest(endpoint, clientIP, userAgent, method, path)
}

// RecordResponse 记录响应数据 - 纯数据收集，不发布事件
// 请求级事件由 lifecycle_manager 负责，系统统计事件由定时广播负责
func (mm *MonitoringMiddleware) RecordResponse(connID string, statusCode int, responseTime time.Duration, bytesSent int64, endpoint string) {
	// 原有的监控数据收集逻辑
	mm.metrics.RecordResponse(connID, statusCode, responseTime, bytesSent, endpoint)

	// 推送真实的连接统计数据（基于实际的统计数据）
	if mm.eventBus != nil && mm.shouldBroadcast("connection_stats", 1*time.Second) {
		mm.broadcastRealConnectionStats(connID, endpoint)
	}
}

// broadcastRealConnectionStats 推送真实的连接统计数据
func (mm *MonitoringMiddleware) broadcastRealConnectionStats(triggerConnID, triggerEndpoint string) {
	if mm.eventBus == nil {
		return
	}

	// 获取真实的连接统计数据
	stats := mm.metrics.GetMetrics()
	suspendedStats := mm.GetSuspendedRequestStats()

	// 计算活跃连接数
	activeConnections := len(stats.ActiveConnections)

	// 计算总Token使用量
	totalTokens := stats.TotalTokenUsage.InputTokens + stats.TotalTokenUsage.OutputTokens +
					stats.TotalTokenUsage.CacheCreationTokens + stats.TotalTokenUsage.CacheReadTokens

	// 构建真实的连接统计数据
	connectionData := map[string]interface{}{
		"total_requests":       stats.TotalRequests,
		"active_connections":   activeConnections,
		"successful_requests":  stats.SuccessfulRequests,
		"failed_requests":      stats.FailedRequests,
		"average_response_time": mm.formatResponseTime(stats.GetAverageResponseTime()),
		"total_tokens":         totalTokens,
		"change_type":          "connection_stats_updated",
		"timestamp":            time.Now().Unix(),
		"trigger_endpoint":     triggerEndpoint,
		"trigger_conn_id":      triggerConnID,
	}

	// 添加暂停请求统计
	if suspendedStats != nil {
		connectionData["total_suspended_requests"] = suspendedStats["total_suspended_requests"]
		connectionData["error_suspended_requests"] = suspendedStats["error_suspended_requests"]
		connectionData["timeout_suspended_requests"] = suspendedStats["timeout_suspended_requests"]
		connectionData["suspended_success_rate"] = suspendedStats["success_rate"]
	}

	// 发布连接统计更新事件
	mm.eventBus.Publish(events.Event{
		Type:     events.EventConnectionStatsUpdated,
		Source:   "monitoring_middleware",
		Priority: events.PriorityNormal,
		Data:     connectionData,
	})

	slog.Debug(fmt.Sprintf("🔗 [连接统计推送] 真实数据: 总请求=%d, 活跃连接=%d, 成功=%d, 失败=%d, Token=%d, 触发端点=%s",
		stats.TotalRequests, activeConnections, stats.SuccessfulRequests, stats.FailedRequests, totalTokens, triggerEndpoint))
}

// formatResponseTime 格式化响应时间为字符串
func (mm *MonitoringMiddleware) formatResponseTime(duration time.Duration) string {
	if duration < time.Second {
		return fmt.Sprintf("%dms", duration.Milliseconds())
	}
	return fmt.Sprintf("%.2fs", duration.Seconds())
}

// RecordRetry records a retry attempt
func (mm *MonitoringMiddleware) RecordRetry(connID string, endpoint string) {
	mm.metrics.RecordRetry(connID, endpoint)
}

// UpdateEndpointHealthStatus 更新端点健康状态 - 专注数据更新
// 端点健康事件由 endpoint_manager 直接发布
func (mm *MonitoringMiddleware) UpdateEndpointHealthStatus() {
	endpoints := mm.endpointManager.GetAllEndpoints()
	for _, ep := range endpoints {
		mm.metrics.UpdateEndpointHealth(
			ep.Config.Name,
			ep.Config.URL,
			ep.IsHealthy(),
			ep.Config.Priority,
		)
	}
	// 不再广播端点事件 - 由 endpoint_manager 负责
}

// UpdateConnectionEndpoint updates the endpoint name for an active connection
func (mm *MonitoringMiddleware) UpdateConnectionEndpoint(connID, endpoint string) {
	mm.metrics.UpdateConnectionEndpoint(connID, endpoint)
}

// RecordTokenUsage records token usage for a specific request
func (mm *MonitoringMiddleware) RecordTokenUsage(connID string, endpoint string, tokens *monitor.TokenUsage) {
	mm.metrics.RecordTokenUsage(connID, endpoint, tokens)
}

// MarkStreamingConnection 标记连接为流式连接 - 纯数据记录
func (mm *MonitoringMiddleware) MarkStreamingConnection(connID string) {
	mm.metrics.MarkStreamingConnection(connID)
	// 不再发布事件 - 流式连接状态由系统统计事件统一处理
}

// RecordRequestSuspended 记录请求挂起 - 纯数据记录
func (mm *MonitoringMiddleware) RecordRequestSuspended(connID string) {
	mm.metrics.RecordRequestSuspended(connID)
	// 不再发布事件 - 请求级事件由 lifecycle_manager 负责
}

// RecordRequestResumed 记录挂起请求恢复 - 纯数据记录
func (mm *MonitoringMiddleware) RecordRequestResumed(connID string) {
	mm.metrics.RecordRequestResumed(connID)
	// 不再发布事件 - 请求级事件由 lifecycle_manager 负责
}

// RecordRequestSuspendTimeout 记录挂起请求超时 - 纯数据记录
func (mm *MonitoringMiddleware) RecordRequestSuspendTimeout(connID string) {
	mm.metrics.RecordRequestSuspendTimeout(connID)
	// 不再发布事件 - 请求级事件由 lifecycle_manager 负责
}

// GetSuspendedRequestStats returns suspended request statistics
func (mm *MonitoringMiddleware) GetSuspendedRequestStats() map[string]interface{} {
	return mm.metrics.GetSuspendedRequestStats()
}

// GetActiveSuspendedConnections returns currently suspended connections
func (mm *MonitoringMiddleware) GetActiveSuspendedConnections() []*monitor.ConnectionInfo {
	return mm.metrics.GetActiveSuspendedConnections()
}

// shouldBroadcast 检查是否应该广播事件（基于频率限制）
func (mm *MonitoringMiddleware) shouldBroadcast(eventType string, interval time.Duration) bool {
	lastTime, exists := mm.lastBroadcast[eventType]
	if !exists || time.Since(lastTime) >= interval {
		mm.lastBroadcast[eventType] = time.Now()
		return true
	}
	return false
}

// broadcastSystemStats 广播系统级统计事件
// 只包含系统级统计：uptime、memory_usage、goroutine_count
func (mm *MonitoringMiddleware) broadcastSystemStats() {
	if mm.eventBus == nil {
		return
	}

	// 获取系统运行时间
	uptime := time.Since(mm.startTime)

	// 获取内存使用情况
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// 获取goroutine数量
	goroutineCount := runtime.NumGoroutine()

	// 只发布系统级统计事件（移除连接统计数据）
	mm.eventBus.Publish(events.Event{
		Type:     events.EventSystemStatsUpdated,
		Source:   "monitoring",
		Priority: events.PriorityLow,
		Data: map[string]interface{}{
			"uptime":           uptime.Seconds(),
			"memory_usage":     memStats.Alloc,
			"goroutine_count":  goroutineCount,
			"change_type":      "system_stats_updated",
			"timestamp":        time.Now().Unix(),
		},
	})

	// 添加诊断日志确认发布的是系统统计事件
	fmt.Printf("🖥️ [系统统计推送] 事件类型: EventSystemStatsUpdated, 映射: status, 数据: uptime=%.0fs, memory=%dMB, goroutines=%d\n",
		uptime.Seconds(), memStats.Alloc/(1024*1024), goroutineCount)
}

// StartPeriodicBroadcast 启动系统统计的定时广播
// 10秒间隔广播系统级统计信息
func (mm *MonitoringMiddleware) StartPeriodicBroadcast() {
	if mm.eventBus == nil {
		return
	}

	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			mm.broadcastSystemStats()
		}
	}()
}

// getStreamingConnections 计算流式连接数量
func (mm *MonitoringMiddleware) getStreamingConnections(activeConnections map[string]*monitor.ConnectionInfo) int {
	count := 0
	for _, conn := range activeConnections {
		if conn.IsStreaming {
			count++
		}
	}
	return count
}

// RecordFailedRequestTokens 记录失败请求的Token使用到监控系统
func (mm *MonitoringMiddleware) RecordFailedRequestTokens(connID, endpoint string, tokens *monitor.TokenUsage, failureReason string) {
	if mm.metrics != nil {
		// 只有当tokens不为nil时才记录普通Token使用
		// 避免nil pointer dereference
		if tokens != nil {
			// 记录普通Token使用（更新总体统计）
			// 即使是失败请求，也需要计入总Token使用量
			mm.metrics.RecordTokenUsage(connID, endpoint, tokens)
		}

		// 总是记录失败请求专用的Token统计（内部处理nil tokens）
		// 这会更新FailedRequestTokens、FailedTokensByReason等专用指标
		mm.metrics.RecordFailedRequestTokenUsage(connID, endpoint, tokens, failureReason)
	}

	// 广播失败请求Token事件
	if mm.eventBus != nil {
		// 安全处理 nil tokens
		var inputTokens, outputTokens, cacheCreationTokens, cacheReadTokens int64
		if tokens != nil {
			inputTokens = tokens.InputTokens
			outputTokens = tokens.OutputTokens
			cacheCreationTokens = tokens.CacheCreationTokens
			cacheReadTokens = tokens.CacheReadTokens
		}

		mm.eventBus.Publish(events.Event{
			Type:      events.EventSystemStatsUpdated, // 使用系统统计更新事件类型
			Source:    "monitoring_middleware",
			Timestamp: time.Now(),
			Priority:  events.PriorityNormal,
			Data: map[string]interface{}{
				"event_type":            "failed_request_tokens",
				"conn_id":               connID,
				"endpoint":              endpoint,
				"input_tokens":          inputTokens,
				"output_tokens":         outputTokens,
				"cache_creation_tokens": cacheCreationTokens,
				"cache_read_tokens":     cacheReadTokens,
				"failure_reason":        failureReason,
			},
		})
	}

	slog.Debug(fmt.Sprintf("📊 [监控失败Token] [%s] 端点: %s, 原因: %s", connID, endpoint, failureReason))
}