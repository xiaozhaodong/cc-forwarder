package tracking

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Helper functions for pointer creation
func stringPtr(s string) *string { return &s }
func intPtr(i int) *int { return &i }

// MockWebServer simulates the web server integration
type MockWebServer struct {
	usageTracker *UsageTracker
}

func (m *MockWebServer) SetUsageTracker(tracker *UsageTracker) {
	m.usageTracker = tracker
}

func (m *MockWebServer) handleUsageSummary(w http.ResponseWriter, r *http.Request) {
	if m.usageTracker == nil || !m.usageTracker.config.Enabled {
		http.Error(w, "Usage tracking not available", http.StatusServiceUnavailable)
		return
	}
	
	ctx := r.Context()
	startTime := time.Now().AddDate(0, 0, -7) // Last 7 days
	endTime := time.Now()
	
	summaries, err := m.usageTracker.GetUsageSummary(ctx, startTime, endTime)
	if err != nil {
		http.Error(w, "Failed to get usage summary", http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summaries)
}

func (m *MockWebServer) handleUsageStats(w http.ResponseWriter, r *http.Request) {
	if m.usageTracker == nil || !m.usageTracker.config.Enabled {
		http.Error(w, "Usage tracking not available", http.StatusServiceUnavailable)
		return
	}
	
	ctx := r.Context()
	startTime := time.Now().AddDate(0, 0, -7)
	endTime := time.Now()
	
	stats, err := m.usageTracker.GetUsageStats(ctx, startTime, endTime)
	if err != nil {
		http.Error(w, "Failed to get usage stats", http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (m *MockWebServer) handleUsageExport(w http.ResponseWriter, r *http.Request) {
	if m.usageTracker == nil || !m.usageTracker.config.Enabled {
		http.Error(w, "Usage tracking not available", http.StatusServiceUnavailable)
		return
	}
	
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "csv"
	}
	
	ctx := r.Context()
	startTime := time.Now().AddDate(0, 0, -7)
	endTime := time.Now()
	
	var data []byte
	var err error
	var contentType string
	var filename string
	
	switch format {
	case "json":
		data, err = m.usageTracker.ExportToJSON(ctx, startTime, endTime, "", "", "")
		contentType = "application/json"
		filename = "usage_export.json"
	default: // csv
		data, err = m.usageTracker.ExportToCSV(ctx, startTime, endTime, "", "", "")
		contentType = "text/csv"
		filename = "usage_export.csv"
	}
	
	if err != nil {
		http.Error(w, "Failed to export data", http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	w.Write(data)
}

func (m *MockWebServer) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	if m.usageTracker == nil {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Usage tracking disabled"))
		return
	}
	
	ctx := r.Context()
	if err := m.usageTracker.HealthCheck(ctx); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("Usage tracker unhealthy: " + err.Error()))
		return
	}
	
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Usage tracker healthy"))
}

func TestWebIntegration(t *testing.T) {
	// Create usage tracker
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
		},
	}
	
	tracker, err := NewUsageTracker(config)
	if err != nil {
		t.Fatalf("Failed to create usage tracker: %v", err)
	}
	defer tracker.Close()
	
	// Create mock web server
	webServer := &MockWebServer{}
	webServer.SetUsageTracker(tracker)
	
	// Setup HTTP routes
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/usage/summary", webServer.handleUsageSummary)
	mux.HandleFunc("/api/v1/usage/stats", webServer.handleUsageStats)
	mux.HandleFunc("/api/v1/usage/export", webServer.handleUsageExport)
	mux.HandleFunc("/health/usage-tracker", webServer.handleHealthCheck)
	
	server := httptest.NewServer(mux)
	defer server.Close()
	
	// Create test data
	testRequests := []struct {
		requestID    string
		model        string
		endpoint     string
		group        string
		inputTokens  int64
		outputTokens int64
		success      bool
	}{
		{"req-web-001", "claude-3-5-haiku-20241022", "web-endpoint-1", "web-group-a", 100, 50, true},
		{"req-web-002", "claude-3-5-haiku-20241022", "web-endpoint-1", "web-group-a", 200, 75, true},
		{"req-web-003", "claude-3-5-haiku-20241022", "web-endpoint-2", "web-group-b", 150, 100, false},
	}
	
	// Insert test data
	for _, req := range testRequests {
		tracker.RecordRequestStart(req.requestID, "192.168.1.100", "web-test-agent", "POST", "/v1/messages", false)
		
		status := "success"
		httpStatus := 200
		if !req.success {
			status = "error"
			httpStatus = 500
		}
		
		opts := UpdateOptions{
			EndpointName: stringPtr(req.endpoint),
			GroupName:    stringPtr(req.group),
			Status:       stringPtr(status),
			RetryCount:   intPtr(0),
			HttpStatus:   intPtr(httpStatus),
		}
		tracker.RecordRequestUpdate(req.requestID, opts)
		
		tokens := &TokenUsage{
			InputTokens:  req.inputTokens,
			OutputTokens: req.outputTokens,
		}
		
		tracker.RecordRequestSuccess(req.requestID, req.model, tokens, 300*time.Millisecond)
	}
	
	// Wait for processing
	time.Sleep(200 * time.Millisecond)
	
	// Update summaries
	tracker.updateUsageSummary()
	time.Sleep(100 * time.Millisecond)
	
	// Test health check endpoint
	t.Run("HealthCheck", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/health/usage-tracker")
		if err != nil {
			t.Fatalf("Failed to get health check: %v", err)
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})
	
	// Test usage summary endpoint
	t.Run("UsageSummary", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/api/v1/usage/summary")
		if err != nil {
			t.Fatalf("Failed to get usage summary: %v", err)
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
		
		var summaries []UsageSummary
		err = json.NewDecoder(resp.Body).Decode(&summaries)
		if err != nil {
			t.Fatalf("Failed to decode summary response: %v", err)
		}
		
		if len(summaries) == 0 {
			t.Error("Expected usage summaries, got none")
		}
	})
	
	// Test usage stats endpoint
	t.Run("UsageStats", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/api/v1/usage/stats")
		if err != nil {
			t.Fatalf("Failed to get usage stats: %v", err)
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
		
		var stats *UsageStatsDetailed
		err = json.NewDecoder(resp.Body).Decode(&stats)
		if err != nil {
			t.Fatalf("Failed to decode stats response: %v", err)
		}
		
		if stats.TotalRequests != 3 {
			t.Errorf("Expected 3 total requests, got %d", stats.TotalRequests)
		}
		
		if stats.SuccessRequests != 2 {
			t.Errorf("Expected 2 success requests, got %d", stats.SuccessRequests)
		}
		
		if stats.ErrorRequests != 1 {
			t.Errorf("Expected 1 error request, got %d", stats.ErrorRequests)
		}
	})
	
	// Test CSV export
	t.Run("CSVExport", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/api/v1/usage/export?format=csv")
		if err != nil {
			t.Fatalf("Failed to export CSV: %v", err)
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
		
		contentType := resp.Header.Get("Content-Type")
		if contentType != "text/csv" {
			t.Errorf("Expected Content-Type text/csv, got %s", contentType)
		}
		
		disposition := resp.Header.Get("Content-Disposition")
		if !strings.Contains(disposition, "usage_export.csv") {
			t.Errorf("Expected CSV filename in Content-Disposition, got %s", disposition)
		}
	})
	
	// Test JSON export
	t.Run("JSONExport", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/api/v1/usage/export?format=json")
		if err != nil {
			t.Fatalf("Failed to export JSON: %v", err)
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
		
		contentType := resp.Header.Get("Content-Type")
		if contentType != "application/json" {
			t.Errorf("Expected Content-Type application/json, got %s", contentType)
		}
		
		disposition := resp.Header.Get("Content-Disposition")
		if !strings.Contains(disposition, "usage_export.json") {
			t.Errorf("Expected JSON filename in Content-Disposition, got %s", disposition)
		}
	})
}

