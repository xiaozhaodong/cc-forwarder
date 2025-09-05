package web

import (
	"encoding/json"
	"fmt"
	"net/http"
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
	_ = query.Get("status")      // TODO: implement filtering by status
	_ = query.Get("model")       // TODO: implement filtering by model
	_ = query.Get("endpoint")    // TODO: implement filtering by endpoint
	limitStr := query.Get("limit")
	
	_ = 100 // default limit
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			_ = parsedLimit // TODO: use parsed limit
		}
	}
	
	// For now, return placeholder data
	requests := []RequestDetailResponse{}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    requests,
		"total":   0,
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
	if period == "" {
		period = "7d" // default to 7 days
	}
	
	// For now, return placeholder data
	stats := UsageStatsResponse{
		Period:        period,
		TotalRequests: 0,
		SuccessRate:   0.0,
		AvgDuration:   0.0,
		TotalCost:     0.0,
		TopModels:     []ModelStats{},
		TopEndpoints:  []EndpointStats{},
		DailyStats:    []DailyStats{},
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    stats,
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
	
	_ = query.Get("start") // TODO: implement date filtering  
	_ = query.Get("end")   // TODO: implement date filtering
	
	switch format {
	case "csv":
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", `attachment; filename="usage_export.csv"`)
		
		// For now, return a simple CSV header
		w.Write([]byte("request_id,start_time,end_time,model_name,endpoint_name,group_name,status,input_tokens,output_tokens,total_cost_usd\n"))
		
	case "json":
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"format":  "json",
			"data":    []RequestDetailResponse{},
		})
		
	default:
		http.Error(w, "Unsupported format. Use 'csv' or 'json'", http.StatusBadRequest)
		return
	}
}