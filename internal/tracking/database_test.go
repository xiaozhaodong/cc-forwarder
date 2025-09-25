package tracking

import (
	"context"
	"testing"
	"time"
)

func TestDatabaseOperations(t *testing.T) {
	config := &Config{
		Enabled:         true,
		DatabasePath:    ":memory:",
		BufferSize:      100,
		BatchSize:       10,
		FlushInterval:   1 * time.Second,
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
		DefaultPricing: ModelPricing{
			Input:         2.00,
			Output:        10.00,
			CacheCreation: 2.50,
			CacheRead:     0.20,
		},
	}
	
	tracker, err := NewUsageTracker(config)
	if err != nil {
		t.Fatalf("Failed to create usage tracker: %v", err)
	}
	defer tracker.Close()
	
	ctx := context.Background()
	
	// Test database initialization
	stats, err := tracker.GetDatabaseStats(ctx)
	if err != nil {
		t.Fatalf("Failed to get database stats: %v", err)
	}
	
	if stats.TotalRequests != 0 {
		t.Errorf("Expected 0 requests initially, got %d", stats.TotalRequests)
	}
	
	// Test inserting a request start event
	events := []RequestEvent{
		{
			Type:      "start",
			RequestID: "req-test-001",
			Timestamp: time.Now(),
			Data: RequestStartData{
				ClientIP:  "127.0.0.1",
				UserAgent: "test-agent",
				Method:    "POST",
				Path:      "/v1/messages",
			},
		},
	}
	
	err = tracker.processBatch(events)
	if err != nil {
		t.Fatalf("Failed to process start event: %v", err)
	}
	
	// Test updating request status
	updateEvents := []RequestEvent{
		{
			Type:      "update",
			RequestID: "req-test-001",
			Timestamp: time.Now(),
			Data: RequestUpdateData{
				EndpointName: "test-endpoint",
				GroupName:    "test-group",
				Status:       "success",
				RetryCount:   0,
				HTTPStatus:   200,
			},
		},
	}
	
	err = tracker.processBatch(updateEvents)
	if err != nil {
		t.Fatalf("Failed to process update event: %v", err)
	}
	
	// Test completing request with token usage
	completeEvents := []RequestEvent{
		{
			Type:      "complete",
			RequestID: "req-test-001",
			Timestamp: time.Now(),
			Data: RequestCompleteData{
				ModelName:           "claude-3-5-haiku-20241022",
				InputTokens:         100,
				OutputTokens:        50,
				CacheCreationTokens: 10,
				CacheReadTokens:     5,
				Duration:            500 * time.Millisecond,
			},
		},
	}
	
	err = tracker.processBatch(completeEvents)
	if err != nil {
		t.Fatalf("Failed to process complete event: %v", err)
	}
	
	// Verify the request was stored correctly
	stats, err = tracker.GetDatabaseStats(ctx)
	if err != nil {
		t.Fatalf("Failed to get database stats after insertion: %v", err)
	}
	
	if stats.TotalRequests != 1 {
		t.Errorf("Expected 1 request after insertion, got %d", stats.TotalRequests)
	}
	
	if stats.TotalCostUSD <= 0 {
		t.Errorf("Expected positive total cost, got %f", stats.TotalCostUSD)
	}
}

