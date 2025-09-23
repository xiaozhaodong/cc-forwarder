package events

import (
	"log/slog"
	"time"
)

// SSEAdapter 将 EventBus 事件转换为前端格式
type SSEAdapter struct {
	webBroadcaster SSEBroadcaster
	logger         *slog.Logger
}

// NewSSEAdapter 创建新的SSE适配器
func NewSSEAdapter(broadcaster SSEBroadcaster, logger *slog.Logger) *SSEAdapter {
	return &SSEAdapter{
		webBroadcaster: broadcaster,
		logger:         logger,
	}
}

// BroadcastEvent 广播事件到前端
func (adapter *SSEAdapter) BroadcastEvent(eventType string, data map[string]interface{}) {
	if adapter.webBroadcaster == nil {
		adapter.logger.Debug("No web broadcaster set")
		return
	}

	// 检查 EventManager 是否活跃
	if !adapter.webBroadcaster.IsEventManagerActive() {
		adapter.logger.Debug("EventManager not active, skipping broadcast")
		return
	}

	// 添加时间戳
	if data == nil {
		data = make(map[string]interface{})
	}
	data["timestamp"] = time.Now().Format("2006-01-02 15:04:05")

	// 推送给前端
	adapter.webBroadcaster.BroadcastEvent(eventType, data)

	adapter.logger.Debug("Event broadcasted via SSE adapter",
		"event_type", eventType,
		"data_keys", getMapKeys(data))
}

// IsEventManagerActive 检查EventManager是否活跃
func (adapter *SSEAdapter) IsEventManagerActive() bool {
	if adapter.webBroadcaster == nil {
		return false
	}
	return adapter.webBroadcaster.IsEventManagerActive()
}

// 辅助函数：获取map的键
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}