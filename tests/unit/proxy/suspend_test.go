package proxy_test

import (
	"testing"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/proxy"
)

// TestRetryHandler_GetSuspendedRequestsCount tests suspended requests count management
func TestRetryHandler_GetSuspendedRequestsCount(t *testing.T) {
	cfg := &config.Config{
		RequestSuspend: config.RequestSuspendConfig{
			Enabled:            true,
			Timeout:            300 * time.Second,
			MaxSuspendedRequests: 100,
		},
	}

	rh := proxy.NewRetryHandler(cfg)

	// Test initial count
	if count := rh.GetSuspendedRequestsCount(); count != 0 {
		t.Errorf("Expected initial suspended requests count to be 0, got %d", count)
	}

	// Note: We cannot test setting the count directly since it's an internal field
	// and there's no public API to modify it. The count would be managed internally
	// during actual request processing.
}

// TestRetryHandler_UpdateConfig tests configuration updates
func TestRetryHandler_UpdateConfig(t *testing.T) {
	initialConfig := &config.Config{
		RequestSuspend: config.RequestSuspendConfig{
			Enabled:            false,
			Timeout:            300 * time.Second,
			MaxSuspendedRequests: 100,
		},
	}

	rh := proxy.NewRetryHandler(initialConfig)

	// Update config
	newConfig := &config.Config{
		RequestSuspend: config.RequestSuspendConfig{
			Enabled:            true,
			Timeout:            600 * time.Second,
			MaxSuspendedRequests: 200,
		},
	}

	// This should not panic
	rh.UpdateConfig(newConfig)

	// Note: We cannot verify the internal config was updated since the config field
	// is not exposed. In a real scenario, the behavior would be tested through 
	// actual request processing.
}

// TestRetryHandler_EdgeCases tests edge cases for suspend functionality
func TestRetryHandler_EdgeCases(t *testing.T) {
	t.Run("Nil config", func(t *testing.T) {
		// This should not panic
		rh := proxy.NewRetryHandler(nil)
		if rh == nil {
			t.Error("NewRetryHandler should not return nil even with nil config")
		}
	})

	t.Run("Zero timeout config", func(t *testing.T) {
		cfg := &config.Config{
			RequestSuspend: config.RequestSuspendConfig{
				Enabled:            true,
				Timeout:            0,
				MaxSuspendedRequests: 100,
			},
		}

		rh := proxy.NewRetryHandler(cfg)
		if rh == nil {
			t.Error("NewRetryHandler should not return nil with zero timeout")
		}
	})

	t.Run("Update with nil config", func(t *testing.T) {
		cfg := &config.Config{
			RequestSuspend: config.RequestSuspendConfig{
				Enabled:            true,
				Timeout:            300 * time.Second,
				MaxSuspendedRequests: 100,
			},
		}

		rh := proxy.NewRetryHandler(cfg)

		// This should not panic
		rh.UpdateConfig(nil)
	})
}

// TestRetryHandler_IsRetryableError tests error classification
func TestRetryHandler_IsRetryableError(t *testing.T) {
	cfg := &config.Config{
		RequestSuspend: config.RequestSuspendConfig{
			Enabled:            true,
			Timeout:            300 * time.Second,
			MaxSuspendedRequests: 100,
		},
	}

	rh := proxy.NewRetryHandler(cfg)

	// Test some basic error conditions
	// Note: The actual implementation details of IsRetryableError would determine
	// what specific errors are considered retryable

	// Test with nil error
	if rh.IsRetryableError(nil) {
		t.Error("nil error should not be retryable")
	}
}

// Benchmark tests for public methods
func BenchmarkRetryHandler_GetSuspendedRequestsCount(b *testing.B) {
	cfg := &config.Config{
		RequestSuspend: config.RequestSuspendConfig{
			Enabled:            true,
			Timeout:            300 * time.Second,
			MaxSuspendedRequests: 100,
		},
	}

	rh := proxy.NewRetryHandler(cfg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rh.GetSuspendedRequestsCount()
	}
}

func BenchmarkRetryHandler_UpdateConfig(b *testing.B) {
	cfg := &config.Config{
		RequestSuspend: config.RequestSuspendConfig{
			Enabled:            false,
			Timeout:            300 * time.Second,
			MaxSuspendedRequests: 100,
		},
	}

	rh := proxy.NewRetryHandler(cfg)

	newConfig := &config.Config{
		RequestSuspend: config.RequestSuspendConfig{
			Enabled:            true,
			Timeout:            600 * time.Second,
			MaxSuspendedRequests: 200,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rh.UpdateConfig(newConfig)
	}
}