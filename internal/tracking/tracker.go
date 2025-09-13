package tracking

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaFS embed.FS

// EventBroadcaster 事件广播器接口
type EventBroadcaster interface {
	BroadcastConnectionUpdateSmart(data interface{}, changeType string)
	BroadcastStatusUpdateSmart(data interface{}, changeType string) 
	BroadcastRequestUpdateSmart(data interface{}, changeType string)
}

// UsageStatsDetailed 详细的使用统计
type UsageStatsDetailed struct {
	TotalRequests    int64                      `json:"total_requests"`
	SuccessRequests  int64                      `json:"success_requests"`
	ErrorRequests    int64                      `json:"error_requests"`
	TotalTokens      int64                      `json:"total_tokens"`
	TotalCost        float64                    `json:"total_cost"`
	ModelStats       map[string]ModelStat       `json:"model_stats"`
	EndpointStats    map[string]EndpointStat    `json:"endpoint_stats"`
	GroupStats       map[string]GroupStat       `json:"group_stats"`
}

// ModelStat 模型统计
type ModelStat struct {
	RequestCount int64   `json:"request_count"`
	TotalCost    float64 `json:"total_cost"`
}

// EndpointStat 端点统计
type EndpointStat struct {
	RequestCount int64   `json:"request_count"`
	TotalCost    float64 `json:"total_cost"`
}

// GroupStat 组统计
type GroupStat struct {
	RequestCount int64   `json:"request_count"`
	TotalCost    float64 `json:"total_cost"`
}

// RequestEvent 表示请求事件
type RequestEvent struct {
	Type      string      `json:"type"`      // "start", "update", "complete"
	RequestID string      `json:"request_id"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data"` // 根据Type不同而变化
}

// RequestStartData 请求开始事件数据
type RequestStartData struct {
	ClientIP    string `json:"client_ip"`
	UserAgent   string `json:"user_agent"`
	Method      string `json:"method"`
	Path        string `json:"path"`
	IsStreaming bool   `json:"is_streaming"` // 是否为流式请求
}

// RequestUpdateData 请求更新事件数据
type RequestUpdateData struct {
	EndpointName string `json:"endpoint_name"`
	GroupName    string `json:"group_name"`
	Status       string `json:"status"`
	RetryCount   int    `json:"retry_count"`
	HTTPStatus   int    `json:"http_status"`
}

// RequestCompleteData 请求完成事件数据
type RequestCompleteData struct {
	ModelName           string        `json:"model_name"`
	InputTokens         int64         `json:"input_tokens"`
	OutputTokens        int64         `json:"output_tokens"`
	CacheCreationTokens int64         `json:"cache_creation_tokens"`
	CacheReadTokens     int64         `json:"cache_read_tokens"`
	Duration            time.Duration `json:"duration"`
}

// TokenUsage token使用统计
type TokenUsage struct {
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
}

// ModelPricing 模型定价配置
type ModelPricing struct {
	Input         float64 `yaml:"input"`          // per 1M tokens
	Output        float64 `yaml:"output"`         // per 1M tokens
	CacheCreation float64 `yaml:"cache_creation"` // per 1M tokens (缓存创建)
	CacheRead     float64 `yaml:"cache_read"`     // per 1M tokens (缓存读取)
}

// Config 使用跟踪配置
type Config struct {
	Enabled         bool                     `yaml:"enabled"`
	DatabasePath    string                   `yaml:"database_path"`
	BufferSize      int                      `yaml:"buffer_size"`
	BatchSize       int                      `yaml:"batch_size"`
	FlushInterval   time.Duration            `yaml:"flush_interval"`
	MaxRetry        int                      `yaml:"max_retry"`
	RetentionDays   int                      `yaml:"retention_days"`
	CleanupInterval time.Duration            `yaml:"cleanup_interval"`
	ModelPricing    map[string]ModelPricing  `yaml:"model_pricing"`
	DefaultPricing  ModelPricing             `yaml:"default_pricing"`
}

