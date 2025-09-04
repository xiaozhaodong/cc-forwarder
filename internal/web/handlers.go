package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// formatResponseTime 格式化响应时间为人性化显示
func formatResponseTime(d time.Duration) string {
	if d == 0 {
		return "0ms"
	}
	
	ms := d.Milliseconds()
	if ms >= 1000 {
		seconds := float64(ms) / 1000
		return fmt.Sprintf("%.1fs", seconds)
	} else if ms >= 1 {
		return fmt.Sprintf("%dms", ms)
	} else {
		// 小于1毫秒的情况，显示微秒
		us := d.Microseconds()
		return fmt.Sprintf("%dμs", us)
	}
}

// formatUptime 格式化运行时间为人性化显示
func formatUptime(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if days > 0 {
		return fmt.Sprintf("%d天 %d小时 %d分钟", days, hours, minutes)
	} else if hours > 0 {
		return fmt.Sprintf("%d小时 %d分钟", hours, minutes)
	} else if minutes > 0 {
		return fmt.Sprintf("%d分钟 %d秒", minutes, seconds)
	} else {
		return fmt.Sprintf("%d秒", seconds)
	}
}

// handleIndex处理主页面
func (ws *WebServer) handleIndex(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, indexHTML)
}

// handleStatus处理状态API
func (ws *WebServer) handleStatus(c *gin.Context) {
	uptime := time.Since(ws.startTime)
	
	status := map[string]interface{}{
		"status":      "running",
		"uptime":      formatUptime(uptime),
		"start_time":  ws.startTime.Format("2006-01-02 15:04:05"),
		"config_file": ws.configPath,
		"version": map[string]string{
			"version": "dev", // 这里可以从构建时变量获取
			"commit":  "unknown",
			"date":    "unknown",
		},
		"server": map[string]interface{}{
			"proxy_port": ws.config.Server.Port,
			"web_port":   ws.config.Web.Port,
			"host":       ws.config.Server.Host,
		},
		"strategy": ws.config.Strategy.Type,
		"auth_enabled": ws.config.Auth.Enabled,
		"proxy_enabled": ws.config.Proxy.Enabled,
	}
	
	c.JSON(http.StatusOK, status)
}

// handleEndpoints处理端点API
func (ws *WebServer) handleEndpoints(c *gin.Context) {
	endpoints := ws.endpointManager.GetEndpoints()
	endpointData := make([]map[string]interface{}, 0, len(endpoints))
	
	for _, ep := range endpoints {
		status := ws.endpointManager.GetEndpointStatus(ep.Config.Name)
		
		endpointData = append(endpointData, map[string]interface{}{
			"name":           ep.Config.Name,
			"url":            ep.Config.URL,
			"priority":       ep.Config.Priority,
			"group":          ep.Config.Group,
			"group_priority": ep.Config.GroupPriority,
			"timeout":        ep.Config.Timeout.String(),
			"healthy":        status.Healthy,
			"last_check":     status.LastCheck.Format("2006-01-02 15:04:05"),
			"response_time":  formatResponseTime(status.ResponseTime),
			"never_checked":  status.NeverChecked,
			"error":          "", // 暂时设为空字符串
		})
	}
	
	c.JSON(http.StatusOK, map[string]interface{}{
		"endpoints": endpointData,
		"total":     len(endpointData),
	})
}

// handleConnections处理连接API
func (ws *WebServer) handleConnections(c *gin.Context) {
	metrics := ws.monitoringMiddleware.GetMetrics()
	stats := metrics.GetMetrics()
	
	// 获取挂起请求统计
	suspendedStats := metrics.GetSuspendedRequestStats()
	
	// 获取当前挂起的连接
	suspendedConnections := metrics.GetActiveSuspendedConnections()
	var suspendedConnectionDetails []map[string]interface{}
	for _, conn := range suspendedConnections {
		suspendedTime := time.Duration(0)
		if !conn.SuspendedAt.IsZero() {
			suspendedTime = time.Since(conn.SuspendedAt)
		}
		
		suspendedConnectionDetails = append(suspendedConnectionDetails, map[string]interface{}{
			"id":             conn.ID,
			"client_ip":      conn.ClientIP,
			"method":         conn.Method,
			"path":           conn.Path,
			"endpoint":       conn.Endpoint,
			"suspended_at":   conn.SuspendedAt.Format("2006-01-02 15:04:05"),
			"suspended_time": formatResponseTime(suspendedTime),
			"retry_count":    conn.RetryCount,
			"user_agent":     conn.UserAgent,
		})
	}
	
	connections := map[string]interface{}{
		"total_requests":       stats.TotalRequests,
		"active_connections":   len(stats.ActiveConnections),
		"successful_requests":  stats.SuccessfulRequests,
		"failed_requests":      stats.FailedRequests,
		"average_response_time": formatResponseTime(stats.GetAverageResponseTime()),
		"requests_per_endpoint": make(map[string]int64),
		"errors_per_endpoint":   make(map[string]int64),
		
		// 挂起请求相关统计
		"suspended":            suspendedStats,
		"suspended_connections": suspendedConnectionDetails,
	}

	// 添加每个端点的请求统计
	for _, endpointMetrics := range stats.EndpointStats {
		connections["requests_per_endpoint"].(map[string]int64)[endpointMetrics.Name] = endpointMetrics.TotalRequests
		connections["errors_per_endpoint"].(map[string]int64)[endpointMetrics.Name] = endpointMetrics.FailedRequests
	}
	
	c.JSON(http.StatusOK, connections)
}

// handleConfig处理配置API
func (ws *WebServer) handleConfig(c *gin.Context) {
	configData := map[string]interface{}{
		"server":    ws.config.Server,
		"web":       ws.config.Web,
		"strategy":  ws.config.Strategy,
		"retry":     ws.config.Retry,
		"health":    ws.config.Health,
		"logging":   ws.config.Logging,
		"streaming": ws.config.Streaming,
		"group":     ws.config.Group,
		"proxy":     ws.config.Proxy,
		"auth":      ws.config.Auth,
		"tui":       ws.config.TUI,
		"global_timeout": ws.config.GlobalTimeout.String(),
	}
	
	c.JSON(http.StatusOK, configData)
}

