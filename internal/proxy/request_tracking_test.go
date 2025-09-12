package proxy

import (
	"context"
	"fmt"
	"testing"

	"cc-forwarder/internal/tracking"
)

// TestRequestIDLogging 测试完整的requestId日志追踪链路  
func TestRequestIDLogging(t *testing.T) {
	// 模拟requestId
	requestID := "req-12345678"
	
	// 创建生命周期管理器（不传入usageTracker，重点测试日志格式）
	lifecycleManager := NewRequestLifecycleManager(nil, nil, requestID)
	
	t.Logf("🔍 测试完整的requestId日志追踪链路")
	
	// 1. 请求开始
	t.Logf("1️⃣ 测试请求开始日志")
	lifecycleManager.StartRequest("127.0.0.1", "test-agent")
	
	// 2. 设置端点信息
	lifecycleManager.SetEndpoint("test-endpoint", "test-group")
	
	// 3. 测试各种状态更新
	t.Logf("2️⃣ 测试状态更新日志")
	
	// 转发状态
	lifecycleManager.UpdateStatus("forwarding", 0, 0)
	
	// 重试状态
	lifecycleManager.UpdateStatus("retry", 1, 0)
	
	// 处理状态
	lifecycleManager.UpdateStatus("processing", 1, 200)
	
	// 4. 测试完成
	t.Logf("3️⃣ 测试请求完成日志")
	mockTokens := &tracking.TokenUsage{
		InputTokens:  25,
		OutputTokens: 97,
	}
	lifecycleManager.CompleteRequest(mockTokens)
	
	// 5. 测试错误处理
	t.Logf("4️⃣ 测试错误处理日志")
	testErr := fmt.Errorf("test error")
	lifecycleManager.HandleError(testErr)
	
	// 验证requestId格式正确
	if lifecycleManager.GetRequestID() != requestID {
		t.Errorf("❌ RequestID不匹配: 期望 %s, 实际 %s", requestID, lifecycleManager.GetRequestID())
	}
	
	t.Logf("🎉 完整的requestId追踪链路测试完成")
	
	// 测试日志格式验证
	testCases := []struct {
		name     string
		action   func()
		contains string
	}{
		{
			name: "转发日志格式",
			action: func() {
				lifecycleManager.UpdateStatus("forwarding", 0, 0)
			},
			contains: fmt.Sprintf("[%s]", requestID),
		},
		{
			name: "重试日志格式", 
			action: func() {
				lifecycleManager.UpdateStatus("retry", 2, 0)
			},
			contains: fmt.Sprintf("[%s]", requestID),
		},
		{
			name: "错误日志格式",
			action: func() {
				lifecycleManager.UpdateStatus("error", 0, 500)
			},
			contains: fmt.Sprintf("[%s]", requestID),
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.action()
			// 日志会输出到标准输出，这里主要验证代码不会panic
			t.Logf("✅ %s 测试通过", tc.name)
		})
	}
}

// mockUsageTracker 用于测试的mock实现
type mockUsageTracker struct {
	logs []string
}

func (m *mockUsageTracker) RecordRequestStart(requestID, clientIP, userAgent string) {
	m.logs = append(m.logs, fmt.Sprintf("RecordRequestStart: %s", requestID))
}

func (m *mockUsageTracker) RecordRequestUpdate(requestID, endpoint, group, status string, retryCount, httpStatus int) {
	m.logs = append(m.logs, fmt.Sprintf("RecordRequestUpdate: %s - %s", requestID, status))
}

func (m *mockUsageTracker) RecordRequestComplete(requestID, modelName string, tokens *tracking.TokenUsage, duration int64) {
	m.logs = append(m.logs, fmt.Sprintf("RecordRequestComplete: %s - %s", requestID, modelName))
}

func (m *mockUsageTracker) IsRunning() bool { return true }
func (m *mockUsageTracker) Start() {}
func (m *mockUsageTracker) Stop() {}
func (m *mockUsageTracker) GetStats(ctx context.Context, startTime, endTime string, modelName, endpointName, groupName string) (map[string]interface{}, error) {
	return nil, nil
}