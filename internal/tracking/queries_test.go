package tracking

import (
	"context"
	"testing"
	"time"
)

func TestQueryOperations(t *testing.T) {
	config := &Config{
		Enabled:         true,
		DatabasePath:    ":memory:",
		BufferSize:      100,
		BatchSize:       10,
		FlushInterval:   50 * time.Millisecond,
		MaxRetry:        3,
		CleanupInterval: 24 * time.Hour,
		RetentionDays:   30,
		ModelPricing: map[string]ModelPricing{
			"claude-3-5-haiku-20241022": {
				Input:         1.00,
				Output:        5.00,
				CacheCreation: 1.25,
				CacheRead:     0.10,
			},
			"claude-sonnet-4-20250514": {
				Input:         3.00,
				Output:        15.00,
				CacheCreation: 3.75,
				CacheRead:     0.30,
			},
		},
	}
	
	tracker, err := NewUsageTracker(config)
	if err != nil {
		t.Fatalf("Failed to create usage tracker: %v", err)
	}
	defer tracker.Close()
	
	// Create test data
	testData := []struct {
		requestID    string
		model        string
		endpoint     string
		group        string
		inputTokens  int64
		outputTokens int64
		success      bool
	}{
		{"req-query-001", "claude-3-5-haiku-20241022", "endpoint-1", "group-a", 100, 50, true},
		{"req-query-002", "claude-3-5-haiku-20241022", "endpoint-1", "group-a", 200, 75, true},
		{"req-query-003", "claude-sonnet-4-20250514", "endpoint-2", "group-b", 150, 100, false},
		{"req-query-004", "claude-sonnet-4-20250514", "endpoint-2", "group-b", 300, 150, true},
		{"req-query-005", "claude-3-5-haiku-20241022", "endpoint-1", "group-a", 120, 60, true},
	}
	
	// Insert test data
	for _, data := range testData {
		tracker.RecordRequestStart(data.requestID, "127.0.0.1", "test-agent", "POST", "/v1/messages", false)
		
		status := "success"
		httpStatus := 200
		if !data.success {
			status = "error"
			httpStatus = 500
		}
		
		opts := UpdateOptions{
			EndpointName: stringPtr(data.endpoint),
			GroupName:    stringPtr(data.group),
			Status:       stringPtr(status),
			RetryCount:   intPtr(0),
			HttpStatus:   intPtr(httpStatus),
		}
		tracker.RecordRequestUpdate(data.requestID, opts)
		
		tokens := &TokenUsage{
			InputTokens:  data.inputTokens,
			OutputTokens: data.outputTokens,
		}
		
		tracker.RecordRequestSuccess(data.requestID, data.model, tokens, 500*time.Millisecond)
	}
	
	// Wait for processing
	time.Sleep(200 * time.Millisecond)
	
	ctx := context.Background()
	
	// Test GetUsageSummary
	t.Run("GetUsageSummary", func(t *testing.T) {
		// First update summaries
		tracker.updateUsageSummary()
		time.Sleep(100 * time.Millisecond)
		
		summaries, err := tracker.GetUsageSummary(ctx, time.Now().AddDate(0, 0, -1), time.Now().AddDate(0, 0, 1))
		if err != nil {
			t.Fatalf("Failed to get usage summary: %v", err)
		}
		
		if len(summaries) == 0 {
			t.Error("Expected usage summaries, got none")
		}
		
		totalRequests := int64(0)
		for _, summary := range summaries {
			totalRequests += int64(summary.RequestCount)
		}
		
		if totalRequests != 5 {
			t.Errorf("Expected total 5 requests in summary, got %d", totalRequests)
		}
	})
	
	// Test GetRequestLogs
	t.Run("GetRequestLogs", func(t *testing.T) {
		logs, err := tracker.GetRequestLogs(ctx, time.Now().AddDate(0, 0, -1), time.Now().AddDate(0, 0, 1), "", "", "", 10, 0)
		if err != nil {
			t.Fatalf("Failed to get request logs: %v", err)
		}
		
		if len(logs) != 5 {
			t.Errorf("Expected 5 request logs, got %d", len(logs))
		}
		
		// Test filtering by model
		haikuLogs, err := tracker.GetRequestLogs(ctx, time.Now().AddDate(0, 0, -1), time.Now().AddDate(0, 0, 1), "claude-3-5-haiku-20241022", "", "", 10, 0)
		if err != nil {
			t.Fatalf("Failed to get haiku logs: %v", err)
		}
		
		if len(haikuLogs) != 3 {
			t.Errorf("Expected 3 haiku logs, got %d", len(haikuLogs))
		}
		
		// Test filtering by endpoint
		endpoint1Logs, err := tracker.GetRequestLogs(ctx, time.Now().AddDate(0, 0, -1), time.Now().AddDate(0, 0, 1), "", "endpoint-1", "", 10, 0)
		if err != nil {
			t.Fatalf("Failed to get endpoint-1 logs: %v", err)
		}
		
		if len(endpoint1Logs) != 3 {
			t.Errorf("Expected 3 endpoint-1 logs, got %d", len(endpoint1Logs))
		}
		
		// Test filtering by group
		groupALogs, err := tracker.GetRequestLogs(ctx, time.Now().AddDate(0, 0, -1), time.Now().AddDate(0, 0, 1), "", "", "group-a", 10, 0)
		if err != nil {
			t.Fatalf("Failed to get group-a logs: %v", err)
		}
		
		if len(groupALogs) != 3 {
			t.Errorf("Expected 3 group-a logs, got %d", len(groupALogs))
		}
	})
	
	// Test GetUsageStats
	t.Run("GetUsageStats", func(t *testing.T) {
		stats, err := tracker.GetUsageStats(ctx, time.Now().AddDate(0, 0, -1), time.Now().AddDate(0, 0, 1))
		if err != nil {
			t.Fatalf("Failed to get usage stats: %v", err)
		}
		
		if stats.TotalRequests != 5 {
			t.Errorf("Expected 5 total requests in stats, got %d", stats.TotalRequests)
		}
		
		if stats.SuccessRequests != 4 {
			t.Errorf("Expected 4 success requests, got %d", stats.SuccessRequests)
		}
		
		if stats.ErrorRequests != 1 {
			t.Errorf("Expected 1 error request, got %d", stats.ErrorRequests)
		}
		
		expectedTotalTokens := int64(100 + 200 + 150 + 300 + 120 + 50 + 75 + 100 + 150 + 60) // input + output
		if stats.TotalTokens != expectedTotalTokens {
			t.Errorf("Expected %d total tokens, got %d", expectedTotalTokens, stats.TotalTokens)
		}
		
		if stats.TotalCost <= 0 {
			t.Errorf("Expected positive total cost, got %f", stats.TotalCost)
		}
		
		if len(stats.ModelStats) != 2 {
			t.Errorf("Expected 2 models in stats, got %d", len(stats.ModelStats))
		}
		
		if len(stats.EndpointStats) != 2 {
			t.Errorf("Expected 2 endpoints in stats, got %d", len(stats.EndpointStats))
		}
		
		if len(stats.GroupStats) != 2 {
			t.Errorf("Expected 2 groups in stats, got %d", len(stats.GroupStats))
		}
	})
	
	// Test pagination
	t.Run("Pagination", func(t *testing.T) {
		// Get first page
		page1, err := tracker.GetRequestLogs(ctx, time.Now().AddDate(0, 0, -1), time.Now().AddDate(0, 0, 1), "", "", "", 2, 0)
		if err != nil {
			t.Fatalf("Failed to get page 1: %v", err)
		}
		
		if len(page1) != 2 {
			t.Errorf("Expected 2 logs in page 1, got %d", len(page1))
		}
		
		// Get second page
		page2, err := tracker.GetRequestLogs(ctx, time.Now().AddDate(0, 0, -1), time.Now().AddDate(0, 0, 1), "", "", "", 2, 2)
		if err != nil {
			t.Fatalf("Failed to get page 2: %v", err)
		}
		
		if len(page2) != 2 {
			t.Errorf("Expected 2 logs in page 2, got %d", len(page2))
		}
		
		// Verify pages contain different records
		if page1[0].RequestID == page2[0].RequestID {
			t.Error("Pages should contain different records")
		}
	})
	
	// Test date range filtering
	t.Run("DateRangeFiltering", func(t *testing.T) {
		yesterday := time.Now().AddDate(0, 0, -1)
		tomorrow := time.Now().AddDate(0, 0, 1)
		
		logs, err := tracker.GetRequestLogs(ctx, yesterday, tomorrow, "", "", "", 10, 0)
		if err != nil {
			t.Fatalf("Failed to get logs with date range: %v", err)
		}
		
		if len(logs) != 5 {
			t.Errorf("Expected 5 logs in date range, got %d", len(logs))
		}
		
		// Test with narrow date range (should return 0)
		lastWeek := time.Now().AddDate(0, 0, -7)
		weekBeforeLast := time.Now().AddDate(0, 0, -14)
		
		oldLogs, err := tracker.GetRequestLogs(ctx, weekBeforeLast, lastWeek, "", "", "", 10, 0)
		if err != nil {
			t.Fatalf("Failed to get old logs: %v", err)
		}
		
		if len(oldLogs) != 0 {
			t.Errorf("Expected 0 old logs, got %d", len(oldLogs))
		}
	})
}