// handleLogs处理日志API
func (ws *WebServer) handleLogs(c *gin.Context) {
	// 这里可以返回最近的日志条目
	// 由于当前日志系统没有内存缓存，我们返回一个占位符响应
	logs := []map[string]interface{}{
		{
			"timestamp": time.Now().Format("2006-01-02 15:04:05"),
			"level":     "INFO",
			"message":   "Web界面日志功能已启用",
			"source":    "web",
		},
	}
	
	c.JSON(http.StatusOK, map[string]interface{}{
		"logs":  logs,
		"total": len(logs),
	})
}

// handleUpdatePriority处理更新端点优先级API
func (ws *WebServer) handleUpdatePriority(c *gin.Context) {
	endpointName := c.Param("name")
	
	var request struct {
		Priority int `json:"priority" binding:"required,min=1"`
	}
	
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "Invalid request: " + err.Error(),
		})
		return
	}
	
	// 更新端点优先级
	if err := ws.endpointManager.UpdateEndpointPriority(endpointName, request.Priority); err != nil {
		c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": err.Error(),
		})
		return
	}
	
	ws.logger.Info("🔄 端点优先级已通过Web界面更新", "endpoint", endpointName, "priority", request.Priority)
	
	c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "优先级更新成功",
	})
}

// handleManualHealthCheck处理手动健康检测API
func (ws *WebServer) handleManualHealthCheck(c *gin.Context) {
	endpointName := c.Param("name")
	
	// 执行手动健康检查
	err := ws.endpointManager.ManualHealthCheck(endpointName)
	if err != nil {
		c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	
	// 获取更新后的端点状态
	status := ws.endpointManager.GetEndpointStatus(endpointName)
	
	ws.logger.Info("🔍 手动健康检测已完成", "endpoint", endpointName, "healthy", status.Healthy)
	
	c.JSON(http.StatusOK, map[string]interface{}{
		"success":       true,
		"message":       "手动健康检测完成",
		"healthy":       status.Healthy,
		"response_time": status.ResponseTime.String(),
		"last_check":    status.LastCheck.Format("2006-01-02 15:04:05"),
		"never_checked": status.NeverChecked,
	})
}

// handleSSE处理Server-Sent Events连接
func (ws *WebServer) handleSSE(c *gin.Context) {
	// 设置SSE标准响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Transfer-Encoding", "identity") // 禁用分块编码

	// 立即刷新以建立连接
	c.Writer.Flush()

	// 获取客户端ID，如果没有则生成一个
	clientID := c.Query("client_id")
	if clientID == "" {
		clientID = uuid.New().String()
	}

	// 解析事件过滤器
	filter := ws.parseEventFilter(c.Query("events"))

	ws.logger.Debug("SSE客户端尝试连接", "client_id", clientID, "filter", filter)

	// 使用context来管理连接生命周期
	ctx := c.Request.Context()
	
	// 发送初始连接确认
	if err := ws.sendSSEEvent(c, "connection", map[string]interface{}{
		"status": "established",
		"message": "SSE连接已建立",
		"timestamp": time.Now().Format("2006-01-02 15:04:05"),
	}); err != nil {
		ws.logger.Debug("发送连接确认失败", "client_id", clientID, "error", err)
		return
	}
	
	// 发送初始状态数据
	ws.sendSSEInitialData(c)

	// 创建简单的数据更新循环
	ticker := time.NewTicker(5 * time.Second) // 5秒更新一次
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// 发送状态更新
			if err := ws.sendSSEStatusUpdate(c); err != nil {
				ws.logger.Debug("发送状态更新失败", "client_id", clientID, "error", err)
				return
			}
			// 发送端点更新
			if err := ws.sendSSEEndpointsUpdate(c); err != nil {
				ws.logger.Debug("发送端点更新失败", "client_id", clientID, "error", err)
				return
			}
			// 发送连接统计更新
			if err := ws.sendSSEConnectionsUpdate(c); err != nil {
				ws.logger.Debug("发送连接统计更新失败", "client_id", clientID, "error", err)
				return
			}

		case <-ctx.Done():
			// 客户端断开连接
			ws.logger.Debug("SSE客户端断开连接", "client_id", clientID)
			return
		}
	}
}

// parseEventFilter解析事件过滤器
func (ws *WebServer) parseEventFilter(eventsParam string) map[EventType]bool {
	filter := make(map[EventType]bool)

	if eventsParam == "" {
		// 默认订阅所有事件类型
		filter[EventTypeStatus] = true
		filter[EventTypeEndpoint] = true
		filter[EventTypeConnection] = true
		filter[EventTypeLog] = true
		filter[EventTypeConfig] = false // 配置事件默认不订阅
		filter[EventTypeChart] = true
		filter[EventTypeGroup] = true
		filter[EventTypeSuspended] = true
		return filter
	}

	// 解析逗号分隔的事件类型
	events := strings.Split(eventsParam, ",")
	for _, event := range events {
		event = strings.TrimSpace(event)
		switch event {
		case "status":
			filter[EventTypeStatus] = true
		case "endpoint":
			filter[EventTypeEndpoint] = true
		case "connection":
			filter[EventTypeConnection] = true
		case "log":
			filter[EventTypeLog] = true
		case "config":
			filter[EventTypeConfig] = true
		case "chart":
			filter[EventTypeChart] = true
		case "group":
			filter[EventTypeGroup] = true
		case "suspended":
			filter[EventTypeSuspended] = true
		}
	}

	return filter
}

// sendSSEEvent发送SSE事件的通用函数
func (ws *WebServer) sendSSEEvent(c *gin.Context, eventType string, data interface{}) error {
	// 检查连接是否已关闭
	select {
	case <-c.Request.Context().Done():
		return c.Request.Context().Err()
	default:
		c.SSEvent(eventType, data)
		return nil
	}
}

// sendSSEInitialData发送初始数据
func (ws *WebServer) sendSSEInitialData(c *gin.Context) error {
	// 发送服务状态
	uptime := time.Since(ws.startTime)
	statusData := map[string]interface{}{
		"status":      "running",
		"uptime":      formatUptime(uptime),
		"start_time":  ws.startTime.Format("2006-01-02 15:04:05"),
		"config_file": ws.configPath,
	}
	if err := ws.sendSSEEvent(c, "status", statusData); err != nil {
		return err
	}

	// 发送端点数据
	if err := ws.sendSSEEndpointsUpdate(c); err != nil {
		return err
	}
	
	// 发送连接统计数据
	return ws.sendSSEConnectionsUpdate(c)
}

