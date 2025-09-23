package web

import (
	"sync/atomic"
)

// BroadcastStatusUpdate广播状态更新事件
func (ws *WebServer) BroadcastStatusUpdate(data map[string]interface{}) {
	if ws.eventManager != nil {
		ws.eventManager.BroadcastEventSmart(EventTypeStatus, data, nil)
	}
}

// BroadcastEndpointUpdate广播端点更新事件
func (ws *WebServer) BroadcastEndpointUpdate(data map[string]interface{}) {
	if ws.eventManager != nil {
		ws.eventManager.BroadcastEventSmart(EventTypeEndpoint, data, nil)
	}
}

// IsEventManagerActive 检查EventManager是否仍在活跃状态
func (ws *WebServer) IsEventManagerActive() bool {
	if ws.eventManager == nil || ws.eventManager.EventManager == nil {
		return false
	}
	// 通过检查closed标志来判断EventManager是否已关闭
	return atomic.LoadInt64(&ws.eventManager.EventManager.closed) == 0
}

// BroadcastEvent 实现events.SSEBroadcaster接口
func (ws *WebServer) BroadcastEvent(eventType string, data map[string]interface{}) {
	if ws.eventManager == nil {
		return
	}

	// 根据EventBus的事件类型映射到Web EventType
	var webEventType EventType
	switch eventType {
	case "request":
		webEventType = EventTypeConnection // 请求事件归类为连接事件
	case "endpoint":
		webEventType = EventTypeEndpoint
	case "connection":
		webEventType = EventTypeConnection
	case "status":
		webEventType = EventTypeStatus
	case "config":
		webEventType = EventTypeConfig
	case "group":
		webEventType = EventTypeGroup // 🔥 修复：添加组事件类型映射
	default:
		webEventType = EventTypeStatus // 默认类型
	}

	ws.eventManager.BroadcastEventSmart(webEventType, data, nil)
}

// BroadcastConnectionUpdate广播连接更新事件
func (ws *WebServer) BroadcastConnectionUpdate(data map[string]interface{}) {
	if ws.eventManager != nil {
		ws.eventManager.BroadcastEventSmart(EventTypeConnection, data, nil)
	}
}

// BroadcastConnectionUpdateSmart智能连接更新事件广播
func (ws *WebServer) BroadcastConnectionUpdateSmart(data map[string]interface{}, changeType string) {
	if ws.eventManager != nil {
		// 使用触发器管理器评估事件
		if triggerManager := ws.getTriggerManager(); triggerManager != nil {
			triggers := triggerManager.EvaluateAllTriggers(EventTypeConnection, data, changeType)
			if len(triggers) > 0 {
				// 选择最高优先级的触发器
				highestPriority := LowPriority
				var bestContext *EventContext
				for _, trigger := range triggers {
					if trigger.Priority < highestPriority { // 数值越小，优先级越高
						highestPriority = trigger.Priority
						bestContext = trigger.Context
					}
				}
				
				// 使用智能推送
				ws.eventManager.BroadcastEventSmart(EventTypeConnection, data, bestContext)
				return
			}
		}
		
		// 降级为常规推送
		ws.eventManager.BroadcastEventSmart(EventTypeConnection, data, nil)
	}
}

// BroadcastEndpointUpdateSmart智能端点更新事件广播
func (ws *WebServer) BroadcastEndpointUpdateSmart(data map[string]interface{}, changeType string) {
	if ws.eventManager != nil {
		// 使用触发器管理器评估事件
		if triggerManager := ws.getTriggerManager(); triggerManager != nil {
			triggers := triggerManager.EvaluateAllTriggers(EventTypeEndpoint, data, changeType)
			if len(triggers) > 0 {
				// 选择最高优先级的触发器
				highestPriority := LowPriority
				var bestContext *EventContext
				for _, trigger := range triggers {
					if trigger.Priority < highestPriority {
						highestPriority = trigger.Priority
						bestContext = trigger.Context
					}
				}
				
				// 使用智能推送
				ws.eventManager.BroadcastEventSmart(EventTypeEndpoint, data, bestContext)
				return
			}
		}
		
		// 降级为常规推送
		ws.eventManager.BroadcastEventSmart(EventTypeEndpoint, data, nil)
	}
}

// getTriggerManager 获取触发器管理器
func (ws *WebServer) getTriggerManager() *TriggerManager {
	// 使用SmartEventManager中已经创建的TriggerManager，避免重复创建
	if ws.eventManager != nil && ws.eventManager.triggerManager != nil {
		return ws.eventManager.triggerManager
	}
	return nil
}

// BroadcastLogEvent广播日志事件
func (ws *WebServer) BroadcastLogEvent(data map[string]interface{}) {
	if ws.eventManager != nil {
		ws.eventManager.BroadcastEventSmart(EventTypeLog, data, nil)
	}
}

// BroadcastConfigUpdate广播配置更新事件
func (ws *WebServer) BroadcastConfigUpdate(data map[string]interface{}) {
	if ws.eventManager != nil {
		ws.eventManager.BroadcastEventSmart(EventTypeConfig, data, nil)
	}
}

// BroadcastGroupUpdate广播组更新事件
func (ws *WebServer) BroadcastGroupUpdate(data map[string]interface{}) {
	if ws.eventManager != nil {
		ws.eventManager.BroadcastEventSmart(EventTypeGroup, data, nil)
	}
}

// BroadcastSuspendedUpdate广播挂起请求事件
func (ws *WebServer) BroadcastSuspendedUpdate(data map[string]interface{}) {
	if ws.eventManager != nil {
		ws.eventManager.BroadcastEventSmart(EventTypeSuspended, data, nil)
	}
}