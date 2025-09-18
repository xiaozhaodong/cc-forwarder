package monitor

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// TokenUsage represents token usage statistics
type TokenUsage struct {
	InputTokens            int64
	OutputTokens           int64
	CacheCreationTokens    int64
	CacheReadTokens        int64
}

// Metrics contains all monitoring metrics
type Metrics struct {
	mu sync.RWMutex

	// Request metrics
	TotalRequests     int64
	SuccessfulRequests int64
	FailedRequests    int64

	// Suspended request metrics
	SuspendedRequests          int64  // Current number of suspended requests
	TotalSuspendedRequests     int64  // Total historical suspended requests
	SuccessfulSuspendedRequests int64  // Successfully resumed suspended requests
	TimeoutSuspendedRequests   int64  // Timed out suspended requests
	TotalSuspendedTime         time.Duration // Total time spent in suspension
	MinSuspendedTime           time.Duration // Minimum suspension time
	MaxSuspendedTime           time.Duration // Maximum suspension time

	// Token usage metrics
	TotalTokenUsage   TokenUsage

	// Failed request token statistics
	FailedRequestTokens     int64                 // Total token count for failed requests
	FailedTokensByReason    map[string]int64      // Token statistics by failure reason
	FailedTokensByEndpoint  map[string]int64      // Failed token statistics by endpoint
	
	// Response time metrics
	ResponseTimes     []time.Duration
	TotalResponseTime time.Duration
	MinResponseTime   time.Duration
	MaxResponseTime   time.Duration
	
	// Endpoint metrics
	EndpointStats map[string]*EndpointMetrics
	
	// Connection metrics  
	ActiveConnections map[string]*ConnectionInfo
	ConnectionHistory []*ConnectionInfo
	
	// System metrics
	StartTime time.Time
	
	// Historical data (circular buffer)
	RequestHistory              []RequestDataPoint
	ResponseHistory             []ResponseTimePoint
	TokenHistory                []TokenHistoryPoint
	SuspendedRequestHistory     []SuspendedRequestHistoryPoint
	MaxHistoryPoints            int
}

// EndpointMetrics tracks metrics for a specific endpoint
type EndpointMetrics struct {
	Name             string
	URL              string
	TotalRequests    int64
	SuccessfulRequests int64
	FailedRequests   int64
	TotalResponseTime time.Duration
	MinResponseTime  time.Duration
	MaxResponseTime  time.Duration
	LastUsed         time.Time
	RetryCount       int64
	Priority         int
	Healthy          bool
	TokenUsage       TokenUsage
}

// ConnectionInfo represents an active connection
type ConnectionInfo struct {
	ID             string
	ClientIP       string
	UserAgent      string
	StartTime      time.Time
	LastActivity   time.Time
	Method         string
	Path           string
	Endpoint       string
	Port           string
	RetryCount     int
	Status         string // "active", "completed", "failed", "timeout", "suspended", "resumed"
	BytesReceived  int64
	BytesSent      int64
	IsStreaming    bool
	TokenUsage     TokenUsage  // Token usage for this connection
	
	// Suspended request related fields
	IsSuspended    bool      // Whether the connection is currently suspended
	SuspendedAt    time.Time // When the request was suspended
	ResumedAt      time.Time // When the request was resumed
	SuspendedTime  time.Duration // Total time spent suspended
}

// RequestDataPoint represents a point in time for request metrics
type RequestDataPoint struct {
	Timestamp  time.Time
	Total      int64
	Successful int64
	Failed     int64
}

// ResponseTimePoint represents response time at a point in time
type ResponseTimePoint struct {
	Timestamp    time.Time
	AverageTime  time.Duration
	MinTime      time.Duration
	MaxTime      time.Duration
}

// TokenHistoryPoint represents token usage at a point in time
type TokenHistoryPoint struct {
	Timestamp           time.Time
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
	TotalTokens         int64
}

// SuspendedRequestHistoryPoint represents suspended request metrics at a point in time
type SuspendedRequestHistoryPoint struct {
	Timestamp                   time.Time
	SuspendedRequests          int64  // Current suspended requests at this point
	TotalSuspendedRequests     int64  // Total historical suspended requests
	SuccessfulSuspendedRequests int64  // Successfully resumed
	TimeoutSuspendedRequests   int64  // Timed out
	AverageSuspendedTime       time.Duration // Average suspension time
}