// sendSSEStatusUpdate发送状态更新
func (ws *WebServer) sendSSEStatusUpdate(c *gin.Context) error {
	uptime := time.Since(ws.startTime)
	statusData := map[string]interface{}{
		"status":     "running",
		"uptime":     formatUptime(uptime),
		"timestamp":  time.Now().Format("2006-01-02 15:04:05"),
	}
	return ws.sendSSEEvent(c, "status", statusData)
}

// sendSSEEndpointsUpdate发送端点更新
func (ws *WebServer) sendSSEEndpointsUpdate(c *gin.Context) error {
	endpoints := ws.endpointManager.GetEndpoints()
	endpointData := make([]map[string]interface{}, 0, len(endpoints))
	
	for _, ep := range endpoints {
		status := ws.endpointManager.GetEndpointStatus(ep.Config.Name)
		endpointData = append(endpointData, map[string]interface{}{
			"name":           ep.Config.Name,
			"url":            ep.Config.URL,
			"priority":       ep.Config.Priority,
			"group":          ep.Config.Group,
			"group_priority": ep.Config.GroupPriority,
			"healthy":        status.Healthy,
			"response_time":  formatResponseTime(status.ResponseTime),
			"last_check":     status.LastCheck.Format("2006-01-02 15:04:05"),
		})
	}
	
	return ws.sendSSEEvent(c, "endpoints", map[string]interface{}{
		"endpoints": endpointData,
		"total":     len(endpointData),
		"timestamp": time.Now().Format("2006-01-02 15:04:05"),
	})
}

// sendSSEConnectionsUpdate发送连接统计更新
func (ws *WebServer) sendSSEConnectionsUpdate(c *gin.Context) error {
	metrics := ws.monitoringMiddleware.GetMetrics()
	stats := metrics.GetMetrics()
	
	connectionData := map[string]interface{}{
		"total_requests":       stats.TotalRequests,
		"active_connections":   len(stats.ActiveConnections),
		"successful_requests":  stats.SuccessfulRequests,
		"failed_requests":      stats.FailedRequests,
		"average_response_time": formatResponseTime(stats.GetAverageResponseTime()),
		"timestamp":            time.Now().Format("2006-01-02 15:04:05"),
	}
	
	return ws.sendSSEEvent(c, "connections", connectionData)
}

// writeSSEEvent写入SSE事件到响应流
func (ws *WebServer) writeSSEEvent(c *gin.Context, event Event) error {
	// 将事件数据序列化为JSON
	data, err := ws.eventManager.formatEventData(event)
	if err != nil {
		return err
	}

	// 直接写入到响应流
	_, err = c.Writer.WriteString(data)
	if err != nil {
		return err
	}

	// 立即刷新缓冲区
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}

	return nil
}

// writeSSEPing写入SSE心跳到响应流
func (ws *WebServer) writeSSEPing(c *gin.Context) error {
	_, err := c.Writer.WriteString(": ping\n\n")
	if err != nil {
		return err
	}

	// 刷新缓冲区
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}

	return nil
}

// BroadcastStatusUpdate广播状态更新事件
func (ws *WebServer) BroadcastStatusUpdate(data map[string]interface{}) {
	if ws.eventManager != nil {
		ws.eventManager.BroadcastEvent(EventTypeStatus, data)
	}
}

// BroadcastEndpointUpdate广播端点更新事件
func (ws *WebServer) BroadcastEndpointUpdate(data map[string]interface{}) {
	if ws.eventManager != nil {
		ws.eventManager.BroadcastEvent(EventTypeEndpoint, data)
	}
}

// BroadcastConnectionUpdate广播连接更新事件
func (ws *WebServer) BroadcastConnectionUpdate(data map[string]interface{}) {
	if ws.eventManager != nil {
		ws.eventManager.BroadcastEvent(EventTypeConnection, data)
	}
}

// BroadcastLogEvent广播日志事件
func (ws *WebServer) BroadcastLogEvent(data map[string]interface{}) {
	if ws.eventManager != nil {
		ws.eventManager.BroadcastEvent(EventTypeLog, data)
	}
}

// BroadcastConfigUpdate广播配置更新事件
func (ws *WebServer) BroadcastConfigUpdate(data map[string]interface{}) {
	if ws.eventManager != nil {
		ws.eventManager.BroadcastEvent(EventTypeConfig, data)
	}
}

// BroadcastGroupUpdate广播组更新事件
func (ws *WebServer) BroadcastGroupUpdate(data map[string]interface{}) {
	if ws.eventManager != nil {
		ws.eventManager.BroadcastEvent(EventTypeGroup, data)
	}
}

// BroadcastSuspendedUpdate广播挂起请求事件
func (ws *WebServer) BroadcastSuspendedUpdate(data map[string]interface{}) {
	if ws.eventManager != nil {
		ws.eventManager.BroadcastEvent(EventTypeSuspended, data)
	}
}

// handleTokenUsage处理Token使用统计API
func (ws *WebServer) handleTokenUsage(c *gin.Context) {
	metrics := ws.monitoringMiddleware.GetMetrics()
	tokenStats := metrics.GetTotalTokenStats()
	
	// 获取历史Token数据
	minutes := 60 // 默认1小时
	if m := c.Query("minutes"); m != "" {
		if parsed, err := strconv.Atoi(m); err == nil && parsed > 0 {
			minutes = parsed
		}
	}
	
	tokenHistory := metrics.GetChartDataForTokenHistory(minutes)
	
	c.JSON(http.StatusOK, map[string]interface{}{
		"current": map[string]interface{}{
			"input_tokens":          tokenStats.InputTokens,
			"output_tokens":         tokenStats.OutputTokens,
			"cache_creation_tokens": tokenStats.CacheCreationTokens,
			"cache_read_tokens":     tokenStats.CacheReadTokens,
			"total_tokens":          tokenStats.InputTokens + tokenStats.OutputTokens,
		},
		"history": tokenHistory,
		"distribution": map[string]interface{}{
			"input_percentage":  calculateTokenPercentage(tokenStats.InputTokens, tokenStats.InputTokens+tokenStats.OutputTokens),
			"output_percentage": calculateTokenPercentage(tokenStats.OutputTokens, tokenStats.InputTokens+tokenStats.OutputTokens),
		},
	})
}