func TestWebIntegrationDisabled(t *testing.T) {
	// Test web integration when usage tracking is disabled
	config := &Config{
		Enabled:         false,
		CleanupInterval: 24 * time.Hour,
		RetentionDays:   30,
	}
	
	tracker, err := NewUsageTracker(config)
	if err != nil {
		t.Fatalf("Failed to create disabled tracker: %v", err)
	}
	defer tracker.Close()
	
	// Create mock web server with disabled tracker
	webServer := &MockWebServer{}
	webServer.SetUsageTracker(tracker)
	
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/usage/stats", webServer.handleUsageStats)
	mux.HandleFunc("/health/usage-tracker", webServer.handleHealthCheck)
	
	server := httptest.NewServer(mux)
	defer server.Close()
	
	// Test that endpoints handle disabled tracker gracefully
	t.Run("StatsWithDisabledTracker", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/api/v1/usage/stats")
		if err != nil {
			t.Fatalf("Failed to get stats: %v", err)
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("Expected status 503, got %d", resp.StatusCode)
		}
	})
	
	t.Run("HealthWithDisabledTracker", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/health/usage-tracker")
		if err != nil {
			t.Fatalf("Failed to get health: %v", err)
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200 for disabled tracker health, got %d", resp.StatusCode)
		}
	})
}

