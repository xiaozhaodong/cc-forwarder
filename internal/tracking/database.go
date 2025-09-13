package tracking

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

// initDatabase 初始化数据库
func (ut *UsageTracker) initDatabase() error {
	// 读取并执行 schema SQL
	schemaSQL, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		return fmt.Errorf("failed to read schema.sql: %w", err)
	}

	// 执行 schema
	if _, err := ut.db.Exec(string(schemaSQL)); err != nil {
		return fmt.Errorf("failed to execute schema: %w", err)
	}

	slog.Debug("Database schema initialized successfully")
	return nil
}

// processEvents 异步事件处理循环
func (ut *UsageTracker) processEvents() {
	defer ut.wg.Done()
	
	ticker := time.NewTicker(ut.config.FlushInterval)
	defer ticker.Stop()

	batch := make([]RequestEvent, 0, ut.config.BatchSize)

	slog.Debug("Usage tracking event processor started")

	for {
		select {
		case event := <-ut.eventChan:
			batch = append(batch, event)
			if len(batch) >= ut.config.BatchSize {
				ut.flushBatch(batch)
				batch = batch[:0] // 重置切片但保留容量
			}

		case <-ticker.C:
			if len(batch) > 0 {
				ut.flushBatch(batch)
				batch = batch[:0]
			}

		case <-ut.ctx.Done():
			// 优雅关闭，处理剩余事件
			slog.Debug("Processing remaining events before shutdown", "count", len(batch))
			if len(batch) > 0 {
				ut.flushBatch(batch)
			}
			
			// 处理通道中剩余的事件
			for {
				select {
				case event := <-ut.eventChan:
					batch = append(batch, event)
					if len(batch) >= ut.config.BatchSize {
						ut.flushBatch(batch)
						batch = batch[:0]
					}
				default:
					if len(batch) > 0 {
						ut.flushBatch(batch)
					}
					slog.Debug("Usage tracking event processor stopped")
					return
				}
			}
		}
	}
}

// flushBatch 批量写入事件到数据库
func (ut *UsageTracker) flushBatch(events []RequestEvent) {
	if len(events) == 0 {
		return
	}

	var retryCount int
	for retryCount < ut.config.MaxRetry {
		if err := ut.processBatch(events); err != nil {
			// 使用错误处理器处理错误
			if ut.errorHandler != nil && ut.errorHandler.HandleDatabaseError(err, "flushBatch") {
				// 错误已处理，重试操作
				retryCount++
				slog.Info("Database error handled successfully, retrying", 
					"retry", retryCount, 
					"batch_size", len(events))
				time.Sleep(time.Duration(retryCount) * time.Second)
				continue
			}
			
			retryCount++
			slog.Warn("Failed to flush batch, retrying", 
				"error", err, 
				"retry", retryCount, 
				"max_retry", ut.config.MaxRetry,
				"batch_size", len(events))
			
			if retryCount < ut.config.MaxRetry {
				time.Sleep(time.Duration(retryCount) * time.Second)
			}
			continue
		}
		
		// 成功处理
		if retryCount > 0 {
			slog.Info("Batch processed successfully after retry", 
				"retry_count", retryCount, 
				"batch_size", len(events))
		}
		return
	}

	// 所有重试都失败
	slog.Error("Failed to process batch after all retries", 
		"batch_size", len(events), 
		"max_retry", ut.config.MaxRetry)
}