// handleMetricsHistory处理历史指标数据API
func (ws *WebServer) handleMetricsHistory(c *gin.Context) {
	metrics := ws.monitoringMiddleware.GetMetrics()
	
	// 解析时间范围参数
	minutes := 60 // 默认1小时
	if m := c.Query("minutes"); m != "" {
		if parsed, err := strconv.Atoi(m); err == nil && parsed > 0 {
			minutes = parsed
		}
	}
	
	requestHistory := metrics.GetChartDataForRequestHistory(minutes)
	responseHistory := metrics.GetChartDataForResponseTime(minutes)
	tokenHistory := metrics.GetChartDataForTokenHistory(minutes)
	
	c.JSON(http.StatusOK, map[string]interface{}{
		"requests":       requestHistory,
		"response_times": responseHistory,
		"tokens":         tokenHistory,
		"summary": map[string]interface{}{
			"data_points":     len(requestHistory),
			"time_range":      minutes,
			"last_updated":    time.Now().Format("2006-01-02 15:04:05"),
		},
	})
}

// handleEndpointPerformance处理端点性能统计API
func (ws *WebServer) handleEndpointPerformance(c *gin.Context) {
	metrics := ws.monitoringMiddleware.GetMetrics()
	performanceData := metrics.GetEndpointPerformanceData()
	
	c.JSON(http.StatusOK, map[string]interface{}{
		"endpoints": performanceData,
		"total":     len(performanceData),
		"timestamp": time.Now().Format("2006-01-02 15:04:05"),
	})
}

// handleRequestTrends处理请求趋势图表API
func (ws *WebServer) handleRequestTrends(c *gin.Context) {
	metrics := ws.monitoringMiddleware.GetMetrics()
	
	// 解析时间范围参数
	minutes := 30 // 默认30分钟
	if m := c.Query("minutes"); m != "" {
		if parsed, err := strconv.Atoi(m); err == nil && parsed > 0 {
			minutes = parsed
		}
	}
	
	requestHistory := metrics.GetChartDataForRequestHistory(minutes)
	
	// 转换为Chart.js格式
	labels := make([]string, len(requestHistory))
	totalData := make([]int64, len(requestHistory))
	successData := make([]int64, len(requestHistory))
	failedData := make([]int64, len(requestHistory))
	
	for i, point := range requestHistory {
		labels[i] = point.Timestamp.Format("15:04")
		totalData[i] = point.Total
		successData[i] = point.Successful
		failedData[i] = point.Failed
	}
	
	c.JSON(http.StatusOK, map[string]interface{}{
		"labels": labels,
		"datasets": []map[string]interface{}{
			{
				"label":           "总请求数",
				"data":            totalData,
				"borderColor":     "#3b82f6",
				"backgroundColor": "rgba(59, 130, 246, 0.1)",
				"fill":            true,
			},
			{
				"label":           "成功请求",
				"data":            successData,
				"borderColor":     "#10b981",
				"backgroundColor": "rgba(16, 185, 129, 0.1)",
				"fill":            true,
			},
			{
				"label":           "失败请求",
				"data":            failedData,
				"borderColor":     "#ef4444",
				"backgroundColor": "rgba(239, 68, 68, 0.1)",
				"fill":            true,
			},
		},
	})
}

// handleResponseTimes处理响应时间图表API
func (ws *WebServer) handleResponseTimes(c *gin.Context) {
	metrics := ws.monitoringMiddleware.GetMetrics()
	
	// 解析时间范围参数
	minutes := 30 // 默认30分钟
	if m := c.Query("minutes"); m != "" {
		if parsed, err := strconv.Atoi(m); err == nil && parsed > 0 {
			minutes = parsed
		}
	}
	
	responseHistory := metrics.GetChartDataForResponseTime(minutes)
	
	// 转换为Chart.js格式
	labels := make([]string, len(responseHistory))
	avgData := make([]int64, len(responseHistory))
	minData := make([]int64, len(responseHistory))
	maxData := make([]int64, len(responseHistory))
	
	for i, point := range responseHistory {
		labels[i] = point.Timestamp.Format("15:04")
		avgData[i] = point.AverageTime.Milliseconds()
		minData[i] = point.MinTime.Milliseconds()
		maxData[i] = point.MaxTime.Milliseconds()
	}
	
	c.JSON(http.StatusOK, map[string]interface{}{
		"labels": labels,
		"datasets": []map[string]interface{}{
			{
				"label":           "平均响应时间",
				"data":            avgData,
				"borderColor":     "#f59e0b",
				"backgroundColor": "rgba(245, 158, 11, 0.1)",
				"fill":            true,
			},
			{
				"label":           "最小响应时间",
				"data":            minData,
				"borderColor":     "#10b981",
				"backgroundColor": "rgba(16, 185, 129, 0.1)",
				"fill":            false,
			},
			{
				"label":           "最大响应时间",
				"data":            maxData,
				"borderColor":     "#ef4444",
				"backgroundColor": "rgba(239, 68, 68, 0.1)",
				"fill":            false,
			},
		},
	})
}

// handleEndpointHealth处理端点健康状态图表API
func (ws *WebServer) handleEndpointHealth(c *gin.Context) {
	metrics := ws.monitoringMiddleware.GetMetrics()
	healthDistribution := metrics.GetEndpointHealthDistribution()
	
	// 转换为Chart.js饼图格式
	c.JSON(http.StatusOK, map[string]interface{}{
		"labels": []string{"健康端点", "不健康端点"},
		"datasets": []map[string]interface{}{
			{
				"data":            []int{healthDistribution["healthy"], healthDistribution["unhealthy"]},
				"backgroundColor": []string{"#10b981", "#ef4444"},
				"borderColor":     []string{"#059669", "#dc2626"},
				"borderWidth":     2,
			},
		},
	})
}

