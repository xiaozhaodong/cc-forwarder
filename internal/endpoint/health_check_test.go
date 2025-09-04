package endpoint

import (
	"cc-forwarder/config"
	"testing"
	"time"
)

func TestImmediateFailureMarking(t *testing.T) {
	// Create test endpoint
	endpoint := &Endpoint{
		Config: config.EndpointConfig{
			Name: "test-endpoint",
		},
		Status: EndpointStatus{
			Healthy: true, // Start as healthy
		},
	}

	// Create manager
	cfg := &config.Config{}
	manager := &Manager{config: cfg}

	// First failure should immediately mark as unhealthy
	manager.updateEndpointStatus(endpoint, false, 100*time.Millisecond)

	if endpoint.IsHealthy() {
		t.Error("Endpoint should be marked as unhealthy after first failure")
	}

	if endpoint.Status.ConsecutiveFails != 1 {
		t.Errorf("Expected ConsecutiveFails to be 1, got %d", endpoint.Status.ConsecutiveFails)
	}

	// Recovery should mark as healthy
	manager.updateEndpointStatus(endpoint, true, 50*time.Millisecond)

	if !endpoint.IsHealthy() {
		t.Error("Endpoint should be marked as healthy after recovery")
	}

	if endpoint.Status.ConsecutiveFails != 0 {
		t.Errorf("Expected ConsecutiveFails to be 0 after recovery, got %d", endpoint.Status.ConsecutiveFails)
	}
}

func TestConsecutiveFailureCounter(t *testing.T) {
	// Create test endpoint
	endpoint := &Endpoint{
		Config: config.EndpointConfig{
			Name: "test-endpoint",
		},
		Status: EndpointStatus{
			Healthy: true,
		},
	}

	// Create manager
	cfg := &config.Config{}
	manager := &Manager{config: cfg}

	// Multiple failures should increment counter
	for i := 1; i <= 5; i++ {
		manager.updateEndpointStatus(endpoint, false, 100*time.Millisecond)
		
		if endpoint.IsHealthy() {
			t.Errorf("Endpoint should be unhealthy after failure %d", i)
		}
		
		if endpoint.Status.ConsecutiveFails != i {
			t.Errorf("Expected ConsecutiveFails to be %d, got %d", i, endpoint.Status.ConsecutiveFails)
		}
	}
}