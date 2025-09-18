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
	Type      string      `json:"type"`      // "start", "update", "update_with_model", "complete", "failed_request_tokens"
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

// RequestUpdateDataWithModel 包含模型信息的状态更新数据
type RequestUpdateDataWithModel struct {
	EndpointName string `json:"endpoint_name"`
	GroupName    string `json:"group_name"`
	Status       string `json:"status"`
	RetryCount   int    `json:"retry_count"`
	HTTPStatus   int    `json:"http_status"`
	ModelName    string `json:"model_name"` // 新增：模型信息
}

// RequestCompleteData 请求完成事件数据
type RequestCompleteData struct {
	ModelName           string        `json:"model_name"`
	InputTokens         int64         `json:"input_tokens"`
	OutputTokens        int64         `json:"output_tokens"`
	CacheCreationTokens int64         `json:"cache_creation_tokens"`
	CacheReadTokens     int64         `json:"cache_read_tokens"`
	Duration            time.Duration `json:"duration"`
	FailureReason       string        `json:"failure_reason,omitempty"` // 可选：失败原因
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

// WriteRequest 写操作请求
type WriteRequest struct {
	Query     string
	Args      []interface{}
	Response  chan error
	Context   context.Context
	EventType string  // 用于调试和监控
}

// UsageTracker 使用跟踪器
type UsageTracker struct {
	// 原有字段（兼容性）
	db           *sql.DB
	eventChan    chan RequestEvent
	config       *Config
	pricing      map[string]ModelPricing
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	mu           sync.RWMutex
	errorHandler *ErrorHandler
	
	// 新增：读写分离组件
	readDB     *sql.DB           // 读连接池 (多连接)
	writeDB    *sql.DB           // 写连接 (单连接)
	writeQueue chan WriteRequest // 写操作队列
	writeMu    sync.Mutex        // 写操作保护锁
	writeWg    sync.WaitGroup    // 写处理器等待组
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

	// 针对:memory:数据库的特殊处理
	var readDB, writeDB *sql.DB
	var err error
	
	if config.DatabasePath == ":memory:" {
		// 内存数据库：使用同一个连接（但配置为读写分离模式）
		db, err := sql.Open("sqlite", config.DatabasePath+"?_journal_mode=WAL&_synchronous=NORMAL&_cache_size=10000&_foreign_keys=1&_busy_timeout=60000")
		if err != nil {
			return nil, fmt.Errorf("failed to open memory database: %w", err)
		}
		
		// 使用同一个连接，但逻辑上分离读写
		readDB = db
		writeDB = db
		
		// 配置连接池参数
		db.SetMaxOpenConns(8)    // 内存数据库可以支持更多连接
		db.SetMaxIdleConns(4)
		db.SetConnMaxLifetime(2 * time.Hour)
	} else {
		// 文件数据库：真正的读写分离
		// 打开读数据库连接 - 读性能优化配置 (支持多并发查询)
		readDB, err = sql.Open("sqlite", config.DatabasePath+"?_journal_mode=WAL&_synchronous=NORMAL&_cache_size=10000&_foreign_keys=1&_busy_timeout=60000")
		if err != nil {
			return nil, fmt.Errorf("failed to open read database: %w", err)
		}

		// 打开写数据库连接 - 写稳定性优化配置 (单连接避免锁竞争)
		writeDB, err = sql.Open("sqlite", config.DatabasePath+"?_journal_mode=WAL&_synchronous=NORMAL&_cache_size=10000&_foreign_keys=1&_busy_timeout=60000")
		if err != nil {
			readDB.Close()
			return nil, fmt.Errorf("failed to open write database: %w", err)
		}

		// 配置读连接池参数 - 支持高并发查询
		readDB.SetMaxOpenConns(8)    // 8个读连接，支持高并发查询
		readDB.SetMaxIdleConns(4)    // 保持4个空闲连接
		readDB.SetConnMaxLifetime(2 * time.Hour)

		// 配置写连接参数 - 单连接避免锁竞争
		writeDB.SetMaxOpenConns(1)   // 关键：只有1个写连接
		writeDB.SetMaxIdleConns(1)   // 保持连接活跃
		writeDB.SetConnMaxLifetime(4 * time.Hour)
	}

	// 保持原有db字段兼容性（用于向后兼容和初始化）
	db := readDB

	ctx, cancel := context.WithCancel(context.Background())

	ut := &UsageTracker{
		// 原有字段（兼容性）
		db:        db,        // 兼容性：指向readDB
		eventChan: make(chan RequestEvent, config.BufferSize),
		config:    config,
		pricing:   config.ModelPricing,
		ctx:       ctx,
		cancel:    cancel,
		
		// 新增：读写分离组件
		readDB:     readDB,
		writeDB:    writeDB,
		writeQueue: make(chan WriteRequest, config.BufferSize), // 与事件队列容量一致
	}

	// 初始化错误处理器
	ut.errorHandler = NewErrorHandler(ut, slog.Default())

	// 初始化数据库（使用写连接以确保表创建）
	if err := ut.initDatabaseWithWriteDB(); err != nil {
		cancel()
		readDB.Close()
		writeDB.Close()
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	// 启动写操作处理器
	ut.startWriteProcessor()

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

	// 等待所有协程完成（包括写处理器）
	ut.wg.Wait()
	ut.writeWg.Wait() // 等待写处理器完成

	// 现在可以安全地持有写锁进行清理
	ut.mu.Lock()
	defer ut.mu.Unlock()
	
	ut.cancel = nil // 标记为已关闭

	// 关闭事件通道
	if ut.eventChan != nil {
		close(ut.eventChan)
		ut.eventChan = nil
	}
	
	// 关闭写操作队列
	if ut.writeQueue != nil {
		close(ut.writeQueue)
		ut.writeQueue = nil
	}

	// 关闭数据库连接
	var errors []error
	if ut.readDB != nil {
		if err := ut.readDB.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close read database: %w", err))
		}
		ut.readDB = nil
	}
	
	if ut.writeDB != nil {
		if err := ut.writeDB.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close write database: %w", err))
		}
		ut.writeDB = nil
	}
	
	// 关闭原有数据库连接（兼容性）
	if ut.db != nil {
		// 由于db指向readDB，避免重复关闭
		ut.db = nil
	}
	
	// 返回第一个错误（如果有）
	if len(errors) > 0 {
		return errors[0]
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
}

// RecordRequestUpdateWithModel 记录包含模型信息的状态更新
func (ut *UsageTracker) RecordRequestUpdateWithModel(requestID, endpoint, group, status string, retryCount, httpStatus int, modelName string) {
	if ut.config == nil || !ut.config.Enabled {
		return
	}

	event := RequestEvent{
		Type:      "update_with_model",
		RequestID: requestID,
		Timestamp: time.Now(),
		Data: RequestUpdateDataWithModel{
			EndpointName: endpoint,
			GroupName:    group,
			Status:       status,
			RetryCount:   retryCount,
			HTTPStatus:   httpStatus,
			ModelName:    modelName, // 包含模型信息
		},
	}

	select {
	case ut.eventChan <- event:
		// 成功发送事件
	default:
		slog.Warn("Usage tracking event buffer full, dropping update_with_model event", 
			"request_id", requestID)
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
}

// RecordFailedRequestTokens 记录失败请求的Token使用
// 只记录Token统计，不影响请求状态
func (ut *UsageTracker) RecordFailedRequestTokens(requestID, modelName string, tokens *TokenUsage, duration time.Duration, failureReason string) {
	if ut.config == nil || !ut.config.Enabled || tokens == nil {
		return
	}

	// 创建特殊的失败请求完成事件
	event := RequestEvent{
		Type:      "failed_request_tokens", // 新的事件类型
		RequestID: requestID,
		Timestamp: time.Now(),
		Data: RequestCompleteData{
			ModelName:           modelName,
			InputTokens:         tokens.InputTokens,
			OutputTokens:        tokens.OutputTokens,
			CacheCreationTokens: tokens.CacheCreationTokens,
			CacheReadTokens:     tokens.CacheReadTokens,
			Duration:            duration,
			FailureReason:       failureReason, // 新增失败原因字段
		},
	}

	select {
	case ut.eventChan <- event:
		slog.Debug(fmt.Sprintf("💾 [失败Token事件] [%s] 原因: %s, 模型: %s", requestID, failureReason, modelName))
	default:
		slog.Warn("Usage tracking event buffer full, dropping failed request tokens event",
			"request_id", requestID)
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

// HealthCheck 检查数据库连接状态和基本功能（使用读连接）
func (ut *UsageTracker) HealthCheck(ctx context.Context) error {
	if ut.config == nil || !ut.config.Enabled {
		return nil // 如果未启用，认为是健康的
	}
	
	if ut.readDB == nil {
		return fmt.Errorf("read database not initialized")
	}
	
	// 测试读数据库连接
	if err := ut.readDB.PingContext(ctx); err != nil {
		return fmt.Errorf("read database ping failed: %w", err)
	}
	
	// 测试写数据库连接
	if ut.writeDB != nil {
		if err := ut.writeDB.PingContext(ctx); err != nil {
			return fmt.Errorf("write database ping failed: %w", err)
		}
	}
	
	// 测试基本查询（使用读连接）
	var count int
	err := ut.readDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table'").Scan(&count)
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
	if ut.eventChan != nil {
		channelLoad := float64(len(ut.eventChan)) / float64(cap(ut.eventChan)) * 100
		if channelLoad > 90 {
			return fmt.Errorf("event channel overloaded: %.1f%% capacity used", channelLoad)
		}
	}
	
	// 检查写队列容量
	if ut.writeQueue != nil {
		writeQueueLoad := float64(len(ut.writeQueue)) / float64(cap(ut.writeQueue)) * 100
		if writeQueueLoad > 90 {
			return fmt.Errorf("write queue overloaded: %.1f%% capacity used", writeQueueLoad)
		}
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

// GetUsageStats 获取使用统计（便利方法，使用读连接）
func (ut *UsageTracker) GetUsageStats(ctx context.Context, startTime, endTime time.Time) (*UsageStatsDetailed, error) {
	if ut.readDB == nil {
		return nil, fmt.Errorf("read database not initialized")
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
	err := ut.readDB.QueryRowContext(ctx, query, startTime, endTime).Scan(
		&stats.TotalRequests,
		&stats.SuccessRequests,
		&stats.ErrorRequests,
		&stats.TotalTokens,
		&stats.TotalCost,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query detailed usage stats: %w", err)
	}
	
	// 获取模型统计（使用读连接）
	modelQuery := `SELECT model_name, COUNT(*), SUM(total_cost_usd)
		FROM request_logs 
		WHERE start_time >= ? AND start_time <= ? AND model_name IS NOT NULL AND model_name != ''
		GROUP BY model_name`
	
	rows, err := ut.readDB.QueryContext(ctx, modelQuery, startTime, endTime)
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
	
	// 获取端点统计（使用读连接）
	endpointQuery := `SELECT endpoint_name, COUNT(*), SUM(total_cost_usd)
		FROM request_logs 
		WHERE start_time >= ? AND start_time <= ? AND endpoint_name IS NOT NULL AND endpoint_name != ''
		GROUP BY endpoint_name`
	
	rows2, err := ut.readDB.QueryContext(ctx, endpointQuery, startTime, endTime)
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
	
	// 获取组统计（使用读连接）
	groupQuery := `SELECT group_name, COUNT(*), SUM(total_cost_usd)
		FROM request_logs 
		WHERE start_time >= ? AND start_time <= ? AND group_name IS NOT NULL AND group_name != ''
		GROUP BY group_name`
	
	rows3, err := ut.readDB.QueryContext(ctx, groupQuery, startTime, endTime)
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

// startWriteProcessor 启动写操作队列处理器（简化版，确保稳定性）
func (ut *UsageTracker) startWriteProcessor() {
	ut.writeWg.Add(1)
	go func() {
		defer ut.writeWg.Done()
		slog.Debug("Write processor started")
		
		for {
			select {
			case writeReq := <-ut.writeQueue:
				err := ut.executeWriteSimple(writeReq)
				writeReq.Response <- err
				
			case <-ut.ctx.Done():
				slog.Debug("Write processor stopped")
				return
			}
		}
	}()
}

// executeWriteSimple 执行简单写操作（避免复杂的批处理）
func (ut *UsageTracker) executeWriteSimple(req WriteRequest) error {
	ut.writeMu.Lock()
	defer ut.writeMu.Unlock()
	
	ctx, cancel := context.WithTimeout(req.Context, 30*time.Second)
	defer cancel()
	
	// 直接执行，不使用事务（对于简单INSERT/UPDATE，不一定需要事务）
	if req.EventType == "vacuum" {
		// VACUUM不能在事务中执行
		_, err := ut.writeDB.ExecContext(ctx, req.Query, req.Args...)
		return err
	}
	
	// 其他操作使用短事务
	tx, err := ut.writeDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	
	committed := false
	defer func() {
		if !committed {
			if rbErr := tx.Rollback(); rbErr != nil {
				slog.Debug("Failed to rollback transaction", "error", rbErr, "event_type", req.EventType)
			}
		}
	}()
	
	_, err = tx.ExecContext(ctx, req.Query, req.Args...)
	if err != nil {
		return fmt.Errorf("failed to execute query: %w", err)
	}
	
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	
	committed = true
	return nil
}

// executeWrite 执行单个写操作
func (ut *UsageTracker) executeWrite(req WriteRequest) error {
	ut.writeMu.Lock()
	defer ut.writeMu.Unlock()
	
	ctx, cancel := context.WithTimeout(req.Context, 60*time.Second)
	defer cancel()
	
	// 安全的事务处理
	return ut.executeWriteTransaction(ctx, req)
}

// executeWriteTransaction 执行写事务（修复defer tx.Rollback()问题）
func (ut *UsageTracker) executeWriteTransaction(ctx context.Context, req WriteRequest) error {
	tx, err := ut.writeDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	
	committed := false
	defer func() {
		if !committed {
			if rbErr := tx.Rollback(); rbErr != nil {
				slog.Error("Failed to rollback transaction", "error", rbErr, "event_type", req.EventType)
			}
		}
	}()
	
	_, err = tx.ExecContext(ctx, req.Query, req.Args...)
	if err != nil {
		return fmt.Errorf("failed to execute query: %w", err)
	}
	
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	
	committed = true  // 标记已提交，避免重复Rollback
	return nil
}

// initDatabaseWithWriteDB 使用写连接初始化数据库（同步等待完成）
func (ut *UsageTracker) initDatabaseWithWriteDB() error {
	// 读取并执行 schema SQL
	schemaSQL, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		return fmt.Errorf("failed to read schema.sql: %w", err)
	}

	// 使用写连接直接执行 schema（同步方式，确保表创建完成）
	if _, err := ut.writeDB.Exec(string(schemaSQL)); err != nil {
		return fmt.Errorf("failed to execute schema: %w", err)
	}

	slog.Debug("Database schema initialized successfully with write connection")
	return nil
}