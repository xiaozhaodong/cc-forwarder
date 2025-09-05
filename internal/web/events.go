package web

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// EventType 定义事件类型
type EventType string

const (
	EventTypeStatus      EventType = "status"      // 服务状态更新
	EventTypeEndpoint    EventType = "endpoint"    // 端点状态变化
	EventTypeConnection  EventType = "connection"  // 连接统计更新
	EventTypeLog         EventType = "log"         // 日志事件
	EventTypeConfig      EventType = "config"      // 配置更新
	EventTypeChart       EventType = "chart"       // 图表数据更新
	EventTypeGroup       EventType = "group"       // 组状态变化
	EventTypeSuspended   EventType = "suspended"   // 挂起请求事件
)

// Event 表示一个SSE事件
type Event struct {
	Type      EventType   `json:"type"`
	Data      interface{} `json:"data"`
	Timestamp time.Time   `json:"timestamp"`
}

// Client 表示一个SSE客户端连接
type Client struct {
	ID       string
	Channel  chan Event
	LastPing time.Time
	Filter   map[EventType]bool // 事件过滤器，true表示订阅该类型事件
	mu       sync.RWMutex
}

// EventManager 管理SSE连接和事件广播
type EventManager struct {
	clients   map[string]*Client
	mu        sync.RWMutex
	logger    *slog.Logger
	ctx       context.Context
	cancel    context.CancelFunc
	broadcast chan Event
	closed    int64 // 原子标志，用于标记是否已关闭
}

// NewEventManager 创建新的事件管理器
func NewEventManager(logger *slog.Logger) *EventManager {
	ctx, cancel := context.WithCancel(context.Background())
	
	em := &EventManager{
		clients:   make(map[string]*Client),
		logger:    logger,
		ctx:       ctx,
		cancel:    cancel,
		broadcast: make(chan Event, 1000), // 缓冲通道，防止阻塞
	}
	
	// 启动广播协程
	go em.broadcastLoop()
	
	// 启动清理协程
	go em.cleanupLoop()
	
	return em
}

// AddClient 添加新的SSE客户端
func (em *EventManager) AddClient(clientID string, filter map[EventType]bool) *Client {
	em.mu.Lock()
	defer em.mu.Unlock()
	
	// 如果过滤器为空，默认订阅所有事件类型
	if filter == nil {
		filter = map[EventType]bool{
			EventTypeStatus:     true,
			EventTypeEndpoint:   true,
			EventTypeConnection: true,
			EventTypeLog:        true,
			EventTypeConfig:     false, // 默认不订阅配置事件
			EventTypeChart:      true,  // 默认订阅图表事件
			EventTypeGroup:      true,  // 默认订阅组事件
			EventTypeSuspended:  true,  // 默认订阅挂起请求事件
		}
	}
	
	client := &Client{
		ID:       clientID,
		Channel:  make(chan Event, 100),
		LastPing: time.Now(),
		Filter:   filter,
	}
	
	em.clients[clientID] = client
	em.logger.Debug("SSE客户端已连接", "client_id", clientID, "total_clients", len(em.clients))
	
	// 发送初始连接事件
	em.sendToClient(client, Event{
		Type: EventTypeStatus,
		Data: map[string]interface{}{
			"event":   "connected",
			"message": "SSE连接已建立",
		},
		Timestamp: time.Now(),
	})
	
	return client
}

// RemoveClient 移除SSE客户端
func (em *EventManager) RemoveClient(clientID string) {
	em.mu.Lock()
	defer em.mu.Unlock()
	
	if client, exists := em.clients[clientID]; exists {
		close(client.Channel)
		delete(em.clients, clientID)
		em.logger.Debug("SSE客户端已断开", "client_id", clientID, "total_clients", len(em.clients))
	}
}

// BroadcastEvent 广播事件到所有符合条件的客户端
func (em *EventManager) BroadcastEvent(eventType EventType, data interface{}) {
	// 检查EventManager是否已关闭
	if atomic.LoadInt64(&em.closed) != 0 {
		// EventManager已关闭，直接返回，不记录日志避免干扰
		return
	}
	
	event := Event{
		Type:      eventType,
		Data:      data,
		Timestamp: time.Now(),
	}
	
	// 使用defer+recover防止panic
	defer func() {
		if r := recover(); r != nil {
			// 通道已关闭，忽略此事件
			em.logger.Debug("广播事件时检测到通道已关闭", "event_type", eventType, "recover", r)
		}
	}()
	
	select {
	case em.broadcast <- event:
		// 事件发送成功
	default:
		// 广播通道已满，跳过此事件
		em.logger.Warn("广播通道已满，跳过事件", "event_type", eventType)
	}
}