// UsageTracker 使用跟踪器
type UsageTracker struct {
	db               *sql.DB
	eventChan        chan RequestEvent
	config           *Config
	pricing          map[string]ModelPricing
	ctx              context.Context
	cancel           context.CancelFunc
	wg               sync.WaitGroup
	mu               sync.RWMutex
	errorHandler     *ErrorHandler
	eventBroadcaster EventBroadcaster // 智能事件广播器
}

// NewUsageTracker 创建新的使用跟踪器
func NewUsageTracker(config *Config) (*UsageTracker, error) {
	if config == nil || !config.Enabled {
		return &UsageTracker{config: config}, nil
	}

	// 设置默认值
	if config.BufferSize <= 0 {
		config.BufferSize = 1000
	}
	if config.BatchSize <= 0 {
		config.BatchSize = 100
	}
	if config.FlushInterval <= 0 {
		config.FlushInterval = 30 * time.Second
	}
	if config.MaxRetry <= 0 {
		config.MaxRetry = 3
	}

	// 确保数据库目录存在
	if config.DatabasePath != ":memory:" && config.DatabasePath != "" {
		dbDir := filepath.Dir(config.DatabasePath)
		if err := os.MkdirAll(dbDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create database directory: %w", err)
		}
	}

	// 打开数据库 - 本地使用优化配置 (NORMAL模式平衡性能和安全性)
	db, err := sql.Open("sqlite", config.DatabasePath+"?_journal_mode=WAL&_synchronous=NORMAL&_cache_size=10000&_foreign_keys=1&_busy_timeout=30000")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// 设置连接池参数
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	ctx, cancel := context.WithCancel(context.Background())

	ut := &UsageTracker{
		db:        db,
		eventChan: make(chan RequestEvent, config.BufferSize),
		config:    config,
		pricing:   config.ModelPricing,
		ctx:       ctx,
		cancel:    cancel,
	}

	// 初始化错误处理器
	ut.errorHandler = NewErrorHandler(ut, slog.Default())

	// 初始化数据库
	if err := ut.initDatabase(); err != nil {
		cancel()
		db.Close()
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	// 启动事件处理协程
	ut.wg.Add(1)
	go ut.processEvents()

	// 启动定期清理协程
	if config.RetentionDays > 0 && config.CleanupInterval > 0 {
		ut.wg.Add(1)
		go ut.periodicCleanup()
	}

	// 启动定期备份协程（每6小时备份一次）
	ut.wg.Add(1)
	go ut.periodicBackup()

	slog.Info("Usage tracker initialized", 
		"database_path", config.DatabasePath,
		"buffer_size", config.BufferSize,
		"retention_days", config.RetentionDays)

	return ut, nil
}

// SetEventBroadcaster 设置事件广播器
func (ut *UsageTracker) SetEventBroadcaster(broadcaster EventBroadcaster) {
	ut.mu.Lock()
	defer ut.mu.Unlock()
	ut.eventBroadcaster = broadcaster
	slog.Info("事件广播器已设置")
}

// Close 关闭使用跟踪器
func (ut *UsageTracker) Close() error {
	if ut.config == nil || !ut.config.Enabled {
		return nil
	}

	// 先检查是否已经关闭
	ut.mu.RLock()
	if ut.cancel == nil {
		ut.mu.RUnlock()
		return nil // 已经关闭过
	}
	ut.mu.RUnlock()

	slog.Info("Shutting down usage tracker...")
	
	// 取消上下文（不需要持有锁）
	ut.cancel()

	// 等待所有协程完成（不持有锁，避免死锁）
	ut.wg.Wait()

	// 现在可以安全地持有写锁进行清理
	ut.mu.Lock()
	defer ut.mu.Unlock()
	
	ut.cancel = nil // 标记为已关闭

	// 关闭事件通道
	if ut.eventChan != nil {
		close(ut.eventChan)
		ut.eventChan = nil
	}

	// 关闭数据库
	if ut.db != nil {
		err := ut.db.Close()
		ut.db = nil
		return err
	}

	return nil
}

// RecordRequestStart 记录请求开始
func (ut *UsageTracker) RecordRequestStart(requestID, clientIP, userAgent, method, path string, isStreaming bool) {
	if ut.config == nil || !ut.config.Enabled {
		return
	}

	event := RequestEvent{
		Type:      "start",
		RequestID: requestID,
		Timestamp: time.Now(),
		Data: RequestStartData{
			ClientIP:    clientIP,
			UserAgent:   userAgent,
			Method:      method,
			Path:        path,
			IsStreaming: isStreaming,
		},
	}

	select {
	case ut.eventChan <- event:
		// 成功发送事件
	default:
		// 缓冲区满时的处理策略
		slog.Warn("Usage tracking event buffer full, dropping start event", 
			"request_id", requestID)
	}

	// 新增：立即推送新请求事件
	if ut.eventBroadcaster != nil {
		ut.eventBroadcaster.BroadcastRequestUpdateSmart(map[string]interface{}{
			"event_type":   "request_started",
			"request_id":   requestID,
			"client_ip":    clientIP,
			"user_agent":   userAgent,
			"timestamp":    time.Now().Format("2006-01-02 15:04:05"),
			"change_type":  "request_started",
		}, "request_started")
	}
}

// RecordRequestUpdate 记录请求状态更新
func (ut *UsageTracker) RecordRequestUpdate(requestID, endpoint, group, status string, retryCount, httpStatus int) {
	if ut.config == nil || !ut.config.Enabled {
		return
	}

	event := RequestEvent{
		Type:      "update",
		RequestID: requestID,
		Timestamp: time.Now(),
		Data: RequestUpdateData{
			EndpointName: endpoint,
			GroupName:    group,
			Status:       status,
			RetryCount:   retryCount,
			HTTPStatus:   httpStatus,
		},
	}

	select {
	case ut.eventChan <- event:
		// 成功发送事件
	default:
		slog.Warn("Usage tracking event buffer full, dropping update event", 
			"request_id", requestID)
	}

	// 新增：智能推送状态变更事件
	if ut.eventBroadcaster != nil {
		// 根据状态确定变更类型和优先级
		var changeType string
		switch status {
		case "error", "timeout":
			changeType = "error_response"
		case "suspended":
			changeType = "suspended_change"
		case "retry":
			changeType = "retry_attempt"
		case "processing":
			changeType = "request_processing"
		case "completed":
			changeType = "request_completed"
		default:
			changeType = "status_changed"
		}

		ut.eventBroadcaster.BroadcastConnectionUpdateSmart(map[string]interface{}{
			"event_type":       "request_updated",
			"request_id":       requestID,
			"endpoint_name":    endpoint,
			"group_name":       group,
			"status":          status,
			"retry_count":     retryCount,
			"http_status":     httpStatus,
			"timestamp":       time.Now().Format("2006-01-02 15:04:05"),
			"change_type":     changeType,
		}, changeType)
	}
}

// RecordRequestComplete 记录请求完成
func (ut *UsageTracker) RecordRequestComplete(requestID, modelName string, tokens *TokenUsage, duration time.Duration) {
	if ut.config == nil || !ut.config.Enabled || tokens == nil {
		return
	}

	event := RequestEvent{
		Type:      "complete",
		RequestID: requestID,
		Timestamp: time.Now(),
		Data: RequestCompleteData{
			ModelName:           modelName,
			InputTokens:         tokens.InputTokens,
			OutputTokens:        tokens.OutputTokens,
			CacheCreationTokens: tokens.CacheCreationTokens,
			CacheReadTokens:     tokens.CacheReadTokens,
			Duration:            duration,
		},
	}

	select {
	case ut.eventChan <- event:
		// 成功发送事件
	default:
		slog.Warn("Usage tracking event buffer full, dropping complete event", 
			"request_id", requestID)
	}

	// 新增：智能推送完成事件
	if ut.eventBroadcaster != nil {
		// 判断是否为慢请求
		changeType := "request_completed"
		if duration > 10*time.Second {
			changeType = "slow_request_completed"
		}

		// 计算总成本
		_, _, _, _, totalCost := ut.calculateCost(modelName, tokens)

		ut.eventBroadcaster.BroadcastRequestUpdateSmart(map[string]interface{}{
			"event_type":             "request_completed",
			"request_id":             requestID,
			"model_name":             modelName,
			"duration_ms":            duration.Milliseconds(),
			"input_tokens":           tokens.InputTokens,
			"output_tokens":          tokens.OutputTokens,
			"cache_creation_tokens":  tokens.CacheCreationTokens,
			"cache_read_tokens":      tokens.CacheReadTokens,
			"total_cost":             totalCost,
			"timestamp":              time.Now().Format("2006-01-02 15:04:05"),
			"change_type":            changeType,
		}, changeType)
	}
}

// UpdatePricing 更新模型定价（运行时动态更新）
func (ut *UsageTracker) UpdatePricing(pricing map[string]ModelPricing) {
	ut.mu.Lock()
	defer ut.mu.Unlock()
	
	ut.pricing = pricing
	slog.Info("Model pricing updated", "model_count", len(pricing))
}

// GetDatabaseStats 获取数据库统计信息（包装方法）
func (ut *UsageTracker) GetDatabaseStats(ctx context.Context) (*DatabaseStats, error) {
	if ut.config == nil || !ut.config.Enabled {
		return nil, fmt.Errorf("usage tracking not enabled")
	}
	if ut.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return ut.getDatabaseStatsInternal(ctx)
}

// HealthCheck 检查数据库连接状态和基本功能
func (ut *UsageTracker) HealthCheck(ctx context.Context) error {
	if ut.config == nil || !ut.config.Enabled {
		return nil // 如果未启用，认为是健康的
	}
	
	if ut.db == nil {
		return fmt.Errorf("database not initialized")
	}
	
	// 测试数据库连接
	if err := ut.db.PingContext(ctx); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}
	
	// 测试基本查询
	var count int
	err := ut.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table'").Scan(&count)
	if err != nil {
		return fmt.Errorf("database query test failed: %w", err)
	}
	
	// 检查表是否存在
	if count < 2 { // 至少应该有 request_logs 和 usage_summary 两个表
		return fmt.Errorf("database schema incomplete: expected at least 2 tables, found %d", count)
	}
	
	// 检查事件处理通道是否正常
	select {
	case <-ut.ctx.Done():
		return fmt.Errorf("usage tracker context cancelled")
	default:
		// 上下文正常
	}
	
	// 检查事件通道容量
	channelLoad := float64(len(ut.eventChan)) / float64(cap(ut.eventChan)) * 100
	if channelLoad > 90 {
		return fmt.Errorf("event channel overloaded: %.1f%% capacity used", channelLoad)
	}
	
	return nil
}

