package endpoint

import (
	"os"
	"testing"

	"cc-forwarder/config"
)

func TestDynamicTokenResolution(t *testing.T) {
	// Create a test configuration YAML content
	configContent := `
server:
  host: "localhost"
  port: 8080

health:
  check_interval: "30s"
  timeout: "5s"
  health_path: "/v1/models"

strategy:
  type: "priority"

global_timeout: "5m"

endpoints:
  # Group 1: main - first endpoint has token
  - name: "main-primary"
    url: "https://api1.example.com"
    group: "main"
    group-priority: 1
    priority: 1
    token: "sk-main-token"
    timeout: "30s"

  - name: "main-backup"
    url: "https://api2.example.com"
    priority: 2
    # No token - should resolve to "sk-main-token"
    timeout: "30s"

  # Group 2: backup - first endpoint has token
  - name: "backup-primary"
    url: "https://api3.example.com"
    group: "backup"
    group-priority: 2
    priority: 1
    token: "sk-backup-token"
    timeout: "45s"

  - name: "backup-secondary"
    url: "https://api4.example.com"
    priority: 2
    # No token - should resolve to "sk-backup-token"
    timeout: "45s"

  # Group 3: override - custom token scenario
  - name: "override-primary"
    url: "https://api5.example.com"
    group: "override"
    group-priority: 3
    priority: 1
    token: "sk-group-token"
    timeout: "60s"

  - name: "override-custom"
    url: "https://api6.example.com"
    priority: 2
    token: "sk-custom-token" # Has its own token
    timeout: "60s"

  # Group 4: no token in entire group
  - name: "notoken-primary"
    url: "https://api7.example.com"
    group: "notoken"
    group-priority: 4
    priority: 1
    # No token
    timeout: "90s"

  - name: "notoken-secondary"
    url: "https://api8.example.com"
    priority: 2
    # No token - should resolve to empty string
    timeout: "90s"
`

	// Create temporary file
	tmpFile, err := os.CreateTemp("", "test-dynamic-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(configContent); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
	tmpFile.Close()

	// Load configuration (this will call setDefaults internally)
	cfg, err := config.LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Create manager
	manager := NewManager(cfg)
	defer manager.Stop()

	// Test Group 1 (main): both endpoints should resolve to main token
	mainPrimary := manager.endpoints[0]
	token := manager.GetTokenForEndpoint(mainPrimary)
	if token != "sk-main-token" {
		t.Errorf("main-primary: expected resolved token 'sk-main-token', got '%s'", token)
	}

	mainBackup := manager.endpoints[1]
	token = manager.GetTokenForEndpoint(mainBackup)
	if token != "sk-main-token" {
		t.Errorf("main-backup: expected resolved token 'sk-main-token' (from group), got '%s'", token)
	}

	// Test Group 2 (backup): both endpoints should resolve to backup token
	backupPrimary := manager.endpoints[2]
	token = manager.GetTokenForEndpoint(backupPrimary)
	if token != "sk-backup-token" {
		t.Errorf("backup-primary: expected resolved token 'sk-backup-token', got '%s'", token)
	}

	backupSecondary := manager.endpoints[3]
	token = manager.GetTokenForEndpoint(backupSecondary)
	if token != "sk-backup-token" {
		t.Errorf("backup-secondary: expected resolved token 'sk-backup-token' (from group), got '%s'", token)
	}

	// Test Group 3 (override): primary uses group token, custom uses its own
	overridePrimary := manager.endpoints[4]
	token = manager.GetTokenForEndpoint(overridePrimary)
	if token != "sk-group-token" {
		t.Errorf("override-primary: expected resolved token 'sk-group-token', got '%s'", token)
	}

	overrideCustom := manager.endpoints[5]
	token = manager.GetTokenForEndpoint(overrideCustom)
	if token != "sk-custom-token" {
		t.Errorf("override-custom: expected resolved token 'sk-custom-token' (own token), got '%s'", token)
	}

	// Test Group 4 (notoken): both endpoints should resolve to empty
	notokenPrimary := manager.endpoints[6]
	token = manager.GetTokenForEndpoint(notokenPrimary)
	if token != "" {
		t.Errorf("notoken-primary: expected resolved token '' (no token), got '%s'", token)
	}

	notokenSecondary := manager.endpoints[7]
	token = manager.GetTokenForEndpoint(notokenSecondary)
	if token != "" {
		t.Errorf("notoken-secondary: expected resolved token '' (no group token), got '%s'", token)
	}

	// Verify group assignments
	if mainBackup.Config.Group != "main" {
		t.Errorf("main-backup: expected group 'main', got '%s'", mainBackup.Config.Group)
	}
	if backupSecondary.Config.Group != "backup" {
		t.Errorf("backup-secondary: expected group 'backup', got '%s'", backupSecondary.Config.Group)
	}
	if overrideCustom.Config.Group != "override" {
		t.Errorf("override-custom: expected group 'override', got '%s'", overrideCustom.Config.Group)
	}
	if notokenSecondary.Config.Group != "notoken" {
		t.Errorf("notoken-secondary: expected group 'notoken', got '%s'", notokenSecondary.Config.Group)
	}
}

func TestDynamicApiKeyResolution(t *testing.T) {
	// Create a test configuration YAML content
	configContent := `
server:
  host: "localhost"
  port: 8080

health:
  check_interval: "30s"
  timeout: "5s"
  health_path: "/v1/models"

strategy:
  type: "priority"

global_timeout: "5m"

endpoints:
  - name: "api-primary"
    url: "https://api1.example.com"
    group: "main"
    group-priority: 1
    priority: 1
    api-key: "api-key-123"
    timeout: "30s"

  - name: "api-backup"
    url: "https://api2.example.com"
    priority: 2
    # No api-key - should resolve to "api-key-123"
    timeout: "30s"
`

	// Create temporary file
	tmpFile, err := os.CreateTemp("", "test-apikey-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(configContent); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
	tmpFile.Close()

	// Load configuration (this will call setDefaults internally)
	cfg, err := config.LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Create manager
	manager := NewManager(cfg)
	defer manager.Stop()

	// Test API key resolution
	apiPrimary := manager.endpoints[0]
	apiKey := manager.GetApiKeyForEndpoint(apiPrimary)
	if apiKey != "api-key-123" {
		t.Errorf("api-primary: expected resolved api-key 'api-key-123', got '%s'", apiKey)
	}

	apiBackup := manager.endpoints[1]
	apiKey = manager.GetApiKeyForEndpoint(apiBackup)
	if apiKey != "api-key-123" {
		t.Errorf("api-backup: expected resolved api-key 'api-key-123' (from group), got '%s'", apiKey)
	}
}