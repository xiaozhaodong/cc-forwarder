package integration

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cc-forwarder/internal/middleware"
	"cc-forwarder/internal/proxy"
	"cc-forwarder/internal/tracking"
)

// TestStreamingTokenPreservationCore 核心流式Token保存测试
// 专门验证 CRITICAL_TOKEN_USAGE_LOSS_BUG.md 中的关键问题
func TestStreamingTokenPreservationCore(t *testing.T) {
	// 简化配置，快速启动
	trackerConfig := &tracking.Config{
		Enabled:       true,
		DatabasePath:  ":memory:",
		BufferSize:    10,
		BatchSize:     2,
		FlushInterval: 20 * time.Millisecond,
		MaxRetry:      1,
		DefaultPricing: tracking.ModelPricing{
			Input:  2.0,
			Output: 10.0,
		},
	}

	tracker, err := tracking.NewUsageTracker(trackerConfig)
	if err != nil {
		t.Fatalf("创建UsageTracker失败: %v", err)
	}

	// 使用defer确保资源清理
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		// 强制关闭
		go func() {
			time.Sleep(800 * time.Millisecond)
			cancel()
		}()

		select {
		case <-ctx.Done():
			t.Log("⏰ UsageTracker关闭超时，强制退出")
		default:
			tracker.Close()
		}
	}()

	monitoringMiddleware := middleware.NewMonitoringMiddleware(nil)

	t.Log("🧪 开始核心流式Token保存测试")

	// 测试场景1: EOF错误Token保存
	t.Run("EOF错误Token保存", func(t *testing.T) {
		requestID := "req-eof-core-001"

		// 创建生命周期管理器
		lifecycleManager := proxy.NewRequestLifecycleManager(
			tracker,
			monitoringMiddleware,
			requestID,
			nil, // eventBus
		)

		lifecycleManager.SetEndpoint("test-endpoint", "test-group")
		lifecycleManager.StartRequest("127.0.0.1", "test-client", "POST", "/v1/messages", true)

		// 模拟EOF错误的SSE数据
		sseData := `event: message_start
data: {"type":"message_start","message":{"model":"claude-3-5-haiku-20241022","usage":{"input_tokens":257,"output_tokens":0}}}

event: message_delta
data: {"type":"message_delta","usage":{"input_tokens":257,"output_tokens":25}}

`
		// 创建EOF错误读取器
		eofReader := &quickEOFReader{data: []byte(sseData)}

		resp := &http.Response{
			StatusCode: 200,
			Body:       eofReader,
			Header:     make(http.Header),
		}

		// 创建流处理器
		recorder := httptest.NewRecorder()
		tokenParser := proxy.NewTokenParser()
		streamProcessor := proxy.NewStreamProcessor(
			tokenParser,
			tracker,
			recorder,
			&quickFlusher{},
			requestID,
			"test-endpoint",
		)

		// 设置短超时以避免测试超时
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// 执行流式处理
		finalTokenUsage, _, err := streamProcessor.ProcessStreamWithRetry(ctx, resp)

		// 验证关键结果
		if err == nil {
			t.Error("❌ 期望收到EOF错误，但未收到")
		} else {
			t.Logf("✅ 收到预期错误: %v", err)
		}

		// 关键验证：Token丢失检测
		if finalTokenUsage == nil {
			t.Error("❌ CRITICAL: EOF错误后Token信息为nil - 这是待修复的核心问题!")

			// 模拟修复方案验证
			parsedUsage := tokenParser.GetFinalUsage()
			if parsedUsage != nil && (parsedUsage.InputTokens > 0 || parsedUsage.OutputTokens > 0) {
				t.Logf("✅ 修复方案可行: Token解析器保留了Token信息 (输入:%d, 输出:%d)",
					parsedUsage.InputTokens, parsedUsage.OutputTokens)

				// 模拟修复后的Token保存
				lifecycleManager.SetModel("claude-3-5-haiku-20241022")
				lifecycleManager.RecordTokensForFailedRequest(parsedUsage, "eof_error")

				t.Log("💾 模拟修复: 失败Token已记录")
			} else {
				t.Error("❌ Token解析器也没有保存Token信息")
			}
		} else {
			t.Logf("✅ Token信息被保留: 输入%d, 输出%d",
				finalTokenUsage.InputTokens, finalTokenUsage.OutputTokens)
		}

		// 验证模型信息
		modelName := tokenParser.GetModelName()
		if modelName == "" || modelName == "unknown" {
			t.Error("❌ 模型名称丢失")
		} else {
			t.Logf("✅ 模型名称正确: %s", modelName)
		}

		t.Log("🎯 EOF错误Token保存测试完成")
	})

	// 测试场景2: 网络中断Token保存
	t.Run("网络中断Token保存", func(t *testing.T) {
		requestID := "req-net-core-002"

		lifecycleManager := proxy.NewRequestLifecycleManager(
			tracker,
			monitoringMiddleware,
			requestID,
			nil, // eventBus
		)

		lifecycleManager.SetEndpoint("test-endpoint", "test-group")
		lifecycleManager.StartRequest("127.0.0.1", "test-client", "POST", "/v1/messages", true)

		// 模拟网络错误的SSE数据
		sseData := `event: message_start
data: {"type":"message_start","message":{"model":"claude-3-5-sonnet-20241022","usage":{"input_tokens":150,"output_tokens":0}}}

event: message_delta
data: {"type":"message_delta","usage":{"input_tokens":150,"output_tokens":45}}

`
		// 创建网络错误读取器
		netReader := &quickNetworkErrorReader{data: []byte(sseData)}

		resp := &http.Response{
			StatusCode: 200,
			Body:       netReader,
			Header:     make(http.Header),
		}

		recorder := httptest.NewRecorder()
		tokenParser := proxy.NewTokenParser()
		streamProcessor := proxy.NewStreamProcessor(
			tokenParser,
			tracker,
			recorder,
			&quickFlusher{},
			requestID,
			"test-endpoint",
		)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		finalTokenUsage, _, err := streamProcessor.ProcessStreamWithRetry(ctx, resp)

		// 验证网络错误处理
		if err == nil {
			t.Error("❌ 期望收到网络错误，但未收到")
		} else {
			t.Logf("✅ 收到预期网络错误: %v", err)
		}

		// 关键验证
		if finalTokenUsage == nil {
			t.Error("❌ CRITICAL: 网络中断后Token信息为nil")

			// 检查修复可行性
			parsedUsage := tokenParser.GetFinalUsage()
			if parsedUsage != nil && (parsedUsage.InputTokens > 0 || parsedUsage.OutputTokens > 0) {
				t.Logf("✅ 修复方案可行: 输入%d, 输出%d",
					parsedUsage.InputTokens, parsedUsage.OutputTokens)
			}
		} else {
			t.Logf("✅ 网络中断后Token信息被保留: 输入%d, 输出%d",
				finalTokenUsage.InputTokens, finalTokenUsage.OutputTokens)
		}

		t.Log("🎯 网络中断Token保存测试完成")
	})

	// 等待短时间让异步操作完成
	time.Sleep(100 * time.Millisecond)

	t.Log("🎯 核心流式Token保存测试全部完成")
}

