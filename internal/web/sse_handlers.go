package web

import (
	"net/http"
	"strings"
	"time"

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