// NewMetrics creates a new metrics instance
func NewMetrics() *Metrics {
	return &Metrics{
		EndpointStats:               make(map[string]*EndpointMetrics),
		ActiveConnections:           make(map[string]*ConnectionInfo),
		ConnectionHistory:           make([]*ConnectionInfo, 0),
		StartTime:                   time.Now(),
		RequestHistory:              make([]RequestDataPoint, 0),
		ResponseHistory:             make([]ResponseTimePoint, 0),
		TokenHistory:                make([]TokenHistoryPoint, 0),
		SuspendedRequestHistory:     make([]SuspendedRequestHistoryPoint, 0),
		MaxHistoryPoints:            300, // 5 minutes of data at 1-second intervals
		MinResponseTime:             time.Duration(0),
		MaxResponseTime:             time.Duration(0),
		MinSuspendedTime:            time.Duration(0),
		MaxSuspendedTime:            time.Duration(0),
		FailedTokensByReason:        make(map[string]int64),
		FailedTokensByEndpoint:      make(map[string]int64),
	}
}

// RecordRequest records a new request
func (m *Metrics) RecordRequest(endpoint, clientIP, userAgent, method, path string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.TotalRequests++
	
	// Update endpoint stats
	if m.EndpointStats[endpoint] == nil {
		m.EndpointStats[endpoint] = &EndpointMetrics{
			Name:            endpoint,
			MinResponseTime: time.Duration(0),
			MaxResponseTime: time.Duration(0),
		}
	}
	m.EndpointStats[endpoint].TotalRequests++
	m.EndpointStats[endpoint].LastUsed = time.Now()

	// Generate connection ID
	connID := generateConnectionID()
	
	// Create connection info
	conn := &ConnectionInfo{
		ID:           connID,
		ClientIP:     clientIP,
		UserAgent:    userAgent,
		StartTime:    time.Now(),
		LastActivity: time.Now(),
		Method:       method,
		Path:         path,
		Endpoint:     endpoint,
		Status:       "active",
		RetryCount:   0,
		BytesReceived: 0,
		BytesSent:    0,
	}
	
	m.ActiveConnections[connID] = conn
	
	return connID
}

// RecordResponse records a response
func (m *Metrics) RecordResponse(connID string, statusCode int, responseTime time.Duration, bytesSent int64, endpoint string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Update overall metrics
	m.TotalResponseTime += responseTime
	m.ResponseTimes = append(m.ResponseTimes, responseTime)
	
	// Update min/max response times
	if m.MinResponseTime == 0 || responseTime < m.MinResponseTime {
		m.MinResponseTime = responseTime
	}
	if responseTime > m.MaxResponseTime {
		m.MaxResponseTime = responseTime
	}

	// Track success/failure
	if statusCode >= 200 && statusCode < 400 {
		m.SuccessfulRequests++
		// Ensure endpoint stats exist
		if m.EndpointStats[endpoint] == nil && endpoint != "unknown" {
			m.EndpointStats[endpoint] = &EndpointMetrics{
				Name:            endpoint,
				MinResponseTime: time.Duration(0),
				MaxResponseTime: time.Duration(0),
			}
		}
		if endpoint != "unknown" && m.EndpointStats[endpoint] != nil {
			m.EndpointStats[endpoint].SuccessfulRequests++
			m.EndpointStats[endpoint].TotalRequests++
		}
	} else {
		m.FailedRequests++
		// Ensure endpoint stats exist
		if m.EndpointStats[endpoint] == nil && endpoint != "unknown" {
			m.EndpointStats[endpoint] = &EndpointMetrics{
				Name:            endpoint,
				MinResponseTime: time.Duration(0),
				MaxResponseTime: time.Duration(0),
			}
		}
		if endpoint != "unknown" && m.EndpointStats[endpoint] != nil {
			m.EndpointStats[endpoint].FailedRequests++
			m.EndpointStats[endpoint].TotalRequests++
		}
	}

	// Update endpoint metrics
	if endpoint != "unknown" && m.EndpointStats[endpoint] != nil {
		endpointMetrics := m.EndpointStats[endpoint]
		endpointMetrics.TotalResponseTime += responseTime
		endpointMetrics.LastUsed = time.Now()
		if endpointMetrics.MinResponseTime == 0 || responseTime < endpointMetrics.MinResponseTime {
			endpointMetrics.MinResponseTime = responseTime
		}
		if responseTime > endpointMetrics.MaxResponseTime {
			endpointMetrics.MaxResponseTime = responseTime
		}
	}

	// Update connection
	if conn, exists := m.ActiveConnections[connID]; exists {
		conn.LastActivity = time.Now()
		conn.BytesSent = bytesSent
		
		if statusCode >= 200 && statusCode < 400 {
			conn.Status = "completed"
		} else {
			conn.Status = "failed"
		}

		// Move to history and remove from active
		m.ConnectionHistory = append(m.ConnectionHistory, conn)
		delete(m.ActiveConnections, connID)
		
		// Limit history size
		if len(m.ConnectionHistory) > 1000 {
			m.ConnectionHistory = m.ConnectionHistory[len(m.ConnectionHistory)-1000:]
		}
	}

	// Limit response times history
	if len(m.ResponseTimes) > 1000 {
		m.ResponseTimes = m.ResponseTimes[len(m.ResponseTimes)-1000:]
	}
}

