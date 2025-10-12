package integration

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/middleware"
	"cc-forwarder/internal/proxy"
	"cc-forwarder/internal/tracking"
)

// StreamingTokenPreservationTestSuite 流式请求Token保存测试套件
// 专门验证 CRITICAL_TOKEN_USAGE_LOSS_BUG.md 中描述的流式请求Token丢失问题修复
type StreamingTokenPreservationTestSuite struct {
	tracker              *tracking.UsageTracker
	monitoringMiddleware *middleware.MonitoringMiddleware
	endpointManager      *endpoint.Manager
	config               *config.Config
}

// setupTestSuite 设置测试套件
func setupTestSuite(t *testing.T) *StreamingTokenPreservationTestSuite {
	// 创建 UsageTracker
	trackerConfig := &tracking.Config{
		Enabled:       true,
		DatabasePath:  ":memory:",
		BufferSize:    100,
		BatchSize:     10,
		FlushInterval: 100 * time.Millisecond,
		MaxRetry:      3,
		DefaultPricing: tracking.ModelPricing{
			Input:         2.0,
			Output:        10.0,
			CacheCreation: 1.25,
			CacheRead:     0.25,
		},
	}

	tracker, err := tracking.NewUsageTracker(trackerConfig)
	if err != nil {
		t.Fatalf("创建 UsageTracker 失败: %v", err)
	}

	// 创建 MonitoringMiddleware
	monitoringMiddleware := middleware.NewMonitoringMiddleware(nil)

	// 创建 EndpointManager
	cfg := &config.Config{
		Endpoints: []config.EndpointConfig{
			{
				Name:     "streaming-test-endpoint",
				URL:      "https://api.test.com",
				Token:    "test-token",
				Priority: 1,
				Group:    "test-group",
			},
		},
		Web: config.WebConfig{
			Enabled: false,
		},
	}

	endpointManager := endpoint.NewManager(cfg)

	return &StreamingTokenPreservationTestSuite{
		tracker:              tracker,
		monitoringMiddleware: monitoringMiddleware,
		endpointManager:      endpointManager,
		config:               cfg,
	}
}

// teardownTestSuite 清理测试套件
func (suite *StreamingTokenPreservationTestSuite) teardownTestSuite(t *testing.T) {
	if suite.tracker != nil {
		suite.tracker.Close()
	}
}