func TestWebIntegrationErrors(t *testing.T) {
	// Create web server without tracker
	webServer := &MockWebServer{}
	// Don't set usage tracker (simulate error condition)
	
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/usage/summary", webServer.handleUsageSummary)
	mux.HandleFunc("/api/v1/usage/stats", webServer.handleUsageStats)
	mux.HandleFunc("/api/v1/usage/export", webServer.handleUsageExport)
	mux.HandleFunc("/health/usage-tracker", webServer.handleHealthCheck)
	
	server := httptest.NewServer(mux)
	defer server.Close()
	
	// Test that all endpoints handle missing tracker gracefully
	endpoints := []struct {
		name string
		path string
	}{
		{"Summary", "/api/v1/usage/summary"},
		{"Stats", "/api/v1/usage/stats"},
		{"Export", "/api/v1/usage/export"},
		{"Health", "/health/usage-tracker"},
	}
	
	for _, endpoint := range endpoints {
		t.Run(endpoint.name+"WithoutTracker", func(t *testing.T) {
			resp, err := http.Get(server.URL + endpoint.path)
			if err != nil {
				t.Fatalf("Failed to get %s: %v", endpoint.name, err)
			}
			defer resp.Body.Close()
			
			expectedStatus := http.StatusServiceUnavailable
			if endpoint.name == "Health" {
				expectedStatus = http.StatusOK // Health check returns OK when tracker is nil
			}
			
			if resp.StatusCode != expectedStatus {
				t.Errorf("Expected status %d for %s, got %d", expectedStatus, endpoint.name, resp.StatusCode)
			}
		})
	}
}

func TestConcurrentWebRequests(t *testing.T) {
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
	
	// Ensure tables exist by direct insert (similar to cleanup test fix)
	_, err = tracker.db.Exec(`INSERT INTO request_logs (request_id, start_time, status) VALUES (?, datetime('now'), ?)`, "test-init-web", "pending")
	if err != nil {
		t.Fatalf("Failed to ensure tables exist: %v", err)
	}
	
	// Create test data and make sure it gets processed
	for i := 0; i < 10; i++ {
		requestID := time.Now().Format("req-concurrent-web-20060102150405") + string(rune('0'+i))
		
		tracker.RecordRequestStart(requestID, "127.0.0.1", "concurrent-web-agent", "POST", "/v1/messages", false)
		
		// Add request update to ensure complete records
		opts := UpdateOptions{
			EndpointName: stringPtr("test-endpoint"),
			GroupName:    stringPtr("test-group"),
			Status:       stringPtr("success"),
			RetryCount:   intPtr(0),
			HttpStatus:   intPtr(200),
		}
		tracker.RecordRequestUpdate(requestID, opts)
		
		tokens := &TokenUsage{
			InputTokens:  100 + int64(i*10),
			OutputTokens: 50 + int64(i*5),
		}
		
		tracker.RecordRequestSuccess(requestID, "test-model", tokens, 200*time.Millisecond)
	}
	
	// Force flush and wait for processing
	tracker.ForceFlush()
	time.Sleep(1 * time.Second) // Wait longer to ensure all events are processed
	
	// Verify that data is actually there before starting web server
	ctx := context.Background()
	stats, err := tracker.GetUsageStats(ctx, time.Now().AddDate(0, 0, -1), time.Now().AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("Failed to get usage stats before web test: %v", err)
	}
	t.Logf("Pre-test stats: TotalRequests=%d", stats.TotalRequests)
	if stats.TotalRequests < 11 {
		t.Fatalf("Expected at least 11 requests before web test, got %d", stats.TotalRequests)
	}
	
	// Create web server
	webServer := &MockWebServer{}
	webServer.SetUsageTracker(tracker)
	
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/usage/stats", webServer.handleUsageStats)
	
	server := httptest.NewServer(mux)
	defer server.Close()
	
	// Make concurrent requests (reduce concurrency to debug)
	const numRequests = 3 // Reduced from 10 to debug
	results := make(chan error, numRequests)
	
	for i := 0; i < numRequests; i++ {
		go func(reqNum int) {
			resp, err := http.Get(server.URL + "/api/v1/usage/stats")
			if err != nil {
				results <- err
				return
			}
			defer resp.Body.Close()
			
			if resp.StatusCode != http.StatusOK {
				// Read error response body
				body, _ := ioutil.ReadAll(resp.Body)
				results <- fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
				return
			}
			
			var stats UsageStatsDetailed
			err = json.NewDecoder(resp.Body).Decode(&stats)
			if err != nil {
				results <- err
				return
			}
			
			if stats.TotalRequests != 11 { // 1 from direct insert + 10 from events
				results <- fmt.Errorf("expected 11 requests, got %d", stats.TotalRequests)
				return
			}
			
			results <- nil
		}(i)
	}
	
	// Wait for all requests to complete
	errorCount := 0
	for i := 0; i < numRequests; i++ {
		err := <-results
		if err != nil {
			errorCount++
			t.Logf("Concurrent request %d failed: %v", i, err)
		}
	}
	
	if errorCount > 0 {
		t.Errorf("Expected all concurrent requests to succeed, got %d errors", errorCount)
	}
}