// RecordRetry records a retry attempt
func (m *Metrics) RecordRetry(connID string, endpoint string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if conn, exists := m.ActiveConnections[connID]; exists {
		conn.RetryCount++
		conn.LastActivity = time.Now()
		// Debug log to verify retry recording
		fmt.Printf("DEBUG: Recorded retry %d for connection %s on endpoint %s\n", conn.RetryCount, connID, endpoint)
	} else {
		fmt.Printf("DEBUG: Connection %s not found for retry recording\n", connID)
	}

	if endpointMetrics := m.EndpointStats[endpoint]; endpointMetrics != nil {
		endpointMetrics.RetryCount++
	}
}

// UpdateEndpointHealth updates endpoint health status
func (m *Metrics) UpdateEndpointHealth(endpoint, url string, healthy bool, priority int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.EndpointStats[endpoint] == nil {
		m.EndpointStats[endpoint] = &EndpointMetrics{
			Name:            endpoint,
			URL:             url,
			Priority:        priority,
			MinResponseTime: time.Duration(0),
			MaxResponseTime: time.Duration(0),
		}
	}
	
	m.EndpointStats[endpoint].Healthy = healthy
	m.EndpointStats[endpoint].URL = url
	m.EndpointStats[endpoint].Priority = priority
}

// UpdateConnectionEndpoint updates the endpoint name for an active connection
func (m *Metrics) UpdateConnectionEndpoint(connID, endpoint string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if conn, exists := m.ActiveConnections[connID]; exists {
		conn.Endpoint = endpoint
		conn.LastActivity = time.Now()
	}
}

// MarkStreamingConnection marks a connection as streaming
func (m *Metrics) MarkStreamingConnection(connID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if conn, exists := m.ActiveConnections[connID]; exists {
		conn.IsStreaming = true
		conn.LastActivity = time.Now()
	}
}

