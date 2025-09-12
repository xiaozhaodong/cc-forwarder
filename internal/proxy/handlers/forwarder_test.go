package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
)

func TestForwarder_ForwardRequestToEndpoint(t *testing.T) {
	// 创建测试服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求头
		if r.Header.Get("Authorization") == "" {
			t.Errorf("Expected Authorization header to be set")
		}
		
		// 验证Host头
		expectedHost := r.Host
		if expectedHost == "" {
			t.Errorf("Expected Host header to be set")
		}
		
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	}))
	defer server.Close()

	// 创建配置
	cfg := &config.Config{}

	// 创建端点配置
	endpointConfig := config.EndpointConfig{
		Name:     "test-endpoint",
		URL:      server.URL,
		Token:    "test-token",
		Timeout:  30 * time.Second,
		Priority: 1,
	}

	// 创建端点管理器
	endpointManager := endpoint.NewManager(cfg)
	ep := &endpoint.Endpoint{Config: endpointConfig}

	// 创建Forwarder
	forwarder := NewForwarder(cfg, endpointManager)

	// 创建测试请求
	bodyBytes := []byte(`{"message": "test"}`)
	req := httptest.NewRequest("POST", "/test", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	// 执行转发
	ctx := context.Background()
	resp, err := forwarder.ForwardRequestToEndpoint(ctx, req, bodyBytes, ep)

	if err != nil {
		t.Fatalf("ForwardRequestToEndpoint failed: %v", err)
	}

	if resp == nil {
		t.Fatal("Expected response, got nil")
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// 验证响应体
	defer resp.Body.Close()
	body := make([]byte, 1024)
	n, _ := resp.Body.Read(body)
	responseBody := string(body[:n])

	if responseBody != "test response" {
		t.Errorf("Expected 'test response', got '%s'", responseBody)
	}
}

func TestForwarder_CopyHeaders(t *testing.T) {
	// 创建配置
	cfg := &config.Config{}

	// 创建端点配置
	endpointConfig := config.EndpointConfig{
		Name:    "test-endpoint",
		URL:     "https://api.example.com",
		Token:   "test-token",
		ApiKey:  "test-api-key",
		Headers: map[string]string{
			"X-Custom-Header": "custom-value",
		},
	}

	// 创建端点管理器
	endpointManager := endpoint.NewManager(cfg)
	ep := &endpoint.Endpoint{Config: endpointConfig}

	// 创建Forwarder
	forwarder := NewForwarder(cfg, endpointManager)

	// 创建源请求
	srcReq := httptest.NewRequest("POST", "/test", nil)
	srcReq.Header.Set("Content-Type", "application/json")
	srcReq.Header.Set("User-Agent", "Test-Client")
	srcReq.Header.Set("Authorization", "Bearer client-token") // 应该被覆盖
	srcReq.Header.Set("X-API-Key", "client-api-key")         // 应该被移除

	// 创建目标请求
	dstReq := httptest.NewRequest("POST", "https://api.example.com/test", nil)

	// 执行头部复制
	forwarder.CopyHeaders(srcReq, dstReq, ep)

	// 验证结果
	if dstReq.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type to be preserved")
	}

	if dstReq.Header.Get("User-Agent") != "Test-Client" {
		t.Errorf("Expected User-Agent to be preserved")
	}

	if dstReq.Header.Get("Authorization") != "Bearer test-token" {
		t.Errorf("Expected Authorization to be replaced with endpoint token")
	}

	if dstReq.Header.Get("X-Api-Key") != "test-api-key" {
		t.Errorf("Expected X-Api-Key to be set from endpoint config")
	}

	if dstReq.Header.Get("X-Custom-Header") != "custom-value" {
		t.Errorf("Expected custom header to be added")
	}

	if dstReq.Header.Get("Host") != "api.example.com" {
		t.Errorf("Expected Host header to be set correctly")
	}

	// 验证敏感头部被移除
	if dstReq.Header.Get("X-API-Key") == "client-api-key" {
		t.Errorf("Expected client X-API-Key to be removed")
	}
}