package events

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// EventBus 接口
type EventBus interface {
	// 发布事件
	Publish(event Event)

	// 设置 SSE 推送器
	SetSSEBroadcaster(broadcaster SSEBroadcaster)

	// 启动和停止
	Start() error
	Stop() error

	// 获取统计信息
	GetStats() BusStats
}

// SSE 广播器接口
type SSEBroadcaster interface {
	BroadcastEvent(eventType string, data map[string]interface{})
	IsEventManagerActive() bool
}

// 事件过滤器
type EventFilter struct {
	// 是否推送给 SSE
	ShouldBroadcast func(event Event) bool

	// 数据转换器
	DataTransformer func(event Event) map[string]interface{}

	// 频率限制（防止过度推送）
	RateLimit time.Duration
}

// EventBus 实现
type eventBus struct {
	// 基础配置
	ctx    context.Context
	cancel context.CancelFunc
	logger *slog.Logger

	// 事件处理
	eventChan      chan Event
	sseBroadcaster SSEBroadcaster

	// 过滤和限制
	filters      map[EventType]EventFilter
	rateLimiters map[EventType]*rateLimiter

	// 统计信息
	stats   BusStats
	statsMu sync.RWMutex

	// 内部状态
	running bool
	wg      sync.WaitGroup
}

// 统计信息
type BusStats struct {
	TotalEvents      int64                    `json:"total_events"`
	ProcessedEvents  int64                    `json:"processed_events"`
	DroppedEvents    int64                    `json:"dropped_events"`
	EventsByType     map[EventType]int64      `json:"events_by_type"`
	EventsByPriority map[EventPriority]int64  `json:"events_by_priority"`
	StartTime        time.Time                `json:"start_time"`
}

// 频率限制器
type rateLimiter struct {
	lastTime time.Time
	limit    time.Duration
	mu       sync.Mutex
}

func (rl *rateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	if now.Sub(rl.lastTime) >= rl.limit {
		rl.lastTime = now
		return true
	}
	return false
}

// NewEventBus 创建新的EventBus实例
func NewEventBus(logger *slog.Logger) EventBus {
	ctx, cancel := context.WithCancel(context.Background())

	bus := &eventBus{
		ctx:          ctx,
		cancel:       cancel,
		logger:       logger,
		eventChan:    make(chan Event, 1000), // 缓冲区大小
		filters:      make(map[EventType]EventFilter),
		rateLimiters: make(map[EventType]*rateLimiter),
		stats: BusStats{
			EventsByType:     make(map[EventType]int64),
			EventsByPriority: make(map[EventPriority]int64),
			StartTime:        time.Now(),
		},
	}

	// 设置默认过滤器
	bus.setupDefaultFilters()

	return bus
}

// 设置默认过滤器
func (eb *eventBus) setupDefaultFilters() {
	// 请求事件过滤器 - 高频率但重要
	eb.filters[EventRequestStarted] = EventFilter{
		ShouldBroadcast: func(event Event) bool { return true },
		DataTransformer: func(event Event) map[string]interface{} { return event.Data },
		RateLimit:       100 * time.Millisecond, // 高频率允许
	}

	eb.filters[EventRequestUpdated] = EventFilter{
		ShouldBroadcast: func(event Event) bool { return true },
		DataTransformer: func(event Event) map[string]interface{} { return event.Data },
		RateLimit:       100 * time.Millisecond,
	}

	eb.filters[EventRequestCompleted] = EventFilter{
		ShouldBroadcast: func(event Event) bool { return true },
		DataTransformer: func(event Event) map[string]interface{} { return event.Data },
		RateLimit:       100 * time.Millisecond,
	}

	// 端点健康事件过滤器 - 关键事件，立即推送
	eb.filters[EventEndpointHealthy] = EventFilter{
		ShouldBroadcast: func(event Event) bool { return true },
		DataTransformer: func(event Event) map[string]interface{} { return event.Data },
		RateLimit:       0, // 无限制
	}

	eb.filters[EventEndpointUnhealthy] = EventFilter{
		ShouldBroadcast: func(event Event) bool { return true },
		DataTransformer: func(event Event) map[string]interface{} { return event.Data },
		RateLimit:       0, // 无限制
	}

	// 连接统计事件过滤器 - 低优先级，限制频率
	eb.filters[EventConnectionStats] = EventFilter{
		ShouldBroadcast: func(event Event) bool { return true },
		DataTransformer: func(event Event) map[string]interface{} { return event.Data },
		RateLimit:       5 * time.Second, // 5秒限制一次
	}

	eb.filters[EventResponseReceived] = EventFilter{
		ShouldBroadcast: func(event Event) bool { return true },
		DataTransformer: func(event Event) map[string]interface{} { return event.Data },
		RateLimit:       1 * time.Second, // 1秒限制一次
	}

	// 系统事件过滤器 - 重要系统事件
	eb.filters[EventSystemError] = EventFilter{
		ShouldBroadcast: func(event Event) bool { return true },
		DataTransformer: func(event Event) map[string]interface{} { return event.Data },
		RateLimit:       0, // 无限制
	}

	eb.filters[EventConfigChanged] = EventFilter{
		ShouldBroadcast: func(event Event) bool { return true },
		DataTransformer: func(event Event) map[string]interface{} { return event.Data },
		RateLimit:       0, // 无限制
	}

	// 组状态变化事件过滤器 - 适度限制，避免频繁更新
	eb.filters[EventGroupStatusChanged] = EventFilter{
		ShouldBroadcast: func(event Event) bool { return true },
		DataTransformer: func(event Event) map[string]interface{} { return event.Data },
		RateLimit:       100 * time.Millisecond, // 适度限制，避免频繁更新
	}

	// 组健康统计变化事件过滤器 - 高优先级，立即推送
	eb.filters[EventGroupHealthStatsChanged] = EventFilter{
		ShouldBroadcast: func(event Event) bool { return true },
		DataTransformer: func(event Event) map[string]interface{} { return event.Data },
		RateLimit:       0, // 暂时移除频率限制用于调试
	}

	// 初始化频率限制器
	for eventType, filter := range eb.filters {
		if filter.RateLimit > 0 {
			eb.rateLimiters[eventType] = &rateLimiter{
				limit: filter.RateLimit,
			}
		}
	}
}