// GetMetrics returns a snapshot of current metrics
func (m *Metrics) GetMetrics() *Metrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Create a copy of metrics
	snapshot := &Metrics{
		TotalRequests:                  m.TotalRequests,
		SuccessfulRequests:             m.SuccessfulRequests,
		FailedRequests:                 m.FailedRequests,
		SuspendedRequests:              m.SuspendedRequests,
		TotalSuspendedRequests:         m.TotalSuspendedRequests,
		SuccessfulSuspendedRequests:    m.SuccessfulSuspendedRequests,
		TimeoutSuspendedRequests:       m.TimeoutSuspendedRequests,
		TotalSuspendedTime:             m.TotalSuspendedTime,
		MinSuspendedTime:               m.MinSuspendedTime,
		MaxSuspendedTime:               m.MaxSuspendedTime,
		TotalTokenUsage:                m.TotalTokenUsage,
		FailedRequestTokens:            m.FailedRequestTokens,
		FailedTokensByReason:           make(map[string]int64),
		FailedTokensByEndpoint:         make(map[string]int64),
		TotalResponseTime:              m.TotalResponseTime,
		MinResponseTime:                m.MinResponseTime,
		MaxResponseTime:                m.MaxResponseTime,
		StartTime:                      m.StartTime,
		EndpointStats:                  make(map[string]*EndpointMetrics),
		ActiveConnections:              make(map[string]*ConnectionInfo),
		ConnectionHistory:              make([]*ConnectionInfo, len(m.ConnectionHistory)),
	}

	// Copy endpoint stats
	for k, v := range m.EndpointStats {
		snapshot.EndpointStats[k] = &EndpointMetrics{
			Name:               v.Name,
			URL:                v.URL,
			TotalRequests:      v.TotalRequests,
			SuccessfulRequests: v.SuccessfulRequests,
			FailedRequests:     v.FailedRequests,
			TotalResponseTime:  v.TotalResponseTime,
			MinResponseTime:    v.MinResponseTime,
			MaxResponseTime:    v.MaxResponseTime,
			LastUsed:           v.LastUsed,
			RetryCount:         v.RetryCount,
			Priority:           v.Priority,
			Healthy:            v.Healthy,
			TokenUsage:         v.TokenUsage,
		}
	}

	// Copy active connections
	for k, v := range m.ActiveConnections {
		snapshot.ActiveConnections[k] = &ConnectionInfo{
			ID:            v.ID,
			ClientIP:      v.ClientIP,
			UserAgent:     v.UserAgent,
			StartTime:     v.StartTime,
			LastActivity:  v.LastActivity,
			Method:        v.Method,
			Path:          v.Path,
			Endpoint:      v.Endpoint,
			Port:          v.Port,
			RetryCount:    v.RetryCount,
			Status:        v.Status,
			BytesReceived: v.BytesReceived,
			BytesSent:     v.BytesSent,
			IsStreaming:   v.IsStreaming,
			TokenUsage:    v.TokenUsage,
			IsSuspended:   v.IsSuspended,
			SuspendedAt:   v.SuspendedAt,
			ResumedAt:     v.ResumedAt,
			SuspendedTime: v.SuspendedTime,
		}
	}

	// Copy connection history
	for i, v := range m.ConnectionHistory {
		snapshot.ConnectionHistory[i] = &ConnectionInfo{
			ID:            v.ID,
			ClientIP:      v.ClientIP,
			UserAgent:     v.UserAgent,
			StartTime:     v.StartTime,
			LastActivity:  v.LastActivity,
			Method:        v.Method,
			Path:          v.Path,
			Endpoint:      v.Endpoint,
			Port:          v.Port,
			RetryCount:    v.RetryCount,
			Status:        v.Status,
			BytesReceived: v.BytesReceived,
			BytesSent:     v.BytesSent,
			IsStreaming:   v.IsStreaming,
			TokenUsage:    v.TokenUsage,
			IsSuspended:   v.IsSuspended,
			SuspendedAt:   v.SuspendedAt,
			ResumedAt:     v.ResumedAt,
			SuspendedTime: v.SuspendedTime,
		}
	}

	// Copy failed tokens maps
	for k, v := range m.FailedTokensByReason {
		snapshot.FailedTokensByReason[k] = v
	}
	for k, v := range m.FailedTokensByEndpoint {
		snapshot.FailedTokensByEndpoint[k] = v
	}

	// Copy response times (last 100)
	if len(m.ResponseTimes) > 0 {
		start := 0
		if len(m.ResponseTimes) > 100 {
			start = len(m.ResponseTimes) - 100
		}
		snapshot.ResponseTimes = make([]time.Duration, len(m.ResponseTimes[start:]))
		copy(snapshot.ResponseTimes, m.ResponseTimes[start:])
	}

	return snapshot
}

// GetAverageResponseTime calculates average response time
func (m *Metrics) GetAverageResponseTime() time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.TotalRequests == 0 {
		return 0
	}
	return m.TotalResponseTime / time.Duration(m.TotalRequests)
}

// GetSuccessRate calculates success rate as percentage
func (m *Metrics) GetSuccessRate() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.TotalRequests == 0 {
		return 0
	}
	return float64(m.SuccessfulRequests) / float64(m.TotalRequests) * 100
}

// GetP95ResponseTime calculates 95th percentile response time
func (m *Metrics) GetP95ResponseTime() time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.ResponseTimes) == 0 {
		return 0
	}

	// Simple approximation for P95
	index := int(float64(len(m.ResponseTimes)) * 0.95)
	if index >= len(m.ResponseTimes) {
		index = len(m.ResponseTimes) - 1
	}
	
	// For a proper implementation, we'd sort the slice
	// For now, return max as approximation
	return m.MaxResponseTime
}

// RecordTokenUsage records token usage for a specific request
func (m *Metrics) RecordTokenUsage(connID string, endpoint string, tokens *TokenUsage) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 安全处理 nil tokens
	if tokens == nil {
		return
	}

	// Update overall token metrics
	m.TotalTokenUsage.InputTokens += tokens.InputTokens
	m.TotalTokenUsage.OutputTokens += tokens.OutputTokens
	m.TotalTokenUsage.CacheCreationTokens += tokens.CacheCreationTokens
	m.TotalTokenUsage.CacheReadTokens += tokens.CacheReadTokens

	// Update endpoint-specific token metrics
	if endpoint != "unknown" && m.EndpointStats[endpoint] != nil {
		m.EndpointStats[endpoint].TokenUsage.InputTokens += tokens.InputTokens
		m.EndpointStats[endpoint].TokenUsage.OutputTokens += tokens.OutputTokens
		m.EndpointStats[endpoint].TokenUsage.CacheCreationTokens += tokens.CacheCreationTokens
		m.EndpointStats[endpoint].TokenUsage.CacheReadTokens += tokens.CacheReadTokens
	}

	// Update connection info if available
	if conn, exists := m.ActiveConnections[connID]; exists {
		// Update token usage for this connection
		conn.TokenUsage.InputTokens += tokens.InputTokens
		conn.TokenUsage.OutputTokens += tokens.OutputTokens
		conn.TokenUsage.CacheCreationTokens += tokens.CacheCreationTokens
		conn.TokenUsage.CacheReadTokens += tokens.CacheReadTokens
		conn.LastActivity = time.Now()
	}
	
	// Note: Token history points are now added by AddHistoryDataPoints() method
	// This avoids duplicate history entries and provides better data sampling
}

