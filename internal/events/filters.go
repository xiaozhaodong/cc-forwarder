package events

import (
	"log/slog"
	"sync"
	"time"
)

// FilterManager 管理事件过滤器
type FilterManager struct {
	filters      map[EventType]EventFilter
	rateLimiters map[EventType]*rateLimiter
	mu           sync.RWMutex
	logger       *slog.Logger
}

// NewFilterManager 创建新的过滤器管理器
func NewFilterManager(logger *slog.Logger) *FilterManager {
	fm := &FilterManager{
		filters:      make(map[EventType]EventFilter),
		rateLimiters: make(map[EventType]*rateLimiter),
		logger:       logger,
	}

	// 设置默认过滤器
	fm.setupDefaultFilters()

	return fm
}

// setupDefaultFilters 设置默认过滤器
func (fm *FilterManager) setupDefaultFilters() {
	// 请求生命周期事件过滤器
	requestFilter := EventFilter{
		ShouldBroadcast: func(event Event) bool {
			// 过滤掉一些不重要的请求状态
			if changeType, ok := event.Data["change_type"].(string); ok {
				switch changeType {
				case "minor_update", "heartbeat":
					return false
				}
			}
			return true
		},
		DataTransformer: func(event Event) map[string]interface{} {
			// 清理和格式化请求数据
			data := make(map[string]interface{})
			for k, v := range event.Data {
				// 过滤掉敏感或不必要的字段
				switch k {
				case "client_ip", "user_agent":
					if fm.logger != nil {
						// 在日志中记录但不发送到前端
						continue
					}
				default:
					data[k] = v
				}
			}
			return data
		},
		RateLimit: 100 * time.Millisecond,
	}

	fm.filters[EventRequestStarted] = requestFilter
	fm.filters[EventRequestUpdated] = requestFilter
	fm.filters[EventRequestCompleted] = requestFilter

	// 端点健康事件过滤器 - 关键事件，无限制
	endpointFilter := EventFilter{
		ShouldBroadcast: func(event Event) bool { return true },
		DataTransformer: func(event Event) map[string]interface{} {
			// 添加端点状态摘要
			data := make(map[string]interface{})
			for k, v := range event.Data {
				data[k] = v
			}
			// 添加健康状态汇总
			if healthy, ok := event.Data["healthy"].(bool); ok {
				if healthy {
					data["status_summary"] = "健康"
				} else {
					data["status_summary"] = "异常"
				}
			}
			return data
		},
		RateLimit: 0, // 无限制
	}

	fm.filters[EventEndpointHealthy] = endpointFilter
	fm.filters[EventEndpointUnhealthy] = endpointFilter

	// 连接统计事件过滤器 - 低优先级，控制频率
	connectionFilter := EventFilter{
		ShouldBroadcast: func(event Event) bool {
			// 只在有显著变化时才广播
			if changeType, ok := event.Data["change_type"].(string); ok {
				switch changeType {
				case "system_stats_updated":
					return true
				case "minor_stats_change":
					return false
				}
			}
			return true
		},
		DataTransformer: func(event Event) map[string]interface{} {
			// 简化统计数据，只发送关键指标
			data := make(map[string]interface{})
			keyMetrics := []string{
				"total_requests", "active_connections", "successful_requests",
				"failed_requests", "average_response_time", "suspended_success_rate",
			}
			
			for _, key := range keyMetrics {
				if value, exists := event.Data[key]; exists {
					data[key] = value
				}
			}
			
			data["change_type"] = event.Data["change_type"]
			return data
		},
		RateLimit: 5 * time.Second, // 5秒限制一次
	}

	fm.filters[EventConnectionStats] = connectionFilter

	// 响应接收事件过滤器
	responseFilter := EventFilter{
		ShouldBroadcast: func(event Event) bool {
			// 过滤掉正常的响应，只关注异常情况
			if statusCode, ok := event.Data["status_code"].(int); ok {
				if statusCode >= 200 && statusCode < 300 {
					return false // 正常响应不广播
				}
			}
			return true
		},
		DataTransformer: func(event Event) map[string]interface{} {
			return event.Data
		},
		RateLimit: 1 * time.Second,
	}

	fm.filters[EventResponseReceived] = responseFilter

	// 系统事件过滤器 - 重要事件，立即处理
	systemFilter := EventFilter{
		ShouldBroadcast: func(event Event) bool { return true },
		DataTransformer: func(event Event) map[string]interface{} {
			data := make(map[string]interface{})
			for k, v := range event.Data {
				data[k] = v
			}
			// 添加系统事件标记
			data["is_system_event"] = true
			return data
		},
		RateLimit: 0, // 无限制
	}

	fm.filters[EventSystemError] = systemFilter
	fm.filters[EventConfigChanged] = systemFilter

	// 初始化频率限制器
	for eventType, filter := range fm.filters {
		if filter.RateLimit > 0 {
			fm.rateLimiters[eventType] = &rateLimiter{
				limit: filter.RateLimit,
			}
		}
	}
}

// GetFilter 获取指定事件类型的过滤器
func (fm *FilterManager) GetFilter(eventType EventType) (EventFilter, bool) {
	fm.mu.RLock()
	defer fm.mu.RUnlock()
	
	filter, exists := fm.filters[eventType]
	return filter, exists
}

// GetRateLimiter 获取指定事件类型的频率限制器
func (fm *FilterManager) GetRateLimiter(eventType EventType) (*rateLimiter, bool) {
	fm.mu.RLock()
	defer fm.mu.RUnlock()
	
	limiter, exists := fm.rateLimiters[eventType]
	return limiter, exists
}

// SetCustomFilter 设置自定义过滤器
func (fm *FilterManager) SetCustomFilter(eventType EventType, filter EventFilter) {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	
	fm.filters[eventType] = filter
	
	// 更新频率限制器
	if filter.RateLimit > 0 {
		fm.rateLimiters[eventType] = &rateLimiter{
			limit: filter.RateLimit,
		}
	} else {
		// 移除频率限制器
		delete(fm.rateLimiters, eventType)
	}
	
	fm.logger.Info("Custom filter set", "event_type", eventType)
}

// RemoveFilter 移除过滤器
func (fm *FilterManager) RemoveFilter(eventType EventType) {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	
	delete(fm.filters, eventType)
	delete(fm.rateLimiters, eventType)
	
	fm.logger.Info("Filter removed", "event_type", eventType)
}

// GetFilterStats 获取过滤器统计信息
func (fm *FilterManager) GetFilterStats() map[EventType]FilterStats {
	fm.mu.RLock()
	defer fm.mu.RUnlock()
	
	stats := make(map[EventType]FilterStats)
	for eventType, limiter := range fm.rateLimiters {
		limiter.mu.Lock()
		stats[eventType] = FilterStats{
			EventType:    eventType,
			HasRateLimit: limiter.limit > 0,
			RateLimit:    limiter.limit,
			LastAllowed:  limiter.lastTime,
		}
		limiter.mu.Unlock()
	}
	
	return stats
}

// FilterStats 过滤器统计信息
type FilterStats struct {
	EventType    EventType     `json:"event_type"`
	HasRateLimit bool          `json:"has_rate_limit"`
	RateLimit    time.Duration `json:"rate_limit"`
	LastAllowed  time.Time     `json:"last_allowed"`
}