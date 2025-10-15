package handlers

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/transport"
)

// Forwarder 负责HTTP请求转发和头部处理
type Forwarder struct {
	config          *config.Config
	endpointManager *endpoint.Manager
}

// NewForwarder 创建新的Forwarder实例
func NewForwarder(cfg *config.Config, endpointManager *endpoint.Manager) *Forwarder {
	return &Forwarder{
		config:          cfg,
		endpointManager: endpointManager,
	}
}

// ForwardRequestToEndpoint 转发请求到指定端点
func (f *Forwarder) ForwardRequestToEndpoint(ctx context.Context, r *http.Request, bodyBytes []byte, ep *endpoint.Endpoint) (*http.Response, error) {
	// 创建目标URL
	targetURL := ep.Config.URL + r.URL.Path
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	// 创建请求
	req, err := http.NewRequestWithContext(ctx, r.Method, targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 复制和修改头部
	f.CopyHeaders(r, req, ep)

	// 创建HTTP传输
	httpTransport, err := transport.CreateTransport(f.config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport: %w", err)
	}
	
	// 优化传输设置用于流式处理
	httpTransport.DisableKeepAlives = false
	httpTransport.MaxIdleConns = 10
	httpTransport.MaxIdleConnsPerHost = 2
	httpTransport.IdleConnTimeout = 0 // 无空闲超时
	httpTransport.TLSHandshakeTimeout = 10 * time.Second
	httpTransport.ExpectContinueTimeout = 1 * time.Second

	// 从配置中读取响应头超时时间，默认60秒
	responseHeaderTimeout := f.config.Streaming.ResponseHeaderTimeout
	if responseHeaderTimeout == 0 {
		responseHeaderTimeout = 60 * time.Second
	}
	httpTransport.ResponseHeaderTimeout = responseHeaderTimeout

	httpTransport.DisableCompression = true // 禁用压缩以防缓冲延迟
	httpTransport.WriteBufferSize = 4096    // 较小的写缓冲区
	httpTransport.ReadBufferSize = 4096     // 较小的读缓冲区
	
	client := &http.Client{
		Timeout:   0, // 流式请求无超时
		Transport: httpTransport,
	}

	// 执行请求
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	// 检查响应状态
	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, fmt.Errorf("endpoint returned error: %d", resp.StatusCode)
	}

	return resp, nil
}

// CopyHeaders 复制头部逻辑
func (f *Forwarder) CopyHeaders(src *http.Request, dst *http.Request, ep *endpoint.Endpoint) {
	// List of headers to skip/remove
	skipHeaders := map[string]bool{
		"host":          true, // We'll set this based on target endpoint
		"authorization": true, // We'll add our own if configured
		"x-api-key":     true, // Remove sensitive client API keys
	}
	
	// Copy all headers except those we want to skip
	for key, values := range src.Header {
		if skipHeaders[strings.ToLower(key)] {
			continue
		}
		
		for _, value := range values {
			dst.Header.Add(key, value)
		}
	}

	// Set Host header based on target endpoint URL
	if u, err := url.Parse(ep.Config.URL); err == nil {
		dst.Header.Set("Host", u.Host)
		// Also set the Host field directly on the request for proper HTTP/1.1 behavior
		dst.Host = u.Host
	}

	// Add or override Authorization header with dynamically resolved token
	token := f.endpointManager.GetTokenForEndpoint(ep)
	if token != "" {
		dst.Header.Set("Authorization", "Bearer "+token)
	}

	// Add or override X-Api-Key header with dynamically resolved api-key
	apiKey := f.endpointManager.GetApiKeyForEndpoint(ep)
	if apiKey != "" {
		dst.Header.Set("X-Api-Key", apiKey)
	}

	// Add custom headers from endpoint configuration
	for key, value := range ep.Config.Headers {
		dst.Header.Set(key, value)
	}

	// Remove hop-by-hop headers
	hopByHopHeaders := []string{
		"Connection",
		"Keep-Alive", 
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Te",
		"Trailers",
		"Transfer-Encoding",
		"Upgrade",
	}

	for _, header := range hopByHopHeaders {
		dst.Header.Del(header)
	}
}