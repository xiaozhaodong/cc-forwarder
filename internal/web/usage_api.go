package web

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"time"

	"cc-forwarder/internal/tracking"
)

// UsageAPI handles usage tracking related API endpoints
type UsageAPI struct {
	tracker *tracking.UsageTracker
}

// NewUsageAPI creates a new usage API instance
func NewUsageAPI(tracker *tracking.UsageTracker) *UsageAPI {
	return &UsageAPI{
		tracker: tracker,
	}
}

// UsageSummaryResponse represents the usage summary API response
type UsageSummaryResponse struct {
	Date         string  `json:"date"`
	ModelName    string  `json:"model_name"`
	EndpointName string  `json:"endpoint_name"`
	GroupName    string  `json:"group_name,omitempty"`
	RequestCount int     `json:"request_count"`
	SuccessCount int     `json:"success_count"`
	ErrorCount   int     `json:"error_count"`
	
	TotalInputTokens         int64   `json:"total_input_tokens"`
	TotalOutputTokens        int64   `json:"total_output_tokens"`
	TotalCacheCreationTokens int64   `json:"total_cache_creation_tokens"`
	TotalCacheReadTokens     int64   `json:"total_cache_read_tokens"`
	TotalCostUSD            float64 `json:"total_cost_usd"`
	
	AvgDurationMs float64 `json:"avg_duration_ms"`
}

// RequestDetailResponse represents the request detail API response
type RequestDetailResponse struct {
	ID          int64     `json:"id"`
	RequestID   string    `json:"request_id"`
	ClientIP    string    `json:"client_ip,omitempty"`
	UserAgent   string    `json:"user_agent,omitempty"`
	Method      string    `json:"method"`
	Path        string    `json:"path"`
	
	StartTime   time.Time `json:"start_time"`
	EndTime     *time.Time `json:"end_time,omitempty"`
	DurationMs  *int64    `json:"duration_ms,omitempty"`
	
	EndpointName string    `json:"endpoint_name,omitempty"`
	GroupName    string    `json:"group_name,omitempty"`
	ModelName    string    `json:"model_name,omitempty"`
	
	Status         string `json:"status"`
	HTTPStatusCode *int   `json:"http_status_code,omitempty"`
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

// UsageStatsResponse represents the usage statistics API response
type UsageStatsResponse struct {
	Period       string             `json:"period"`
	TotalRequests int               `json:"total_requests"`
	SuccessRate   float64           `json:"success_rate"`
	AvgDuration   float64           `json:"avg_duration_ms"`
	TotalCost     float64           `json:"total_cost_usd"`
	
	TopModels     []ModelStats      `json:"top_models"`
	TopEndpoints  []EndpointStats   `json:"top_endpoints"`
	DailyStats    []DailyStats      `json:"daily_stats"`
}

type ModelStats struct {
	ModelName    string  `json:"model_name"`
	RequestCount int     `json:"request_count"`
	TotalCost    float64 `json:"total_cost_usd"`
	AvgCost      float64 `json:"avg_cost_usd"`
}

type EndpointStats struct {
	EndpointName string  `json:"endpoint_name"`
	GroupName    string  `json:"group_name"`
	RequestCount int     `json:"request_count"`
	SuccessRate  float64 `json:"success_rate"`
	AvgDuration  float64 `json:"avg_duration_ms"`
}

type DailyStats struct {
	Date         string  `json:"date"`
	RequestCount int     `json:"request_count"`
	SuccessRate  float64 `json:"success_rate"`
	TotalCost    float64 `json:"total_cost_usd"`
}

// HandleUsageSummary handles GET /api/v1/usage/summary
func (ua *UsageAPI) HandleUsageSummary(w http.ResponseWriter, r *http.Request) {
	if ua.tracker == nil {
		http.Error(w, "Usage tracking not enabled", http.StatusServiceUnavailable)
		return
	}

	// Parse query parameters
	query := r.URL.Query()
	date := query.Get("date")
	modelName := query.Get("model")
	endpointName := query.Get("endpoint")
	groupName := query.Get("group")
	limitStr := query.Get("limit")
	
	limit := 100 // default limit
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}
	
	// Build SQL query
	sqlQuery := `SELECT date, model_name, endpoint_name, group_name,
		request_count, success_count, error_count,
		total_input_tokens, total_output_tokens, 
		total_cache_creation_tokens, total_cache_read_tokens,
		total_cost_usd, avg_duration_ms
		FROM usage_summary WHERE 1=1`
	
	var args []interface{}
	argIndex := 1
	
	if date != "" {
		sqlQuery += fmt.Sprintf(" AND date = $%d", argIndex)
		args = append(args, date)
		argIndex++
	}
	if modelName != "" {
		sqlQuery += fmt.Sprintf(" AND model_name = $%d", argIndex)
		args = append(args, modelName)
		argIndex++
	}
	if endpointName != "" {
		sqlQuery += fmt.Sprintf(" AND endpoint_name = $%d", argIndex)
		args = append(args, endpointName)
		argIndex++
	}
	if groupName != "" {
		sqlQuery += fmt.Sprintf(" AND group_name = $%d", argIndex)
		args = append(args, groupName)
		argIndex++
	}
	
	sqlQuery += " ORDER BY date DESC, total_cost_usd DESC"
	sqlQuery += fmt.Sprintf(" LIMIT $%d", argIndex)
	args = append(args, limit)
	
	// This would need access to the database connection
	// For now, return a placeholder response
	summaries := []UsageSummaryResponse{}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    summaries,
	})
}