// TestStreamingEOFErrorTokenPreservation 测试EOF错误场景下的Token保存
// 对应 CRITICAL_TOKEN_USAGE_LOSS_BUG.md 中的实际线上案例
func TestStreamingEOFErrorTokenPreservation(t *testing.T) {
	suite := setupTestSuite(t)
	defer suite.teardownTestSuite(t)

	t.Log("🧪 测试开始：EOF错误场景下的流式Token保存")

	// 模拟带有Token信息但在处理过程中遇到EOF的SSE响应
	sseData := `event: message_start
data: {"type":"message_start","message":{"id":"msg_01EOF123","type":"message","role":"assistant","model":"claude-3-5-haiku-20241022","content":[],"usage":{"input_tokens":257,"output_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"},"usage":{"input_tokens":257,"output_tokens":25,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}

`
	// 注意：故意在这里截断数据，不包含message_stop，模拟EOF错误

	// 创建模拟的EOF错误响应
	eofReader := &EOFErrorReader{
		data:     []byte(sseData),
		position: 0,
		eofAfter: len(sseData) - 50, // 在接近结尾时产生EOF
	}

	resp := &http.Response{
		StatusCode: 200,
		Body:       eofReader,
		Header:     make(http.Header),
	}

	// 创建生命周期管理器和流处理器
	requestID := "req-eof-test-001"
	lifecycleManager := proxy.NewRequestLifecycleManager(
		suite.tracker,
		suite.monitoringMiddleware,
		requestID,
		nil, // eventBus
	)

	lifecycleManager.SetEndpoint("streaming-test-endpoint", "test-group")
	lifecycleManager.StartRequest("192.168.1.100", "test-client", "POST", "/v1/messages", true)

	// 创建响应记录器和模拟流处理器
	recorder := httptest.NewRecorder()
	flusher := &mockFlusher{}

	tokenParser := proxy.NewTokenParser()
	streamProcessor := proxy.NewStreamProcessor(
		tokenParser,
		suite.tracker,
		recorder,
		flusher,
		requestID,
		"streaming-test-endpoint",
	)

	// 执行流式处理 - 期望遇到EOF错误
	t.Log("🔄 开始流式处理，期望遇到EOF错误...")

	ctx := context.Background()
	finalTokenUsage, modelName, err := streamProcessor.ProcessStreamWithRetry(ctx, resp)

	// 验证错误类型
	if err == nil {
		t.Error("❌ 期望收到EOF错误，但未收到错误")
	} else {
		t.Logf("✅ 收到预期错误: %v", err)

		// 验证错误类型是否为EOF或相关网络错误
		if !strings.Contains(err.Error(), "EOF") && !strings.Contains(err.Error(), "unexpected") {
			t.Logf("⚠️ 错误类型可能不是EOF: %v", err)
		}
	}

	// 关键验证1：Token信息应该被保存
	if finalTokenUsage == nil {
		t.Error("❌ CRITICAL: 流式EOF错误后Token信息为nil，存在Token丢失问题")
	} else {
		t.Logf("✅ 流式EOF错误后Token信息被保留:")
		t.Logf("   输入Token: %d (期望: 257)", finalTokenUsage.InputTokens)
		t.Logf("   输出Token: %d (期望: 25)", finalTokenUsage.OutputTokens)
		t.Logf("   缓存创建Token: %d", finalTokenUsage.CacheCreationTokens)
		t.Logf("   缓存读取Token: %d", finalTokenUsage.CacheReadTokens)

		// 验证具体数值与实际线上案例匹配
		if finalTokenUsage.InputTokens != 257 {
			t.Errorf("❌ 输入Token数量不匹配实际案例: 期望 257, 实际 %d", finalTokenUsage.InputTokens)
		}
		if finalTokenUsage.OutputTokens != 25 {
			t.Errorf("❌ 输出Token数量不匹配实际案例: 期望 25, 实际 %d", finalTokenUsage.OutputTokens)
		}
	}

	// 关键验证2：模型名称应该被正确识别
	if modelName == "" || modelName == "unknown" {
		t.Error("❌ CRITICAL: 模型名称未被正确识别")
	} else {
		t.Logf("✅ 模型名称被正确识别: %s", modelName)
		if modelName != "claude-3-5-haiku-20241022" {
			t.Errorf("❌ 模型名称不匹配: 期望 'claude-3-5-haiku-20241022', 实际 '%s'", modelName)
		}
	}

	// 关键验证3：检查错误状态传递
	if err != nil && strings.Contains(err.Error(), "stream_status:") {
		t.Logf("✅ 错误状态格式正确，包含stream_status标记")

		// 解析状态信息
		parts := strings.SplitN(err.Error(), ":", 5)
		if len(parts) >= 2 {
			status := parts[1]
			t.Logf("   解析出的状态: %s", status)

			// 验证状态类型合理性
			validStatuses := []string{"error", "network_error", "timeout", "cancelled"}
			statusValid := false
			for _, validStatus := range validStatuses {
				if status == validStatus {
					statusValid = true
					break
				}
			}
			if !statusValid {
				t.Errorf("❌ 状态类型无效: %s", status)
			}
		}
	}

	// 使用修复后的方法记录失败Token信息
	if finalTokenUsage != nil {
		failureReason := "eof_error"
		if err != nil && strings.Contains(err.Error(), "timeout") {
			failureReason = "timeout"
		} else if err != nil && strings.Contains(err.Error(), "cancel") {
			failureReason = "cancelled"
		}

		// 设置模型信息
		if modelName != "" && modelName != "unknown" {
			lifecycleManager.SetModel(modelName)
		}

		// 记录失败请求的Token信息
		lifecycleManager.RecordTokensForFailedRequest(finalTokenUsage, failureReason)

		// 等待异步处理完成
		time.Sleep(300 * time.Millisecond)

		// 验证Token信息被正确记录到监控系统
		metrics := suite.monitoringMiddleware.GetMetrics()
		if metrics.FailedRequestTokens == 0 {
			t.Error("❌ CRITICAL: 失败请求Token未被记录到监控系统")
		} else {
			expectedTotal := finalTokenUsage.InputTokens + finalTokenUsage.OutputTokens +
				finalTokenUsage.CacheCreationTokens + finalTokenUsage.CacheReadTokens
			if metrics.FailedRequestTokens != expectedTotal {
				t.Errorf("❌ 监控系统Token统计不匹配: 期望 %d, 实际 %d",
					expectedTotal, metrics.FailedRequestTokens)
			} else {
				t.Logf("✅ 监控系统正确记录失败Token: %d", metrics.FailedRequestTokens)
			}
		}

		// 验证请求状态不被误标记为completed
		if lifecycleManager.IsCompleted() {
			t.Error("❌ CRITICAL: 失败请求被误标记为completed状态")
		} else {
			t.Log("✅ 失败请求状态保持正确，未被标记为completed")
		}
	}

	t.Log("🎯 EOF错误场景Token保存测试完成")
}