// processBatch 处理一批事件
func (ut *UsageTracker) processBatch(events []RequestEvent) error {
	// 增加超时时间以处理高并发场景
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	tx, err := ut.db.BeginTx(ctx, nil)
	if err != nil {
		slog.Error("Failed to begin database transaction", 
			"error", err, 
			"batch_size", len(events),
			"context_deadline", ctx.Err())
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	successCount := 0
	for _, event := range events {
		// 特殊处理flush事件
		if event.Type == "flush" {
			// flush事件不需要处理，只是用来触发批处理
			successCount++
			continue
		}
		
		switch event.Type {
		case "start":
			err = ut.insertRequestStart(ctx, tx, event)
		case "update":
			err = ut.updateRequestStatus(ctx, tx, event)
		case "complete":
			err = ut.completeRequest(ctx, tx, event)
		default:
			slog.Warn("Unknown event type", "type", event.Type, "request_id", event.RequestID)
			continue
		}

		if err != nil {
			slog.Error("Failed to process tracking event", 
				"error", err, 
				"event_type", event.Type, 
				"request_id", event.RequestID,
				"event_timestamp", event.Timestamp,
				"data_type", fmt.Sprintf("%T", event.Data))
			continue // 继续处理其他事件
		}
		successCount++
	}

	if err := tx.Commit(); err != nil {
		slog.Error("Failed to commit database transaction", 
			"error", err, 
			"batch_size", len(events),
			"success_count", successCount,
			"context_deadline", ctx.Err())
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	if successCount < len(events) {
		slog.Warn("Some events failed to process", 
			"success", successCount, 
			"total", len(events))
	}

	return nil
}

// insertRequestStart 插入请求开始记录
func (ut *UsageTracker) insertRequestStart(ctx context.Context, tx *sql.Tx, event RequestEvent) error {
	data, ok := event.Data.(RequestStartData)
	if !ok {
		return fmt.Errorf("invalid start event data type")
	}

	query := `INSERT INTO request_logs (
		request_id, client_ip, user_agent, method, path, start_time, status, is_streaming
	) VALUES (?, ?, ?, ?, ?, ?, 'pending', ?)
	ON CONFLICT(request_id) DO UPDATE SET
		client_ip = excluded.client_ip,
		user_agent = excluded.user_agent,
		method = excluded.method,
		path = excluded.path,
		start_time = excluded.start_time,
		is_streaming = excluded.is_streaming,
		updated_at = datetime('now', 'localtime')`

	_, err := tx.ExecContext(ctx, query, 
		event.RequestID, 
		data.ClientIP, 
		data.UserAgent, 
		data.Method, 
		data.Path, 
		event.Timestamp,
		data.IsStreaming)
	
	return err
}

// updateRequestStatus 更新请求状态
func (ut *UsageTracker) updateRequestStatus(ctx context.Context, tx *sql.Tx, event RequestEvent) error {
	data, ok := event.Data.(RequestUpdateData)
	if !ok {
		return fmt.Errorf("invalid update event data type")
	}

	// 如果端点名和组名都为空，只更新状态相关字段
	if data.EndpointName == "" && data.GroupName == "" {
		query := `UPDATE request_logs SET
			status = ?,
			retry_count = ?,
			http_status_code = ?,
			updated_at = datetime('now', 'localtime')
		WHERE request_id = ?`

		result, err := tx.ExecContext(ctx, query,
			data.Status,
			data.RetryCount,
			data.HTTPStatus,
			event.RequestID)
		
		if err != nil {
			return err
		}

		// 检查是否更新了记录
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		
		if rowsAffected == 0 {
			// 记录不存在，先创建一个基本记录
			return ut.insertRequestStart(ctx, tx, RequestEvent{
				Type:      "start", 
				RequestID: event.RequestID,
				Timestamp: event.Timestamp,
				Data: RequestStartData{
					ClientIP:    "unknown",
					UserAgent:   "unknown", 
					Method:      "unknown",
					Path:        "unknown",
					IsStreaming: false, // 默认为非流式
				},
			})
		}
		
		return nil
	}

	// 正常更新所有字段
	query := `UPDATE request_logs SET
		endpoint_name = ?,
		group_name = ?,
		status = ?,
		retry_count = ?,
		http_status_code = ?,
		updated_at = datetime('now', 'localtime')
	WHERE request_id = ?`

	result, err := tx.ExecContext(ctx, query,
		data.EndpointName,
		data.GroupName,
		data.Status,
		data.RetryCount,
		data.HTTPStatus,
		event.RequestID)
	
	if err != nil {
		return err
	}

	// 检查是否更新了记录
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	
	if rowsAffected == 0 {
		// 记录不存在，先创建一个基本记录
		insertQuery := `INSERT INTO request_logs (
			request_id, endpoint_name, group_name, status, retry_count, 
			http_status_code, start_time
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(request_id) DO UPDATE SET
			endpoint_name = excluded.endpoint_name,
			group_name = excluded.group_name,
			status = excluded.status,
			retry_count = excluded.retry_count,
			http_status_code = excluded.http_status_code,
			updated_at = datetime('now', 'localtime')`
		
		_, err = tx.ExecContext(ctx, insertQuery,
			event.RequestID,
			data.EndpointName,
			data.GroupName,
			data.Status,
			data.RetryCount,
			data.HTTPStatus,
			event.Timestamp)
	}

	return err
}

// completeRequest 完成请求记录
func (ut *UsageTracker) completeRequest(ctx context.Context, tx *sql.Tx, event RequestEvent) error {
	data, ok := event.Data.(RequestCompleteData)
	if !ok {
		return fmt.Errorf("invalid complete event data type")
	}

	// 首先获取请求的开始时间来计算正确的持续时间
	var startTime time.Time
	var durationMs int64
	
	queryStartTime := `SELECT start_time FROM request_logs WHERE request_id = ?`
	err := tx.QueryRowContext(ctx, queryStartTime, event.RequestID).Scan(&startTime)
	if err != nil {
		if err == sql.ErrNoRows {
			// 记录不存在，使用估算的开始时间
			startTime = event.Timestamp.Add(-data.Duration)
			durationMs = data.Duration.Milliseconds()
		} else {
			return fmt.Errorf("failed to get start time for duration calculation: %w", err)
		}
	} else {
		// 计算正确的持续时间：end_time - start_time
		durationMs = event.Timestamp.Sub(startTime).Milliseconds()
	}

	// 计算成本
	tokens := &TokenUsage{
		InputTokens:         data.InputTokens,
		OutputTokens:        data.OutputTokens,
		CacheCreationTokens: data.CacheCreationTokens,
		CacheReadTokens:     data.CacheReadTokens,
	}
	
	inputCost, outputCost, cacheCost, readCost, totalCost := ut.calculateCost(data.ModelName, tokens)

	query := `UPDATE request_logs SET
		end_time = ?,
		duration_ms = ?,
		model_name = ?,
		input_tokens = ?,
		output_tokens = ?,
		cache_creation_tokens = ?,
		cache_read_tokens = ?,
		input_cost_usd = ?,
		output_cost_usd = ?,
		cache_creation_cost_usd = ?,
		cache_read_cost_usd = ?,
		total_cost_usd = ?,
		status = CASE WHEN status != 'completed' THEN 'completed' ELSE status END,
		updated_at = datetime('now', 'localtime')
	WHERE request_id = ?`

	result, err := tx.ExecContext(ctx, query,
		event.Timestamp,
		durationMs,  // 使用计算出的正确持续时间
		data.ModelName,
		data.InputTokens,
		data.OutputTokens,
		data.CacheCreationTokens,
		data.CacheReadTokens,
		inputCost,
		outputCost,
		cacheCost,
		readCost,
		totalCost,
		event.RequestID)

	if err != nil {
		return err
	}

	// 检查是否更新了记录
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	
	if rowsAffected == 0 {
		// 记录不存在，创建完整记录
		insertQuery := `INSERT INTO request_logs (
			request_id, start_time, end_time, duration_ms, model_name,
			input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
			input_cost_usd, output_cost_usd, cache_creation_cost_usd, 
			cache_read_cost_usd, total_cost_usd, status
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'completed')`
		
		// 使用已计算的 startTime 和 durationMs
		_, err = tx.ExecContext(ctx, insertQuery,
			event.RequestID,
			startTime,
			event.Timestamp,
			durationMs,
			data.ModelName,
			data.InputTokens,
			data.OutputTokens,
			data.CacheCreationTokens,
			data.CacheReadTokens,
			inputCost,
			outputCost,
			cacheCost,
			readCost,
			totalCost)
	}

	return err
}

// calculateCost 计算请求成本
func (ut *UsageTracker) calculateCost(modelName string, tokens *TokenUsage) (inputCost, outputCost, cacheCost, readCost, totalCost float64) {
	pricing := ut.GetPricing(modelName)
	
	inputCost = float64(tokens.InputTokens) * pricing.Input / 1000000
	outputCost = float64(tokens.OutputTokens) * pricing.Output / 1000000
	cacheCost = float64(tokens.CacheCreationTokens) * pricing.CacheCreation / 1000000
	readCost = float64(tokens.CacheReadTokens) * pricing.CacheRead / 1000000
	totalCost = inputCost + outputCost + cacheCost + readCost

	return
}

// periodicCleanup 定期清理历史数据
func (ut *UsageTracker) periodicCleanup() {
	defer ut.wg.Done()
	
	ticker := time.NewTicker(ut.config.CleanupInterval)
	defer ticker.Stop()

	slog.Debug("Periodic cleanup task started", "interval", ut.config.CleanupInterval)

	for {
		select {
		case <-ticker.C:
			if err := ut.cleanupOldRecords(); err != nil {
				slog.Error("Failed to cleanup old records", "error", err)
			}
			
		case <-ut.ctx.Done():
			slog.Debug("Periodic cleanup task stopped")
			return
		}
	}
}

// cleanupOldRecords 清理过期记录
func (ut *UsageTracker) cleanupOldRecords() error {
	if ut.config.RetentionDays <= 0 {
		return nil // 永久保留
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cutoffTime := time.Now().AddDate(0, 0, -ut.config.RetentionDays)
	
	// 开始事务以确保数据一致性
	tx, err := ut.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin cleanup transaction: %w", err)
	}
	defer tx.Rollback()
	
	// 删除过期的请求记录
	requestQuery := "DELETE FROM request_logs WHERE start_time < ?"
	requestResult, err := tx.ExecContext(ctx, requestQuery, cutoffTime)
	if err != nil {
		return fmt.Errorf("failed to delete old request logs: %w", err)
	}

	requestDeletedCount, _ := requestResult.RowsAffected()
	
	// 清理过期的汇总数据
	summaryQuery := "DELETE FROM usage_summary WHERE date < ?"
	summaryResult, err := tx.ExecContext(ctx, summaryQuery, cutoffTime.Format("2006-01-02"))
	if err != nil {
		return fmt.Errorf("failed to delete old usage summary: %w", err)
	}

	summaryDeletedCount, _ := summaryResult.RowsAffected()
	
	// 提交事务
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit cleanup transaction: %w", err)
	}
	
	// 运行VACUUM以回收空间（仅在删除了数据时）
	if requestDeletedCount > 0 || summaryDeletedCount > 0 {
		if err := ut.vacuumDatabase(ctx); err != nil {
			slog.Warn("Failed to vacuum database after cleanup", "error", err)
		}
	}

	// 记录清理结果
	if requestDeletedCount > 0 {
		slog.Info("Cleaned up old request records", 
			"deleted_count", requestDeletedCount, 
			"cutoff_date", cutoffTime.Format("2006-01-02"),
			"retention_days", ut.config.RetentionDays)
	}
	
	if summaryDeletedCount > 0 {
		slog.Info("Cleaned up old summary records", 
			"deleted_count", summaryDeletedCount,
			"retention_days", ut.config.RetentionDays)
	}
	
	// 更新汇总统计（如果有数据变化）
	if requestDeletedCount > 0 {
		go ut.updateUsageSummary() // 异步更新汇总数据
	}

	return nil
}

// vacuumDatabase 运行VACUUM命令回收数据库空间
func (ut *UsageTracker) vacuumDatabase(ctx context.Context) error {
	slog.Debug("Running database VACUUM to reclaim space...")
	
	// VACUUM不能在事务中运行
	_, err := ut.db.ExecContext(ctx, "VACUUM")
	if err != nil {
		return fmt.Errorf("failed to vacuum database: %w", err)
	}
	
	slog.Debug("Database VACUUM completed successfully")
	return nil
}

// updateUsageSummary 更新使用汇总数据
func (ut *UsageTracker) updateUsageSummary() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	
	// 获取需要更新汇总的日期范围（最近7天）
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -7)
	
	query := `
	INSERT OR REPLACE INTO usage_summary (
		date, model_name, endpoint_name, group_name,
		request_count, success_count, error_count,
		total_input_tokens, total_output_tokens,
		total_cache_creation_tokens, total_cache_read_tokens,
		total_cost_usd, avg_duration_ms,
		created_at, updated_at
	)
	SELECT 
		DATE(start_time) as date,
		COALESCE(model_name, '') as model_name,
		COALESCE(endpoint_name, '') as endpoint_name,
		COALESCE(group_name, '') as group_name,
		COUNT(*) as request_count,
		SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) as success_count,
		SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END) as error_count,
		SUM(input_tokens) as total_input_tokens,
		SUM(output_tokens) as total_output_tokens,
		SUM(cache_creation_tokens) as total_cache_creation_tokens,
		SUM(cache_read_tokens) as total_cache_read_tokens,
		SUM(total_cost_usd) as total_cost_usd,
		AVG(CASE WHEN duration_ms IS NOT NULL AND duration_ms > 0 THEN duration_ms ELSE NULL END) as avg_duration_ms,
		datetime('now', 'localtime') as created_at,
		datetime('now', 'localtime') as updated_at
	FROM request_logs
	WHERE start_time >= ? AND start_time < ?
		AND (model_name IS NOT NULL OR endpoint_name IS NOT NULL)
	GROUP BY DATE(start_time), model_name, endpoint_name, group_name
	`
	
	result, err := ut.db.ExecContext(ctx, query, startDate, endDate.AddDate(0, 0, 1))
	if err != nil {
		slog.Error("Failed to update usage summary", "error", err)
		return
	}
	
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		slog.Info("Usage summary updated", "rows_updated", rowsAffected)
	}
}