// broadcastLoop 广播循环
func (em *EventManager) broadcastLoop() {
	for {
		select {
		case event := <-em.broadcast:
			em.mu.RLock()
			var targetClients []*Client
			for _, client := range em.clients {
				// 检查客户端是否订阅了此类型的事件
				client.mu.RLock()
				if client.Filter[event.Type] {
					targetClients = append(targetClients, client)
				}
				client.mu.RUnlock()
			}
			em.mu.RUnlock()
			
			// 并发发送给所有目标客户端
			for _, client := range targetClients {
				go em.sendToClient(client, event)
			}
			
		case <-em.ctx.Done():
			return
		}
	}
}

// sendToClient 向特定客户端发送事件
func (em *EventManager) sendToClient(client *Client, event Event) {
	select {
	case client.Channel <- event:
		// 发送成功
	case <-time.After(1 * time.Second):
		// 发送超时，客户端可能已断开
		em.logger.Debug("向客户端发送事件超时", "client_id", client.ID, "event_type", event.Type)
		em.RemoveClient(client.ID)
	}
}

// UpdateClientPing 更新客户端最后活动时间
func (em *EventManager) UpdateClientPing(clientID string) {
	em.mu.RLock()
	defer em.mu.RUnlock()
	
	if client, exists := em.clients[clientID]; exists {
		client.LastPing = time.Now()
	}
}

// GetClientCount 获取当前客户端数量
func (em *EventManager) GetClientCount() int {
	em.mu.RLock()
	defer em.mu.RUnlock()
	return len(em.clients)
}

// cleanupLoop 清理循环，移除不活跃的客户端
func (em *EventManager) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			em.cleanupInactiveClients()
		case <-em.ctx.Done():
			return
		}
	}
}

// cleanupInactiveClients 清理不活跃的客户端
func (em *EventManager) cleanupInactiveClients() {
	em.mu.Lock()
	defer em.mu.Unlock()
	
	timeout := 2 * time.Minute
	now := time.Now()
	
	var toRemove []string
	for clientID, client := range em.clients {
		if now.Sub(client.LastPing) > timeout {
			toRemove = append(toRemove, clientID)
		}
	}
	
	for _, clientID := range toRemove {
		if client, exists := em.clients[clientID]; exists {
			close(client.Channel)
			delete(em.clients, clientID)
			em.logger.Debug("清理不活跃的SSE客户端", "client_id", clientID)
		}
	}
	
	if len(toRemove) > 0 {
		em.logger.Debug("清理完成", "removed_clients", len(toRemove), "active_clients", len(em.clients))
	}
}

// Stop 停止事件管理器
func (em *EventManager) Stop() {
	// 原子性标记为已关闭
	if !atomic.CompareAndSwapInt64(&em.closed, 0, 1) {
		// 已经关闭过了，直接返回
		return
	}
	
	em.logger.Info("⏹️ 正在停止SSE事件管理器...")
	
	// 1. 取消上下文，停止所有goroutines
	em.cancel()
	
	// 2. 短暂等待，让正在处理的事件完成
	time.Sleep(100 * time.Millisecond)
	
	em.mu.Lock()
	defer em.mu.Unlock()
	
	// 3. 关闭所有客户端连接
	for clientID, client := range em.clients {
		close(client.Channel)
		delete(em.clients, clientID)
	}
	
	// 4. 安全关闭广播通道
	defer func() {
		if r := recover(); r != nil {
			em.logger.Debug("关闭广播通道时检测到已关闭", "recover", r)
		}
	}()
	close(em.broadcast)
	
	em.logger.Info("✅ SSE事件管理器已停止")
}

// formatEventData 格式化事件数据为SSE格式
func (em *EventManager) formatEventData(event Event) (string, error) {
	data, err := json.Marshal(event.Data)
	if err != nil {
		return "", err
	}
	
	return fmt.Sprintf("event: %s\ndata: %s\nid: %d\n\n", 
		event.Type, 
		string(data), 
		event.Timestamp.Unix()), nil
}