// ForceFlush 强制刷新所有待处理事件
func (ut *UsageTracker) ForceFlush() error {
	if ut.config == nil || !ut.config.Enabled {
		return nil
	}
	
	// 尝试发送一个特殊事件来触发批处理
	flushEvent := RequestEvent{
		Type:      "flush",
		RequestID: "force-flush-" + time.Now().Format("20060102150405"),
		Timestamp: time.Now(),
		Data:      nil,
	}
	
	select {
	case ut.eventChan <- flushEvent:
		slog.Info("Force flush event sent")
		return nil
	default:
		return fmt.Errorf("event channel full, cannot force flush")
	}
}

// GetPricing 获取模型定价
func (ut *UsageTracker) GetPricing(modelName string) ModelPricing {
	ut.mu.RLock()
	defer ut.mu.RUnlock()
	
	if pricing, exists := ut.pricing[modelName]; exists {
		return pricing
	}
	return ut.config.DefaultPricing
}

// GetConfiguredModels 获取配置中的所有模型列表
func (ut *UsageTracker) GetConfiguredModels() []string {
	ut.mu.RLock()
	defer ut.mu.RUnlock()
	
	models := make([]string, 0, len(ut.pricing))
	for modelName := range ut.pricing {
		models = append(models, modelName)
	}
	
	return models
}

