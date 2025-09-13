package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestNoOpFlusher 测试无操作Flusher的行为
func TestNoOpFlusher(t *testing.T) {
	flusher := &noOpFlusher{}
	
	// 应该不会panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("noOpFlusher.Flush() 不应该panic: %v", r)
		}
	}()
	
	// 调用Flush方法
	flusher.Flush()
	
	t.Logf("✅ noOpFlusher.Flush() 执行成功，没有panic")
}

// TestResponseRecorderFlusherSupport 验证httptest.ResponseRecorder对Flusher的支持情况
func TestResponseRecorderFlusherSupport(t *testing.T) {
	recorder := httptest.NewRecorder()
	
	// 检查是否支持 Flusher
	if flusher, ok := interface{}(recorder).(http.Flusher); ok {
		t.Logf("✅ httptest.ResponseRecorder 支持 http.Flusher 接口")
		
		// 测试Flush不会panic
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("httptest.ResponseRecorder.Flush() panic: %v", r)
			}
		}()
		
		flusher.Flush()
		t.Logf("✅ httptest.ResponseRecorder.Flush() 执行成功")
	} else {
		t.Logf("ℹ️  httptest.ResponseRecorder 不支持 http.Flusher 接口")
		t.Logf("💡 这解释了为什么某些环境中会触发 Flusher 不支持的逻辑")
	}
}