// handleConnectionActivity处理连接活动图表API
func (ws *WebServer) handleConnectionActivity(c *gin.Context) {
	metrics := ws.monitoringMiddleware.GetMetrics()
	
	// 解析时间范围参数
	minutes := 60 // 默认1小时
	if m := c.Query("minutes"); m != "" {
		if parsed, err := strconv.Atoi(m); err == nil && parsed > 0 {
			minutes = parsed
		}
	}
	
	activityData := metrics.GetConnectionActivityData(minutes)
	
	// 转换为Chart.js格式
	labels := make([]string, len(activityData))
	connectionCounts := make([]int, len(activityData))
	
	for i, point := range activityData {
		labels[i] = point["time"].(string)
		connectionCounts[i] = point["count"].(int)
	}
	
	c.JSON(http.StatusOK, map[string]interface{}{
		"labels": labels,
		"datasets": []map[string]interface{}{
			{
				"label":           "连接数",
				"data":            connectionCounts,
				"borderColor":     "#8b5cf6",
				"backgroundColor": "rgba(139, 92, 246, 0.1)",
				"fill":            true,
			},
		},
	})
}

// handleGroups处理组管理API
func (ws *WebServer) handleGroups(c *gin.Context) {
	groupDetails := ws.endpointManager.GetGroupDetails()
	
	// 为组信息添加挂起请求相关数据
	metrics := ws.monitoringMiddleware.GetMetrics()
	suspendedConnections := metrics.GetActiveSuspendedConnections()
	
	// 统计每个组的挂起请求数量
	groupSuspendedCounts := make(map[string]int)
	for _, conn := range suspendedConnections {
		// 根据endpoint名称查找对应的组
		endpoints := ws.endpointManager.GetEndpoints()
		for _, ep := range endpoints {
			if ep.Config.Name == conn.Endpoint {
				groupSuspendedCounts[ep.Config.Group]++
				break
			}
		}
	}
	
	// 为响应数据添加挂起信息
	response := map[string]interface{}{
		"groups":                groupDetails["groups"],
		"active_group":          groupDetails["active_group"],
		"total_groups":          groupDetails["total_groups"],
		"auto_switch_enabled":   groupDetails["auto_switch_enabled"],
		"group_suspended_counts": groupSuspendedCounts,
		"total_suspended_requests": len(suspendedConnections),
		"timestamp":             time.Now().Format("2006-01-02 15:04:05"),
	}
	
	c.JSON(http.StatusOK, response)
}

// handleActivateGroup处理手动激活组API
func (ws *WebServer) handleActivateGroup(c *gin.Context) {
	groupName := c.Param("name")
	
	if groupName == "" {
		c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "组名不能为空",
		})
		return
	}
	
	err := ws.endpointManager.ManualActivateGroup(groupName)
	if err != nil {
		c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": err.Error(),
		})
		return
	}
	
	ws.logger.Info("🔄 组已通过Web界面手动激活", "group", groupName)
	
	c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("组 %s 已成功激活", groupName),
		"timestamp": time.Now().Format("2006-01-02 15:04:05"),
	})
}

// handlePauseGroup处理手动暂停组API
func (ws *WebServer) handlePauseGroup(c *gin.Context) {
	groupName := c.Param("name")
	
	var request struct {
		Duration string `json:"duration"` // 可选的暂停时长，如"30m", "1h"等
	}
	
	if err := c.ShouldBindJSON(&request); err != nil {
		request.Duration = "" // 默认无限期暂停
	}
	
	var duration time.Duration
	if request.Duration != "" {
		var err error
		duration, err = time.ParseDuration(request.Duration)
		if err != nil {
			c.JSON(http.StatusBadRequest, map[string]interface{}{
				"error": fmt.Sprintf("无效的时间格式: %s", request.Duration),
			})
			return
		}
	}
	
	err := ws.endpointManager.ManualPauseGroup(groupName, duration)
	if err != nil {
		c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": err.Error(),
		})
		return
	}
	
	ws.logger.Info("⏸️ 组已通过Web界面手动暂停", "group", groupName, "duration", request.Duration)
	
	message := fmt.Sprintf("组 %s 已暂停", groupName)
	if duration > 0 {
		message += fmt.Sprintf("，将在 %v 后自动恢复", duration)
	} else {
		message += "，需要手动恢复"
	}
	
	c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"message": message,
		"timestamp": time.Now().Format("2006-01-02 15:04:05"),
	})
}

// handleResumeGroup处理手动恢复组API
func (ws *WebServer) handleResumeGroup(c *gin.Context) {
	groupName := c.Param("name")
	
	err := ws.endpointManager.ManualResumeGroup(groupName)
	if err != nil {
		c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": err.Error(),
		})
		return
	}
	
	ws.logger.Info("▶️ 组已通过Web界面手动恢复", "group", groupName)
	
	c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("组 %s 已恢复", groupName),
		"timestamp": time.Now().Format("2006-01-02 15:04:05"),
	})
}

// calculateTokenPercentage计算Token百分比
func calculateTokenPercentage(part, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(part) / float64(total) * 100
}

// handleSuspendedRequests处理挂起请求统计API
func (ws *WebServer) handleSuspendedRequests(c *gin.Context) {
	metrics := ws.monitoringMiddleware.GetMetrics()
	suspendedStats := metrics.GetSuspendedRequestStats()
	
	// 解析时间范围参数
	minutes := 60 // 默认1小时
	if m := c.Query("minutes"); m != "" {
		if parsed, err := strconv.Atoi(m); err == nil && parsed > 0 {
			minutes = parsed
		}
	}
	
	suspendedHistory := metrics.GetChartDataForSuspendedRequests(minutes)
	
	c.JSON(http.StatusOK, map[string]interface{}{
		"current": suspendedStats,
		"history": suspendedHistory,
		"suspended_connections": metrics.GetActiveSuspendedConnections(),
		"timestamp": time.Now().Format("2006-01-02 15:04:05"),
	})
}

