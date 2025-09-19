package proxy

import (
	"context"
	"errors"
	"testing"

	"cc-forwarder/internal/tracking"
)

// TestClientCancelDetection 测试客户端取消检测功能
func TestClientCancelDetection(t *testing.T) {
	// 创建错误恢复管理器
	tracker := &tracking.UsageTracker{} // Mock tracker
	errorRecovery := NewErrorRecoveryManager(tracker)
	
	// 测试案例
	tests := []struct {
		name        string
		err         error
		expectedType ErrorType
		shouldRetry bool
	}{
		{
			name:        "Context Canceled",
			err:         context.Canceled,
			expectedType: ErrorTypeClientCancel,
			shouldRetry: false,
		},
		{
			name:        "Custom Cancel Error",
			err:         errors.New("context canceled"),
			expectedType: ErrorTypeClientCancel, 
			shouldRetry: false,
		},
		{
			name:        "Network Error",
			err:         errors.New("connection refused"),
			expectedType: ErrorTypeNetwork,
			shouldRetry: true,
		},
		{
			name:        "Timeout Error", 
			err:         context.DeadlineExceeded,
			expectedType: ErrorTypeTimeout,
			shouldRetry: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 分类错误
			errorCtx := errorRecovery.ClassifyError(tt.err, "test-req", "test-endpoint", "test-group", 0)
			
			// 验证错误类型
			if errorCtx.ErrorType != tt.expectedType {
				t.Errorf("Expected error type %v, got %v", tt.expectedType, errorCtx.ErrorType)
			}
			
			// 验证重试判断
			shouldRetry := errorRecovery.ShouldRetry(errorCtx)
			if shouldRetry != tt.shouldRetry {
				t.Errorf("Expected shouldRetry %v, got %v", tt.shouldRetry, shouldRetry)
			}
			
			// 特别验证客户端取消错误不可重试
			if tt.expectedType == ErrorTypeClientCancel && shouldRetry {
				t.Error("Client cancel error should never be retryable")
			}
		})
	}
}

// TestLifecycleManagerClientCancel 测试生命周期管理器处理客户端取消
func TestLifecycleManagerClientCancel(t *testing.T) {
	// 创建生命周期管理器
	lifecycle := NewRequestLifecycleManager(nil, nil, "test-req-123")
	lifecycle.SetEndpoint("test-endpoint", "test-group")
	
	// 模拟客户端取消错误
	cancelErr := context.Canceled
	
	// 处理错误
	lifecycle.HandleError(cancelErr)
	
	// 验证状态
	if lifecycle.GetLastStatus() != "cancelled" {
		t.Errorf("Expected status 'cancelled', got '%s'", lifecycle.GetLastStatus())
	}
	
	// 验证不应该重试 - 直接使用错误恢复管理器判断
	errorCtx := lifecycle.errorRecovery.ClassifyError(cancelErr, "test-req-123", "test-endpoint", "test-group", 0)
	if lifecycle.errorRecovery.ShouldRetry(errorCtx) {
		t.Error("Client cancelled request should not be retryable")
	}
	
	// 验证错误记录
	if lifecycle.GetLastError() != cancelErr {
		t.Error("Last error should be the cancel error")
	}
}

// TestStreamProcessingClientCancel 测试流式处理中的客户端取消
func TestStreamProcessingClientCancel(t *testing.T) {
	// 创建上下文并取消
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消
	
	// 创建错误恢复管理器
	errorRecovery := NewErrorRecoveryManager(nil)
	
	// 模拟流式处理错误（context canceled）
	streamErr := ctx.Err() // 这会是 context.Canceled
	
	// 分类错误
	errorCtx := errorRecovery.ClassifyError(streamErr, "test-stream", "test-endpoint", "test-group", 1)
	
	// 验证被正确分类为客户端取消
	if errorCtx.ErrorType != ErrorTypeClientCancel {
		t.Errorf("Stream cancel error should be classified as ClientCancel, got %v", errorCtx.ErrorType)
	}
	
	// 验证不应该重试
	if errorRecovery.ShouldRetry(errorCtx) {
		t.Error("Stream cancel error should not be retryable")
	}
	
	// 验证重试延迟为0（不可重试）
	if errorCtx.RetryableAfter != 0 {
		t.Errorf("Cancel error should have RetryableAfter = 0, got %v", errorCtx.RetryableAfter)
	}
}

// TestErrorTypePriority 测试错误类型优先级
func TestErrorTypePriority(t *testing.T) {
	errorRecovery := NewErrorRecoveryManager(nil)
	
	// 测试混合错误（取消应该有最高优先级）
	mixedErr := errors.New("connection timeout: context canceled")
	
	errorCtx := errorRecovery.ClassifyError(mixedErr, "test", "endpoint", "group", 0)
	
	// 应该被分类为客户端取消（虽然包含timeout字样）
	if errorCtx.ErrorType != ErrorTypeClientCancel {
		t.Errorf("Mixed error with 'context canceled' should be classified as ClientCancel, got %v", errorCtx.ErrorType)
	}
}

// BenchmarkErrorClassification 基准测试错误分类性能
func BenchmarkErrorClassification(b *testing.B) {
	errorRecovery := NewErrorRecoveryManager(nil)
	cancelErr := context.Canceled
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		errorCtx := errorRecovery.ClassifyError(cancelErr, "bench-req", "bench-endpoint", "bench-group", i)
		if errorCtx.ErrorType != ErrorTypeClientCancel {
			b.Error("Unexpected error classification")
		}
	}
}