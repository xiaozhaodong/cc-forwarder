package web

import (
	"context"
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// EventPriority 事件优先级
type EventPriority int

const (
	HighPriority   EventPriority = iota // <100ms推送（健康状态变化、错误告警）
	NormalPriority                      // <2s推送（请求完成、常规更新）
	LowPriority                         // <10s推送（统计数据、图表更新）
)

// String 返回优先级的字符串表示
func (p EventPriority) String() string {
	switch p {
	case HighPriority:
		return "high"
	case NormalPriority:
		return "normal"
	case LowPriority:
		return "low"
	default:
		return "unknown"
	}
}

// PushStrategy 推送策略配置
type PushStrategy struct {
	Priority    EventPriority
	MaxDelay    time.Duration // 最大延迟
	BatchSize   int           // 批处理大小
	MergeWindow time.Duration // 合并窗口
}

// EventContext 事件上下文
type EventContext struct {
	Source     string                 // 事件源
	ChangeType string                 // 变化类型
	Metadata   map[string]interface{} // 元数据
	Priority   EventPriority          // 事件优先级
}

// EventBuffer 事件缓冲区
type EventBuffer struct {
	events    []Event
	lastFlush time.Time
	mutex     sync.RWMutex
}

// SmartEventManager 智能事件管理器
type SmartEventManager struct {
	*EventManager

	// 智能推送控制
	pushStrategies map[EventType]*PushStrategy
	eventBuffers   map[EventType]*EventBuffer
	triggerManager *TriggerManager // 新增：触发器管理器

	// 事件处理通道
	immediate chan Event // 立即推送通道
	normal    chan Event // 常规推送通道
	batch     chan Event // 批量推送通道

	// 统计信息
	stats struct {
		totalEvents     int64
		immediateEvents int64
		normalEvents    int64
		batchEvents     int64
		mutex           sync.RWMutex
	}

	logger *slog.Logger
	mutex  sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
}

// NewSmartEventManager 创建智能事件管理器
func NewSmartEventManager(logger *slog.Logger) *SmartEventManager {
	em := NewEventManager(logger)
	ctx, cancel := context.WithCancel(context.Background())

	sem := &SmartEventManager{
		EventManager:   em,
		pushStrategies: make(map[EventType]*PushStrategy),
		eventBuffers:   make(map[EventType]*EventBuffer),
		triggerManager: NewTriggerManager(logger), // 创建单例触发器管理器
		immediate:      make(chan Event, 100),
		normal:         make(chan Event, 500),
		batch:          make(chan Event, 1000),
		logger:         logger,
		ctx:            ctx,
		cancel:         cancel,
	}

	// 配置默认推送策略
	sem.configureDefaultStrategies()

	// 启动智能处理协程
	go sem.processImmediateEvents()
	go sem.processNormalEvents()
	go sem.processBatchEvents()

	return sem
}

// BroadcastEventSmart 智能事件广播
func (sem *SmartEventManager) BroadcastEventSmart(eventType EventType, data interface{}, context *EventContext) {
	// 严格验证输入参数
	if eventType == "" {
		sem.logger.Warn("⚠️ 智能广播尝试使用空事件类型，跳过", "data", data)
		return
	}
	
	sem.stats.mutex.Lock()
	sem.stats.totalEvents++
	sem.stats.mutex.Unlock()

	// 确保时间戳有效
	timestamp := time.Now()
	if timestamp.IsZero() {
		sem.logger.Error("❌ 无法获取当前时间，跳过智能事件广播", "event_type", eventType)
		return
	}

	// 创建增强的事件
	event := Event{
		Type:      eventType,
		Data:      data,
		Timestamp: timestamp,
		Context:   context,
	}
	
	// 二次验证Event结构
	if event.Type == "" || event.Timestamp.IsZero() {
		sem.logger.Error("❌ 智能Event结构验证失败", "type", event.Type, "timestamp", event.Timestamp.Unix())
		return
	}

	// 如果没有提供上下文，使用默认策略
	if context == nil {
		context = &EventContext{
			Priority: NormalPriority,
		}
	}

	// 设置事件优先级
	event.Priority = context.Priority

	strategy := sem.pushStrategies[eventType]
	if strategy == nil {
		// 默认策略
		strategy = &PushStrategy{Priority: NormalPriority, MaxDelay: 2 * time.Second}
	}

	// 根据上下文优先级选择处理通道
	switch context.Priority {
	case HighPriority:
		select {
		case sem.immediate <- event:
			sem.stats.mutex.Lock()
			sem.stats.immediateEvents++
			sem.stats.mutex.Unlock()
		default:
			// 立即通道满，降级到常规通道
			select {
			case sem.normal <- event:
				sem.stats.mutex.Lock()
				sem.stats.normalEvents++
				sem.stats.mutex.Unlock()
			default:
				sem.batch <- event
				sem.stats.mutex.Lock()
				sem.stats.batchEvents++
				sem.stats.mutex.Unlock()
			}
		}
	case NormalPriority:
		select {
		case sem.normal <- event:
			sem.stats.mutex.Lock()
			sem.stats.normalEvents++
			sem.stats.mutex.Unlock()
		default:
			sem.batch <- event
			sem.stats.mutex.Lock()
			sem.stats.batchEvents++
			sem.stats.mutex.Unlock()
		}
	default:
		sem.batch <- event
		sem.stats.mutex.Lock()
		sem.stats.batchEvents++
		sem.stats.mutex.Unlock()
	}
}

// configureDefaultStrategies 配置默认推送策略
func (sem *SmartEventManager) configureDefaultStrategies() {
	sem.pushStrategies[EventTypeEndpoint] = &PushStrategy{
		Priority:    HighPriority,
		MaxDelay:    100 * time.Millisecond,
		BatchSize:   1,
		MergeWindow: 50 * time.Millisecond,
	}

	sem.pushStrategies[EventTypeConnection] = &PushStrategy{
		Priority:    NormalPriority,
		MaxDelay:    2 * time.Second,
		BatchSize:   5,
		MergeWindow: 1 * time.Second,
	}

	sem.pushStrategies[EventTypeChart] = &PushStrategy{
		Priority:    LowPriority,
		MaxDelay:    20 * time.Second,  // 调整为20秒，既不频繁也不过慢
		BatchSize:   1,                 // 保持为1，现在已经在事件源头聚合
		MergeWindow: 5 * time.Second,   // 恢复到5秒合并窗口
	}

	sem.pushStrategies[EventTypeGroup] = &PushStrategy{
		Priority:    HighPriority,
		MaxDelay:    100 * time.Millisecond,
		BatchSize:   1,
		MergeWindow: 50 * time.Millisecond,
	}

	sem.pushStrategies[EventTypeStatus] = &PushStrategy{
		Priority:    NormalPriority,
		MaxDelay:    1 * time.Second,
		BatchSize:   3,
		MergeWindow: 500 * time.Millisecond,
	}

	sem.pushStrategies[EventTypeLog] = &PushStrategy{
		Priority:    LowPriority,
		MaxDelay:    5 * time.Second,
		BatchSize:   15,
		MergeWindow: 3 * time.Second,
	}

	sem.pushStrategies[EventTypeSuspended] = &PushStrategy{
		Priority:    HighPriority,
		MaxDelay:    100 * time.Millisecond,
		BatchSize:   1,
		MergeWindow: 50 * time.Millisecond,
	}
}

// processImmediateEvents 处理立即推送事件
func (sem *SmartEventManager) processImmediateEvents() {
	for {
		select {
		case event, ok := <-sem.immediate:
			if !ok {
				// Channel已关闭，退出goroutine
				sem.logger.Info("立即事件Channel已关闭，processImmediateEvents退出")
				return
			}
			// 确保事件有有效的时间戳
			if event.Timestamp.IsZero() {
				event.Timestamp = time.Now()
			}
			// 直接广播完整事件，避免重复创建
			sem.broadcastCompleteEvent(event)
			sem.logger.Debug("立即推送事件",
				"type", event.Type,
				"priority", event.Priority.String())
		case <-sem.ctx.Done():
			return
		}
	}
}

// processNormalEvents 处理常规事件
func (sem *SmartEventManager) processNormalEvents() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	buffer := make([]Event, 0, 10)

	for {
		select {
		case event, ok := <-sem.normal:
			if !ok {
				// Channel已关闭，处理剩余缓冲区事件后退出
				if len(buffer) > 0 {
					sem.flushBuffer(buffer)
				}
				sem.logger.Info("常规事件Channel已关闭，processNormalEvents退出")
				return
			}
			buffer = append(buffer, event)
			// 检查是否需要立即刷新（高频事件类型）
			strategy := sem.pushStrategies[event.Type]
			if strategy != nil && len(buffer) >= strategy.BatchSize {
				sem.flushBuffer(buffer)
				buffer = buffer[:0]
			}
		case <-ticker.C:
			if len(buffer) > 0 {
				sem.flushBuffer(buffer)
				buffer = buffer[:0]
			}
		case <-sem.ctx.Done():
			return
		}
	}
}