// handleSuspendedChart处理挂起请求图表API
func (ws *WebServer) handleSuspendedChart(c *gin.Context) {
	metrics := ws.monitoringMiddleware.GetMetrics()
	
	// 解析时间范围参数
	minutes := 30 // 默认30分钟
	if m := c.Query("minutes"); m != "" {
		if parsed, err := strconv.Atoi(m); err == nil && parsed > 0 {
			minutes = parsed
		}
	}
	
	suspendedHistory := metrics.GetChartDataForSuspendedRequests(minutes)
	
	// 转换为Chart.js格式
	labels := make([]string, len(suspendedHistory))
	suspendedData := make([]int64, len(suspendedHistory))
	successfulData := make([]int64, len(suspendedHistory))
	timeoutData := make([]int64, len(suspendedHistory))
	
	for i, point := range suspendedHistory {
		labels[i] = point.Timestamp.Format("15:04")
		suspendedData[i] = point.SuspendedRequests
		successfulData[i] = point.SuccessfulSuspendedRequests
		timeoutData[i] = point.TimeoutSuspendedRequests
	}
	
	c.JSON(http.StatusOK, map[string]interface{}{
		"labels": labels,
		"datasets": []map[string]interface{}{
			{
				"label":           "当前挂起请求",
				"data":            suspendedData,
				"borderColor":     "#f59e0b",
				"backgroundColor": "rgba(245, 158, 11, 0.1)",
				"fill":            true,
			},
			{
				"label":           "成功恢复",
				"data":            successfulData,
				"borderColor":     "#10b981",
				"backgroundColor": "rgba(16, 185, 129, 0.1)",
				"fill":            false,
			},
			{
				"label":           "超时失败",
				"data":            timeoutData,
				"borderColor":     "#ef4444",
				"backgroundColor": "rgba(239, 68, 68, 0.1)",
				"fill":            false,
			},
		},
	})
}
const indexHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Claude Request Forwarder - Web界面</title>
    <link rel="stylesheet" href="/static/css/style.css">
    <!-- Chart.js CDN -->
    <script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.0/dist/chart.umd.js"></script>
    <style>
        .connection-indicator {
            position: absolute;
            top: 20px;
            right: 20px;
            font-size: 20px;
            cursor: help;
            transition: all 0.3s ease;
        }
        .connection-indicator.connected {
            color: #10b981;
        }
        .connection-indicator.connecting {
            color: #f59e0b;
            animation: pulse 1s infinite;
        }
        .connection-indicator.reconnecting {
            color: #f97316;
            animation: pulse 1.5s infinite;
        }
        .connection-indicator.error {
            color: #ef4444;
        }
        .connection-indicator.failed {
            color: #6b7280;
        }
        .connection-indicator.disconnected {
            color: #9ca3af;
        }
        @keyframes pulse {
            0%, 100% { opacity: 1; }
            50% { opacity: 0.5; }
        }
        header {
            position: relative;
        }
        .notification {
            animation: slideIn 0.3s ease-out;
        }
        @keyframes slideIn {
            from {
                transform: translateX(100%);
                opacity: 0;
            }
            to {
                transform: translateX(0);
                opacity: 1;
            }
        }
        
        /* 图表样式 */
        .charts-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(500px, 1fr));
            gap: 20px;
            margin-top: 20px;
        }
        .chart-container {
            background: white;
            border-radius: 12px;
            padding: 20px;
            box-shadow: 0 2px 10px rgba(0, 0, 0, 0.1);
            min-height: 400px;
            position: relative;
        }
        .chart-header {
            display: flex;
            justify-content: between;
            align-items: center;
            margin-bottom: 15px;
            padding-bottom: 10px;
            border-bottom: 1px solid #e5e7eb;
        }
        .chart-title {
            font-size: 18px;
            font-weight: 600;
            color: #1f2937;
        }
        .chart-controls {
            display: flex;
            gap: 10px;
        }
        .chart-controls select {
            padding: 5px 10px;
            border: 1px solid #d1d5db;
            border-radius: 6px;
            font-size: 12px;
            background: white;
        }
        .chart-controls button {
            padding: 5px 10px;
            border: 1px solid #d1d5db;
            border-radius: 6px;
            font-size: 12px;
            background: white;
            cursor: pointer;
            transition: all 0.2s ease;
        }
        .chart-controls button:hover {
            background: #f3f4f6;
        }
        .chart-canvas {
            position: relative;
            height: 300px;
            width: 100%;
        }
        .chart-loading {
            position: absolute;
            top: 50%;
            left: 50%;
            transform: translate(-50%, -50%);
            color: #6b7280;
            font-size: 14px;
        }
        
        @media (max-width: 768px) {
            .charts-grid {
                grid-template-columns: 1fr;
            }
            .chart-container {
                min-height: 300px;
            }
            .chart-canvas {
                height: 250px;
            }
        }
        
        /* 挂起请求相关样式 */
        .alert-banner {
            background: linear-gradient(135deg, #fef3c7, #fbbf24);
            border: 2px solid #f59e0b;
            border-radius: 12px;
            padding: 15px;
            margin-bottom: 20px;
            display: flex;
            align-items: center;
            gap: 12px;
            box-shadow: 0 4px 12px rgba(245, 158, 11, 0.2);
            animation: slideInFromTop 0.5s ease-out;
        }
        
        .alert-banner.warning {
            background: linear-gradient(135deg, #fef3c7, #fbbf24);
            border-color: #f59e0b;
        }
        
        .alert-banner.info {
            background: linear-gradient(135deg, #dbeafe, #60a5fa);
            border-color: #3b82f6;
            box-shadow: 0 4px 12px rgba(59, 130, 246, 0.2);
        }
        
        .alert-icon {
            font-size: 24px;
            flex-shrink: 0;
        }
        
        .alert-content {
            flex-grow: 1;
        }
        
        .alert-title {
            font-weight: 600;
            font-size: 16px;
            margin-bottom: 4px;
            color: #1f2937;
        }
        
        .alert-message {
            font-size: 14px;
            color: #4b5563;
            line-height: 1.4;
        }
        
        .alert-close {
            background: none;
            border: none;
            font-size: 20px;
            cursor: pointer;
            color: #6b7280;
            padding: 4px 8px;
            border-radius: 4px;
            transition: all 0.2s ease;
        }
        
        .alert-close:hover {
            background: rgba(0, 0, 0, 0.1);
            color: #1f2937;
        }
        
        @keyframes slideInFromTop {
            from {
                transform: translateY(-20px);
                opacity: 0;
            }
            to {
                transform: translateY(0);
                opacity: 1;
            }
        }
        
        .suspended-connection-item {
            background: #fef3c7;
            border-left: 4px solid #f59e0b;
            padding: 12px;
            margin-bottom: 8px;
            border-radius: 0 8px 8px 0;
            transition: all 0.2s ease;
        }
        
        .suspended-connection-item:hover {
            background: #fde68a;
            transform: translateX(2px);
        }
        
        .connection-header {
            display: flex;
            justify-content: between;
            align-items: center;
            margin-bottom: 8px;
        }
        
        .connection-id {
            font-weight: 600;
            font-family: monospace;
            font-size: 12px;
            color: #1f2937;
        }
        
        .suspended-time {
            color: #f59e0b;
            font-weight: 500;
            font-size: 12px;
        }
        
        .connection-details {
            font-size: 12px;
            color: #6b7280;
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 8px;
        }
        
        .text-muted {
            color: #6b7280;
            font-size: 12px;
        }
        
        .text-warning {
            color: #f59e0b;
            font-weight: 500;
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <h1>🌐 Claude Request Forwarder</h1>
            <p>高性能API请求转发器 - Web监控界面</p>
        </header>

        <nav class="nav-tabs">
            <button class="nav-tab active" onclick="showTab('overview')">📊 概览</button>
            <button class="nav-tab" onclick="showTab('charts')">📈 图表</button>
            <button class="nav-tab" onclick="showTab('endpoints')">📡 端点</button>
            <button class="nav-tab" onclick="showTab('groups')">📦 组管理</button>
            <button class="nav-tab" onclick="showTab('connections')">🔗 连接</button>
            <button class="nav-tab" onclick="showTab('logs')">📝 日志</button>
            <button class="nav-tab" onclick="showTab('config')">⚙️ 配置</button>
        </nav>

        <main>
            <!-- 概览标签页 -->
            <div id="overview" class="tab-content active">
                <div class="cards">
                    <div class="card">
                        <h3>🚀 服务状态</h3>
                        <p id="server-status">加载中...</p>
                    </div>
                    <div class="card">
                        <h3>⏱️ 运行时间</h3>
                        <p id="uptime">加载中...</p>
                    </div>
                    <div class="card">
                        <h3>📡 端点数量</h3>
                        <p id="endpoint-count">加载中...</p>
                    </div>
                    <div class="card">
                        <h3>🔗 总请求数</h3>
                        <p id="total-requests">加载中...</p>
                    </div>
                    <div class="card">
                        <h3>⏸️ 挂起请求</h3>
                        <p id="suspended-requests">加载中...</p>
                        <small id="suspended-success-rate" class="text-muted">成功率: --</small>
                    </div>
                    <div class="card">
                        <h3>🔄 当前活动组</h3>
                        <p id="active-group">加载中...</p>
                        <small id="group-suspended-info" class="text-warning"></small>
                    </div>
                </div>
            </div>

            <!-- 图表标签页 -->
            <div id="charts" class="tab-content">
                <div class="section">
                    <h2>📈 数据可视化</h2>
                    <div class="charts-grid">
                        <!-- 请求趋势图 -->
                        <div class="chart-container">
                            <div class="chart-header">
                                <div class="chart-title">请求趋势</div>
                                <div class="chart-controls">
                                    <select id="requestTrendTimeRange" onchange="updateChartTimeRange('requestTrend', this.value)">
                                        <option value="15">15分钟</option>
                                        <option value="30" selected>30分钟</option>
                                        <option value="60">1小时</option>
                                        <option value="180">3小时</option>
                                    </select>
                                    <button onclick="exportChart('requestTrend', '请求趋势图.png')" title="导出图片">📷</button>
                                </div>
                            </div>
                            <div class="chart-canvas">
                                <canvas id="requestTrendChart"></canvas>
                                <div id="requestTrendLoading" class="chart-loading">加载中...</div>
                            </div>
                        </div>

                        <!-- 响应时间图 -->
                        <div class="chart-container">
                            <div class="chart-header">
                                <div class="chart-title">响应时间</div>
                                <div class="chart-controls">
                                    <select id="responseTimeTimeRange" onchange="updateChartTimeRange('responseTime', this.value)">
                                        <option value="15">15分钟</option>
                                        <option value="30" selected>30分钟</option>
                                        <option value="60">1小时</option>
                                        <option value="180">3小时</option>
                                    </select>
                                    <button onclick="exportChart('responseTime', '响应时间图.png')" title="导出图片">📷</button>
                                </div>
                            </div>
                            <div class="chart-canvas">
                                <canvas id="responseTimeChart"></canvas>
                                <div id="responseTimeLoading" class="chart-loading">加载中...</div>
                            </div>
                        </div>

                        <!-- Token使用统计 -->
                        <div class="chart-container">
                            <div class="chart-header">
                                <div class="chart-title">Token使用分布</div>
                                <div class="chart-controls">
                                    <button onclick="exportChart('tokenUsage', 'Token使用图.png')" title="导出图片">📷</button>
                                </div>
                            </div>
                            <div class="chart-canvas">
                                <canvas id="tokenUsageChart"></canvas>
                                <div id="tokenUsageLoading" class="chart-loading">加载中...</div>
                            </div>
                        </div>

                        <!-- 端点健康状态 -->
                        <div class="chart-container">
                            <div class="chart-header">
                                <div class="chart-title">端点健康状态</div>
                                <div class="chart-controls">
                                    <button onclick="exportChart('endpointHealth', '端点健康图.png')" title="导出图片">📷</button>
                                </div>
                            </div>
                            <div class="chart-canvas">
                                <canvas id="endpointHealthChart"></canvas>
                                <div id="endpointHealthLoading" class="chart-loading">加载中...</div>
                            </div>
                        </div>

                        <!-- 连接活动 -->
                        <div class="chart-container">
                            <div class="chart-header">
                                <div class="chart-title">连接活动</div>
                                <div class="chart-controls">
                                    <select id="connectionActivityTimeRange" onchange="updateChartTimeRange('connectionActivity', this.value)">
                                        <option value="30">30分钟</option>
                                        <option value="60" selected>1小时</option>
                                        <option value="180">3小时</option>
                                        <option value="360">6小时</option>
                                    </select>
                                    <button onclick="exportChart('connectionActivity', '连接活动图.png')" title="导出图片">📷</button>
                                </div>
                            </div>
                            <div class="chart-canvas">
                                <canvas id="connectionActivityChart"></canvas>
                                <div id="connectionActivityLoading" class="chart-loading">加载中...</div>
                            </div>
                        </div>

                        <!-- 端点性能对比 -->
                        <div class="chart-container">
                            <div class="chart-header">
                                <div class="chart-title">端点性能对比</div>
                                <div class="chart-controls">
                                    <button onclick="exportChart('endpointPerformance', '端点性能图.png')" title="导出图片">📷</button>
                                </div>
                            </div>
                            <div class="chart-canvas">
                                <canvas id="endpointPerformanceChart"></canvas>
                                <div id="endpointPerformanceLoading" class="chart-loading">加载中...</div>
                            </div>
                        </div>
                        
                        <!-- 挂起请求趋势 -->
                        <div class="chart-container">
                            <div class="chart-header">
                                <div class="chart-title">挂起请求趋势</div>
                                <div class="chart-controls">
                                    <select id="suspendedTrendTimeRange" onchange="updateChartTimeRange('suspendedTrend', this.value)">
                                        <option value="15">15分钟</option>
                                        <option value="30" selected>30分钟</option>
                                        <option value="60">1小时</option>
                                        <option value="180">3小时</option>
                                    </select>
                                    <button onclick="exportChart('suspendedTrend', '挂起请求趋势图.png')" title="导出图片">📷</button>
                                </div>
                            </div>
                            <div class="chart-canvas">
                                <canvas id="suspendedTrendChart"></canvas>
                                <div id="suspendedTrendLoading" class="chart-loading">加载中...</div>
                            </div>
                        </div>
                    </div>
                </div>
            </div>

            <!-- 端点标签页 -->
            <div id="endpoints" class="tab-content">
                <div class="section">
                    <h2>📡 端点状态</h2>
                    <div id="endpoints-table">
                        <p>加载中...</p>
                    </div>
                </div>
            </div>

            <!-- 组管理标签页 -->
            <div id="groups" class="tab-content">
                <div class="section">
                    <h2>📦 组管理</h2>
                    
                    <!-- 挂起请求提示信息 -->
                    <div id="group-suspended-alert" class="alert-banner" style="display: none;">
                        <div class="alert-icon">⏸️</div>
                        <div class="alert-content">
                            <div class="alert-title">挂起请求通知</div>
                            <div class="alert-message" id="suspended-alert-message">有请求正在等待组切换...</div>
                        </div>
                        <button class="alert-close" onclick="hideSuspendedAlert()">×</button>
                    </div>
                    
                    <div class="group-info-cards" id="group-info-cards">
                        <p>加载中...</p>
                    </div>
                    <div class="groups-container" id="groups-container">
                        <p>加载中...</p>
                    </div>
                </div>
            </div>

            <!-- 连接标签页 -->
            <div id="connections" class="tab-content">
                <div class="section">
                    <h2>🔗 连接统计</h2>
                    <div id="connections-stats">
                        <p>加载中...</p>
                    </div>
                    
                    <!-- 挂起请求统计 -->
                    <h3>⏸️ 挂起请求状态</h3>
                    <div id="suspended-stats" class="cards">
                        <div class="card">
                            <h4>当前挂起</h4>
                            <p id="current-suspended">0</p>
                        </div>
                        <div class="card">
                            <h4>历史总数</h4>
                            <p id="total-suspended">0</p>
                        </div>
                        <div class="card">
                            <h4>成功恢复</h4>
                            <p id="successful-suspended">0</p>
                        </div>
                        <div class="card">
                            <h4>超时失败</h4>
                            <p id="timeout-suspended">0</p>
                        </div>
                        <div class="card">
                            <h4>成功率</h4>
                            <p id="suspended-success-rate-detail">0%</p>
                        </div>
                        <div class="card">
                            <h4>平均挂起时间</h4>
                            <p id="avg-suspended-time">0ms</p>
                        </div>
                    </div>
                    
                    <!-- 当前挂起的连接列表 -->
                    <div id="suspended-connections-section">
                        <h3>当前挂起的连接</h3>
                        <div id="suspended-connections-table">
                            <p>无挂起连接</p>
                        </div>
                    </div>
                </div>
            </div>

            <!-- 日志标签页 -->
            <div id="logs" class="tab-content">
                <div class="section">
                    <h2>📝 系统日志</h2>
                    <div id="logs-container">
                        <p>加载中...</p>
                    </div>
                </div>
            </div>

            <!-- 配置标签页 -->
            <div id="config" class="tab-content">
                <div class="section">
                    <h2>⚙️ 当前配置</h2>
                    <div id="config-display">
                        <p>加载中...</p>
                    </div>
                </div>
            </div>
        </main>
    </div>

    <script src="/static/js/charts.js"></script>
    <script src="/static/js/app.js"></script>
    <script>
        // 全局图表管理器
        let chartManager = null;

        // 等待页面完全加载后再扩展功能
        window.addEventListener('load', function() {
            // 等待WebInterface初始化完成
            setTimeout(() => {
                if (window.webInterface) {
                    console.log('📊 扩展图表功能到WebInterface');
                    
                    // 保存原始的showTab方法
                    const originalShowTab = window.webInterface.showTab.bind(window.webInterface);
                    
                    // 扩展showTab方法以支持图表
                    window.webInterface.showTab = function(tabName) {
                        originalShowTab(tabName);
                        
                        // 当切换到图表标签时，确保图表已初始化并更新数据
                        if (tabName === 'charts') {
                            initializeCharts();
                        }
                    };
                    
                    console.log('✅ 图表功能扩展完成');
                } else {
                    console.error('❌ WebInterface未找到，无法扩展图表功能');
                }
            }, 200);
        });

        // 初始化图表
        async function initializeCharts() {
            if (chartManager) {
                return; // 已经初始化过了
            }
            
            try {
                console.log('🔄 开始初始化图表系统...');
                chartManager = new ChartManager();
                await chartManager.initializeCharts();
                
                // 隐藏加载指示器
                document.querySelectorAll('.chart-loading').forEach(loading => {
                    loading.style.display = 'none';
                });
                
                console.log('✅ 图表系统初始化完成');
            } catch (error) {
                console.error('❌ 图表初始化失败:', error);
                if (window.webInterface && window.webInterface.showError) {
                    window.webInterface.showError('图表初始化失败: ' + error.message);
                }
            }
        }

        // 更新图表时间范围
        function updateChartTimeRange(chartName, minutes) {
            if (chartManager) {
                chartManager.updateTimeRange(chartName, parseInt(minutes));
            }
        }

        // 导出图表
        function exportChart(chartName, filename) {
            if (chartManager) {
                chartManager.exportChart(chartName, filename);
            } else {
                window.webInterface?.showError('图表管理器未初始化');
            }
        }

        // 页面卸载时清理图表资源
        window.addEventListener('beforeunload', () => {
            if (chartManager) {
                chartManager.destroy();
            }
        });
    </script>
</body>
</html>`