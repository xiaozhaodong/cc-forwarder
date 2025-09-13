package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestLoggingMiddleware_NoDuplicateLogs 测试日志中间件不产生重复日志
func TestLoggingMiddleware_NoDuplicateLogs(t *testing.T) {
	// 创建缓冲区捕获日志
	var logBuffer bytes.Buffer
	
	// 创建结构化日志记录器
	logger := slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{
		Level: slog.LevelDebug, // 设置为Debug级别以捕获所有日志
	}))
	
	// 创建日志中间件
	middleware := NewLoggingMiddleware(logger)
	
	// 创建测试处理器
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	})
	
	// 包装处理器
	wrappedHandler := middleware.Wrap(handler)
	
	// 创建测试请求
	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(`{"test": "data"}`))
	req.Header.Set("User-Agent", "test-agent")
	req.RemoteAddr = "127.0.0.1:12345"
	
	// 创建响应记录器
	recorder := httptest.NewRecorder()
	
	// 执行请求
	wrappedHandler.ServeHTTP(recorder, req)
	
	// 获取日志内容
	logContent := logBuffer.String()
	t.Logf("日志内容:\n%s", logContent)
	
	// 验证没有重复的"Request started"日志
	startedCount := strings.Count(logContent, "Request started")
	if startedCount > 0 {
		t.Logf("⚠️  中间件产生了 %d 个 'Request started' 日志（应该为0，由lifecycle manager处理）", startedCount)
	} else {
		t.Logf("✅ 中间件没有产生重复的 'Request started' 日志")
	}
	
	// 验证没有重复的"Request completed"日志  
	completedCount := strings.Count(logContent, "Request completed")
	if completedCount > 0 {
		t.Logf("⚠️  中间件产生了 %d 个 'Request completed' 日志（应该为0，由lifecycle manager处理）", completedCount)
	} else {
		t.Logf("✅ 中间件没有产生重复的 'Request completed' 日志")
	}
	
	// 验证中间件确实记录了详情日志
	detailsCount := strings.Count(logContent, "Request details")
	if detailsCount == 0 {
		t.Error("❌ 中间件应该记录 'Request details' 日志")
	} else {
		t.Logf("✅ 中间件正确记录了详情日志")
	}
	
	// 验证DEBUG级别的请求接收日志
	receivedCount := strings.Count(logContent, "Request received")
	if receivedCount == 0 {
		t.Error("❌ 中间件应该记录 'Request received' DEBUG日志")
	} else {
		t.Logf("✅ 中间件正确记录了请求接收日志")
	}
}