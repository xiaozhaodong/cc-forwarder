package tracking

import (
	"testing"
	"time"
)

func TestConfig_Defaults(t *testing.T) {
	config := &Config{}
	
	// Test that NewUsageTracker sets defaults correctly
	tracker, err := NewUsageTracker(config)
	if err != nil {
		t.Fatalf("Failed to create usage tracker with nil config: %v", err)
	}
	defer tracker.Close()
	
	if tracker.config == nil {
		t.Fatal("Config should not be nil after initialization")
	}
	
	if !config.Enabled {
		// When disabled, should return successfully
		return
	}
	
	// Test default values
	if config.BufferSize != 1000 {
		t.Errorf("Expected BufferSize to be 1000, got %d", config.BufferSize)
	}
	
	if config.BatchSize != 100 {
		t.Errorf("Expected BatchSize to be 100, got %d", config.BatchSize)
	}
	
	if config.FlushInterval != 30*time.Second {
		t.Errorf("Expected FlushInterval to be 30s, got %v", config.FlushInterval)
	}
	
	if config.MaxRetry != 3 {
		t.Errorf("Expected MaxRetry to be 3, got %d", config.MaxRetry)
	}
}

func TestConfig_Enabled(t *testing.T) {
	config := &Config{
		Enabled:         true,
		DatabasePath:    ":memory:",
		BufferSize:      500,
		BatchSize:       50,
		FlushInterval:   10 * time.Second,
		MaxRetry:        5,
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
		t.Fatalf("Failed to create enabled usage tracker: %v", err)
	}
	defer tracker.Close()
	
	// Test that custom values are preserved
	if config.BufferSize != 500 {
		t.Errorf("Expected BufferSize to be 500, got %d", config.BufferSize)
	}
	
	if config.BatchSize != 50 {
		t.Errorf("Expected BatchSize to be 50, got %d", config.BatchSize)
	}
	
	if config.FlushInterval != 10*time.Second {
		t.Errorf("Expected FlushInterval to be 10s, got %v", config.FlushInterval)
	}
	
	if config.MaxRetry != 5 {
		t.Errorf("Expected MaxRetry to be 5, got %d", config.MaxRetry)
	}
}

func TestConfig_Disabled(t *testing.T) {
	config := &Config{
		Enabled: false,
	}
	
	tracker, err := NewUsageTracker(config)
	if err != nil {
		t.Fatalf("Failed to create disabled usage tracker: %v", err)
	}
	defer tracker.Close()
	
	if tracker.db != nil {
		t.Error("Database should be nil when tracking is disabled")
	}
	
	if tracker.eventChan != nil {
		t.Error("Event channel should be nil when tracking is disabled")
	}
}

func TestModelPricing(t *testing.T) {
	config := &Config{
		Enabled:         true,
		DatabasePath:    ":memory:",
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
	
	// Test getting specific model pricing
	pricing := tracker.GetPricing("claude-3-5-haiku-20241022")
	if pricing.Input != 1.00 {
		t.Errorf("Expected input pricing 1.00, got %f", pricing.Input)
	}
	if pricing.Output != 5.00 {
		t.Errorf("Expected output pricing 5.00, got %f", pricing.Output)
	}
	if pricing.CacheCreation != 1.25 {
		t.Errorf("Expected cache creation pricing 1.25, got %f", pricing.CacheCreation)
	}
	if pricing.CacheRead != 0.10 {
		t.Errorf("Expected cache read pricing 0.10, got %f", pricing.CacheRead)
	}
	
	// Test getting default pricing for unknown model
	defaultPricing := tracker.GetPricing("unknown-model")
	if defaultPricing.Input != 2.00 {
		t.Errorf("Expected default input pricing 2.00, got %f", defaultPricing.Input)
	}
	if defaultPricing.Output != 10.00 {
		t.Errorf("Expected default output pricing 10.00, got %f", defaultPricing.Output)
	}
	if defaultPricing.CacheCreation != 2.50 {
		t.Errorf("Expected default cache creation pricing 2.50, got %f", defaultPricing.CacheCreation)
	}
	if defaultPricing.CacheRead != 0.20 {
		t.Errorf("Expected default cache read pricing 0.20, got %f", defaultPricing.CacheRead)
	}
}

func TestUpdatePricing(t *testing.T) {
	config := &Config{
		Enabled:         true,
		DatabasePath:    ":memory:",
		CleanupInterval: 24 * time.Hour,
		RetentionDays:   30,
		ModelPricing: map[string]ModelPricing{
			"old-model": {
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
	
	// Test initial pricing
	pricing := tracker.GetPricing("old-model")
	if pricing.Input != 1.00 {
		t.Errorf("Expected initial input pricing 1.00, got %f", pricing.Input)
	}
	
	// Update pricing
	newPricing := map[string]ModelPricing{
		"new-model": {
			Input:         3.00,
			Output:        15.00,
			CacheCreation: 3.75,
			CacheRead:     0.30,
		},
	}
	
	tracker.UpdatePricing(newPricing)
	
	// Test that old model now uses default pricing
	oldModelPricing := tracker.GetPricing("old-model")
	if oldModelPricing.Input == 1.00 {
		t.Error("Old model should now use default pricing")
	}
	
	// Test new model pricing
	newModelPricing := tracker.GetPricing("new-model")
	if newModelPricing.Input != 3.00 {
		t.Errorf("Expected new input pricing 3.00, got %f", newModelPricing.Input)
	}
	if newModelPricing.Output != 15.00 {
		t.Errorf("Expected new output pricing 15.00, got %f", newModelPricing.Output)
	}
}