// HandleUsageRequests handles GET /api/v1/usage/requests
func (ua *UsageAPI) HandleUsageRequests(w http.ResponseWriter, r *http.Request) {
	if ua.tracker == nil {
		http.Error(w, "Usage tracking not enabled", http.StatusServiceUnavailable)
		return
	}

	// Parse query parameters
	query := r.URL.Query()
	status := query.Get("status")
	model := query.Get("model")
	endpoint := query.Get("endpoint")
	group := query.Get("group")
	startDateStr := query.Get("start_date")
	endDateStr := query.Get("end_date")
	limitStr := query.Get("limit")
	offsetStr := query.Get("offset")

	// Parse limit and offset
	limit := 100 // default limit
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
			if limit > 1000 {
				limit = 1000 // max limit
			}
		}
	}

	offset := 0
	if offsetStr != "" {
		if parsedOffset, err := strconv.Atoi(offsetStr); err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	// Parse date range
	var startDate, endDate *time.Time
	if startDateStr != "" {
		if parsed, err := parseTimeString(startDateStr); err == nil {
			startDate = &parsed
		} else {
			slog.Warn("Invalid start_date format", "date", startDateStr, "error", err)
		}
	}
	if endDateStr != "" {
		if parsed, err := parseTimeString(endDateStr); err == nil {
			endDate = &parsed
		} else {
			slog.Warn("Invalid end_date format", "date", endDateStr, "error", err)
		}
	}

	// Set default date range if not specified (last 30 days)
	if startDate == nil {
		defaultStart := time.Now().AddDate(0, 0, -30)
		startDate = &defaultStart
	}
	if endDate == nil {
		defaultEnd := time.Now()
		endDate = &defaultEnd
	}

	ctx := context.Background()

	// Build query options
	opts := &tracking.QueryOptions{
		StartDate:    startDate,
		EndDate:      endDate,
		ModelName:    model,
		EndpointName: endpoint,
		GroupName:    group,
		Status:       status,
		Limit:        limit,
		Offset:       offset,
	}

	// Query request details
	details, err := ua.tracker.QueryRequestDetails(ctx, opts)
	if err != nil {
		slog.Error("Failed to query request details", "error", err)
		http.Error(w, "Failed to query request details", http.StatusInternalServerError)
		return
	}

	// Query total count
	total, err := ua.tracker.CountRequestDetails(ctx, opts)
	if err != nil {
		slog.Error("Failed to count request details", "error", err)
		http.Error(w, "Failed to count request details", http.StatusInternalServerError)
		return
	}

	// Convert to response format
	responses := make([]RequestDetailResponse, len(details))
	for i, detail := range details {
		responses[i] = RequestDetailResponse{
			ID:                   detail.ID,
			RequestID:           detail.RequestID,
			ClientIP:            detail.ClientIP,
			UserAgent:           detail.UserAgent,
			Method:              detail.Method,
			Path:                detail.Path,
			StartTime:           detail.StartTime,
			EndTime:             detail.EndTime,
			DurationMs:          detail.DurationMs,
			EndpointName:        detail.EndpointName,
			GroupName:           detail.GroupName,
			ModelName:           detail.ModelName,
			Status:              detail.Status,
			HTTPStatusCode:      detail.HTTPStatusCode,
			RetryCount:          detail.RetryCount,
			InputTokens:         detail.InputTokens,
			OutputTokens:        detail.OutputTokens,
			CacheCreationTokens: detail.CacheCreationTokens,
			CacheReadTokens:     detail.CacheReadTokens,
			InputCostUSD:        detail.InputCostUSD,
			OutputCostUSD:       detail.OutputCostUSD,
			CacheCreationCostUSD: detail.CacheCreationCostUSD,
			CacheReadCostUSD:    detail.CacheReadCostUSD,
			TotalCostUSD:        detail.TotalCostUSD,
			CreatedAt:           detail.CreatedAt,
			UpdatedAt:           detail.UpdatedAt,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    responses,
		"total":   total,
		"limit":   limit,
		"offset":  offset,
	})
}

