package proxy

import (
	"fmt"
	"testing"
)

// TestErrorRecoveryManager_StreamingErrorClassification 测试错误分类修复
func TestErrorRecoveryManager_StreamingErrorClassification(t *testing.T) {
	erm := NewErrorRecoveryManager(nil)
	
	// 测试"streaming not supported"错误分类
	err := fmt.Errorf("streaming not supported")
	ctx := erm.ClassifyError(err, "test-req", "test-endpoint", "test-group", 1)
	
	if ctx.ErrorType != ErrorTypeUnknown {
		t.Errorf("期望错误类型为Unknown，实际为: %v", ctx.ErrorType)
	}
	
	if ctx.RetryableAfter != 0 {
		t.Errorf("streaming not supported错误应该不可重试，但RetryableAfter = %v", ctx.RetryableAfter)
	}
	
	t.Logf("✅ 错误分类修复验证通过")
	
	// 测试其他流处理错误
	streamErr := fmt.Errorf("stream parsing failed")
	streamCtx := erm.ClassifyError(streamErr, "test-req", "test-endpoint", "test-group", 1)
	
	if streamCtx.ErrorType != ErrorTypeStream {
		t.Errorf("期望错误类型为Stream，实际为: %v", streamCtx.ErrorType)
	}
	
	if streamCtx.RetryableAfter <= 0 {
		t.Errorf("流处理错误应该可重试，但RetryableAfter = %v", streamCtx.RetryableAfter)
	}
	
	t.Logf("✅ 流处理错误分类验证通过")
}