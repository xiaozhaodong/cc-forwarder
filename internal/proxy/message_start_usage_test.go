package proxy

import (
	"testing"
)

// TestMessageStartUsageExtraction 测试从message_start中提取usage信息的新功能
func TestMessageStartUsageExtraction(t *testing.T) {
	t.Run("message_start包含完整usage信息", func(t *testing.T) {
		tokenParser := NewTokenParserWithRequestID("test-msg-start-001")

		// 处理事件
		lines := []string{
			"event: message_start",
			`data: {"type":"message_start","message":{"id":"msg_014ZcQTryoxeMyCMKErsjnuc","type":"message","role":"assistant","model":"claude-sonnet-4-20250514","content":[],"usage":{"input_tokens":129351,"output_tokens":0,"cache_creation_input_tokens":1223,"cache_read_input_tokens":0}}}`,
			"",
		}

		for _, line := range lines {
			tokenParser.ParseSSELine(line)
		}

		// 验证partialUsage被正确设置
		partialUsage := tokenParser.GetPartialUsage()
		if partialUsage == nil {
			t.Error("❌ partialUsage应该不为nil，message_start包含usage信息")
			return
		}

		// 验证usage数据正确性
		if partialUsage.InputTokens != 129351 {
			t.Errorf("❌ InputTokens错误: 期望129351, 得到%d", partialUsage.InputTokens)
		}
		if partialUsage.OutputTokens != 0 {
			t.Errorf("❌ OutputTokens错误: 期望0, 得到%d", partialUsage.OutputTokens)
		}
		if partialUsage.CacheCreationTokens != 1223 {
			t.Errorf("❌ CacheCreationTokens错误: 期望1223, 得到%d", partialUsage.CacheCreationTokens)
		}

		t.Logf("✅ 成功从message_start提取usage: input=%d, output=%d, cache_create=%d, cache_read=%d",
			partialUsage.InputTokens, partialUsage.OutputTokens, partialUsage.CacheCreationTokens, partialUsage.CacheReadTokens)
	})

	t.Run("流中断场景_使用fallback机制", func(t *testing.T) {
		tokenParser := NewTokenParserWithRequestID("test-fallback-001")

		// 模拟message_start事件（包含usage）
		lines := []string{
			"event: message_start",
			`data: {"type":"message_start","message":{"id":"msg_123","model":"claude-3-sonnet","usage":{"input_tokens":5000,"output_tokens":100,"cache_creation_input_tokens":0,"cache_read_input_tokens":200}}}`,
			"",
		}

		for _, line := range lines {
			tokenParser.ParseSSELine(line)
		}

		// 验证partialUsage存在
		partialUsage := tokenParser.GetPartialUsage()
		if partialUsage == nil {
			t.Fatal("❌ partialUsage不应该为nil")
		}

		// 模拟流中断 - 没有收到message_delta事件
		// 此时finalUsage应该为nil，但GetFinalUsage()应该fallback到partialUsage

		finalUsage := tokenParser.GetFinalUsage()
		if finalUsage == nil {
			t.Error("❌ GetFinalUsage()应该fallback到partialUsage，不应该为nil")
			return
		}

		// 验证fallback数据的正确性
		if finalUsage.InputTokens != 5000 {
			t.Errorf("❌ Fallback InputTokens错误: 期望5000, 得到%d", finalUsage.InputTokens)
		}
		if finalUsage.OutputTokens != 100 {
			t.Errorf("❌ Fallback OutputTokens错误: 期望100, 得到%d", finalUsage.OutputTokens)
		}
		if finalUsage.CacheReadTokens != 200 {
			t.Errorf("❌ Fallback CacheReadTokens错误: 期望200, 得到%d", finalUsage.CacheReadTokens)
		}

		t.Logf("✅ Fallback机制正常工作: input=%d, output=%d, cache_read=%d",
			finalUsage.InputTokens, finalUsage.OutputTokens, finalUsage.CacheReadTokens)
	})

	t.Run("正常流程_message_delta覆盖message_start", func(t *testing.T) {
		tokenParser := NewTokenParserWithRequestID("test-normal-flow-001")

		// 1. 先处理message_start事件（包含初始usage）
		messageStartLines := []string{
			"event: message_start",
			`data: {"type":"message_start","message":{"id":"msg_123","model":"claude-3-sonnet","usage":{"input_tokens":5000,"output_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":200}}}`,
			"",
		}

		for _, line := range messageStartLines {
			tokenParser.ParseSSELine(line)
		}

		// 验证partialUsage存在
		partialUsage := tokenParser.GetPartialUsage()
		if partialUsage == nil {
			t.Fatal("❌ partialUsage不应该为nil")
		}

		// 2. 再处理message_delta事件（包含最终usage）
		messageDeltaLines := []string{
			"event: message_delta",
			`data: {"type":"message_delta","delta":{"type":"text","text":"Hello"},"usage":{"input_tokens":5000,"output_tokens":150,"cache_creation_input_tokens":0,"cache_read_input_tokens":200}}`,
			"",
		}

		for _, line := range messageDeltaLines {
			tokenParser.ParseSSELine(line)
		}

		// 3. 验证finalUsage优先于partialUsage
		finalUsage := tokenParser.GetFinalUsage()
		if finalUsage == nil {
			t.Error("❌ finalUsage不应该为nil")
			return
		}

		// 验证使用的是message_delta中的数据（output_tokens=150，而不是message_start的0）
		if finalUsage.OutputTokens != 150 {
			t.Errorf("❌ 应该使用message_delta的output_tokens(150)，而不是message_start的(0)，实际得到%d", finalUsage.OutputTokens)
		}

		t.Logf("✅ 正常流程工作正常: input=%d, output=%d（来自message_delta）",
			finalUsage.InputTokens, finalUsage.OutputTokens)
	})
}