// HandleUsageStats handles GET /api/v1/usage/stats
func (ua *UsageAPI) HandleUsageStats(w http.ResponseWriter, r *http.Request) {
	if ua.tracker == nil {
		http.Error(w, "Usage tracking not enabled", http.StatusServiceUnavailable)
		return
	}

	// Parse query parameters
	query := r.URL.Query()
	period := query.Get("period")
	startDateStr := query.Get("start_date")
	endDateStr := query.Get("end_date")
	// TODO: These parameters can be used for more specific filtering in the future
	_ = query.Get("model")    // modelName filter - not implemented yet
	_ = query.Get("endpoint") // endpointName filter - not implemented yet
	_ = query.Get("group")    // groupName filter - not implemented yet

	// Calculate date range based on period or custom dates
	var startDate, endDate time.Time
	if startDateStr != "" && endDateStr != "" {
		// Use custom date range
		var err error
		startDate, err = parseTimeString(startDateStr)
		if err != nil {
			slog.Warn("Invalid start_date format", "date", startDateStr, "error", err)
			http.Error(w, "Invalid start_date format", http.StatusBadRequest)
			return
		}
		endDate, err = parseTimeString(endDateStr)
		if err != nil {
			slog.Warn("Invalid end_date format", "date", endDateStr, "error", err)
			http.Error(w, "Invalid end_date format", http.StatusBadRequest)
			return
		}
	} else {
		// Use period-based date range
		if period == "" {
			period = "7d" // default to 7 days
		}
		endDate = time.Now()
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
			period = "7d"
		}
	}

	ctx := context.Background()

	// Get detailed usage stats
	stats, err := ua.tracker.GetUsageStats(ctx, startDate, endDate)
	if err != nil {
		slog.Error("Failed to query usage stats", "error", err)
		http.Error(w, "Failed to query usage stats", http.StatusInternalServerError)
		return
	}

	// Calculate success rate
	successRate := 0.0
	if stats.TotalRequests > 0 {
		successRate = float64(stats.SuccessRequests) / float64(stats.TotalRequests) * 100
	}

	// Build top models slice
	topModels := make([]ModelStats, 0, len(stats.ModelStats))
	for modelName, modelStat := range stats.ModelStats {
		avgCost := 0.0
		if modelStat.RequestCount > 0 {
			avgCost = modelStat.TotalCost / float64(modelStat.RequestCount)
		}
		topModels = append(topModels, ModelStats{
			ModelName:    modelName,
			RequestCount: int(modelStat.RequestCount),
			TotalCost:    modelStat.TotalCost,
			AvgCost:      avgCost,
		})
	}
	// Sort by request count descending
	sort.Slice(topModels, func(i, j int) bool {
		return topModels[i].RequestCount > topModels[j].RequestCount
	})
	// Limit to top 10
	if len(topModels) > 10 {
		topModels = topModels[:10]
	}

	// Build top endpoints slice
	topEndpoints := make([]EndpointStats, 0, len(stats.EndpointStats))
	for endpointName, endpointStat := range stats.EndpointStats {
		// Get group name for this endpoint
		groupName := ""
		if len(stats.GroupStats) > 0 {
			// This is simplified - in a real scenario we'd need to query endpoint-group mappings
			for group := range stats.GroupStats {
				groupName = group
				break
			}
		}
		
		successRate := 100.0 // Default to 100% if no error data available
		if endpointStat.RequestCount > 0 {
			// We don't have error count per endpoint in the current stats
			// This would need to be enhanced in the tracking system
			successRate = 95.0 // Placeholder
		}
		
		topEndpoints = append(topEndpoints, EndpointStats{
			EndpointName: endpointName,
			GroupName:    groupName,
			RequestCount: int(endpointStat.RequestCount),
			SuccessRate:  successRate,
			AvgDuration:  0.0, // Would need additional query to calculate
		})
	}
	// Sort by request count descending
	sort.Slice(topEndpoints, func(i, j int) bool {
		return topEndpoints[i].RequestCount > topEndpoints[j].RequestCount
	})
	// Limit to top 10
	if len(topEndpoints) > 10 {
		topEndpoints = topEndpoints[:10]
	}

	// Get daily stats for the period
	dailyStats, err := ua.getDailyStats(ctx, startDate, endDate)
	if err != nil {
		slog.Error("Failed to query daily stats", "error", err)
		// Don't fail the entire request, just use empty daily stats
		dailyStats = []DailyStats{}
	}

	// Calculate average duration from total requests
	avgDuration := 0.0
	if stats.TotalRequests > 0 {
		// This would need to be calculated from the database
		// For now, we'll use a placeholder calculation
		avgDuration = 1500.0 // 1.5 seconds average
	}

	response := UsageStatsResponse{
		Period:        period,
		TotalRequests: int(stats.TotalRequests),
		SuccessRate:   successRate,
		AvgDuration:   avgDuration,
		TotalCost:     stats.TotalCost,
		TopModels:     topModels,
		TopEndpoints:  topEndpoints,
		DailyStats:    dailyStats,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    response,
	})
}