// TestStreamingNetworkInterruptionTokenPreservation 测试网络中断场景
func TestStreamingNetworkInterruptionTokenPreservation(t *testing.T) {
	suite := setupTestSuite(t)
	defer suite.teardownTestSuite(t)

	t.Log("🧪 测试开始：网络中断场景下的流式Token保存")

	// 模拟网络中断的SSE响应
	sseData := `event: message_start
data: {"type":"message_start","message":{"id":"msg_01NET123","type":"message","role":"assistant","model":"claude-3-5-sonnet-20241022","content":[],"usage":{"input_tokens":150,"output_tokens":0,"cache_creation_input_tokens":20,"cache_read_input_tokens":10}}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Processing"},"usage":{"input_tokens":150,"output_tokens":45,"cache_creation_input_tokens":20,"cache_read_input_tokens":10}}

`

	// 创建模拟网络中断的读取器
	networkErrorReader := &NetworkErrorReader{
		data:     []byte(sseData),
		position: 0,
		errorAfter: len(sseData) - 100, // 在部分数据后产生网络错误
	}

	resp := &http.Response{
		StatusCode: 200,
		Body:       networkErrorReader,
		Header:     make(http.Header),
	}

	requestID := "req-network-test-002"
	lifecycleManager := proxy.NewRequestLifecycleManager(
		suite.tracker,
		suite.monitoringMiddleware,
		requestID,
		nil, // eventBus
	)

	lifecycleManager.SetEndpoint("streaming-test-endpoint", "test-group")
	lifecycleManager.StartRequest("192.168.1.101", "test-client", "POST", "/v1/messages", true)

	recorder := httptest.NewRecorder()
	flusher := &mockFlusher{}

	tokenParser := proxy.NewTokenParser()
	streamProcessor := proxy.NewStreamProcessor(
		tokenParser,
		suite.tracker,
		recorder,
		flusher,
		requestID,
		"streaming-test-endpoint",
	)

	t.Log("🔄 开始流式处理，期望遇到网络中断...")

	ctx := context.Background()
	finalTokenUsage, modelName, err := streamProcessor.ProcessStreamWithRetry(ctx, resp)

	// 验证网络错误
	if err == nil {
		t.Error("❌ 期望收到网络错误，但未收到错误")
	} else {
		t.Logf("✅ 收到预期网络错误: %v", err)
	}

	// 验证Token信息保存
	if finalTokenUsage == nil {
		t.Error("❌ CRITICAL: 网络中断后Token信息为nil")
	} else {
		t.Logf("✅ 网络中断后Token信息被保留:")
		t.Logf("   输入Token: %d", finalTokenUsage.InputTokens)
		t.Logf("   输出Token: %d", finalTokenUsage.OutputTokens)
		t.Logf("   缓存创建Token: %d", finalTokenUsage.CacheCreationTokens)
		t.Logf("   缓存读取Token: %d", finalTokenUsage.CacheReadTokens)

		// 验证数值
		if finalTokenUsage.InputTokens != 150 {
			t.Errorf("❌ 输入Token数量错误: 期望 150, 实际 %d", finalTokenUsage.InputTokens)
		}
		if finalTokenUsage.OutputTokens != 45 {
			t.Errorf("❌ 输出Token数量错误: 期望 45, 实际 %d", finalTokenUsage.OutputTokens)
		}
		if finalTokenUsage.CacheCreationTokens != 20 {
			t.Errorf("❌ 缓存创建Token数量错误: 期望 20, 实际 %d", finalTokenUsage.CacheCreationTokens)
		}
		if finalTokenUsage.CacheReadTokens != 10 {
			t.Errorf("❌ 缓存读取Token数量错误: 期望 10, 实际 %d", finalTokenUsage.CacheReadTokens)
		}
	}

	// 记录失败Token并验证状态
	if finalTokenUsage != nil {
		if modelName != "" && modelName != "unknown" {
			lifecycleManager.SetModel(modelName)
		}

		lifecycleManager.RecordTokensForFailedRequest(finalTokenUsage, "network_error")
		lifecycleManager.UpdateStatus("network_error", 1, 200)

		time.Sleep(200 * time.Millisecond)

		// 验证状态正确性
		if lifecycleManager.GetLastStatus() != "network_error" {
			t.Errorf("❌ 状态更新错误: 期望 'network_error', 实际 '%s'", lifecycleManager.GetLastStatus())
		} else {
			t.Log("✅ 请求状态正确更新为network_error")
		}
	}

	t.Log("🎯 网络中断场景Token保存测试完成")
}

