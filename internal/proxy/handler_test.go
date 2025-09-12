package proxy

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
)

func TestSensitiveHeaderRemoval(t *testing.T) {
	// Create a test endpoint config
	cfg := &config.Config{
		Endpoints: []config.EndpointConfig{
			{
				Name:     "test-endpoint",
				URL:      "https://api.example.com",
				Priority: 1,
				Timeout:  30 * time.Second,
				Token:    "endpoint-token",
			},
		},
	}

	// Create endpoint manager
	endpointManager := endpoint.NewManager(cfg)

	// Create proxy handler
	handler := NewHandler(endpointManager, cfg)

	// Create a test HTTP request with sensitive headers
	originalBody := `{"test": "data"}`
	req := httptest.NewRequest("POST", "/v1/messages", bytes.NewBufferString(originalBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Host", "localhost:8080")
	req.Header.Set("User-Agent", "Test-Client/1.0")
	req.Header.Set("X-API-Key", "client-api-key-12345")        // Should be removed
	req.Header.Set("Authorization", "Bearer client-token")     // Should be removed
	req.Header.Set("X-Custom-Header", "should-be-preserved")   // Should be kept

	// Test the copyHeaders function
	targetURL := "https://api.example.com/v1/messages"
	targetReq, err := http.NewRequest("POST", targetURL, bytes.NewBufferString(originalBody))
	if err != nil {
		t.Fatalf("Failed to create target request: %v", err)
	}

	ep := endpointManager.GetAllEndpoints()[0]
	handler.forwarder.CopyHeaders(req, targetReq, ep)

	// Verify sensitive headers are removed
	if targetReq.Header.Get("X-API-Key") != "" {
		t.Errorf("Expected X-API-Key header to be removed, but it's present: %s", targetReq.Header.Get("X-API-Key"))
	}

	// Verify Authorization is replaced with endpoint token
	expectedAuth := "Bearer endpoint-token"
	if targetReq.Header.Get("Authorization") != expectedAuth {
		t.Errorf("Expected Authorization '%s', got '%s'", expectedAuth, targetReq.Header.Get("Authorization"))
	}

	// Verify Host header is set correctly
	expectedHost := "api.example.com"
	if targetReq.Header.Get("Host") != expectedHost {
		t.Errorf("Expected Host header '%s', got '%s'", expectedHost, targetReq.Header.Get("Host"))
	}

	// Verify non-sensitive headers are preserved
	if targetReq.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type to be preserved")
	}

	if targetReq.Header.Get("User-Agent") != "Test-Client/1.0" {
		t.Errorf("Expected User-Agent to be preserved")
	}

	if targetReq.Header.Get("X-Custom-Header") != "should-be-preserved" {
		t.Errorf("Expected X-Custom-Header to be preserved")
	}
}

func TestHostHeaderOverride(t *testing.T) {
	// Create a test endpoint config
	cfg := &config.Config{
		Endpoints: []config.EndpointConfig{
			{
				Name:     "test-endpoint",
				URL:      "https://api.example.com",
				Priority: 1,
				Timeout:  30 * time.Second,
				Token:    "test-token",
				Headers: map[string]string{
					"X-Custom": "custom-value",
				},
			},
		},
	}

	// Create endpoint manager
	endpointManager := endpoint.NewManager(cfg)

	// Create proxy handler
	handler := NewHandler(endpointManager, cfg)

	// Create a test HTTP request with original Host header
	originalBody := `{"test": "data"}`
	req := httptest.NewRequest("POST", "/v1/messages", bytes.NewBufferString(originalBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Host", "localhost:8080")  // Original host
	req.Header.Set("User-Agent", "Test-Client/1.0")
	req.Header.Set("X-Original", "original-value")

	// Test the copyHeaders function
	targetURL := "https://api.example.com/v1/messages"
	targetReq, err := http.NewRequest("POST", targetURL, bytes.NewBufferString(originalBody))
	if err != nil {
		t.Fatalf("Failed to create target request: %v", err)
	}

	ep := endpointManager.GetAllEndpoints()[0]
	handler.forwarder.CopyHeaders(req, targetReq, ep)

	// Verify Host header is set to target endpoint's host
	expectedHost := "api.example.com"
	if targetReq.Header.Get("Host") != expectedHost {
		t.Errorf("Expected Host header '%s', got '%s'", expectedHost, targetReq.Header.Get("Host"))
	}

	// Verify Host field is also set
	if targetReq.Host != expectedHost {
		t.Errorf("Expected Host field '%s', got '%s'", expectedHost, targetReq.Host)
	}

	// Verify other headers are copied correctly
	if targetReq.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type to be copied")
	}

	if targetReq.Header.Get("User-Agent") != "Test-Client/1.0" {
		t.Errorf("Expected User-Agent to be copied")
	}

	if targetReq.Header.Get("X-Original") != "original-value" {
		t.Errorf("Expected X-Original to be copied")
	}

	// Verify Authorization token is added
	expectedAuth := "Bearer test-token"
	if targetReq.Header.Get("Authorization") != expectedAuth {
		t.Errorf("Expected Authorization '%s', got '%s'", expectedAuth, targetReq.Header.Get("Authorization"))
	}

	// Verify custom headers are added
	if targetReq.Header.Get("X-Custom") != "custom-value" {
		t.Errorf("Expected X-Custom header to be set")
	}
}

func TestHostHeaderWithDifferentPorts(t *testing.T) {
	testCases := []struct {
		endpointURL  string
		expectedHost string
	}{
		{"https://api.example.com:443", "api.example.com:443"},
		{"http://localhost:3000", "localhost:3000"},
		{"https://custom.domain.com:8443", "custom.domain.com:8443"},
		{"http://192.168.1.100:8080", "192.168.1.100:8080"},
	}

	for _, tc := range testCases {
		t.Run(tc.endpointURL, func(t *testing.T) {
			cfg := &config.Config{
				Endpoints: []config.EndpointConfig{
					{
						Name:     "test-endpoint",
						URL:      tc.endpointURL,
						Priority: 1,
						Timeout:  30 * time.Second,
					},
				},
			}

			endpointManager := endpoint.NewManager(cfg)
			handler := NewHandler(endpointManager, cfg)

			// Create test request
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("Host", "original-host.com")

			// Create target request
			targetReq, _ := http.NewRequest("GET", tc.endpointURL+"/test", nil)

			ep := endpointManager.GetAllEndpoints()[0]
			handler.forwarder.CopyHeaders(req, targetReq, ep)

			if targetReq.Header.Get("Host") != tc.expectedHost {
				t.Errorf("Expected Host header '%s', got '%s'", tc.expectedHost, targetReq.Header.Get("Host"))
			}

			if targetReq.Host != tc.expectedHost {
				t.Errorf("Expected Host field '%s', got '%s'", tc.expectedHost, targetReq.Host)
			}
		})
	}
}