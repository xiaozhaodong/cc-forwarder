package events

import "time"

// 事件类型枚举
type EventType string

const (
	// 请求生命周期事件
	EventRequestStarted   EventType = "request_started"
	EventRequestUpdated   EventType = "request_updated"
	EventRequestCompleted EventType = "request_completed"

	// 端点健康事件
	EventEndpointHealthy   EventType = "endpoint_healthy"
	EventEndpointUnhealthy EventType = "endpoint_unhealthy"

	// 连接统计事件
	EventConnectionStats        EventType = "connection_stats"
	EventConnectionStatsUpdated EventType = "connection_stats_updated"
	EventResponseReceived       EventType = "response_received"

	// 组管理事件
	EventGroupStatusChanged      EventType = "group_status_changed"
	EventGroupHealthStatsChanged EventType = "group_health_stats_changed"

	// 系统级事件
	EventSystemError        EventType = "system_error"
	EventSystemStatsUpdated EventType = "system_stats_updated"
	EventConfigChanged      EventType = "config_changed"
)

// 事件优先级
type EventPriority int

const (
	PriorityLow EventPriority = iota // 批量处理，如统计数据
	PriorityNormal                   // 延迟处理，如请求完成
	PriorityHigh                     // 立即处理，如健康状态变化
	PriorityCritical                 // 紧急处理，如系统错误
)

// 事件结构
type Event struct {
	Type      EventType              `json:"type"`
	Source    string                 `json:"source"`   // 事件来源组件
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
	Priority  EventPriority          `json:"priority"`
	Context   map[string]interface{} `json:"context,omitempty"`
}

// 前端事件类型映射
var EventTypeMapping = map[EventType]string{
	EventRequestStarted:          "request",
	EventRequestUpdated:          "request",
	EventRequestCompleted:        "request",
	EventEndpointHealthy:         "endpoint",
	EventEndpointUnhealthy:       "endpoint",
	EventConnectionStats:         "connection",
	EventConnectionStatsUpdated:  "connection",
	EventResponseReceived:        "connection",
	EventGroupStatusChanged:      "group",
	EventGroupHealthStatsChanged: "group",
	EventSystemError:             "status",
	EventSystemStatsUpdated:      "status",
	EventConfigChanged:           "config",
}