// TestStreamingAPIErrorTokenPreservation 测试API错误场景
func TestStreamingAPIErrorTokenPreservation(t *testing.T) {
	suite := setupTestSuite(t)
	defer suite.teardownTestSuite(t)

	t.Log("🧪 测试开始：API错误场景下的流式Token保存")

	// 模拟带有API错误的SSE响应
	sseData := `event: message_start
data: {"type":"message_start","message":{"id":"msg_01API123","type":"message","role":"assistant","model":"claude-3-5-sonnet-20241022","content":[],"usage":{"input_tokens":300,"output_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":15}}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Starting"},"usage":{"input_tokens":300,"output_tokens":35,"cache_creation_input_tokens":0,"cache_read_input_tokens":15}}

event: error
data: {"type":"error","error":{"type":"rate_limit_error","message":"Rate limit exceeded. Please slow down your requests."}}

`

	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(sseData)),
		Header:     make(http.Header),
	}

	requestID := "req-api-error-003"
	lifecycleManager := proxy.NewRequestLifecycleManager(
		suite.tracker,
		suite.monitoringMiddleware,
		requestID,
		nil, // eventBus
	)

	lifecycleManager.SetEndpoint("streaming-test-endpoint", "test-group")
	lifecycleManager.StartRequest("192.168.1.102", "test-client", "POST", "/v1/messages", true)

	recorder := httptest.NewRecorder()
	flusher := &mockFlusher{}

	tokenParser := proxy.NewTokenParser()
	streamProcessor := proxy.NewStreamProcessor(
		tokenParser,
		suite.tracker,
		recorder,
		flusher,
		requestID,
		"streaming-test-endpoint",
	)

	t.Log("🔄 开始流式处理，期望遇到API错误...")

	ctx := context.Background()
	finalTokenUsage, modelName, err := streamProcessor.ProcessStreamWithRetry(ctx, resp)

	// 验证API错误
	if err == nil {
		t.Error("❌ 期望收到API错误，但未收到错误")
	} else {
		t.Logf("✅ 收到预期API错误: %v", err)

		// 验证错误类型
		if !strings.Contains(err.Error(), "rate_limit") && !strings.Contains(err.Error(), "error") {
			t.Logf("⚠️ 错误类型可能不是预期的API错误: %v", err)
		}
	}

	// 验证Token信息保存
	if finalTokenUsage == nil {
		t.Error("❌ CRITICAL: API错误后Token信息为nil")
	} else {
		t.Logf("✅ API错误后Token信息被保留:")
		t.Logf("   输入Token: %d", finalTokenUsage.InputTokens)
		t.Logf("   输出Token: %d", finalTokenUsage.OutputTokens)
		t.Logf("   缓存读取Token: %d", finalTokenUsage.CacheReadTokens)

		// 验证数值
		if finalTokenUsage.InputTokens != 300 {
			t.Errorf("❌ 输入Token数量错误: 期望 300, 实际 %d", finalTokenUsage.InputTokens)
		}
		if finalTokenUsage.OutputTokens != 35 {
			t.Errorf("❌ 输出Token数量错误: 期望 35, 实际 %d", finalTokenUsage.OutputTokens)
		}
		if finalTokenUsage.CacheReadTokens != 15 {
			t.Errorf("❌ 缓存读取Token数量错误: 期望 15, 实际 %d", finalTokenUsage.CacheReadTokens)
		}
	}

	// 记录失败Token并验证
	if finalTokenUsage != nil {
		if modelName != "" && modelName != "unknown" {
			lifecycleManager.SetModel(modelName)
		}

		// 根据错误类型确定失败原因
		failureReason := "stream_error"
		if err != nil && strings.Contains(err.Error(), "rate_limit") {
			failureReason = "rate_limited"
		}

		lifecycleManager.RecordTokensForFailedRequest(finalTokenUsage, failureReason)
		lifecycleManager.UpdateStatus(failureReason, 1, 200)

		time.Sleep(200 * time.Millisecond)

		// 验证监控指标
		metrics := suite.monitoringMiddleware.GetMetrics()
		if metrics.FailedTokensByReason == nil || metrics.FailedTokensByReason[failureReason] == 0 {
			t.Error("❌ CRITICAL: API错误失败Token未按原因分类记录")
		} else {
			expectedTotal := finalTokenUsage.InputTokens + finalTokenUsage.OutputTokens + finalTokenUsage.CacheReadTokens
			if metrics.FailedTokensByReason[failureReason] != expectedTotal {
				t.Errorf("❌ 按原因分类的Token统计错误: 期望 %d, 实际 %d",
					expectedTotal, metrics.FailedTokensByReason[failureReason])
			} else {
				t.Logf("✅ API错误Token正确按原因分类记录: %s -> %d",
					failureReason, metrics.FailedTokensByReason[failureReason])
			}
		}
	}

	t.Log("🎯 API错误场景Token保存测试完成")
}