// GetUsageSummary 获取使用摘要（便利方法）
func (ut *UsageTracker) GetUsageSummary(ctx context.Context, startTime, endTime time.Time) ([]UsageSummary, error) {
	opts := &QueryOptions{
		StartDate: &startTime,
		EndDate:   &endTime,
		Limit:     100,
	}
	return ut.QueryUsageSummary(ctx, opts)
}

// GetRequestLogs 获取请求日志（便利方法）
func (ut *UsageTracker) GetRequestLogs(ctx context.Context, startTime, endTime time.Time, modelName, endpointName, groupName string, limit, offset int) ([]RequestDetail, error) {
	opts := &QueryOptions{
		StartDate:    &startTime,
		EndDate:      &endTime,
		ModelName:    modelName,
		EndpointName: endpointName,
		GroupName:    groupName,
		Limit:        limit,
		Offset:       offset,
	}
	return ut.QueryRequestDetails(ctx, opts)
}

// GetUsageStats 获取使用统计（便利方法）
func (ut *UsageTracker) GetUsageStats(ctx context.Context, startTime, endTime time.Time) (*UsageStatsDetailed, error) {
	if ut.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `SELECT 
		COUNT(*) as total_requests,
		SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) as success_requests,
		SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END) as error_requests,
		SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens) as total_tokens,
		SUM(total_cost_usd) as total_cost
		FROM request_logs 
		WHERE start_time >= ? AND start_time <= ?`
	
	var stats UsageStatsDetailed
	err := ut.db.QueryRowContext(ctx, query, startTime, endTime).Scan(
		&stats.TotalRequests,
		&stats.SuccessRequests,
		&stats.ErrorRequests,
		&stats.TotalTokens,
		&stats.TotalCost,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query detailed usage stats: %w", err)
	}
	
	// 获取模型统计
	modelQuery := `SELECT model_name, COUNT(*), SUM(total_cost_usd)
		FROM request_logs 
		WHERE start_time >= ? AND start_time <= ? AND model_name IS NOT NULL AND model_name != ''
		GROUP BY model_name`
	
	rows, err := ut.db.QueryContext(ctx, modelQuery, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to query model stats: %w", err)
	}
	defer rows.Close()
	
	stats.ModelStats = make(map[string]ModelStat)
	for rows.Next() {
		var modelName string
		var requests int64
		var cost float64
		if err := rows.Scan(&modelName, &requests, &cost); err != nil {
			continue
		}
		stats.ModelStats[modelName] = ModelStat{
			RequestCount: requests,
			TotalCost:    cost,
		}
	}
	
	// 获取端点统计
	endpointQuery := `SELECT endpoint_name, COUNT(*), SUM(total_cost_usd)
		FROM request_logs 
		WHERE start_time >= ? AND start_time <= ? AND endpoint_name IS NOT NULL AND endpoint_name != ''
		GROUP BY endpoint_name`
	
	rows2, err := ut.db.QueryContext(ctx, endpointQuery, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to query endpoint stats: %w", err)
	}
	defer rows2.Close()
	
	stats.EndpointStats = make(map[string]EndpointStat)
	for rows2.Next() {
		var endpointName string
		var requests int64
		var cost float64
		if err := rows2.Scan(&endpointName, &requests, &cost); err != nil {
			continue
		}
		stats.EndpointStats[endpointName] = EndpointStat{
			RequestCount: requests,
			TotalCost:    cost,
		}
	}
	
	// 获取组统计
	groupQuery := `SELECT group_name, COUNT(*), SUM(total_cost_usd)
		FROM request_logs 
		WHERE start_time >= ? AND start_time <= ? AND group_name IS NOT NULL AND group_name != ''
		GROUP BY group_name`
	
	rows3, err := ut.db.QueryContext(ctx, groupQuery, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to query group stats: %w", err)
	}
	defer rows3.Close()
	
	stats.GroupStats = make(map[string]GroupStat)
	for rows3.Next() {
		var groupName string
		var requests int64
		var cost float64
		if err := rows3.Scan(&groupName, &requests, &cost); err != nil {
			continue
		}
		stats.GroupStats[groupName] = GroupStat{
			RequestCount: requests,
			TotalCost:    cost,
		}
	}
	
	return &stats, nil
}