// TestFallbackMechanismIntegration 测试fallback机制的完整性
func TestFallbackMechanismIntegration(t *testing.T) {
	t.Run("只有message_start_无message_delta", func(t *testing.T) {
		tokenParser := NewTokenParserWithRequestID("test-only-start-001")

		// 只处理message_start，模拟stream中断
		lines := []string{
			"event: message_start",
			`data: {"type":"message_start","message":{"model":"claude-3","usage":{"input_tokens":1000,"output_tokens":50}}}`,
			"",
		}

		for _, line := range lines {
			tokenParser.ParseSSELine(line)
		}

		// GetFinalUsage应该返回partialUsage的内容
		usage := tokenParser.GetFinalUsage()
		if usage == nil {
			t.Fatal("❌ 只有message_start时，GetFinalUsage()应该返回partialUsage")
		}

		expectedInput, expectedOutput := int64(1000), int64(50)
		if usage.InputTokens != expectedInput || usage.OutputTokens != expectedOutput {
			t.Errorf("❌ Fallback数据错误: 期望input=%d,output=%d, 得到input=%d,output=%d",
				expectedInput, expectedOutput, usage.InputTokens, usage.OutputTokens)
		}

		t.Log("✅ 只有message_start的场景fallback机制正常")
	})

	t.Run("既无message_start_usage也无message_delta", func(t *testing.T) {
		tokenParser := NewTokenParserWithRequestID("test-no-usage-001")

		// message_start没有usage字段
		lines := []string{
			"event: message_start",
			`data: {"type":"message_start","message":{"model":"claude-3"}}`,
			"",
		}

		for _, line := range lines {
			tokenParser.ParseSSELine(line)
		}

		// 应该返回nil
		usage := tokenParser.GetFinalUsage()
		if usage != nil {
			t.Errorf("❌ 没有usage信息时，GetFinalUsage()应该返回nil，实际得到%v", usage)
		}

		t.Log("✅ 无usage信息场景处理正常")
	})
}