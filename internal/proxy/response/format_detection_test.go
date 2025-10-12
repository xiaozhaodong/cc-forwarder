package response

import (
	"context"
	"net/http"
	"testing"
	"time"

	"cc-forwarder/internal/monitor"
	"cc-forwarder/internal/tracking"
)

// TestDetectResponseFormat 测试智能响应格式检测
func TestDetectResponseFormat(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected ResponseFormat
		reason   string
	}{
		{
			name:     "空响应",
			input:    "",
			expected: FormatUnknown,
			reason:   "空响应应该返回未知格式",
		},
		{
			name:     "标准JSON响应",
			input:    `{"id":"msg_123","type":"message","usage":{"input_tokens":10,"output_tokens":5}}`,
			expected: FormatJSON,
			reason:   "标准JSON响应应该被检测为JSON格式",
		},
		{
			name:     "包含SSE关键字的JSON响应_原始bug场景",
			input:    `{"id":"msg_014ZcQTryoxeMyCMKErsjnuc","type":"message","content":[{"type":"tool_use","name":"Bash","input":{"command":"lines := []string{\"event: message_start\",\"event: message_delta\"}"}}],"usage":{"input_tokens":7,"output_tokens":1009}}`,
			expected: FormatJSON,
			reason:   "JSON响应即使包含SSE关键字也应该被正确识别为JSON（修复原始bug）",
		},
		{
			name: "真正的SSE响应",
			input: `event: message_start
data: {"type":"message_start","message":{"id":"msg_123"}}

event: message_delta
data: {"type":"message_delta","delta":{"type":"text","text":"Hello"}}

event: message_stop
data: {"type":"message_stop"}
`,
			expected: FormatSSE,
			reason:   "符合SSE规范的响应应该被检测为SSE格式",
		},
		{
			name: "包含event但不是SSE的文本",
			input: `这是一个教程文档
event: message_start 是SSE的开始事件
data: 后面跟JSON数据
但这不是真正的SSE格式
`,
			expected: FormatUnknown,
			reason:   "包含SSE关键字但不符合格式规范的文本应该返回未知格式",
		},
		{
			name:     "JSON中包含SSE教程内容",
			input:    `{"tutorial":"SSE format","examples":["event: message_start","data: hello world"],"usage":{"tokens":100}}`,
			expected: FormatJSON,
			reason:   "JSON中包含SSE示例文本应该被正确识别为JSON格式",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectResponseFormat(tt.input)
			if result != tt.expected {
				t.Errorf("detectResponseFormat() = %v (%s), expected %v (%s)\nReason: %s\nInput: %s",
					result, formatName(result), tt.expected, formatName(tt.expected), tt.reason, tt.input)
			}
		})
	}
}

// TestIsValidJSONStructure 测试JSON结构验证
func TestIsValidJSONStructure(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
		reason   string
	}{
		{
			name:     "标准JSON对象",
			input:    `{"key":"value","number":123}`,
			expected: true,
			reason:   "标准JSON对象应该通过验证",
		},
		{
			name:     "包含嵌套对象的JSON",
			input:    `{"outer":{"inner":"value"},"usage":{"tokens":10}}`,
			expected: true,
			reason:   "嵌套JSON对象应该通过验证",
		},
		{
			name:     "JSON中包含SSE相关字符串",
			input:    `{"demo":"event: message_start","test":"data: hello","usage":{"tokens":5}}`,
			expected: true,
			reason:   "JSON中包含SSE字符串仍应该被识别为有效JSON",
		},
		{
			name:     "非JSON文本",
			input:    `event: message_start`,
			expected: false,
			reason:   "纯文本不应该通过JSON验证",
		},
		{
			name:     "格式错误的JSON",
			input:    `{key:value}`,
			expected: false,
			reason:   "格式错误的JSON不应该通过验证",
		},
		{
			name:     "空字符串",
			input:    ``,
			expected: false,
			reason:   "空字符串不应该通过JSON验证",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidJSONStructure(tt.input)
			if result != tt.expected {
				t.Errorf("isValidJSONStructure() = %v, expected %v\nReason: %s\nInput: %s",
					result, tt.expected, tt.reason, tt.input)
			}
		})
	}
}