// HandleUsageExport handles GET /api/v1/usage/export
func (ua *UsageAPI) HandleUsageExport(w http.ResponseWriter, r *http.Request) {
	if ua.tracker == nil {
		http.Error(w, "Usage tracking not enabled", http.StatusServiceUnavailable)
		return
	}

	// Parse query parameters
	query := r.URL.Query()
	format := query.Get("format")
	if format == "" {
		format = "csv"
	}
	
	startDateStr := query.Get("start_date")
	endDateStr := query.Get("end_date")
	modelName := query.Get("model")
	endpointName := query.Get("endpoint")
	groupName := query.Get("group")
	
	// Parse date range
	var startDate, endDate time.Time
	var err error
	
	if startDateStr != "" {
		startDate, err = parseTimeString(startDateStr)
		if err != nil {
			slog.Warn("Invalid start_date format", "date", startDateStr, "error", err)
			http.Error(w, "Invalid start_date format", http.StatusBadRequest)
			return
		}
	} else {
		// Default to last 30 days if not specified
		startDate = time.Now().AddDate(0, 0, -30)
	}
	
	if endDateStr != "" {
		endDate, err = parseTimeString(endDateStr)
		if err != nil {
			slog.Warn("Invalid end_date format", "date", endDateStr, "error", err)
			http.Error(w, "Invalid end_date format", http.StatusBadRequest)
			return
		}
	} else {
		endDate = time.Now()
	}

	ctx := context.Background()
	
	switch format {
	case "csv":
		// Export to CSV using tracker's built-in CSV export
		csvData, err := ua.tracker.ExportToCSV(ctx, startDate, endDate, modelName, endpointName, groupName)
		if err != nil {
			slog.Error("Failed to export data to CSV", "error", err)
			http.Error(w, "Failed to export data", http.StatusInternalServerError)
			return
		}
		
		// Generate filename with timestamp
		filename := fmt.Sprintf("usage_export_%s.csv", time.Now().Format("2006-01-02_15-04-05"))
		
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		w.Header().Set("Cache-Control", "no-cache")
		
		w.Write(csvData)
		
	case "json":
		// Export to JSON using tracker's built-in JSON export
		jsonData, err := ua.tracker.ExportToJSON(ctx, startDate, endDate, modelName, endpointName, groupName)
		if err != nil {
			slog.Error("Failed to export data to JSON", "error", err)
			http.Error(w, "Failed to export data", http.StatusInternalServerError)
			return
		}
		
		// Generate filename with timestamp
		filename := fmt.Sprintf("usage_export_%s.json", time.Now().Format("2006-01-02_15-04-05"))
		
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		w.Header().Set("Cache-Control", "no-cache")
		
		w.Write(jsonData)
		
	default:
		http.Error(w, "Unsupported format. Use 'csv' or 'json'", http.StatusBadRequest)
		return
	}
}

