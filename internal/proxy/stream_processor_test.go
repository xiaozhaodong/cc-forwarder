package proxy

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

// mockResponseWriter 实现 http.ResponseWriter 和 http.Flusher
type mockResponseWriter struct {
	buffer  bytes.Buffer
	headers http.Header
	status  int
	flushed int
}

func (m *mockResponseWriter) Header() http.Header {
	if m.headers == nil {
		m.headers = make(http.Header)
	}
	return m.headers
}

func (m *mockResponseWriter) Write(data []byte) (int, error) {
	return m.buffer.Write(data)
}

func (m *mockResponseWriter) WriteHeader(statusCode int) {
	m.status = statusCode
}

func (m *mockResponseWriter) Flush() {
	m.flushed++
}

// mockResponse 创建模拟HTTP响应
func mockResponse(body string, statusCode int) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestStreamProcessor_NewStreamProcessor(t *testing.T) {
	tokenParser := NewTokenParser()
	writer := &mockResponseWriter{}
	requestID := "test-req-123"
	endpoint := "test-endpoint"

	// Test with nil usage tracker for simplicity
	processor := NewStreamProcessor(tokenParser, nil, writer, writer, requestID, endpoint)

	if processor.tokenParser != tokenParser {
		t.Error("TokenParser not set correctly")
	}
	if processor.requestID != requestID {
		t.Error("RequestID not set correctly")
	}
	if processor.endpoint != endpoint {
		t.Error("Endpoint not set correctly")
	}
	if processor.maxParseErrors != 10 {
		t.Error("MaxParseErrors not set to default value")
	}
}

func TestStreamProcessor_ProcessStream_SimpleData(t *testing.T) {
	// 准备测试数据
	testData := "data: test line 1\ndata: test line 2\n"
	resp := mockResponse(testData, 200)
	
	// 创建处理器
	tokenParser := NewTokenParser()
	writer := &mockResponseWriter{}
	processor := NewStreamProcessor(tokenParser, nil, writer, writer, "test-123", "endpoint")

	// 执行流处理
	_, err := processor.ProcessStream(context.Background(), resp)
	
	// 验证结果
	if err != nil {
		t.Errorf("ProcessStream failed: %v", err)
	}
	
	// 验证数据被写入
	output := writer.buffer.String()
	if !strings.Contains(output, "test line 1") {
		t.Error("Output should contain 'test line 1'")
	}
	if !strings.Contains(output, "test line 2") {
		t.Error("Output should contain 'test line 2'")
	}
	
	// 验证Flush被调用
	if writer.flushed == 0 {
		t.Error("Flush should have been called")
	}
	
	// 验证字节数统计
	if processor.bytesProcessed == 0 {
		t.Error("BytesProcessed should be greater than 0")
	}
}

func TestStreamProcessor_GetProcessingStats(t *testing.T) {
	// 创建处理器
	tokenParser := NewTokenParser()
	writer := &mockResponseWriter{}
	processor := NewStreamProcessor(tokenParser, nil, writer, writer, "test-stats", "test-endpoint")
	
	// 设置一些处理统计
	processor.bytesProcessed = 2048
	
	// 获取统计信息
	stats := processor.GetProcessingStats()
	
	// 验证统计信息
	if stats["request_id"] != "test-stats" {
		t.Error("Request ID not in stats")
	}
	if stats["endpoint"] != "test-endpoint" {
		t.Error("Endpoint not in stats")
	}
	if stats["bytes_processed"] != int64(2048) {
		t.Error("Bytes processed not in stats")
	}
}

func TestStreamProcessor_Reset(t *testing.T) {
	// 创建处理器
	tokenParser := NewTokenParser()
	writer := &mockResponseWriter{}
	processor := NewStreamProcessor(tokenParser, nil, writer, writer, "test-reset", "endpoint")
	
	// 设置一些状态
	processor.bytesProcessed = 1024
	processor.parseErrors = append(processor.parseErrors, io.EOF)
	
	// 重置处理器
	processor.Reset()
	
	// 验证状态被重置
	if processor.bytesProcessed != 0 {
		t.Error("Bytes processed not reset")
	}
	if len(processor.parseErrors) != 0 {
		t.Error("Parse errors not reset")
	}
}

func TestStreamProcessor_IsNetworkError(t *testing.T) {
	processor := &StreamProcessor{}
	// 创建错误恢复管理器用于测试
	processor.errorRecovery = NewErrorRecoveryManager(nil)
	
	testCases := []struct {
		err      error
		expected bool
	}{
		{nil, false},
		{io.ErrUnexpectedEOF, true}, // This is now classified as a network error
		{&mockNetError{"connection reset"}, true},
		{&mockNetError{"connection refused"}, true},
		{&mockNetError{"timeout"}, false}, // This is now classified as a timeout error, not network
		{&mockNetError{"network is unreachable"}, true},
		{&mockNetError{"no route to host"}, true},
		{&mockNetError{"broken pipe"}, true},
		{&mockNetError{"unknown error"}, false},
	}
	
	for _, tc := range testCases {
		result := processor.errorRecovery.isNetworkError(tc.err)
		if result != tc.expected {
			t.Errorf("isNetworkError(%v) = %v, expected %v", tc.err, result, tc.expected)
		}
	}
}

// mockNetError 模拟网络错误
type mockNetError struct {
	msg string
}

func (e *mockNetError) Error() string {
	return e.msg
}