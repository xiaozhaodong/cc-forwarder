package proxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	
	"cc-forwarder/internal/tracking"
)

// readCloser 包装 strings.Reader 实现 io.ReadCloser
type readCloser struct {
	*strings.Reader
}

func (r readCloser) Close() error {
	return nil
}

// slowReader 慢速读取器，模拟网络延迟
type slowReader struct {
	data   []byte
	pos    int
	delay  time.Duration
}

func newSlowReader(data string, delay time.Duration) *slowReader {
	return &slowReader{
		data:  []byte(data),
		delay: delay,
	}
}

func (s *slowReader) Read(p []byte) (n int, err error) {
	if s.pos >= len(s.data) {
		return 0, io.EOF
	}
	
	// 模拟网络延迟
	time.Sleep(s.delay)
	
	// 每次只读取少量数据，模拟慢速网络
	maxRead := min(len(p), 100, len(s.data)-s.pos)
	copy(p, s.data[s.pos:s.pos+maxRead])
	s.pos += maxRead
	return maxRead, nil
}

func (s *slowReader) Close() error {
	return nil
}

func min(a, b, c int) int {
	if a < b && a < c {
		return a
	}
	if b < c {
		return b
	}
	return c
}

// TestStreamProcessor_CancellationDetection 测试流式处理中的取消检测
func TestStreamProcessor_CancellationDetection(t *testing.T) {
	// 创建模拟的HTTP响应
	testData := "data: {\"type\":\"message_start\",\"message\":{\"model\":\"claude-3-5-haiku\"}}\n\n" +
		"data: {\"type\":\"message_delta\",\"usage\":{\"input_tokens\":25,\"output_tokens\":50}}\n\n"
	
	resp := &http.Response{
		StatusCode: 200,
		Body:       readCloser{strings.NewReader(testData)},
		Header:     make(http.Header),
	}
	
	// 创建可取消的context
	ctx, cancel := context.WithCancel(context.Background())
	
	// 创建流处理器
	tokenParser := NewTokenParser()
	processor := NewStreamProcessor(tokenParser, nil, httptest.NewRecorder(), httptest.NewRecorder(), "test-req-001", "test-endpoint")
	
	// 立即取消context
	cancel()
	
	// 执行流处理
	_, err := processor.ProcessStream(ctx, resp)
	
	// 验证检测到取消错误
	if err == nil {
		t.Error("Expected cancellation error, got nil")
	}
	
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled error, got: %v", err)
	}
	
	t.Logf("Successfully detected client cancellation: %v", err)
}

// TestStreamProcessor_CancellationWithTimeout 测试取消时的超时等待机制
func TestStreamProcessor_CancellationWithTimeout(t *testing.T) {
	// 创建模拟的长时间处理响应 - 使用慢速读取器
	testData := strings.Repeat("data: processing...\n\n", 1000)
	
	resp := &http.Response{
		StatusCode: 200,
		Body:       newSlowReader(testData, 10*time.Millisecond), // 每次读取延迟10ms
		Header:     make(http.Header),
	}
	
	// 创建context，设置较短的超时
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond) // 50ms超时
	defer cancel()
	
	// 创建流处理器
	tokenParser := NewTokenParser()
	processor := NewStreamProcessor(tokenParser, nil, httptest.NewRecorder(), httptest.NewRecorder(), "test-req-002", "test-endpoint")
	
	start := time.Now()
	
	// 执行流处理
	_, err := processor.ProcessStream(ctx, resp)
	
	duration := time.Since(start)
	
	// 验证在合理时间内检测到超时
	if duration > 200*time.Millisecond {
		t.Errorf("Cancellation detection took too long: %v", duration)
	}
	
	if err == nil {
		t.Error("Expected timeout error, got nil")
	}
	
	// 验证是超时相关的错误
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Errorf("Expected timeout/cancel error, got: %v", err)
	}
	
	t.Logf("Cancellation detected in %v with error: %v", duration, err)
}

// TestStreamProcessor_WaitForParsingWithTimeout 测试超时等待解析完成机制
func TestStreamProcessor_WaitForParsingWithTimeout(t *testing.T) {
	tokenParser := NewTokenParser()
	processor := NewStreamProcessor(tokenParser, nil, httptest.NewRecorder(), httptest.NewRecorder(), "test-req-003", "test-endpoint")
	
	tests := []struct {
		name     string
		timeout  time.Duration
		expected bool
	}{
		{
			name:     "快速完成",
			timeout:  1 * time.Second,
			expected: true, // 应该在超时前完成
		},
		{
			name:     "极短超时",
			timeout:  1 * time.Nanosecond,
			expected: false, // 应该超时
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := time.Now()
			result := processor.waitForParsingWithTimeout(tt.timeout)
			duration := time.Since(start)
			
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
			
			// 验证超时机制工作正常
			if !tt.expected && duration < tt.timeout {
				t.Errorf("Function returned too quickly for timeout test: %v < %v", duration, tt.timeout)
			}
			
			t.Logf("Timeout test completed in %v, result: %v", duration, result)
		})
	}
}