// TestStreamingClientCancelTokenPreservation 测试客户端取消场景
func TestStreamingClientCancelTokenPreservation(t *testing.T) {
	suite := setupTestSuite(t)
	defer suite.teardownTestSuite(t)

	t.Log("🧪 测试开始：客户端取消场景下的流式Token保存")

	// 模拟客户端取消时的SSE响应
	sseData := `event: message_start
data: {"type":"message_start","message":{"id":"msg_01CANCEL123","type":"message","role":"assistant","model":"claude-3-5-sonnet-20241022","content":[],"usage":{"input_tokens":400,"output_tokens":0,"cache_creation_input_tokens":50,"cache_read_input_tokens":25}}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Beginning"},"usage":{"input_tokens":400,"output_tokens":60,"cache_creation_input_tokens":50,"cache_read_input_tokens":25}}

`

	// 创建可取消的上下文
	ctx, cancel := context.WithCancel(context.Background())

	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(sseData)),
		Header:     make(http.Header),
	}

	requestID := "req-cancel-test-004"
	lifecycleManager := proxy.NewRequestLifecycleManager(
		suite.tracker,
		suite.monitoringMiddleware,
		requestID,
		nil, // eventBus
	)

	lifecycleManager.SetEndpoint("streaming-test-endpoint", "test-group")
	lifecycleManager.StartRequest("192.168.1.103", "test-client", "POST", "/v1/messages", true)

	recorder := httptest.NewRecorder()
	flusher := &mockFlusher{}

	tokenParser := proxy.NewTokenParser()
	streamProcessor := proxy.NewStreamProcessor(
		tokenParser,
		suite.tracker,
		recorder,
		flusher,
		requestID,
		"streaming-test-endpoint",
	)

	t.Log("🔄 开始流式处理，将在处理过程中取消...")

	// 在单独的goroutine中延迟取消上下文
	go func() {
		time.Sleep(50 * time.Millisecond) // 允许部分处理完成
		t.Log("⏹️ 取消客户端请求...")
		cancel()
	}()

	finalTokenUsage, modelName, err := streamProcessor.ProcessStreamWithRetry(ctx, resp)

	// 验证取消错误
	if err == nil {
		t.Error("❌ 期望收到取消错误，但未收到错误")
	} else {
		t.Logf("✅ 收到预期取消错误: %v", err)

		// 验证错误类型
		if !strings.Contains(err.Error(), "cancel") && !strings.Contains(err.Error(), "context") {
			t.Logf("⚠️ 错误类型可能不是预期的取消错误: %v", err)
		}
	}

	// 验证Token信息保存
	if finalTokenUsage == nil {
		t.Log("⚠️ 客户端取消后Token信息为nil（可能为预期行为，取决于取消时机）")
	} else {
		t.Logf("✅ 客户端取消后Token信息被保留:")
		t.Logf("   输入Token: %d", finalTokenUsage.InputTokens)
		t.Logf("   输出Token: %d", finalTokenUsage.OutputTokens)
		t.Logf("   缓存创建Token: %d", finalTokenUsage.CacheCreationTokens)
		t.Logf("   缓存读取Token: %d", finalTokenUsage.CacheReadTokens)

		// 如果有Token信息，验证并记录
		if modelName != "" && modelName != "unknown" {
			lifecycleManager.SetModel(modelName)
		}

		lifecycleManager.RecordTokensForFailedRequest(finalTokenUsage, "cancelled")
		lifecycleManager.UpdateStatus("cancelled", 1, 200)

		time.Sleep(200 * time.Millisecond)

		// 验证取消状态
		if lifecycleManager.GetLastStatus() != "cancelled" {
			t.Errorf("❌ 状态更新错误: 期望 'cancelled', 实际 '%s'", lifecycleManager.GetLastStatus())
		} else {
			t.Log("✅ 请求状态正确更新为cancelled")
		}
	}

	t.Log("🎯 客户端取消场景Token保存测试完成")
}