// RecordFailedRequestTokenUsage 记录失败请求Token使用指标
func (m *Metrics) RecordFailedRequestTokenUsage(connID, endpoint string, tokens *TokenUsage, failureReason string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 安全处理 nil tokens
	var inputTokens, outputTokens, cacheCreationTokens, cacheReadTokens int64
	if tokens != nil {
		inputTokens = tokens.InputTokens
		outputTokens = tokens.OutputTokens
		cacheCreationTokens = tokens.CacheCreationTokens
		cacheReadTokens = tokens.CacheReadTokens
	}

	// 计算总Token数
	totalTokens := inputTokens + outputTokens + cacheCreationTokens + cacheReadTokens

	// 记录失败请求Token统计
	m.FailedRequestTokens += totalTokens

	// 按失败原因分类统计
	if m.FailedTokensByReason == nil {
		m.FailedTokensByReason = make(map[string]int64)
	}
	m.FailedTokensByReason[failureReason] += totalTokens

	// 按端点分类统计失败Token
	if m.FailedTokensByEndpoint == nil {
		m.FailedTokensByEndpoint = make(map[string]int64)
	}
	m.FailedTokensByEndpoint[endpoint] += totalTokens

	// 注意：不再更新连接的Token使用情况
	// 这避免了与RecordTokenUsage的重复计算
	// 连接的Token统计应该由RecordTokenUsage负责
}

// GetTotalTokenStats returns total token usage statistics
func (m *Metrics) GetTotalTokenStats() TokenUsage {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.TotalTokenUsage
}

// GetTokenHistory returns the token usage history
func (m *Metrics) GetTokenHistory() []TokenHistoryPoint {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy of the token history
	history := make([]TokenHistoryPoint, len(m.TokenHistory))
	copy(history, m.TokenHistory)
	return history
}

// AddHistoryDataPoints 定期收集历史数据点
func (m *Metrics) AddHistoryDataPoints() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()

	// 添加请求历史数据点
	requestPoint := RequestDataPoint{
		Timestamp:  now,
		Total:      m.TotalRequests,
		Successful: m.SuccessfulRequests,
		Failed:     m.FailedRequests,
	}
	m.RequestHistory = append(m.RequestHistory, requestPoint)

	// 添加响应时间历史数据点
	responsePoint := ResponseTimePoint{
		Timestamp:   now,
		AverageTime: m.GetAverageResponseTimeUnlocked(),
		MinTime:     m.MinResponseTime,
		MaxTime:     m.MaxResponseTime,
	}
	m.ResponseHistory = append(m.ResponseHistory, responsePoint)

	// 添加Token使用历史数据点 (定期快照，而不是在每次Token使用时添加)
	tokenPoint := TokenHistoryPoint{
		Timestamp:           now,
		InputTokens:         m.TotalTokenUsage.InputTokens,
		OutputTokens:        m.TotalTokenUsage.OutputTokens,
		CacheCreationTokens: m.TotalTokenUsage.CacheCreationTokens,
		CacheReadTokens:     m.TotalTokenUsage.CacheReadTokens,
		TotalTokens:         m.TotalTokenUsage.InputTokens + m.TotalTokenUsage.OutputTokens + m.TotalTokenUsage.CacheCreationTokens + m.TotalTokenUsage.CacheReadTokens,
	}
	
	// 只有当Token数据有变化时才添加新点，避免重复数据
	if len(m.TokenHistory) == 0 || 
		m.TokenHistory[len(m.TokenHistory)-1].InputTokens != tokenPoint.InputTokens ||
		m.TokenHistory[len(m.TokenHistory)-1].OutputTokens != tokenPoint.OutputTokens ||
		m.TokenHistory[len(m.TokenHistory)-1].CacheCreationTokens != tokenPoint.CacheCreationTokens ||
		m.TokenHistory[len(m.TokenHistory)-1].CacheReadTokens != tokenPoint.CacheReadTokens {
		m.TokenHistory = append(m.TokenHistory, tokenPoint)
	}

	// 添加挂起请求历史数据点
	suspendedPoint := SuspendedRequestHistoryPoint{
		Timestamp:                   now,
		SuspendedRequests:          m.SuspendedRequests,
		TotalSuspendedRequests:     m.TotalSuspendedRequests,
		SuccessfulSuspendedRequests: m.SuccessfulSuspendedRequests,
		TimeoutSuspendedRequests:   m.TimeoutSuspendedRequests,
		AverageSuspendedTime:       m.GetAverageSuspendedTimeUnlocked(),
	}
	
	// 只有当挂起请求数据有变化时才添加新点
	if len(m.SuspendedRequestHistory) == 0 ||
		m.SuspendedRequestHistory[len(m.SuspendedRequestHistory)-1].SuspendedRequests != suspendedPoint.SuspendedRequests ||
		m.SuspendedRequestHistory[len(m.SuspendedRequestHistory)-1].TotalSuspendedRequests != suspendedPoint.TotalSuspendedRequests ||
		m.SuspendedRequestHistory[len(m.SuspendedRequestHistory)-1].SuccessfulSuspendedRequests != suspendedPoint.SuccessfulSuspendedRequests ||
		m.SuspendedRequestHistory[len(m.SuspendedRequestHistory)-1].TimeoutSuspendedRequests != suspendedPoint.TimeoutSuspendedRequests {
		m.SuspendedRequestHistory = append(m.SuspendedRequestHistory, suspendedPoint)
	}

	// 限制历史数据大小
	if len(m.RequestHistory) > m.MaxHistoryPoints {
		m.RequestHistory = m.RequestHistory[len(m.RequestHistory)-m.MaxHistoryPoints:]
	}

	if len(m.ResponseHistory) > m.MaxHistoryPoints {
		m.ResponseHistory = m.ResponseHistory[len(m.ResponseHistory)-m.MaxHistoryPoints:]
	}

	if len(m.TokenHistory) > m.MaxHistoryPoints {
		m.TokenHistory = m.TokenHistory[len(m.TokenHistory)-m.MaxHistoryPoints:]
	}

	if len(m.SuspendedRequestHistory) > m.MaxHistoryPoints {
		m.SuspendedRequestHistory = m.SuspendedRequestHistory[len(m.SuspendedRequestHistory)-m.MaxHistoryPoints:]
	}
}

