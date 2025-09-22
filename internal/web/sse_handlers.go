package web

import (
	"net/http"
	"strings"
	"time"
	"errors"
	"net"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

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

	ws.logger.Debug("🔍 SSE客户端连接", "client_id", clientID, "filter_count", len(filter), "remote_addr", c.Request.RemoteAddr)

	// 使用context来管理连接生命周期
	ctx := c.Request.Context()
	
	// 发送初始连接确认
	if err := ws.sendSSEEvent(c, "connection", map[string]interface{}{
		"status": "established",
		"message": "SSE连接已建立",
		"timestamp": time.Now().Format("2006-01-02 15:04:05"),
	}); err != nil {
		ws.logger.Debug("❌ 发送连接确认失败", "client_id", clientID, "error", err)
		return
	}
	
	// 发送初始状态数据
	if err := ws.sendSSEInitialData(c); err != nil {
		ws.logger.Debug("❌ 发送初始数据失败", "client_id", clientID, "error", err)
		return
	}
	ws.logger.Debug("✅ SSE初始数据发送成功", "client_id", clientID)

	// 注意：移除定时轮询机制，改为纯事件驱动
	// 原先的5秒定时轮询已被移除，现在依赖事件推送
	
	// 创建客户端连接并注册到事件管理器
	client := ws.eventManager.AddClient(clientID, filter)
	defer ws.eventManager.RemoveClient(clientID)

	ws.logger.Debug("SSE客户端已注册", "client_id", clientID, "total_clients", ws.eventManager.GetClientCount())

	// 心跳ticker保持连接活跃 (恢复到30秒，连接稳定后无需过长间隔)
	heartbeatTicker := time.NewTicker(30 * time.Second)
	defer heartbeatTicker.Stop()

	for {
		select {
		case event, ok := <-client.Channel:
			if !ok {
				// 客户端Channel已关闭，退出SSE处理循环
				ws.logger.Debug("📤 客户端Channel已关闭", "client_id", clientID)
				return
			}
			// 处理事件推送
			ws.logger.Debug("📡 SSE推送事件", "client_id", clientID, "event_type", event.Type)
			if err := ws.writeSSEEvent(c, event); err != nil {
				ws.logger.Debug("❌ 发送SSE事件失败", "client_id", clientID, "error", err, "event_type", event.Type)
				return
			}
			ws.logger.Debug("✅ SSE事件推送成功", "client_id", clientID, "event_type", event.Type)

		case <-heartbeatTicker.C:
			// 发送心跳保持连接 - 智能错误处理，区分临时错误和致命错误
			ws.logger.Debug("💓 发送SSE心跳", "client_id", clientID)
			if err := ws.writeSSEPing(c); err != nil {
				// 检查是否为致命连接错误
				if isConnectionError(err) {
					ws.logger.Debug("💔 心跳检测到连接已断开", "client_id", clientID, "error", err)
					return
				} else {
					// 非致命错误，记录警告但继续维持连接
					ws.logger.Warn("⚠️ 发送心跳失败，但保持连接", "client_id", clientID, "error", err)
				}
			} else {
				// 心跳发送成功，更新客户端最后活动时间，防止被清理机制误删
				ws.eventManager.UpdateClientPing(clientID)
				ws.logger.Debug("✅ SSE心跳发送成功", "client_id", clientID)
			}

		case <-ctx.Done():
			// 客户端断开连接
			ws.logger.Debug("🔌 SSE客户端主动断开连接", "client_id", clientID, "context_error", ctx.Err())
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
	// 发送服务状态 - 只发送启动时间戳，由前端计算实时运行时间
	statusData := map[string]interface{}{
		"status":           "running",
		"start_timestamp":  ws.startTime.Unix(),                         // Unix时间戳用于前端计算
		"start_time":       ws.startTime.Format("2006-01-02 15:04:05"), // 格式化时间用于显示
		"config_file":      ws.configPath,
	}
	if err := ws.sendSSEEvent(c, "status", statusData); err != nil {
		return err
	}

	// 发送端点数据
	if err := ws.sendSSEEndpointsUpdate(c); err != nil {
		return err
	}
	
	// 发送组数据
	if err := ws.sendSSEGroupsUpdate(c); err != nil {
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

// sendSSEGroupsUpdate发送组数据更新
func (ws *WebServer) sendSSEGroupsUpdate(c *gin.Context) error {
	groupDetails := ws.endpointManager.GetGroupDetails()
	
	return ws.sendSSEEvent(c, "group", map[string]interface{}{
		"groups":              groupDetails["groups"],
		"active_group":        groupDetails["active_group"],
		"total_groups":        groupDetails["total_groups"],
		"auto_switch_enabled": groupDetails["auto_switch_enabled"],
		"timestamp":           time.Now().Format("2006-01-02 15:04:05"),
	})
}

// sendSSEConnectionsUpdate发送连接统计更新
func (ws *WebServer) sendSSEConnectionsUpdate(c *gin.Context) error {
	metrics := ws.monitoringMiddleware.GetMetrics()
	stats := metrics.GetMetrics()

	// 计算总Token使用量
	totalTokens := stats.TotalTokenUsage.InputTokens +
		stats.TotalTokenUsage.OutputTokens +
		stats.TotalTokenUsage.CacheCreationTokens +
		stats.TotalTokenUsage.CacheReadTokens

	connectionData := map[string]interface{}{
		"total_requests":       stats.TotalRequests,
		"active_connections":   len(stats.ActiveConnections),
		"successful_requests":  stats.SuccessfulRequests,
		"failed_requests":      stats.FailedRequests,
		"average_response_time": formatResponseTime(stats.GetAverageResponseTime()),
		"total_tokens":         totalTokens,
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

// isConnectionError 检查错误是否为连接相关的致命错误
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	
	// 检查网络连接错误
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	
	// 检查连接重置、管道损坏等错误
	errStr := err.Error()
	connectionErrors := []string{
		"connection reset by peer",
		"broken pipe",
		"write: connection reset by peer",
		"write: broken pipe", 
		"client disconnected",
		"connection closed",
	}
	
	for _, connErr := range connectionErrors {
		if strings.Contains(strings.ToLower(errStr), connErr) {
			return true
		}
	}
	
	return false
}