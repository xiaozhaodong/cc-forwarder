package web

import (
	"fmt"
	"log/slog"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// TriggerCondition 触发条件
type TriggerCondition struct {
	Field    string      // 字段名
	Operator string      // 操作符: "eq", "ne", "gt", "lt", "gte", "lte", "contains", "changed"
	Value    interface{} // 比较值
}

// EventTrigger 事件触发器
type EventTrigger struct {
	Name        string             // 触发器名称
	EventType   EventType          // 事件类型
	Priority    EventPriority      // 优先级
	ChangeTypes []string           // 监听的变化类型
	Conditions  []TriggerCondition // 触发条件
	Enabled     bool               // 是否启用
	Description string             // 描述信息
}

// TriggerManager 触发器管理器
type TriggerManager struct {
	triggers    map[string]*EventTrigger
	eventStates map[string]interface{} // 用于检测变化的状态缓存
	logger      *slog.Logger
}

// NewTriggerManager 创建触发器管理器
func NewTriggerManager(logger *slog.Logger) *TriggerManager {
	tm := &TriggerManager{
		triggers:    make(map[string]*EventTrigger),
		eventStates: make(map[string]interface{}),
		logger:      logger,
	}

	// 注册默认触发器
	tm.registerDefaultTriggers()

	return tm
}

// registerDefaultTriggers 注册默认触发器
func (tm *TriggerManager) registerDefaultTriggers() {
	// 端点健康状态变化 - 高优先级
	tm.AddTrigger(&EventTrigger{
		Name:        "endpoint_health_critical",
		EventType:   EventTypeEndpoint,
		Priority:    HighPriority,
		ChangeTypes: []string{"health_changed", "status_changed"},
		Conditions: []TriggerCondition{
			{Field: "healthy", Operator: "changed", Value: nil}, // 健康状态发生变化
		},
		Enabled:     true,
		Description: "端点健康状态发生变化时立即推送",
	})

	// 响应时间激增 - 中优先级
	tm.AddTrigger(&EventTrigger{
		Name:        "response_time_spike",
		EventType:   EventTypeEndpoint,
		Priority:    NormalPriority,
		ChangeTypes: []string{"performance_changed", "metrics_updated"},
		Conditions: []TriggerCondition{
			{Field: "response_time_ms", Operator: "gt", Value: 5000}, // 响应时间>5秒
		},
		Enabled:     true,
		Description: "端点响应时间超过5秒时推送",
	})

	// 端点完全不可用 - 极高优先级
	tm.AddTrigger(&EventTrigger{
		Name:        "endpoint_down_critical",
		EventType:   EventTypeEndpoint,
		Priority:    HighPriority,
		ChangeTypes: []string{"health_changed"},
		Conditions: []TriggerCondition{
			{Field: "healthy", Operator: "eq", Value: false},
			{Field: "consecutive_fails", Operator: "gte", Value: 3},
		},
		Enabled:     true,
		Description: "端点连续失败3次以上时立即告警",
	})

	// 连接错误率告警 - 高优先级
	tm.AddTrigger(&EventTrigger{
		Name:        "connection_error_rate",
		EventType:   EventTypeConnection,
		Priority:    HighPriority,
		ChangeTypes: []string{"error_spike", "metrics_updated"},
		Conditions: []TriggerCondition{
			{Field: "error_rate", Operator: "gt", Value: 0.1}, // 错误率>10%
		},
		Enabled:     true,
		Description: "连接错误率超过10%时告警",
	})

	// 请求完成 - 常规优先级
	tm.AddTrigger(&EventTrigger{
		Name:        "request_completed",
		EventType:   EventTypeConnection,
		Priority:    NormalPriority,
		ChangeTypes: []string{"request_completed", "request_updated"},
		Conditions:  []TriggerCondition{}, // 无特定条件，所有请求完成都推送
		Enabled:     true,
		Description: "请求完成时推送更新",
	})

	// 组状态变化 - 高优先级
	tm.AddTrigger(&EventTrigger{
		Name:        "group_status_change",
		EventType:   EventTypeGroup,
		Priority:    HighPriority,
		ChangeTypes: []string{"status_changed", "activated", "paused", "resumed"},
		Conditions:  []TriggerCondition{}, // 所有组状态变化都是高优先级
		Enabled:     true,
		Description: "端点组状态变化时立即推送",
	})

	// 请求挂起 - 高优先级
	tm.AddTrigger(&EventTrigger{
		Name:        "request_suspended",
		EventType:   EventTypeSuspended,
		Priority:    HighPriority,
		ChangeTypes: []string{"suspended", "resumed", "timeout"},
		Conditions:  []TriggerCondition{},
		Enabled:     true,
		Description: "请求挂起、恢复或超时时立即推送",
	})

	// 性能降级告警 - 常规优先级
	tm.AddTrigger(&EventTrigger{
		Name:        "performance_degraded",
		EventType:   EventTypeEndpoint,
		Priority:    NormalPriority,
		ChangeTypes: []string{"performance_changed"},
		Conditions: []TriggerCondition{
			{Field: "response_time_ms", Operator: "gt", Value: 2000},
			{Field: "response_time_ms", Operator: "lt", Value: 5000},
		},
		Enabled:     true,
		Description: "端点响应时间在2-5秒之间时推送性能告警",
	})

	// 日志事件 - 低优先级（除非是错误）
	tm.AddTrigger(&EventTrigger{
		Name:        "log_error",
		EventType:   EventTypeLog,
		Priority:    NormalPriority,
		ChangeTypes: []string{"error", "warning"},
		Conditions: []TriggerCondition{
			{Field: "level", Operator: "contains", Value: "error"},
		},
		Enabled:     true,
		Description: "错误级别日志事件时推送",
	})

	tm.logger.Info("已注册默认事件触发器", "trigger_count", len(tm.triggers))
}

// AddTrigger 添加触发器
func (tm *TriggerManager) AddTrigger(trigger *EventTrigger) {
	tm.triggers[trigger.Name] = trigger
	tm.logger.Debug("添加事件触发器",
		"name", trigger.Name,
		"event_type", trigger.EventType,
		"priority", trigger.Priority.String())
}

// RemoveTrigger 移除触发器
func (tm *TriggerManager) RemoveTrigger(name string) {
	if _, exists := tm.triggers[name]; exists {
		delete(tm.triggers, name)
		tm.logger.Debug("移除事件触发器", "name", name)
	}
}

// EnableTrigger 启用触发器
func (tm *TriggerManager) EnableTrigger(name string) bool {
	if trigger, exists := tm.triggers[name]; exists {
		trigger.Enabled = true
		tm.logger.Debug("启用事件触发器", "name", name)
		return true
	}
	return false
}

// DisableTrigger 禁用触发器
func (tm *TriggerManager) DisableTrigger(name string) bool {
	if trigger, exists := tm.triggers[name]; exists {
		trigger.Enabled = false
		tm.logger.Debug("禁用事件触发器", "name", name)
		return true
	}
	return false
}

// ShouldTrigger 检查是否应该触发事件
func (tm *TriggerManager) ShouldTrigger(triggerName string, data interface{}, changeType string) (bool, EventPriority, *EventContext) {
	trigger, exists := tm.triggers[triggerName]
	if !exists || !trigger.Enabled {
		return false, LowPriority, nil
	}

	// 检查变化类型
	if len(trigger.ChangeTypes) > 0 {
		found := false
		for _, ct := range trigger.ChangeTypes {
			if ct == changeType || strings.Contains(changeType, ct) {
				found = true
				break
			}
		}
		if !found {
			return false, LowPriority, nil
		}
	}

	// 检查条件
	if len(trigger.Conditions) > 0 {
		conditionMet, metadata := tm.evaluateConditions(trigger.Conditions, data, triggerName)
		if !conditionMet {
			return false, LowPriority, nil
		}

		// 创建事件上下文
		context := &EventContext{
			Source:     triggerName,
			ChangeType: changeType,
			Metadata:   metadata,
			Priority:   trigger.Priority,
		}

		tm.logger.Debug("触发器条件满足",
			"trigger", triggerName,
			"change_type", changeType,
			"priority", trigger.Priority.String())

		return true, trigger.Priority, context
	}

	// 无条件触发器
	context := &EventContext{
		Source:     triggerName,
		ChangeType: changeType,
		Metadata:   make(map[string]interface{}),
		Priority:   trigger.Priority,
	}

	return true, trigger.Priority, context
}

// EvaluateAllTriggers 评估所有相关触发器
func (tm *TriggerManager) EvaluateAllTriggers(eventType EventType, data interface{}, changeType string) []struct {
	Name     string
	Priority EventPriority
	Context  *EventContext
} {
	var results []struct {
		Name     string
		Priority EventPriority
		Context  *EventContext
	}

	for name, trigger := range tm.triggers {
		if trigger.EventType == eventType && trigger.Enabled {
			if shouldTrigger, priority, context := tm.ShouldTrigger(name, data, changeType); shouldTrigger {
				results = append(results, struct {
					Name     string
					Priority EventPriority
					Context  *EventContext
				}{
					Name:     name,
					Priority: priority,
					Context:  context,
				})
			}
		}
	}

	return results
}

// evaluateConditions 评估触发条件
func (tm *TriggerManager) evaluateConditions(conditions []TriggerCondition, data interface{}, triggerName string) (bool, map[string]interface{}) {
	metadata := make(map[string]interface{})
	
	dataValue := reflect.ValueOf(data)
	if dataValue.Kind() == reflect.Ptr && !dataValue.IsNil() {
		dataValue = dataValue.Elem()
	}

	for _, condition := range conditions {
		met, conditionMeta := tm.evaluateCondition(condition, dataValue, triggerName)
		if !met {
			return false, metadata
		}
		// 合并条件元数据
		for k, v := range conditionMeta {
			metadata[k] = v
		}
	}

	return true, metadata
}

// evaluateCondition 评估单个条件
func (tm *TriggerManager) evaluateCondition(condition TriggerCondition, dataValue reflect.Value, triggerName string) (bool, map[string]interface{}) {
	metadata := make(map[string]interface{})
	
	fieldValue := tm.getFieldValue(dataValue, condition.Field)
	if !fieldValue.IsValid() {
		return false, metadata
	}

	currentValue := fieldValue.Interface()
	metadata["field"] = condition.Field
	metadata["current_value"] = currentValue
	metadata["operator"] = condition.Operator
	metadata["expected_value"] = condition.Value

	switch condition.Operator {
	case "eq":
		return tm.compareValues(currentValue, condition.Value) == 0, metadata
	case "ne":
		return tm.compareValues(currentValue, condition.Value) != 0, metadata
	case "gt":
		return tm.compareValues(currentValue, condition.Value) > 0, metadata
	case "lt":
		return tm.compareValues(currentValue, condition.Value) < 0, metadata
	case "gte":
		return tm.compareValues(currentValue, condition.Value) >= 0, metadata
	case "lte":
		return tm.compareValues(currentValue, condition.Value) <= 0, metadata
	case "contains":
		str1, ok1 := currentValue.(string)
		str2, ok2 := condition.Value.(string)
		if ok1 && ok2 {
			return strings.Contains(str1, str2), metadata
		}
		return false, metadata
	case "changed":
		// 检查值是否发生变化
		stateKey := triggerName + ":" + condition.Field
		if previousValue, exists := tm.eventStates[stateKey]; exists {
			changed := tm.compareValues(currentValue, previousValue) != 0
			tm.eventStates[stateKey] = currentValue
			metadata["previous_value"] = previousValue
			metadata["changed"] = changed
			return changed, metadata
		} else {
			// 首次记录，认为发生了变化
			tm.eventStates[stateKey] = currentValue
			metadata["first_time"] = true
			return true, metadata
		}
	}

	return false, metadata
}

// getFieldValue 获取字段值（支持嵌套字段和Map）
func (tm *TriggerManager) getFieldValue(dataValue reflect.Value, fieldPath string) reflect.Value {
	if !dataValue.IsValid() {
		return reflect.Value{}
	}

	// 支持点号分隔的嵌套字段
	fields := strings.Split(fieldPath, ".")
	currentValue := dataValue

	for _, field := range fields {
		if currentValue.Kind() == reflect.Map {
			// Map类型
			mapValue := currentValue.MapIndex(reflect.ValueOf(field))
			if mapValue.IsValid() {
				currentValue = mapValue
			} else {
				return reflect.Value{}
			}
		} else if currentValue.Kind() == reflect.Struct {
			// 结构体类型
			structValue := currentValue.FieldByName(field)
			if structValue.IsValid() {
				currentValue = structValue
			} else {
				return reflect.Value{}
			}
		} else {
			return reflect.Value{}
		}
	}

	return currentValue
}

// compareValues 比较值
func (tm *TriggerManager) compareValues(a, b interface{}) int {
	// 处理nil值
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}

	switch av := a.(type) {
	case int:
		if bv, ok := tm.convertToInt(b); ok {
			if av < bv {
				return -1
			} else if av > bv {
				return 1
			}
			return 0
		}
	case int64:
		if bv, ok := tm.convertToInt64(b); ok {
			if av < bv {
				return -1
			} else if av > bv {
				return 1
			}
			return 0
		}
	case float64:
		if bv, ok := tm.convertToFloat64(b); ok {
			if av < bv {
				return -1
			} else if av > bv {
				return 1
			}
			return 0
		}
	case string:
		if bv, ok := b.(string); ok {
			return strings.Compare(av, bv)
		}
	case bool:
		if bv, ok := b.(bool); ok {
			if av == bv {
				return 0
			} else if av {
				return 1
			} else {
				return -1
			}
		}
	case time.Time:
		if bv, ok := b.(time.Time); ok {
			if av.Before(bv) {
				return -1
			} else if av.After(bv) {
				return 1
			}
			return 0
		}
	}

	// 默认转换为字符串比较
	return strings.Compare(tm.toString(a), tm.toString(b))
}