// GetAverageResponseTimeUnlocked 计算平均响应时间（无锁版本）
func (m *Metrics) GetAverageResponseTimeUnlocked() time.Duration {
	if m.TotalRequests == 0 {
		return 0
	}
	return m.TotalResponseTime / time.Duration(m.TotalRequests)
}

// GetChartDataForRequestHistory 获取请求历史图表数据
func (m *Metrics) GetChartDataForRequestHistory(minutes int) []RequestDataPoint {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	cutoff := now.Add(-time.Duration(minutes) * time.Minute)

	var result []RequestDataPoint
	for _, point := range m.RequestHistory {
		if point.Timestamp.After(cutoff) {
			result = append(result, point)
		}
	}

	return result
}

// GetChartDataForResponseTime 获取响应时间图表数据
func (m *Metrics) GetChartDataForResponseTime(minutes int) []ResponseTimePoint {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	cutoff := now.Add(-time.Duration(minutes) * time.Minute)

	var result []ResponseTimePoint
	for _, point := range m.ResponseHistory {
		if point.Timestamp.After(cutoff) {
			result = append(result, point)
		}
	}

	return result
}

// GetChartDataForTokenHistory 获取Token历史图表数据
func (m *Metrics) GetChartDataForTokenHistory(minutes int) []TokenHistoryPoint {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	cutoff := now.Add(-time.Duration(minutes) * time.Minute)

	var result []TokenHistoryPoint
	for _, point := range m.TokenHistory {
		if point.Timestamp.After(cutoff) {
			result = append(result, point)
		}
	}

	return result
}

// GetEndpointPerformanceData 获取端点性能统计数据
func (m *Metrics) GetEndpointPerformanceData() []map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []map[string]interface{}
	for _, endpoint := range m.EndpointStats {
		avgResponseTime := time.Duration(0)
		if endpoint.TotalRequests > 0 {
			avgResponseTime = endpoint.TotalResponseTime / time.Duration(endpoint.TotalRequests)
		}

		result = append(result, map[string]interface{}{
			"name":                 endpoint.Name,
			"url":                  endpoint.URL,
			"healthy":              endpoint.Healthy,
			"total_requests":       endpoint.TotalRequests,
			"successful_requests": endpoint.SuccessfulRequests,
			"failed_requests":     endpoint.FailedRequests,
			"success_rate":         m.calculateEndpointSuccessRate(endpoint),
			"avg_response_time":    avgResponseTime.Milliseconds(),
			"min_response_time":    endpoint.MinResponseTime.Milliseconds(),
			"max_response_time":    endpoint.MaxResponseTime.Milliseconds(),
			"priority":             endpoint.Priority,
			"retry_count":          endpoint.RetryCount,
			"last_used":            endpoint.LastUsed,
			"token_usage":          endpoint.TokenUsage,
		})
	}

	return result
}

