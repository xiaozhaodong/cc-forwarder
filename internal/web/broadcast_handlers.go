package web

import (
	"sync/atomic"
)

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

// IsEventManagerActive 检查EventManager是否仍在活跃状态
func (ws *WebServer) IsEventManagerActive() bool {
	if ws.eventManager == nil {
		return false
	}
	// 通过检查closed标志来判断EventManager是否已关闭
	return atomic.LoadInt64(&ws.eventManager.closed) == 0
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