// TestTokenParserIsolated 独立的Token解析器测试
func TestTokenParserIsolated(t *testing.T) {
	t.Log("🧪 开始独立Token解析器测试")

	tokenParser := proxy.NewTokenParser()

	// 测试完整的SSE解析流程
	t.Log("🔍 测试message_start解析...")
	tokenParser.ParseSSELine("event: message_start")
	tokenParser.ParseSSELine(`data: {"type":"message_start","message":{"model":"claude-3-5-haiku-20241022","usage":{"input_tokens":257,"output_tokens":0}}}`)
	tokenParser.ParseSSELine("")

	modelName1 := tokenParser.GetModelName()
	if modelName1 == "claude-3-5-haiku-20241022" {
		t.Logf("✅ 模型名称解析正确: %s", modelName1)
	} else {
		t.Errorf("❌ 模型名称错误: 期望'claude-3-5-haiku-20241022', 实际'%s'", modelName1)
	}

	t.Log("🔍 测试message_delta解析...")
	tokenParser.ParseSSELine("event: message_delta")
	tokenParser.ParseSSELine(`data: {"type":"message_delta","usage":{"input_tokens":257,"output_tokens":25}}`)
	usage2 := tokenParser.ParseSSELine("")

	if usage2 != nil {
		t.Logf("✅ message_delta解析成功: 输入%d, 输出%d", usage2.InputTokens, usage2.OutputTokens)
	} else {
		t.Error("❌ message_delta解析失败")
	}

	// 测试最终统计
	finalUsage := tokenParser.GetFinalUsage()
	if finalUsage != nil {
		t.Logf("✅ 最终Token统计: 输入%d, 输出%d", finalUsage.InputTokens, finalUsage.OutputTokens)

		if finalUsage.InputTokens == 257 && finalUsage.OutputTokens == 25 {
			t.Log("🎯 Token数值验证完全正确!")
		}
	} else {
		t.Error("❌ 最终Token统计获取失败")
	}

	t.Log("🎯 独立Token解析器测试完成")
}

// 快速辅助类型，避免测试超时

type quickFlusher struct{}
func (f *quickFlusher) Flush() {}

type quickEOFReader struct {
	data []byte
	pos  int
}

func (r *quickEOFReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data)-10 { // 提前10字节触发EOF
		return 0, io.ErrUnexpectedEOF
	}

	remaining := len(r.data) - r.pos
	if remaining == 0 {
		return 0, io.EOF
	}

	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func (r *quickEOFReader) Close() error {
	return nil
}

type quickNetworkErrorReader struct {
	data []byte
	pos  int
}

func (r *quickNetworkErrorReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data)-20 { // 提前20字节触发网络错误
		return 0, &quickNetworkError{}
	}

	remaining := len(r.data) - r.pos
	if remaining == 0 {
		return 0, &quickNetworkError{}
	}

	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func (r *quickNetworkErrorReader) Close() error {
	return nil
}

type quickNetworkError struct{}

func (e *quickNetworkError) Error() string {
	return "network connection lost"
}

func (e *quickNetworkError) Timeout() bool {
	return false
}

func (e *quickNetworkError) Temporary() bool {
	return true
}