// GetDatabaseStats 获取数据库统计信息
func (ut *UsageTracker) getDatabaseStatsInternal(ctx context.Context) (*DatabaseStats, error) {
	if ut.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	stats := &DatabaseStats{}
	
	// 获取请求记录总数
	err := ut.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM request_logs").Scan(&stats.TotalRequests)
	if err != nil {
		return nil, fmt.Errorf("failed to get total requests count: %w", err)
	}
	
	// 获取汇总记录总数
	err = ut.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_summary").Scan(&stats.TotalSummaries)
	if err != nil {
		return nil, fmt.Errorf("failed to get total summaries count: %w", err)
	}
	
	// 获取最早和最新的记录时间
	var earliestStr, latestStr sql.NullString
	err = ut.db.QueryRowContext(ctx, "SELECT MIN(start_time), MAX(start_time) FROM request_logs").Scan(&earliestStr, &latestStr)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to get record time range: %w", err)
	}
	
	if earliestStr.Valid {
		if t, err := time.Parse(time.RFC3339, earliestStr.String); err == nil {
			stats.EarliestRecord = &t
		}
	}
	if latestStr.Valid {
		if t, err := time.Parse(time.RFC3339, latestStr.String); err == nil {
			stats.LatestRecord = &t
		}
	}
	
	// 获取数据库文件大小（SQLite特有）
	var pageCount, pageSize int64
	err = ut.db.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pageCount)
	if err == nil {
		err = ut.db.QueryRowContext(ctx, "PRAGMA page_size").Scan(&pageSize)
		if err == nil {
			stats.DatabaseSize = pageCount * pageSize
		}
	}
	
	// 获取总成本
	err = ut.db.QueryRowContext(ctx, "SELECT COALESCE(SUM(total_cost_usd), 0) FROM request_logs WHERE total_cost_usd > 0").Scan(&stats.TotalCostUSD)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to get total cost: %w", err)
	}
	
	return stats, nil
}

// periodicBackup 定期备份数据库
func (ut *UsageTracker) periodicBackup() {
	defer ut.wg.Done()
	
	ticker := time.NewTicker(6 * time.Hour) // 每6小时备份一次
	defer ticker.Stop()

	slog.Debug("Periodic backup task started")

	for {
		select {
		case <-ticker.C:
			if ut.errorHandler != nil {
				if err := ut.errorHandler.CreateBackup(); err != nil {
					slog.Error("Scheduled backup failed", "error", err)
				} else {
					slog.Debug("Scheduled backup completed successfully")
				}
			}
			
		case <-ut.ctx.Done():
			slog.Debug("Periodic backup task stopped")
			return
		}
	}
}

// DatabaseStats 数据库统计信息
type DatabaseStats struct {
	TotalRequests   int64      `json:"total_requests"`
	TotalSummaries  int64      `json:"total_summaries"`
	EarliestRecord  *time.Time `json:"earliest_record,omitempty"`
	LatestRecord    *time.Time `json:"latest_record,omitempty"`
	DatabaseSize    int64      `json:"database_size_bytes"`
	TotalCostUSD    float64    `json:"total_cost_usd"`
}