// convertToInt 转换为int类型
func (tm *TriggerManager) convertToInt(value interface{}) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case string:
		if i, err := strconv.Atoi(v); err == nil {
			return i, true
		}
	}
	return 0, false
}

// convertToInt64 转换为int64类型
func (tm *TriggerManager) convertToInt64(value interface{}) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int64:
		return v, true
	case float64:
		return int64(v), true
	case string:
		if i, err := strconv.ParseInt(v, 10, 64); err == nil {
			return i, true
		}
	}
	return 0, false
}

// convertToFloat64 转换为float64类型
func (tm *TriggerManager) convertToFloat64(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case float64:
		return v, true
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

// toString 转换为字符串
func (tm *TriggerManager) toString(value interface{}) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case int, int64, float64, bool:
		return fmt.Sprintf("%v", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// GetTriggers 获取所有触发器
func (tm *TriggerManager) GetTriggers() map[string]*EventTrigger {
	triggers := make(map[string]*EventTrigger)
	for k, v := range tm.triggers {
		triggers[k] = v
	}
	return triggers
}

// GetTriggersByEventType 根据事件类型获取触发器
func (tm *TriggerManager) GetTriggersByEventType(eventType EventType) []*EventTrigger {
	var triggers []*EventTrigger
	for _, trigger := range tm.triggers {
		if trigger.EventType == eventType {
			triggers = append(triggers, trigger)
		}
	}
	return triggers
}

// GetStats 获取触发器统计信息
func (tm *TriggerManager) GetStats() map[string]interface{} {
	totalTriggers := len(tm.triggers)
	enabledTriggers := 0
	triggersByType := make(map[EventType]int)
	triggersByPriority := make(map[EventPriority]int)

	for _, trigger := range tm.triggers {
		if trigger.Enabled {
			enabledTriggers++
		}
		triggersByType[trigger.EventType]++
		triggersByPriority[trigger.Priority]++
	}

	return map[string]interface{}{
		"total_triggers":        totalTriggers,
		"enabled_triggers":      enabledTriggers,
		"disabled_triggers":     totalTriggers - enabledTriggers,
		"triggers_by_type":      triggersByType,
		"triggers_by_priority":  triggersByPriority,
		"event_states_count":    len(tm.eventStates),
	}
}