// ExportToCSV 导出为CSV格式
func (ut *UsageTracker) ExportToCSV(ctx context.Context, startTime, endTime time.Time, modelName, endpointName, groupName string) ([]byte, error) {
	logs, err := ut.GetRequestLogs(ctx, startTime, endTime, modelName, endpointName, groupName, 10000, 0) // Export up to 10k records
	if err != nil {
		return nil, fmt.Errorf("failed to get request logs for CSV export: %w", err)
	}
	
	// CSV header
	csv := "request_id,client_ip,user_agent,method,path,start_time,end_time,duration_ms,endpoint_name,group_name,model_name,status,http_status_code,retry_count,input_tokens,output_tokens,cache_creation_tokens,cache_read_tokens,input_cost_usd,output_cost_usd,cache_creation_cost_usd,cache_read_cost_usd,total_cost_usd,created_at,updated_at\n"
	
	// CSV rows
	for _, log := range logs {
		endTime := ""
		if log.EndTime != nil {
			endTime = log.EndTime.Format(time.RFC3339)
		}
		
		durationMs := ""
		if log.DurationMs != nil {
			durationMs = fmt.Sprintf("%d", *log.DurationMs)
		}
		
		httpStatus := ""
		if log.HTTPStatusCode != nil {
			httpStatus = fmt.Sprintf("%d", *log.HTTPStatusCode)
		}
		
		csv += fmt.Sprintf("%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%d,%d,%d,%d,%d,%.6f,%.6f,%.6f,%.6f,%.6f,%s,%s\n",
			log.RequestID, log.ClientIP, log.UserAgent, log.Method, log.Path,
			log.StartTime.Format(time.RFC3339), endTime, durationMs,
			log.EndpointName, log.GroupName, log.ModelName, log.Status,
			httpStatus, log.RetryCount,
			log.InputTokens, log.OutputTokens, log.CacheCreationTokens, log.CacheReadTokens,
			log.InputCostUSD, log.OutputCostUSD, log.CacheCreationCostUSD, log.CacheReadCostUSD, log.TotalCostUSD,
			log.CreatedAt.Format(time.RFC3339), log.UpdatedAt.Format(time.RFC3339),
		)
	}
	
	return []byte(csv), nil
}

// ExportToJSON 导出为JSON格式
func (ut *UsageTracker) ExportToJSON(ctx context.Context, startTime, endTime time.Time, modelName, endpointName, groupName string) ([]byte, error) {
	logs, err := ut.GetRequestLogs(ctx, startTime, endTime, modelName, endpointName, groupName, 10000, 0) // Export up to 10k records
	if err != nil {
		return nil, fmt.Errorf("failed to get request logs for JSON export: %w", err)
	}
	
	// 使用标准库的json包序列化
	jsonBytes, err := json.Marshal(logs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal logs to JSON: %w", err)
	}
	
	return jsonBytes, nil
}