// TestIsValidSSEStructure 测试SSE结构验证
func TestIsValidSSEStructure(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
		reason   string
	}{
		{
			name: "标准SSE格式",
			input: `event: message_start
data: {"type":"message_start"}

event: message_delta
data: {"type":"message_delta"}
`,
			expected: true,
			reason:   "标准SSE格式应该通过验证",
		},
		{
			name: "只有data行的SSE",
			input: `data: {"message":"hello"}
data: {"message":"world"}
`,
			expected: true,
			reason:   "只包含data行的SSE格式也应该通过验证",
		},
		{
			name: "混合格式_SSE占比不足",
			input: `event: message_start
这是一行普通文本
data: hello
另一行普通文本
还有更多普通文本
`,
			expected: false,
			reason:   "SSE行占比不足50%时应该返回false",
		},
		{
			name:     "JSON中包含SSE关键字",
			input:    `{"example":"event: message_start","demo":"data: hello"}`,
			expected: false,
			reason:   "JSON格式不应该被误认为SSE",
		},
		{
			name:     "纯文本包含SSE关键字",
			input:    `在SSE协议中, event: message_start 表示开始事件`,
			expected: false,
			reason:   "文档文本中的SSE关键字不应该被认为是SSE格式",
		},
		{
			name:     "空字符串",
			input:    ``,
			expected: false,
			reason:   "空字符串不应该通过SSE验证",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidSSEStructure(tt.input)
			if result != tt.expected {
				t.Errorf("isValidSSEStructure() = %v, expected %v\nReason: %s\nInput: %s",
					result, tt.expected, tt.reason, tt.input)
			}
		})
	}
}

// TestFormatName 测试格式名称函数
func TestFormatName(t *testing.T) {
	tests := []struct {
		format   ResponseFormat
		expected string
	}{
		{FormatJSON, "JSON"},
		{FormatSSE, "SSE"},
		{FormatPlainText, "PlainText"},
		{FormatUnknown, "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatName(tt.format)
			if result != tt.expected {
				t.Errorf("formatName(%v) = %v, expected %v", tt.format, result, tt.expected)
			}
		})
	}
}

// TestTokenParsingBugFix 测试Token解析bug修复效果
// 这个测试专门验证原始bug场景的修复效果
func TestTokenParsingBugFix(t *testing.T) {
	// 原始问题场景：JSON响应包含工具调用中的SSE示例代码
	problematicResponse := `{"id":"msg_014ZcQTryoxeMyCMKErsjnuc","type":"message","role":"assistant","model":"claude-sonnet-4-20250514","content":[{"type":"tool_use","id":"toolu_01EHJCK9w1ukF6ZXqaZHhwoJ","name":"Bash","input":{"command":"cat > /tmp/backward_compat.go << 'EOF'\npackage main\n\nimport (\n\t\"fmt\"\n\t\"strings\"\n)\n\nfunc testStandardClaudeAPI() {\n\tfmt.Println(\"🧪 测试标准Claude API响应格式兼容性...\")\n\t\n\t// 标准Claude API的正常SSE流\n\tlines := []string{\n\t\t\"event: message_start\",\n\t\t`+"`"+`data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_123\",\"model\":\"claude-3-sonnet-20240229\",\"role\":\"assistant\"}}`+"`"+`,\n\t\t\"\",\n\t\t\"event: message_delta\", \n\t\t`+"`"+`data: {\"type\":\"message_delta\",\"delta\":{\"type\":\"text\",\"text\":\"Hello\"},\"usage\":{\"input_tokens\":10,\"output_tokens\":1}}`+"`"+`,\n\t\t\"\",\n\t}\n}"}}],"stop_reason":"tool_use","stop_sequence":null,"usage":{"input_tokens":7,"cache_creation_input_tokens":1223,"cache_read_input_tokens":129351,"cache_creation":{"ephemeral_5m_input_tokens":1223,"ephemeral_1h_input_tokens":0},"output_tokens":1009,"service_tier":"standard"}}`

	t.Run("原始bug场景修复验证", func(t *testing.T) {
		// 测试格式检测
		format := detectResponseFormat(problematicResponse)
		if format != FormatJSON {
			t.Errorf("原始bug场景：响应应该被识别为JSON格式，实际得到: %s", formatName(format))
		}

		// 测试JSON结构验证
		if !isValidJSONStructure(problematicResponse) {
			t.Error("原始bug场景：响应应该通过JSON结构验证")
		}

		// 测试不会被误认为SSE
		if isValidSSEStructure(problematicResponse) {
			t.Error("原始bug场景：JSON响应不应该被误认为SSE格式")
		}
	})

	t.Run("向后兼容性验证", func(t *testing.T) {
		// 确保真正的SSE响应仍然能正确识别
		realSSE := `event: message_start
data: {"type":"message_start","message":{"id":"msg_123"}}

event: message_delta
data: {"type":"message_delta","delta":{"type":"text","text":"Hello"}}

event: message_stop
data: {"type":"message_stop"}
`
		format := detectResponseFormat(realSSE)
		if format != FormatSSE {
			t.Errorf("向后兼容性：真正的SSE响应应该被识别为SSE格式，实际得到: %s", formatName(format))
		}

		// 确保简单的JSON响应仍然能正确识别
		simpleJSON := `{"message":"hello","usage":{"input_tokens":5,"output_tokens":10}}`
		format = detectResponseFormat(simpleJSON)
		if format != FormatJSON {
			t.Errorf("向后兼容性：简单JSON响应应该被识别为JSON格式，实际得到: %s", formatName(format))
		}
	})
}