// processBatchEvents 处理批量事件
func (sem *SmartEventManager) processBatchEvents() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	buffer := make([]Event, 0, 20)

	for {
		select {
		case event, ok := <-sem.batch:
			if !ok {
				// Channel已关闭，处理剩余缓冲区事件后退出
				if len(buffer) > 0 {
					sem.flushBatchBuffer(buffer)
				}
				sem.logger.Info("批量事件Channel已关闭，processBatchEvents退出")
				return
			}
			buffer = append(buffer, event)
			// 对于图表类型的事件，执行更激进的批处理
			if len(buffer) >= 15 {
				sem.flushBatchBuffer(buffer)
				buffer = buffer[:0]
			}
		case <-ticker.C:
			if len(buffer) > 0 {
				sem.flushBatchBuffer(buffer)
				buffer = buffer[:0]
			}
		case <-sem.ctx.Done():
			return
		}
	}
}

// flushBuffer 刷新缓冲区
func (sem *SmartEventManager) flushBuffer(events []Event) {
	for i := range events { // 使用索引而不是值拷贝
		// 确保每个事件都有有效的时间戳
		if events[i].Timestamp.IsZero() {
			events[i].Timestamp = time.Now() // 修改原始数据
		}
		// 广播完整事件而不是重新创建
		sem.broadcastCompleteEvent(events[i])
	}
	sem.logger.Debug("刷新常规事件缓冲区", "count", len(events))
}

