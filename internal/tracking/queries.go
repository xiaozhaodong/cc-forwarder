package tracking

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

// QueryOptions represents options for querying usage data
type QueryOptions struct {
	StartDate    *time.Time
	EndDate      *time.Time
	ModelName    string
	EndpointName string
	GroupName    string
	Status       string
	Limit        int
	Offset       int
}

// UsageSummary represents a summary of usage data
type UsageSummary struct {
	Date         string    `json:"date"`
	ModelName    string    `json:"model_name"`
	EndpointName string    `json:"endpoint_name"`
	GroupName    string    `json:"group_name"`
	RequestCount int       `json:"request_count"`
	SuccessCount int       `json:"success_count"`
	ErrorCount   int       `json:"error_count"`
	
	TotalInputTokens         int64   `json:"total_input_tokens"`
	TotalOutputTokens        int64   `json:"total_output_tokens"`
	TotalCacheCreationTokens int64   `json:"total_cache_creation_tokens"`
	TotalCacheReadTokens     int64   `json:"total_cache_read_tokens"`
	TotalCostUSD            float64 `json:"total_cost_usd"`
	
	AvgDurationMs float64 `json:"avg_duration_ms"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// RequestDetail represents a detailed request record
type RequestDetail struct {
	ID          int64      `json:"id"`
	RequestID   string     `json:"request_id"`
	ClientIP    string     `json:"client_ip"`
	UserAgent   string     `json:"user_agent"`
	Method      string     `json:"method"`
	Path        string     `json:"path"`
	
	StartTime   time.Time  `json:"start_time"`
	EndTime     *time.Time `json:"end_time"`
	DurationMs  *int64     `json:"duration_ms"`
	
	EndpointName string    `json:"endpoint_name"`
	GroupName    string    `json:"group_name"`
	ModelName    string    `json:"model_name"`
	IsStreaming  bool      `json:"is_streaming"` // 是否为流式请求
	
	Status         string `json:"status"`
	HTTPStatusCode *int   `json:"http_status_code"`
	RetryCount     int    `json:"retry_count"`
	
	InputTokens         int64 `json:"input_tokens"`
	OutputTokens        int64 `json:"output_tokens"`
	CacheCreationTokens int64 `json:"cache_creation_tokens"`
	CacheReadTokens     int64 `json:"cache_read_tokens"`
	
	InputCostUSD         float64 `json:"input_cost_usd"`
	OutputCostUSD        float64 `json:"output_cost_usd"`
	CacheCreationCostUSD float64 `json:"cache_creation_cost_usd"`
	CacheReadCostUSD     float64 `json:"cache_read_cost_usd"`
	TotalCostUSD         float64 `json:"total_cost_usd"`
	
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UsageStats represents aggregated usage statistics
type UsageStats struct {
	Period        string  `json:"period"`
	TotalRequests int     `json:"total_requests"`
	SuccessRate   float64 `json:"success_rate"`
	AvgDuration   float64 `json:"avg_duration_ms"`
	TotalCost     float64 `json:"total_cost_usd"`
}

// GetDB returns the database connection for external queries
func (ut *UsageTracker) GetDB() *sql.DB {
	return ut.db
}

// QueryUsageSummary queries usage summary data
func (ut *UsageTracker) QueryUsageSummary(ctx context.Context, opts *QueryOptions) ([]UsageSummary, error) {
	if ut.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `SELECT date, model_name, endpoint_name, 
		COALESCE(group_name, '') as group_name,
		request_count, success_count, error_count,
		total_input_tokens, total_output_tokens, 
		total_cache_creation_tokens, total_cache_read_tokens,
		total_cost_usd, COALESCE(avg_duration_ms, 0.0) as avg_duration_ms, created_at, updated_at
		FROM usage_summary WHERE 1=1`
	
	var args []interface{}
	
	if opts.StartDate != nil {
		query += " AND date >= ?"
		args = append(args, opts.StartDate.Format("2006-01-02"))
	}
	if opts.EndDate != nil {
		query += " AND date <= ?"
		args = append(args, opts.EndDate.Format("2006-01-02"))
	}
	if opts.ModelName != "" {
		query += " AND model_name = ?"
		args = append(args, opts.ModelName)
	}
	if opts.EndpointName != "" {
		query += " AND endpoint_name = ?"
		args = append(args, opts.EndpointName)
	}
	if opts.GroupName != "" {
		query += " AND group_name = ?"
		args = append(args, opts.GroupName)
	}
	
	query += " ORDER BY date DESC, total_cost_usd DESC"
	
	if opts.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, opts.Limit)
	}
	if opts.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, opts.Offset)
	}
	
	rows, err := ut.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query usage summary: %w", err)
	}
	defer rows.Close()
	
	var summaries []UsageSummary
	for rows.Next() {
		var summary UsageSummary
		err := rows.Scan(
			&summary.Date, &summary.ModelName, &summary.EndpointName, &summary.GroupName,
			&summary.RequestCount, &summary.SuccessCount, &summary.ErrorCount,
			&summary.TotalInputTokens, &summary.TotalOutputTokens,
			&summary.TotalCacheCreationTokens, &summary.TotalCacheReadTokens,
			&summary.TotalCostUSD, &summary.AvgDurationMs,
			&summary.CreatedAt, &summary.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan usage summary: %w", err)
		}
		summaries = append(summaries, summary)
	}
	
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating usage summary rows: %w", err)
	}
	
	return summaries, nil
}

// QueryRequestDetails queries detailed request records
func (ut *UsageTracker) QueryRequestDetails(ctx context.Context, opts *QueryOptions) ([]RequestDetail, error) {
	if ut.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `SELECT id, request_id, 
		COALESCE(client_ip, '') as client_ip,
		COALESCE(user_agent, '') as user_agent,
		method, path, start_time, end_time, duration_ms,
		COALESCE(endpoint_name, '') as endpoint_name,
		COALESCE(group_name, '') as group_name,
		COALESCE(model_name, '') as model_name,
		COALESCE(is_streaming, false) as is_streaming,
		status, http_status_code, retry_count,
		input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
		input_cost_usd, output_cost_usd, cache_creation_cost_usd, 
		cache_read_cost_usd, total_cost_usd,
		created_at, updated_at
		FROM request_logs WHERE 1=1`
	
	var args []interface{}
	
	if opts.StartDate != nil {
		query += " AND start_time >= ?"
		args = append(args, opts.StartDate.Format("2006-01-02 15:04:05-07:00"))
	}
	if opts.EndDate != nil {
		query += " AND start_time <= ?"
		args = append(args, opts.EndDate.Format("2006-01-02 15:04:05-07:00"))
	}
	if opts.ModelName != "" {
		query += " AND model_name = ?"
		args = append(args, opts.ModelName)
	}
	if opts.EndpointName != "" {
		query += " AND endpoint_name = ?"
		args = append(args, opts.EndpointName)
	}
	if opts.GroupName != "" {
		query += " AND group_name = ?"
		args = append(args, opts.GroupName)
	}
	if opts.Status != "" {
		query += " AND status = ?"
		args = append(args, opts.Status)
	}
	
	query += " ORDER BY start_time DESC"
	
	if opts.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, opts.Limit)
	}
	if opts.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, opts.Offset)
	}
	
	rows, err := ut.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query request details: %w", err)
	}
	defer rows.Close()
	
	var details []RequestDetail
	for rows.Next() {
		var detail RequestDetail
		err := rows.Scan(
			&detail.ID, &detail.RequestID, 
			&detail.ClientIP, &detail.UserAgent, &detail.Method, &detail.Path,
			&detail.StartTime, &detail.EndTime, &detail.DurationMs,
			&detail.EndpointName, &detail.GroupName, &detail.ModelName, &detail.IsStreaming,
			&detail.Status, &detail.HTTPStatusCode, &detail.RetryCount,
			&detail.InputTokens, &detail.OutputTokens, 
			&detail.CacheCreationTokens, &detail.CacheReadTokens,
			&detail.InputCostUSD, &detail.OutputCostUSD, 
			&detail.CacheCreationCostUSD, &detail.CacheReadCostUSD, &detail.TotalCostUSD,
			&detail.CreatedAt, &detail.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan request detail: %w", err)
		}
		details = append(details, detail)
	}
	
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating request detail rows: %w", err)
	}
	
	return details, nil
}

// QueryUsageStats queries aggregated usage statistics
func (ut *UsageTracker) QueryUsageStats(ctx context.Context, period string) (*UsageStats, error) {
	if ut.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Calculate date range based on period
	endDate := time.Now()
	var startDate time.Time
	
	switch period {
	case "1d":
		startDate = endDate.AddDate(0, 0, -1)
	case "7d":
		startDate = endDate.AddDate(0, 0, -7)
	case "30d":
		startDate = endDate.AddDate(0, 0, -30)
	case "90d":
		startDate = endDate.AddDate(0, 0, -90)
	default:
		startDate = endDate.AddDate(0, 0, -7) // default to 7 days
	}
	
	query := `SELECT 
		COUNT(*) as total_requests,
		CAST(SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) AS FLOAT) / COUNT(*) * 100 as success_rate,
		AVG(CASE WHEN duration_ms IS NOT NULL THEN duration_ms ELSE 0 END) as avg_duration,
		SUM(total_cost_usd) as total_cost
		FROM request_logs 
		WHERE start_time >= ? AND start_time <= ?`
	
	var stats UsageStats
	stats.Period = period
	
	err := ut.db.QueryRowContext(ctx, query, startDate, endDate).Scan(
		&stats.TotalRequests,
		&stats.SuccessRate,
		&stats.AvgDuration,
		&stats.TotalCost,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query usage stats: %w", err)
	}
	
	// 添加调试日志
	slog.Debug("Usage stats query result", 
		"total_requests", stats.TotalRequests,
		"success_rate", stats.SuccessRate,
		"period", period,
		"start_date", startDate,
		"end_date", endDate)
	
	return &stats, nil
}

// CountRequestDetails returns the total count of request details matching the query options
func (ut *UsageTracker) CountRequestDetails(ctx context.Context, opts *QueryOptions) (int, error) {
	if ut.db == nil {
		return 0, fmt.Errorf("database not initialized")
	}

	query := "SELECT COUNT(*) FROM request_logs WHERE 1=1"
	var args []interface{}
	
	if opts.StartDate != nil {
		query += " AND start_time >= ?"
		args = append(args, opts.StartDate.Format("2006-01-02 15:04:05-07:00"))
	}
	if opts.EndDate != nil {
		query += " AND start_time <= ?"
		args = append(args, opts.EndDate.Format("2006-01-02 15:04:05-07:00"))
	}
	if opts.ModelName != "" {
		query += " AND model_name = ?"
		args = append(args, opts.ModelName)
	}
	if opts.EndpointName != "" {
		query += " AND endpoint_name = ?"
		args = append(args, opts.EndpointName)
	}
	if opts.GroupName != "" {
		query += " AND group_name = ?"
		args = append(args, opts.GroupName)
	}
	if opts.Status != "" {
		query += " AND status = ?"
		args = append(args, opts.Status)
	}
	
	var count int
	err := ut.db.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count request details: %w", err)
	}
	
	return count, nil
}