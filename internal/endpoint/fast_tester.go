package endpoint

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/transport"
)

// FastTestResult represents the result of a fast endpoint test
type FastTestResult struct {
	Endpoint     *Endpoint
	ResponseTime time.Duration
	Success      bool
	Error        error
	TestTime     time.Time
}

// FastTester performs quick parallel tests on endpoints
type FastTester struct {
	config      *config.Config
	client      *http.Client
	resultCache map[string]*FastTestResult
	cacheMutex  sync.RWMutex
	manager     *Manager // Reference to manager for dynamic token resolution
}

// NewFastTester creates a new fast tester
func NewFastTester(cfg *config.Config) *FastTester {
	// Create transport with proxy support
	httpTransport, err := transport.CreateTransport(cfg)
	if err != nil {
		slog.Error(fmt.Sprintf("❌ Failed to create HTTP transport with proxy for fast tester: %s", err.Error()))
		// Fall back to default transport
		httpTransport = &http.Transport{}
	}
	
	return &FastTester{
		config: cfg,
		client: &http.Client{
			Timeout:   cfg.Strategy.FastTestTimeout,
			Transport: httpTransport,
		},
		resultCache: make(map[string]*FastTestResult),
	}
}

// SetManager sets the manager reference for dynamic token resolution
func (ft *FastTester) SetManager(manager *Manager) {
	ft.manager = manager
}

// TestEndpointsParallel performs parallel testing on all healthy endpoints
// Returns (results, usedCache) where usedCache indicates if cached results were used
func (ft *FastTester) TestEndpointsParallel(ctx context.Context, endpoints []*Endpoint) ([]*FastTestResult, bool) {
	if !ft.config.Strategy.FastTestEnabled {
		// Fast testing disabled, return endpoints with artificial results based on current status
		results := make([]*FastTestResult, 0, len(endpoints))
		for _, ep := range endpoints {
			ep.mutex.RLock()
			result := &FastTestResult{
				Endpoint:     ep,
				ResponseTime: ep.Status.ResponseTime,
				Success:      ep.Status.Healthy,
				TestTime:     time.Now(),
			}
			ep.mutex.RUnlock()
			results = append(results, result)
		}
		return results, false
	}

	// Check cache first
	cachedResults := ft.getCachedResults(endpoints)
	if len(cachedResults) == len(endpoints) {
		slog.Info("📋 Using cached fast test results",
			"cached_endpoints", len(cachedResults),
			"cache_ttl", ft.config.Strategy.FastTestCacheTTL)
		return cachedResults, true
	}

	slog.Debug("🚀 Starting parallel fast test",
		"endpoints", len(endpoints),
		"timeout", ft.config.Strategy.FastTestTimeout,
		"test_path", ft.config.Strategy.FastTestPath)

	// Perform parallel tests
	results := make([]*FastTestResult, len(endpoints))
	var wg sync.WaitGroup

	for i, endpoint := range endpoints {
		wg.Add(1)
		go func(idx int, ep *Endpoint) {
			defer wg.Done()
			results[idx] = ft.testSingleEndpoint(ctx, ep)
		}(i, endpoint)
	}

	// Wait for all tests to complete
	wg.Wait()

	// Update cache
	ft.updateCache(results)

	slog.Debug("✅ Parallel fast test completed",
		"total_endpoints", len(results),
		"successful", ft.countSuccessful(results))

	return results, false
}

