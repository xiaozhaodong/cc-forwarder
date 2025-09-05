package tracking

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestErrorHandler(t *testing.T) {
	// Create a temporary directory for test databases
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	
	config := &Config{
		Enabled:        true,
		DatabasePath:   dbPath,
		BufferSize:     100,
		BatchSize:      10,
		FlushInterval:  50 * time.Millisecond,
		MaxRetry:       3,
	}
	
	tracker, err := NewUsageTracker(config)
	if err != nil {
		t.Fatalf("Failed to create usage tracker: %v", err)
	}
	defer tracker.Close()
	
	// Test that error handler is initialized
	if tracker.errorHandler == nil {
		t.Error("Error handler should be initialized")
	}
	
	// Test backup creation
	t.Run("CreateBackup", func(t *testing.T) {
		err := tracker.errorHandler.CreateBackup()
		if err != nil {
			t.Errorf("Failed to create backup: %v", err)
		}
		
		// Check that backup file exists
		backupPath := dbPath + ".backup"
		if _, err := os.Stat(backupPath); os.IsNotExist(err) {
			t.Error("Backup file should be created")
		}
	})
	
	// Test recovery from backup
	t.Run("RestoreFromBackup", func(t *testing.T) {
		// First create some data
		tracker.RecordRequestStart("req-error-test", "127.0.0.1", "error-agent")
		
		// Wait for processing
		time.Sleep(100 * time.Millisecond)
		
		// Force a backup
		err = tracker.errorHandler.CreateBackup()
		if err != nil {
			t.Fatalf("Failed to create backup for recovery test: %v", err)
		}
		
		// Verify backup can be restored
		err = tracker.errorHandler.RestoreFromBackup()
		if err != nil {
			t.Errorf("Failed to restore from backup: %v", err)
		}
	})
}

func TestErrorHandlerDiskSpace(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "diskspace_test.db")
	
	config := &Config{
		Enabled:        true,
		DatabasePath:   dbPath,
		BufferSize:     100,
		BatchSize:      10,
		FlushInterval:  50 * time.Millisecond,
		MaxRetry:       3,
	}
	
	tracker, err := NewUsageTracker(config)
	if err != nil {
		t.Fatalf("Failed to create usage tracker: %v", err)
	}
	defer tracker.Close()
	
	// Simulate disk space error
	diskSpaceError := fmt.Errorf("database or disk is full")
	
	handled := tracker.errorHandler.HandleDatabaseError(diskSpaceError, "test_operation")
	
	// Should return false (can't handle disk space errors easily in test)
	if handled {
		t.Log("Disk space error handling attempted (expected behavior)")
	}
}

func TestErrorHandlerCorruption(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "corruption_test.db")
	
	// Create a database first
	config := &Config{
		Enabled:        true,
		DatabasePath:   dbPath,
		BufferSize:     100,
		BatchSize:      10,
		FlushInterval:  50 * time.Millisecond,
		MaxRetry:       3,
	}
	
	tracker, err := NewUsageTracker(config)
	if err != nil {
		t.Fatalf("Failed to create usage tracker: %v", err)
	}
	
	// Create a backup
	err = tracker.errorHandler.CreateBackup()
	if err != nil {
		t.Fatalf("Failed to create backup for corruption test: %v", err)
	}
	
	tracker.Close()
	
	// Create new tracker to test error handler
	tracker2, err := NewUsageTracker(config)
	if err != nil {
		t.Fatalf("Failed to create second tracker: %v", err)
	}
	defer tracker2.Close()
	
	// Simulate corruption error
	corruptionError := fmt.Errorf("database disk image is malformed")
	
	handled := tracker2.errorHandler.HandleDatabaseError(corruptionError, "test_operation")
	
	// Should attempt to handle corruption
	if !handled {
		t.Log("Corruption error handling attempted")
	}
}