func TestExportOperations(t *testing.T) {
	config := &Config{
		Enabled:         true,
		DatabasePath:    ":memory:",
		BufferSize:      100,
		BatchSize:       10,
		FlushInterval:   50 * time.Millisecond,
		MaxRetry:        3,
		CleanupInterval: 24 * time.Hour,
		RetentionDays:   30,
		ModelPricing: map[string]ModelPricing{
			"test-model": {
				Input:  1.00,
				Output: 5.00,
			},
		},
	}
	
	tracker, err := NewUsageTracker(config)
	if err != nil {
		t.Fatalf("Failed to create usage tracker: %v", err)
	}
	defer tracker.Close()
	
	// Create test data
	for i := 0; i < 3; i++ {
		requestID := time.Now().Format("req-export-20060102150405") + string(rune('0'+i))
		
		tracker.RecordRequestStart(requestID, "127.0.0.1", "export-agent", "POST", "/v1/messages", false)
		
		opts := UpdateOptions{
			EndpointName: stringPtr("export-endpoint"),
			GroupName:    stringPtr("export-group"),
			Status:       stringPtr("success"),
			RetryCount:   intPtr(0),
			HttpStatus:   intPtr(200),
		}
		tracker.RecordRequestUpdate(requestID, opts)
		
		tokens := &TokenUsage{
			InputTokens:  100 + int64(i*10),
			OutputTokens: 50 + int64(i*5),
		}
		
		tracker.RecordRequestSuccess(requestID, "test-model", tokens, time.Duration(500+i*100)*time.Millisecond)
	}
	
	// Wait for processing
	time.Sleep(200 * time.Millisecond)
	
	ctx := context.Background()
	
	// Test CSV export
	t.Run("CSVExport", func(t *testing.T) {
		csvData, err := tracker.ExportToCSV(ctx, time.Now().AddDate(0, 0, -1), time.Now().AddDate(0, 0, 1), "", "", "")
		if err != nil {
			t.Fatalf("Failed to export to CSV: %v", err)
		}
		
		if len(csvData) == 0 {
			t.Error("Expected CSV data, got empty")
		}
		
		// Check that CSV contains header
		csvStr := string(csvData)
		if !stringContains(csvStr, "request_id") {
			t.Error("CSV should contain header with request_id")
		}
		
		if !stringContains(csvStr, "model_name") {
			t.Error("CSV should contain header with model_name")
		}
		
		// Check that CSV contains data
		if !stringContains(csvStr, "test-model") {
			t.Error("CSV should contain test data")
		}
	})
	
	// Test JSON export
	t.Run("JSONExport", func(t *testing.T) {
		jsonData, err := tracker.ExportToJSON(ctx, time.Now().AddDate(0, 0, -1), time.Now().AddDate(0, 0, 1), "", "", "")
		if err != nil {
			t.Fatalf("Failed to export to JSON: %v", err)
		}
		
		if len(jsonData) == 0 {
			t.Error("Expected JSON data, got empty")
		}
		
		// Check that JSON is valid
		jsonStr := string(jsonData)
		if !stringContains(jsonStr, "request_id") {
			t.Error("JSON should contain request_id field")
		}
		
		if !stringContains(jsonStr, "test-model") {
			t.Error("JSON should contain test data")
		}
		
		// Should be valid JSON array
		if !stringStartsWith(jsonStr, "[") || !stringEndsWith(jsonStr, "]") {
			t.Error("JSON should be an array")
		}
	})
}