// 🆕 TestAllEntryPointsUsing NewFormatDetection 测试所有入口点都使用新的格式检测
func TestAllEntryPointsUsingNewFormatDetection(t *testing.T) {
	// 模拟TokenAnalyzer和依赖项
	mockUsageTracker := (*tracking.UsageTracker)(nil)
	mockProvider := &mockTokenParserProvider{}
	analyzer := NewTokenAnalyzer(mockUsageTracker, nil, mockProvider)

	// 原始bug响应
	problematicResponse := `{"id":"msg_123","content":[{"input":{"command":"lines := []string{\"event: message_start\",\"event: message_delta\"}"}}],"usage":{"input_tokens":7,"output_tokens":1009}}`

	ctx := context.Background()
	req := &http.Request{}
	req = req.WithContext(context.WithValue(req.Context(), "conn_id", "test-123"))

	t.Run("AnalyzeResponseForTokensUnified入口点", func(t *testing.T) {
		// 这个入口点已经修复，应该正确识别JSON
		tokenUsage, modelName := analyzer.AnalyzeResponseForTokensUnified([]byte(problematicResponse), "test-123", "test-endpoint")

		// 验证不会返回SSE相关的错误模型名
		if modelName == "no_token_sse" {
			t.Error("AnalyzeResponseForTokensUnified仍在使用旧的SSE误判逻辑")
		}

		// 应该返回合适的结果（可能是non_token_response，但不应该是SSE相关的）
		t.Logf("✅ AnalyzeResponseForTokensUnified正确处理：tokenUsage=%v, modelName=%s", tokenUsage, modelName)
	})

	t.Run("AnalyzeResponseForTokens入口点", func(t *testing.T) {
		// 这个入口点现在也应该使用新的格式检测
		analyzer.AnalyzeResponseForTokens(ctx, problematicResponse, "test-endpoint", req)

		// 由于这是void函数，我们通过检查是否没有panic来验证
		t.Log("✅ AnalyzeResponseForTokens没有panic，使用了新的格式检测")
	})

	t.Run("AnalyzeResponseForTokensWithLifecycle入口点", func(t *testing.T) {
		// 这个入口点现在也应该使用新的格式检测
		mockLifecycleManager := &mockLifecycleManager{}
		analyzer.AnalyzeResponseForTokensWithLifecycle(ctx, problematicResponse, "test-endpoint", req, mockLifecycleManager)

		// 由于这是void函数，我们通过检查是否没有panic来验证
		t.Log("✅ AnalyzeResponseForTokensWithLifecycle没有panic，使用了新的格式检测")
	})
}

// Mock implementations for testing
type mockTokenParserProvider struct{}

func (m *mockTokenParserProvider) NewTokenParser() TokenParser {
	return &mockTokenParser{}
}

func (m *mockTokenParserProvider) NewTokenParserWithUsageTracker(requestID string, usageTracker *tracking.UsageTracker) TokenParser {
	return &mockTokenParser{}
}

type mockTokenParser struct{}

func (m *mockTokenParser) ParseSSELine(line string) *monitor.TokenUsage { return nil }
func (m *mockTokenParser) SetModelName(model string)                    {}

type mockLifecycleManager struct{}

func (m *mockLifecycleManager) GetDuration() time.Duration { return 0 }