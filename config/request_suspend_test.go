package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"cc-forwarder/config"
)

// TestRequestSuspendConfig_Defaults tests default values for RequestSuspendConfig
func TestRequestSuspendConfig_Defaults(t *testing.T) {
	// Create a minimal config file that will trigger default value setting
	yamlContent := `
server:
  host: localhost
  port: 8080

endpoints:
  - name: test
    url: http://example.com
`
	
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config.yaml")
	err := os.WriteFile(configPath, []byte(yamlContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// 测试默认值
	if cfg.RequestSuspend.Enabled != false {
		t.Errorf("Expected RequestSuspend.Enabled to be false by default, got %v", cfg.RequestSuspend.Enabled)
	}

	if cfg.RequestSuspend.Timeout != 300*time.Second {
		t.Errorf("Expected RequestSuspend.Timeout to be 300s by default, got %v", cfg.RequestSuspend.Timeout)
	}

	if cfg.RequestSuspend.MaxSuspendedRequests != 100 {
		t.Errorf("Expected RequestSuspend.MaxSuspendedRequests to be 100 by default, got %d", cfg.RequestSuspend.MaxSuspendedRequests)
	}
}

// TestRequestSuspendConfig_Validation tests validation logic for RequestSuspendConfig
func TestRequestSuspendConfig_Validation(t *testing.T) {
	tests := []struct {
		name      string
		yamlContent string
		expectErr bool
		errMsg    string
	}{
		{
			name: "Valid disabled config",
			yamlContent: `
server:
  host: localhost
  port: 8080

request_suspend:
  enabled: false
  timeout: "0s"
  max_suspended_requests: 0

endpoints:
  - name: test
    url: http://example.com
`,
			expectErr: false,
		},
		{
			name: "Valid enabled config",
			yamlContent: `
server:
  host: localhost
  port: 8080

request_suspend:
  enabled: true
  timeout: "300s"
  max_suspended_requests: 100

endpoints:
  - name: test
    url: http://example.com
`,
			expectErr: false,
		},
		{
			name: "Enabled with zero timeout (gets default)",
			yamlContent: `
server:
  host: localhost
  port: 8080

request_suspend:
  enabled: true
  max_suspended_requests: 100

endpoints:
  - name: test
    url: http://example.com
`,
			expectErr: false, // No error because LoadConfig() will set defaults
		},
		{
			name: "Enabled with negative timeout",
			yamlContent: `
server:
  host: localhost
  port: 8080

request_suspend:
  enabled: true
  timeout: "-1s"
  max_suspended_requests: 100

endpoints:
  - name: test
    url: http://example.com
`,
			expectErr: true,
			errMsg:    "request suspend timeout must be greater than 0",
		},
		{
			name: "Enabled with zero max suspended requests (gets default)",
			yamlContent: `
server:
  host: localhost
  port: 8080

request_suspend:
  enabled: true
  timeout: "300s"

endpoints:
  - name: test
    url: http://example.com
`,
			expectErr: false, // No error because LoadConfig() will set defaults
		},
		{
			name: "Enabled with negative max suspended requests",
			yamlContent: `
server:
  host: localhost
  port: 8080

request_suspend:
  enabled: true
  timeout: "300s"
  max_suspended_requests: -1

endpoints:
  - name: test
    url: http://example.com
`,
			expectErr: true,
			errMsg:    "max suspended requests must be greater than 0",
		},
		{
			name: "Enabled with excessive max suspended requests",
			yamlContent: `
server:
  host: localhost
  port: 8080

request_suspend:
  enabled: true
  timeout: "300s"
  max_suspended_requests: 20000

endpoints:
  - name: test
    url: http://example.com
`,
			expectErr: true,
			errMsg:    "max suspended requests cannot exceed 10000",
		},
		{
			name: "Boundary test - max allowed requests",
			yamlContent: `
server:
  host: localhost
  port: 8080

request_suspend:
  enabled: true
  timeout: "300s"
  max_suspended_requests: 10000

endpoints:
  - name: test
    url: http://example.com
`,
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "test_config.yaml")
			
			err := os.WriteFile(configPath, []byte(tt.yamlContent), 0644)
			if err != nil {
				t.Fatalf("Failed to create test config file: %v", err)
			}

			_, err = config.LoadConfig(configPath)
			if tt.expectErr {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("Expected error message containing '%s', got: %s", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

// TestRequestSuspendConfig_ConfigFileParsing tests parsing RequestSuspend from YAML config file
func TestRequestSuspendConfig_ConfigFileParsing(t *testing.T) {
	tests := []struct {
		name           string
		yamlContent    string
		expectedConfig config.RequestSuspendConfig
		expectErr      bool
	}{
		{
			name: "Complete request_suspend config",
			yamlContent: `
server:
  host: localhost
  port: 8080

request_suspend:
  enabled: true
  timeout: "600s"
  max_suspended_requests: 200

endpoints:
  - name: test
    url: http://example.com
`,
			expectedConfig: config.RequestSuspendConfig{
				Enabled:            true,
				Timeout:            600 * time.Second,
				MaxSuspendedRequests: 200,
			},
			expectErr: false,
		},
		{
			name: "Partial request_suspend config (only enabled)",
			yamlContent: `
server:
  host: localhost
  port: 8080

request_suspend:
  enabled: true

endpoints:
  - name: test
    url: http://example.com
`,
			expectedConfig: config.RequestSuspendConfig{
				Enabled:            true,
				Timeout:            300 * time.Second, // default
				MaxSuspendedRequests: 100,             // default
			},
			expectErr: false,
		},
		{
			name: "No request_suspend config (uses defaults)",
			yamlContent: `
server:
  host: localhost
  port: 8080

endpoints:
  - name: test
    url: http://example.com
`,
			expectedConfig: config.RequestSuspendConfig{
				Enabled:            false,            // default
				Timeout:            300 * time.Second, // default
				MaxSuspendedRequests: 100,             // default
			},
			expectErr: false,
		},
		{
			name: "Disabled request_suspend config",
			yamlContent: `
server:
  host: localhost
  port: 8080

request_suspend:
  enabled: false
  timeout: "120s"
  max_suspended_requests: 50

endpoints:
  - name: test
    url: http://example.com
`,
			expectedConfig: config.RequestSuspendConfig{
				Enabled:            false,
				Timeout:            120 * time.Second,
				MaxSuspendedRequests: 50,
			},
			expectErr: false,
		},
		{
			name: "Invalid timeout format",
			yamlContent: `
server:
  host: localhost
  port: 8080

request_suspend:
  enabled: true
  timeout: "invalid"
  max_suspended_requests: 100

endpoints:
  - name: test
    url: http://example.com
`,
			expectedConfig: config.RequestSuspendConfig{},
			expectErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建临时配置文件
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "test_config.yaml")

			err := os.WriteFile(configPath, []byte(tt.yamlContent), 0644)
			if err != nil {
				t.Fatalf("Failed to create test config file: %v", err)
			}

			// 加载配置
			cfg, err := config.LoadConfig(configPath)
			if tt.expectErr {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error loading config: %v", err)
				return
			}

			// 验证配置解析结果
			if cfg.RequestSuspend.Enabled != tt.expectedConfig.Enabled {
				t.Errorf("Expected Enabled=%v, got %v", tt.expectedConfig.Enabled, cfg.RequestSuspend.Enabled)
			}
			if cfg.RequestSuspend.Timeout != tt.expectedConfig.Timeout {
				t.Errorf("Expected Timeout=%v, got %v", tt.expectedConfig.Timeout, cfg.RequestSuspend.Timeout)
			}
			if cfg.RequestSuspend.MaxSuspendedRequests != tt.expectedConfig.MaxSuspendedRequests {
				t.Errorf("Expected MaxSuspendedRequests=%d, got %d", tt.expectedConfig.MaxSuspendedRequests, cfg.RequestSuspend.MaxSuspendedRequests)
			}
		})
	}
}

// TestRequestSuspendConfig_EdgeCases tests edge cases for RequestSuspendConfig
func TestRequestSuspendConfig_EdgeCases(t *testing.T) {
	t.Run("Very small timeout", func(t *testing.T) {
		yamlContent := `
server:
  host: localhost
  port: 8080

request_suspend:
  enabled: true
  timeout: "1ms"
  max_suspended_requests: 1

endpoints:
  - name: test
    url: http://example.com
`
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "test_config.yaml")
		err := os.WriteFile(configPath, []byte(yamlContent), 0644)
		if err != nil {
			t.Fatalf("Failed to create test config file: %v", err)
		}

		_, err = config.LoadConfig(configPath)
		if err != nil {
			t.Errorf("Very small timeout should be valid, but got error: %v", err)
		}
	})

	t.Run("Very large timeout", func(t *testing.T) {
		yamlContent := `
server:
  host: localhost
  port: 8080

request_suspend:
  enabled: true
  timeout: "24h"
  max_suspended_requests: 1

endpoints:
  - name: test
    url: http://example.com
`
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "test_config.yaml")
		err := os.WriteFile(configPath, []byte(yamlContent), 0644)
		if err != nil {
			t.Fatalf("Failed to create test config file: %v", err)
		}

		_, err = config.LoadConfig(configPath)
		if err != nil {
			t.Errorf("Very large timeout should be valid, but got error: %v", err)
		}
	})

	t.Run("Minimum valid configuration", func(t *testing.T) {
		yamlContent := `
server:
  host: localhost
  port: 8080

request_suspend:
  enabled: true
  timeout: "1ns"
  max_suspended_requests: 1

endpoints:
  - name: test
    url: http://example.com
`
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "test_config.yaml")
		err := os.WriteFile(configPath, []byte(yamlContent), 0644)
		if err != nil {
			t.Fatalf("Failed to create test config file: %v", err)
		}

		_, err = config.LoadConfig(configPath)
		if err != nil {
			t.Errorf("Minimum valid configuration should be valid, but got error: %v", err)
		}
	})
}

// Benchmark tests for configuration operations
func BenchmarkRequestSuspendConfig_LoadConfig(b *testing.B) {
	yamlContent := `
server:
  host: localhost
  port: 8080

request_suspend:
  enabled: true
  timeout: "300s"
  max_suspended_requests: 100

endpoints:
  - name: test
    url: http://example.com
`
	tmpDir := b.TempDir()
	configPath := filepath.Join(tmpDir, "test_config.yaml")
	err := os.WriteFile(configPath, []byte(yamlContent), 0644)
	if err != nil {
		b.Fatalf("Failed to create test config file: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = config.LoadConfig(configPath)
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(s) > len(substr) && stringContains(s, substr)))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}