// Publish 发布事件
func (eb *eventBus) Publish(event Event) {
	if !eb.running {
		eb.logger.Debug("EventBus not running, dropping event", "type", event.Type)
		return
	}

	// 设置时间戳
	event.Timestamp = time.Now()

	// 更新统计信息
	eb.updateStats(event, "total")

	select {
	case eb.eventChan <- event:
		// 事件发送成功
	default:
		// 缓冲区满，丢弃事件
		eb.updateStats(event, "dropped")
		eb.logger.Warn("EventBus buffer full, dropping event", "type", event.Type, "source", event.Source)
	}
}

// SetSSEBroadcaster 设置SSE广播器
func (eb *eventBus) SetSSEBroadcaster(broadcaster SSEBroadcaster) {
	eb.sseBroadcaster = broadcaster
}

// Start 启动EventBus
func (eb *eventBus) Start() error {
	if eb.running {
		return nil
	}

	eb.running = true
	eb.wg.Add(1)

	go eb.eventProcessor()

	eb.logger.Info("EventBus started")
	return nil
}

// Stop 停止EventBus
func (eb *eventBus) Stop() error {
	if !eb.running {
		return nil
	}

	eb.running = false
	eb.cancel()
	close(eb.eventChan)

	eb.wg.Wait()

	eb.logger.Info("EventBus stopped")
	return nil
}

// GetStats 获取统计信息
func (eb *eventBus) GetStats() BusStats {
	eb.statsMu.RLock()
	defer eb.statsMu.RUnlock()

	// 深拷贝统计信息
	stats := BusStats{
		TotalEvents:      eb.stats.TotalEvents,
		ProcessedEvents:  eb.stats.ProcessedEvents,
		DroppedEvents:    eb.stats.DroppedEvents,
		EventsByType:     make(map[EventType]int64),
		EventsByPriority: make(map[EventPriority]int64),
		StartTime:        eb.stats.StartTime,
	}

	for k, v := range eb.stats.EventsByType {
		stats.EventsByType[k] = v
	}
	for k, v := range eb.stats.EventsByPriority {
		stats.EventsByPriority[k] = v
	}

	return stats
}

// 事件处理器
func (eb *eventBus) eventProcessor() {
	defer eb.wg.Done()

	eb.logger.Debug("EventBus processor started")

	for {
		select {
		case event, ok := <-eb.eventChan:
			if !ok {
				eb.logger.Debug("EventBus processor stopped")
				return
			}

			eb.processEvent(event)

		case <-eb.ctx.Done():
			eb.logger.Debug("EventBus processor context cancelled")
			return
		}
	}
}

// 处理单个事件
func (eb *eventBus) processEvent(event Event) {
	// 更新处理统计
	eb.updateStats(event, "processed")

	// 获取事件过滤器
	filter, exists := eb.filters[event.Type]
	if !exists {
		eb.logger.Debug("No filter for event type", "type", event.Type)
		return
	}

	// 检查是否应该广播
	if !filter.ShouldBroadcast(event) {
		eb.logger.Debug("Event filtered out", "type", event.Type)
		return
	}

	// 检查频率限制
	if limiter, exists := eb.rateLimiters[event.Type]; exists {
		if !limiter.Allow() {
			eb.logger.Debug("Event rate limited", "type", event.Type)
			return
		}
	}

	// 检查SSE广播器是否可用
	if eb.sseBroadcaster == nil {
		eb.logger.Debug("No SSE broadcaster set")
		return
	}

	if !eb.sseBroadcaster.IsEventManagerActive() {
		eb.logger.Debug("SSE EventManager not active")
		return
	}

	// 转换数据并广播
	data := filter.DataTransformer(event)
	if frontendEventType, exists := EventTypeMapping[event.Type]; exists {
		eb.sseBroadcaster.BroadcastEvent(frontendEventType, data)
		eb.logger.Debug("Event broadcasted", "type", event.Type, "frontend_type", frontendEventType)
	} else {
		eb.logger.Warn("No frontend mapping for event type", "type", event.Type)
	}
}

// 更新统计信息
func (eb *eventBus) updateStats(event Event, statType string) {
	eb.statsMu.Lock()
	defer eb.statsMu.Unlock()

	switch statType {
	case "total":
		eb.stats.TotalEvents++
		eb.stats.EventsByType[event.Type]++
		eb.stats.EventsByPriority[event.Priority]++
	case "processed":
		eb.stats.ProcessedEvents++
	case "dropped":
		eb.stats.DroppedEvents++
	}
}