// testSingleEndpoint tests a single endpoint
func (ft *FastTester) testSingleEndpoint(ctx context.Context, endpoint *Endpoint) *FastTestResult {
	start := time.Now()

	// Create test URL
	testURL := endpoint.Config.URL + ft.config.Strategy.FastTestPath

	req, err := http.NewRequestWithContext(ctx, "GET", testURL, nil)
	if err != nil {
		return &FastTestResult{
			Endpoint:     endpoint,
			ResponseTime: time.Since(start),
			Success:      false,
			Error:        err,
			TestTime:     time.Now(),
		}
	}

	// Add authorization with dynamically resolved token
	var token string
	if ft.manager != nil {
		token = ft.manager.GetTokenForEndpoint(endpoint)
	} else {
		// Fallback to endpoint's own token if manager is not available
		token = endpoint.Config.Token
	}
	
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	// Add custom headers
	for key, value := range endpoint.Config.Headers {
		req.Header.Set(key, value)
	}

	resp, err := ft.client.Do(req)
	responseTime := time.Since(start)

	if err != nil {
		slog.Warn("❌ Fast test failed with network error",
			"endpoint", endpoint.Config.Name,
			"url", testURL,
			"response_time_ms", responseTime.Milliseconds(),
			"error", err.Error(),
			"reason", "Network or connection error")

		return &FastTestResult{
			Endpoint:     endpoint,
			ResponseTime: responseTime,
			Success:      false,
			Error:        err,
			TestTime:     time.Now(),
		}
	}

	resp.Body.Close()

	// Consider 2xx and 40x as success (same logic as health check)
	success := (resp.StatusCode >= 200 && resp.StatusCode < 300) ||
		(resp.StatusCode >= 400 && resp.StatusCode < 500)

	// Log detailed test results
	if success {
		slog.Debug("⚡ Fast test completed successfully",
			"endpoint", endpoint.Config.Name,
			"url", testURL,
			"status_code", resp.StatusCode,
			"response_time_ms", responseTime.Milliseconds(),
			"success", success)
	} else {
		slog.Warn("❌ Fast test failed with bad status",
			"endpoint", endpoint.Config.Name,
			"url", testURL,
			"status_code", resp.StatusCode,
			"response_time_ms", responseTime.Milliseconds(),
			"success", success,
			"reason", "Invalid HTTP status code")
	}

	return &FastTestResult{
		Endpoint:     endpoint,
		ResponseTime: responseTime,
		Success:      success,
		TestTime:     time.Now(),
	}
}

// getCachedResults returns cached results for endpoints if they're still valid
func (ft *FastTester) getCachedResults(endpoints []*Endpoint) []*FastTestResult {
	ft.cacheMutex.RLock()
	defer ft.cacheMutex.RUnlock()

	now := time.Now()
	results := make([]*FastTestResult, 0, len(endpoints))

	for _, ep := range endpoints {
		if cached, exists := ft.resultCache[ep.Config.Name]; exists {
			if now.Sub(cached.TestTime) <= ft.config.Strategy.FastTestCacheTTL {
				results = append(results, cached)
			} else {
				// Cache expired for this endpoint
				return nil
			}
		} else {
			// No cache for this endpoint
			return nil
		}
	}

	return results
}

// updateCache updates the cache with new test results
func (ft *FastTester) updateCache(results []*FastTestResult) {
	ft.cacheMutex.Lock()
	defer ft.cacheMutex.Unlock()

	for _, result := range results {
		ft.resultCache[result.Endpoint.Config.Name] = result
	}
}

// countSuccessful counts successful test results
func (ft *FastTester) countSuccessful(results []*FastTestResult) int {
	count := 0
	for _, result := range results {
		if result.Success {
			count++
		}
	}
	return count
}

// SortByResponseTime sorts test results by response time (fastest first)
func SortByResponseTime(results []*FastTestResult) []*FastTestResult {
	// Filter successful results first
	successful := make([]*FastTestResult, 0, len(results))
	for _, result := range results {
		if result.Success {
			successful = append(successful, result)
		}
	}

	// Sort successful results by response time
	for i := 0; i < len(successful)-1; i++ {
		for j := i + 1; j < len(successful); j++ {
			if successful[i].ResponseTime > successful[j].ResponseTime {
				successful[i], successful[j] = successful[j], successful[i]
			}
		}
	}

	return successful
}

// UpdateConfig updates the fast tester configuration and recreates client if needed
func (ft *FastTester) UpdateConfig(cfg *config.Config) {
	ft.config = cfg
	
	// Recreate client with new timeout and transport settings
	if transport, err := transport.CreateTransport(cfg); err == nil {
		ft.client = &http.Client{
			Timeout:   cfg.Strategy.FastTestTimeout,
			Transport: transport,
		}
	}
	
	// Clear cache when configuration changes
	ft.cacheMutex.Lock()
	ft.resultCache = make(map[string]*FastTestResult)
	ft.cacheMutex.Unlock()
}