func TestCostCalculation(t *testing.T) {
	config := &Config{
		Enabled:         true,
		DatabasePath:    ":memory:",
		CleanupInterval: 24 * time.Hour,
		RetentionDays:   30,
		ModelPricing: map[string]ModelPricing{
			"claude-3-5-haiku-20241022": {
				Input:         1.00,   // $1 per 1M tokens
				Output:        5.00,   // $5 per 1M tokens
				CacheCreation: 1.25,   // $1.25 per 1M tokens
				CacheRead:     0.10,   // $0.10 per 1M tokens
			},
		},
		DefaultPricing: ModelPricing{
			Input:         2.00,
			Output:        10.00,
			CacheCreation: 2.50,
			CacheRead:     0.20,
		},
	}
	
	tracker, err := NewUsageTracker(config)
	if err != nil {
		t.Fatalf("Failed to create usage tracker: %v", err)
	}
	defer tracker.Close()
	
	tokens := &TokenUsage{
		InputTokens:         100000,  // 0.1M tokens
		OutputTokens:        50000,   // 0.05M tokens
		CacheCreationTokens: 10000,   // 0.01M tokens
		CacheReadTokens:     5000,    // 0.005M tokens
	}
	
	inputCost, outputCost, cacheCost, readCost, totalCost := tracker.calculateCost("claude-3-5-haiku-20241022", tokens)
	
	// Expected costs:
	// Input: 0.1M * $1 = $0.10
	// Output: 0.05M * $5 = $0.25
	// Cache Creation: 0.01M * $1.25 = $0.0125
	// Cache Read: 0.005M * $0.10 = $0.0005
	// Total: $0.363
	
	expectedInputCost := 0.10
	expectedOutputCost := 0.25
	expectedCacheCost := 0.0125
	expectedReadCost := 0.0005
	expectedTotalCost := 0.363
	
	tolerance := 0.0001
	
	if abs(inputCost-expectedInputCost) > tolerance {
		t.Errorf("Expected input cost %f, got %f", expectedInputCost, inputCost)
	}
	
	if abs(outputCost-expectedOutputCost) > tolerance {
		t.Errorf("Expected output cost %f, got %f", expectedOutputCost, outputCost)
	}
	
	if abs(cacheCost-expectedCacheCost) > tolerance {
		t.Errorf("Expected cache cost %f, got %f", expectedCacheCost, cacheCost)
	}
	
	if abs(readCost-expectedReadCost) > tolerance {
		t.Errorf("Expected read cost %f, got %f", expectedReadCost, readCost)
	}
	
	if abs(totalCost-expectedTotalCost) > tolerance {
		t.Errorf("Expected total cost %f, got %f", expectedTotalCost, totalCost)
	}
}

func TestDefaultPricing(t *testing.T) {
	config := &Config{
		Enabled:         true,
		DatabasePath:    ":memory:",
		CleanupInterval: 24 * time.Hour,
		RetentionDays:   30,
		DefaultPricing: ModelPricing{
			Input:         2.00,
			Output:        10.00,
			CacheCreation: 2.50,
			CacheRead:     0.20,
		},
	}
	
	tracker, err := NewUsageTracker(config)
	if err != nil {
		t.Fatalf("Failed to create usage tracker: %v", err)
	}
	defer tracker.Close()
	
	tokens := &TokenUsage{
		InputTokens:  100000,  // 0.1M tokens
		OutputTokens: 50000,   // 0.05M tokens
	}
	
	inputCost, outputCost, _, _, totalCost := tracker.calculateCost("unknown-model", tokens)
	
	// Expected costs with default pricing:
	// Input: 0.1M * $2 = $0.20
	// Output: 0.05M * $10 = $0.50
	// Total: $0.70
	
	expectedInputCost := 0.20
	expectedOutputCost := 0.50
	expectedTotalCost := 0.70
	
	tolerance := 0.0001
	
	if abs(inputCost-expectedInputCost) > tolerance {
		t.Errorf("Expected default input cost %f, got %f", expectedInputCost, inputCost)
	}
	
	if abs(outputCost-expectedOutputCost) > tolerance {
		t.Errorf("Expected default output cost %f, got %f", expectedOutputCost, outputCost)
	}
	
	if abs(totalCost-expectedTotalCost) > tolerance {
		t.Errorf("Expected default total cost %f, got %f", expectedTotalCost, totalCost)
	}
}

func TestHealthCheck(t *testing.T) {
	config := &Config{
		Enabled:         true,
		DatabasePath:    ":memory:",
		BufferSize:      100,
		CleanupInterval: 24 * time.Hour,
		RetentionDays:   30,
	}
	
	tracker, err := NewUsageTracker(config)
	if err != nil {
		t.Fatalf("Failed to create usage tracker: %v", err)
	}
	defer tracker.Close()
	
	ctx := context.Background()
	
	// Test healthy state
	err = tracker.HealthCheck(ctx)
	if err != nil {
		t.Errorf("Expected healthy tracker, got error: %v", err)
	}
}

func TestHealthCheckDisabled(t *testing.T) {
	config := &Config{
		Enabled:         false,
		CleanupInterval: 24 * time.Hour,
		RetentionDays:   30,
	}
	
	tracker, err := NewUsageTracker(config)
	if err != nil {
		t.Fatalf("Failed to create disabled usage tracker: %v", err)
	}
	defer tracker.Close()
	
	ctx := context.Background()
	
	// Test that disabled tracker is considered healthy
	err = tracker.HealthCheck(ctx)
	if err != nil {
		t.Errorf("Expected disabled tracker to be healthy, got error: %v", err)
	}
}

// Helper function to calculate absolute difference
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}