// flushBatchBuffer 刷新批量缓冲区（可以做事件合并）
func (sem *SmartEventManager) flushBatchBuffer(events []Event) {
	if len(events) == 0 {
		return
	}

	// 首先确保所有events都有有效的时间戳
	for i := range events {
		if events[i].Timestamp.IsZero() {
			events[i].Timestamp = time.Now()
		}
	}
	
	// 按事件类型分组
	eventGroups := make(map[EventType][]Event)
	for _, event := range events {
		eventGroups[event.Type] = append(eventGroups[event.Type], event)
	}

	// 逐组广播，对某些事件类型进行去重
	for eventType, groupEvents := range eventGroups {
		switch eventType {
		case EventTypeChart:
			// 图表数据只发送最新的
			latestEvent := &groupEvents[len(groupEvents)-1] // 获取指针
			if latestEvent.Timestamp.IsZero() {
				latestEvent.Timestamp = time.Now()
			}
			sem.broadcastCompleteEvent(*latestEvent)
			sem.logger.Debug("合并图表事件", "original_count", len(groupEvents), "sent", 1)
		case EventTypeConnection:
			// 连接统计进行智能去重
			deduped := sem.deduplicateConnectionEvents(groupEvents)
			for i := range deduped { // 使用索引遍历
				if deduped[i].Timestamp.IsZero() {
					deduped[i].Timestamp = time.Now()
				}
				sem.broadcastCompleteEvent(deduped[i])
			}
			sem.logger.Debug("去重连接事件", "original_count", len(groupEvents), "sent", len(deduped))
		default:
			// 其他事件类型正常发送
			for i := range groupEvents { // 使用索引遍历
				if groupEvents[i].Timestamp.IsZero() {
					groupEvents[i].Timestamp = time.Now()
				}
				sem.broadcastCompleteEvent(groupEvents[i])
			}
		}
	}

	sem.logger.Debug("刷新批量事件缓冲区",
		"total_events", len(events),
		"event_groups", len(eventGroups))
}

// deduplicateConnectionEvents 去重连接事件
func (sem *SmartEventManager) deduplicateConnectionEvents(events []Event) []Event {
	if len(events) <= 1 {
		return events
	}

	// 简单策略：保留最新的事件，但如果时间间隔很小，进行采样
	var result []Event
	lastTime := time.Time{}

	for _, event := range events {
		// 如果距离上次事件时间超过1秒，或者这是第一个事件，则保留
		if lastTime.IsZero() || event.Timestamp.Sub(lastTime) > 1*time.Second {
			result = append(result, event)
			lastTime = event.Timestamp
		}
	}

	// 确保至少保留最后一个事件
	if len(result) == 0 || !result[len(result)-1].Timestamp.Equal(events[len(events)-1].Timestamp) {
		result = append(result, events[len(events)-1])
	}

	return result
}