// calculateEndpointSuccessRate 计算端点成功率
func (m *Metrics) calculateEndpointSuccessRate(endpoint *EndpointMetrics) float64 {
	if endpoint.TotalRequests == 0 {
		return 0
	}
	return float64(endpoint.SuccessfulRequests) / float64(endpoint.TotalRequests) * 100
}

// GetConnectionActivityData 获取连接活动数据
func (m *Metrics) GetConnectionActivityData(minutes int) []map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	cutoff := now.Add(-time.Duration(minutes) * time.Minute)

	// 创建时间间隔计数器
	intervals := make(map[string]int)
	intervalSize := time.Minute // 1分钟间隔

	// 从所有连接历史中收集数据
	for _, conn := range m.ConnectionHistory {
		if conn.StartTime.After(cutoff) {
			// 计算所属时间间隔
			interval := conn.StartTime.Truncate(intervalSize).Format("15:04")
			intervals[interval]++
		}
	}

	// 当前活跃连接
	for _, conn := range m.ActiveConnections {
		if conn.StartTime.After(cutoff) {
			interval := conn.StartTime.Truncate(intervalSize).Format("15:04")
			intervals[interval]++
		}
	}

	// 转换为图表数据格式
	var result []map[string]interface{}
	for interval, count := range intervals {
		result = append(result, map[string]interface{}{
			"time":  interval,
			"count": count,
		})
	}

	return result
}

// GetEndpointHealthDistribution 获取端点健康状态分布
func (m *Metrics) GetEndpointHealthDistribution() map[string]int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	distribution := map[string]int{
		"healthy":   0,
		"unhealthy": 0,
	}

	for _, endpoint := range m.EndpointStats {
		if endpoint.Healthy {
			distribution["healthy"]++
		} else {
			distribution["unhealthy"]++
		}
	}

	return distribution
}

// generateConnectionID generates a unique connection ID in format req-xxxxxxxx
func generateConnectionID() string {
	// Generate 4 random bytes for 8 hex characters
	bytes := make([]byte, 4)
	rand.Read(bytes)
	return "req-" + hex.EncodeToString(bytes)
}

// RecordRequestSuspended records a request being suspended
func (m *Metrics) RecordRequestSuspended(connID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.SuspendedRequests++
	m.TotalSuspendedRequests++

	// Update connection status
	if conn, exists := m.ActiveConnections[connID]; exists {
		conn.IsSuspended = true
		conn.SuspendedAt = time.Now()
		conn.Status = "suspended"
		conn.LastActivity = time.Now()
	}
}

// RecordRequestResumed records a suspended request being resumed
func (m *Metrics) RecordRequestResumed(connID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.SuspendedRequests--
	m.SuccessfulSuspendedRequests++

	// Update connection status and calculate suspended time
	if conn, exists := m.ActiveConnections[connID]; exists && conn.IsSuspended {
		conn.IsSuspended = false
		conn.ResumedAt = time.Now()
		conn.Status = "active" // Back to active status
		conn.LastActivity = time.Now()

		// Calculate suspended duration
		if !conn.SuspendedAt.IsZero() {
			suspendedDuration := conn.ResumedAt.Sub(conn.SuspendedAt)
			conn.SuspendedTime = suspendedDuration

			// Update overall suspended time metrics
			m.TotalSuspendedTime += suspendedDuration
			if m.MinSuspendedTime == 0 || suspendedDuration < m.MinSuspendedTime {
				m.MinSuspendedTime = suspendedDuration
			}
			if suspendedDuration > m.MaxSuspendedTime {
				m.MaxSuspendedTime = suspendedDuration
			}
		}
	}
}

