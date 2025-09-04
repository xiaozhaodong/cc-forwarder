package transport

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"cc-forwarder/config"
	"golang.org/x/net/proxy"
)

// CreateTransport creates an HTTP transport with optional proxy support
func CreateTransport(cfg *config.Config) (*http.Transport, error) {
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	// If proxy is not enabled, return default transport
	if !cfg.Proxy.Enabled {
		return transport, nil
	}

	// Validate proxy configuration
	if cfg.Proxy.Type == "" {
		return nil, fmt.Errorf("proxy type is required when proxy is enabled")
	}
	if cfg.Proxy.Type != "http" && cfg.Proxy.Type != "https" && cfg.Proxy.Type != "socks5" {
		return nil, fmt.Errorf("unsupported proxy type: %s", cfg.Proxy.Type)
	}
	if cfg.Proxy.URL == "" && (cfg.Proxy.Host == "" || cfg.Proxy.Port == 0) {
		return nil, fmt.Errorf("proxy URL or host:port must be specified when proxy is enabled")
	}

	switch cfg.Proxy.Type {
	case "http", "https":
		return createHTTPProxyTransport(cfg, transport)
	case "socks5":
		return createSOCKS5ProxyTransport(cfg, transport)
	default:
		return nil, fmt.Errorf("unsupported proxy type: %s", cfg.Proxy.Type)
	}
}

// createHTTPProxyTransport creates transport with HTTP/HTTPS proxy
func createHTTPProxyTransport(cfg *config.Config, transport *http.Transport) (*http.Transport, error) {
	var proxyURL *url.URL
	var err error

	if cfg.Proxy.URL != "" {
		// Use complete proxy URL
		proxyURL, err = url.Parse(cfg.Proxy.URL)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy URL: %w", err)
		}
	} else {
		// Construct proxy URL from host:port
		proxyURLStr := fmt.Sprintf("%s://%s:%d", cfg.Proxy.Type, cfg.Proxy.Host, cfg.Proxy.Port)
		proxyURL, err = url.Parse(proxyURLStr)
		if err != nil {
			return nil, fmt.Errorf("failed to construct proxy URL: %w", err)
		}
	}

	// Add authentication if provided
	if cfg.Proxy.Username != "" {
		if cfg.Proxy.Password != "" {
			proxyURL.User = url.UserPassword(cfg.Proxy.Username, cfg.Proxy.Password)
		} else {
			proxyURL.User = url.User(cfg.Proxy.Username)
		}
	}

	transport.Proxy = http.ProxyURL(proxyURL)
	return transport, nil
}

// createSOCKS5ProxyTransport creates transport with SOCKS5 proxy
func createSOCKS5ProxyTransport(cfg *config.Config, transport *http.Transport) (*http.Transport, error) {
	var proxyAddr string
	if cfg.Proxy.URL != "" {
		// Parse SOCKS5 URL
		proxyURL, err := url.Parse(cfg.Proxy.URL)
		if err != nil {
			return nil, fmt.Errorf("invalid SOCKS5 proxy URL: %w", err)
		}
		proxyAddr = proxyURL.Host
	} else {
		// Construct address from host:port
		proxyAddr = fmt.Sprintf("%s:%d", cfg.Proxy.Host, cfg.Proxy.Port)
	}

	// Create SOCKS5 dialer
	var dialer proxy.Dialer
	var err error

	if cfg.Proxy.Username != "" && cfg.Proxy.Password != "" {
		// SOCKS5 with authentication
		auth := &proxy.Auth{
			User:     cfg.Proxy.Username,
			Password: cfg.Proxy.Password,
		}
		dialer, err = proxy.SOCKS5("tcp", proxyAddr, auth, proxy.Direct)
	} else {
		// SOCKS5 without authentication
		dialer, err = proxy.SOCKS5("tcp", proxyAddr, nil, proxy.Direct)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create SOCKS5 dialer: %w", err)
	}

	// Set the custom dialer using DialContext
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialer.Dial(network, addr)
	}
	return transport, nil
}

// GetProxyInfo returns human-readable proxy information
func GetProxyInfo(cfg *config.Config) string {
	if !cfg.Proxy.Enabled {
		return "No proxy"
	}

	var addr string
	if cfg.Proxy.URL != "" {
		addr = cfg.Proxy.URL
	} else {
		addr = fmt.Sprintf("%s:%d", cfg.Proxy.Host, cfg.Proxy.Port)
	}

	authInfo := ""
	if cfg.Proxy.Username != "" {
		authInfo = " (with auth)"
	}

	return fmt.Sprintf("%s proxy: %s%s", cfg.Proxy.Type, addr, authInfo)
}