// TestStreamProcessor_CollectAvailableInfo 测试智能信息收集功能
func TestStreamProcessor_CollectAvailableInfo(t *testing.T) {
	// 创建模拟的UsageTracker
	mockTracker := &tracking.UsageTracker{} // 这里应该创建真实的tracker，但为了测试简化
	
	tokenParser := NewTokenParser()
	processor := NewStreamProcessor(tokenParser, mockTracker, httptest.NewRecorder(), httptest.NewRecorder(), "test-req-004", "test-endpoint")
	
	// 模拟已解析的模型信息
	tokenParser.modelName = "claude-3-5-haiku"
	
	cancelErr := context.Canceled
	
	// 测试不同的取消状态
	tests := []struct {
		name   string
		status string
	}{
		{"取消带数据", "cancelled_with_data"},
		{"取消超时", "cancelled_timeout"},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 重置完成标志
			processor.completionRecorded = false
			
			// 测试新版本的 collectAvailableInfoV2 方法
			tokenUsage, err := processor.collectAvailableInfoV2(cancelErr, tt.status)
			
			// 验证返回原始取消错误
			if !errors.Is(err, context.Canceled) {
				t.Errorf("Expected context.Canceled, got: %v", err)
			}
			
			// 验证Token使用信息被返回（新架构下的预期）
			if tokenUsage == nil {
				t.Error("Expected tokenUsage to be returned, got nil")
			}
			
			// 注释掉旧的验证，因为新架构不再直接设置这个标志
			// 验证完成状态被记录 (由于UsageTracker为模拟对象，这里验证逻辑执行即可)
			// if !processor.completionRecorded {
			//     t.Error("Expected completion to be recorded")
			// }
			
			t.Logf("Successfully collected info for status: %s", tt.status)
		})
	}
}

// TestErrorRecoveryManager_ClientCancelError 测试客户端取消错误分类
func TestErrorRecoveryManager_ClientCancelError(t *testing.T) {
	erm := NewErrorRecoveryManager(nil)
	
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "context.Canceled",
			err:      context.Canceled,
			expected: true,
		},
		{
			name:     "自定义取消错误",
			err:      errors.New("context canceled"),
			expected: true,
		},
		{
			name:     "客户端断开连接",
			err:      errors.New("client disconnected"),
			expected: true,
		},
		{
			name:     "普通网络错误",
			err:      errors.New("connection refused"),
			expected: false,
		},
		{
			name:     "nil错误",
			err:      nil,
			expected: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := erm.isClientCancelError(tt.err)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v for error: %v", tt.expected, result, tt.err)
			}
		})
	}
}

// TestErrorRecoveryManager_ClassifyClientCancel 测试完整的客户端取消错误分类流程
func TestErrorRecoveryManager_ClassifyClientCancel(t *testing.T) {
	erm := NewErrorRecoveryManager(nil)
	
	cancelErr := context.Canceled
	errorCtx := erm.ClassifyError(cancelErr, "test-req-005", "test-endpoint", "test-group", 1)
	
	// 验证错误类型分类正确
	if errorCtx.ErrorType != ErrorTypeClientCancel {
		t.Errorf("Expected ErrorTypeClientCancel, got: %v", errorCtx.ErrorType)
	}
	
	// 验证不可重试
	if errorCtx.RetryableAfter != 0 {
		t.Errorf("Expected RetryableAfter to be 0, got: %v", errorCtx.RetryableAfter)
	}
	
	// 验证不应该重试
	shouldRetry := erm.ShouldRetry(errorCtx)
	if shouldRetry {
		t.Error("Client cancel error should not be retryable")
	}
	
	// 验证错误类型名称
	typeName := erm.getErrorTypeName(ErrorTypeClientCancel)
	if typeName != "客户端取消" {
		t.Errorf("Expected '客户端取消', got: %s", typeName)
	}
	
	t.Logf("Client cancel error correctly classified: %s", typeName)
}

// BenchmarkStreamProcessor_CancellationCheck 基准测试取消检测性能
func BenchmarkStreamProcessor_CancellationCheck(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		select {
		case <-ctx.Done():
			// 模拟取消处理
		default:
			// 模拟正常处理
		}
	}
}