func TestCleanupOperations(t *testing.T) {
	config := &Config{
		Enabled:         true,
		DatabasePath:    ":memory:",
		BufferSize:      100,
		BatchSize:       10,
		FlushInterval:   50 * time.Millisecond,
		MaxRetry:        3,
		RetentionDays:   1, // Keep data for 1 day
		CleanupInterval: 0, // Disable automatic cleanup for test
	}
	
	tracker, err := NewUsageTracker(config)
	if err != nil {
		t.Fatalf("Failed to create usage tracker: %v", err)
	}
	defer tracker.Close()
	
	// Directly insert a test record to trigger table creation
	_, err = tracker.db.Exec(`INSERT INTO request_logs (request_id, start_time, status) VALUES (?, datetime('now'), ?)`, "test-init", "pending")
	if err != nil {
		t.Fatalf("Failed to insert initial test record: %v", err)
	}
	
	// Now test cleanup with existing tables
	err = tracker.cleanupOldRecords()
	if err != nil {
		t.Errorf("Cleanup should not fail even with no old records: %v", err)
	}
	
	// Create some current data
	for i := 0; i < 3; i++ {
		requestID := time.Now().Format("req-cleanup-20060102150405") + string(rune('0'+i))
		
		tracker.RecordRequestStart(requestID, "127.0.0.1", "cleanup-agent", "POST", "/v1/messages", false)
	}
	
	// Wait for processing
	time.Sleep(200 * time.Millisecond)
	
	// Verify data exists
	ctx := context.Background()
	stats, err := tracker.GetDatabaseStats(ctx)
	if err != nil {
		t.Fatalf("Failed to get database stats: %v", err)
	}
	
	if stats.TotalRequests != 4 { // 3 from RecordRequestStart + 1 from direct insert
		t.Errorf("Expected 4 requests before cleanup, got %d", stats.TotalRequests)
	}
	
	// Run cleanup again (should not delete current data)
	err = tracker.cleanupOldRecords()
	if err != nil {
		t.Errorf("Cleanup failed: %v", err)
	}
	
	// Data should still exist (not old enough)
	stats, err = tracker.GetDatabaseStats(ctx)
	if err != nil {
		t.Fatalf("Failed to get database stats after cleanup: %v", err)
	}
	
	if stats.TotalRequests != 4 { // Should still be 4 (data not old enough to clean)
		t.Errorf("Expected 4 requests after cleanup, got %d", stats.TotalRequests)
	}
}

// Helper functions for string operations (moved to avoid conflicts)
func stringContains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr) >= 0
}

func stringStartsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[0:len(prefix)] == prefix
}

func stringEndsWith(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}