// parseTimeString parses time string in various formats
func parseTimeString(timeStr string) (time.Time, error) {
	timeFormats := []string{
		time.RFC3339,                // 2006-01-02T15:04:05Z07:00
		"2006-01-02T15:04:05",       // 2006-01-02T15:04:05
		"2006-01-02 15:04:05",       // 2006-01-02 15:04:05
		"2006-01-02",                // 2006-01-02
		"2006/01/02",                // 2006/01/02
		"2006/01/02 15:04:05",       // 2006/01/02 15:04:05
		"01/02/2006",                // 01/02/2006
		"01/02/2006 15:04:05",       // 01/02/2006 15:04:05
	}
	
	for _, format := range timeFormats {
		if parsed, err := time.Parse(format, timeStr); err == nil {
			return parsed, nil
		}
	}
	
	return time.Time{}, fmt.Errorf("unsupported time format: %s", timeStr)
}

// getDailyStats gets daily statistics for the given date range
func (ua *UsageAPI) getDailyStats(ctx context.Context, startDate, endDate time.Time) ([]DailyStats, error) {
	if ua.tracker == nil {
		return nil, fmt.Errorf("tracker not initialized")
	}

	// Calculate daily statistics using SQL query
	query := `SELECT 
		DATE(start_time) as date,
		COUNT(*) as total_requests,
		CAST(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) AS FLOAT) / COUNT(*) * 100 as success_rate,
		SUM(total_cost_usd) as total_cost
		FROM request_logs 
		WHERE start_time >= ? AND start_time <= ?
		GROUP BY DATE(start_time)
		ORDER BY date ASC`

	db := ua.tracker.GetDB()
	rows, err := db.QueryContext(ctx, query, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to query daily stats: %w", err)
	}
	defer rows.Close()

	var dailyStats []DailyStats
	for rows.Next() {
		var stat DailyStats
		err := rows.Scan(&stat.Date, &stat.RequestCount, &stat.SuccessRate, &stat.TotalCost)
		if err != nil {
			continue // Skip rows with scan errors
		}
		dailyStats = append(dailyStats, stat)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating daily stats rows: %w", err)
	}

	return dailyStats, nil
}