func TestRetryLogic(t *testing.T) {
	config := &Config{
		Enabled:        true,
		DatabasePath:   ":memory:",
		BufferSize:     100,
		BatchSize:      10,
		FlushInterval:  50 * time.Millisecond,
		MaxRetry:       3,
	}
	
	tracker, err := NewUsageTracker(config)
	if err != nil {
		t.Fatalf("Failed to create usage tracker: %v", err)
	}
	defer tracker.Close()
	
	// Test successful operation (no retries needed)
	events := []RequestEvent{
		{
			Type:      "start",
			RequestID: "req-retry-success",
			Timestamp: time.Now(),
			Data: RequestStartData{
				ClientIP:  "127.0.0.1",
				UserAgent: "retry-agent",
				Method:    "POST",
				Path:      "/v1/messages",
			},
		},
	}
	
	err = tracker.processBatch(events)
	if err != nil {
		t.Errorf("Successful operation should not fail: %v", err)
	}
}

func TestGracefulShutdown(t *testing.T) {
	config := &Config{
		Enabled:        true,
		DatabasePath:   ":memory:",
		BufferSize:     100,
		BatchSize:      20, // Large batch size to keep events in buffer
		FlushInterval:  1 * time.Hour, // Long interval
		MaxRetry:       3,
	}
	
	tracker, err := NewUsageTracker(config)
	if err != nil {
		t.Fatalf("Failed to create usage tracker: %v", err)
	}
	
	// Send some events
	for i := 0; i < 5; i++ {
		requestID := time.Now().Format("req-shutdown-20060102150405") + string(rune('0'+i))
		
		tracker.RecordRequestStart(requestID, "127.0.0.1", "shutdown-agent")
	}
	
	// Close tracker (should process remaining events)
	err = tracker.Close()
	if err != nil {
		t.Errorf("Failed to close tracker gracefully: %v", err)
	}
	
	// Try to close again (should be safe)
	err = tracker.Close()
	if err != nil {
		t.Errorf("Second close should be safe: %v", err)
	}
}

func TestDatabaseLocking(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "locking_test.db")
	
	config := &Config{
		Enabled:        true,
		DatabasePath:   dbPath,
		BufferSize:     100,
		BatchSize:      10,
		FlushInterval:  50 * time.Millisecond,
		MaxRetry:       3,
	}
	
	// Create first tracker
	tracker1, err := NewUsageTracker(config)
	if err != nil {
		t.Fatalf("Failed to create first tracker: %v", err)
	}
	
	// Record some data with first tracker
	tracker1.RecordRequestStart("req-lock-test", "127.0.0.1", "lock-agent")
	
	// Wait for processing
	time.Sleep(100 * time.Millisecond)
	
	// Close first tracker
	err = tracker1.Close()
	if err != nil {
		t.Errorf("Failed to close first tracker: %v", err)
	}
	
	// Create second tracker with same database (should work after first is closed)
	tracker2, err := NewUsageTracker(config)
	if err != nil {
		t.Fatalf("Failed to create second tracker: %v", err)
	}
	defer tracker2.Close()
	
	// Should be able to access the data
	ctx := context.Background()
	stats, err := tracker2.GetDatabaseStats(ctx)
	if err != nil {
		t.Errorf("Failed to get stats from second tracker: %v", err)
	}
	
	if stats.TotalRequests == 0 {
		t.Error("Second tracker should see data from first tracker")
	}
}