// GetStats 获取智能事件管理器统计信息
func (sem *SmartEventManager) GetStats() map[string]interface{} {
	sem.stats.mutex.RLock()
	defer sem.stats.mutex.RUnlock()

	return map[string]interface{}{
		"total_events":     sem.stats.totalEvents,
		"immediate_events": sem.stats.immediateEvents,
		"normal_events":    sem.stats.normalEvents,
		"batch_events":     sem.stats.batchEvents,
		"client_count":     sem.GetClientCount(),
		"channel_usage": map[string]interface{}{
			"immediate": map[string]interface{}{
				"capacity": cap(sem.immediate),
				"length":   len(sem.immediate),
				"usage":    math.Round(float64(len(sem.immediate))/float64(cap(sem.immediate))*100*100) / 100,
			},
			"normal": map[string]interface{}{
				"capacity": cap(sem.normal),
				"length":   len(sem.normal),
				"usage":    math.Round(float64(len(sem.normal))/float64(cap(sem.normal))*100*100) / 100,
			},
			"batch": map[string]interface{}{
				"capacity": cap(sem.batch),
				"length":   len(sem.batch),
				"usage":    math.Round(float64(len(sem.batch))/float64(cap(sem.batch))*100*100) / 100,
			},
		},
	}
}

// DeterminePriorityByData 根据数据内容智能判断优先级
func (sem *SmartEventManager) DeterminePriorityByData(eventType EventType, data interface{}) EventPriority {
	switch eventType {
	case EventTypeEndpoint:
		// 端点事件检查健康状态变化
		if dataMap, ok := data.(map[string]interface{}); ok {
			if healthChanged, exists := dataMap["health_changed"]; exists && healthChanged.(bool) {
				return HighPriority
			}
			// 检查响应时间异常
			if respTime, exists := dataMap["response_time_ms"]; exists {
				if rt, ok := respTime.(float64); ok && rt > 5000 {
					return NormalPriority
				}
			}
		}
		return LowPriority

	case EventTypeConnection:
		// 连接事件检查错误率
		if dataMap, ok := data.(map[string]interface{}); ok {
			if errorRate, exists := dataMap["error_rate"]; exists {
				if er, ok := errorRate.(float64); ok && er > 0.1 {
					return HighPriority
				}
			}
		}
		return NormalPriority

	case EventTypeGroup:
		// 组状态变化通常是高优先级
		return HighPriority

	case EventTypeSuspended:
		// 挂起请求事件是高优先级
		return HighPriority

	case EventTypeChart:
		// 图表数据是低优先级
		return LowPriority

	default:
		return NormalPriority
	}
}

// Stop 停止智能事件管理器
func (sem *SmartEventManager) Stop() {
	sem.logger.Info("⏹️ 正在停止智能事件管理器...")

	// 1. 停止处理协程
	sem.cancel()

	// 2. 等待一段时间让缓冲区处理完成
	time.Sleep(200 * time.Millisecond)

	// 3. 停止基础事件管理器
	sem.EventManager.Stop()

	sem.logger.Info("✅ 智能事件管理器已停止")
}

// broadcastCompleteEvent 广播完整事件信息
func (sem *SmartEventManager) broadcastCompleteEvent(event Event) {
	// 严格验证Event结构
	if event.Type == "" || event.Timestamp.IsZero() {
		sem.logger.Error("❌ broadcastCompleteEvent收到无效Event", "type", event.Type, "timestamp", event.Timestamp.Unix())
		return
	}
	
	// 检查EventManager是否已关闭
	if atomic.LoadInt64(&sem.EventManager.closed) != 0 {
		return
	}
	
	// 使用defer+recover防止panic
	defer func() {
		if r := recover(); r != nil {
			sem.logger.Debug("广播完整事件时检测到通道已关闭", "event_type", event.Type, "recover", r)
		}
	}()
	
	select {
	case sem.EventManager.broadcast <- event:
		// 事件发送成功
	default:
		// 广播通道已满，跳过此事件
		sem.logger.Warn("广播通道已满，跳过事件", "event_type", event.Type)
	}
}