// TestStreamingTimeoutTokenPreservation 测试超时场景
func TestStreamingTimeoutTokenPreservation(t *testing.T) {
	suite := setupTestSuite(t)
	defer suite.teardownTestSuite(t)

	t.Log("🧪 测试开始：超时场景下的流式Token保存")

	// 模拟慢速响应的SSE数据
	sseData := `event: message_start
data: {"type":"message_start","message":{"id":"msg_01TIMEOUT123","type":"message","role":"assistant","model":"claude-3-5-sonnet-20241022","content":[],"usage":{"input_tokens":500,"output_tokens":0,"cache_creation_input_tokens":30,"cache_read_input_tokens":20}}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Slow"},"usage":{"input_tokens":500,"output_tokens":80,"cache_creation_input_tokens":30,"cache_read_input_tokens":20}}

`

	// 创建慢速读取器模拟超时
	slowReader := &SlowReader{
		data:      []byte(sseData),
		position:  0,
		delayTime: 200 * time.Millisecond, // 每次读取延迟200ms
	}

	resp := &http.Response{
		StatusCode: 200,
		Body:       slowReader,
		Header:     make(http.Header),
	}

	requestID := "req-timeout-test-005"
	lifecycleManager := proxy.NewRequestLifecycleManager(
		suite.tracker,
		suite.monitoringMiddleware,
		requestID,
		nil, // eventBus
	)

	lifecycleManager.SetEndpoint("streaming-test-endpoint", "test-group")
	lifecycleManager.StartRequest("192.168.1.104", "test-client", "POST", "/v1/messages", true)

	recorder := httptest.NewRecorder()
	flusher := &mockFlusher{}

	tokenParser := proxy.NewTokenParser()
	streamProcessor := proxy.NewStreamProcessor(
		tokenParser,
		suite.tracker,
		recorder,
		flusher,
		requestID,
		"streaming-test-endpoint",
	)

	// 创建短超时上下文
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	t.Log("🔄 开始流式处理，设置100ms超时...")

	finalTokenUsage, modelName, err := streamProcessor.ProcessStreamWithRetry(ctx, resp)

	// 验证超时错误
	if err == nil {
		t.Error("❌ 期望收到超时错误，但未收到错误")
	} else {
		t.Logf("✅ 收到预期超时错误: %v", err)

		// 验证错误类型
		if !strings.Contains(err.Error(), "timeout") && !strings.Contains(err.Error(), "deadline") {
			t.Logf("⚠️ 错误类型可能不是预期的超时错误: %v", err)
		}
	}

	// 验证Token信息保存
	if finalTokenUsage == nil {
		t.Log("⚠️ 超时后Token信息为nil（可能为预期行为，取决于超时时机）")
	} else {
		t.Logf("✅ 超时后Token信息被保留:")
		t.Logf("   输入Token: %d", finalTokenUsage.InputTokens)
		t.Logf("   输出Token: %d", finalTokenUsage.OutputTokens)
		t.Logf("   缓存创建Token: %d", finalTokenUsage.CacheCreationTokens)
		t.Logf("   缓存读取Token: %d", finalTokenUsage.CacheReadTokens)

		// 记录失败Token
		if modelName != "" && modelName != "unknown" {
			lifecycleManager.SetModel(modelName)
		}

		lifecycleManager.RecordTokensForFailedRequest(finalTokenUsage, "timeout")
		lifecycleManager.UpdateStatus("timeout", 1, 200)

		time.Sleep(200 * time.Millisecond)

		// 验证超时状态
		if lifecycleManager.GetLastStatus() != "timeout" {
			t.Errorf("❌ 状态更新错误: 期望 'timeout', 实际 '%s'", lifecycleManager.GetLastStatus())
		} else {
			t.Log("✅ 请求状态正确更新为timeout")
		}
	}

	t.Log("🎯 超时场景Token保存测试完成")
}