// RecordRequestSuspendTimeout records a suspended request timing out
func (m *Metrics) RecordRequestSuspendTimeout(connID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.SuspendedRequests--
	m.TimeoutSuspendedRequests++

	// Update connection status and calculate suspended time
	if conn, exists := m.ActiveConnections[connID]; exists && conn.IsSuspended {
		conn.IsSuspended = false
		conn.Status = "timeout"
		conn.LastActivity = time.Now()

		// Calculate suspended duration
		if !conn.SuspendedAt.IsZero() {
			suspendedDuration := time.Since(conn.SuspendedAt)
			conn.SuspendedTime = suspendedDuration

			// Update overall suspended time metrics
			m.TotalSuspendedTime += suspendedDuration
			if m.MinSuspendedTime == 0 || suspendedDuration < m.MinSuspendedTime {
				m.MinSuspendedTime = suspendedDuration
			}
			if suspendedDuration > m.MaxSuspendedTime {
				m.MaxSuspendedTime = suspendedDuration
			}
		}

		// Move to history since the request failed due to timeout
		m.ConnectionHistory = append(m.ConnectionHistory, conn)
		delete(m.ActiveConnections, connID)

		// Limit history size
		if len(m.ConnectionHistory) > 1000 {
			m.ConnectionHistory = m.ConnectionHistory[len(m.ConnectionHistory)-1000:]
		}
	}
}

// GetAverageSuspendedTime calculates average suspended time
func (m *Metrics) GetAverageSuspendedTime() time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()

	totalProcessed := m.SuccessfulSuspendedRequests + m.TimeoutSuspendedRequests
	if totalProcessed == 0 {
		return 0
	}
	return m.TotalSuspendedTime / time.Duration(totalProcessed)
}

// GetSuspendedRequestStats returns suspended request statistics
func (m *Metrics) GetSuspendedRequestStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	totalProcessed := m.SuccessfulSuspendedRequests + m.TimeoutSuspendedRequests
	successRate := 0.0
	if totalProcessed > 0 {
		successRate = float64(m.SuccessfulSuspendedRequests) / float64(totalProcessed) * 100
	}

	return map[string]interface{}{
		"suspended_requests":            m.SuspendedRequests,
		"total_suspended_requests":      m.TotalSuspendedRequests,
		"successful_suspended_requests": m.SuccessfulSuspendedRequests,
		"timeout_suspended_requests":    m.TimeoutSuspendedRequests,
		"success_rate":                  successRate,
		"total_suspended_time":          m.TotalSuspendedTime.String(),
		"average_suspended_time":        m.GetAverageSuspendedTimeUnlocked().String(),
		"min_suspended_time":            m.MinSuspendedTime.String(),
		"max_suspended_time":            m.MaxSuspendedTime.String(),
	}
}

// GetAverageSuspendedTimeUnlocked calculates average suspended time (unlocked version)
func (m *Metrics) GetAverageSuspendedTimeUnlocked() time.Duration {
	totalProcessed := m.SuccessfulSuspendedRequests + m.TimeoutSuspendedRequests
	if totalProcessed == 0 {
		return 0
	}
	return m.TotalSuspendedTime / time.Duration(totalProcessed)
}

// GetSuspendedRequestHistory returns the suspended request history
func (m *Metrics) GetSuspendedRequestHistory() []SuspendedRequestHistoryPoint {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy of the suspended request history
	history := make([]SuspendedRequestHistoryPoint, len(m.SuspendedRequestHistory))
	copy(history, m.SuspendedRequestHistory)
	return history
}

// GetChartDataForSuspendedRequests gets suspended request chart data
func (m *Metrics) GetChartDataForSuspendedRequests(minutes int) []SuspendedRequestHistoryPoint {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	cutoff := now.Add(-time.Duration(minutes) * time.Minute)

	var result []SuspendedRequestHistoryPoint
	for _, point := range m.SuspendedRequestHistory {
		if point.Timestamp.After(cutoff) {
			result = append(result, point)
		}
	}

	return result
}

// GetActiveSuspendedConnections returns currently suspended connections
func (m *Metrics) GetActiveSuspendedConnections() []*ConnectionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var suspendedConnections []*ConnectionInfo
	for _, conn := range m.ActiveConnections {
		if conn.IsSuspended {
			suspendedConnections = append(suspendedConnections, &ConnectionInfo{
				ID:            conn.ID,
				ClientIP:      conn.ClientIP,
				UserAgent:     conn.UserAgent,
				StartTime:     conn.StartTime,
				LastActivity:  conn.LastActivity,
				Method:        conn.Method,
				Path:          conn.Path,
				Endpoint:      conn.Endpoint,
				Port:          conn.Port,
				RetryCount:    conn.RetryCount,
				Status:        conn.Status,
				BytesReceived: conn.BytesReceived,
				BytesSent:     conn.BytesSent,
				IsStreaming:   conn.IsStreaming,
				TokenUsage:    conn.TokenUsage,
				IsSuspended:   conn.IsSuspended,
				SuspendedAt:   conn.SuspendedAt,
				ResumedAt:     conn.ResumedAt,
				SuspendedTime: conn.SuspendedTime,
			})
		}
	}

	return suspendedConnections
}