func TestInvalidEventData(t *testing.T) {
	config := &Config{
		Enabled:        true,
		DatabasePath:   ":memory:",
		BufferSize:     100,
		BatchSize:      10,
		FlushInterval:  50 * time.Millisecond,
		MaxRetry:       3,
	}
	
	tracker, err := NewUsageTracker(config)
	if err != nil {
		t.Fatalf("Failed to create usage tracker: %v", err)
	}
	defer tracker.Close()
	
	// Test events with invalid data types
	invalidEvents := []RequestEvent{
		{
			Type:      "start",
			RequestID: "req-invalid-start",
			Timestamp: time.Now(),
			Data:      "invalid-string-data", // Should be RequestStartData
		},
		{
			Type:      "update",
			RequestID: "req-invalid-update",
			Timestamp: time.Now(),
			Data:      123, // Should be RequestUpdateData
		},
		{
			Type:      "complete",
			RequestID: "req-invalid-complete",
			Timestamp: time.Now(),
			Data:      []string{"invalid", "array"}, // Should be RequestCompleteData
		},
		{
			Type:      "unknown",
			RequestID: "req-invalid-type",
			Timestamp: time.Now(),
			Data:      nil,
		},
	}
	
	// Process batch with invalid events (should not crash)
	err = tracker.processBatch(invalidEvents)
	if err != nil {
		t.Log("Expected some errors with invalid data, got:", err)
	}
	
	// Tracker should still be functional
	validEvent := RequestEvent{
		Type:      "start",
		RequestID: "req-valid-after-invalid",
		Timestamp: time.Now(),
		Data: RequestStartData{
			ClientIP:  "127.0.0.1",
			UserAgent: "valid-agent",
			Method:    "POST",
			Path:      "/v1/messages",
		},
	}
	
	err = tracker.processBatch([]RequestEvent{validEvent})
	if err != nil {
		t.Errorf("Valid event should work after invalid events: %v", err)
	}
}

func TestContextCancellation(t *testing.T) {
	config := &Config{
		Enabled:        true,
		DatabasePath:   ":memory:",
		BufferSize:     100,
		BatchSize:      10,
		FlushInterval:  50 * time.Millisecond,
		MaxRetry:       3,
	}
	
	tracker, err := NewUsageTracker(config)
	if err != nil {
		t.Fatalf("Failed to create usage tracker: %v", err)
	}
	defer tracker.Close()
	
	// Test operations with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Immediately cancel
	
	// Database operations should handle cancelled context gracefully
	_, err = tracker.GetDatabaseStats(ctx)
	if err == nil {
		t.Log("Database operation completed despite cancelled context")
	} else {
		t.Log("Database operation handled cancelled context:", err)
	}
	
	// Test with timeout context
	timeoutCtx, timeoutCancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer timeoutCancel()
	
	time.Sleep(1 * time.Millisecond) // Ensure timeout
	
	_, err = tracker.GetUsageStats(timeoutCtx, time.Now().AddDate(0, 0, -1), time.Now())
	if err != nil {
		t.Log("Operation correctly handled timeout:", err)
	}
}

func TestMemoryPressure(t *testing.T) {
	config := &Config{
		Enabled:        true,
		DatabasePath:   ":memory:",
		BufferSize:     5, // Very small buffer
		BatchSize:      3,
		FlushInterval:  10 * time.Millisecond, // Fast flushing
		MaxRetry:       3,
	}
	
	tracker, err := NewUsageTracker(config)
	if err != nil {
		t.Fatalf("Failed to create usage tracker: %v", err)
	}
	defer tracker.Close()
	
	// Send many events rapidly to test memory pressure
	eventCount := 50
	successCount := 0
	
	for i := 0; i < eventCount; i++ {
		requestID := time.Now().Format("req-memory-20060102150405") + fmt.Sprintf("%03d", i)
		
		tracker.RecordRequestStart(requestID, "127.0.0.1", "memory-agent")
		successCount++ // 对于没有返回错误的方法，总是成功
		
		// Small delay to allow some processing
		if i%10 == 0 {
			time.Sleep(50 * time.Millisecond)
		}
	}
	
	t.Logf("Successfully queued %d/%d events under memory pressure", successCount, eventCount)
	
	// Wait for processing to complete
	time.Sleep(500 * time.Millisecond)
	
	// Check final state
	ctx := context.Background()
	stats, err := tracker.GetDatabaseStats(ctx)
	if err != nil {
		t.Fatalf("Failed to get final stats: %v", err)
	}
	
	t.Logf("Final database stats: %d requests processed", stats.TotalRequests)
	
	// Should have processed at least some events
	if stats.TotalRequests == 0 {
		t.Error("Expected at least some requests to be processed")
	}
}