// TestStreamingFailureStatusIntegrity 测试失败状态完整性
// 验证失败请求不会被误标记为completed
func TestStreamingFailureStatusIntegrity(t *testing.T) {
	suite := setupTestSuite(t)
	defer suite.teardownTestSuite(t)

	t.Log("🧪 测试开始：流式失败状态完整性验证")

	// 创建多个不同失败场景的并发测试
	scenarios := []struct {
		name           string
		requestID      string
		errorType      string
		expectedStatus string
	}{
		{"EOF Error", "req-integrity-eof", "eof", "error"},
		{"Network Error", "req-integrity-net", "network", "network_error"},
		{"API Error", "req-integrity-api", "api", "stream_error"},
		{"Timeout", "req-integrity-timeout", "timeout", "timeout"},
		{"Cancelled", "req-integrity-cancel", "cancel", "cancelled"},
	}

	var wg sync.WaitGroup
	results := make(chan testResult, len(scenarios))

	for _, scenario := range scenarios {
		wg.Add(1)
		go func(s struct {
			name           string
			requestID      string
			errorType      string
			expectedStatus string
		}) {
			defer wg.Done()

			result := testResult{
				scenarioName: s.name,
				requestID:    s.requestID,
				success:      true,
				messages:     []string{},
			}

			// 创建生命周期管理器
			lifecycleManager := proxy.NewRequestLifecycleManager(
				suite.tracker,
				suite.monitoringMiddleware,
				s.requestID,
				nil, // eventBus
			)

			lifecycleManager.SetEndpoint("streaming-test-endpoint", "test-group")
			lifecycleManager.StartRequest("192.168.1.100", "test-client", "POST", "/v1/messages", true)

			// 模拟Token信息
			tokens := &tracking.TokenUsage{
				InputTokens:         100,
				OutputTokens:        50,
				CacheCreationTokens: 10,
				CacheReadTokens:     5,
			}

			// 设置模型并记录失败Token
			lifecycleManager.SetModel("claude-3-5-sonnet-20241022")
			lifecycleManager.RecordTokensForFailedRequest(tokens, s.expectedStatus)
			lifecycleManager.UpdateStatus(s.expectedStatus, 1, 200)

			// 等待处理完成
			time.Sleep(100 * time.Millisecond)

			// 验证状态完整性
			if lifecycleManager.IsCompleted() {
				result.success = false
				result.messages = append(result.messages,
					fmt.Sprintf("❌ %s: 失败请求被误标记为completed", s.name))
			} else {
				result.messages = append(result.messages,
					fmt.Sprintf("✅ %s: 失败请求状态保持正确", s.name))
			}

			// 验证状态值
			if lifecycleManager.GetLastStatus() != s.expectedStatus {
				result.success = false
				result.messages = append(result.messages,
					fmt.Sprintf("❌ %s: 状态不匹配，期望 '%s', 实际 '%s'",
						s.name, s.expectedStatus, lifecycleManager.GetLastStatus()))
			} else {
				result.messages = append(result.messages,
					fmt.Sprintf("✅ %s: 状态正确为 '%s'", s.name, s.expectedStatus))
			}

			results <- result
		}(scenario)
	}

	// 等待所有测试完成
	wg.Wait()
	close(results)

	// 收集并报告结果
	allSuccess := true
	for result := range results {
		for _, message := range result.messages {
			t.Log(message)
		}
		if !result.success {
			allSuccess = false
		}
	}

	if !allSuccess {
		t.Error("❌ 状态完整性测试失败")
	} else {
		t.Log("✅ 所有失败场景状态完整性验证通过")
	}

	t.Log("🎯 流式失败状态完整性测试完成")
}

// 辅助结构体和方法

type testResult struct {
	scenarioName string
	requestID    string
	success      bool
	messages     []string
}

