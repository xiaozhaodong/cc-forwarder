package proxy

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"cc-forwarder/internal/monitor"
	"cc-forwarder/internal/tracking"
)

// ParseResult 解析结果结构体
// 用于将Token解析与状态记录分离，支持职责纯化
type ParseResult struct {
	TokenUsage  *tracking.TokenUsage
	ModelName   string
	ErrorInfo   *ErrorInfo
	IsCompleted bool
	Status      string
}

// ErrorInfo 错误信息结构体
type ErrorInfo struct {
	Type    string
	Message string
}

// TokenParserInterface 统一的Token解析接口
// 根据STREAMING_REFACTOR_PROPOSAL.md方案设计
type TokenParserInterface interface {
	ParseMessageStart(line string) *ModelInfo
	ParseMessageDelta(line string) *tracking.TokenUsage
	SetModel(modelName string)
	GetFinalUsage() *tracking.TokenUsage
	Reset()

	// V2 职责纯化方法
	ParseSSELineV2(line string) *ParseResult
}

// ModelInfo 模型信息结构体
type ModelInfo struct {
	Model string `json:"model"`
}

// UsageData represents the usage field in Claude API SSE events
type UsageData struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
}

// MessageStartData represents the message object in message_start events
type MessageStartData struct {
	ID      string        `json:"id"`
	Type    string        `json:"type"`
	Role    string        `json:"role"`
	Model   string        `json:"model"`
	Content []interface{} `json:"content"`
	Usage   *UsageData    `json:"usage,omitempty"`
}

// MessageStart represents the structure of message_start events
type MessageStart struct {
	Type    string            `json:"type"`
	Message *MessageStartData `json:"message"`
}

// MessageDelta represents the structure of message_delta events
type MessageDelta struct {
	Type  string      `json:"type"`
	Delta interface{} `json:"delta"`
	Usage *UsageData  `json:"usage,omitempty"`
}

// SSEErrorData represents the structure of error events in SSE streams
type SSEErrorData struct {
	Type  string `json:"type"`
	Error struct {
		Type      string `json:"type"`
		Message   string `json:"message"`
		RequestID string `json:"request_id,omitempty"`
	} `json:"error"`
}

// Status constants for request processing states
const (
	StatusCompleted    = "completed"     // 真正成功（有Token或正常响应）
	StatusErrorAPI     = "error_api"     // API层错误（overloaded等）
	StatusErrorNetwork = "error_network" // 网络层错误（超时等）
	StatusProcessing   = "processing"    // 处理中
)

// TokenParser handles parsing of SSE events for token usage extraction
// 实现TokenParserInterface接口
type TokenParser struct {
	// Buffer to collect multi-line JSON data
	eventBuffer    strings.Builder
	currentEvent   string
	collectingData bool
	// Request ID for logging purposes
	requestID string
	// Model name extracted from message_start event
	modelName string
	// Usage tracker for recording token usage and costs
	usageTracker *tracking.UsageTracker
	// Start time for duration calculation
	startTime time.Time
	// Final token usage for accumulation
	finalUsage *tracking.TokenUsage
	// Partial usage for handling interruptions
	partialUsage *tracking.TokenUsage
}

// NewTokenParser creates a new token parser instance
func NewTokenParser() *TokenParser {
	return &TokenParser{
		startTime: time.Now(),
	}
}

// NewTokenParserWithRequestID creates a new token parser instance with request ID
func NewTokenParserWithRequestID(requestID string) *TokenParser {
	return &TokenParser{
		requestID: requestID,
		startTime: time.Now(),
	}
}

// NewTokenParserWithUsageTracker creates a new token parser instance with usage tracker
func NewTokenParserWithUsageTracker(requestID string, usageTracker *tracking.UsageTracker) *TokenParser {
	return &TokenParser{
		requestID:    requestID,
		usageTracker: usageTracker,
		startTime:    time.Now(),
	}
}

// ParseSSELineV2 新版本的SSE解析方法
// 返回 ParseResult 而不是直接调用 usageTracker
func (tp *TokenParser) ParseSSELineV2(line string) *ParseResult {
	line = strings.TrimSpace(line)

	// Handle event type lines - support both "event: " and "event:" formats
	if strings.HasPrefix(line, "event:") {
		var eventType string
		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
		} else {
			eventType = strings.TrimPrefix(line, "event:")
		}
		tp.currentEvent = eventType
		// Collect data for message_start (model info), message_delta (usage), and error events
		tp.collectingData = (eventType == "message_delta" || eventType == "message_start" || eventType == "error")
		tp.eventBuffer.Reset()
		return nil
	}

	// Handle data lines - support both "data: " and "data:" formats
	if strings.HasPrefix(line, "data:") && tp.collectingData {
		var dataContent string
		if strings.HasPrefix(line, "data: ") {
			dataContent = strings.TrimPrefix(line, "data: ")
		} else {
			dataContent = strings.TrimPrefix(line, "data:")
		}
		tp.eventBuffer.WriteString(dataContent)
		return nil
	}

	// Handle empty lines that signal end of SSE event
	if line == "" && tp.collectingData && tp.eventBuffer.Len() > 0 {
		if tp.currentEvent == "message_start" {
			// Parse message_start for model info only (no ParseResult needed)
			tp.parseMessageStart()
			return nil
		} else if tp.currentEvent == "message_delta" {
			// Parse message_delta using new V2 method
			return tp.parseMessageDeltaV2()
		} else if tp.currentEvent == "error" {
			// Parse error event using new V2 method
			return tp.parseErrorEventV2()
		}
	}

	return nil
}

// ParseSSELine processes a single line from SSE stream and extracts token usage if found
func (tp *TokenParser) ParseSSELine(line string) *monitor.TokenUsage {
	line = strings.TrimSpace(line)

	// Handle event type lines - support both "event: " and "event:" formats
	if strings.HasPrefix(line, "event:") {
		var eventType string
		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
		} else {
			eventType = strings.TrimPrefix(line, "event:")
		}
		tp.currentEvent = eventType
		// Collect data for message_start (model info), message_delta (usage), and error events
		tp.collectingData = (eventType == "message_delta" || eventType == "message_start" || eventType == "error")
		tp.eventBuffer.Reset()
		return nil
	}

	// Handle data lines - support both "data: " and "data:" formats
	if strings.HasPrefix(line, "data:") && tp.collectingData {
		var dataContent string
		if strings.HasPrefix(line, "data: ") {
			dataContent = strings.TrimPrefix(line, "data: ")
		} else {
			dataContent = strings.TrimPrefix(line, "data:")
		}
		tp.eventBuffer.WriteString(dataContent)
		return nil
	}

	// Handle empty lines that signal end of SSE event
	if line == "" && tp.collectingData && tp.eventBuffer.Len() > 0 {
		if tp.currentEvent == "message_start" {
			// Parse message_start for both model info and token usage
			return tp.parseMessageStart()
		} else if tp.currentEvent == "message_delta" {
			// Parse message_delta for usage info
			return tp.parseMessageDelta()
		} else if tp.currentEvent == "error" {
			// Parse error event and record as API error
			// 🚫 修复：注释掉违规的直接usageTracker调用，让生命周期管理器处理
			// tp.parseErrorEvent()
			slog.Info(fmt.Sprintf("❌ [错误事件] [%s] 检测到API错误事件", tp.requestID))
			return nil // Error events don't return TokenUsage
		}
	}

	return nil
}

// parseMessageStart parses the collected message_start JSON data to extract model info only
func (tp *TokenParser) parseMessageStart() *monitor.TokenUsage {
	defer func() {
		tp.eventBuffer.Reset()
		tp.collectingData = false
		tp.currentEvent = ""
	}()

	jsonData := tp.eventBuffer.String()
	if jsonData == "" {
		return nil
	}

	// Parse the JSON data
	var messageStart MessageStart
	if err := json.Unmarshal([]byte(jsonData), &messageStart); err != nil {
		return nil
	}

	// Extract model name if available
	if messageStart.Message != nil && messageStart.Message.Model != "" {
		tp.modelName = messageStart.Message.Model

		// Log model extraction (不处理token usage) - 始终包含requestID
		slog.Info(fmt.Sprintf("🎯 [模型提取] [%s] 从message_start事件中提取模型信息: %s",
			tp.requestID, tp.modelName))
	}

	// ⚠️ 重要：message_start事件不处理token usage信息
	// Token usage信息应该从message_delta事件中获取，该事件包含完整的使用统计

	return nil
}

// parseMessageDeltaV2 新版本的message_delta解析方法
// 返回 ParseResult 而不是直接调用 usageTracker
func (tp *TokenParser) parseMessageDeltaV2() *ParseResult {
	defer func() {
		tp.eventBuffer.Reset()
		tp.collectingData = false
		tp.currentEvent = ""
	}()

	jsonData := tp.eventBuffer.String()
	if jsonData == "" {
		return nil
	}

	// Parse the JSON data
	var messageDelta MessageDelta
	if err := json.Unmarshal([]byte(jsonData), &messageDelta); err != nil {
		return nil
	}

	// Check if this message_delta contains usage information
	if messageDelta.Usage == nil {
		// ⚠️ 兼容性处理：对于非Claude端点，message_delta可能不包含usage信息
		// 这种情况下需要fallback机制来标记请求完成
		if tp.requestID != "" {
			// Use "default" as model name if no model was extracted from message_start
			modelName := tp.modelName
			if modelName == "" {
				modelName = "default"
			}

			slog.Info(fmt.Sprintf("🎯 [无Token响应] [%s] message_delta事件不包含token信息，标记为完成 - 模型: %s",
				tp.requestID, modelName))

			// 返回空Token的完成结果
			return &ParseResult{
				TokenUsage: &tracking.TokenUsage{
					InputTokens:         0,
					OutputTokens:        0,
					CacheCreationTokens: 0,
					CacheReadTokens:     0,
				},
				ModelName:   modelName,
				IsCompleted: true,
				Status:      "non_token_response",
			}
		}
		return nil
	}

	// ✅ 设置finalUsage供GetFinalUsage()方法使用
	tp.finalUsage = &tracking.TokenUsage{
		InputTokens:         messageDelta.Usage.InputTokens,
		OutputTokens:        messageDelta.Usage.OutputTokens,
		CacheCreationTokens: messageDelta.Usage.CacheCreationInputTokens,
		CacheReadTokens:     messageDelta.Usage.CacheReadInputTokens,
	}

	// 返回解析结果而不是直接记录
	return &ParseResult{
		TokenUsage:  tp.finalUsage,
		ModelName:   tp.modelName,
		IsCompleted: true,
		Status:      "completed",
	}
}

// parseMessageDelta parses the collected message_delta JSON data for complete token usage
func (tp *TokenParser) parseMessageDelta() *monitor.TokenUsage {
	defer func() {
		tp.eventBuffer.Reset()
		tp.collectingData = false
		tp.currentEvent = ""
	}()

	jsonData := tp.eventBuffer.String()
	if jsonData == "" {
		return nil
	}

	// Parse the JSON data
	var messageDelta MessageDelta
	if err := json.Unmarshal([]byte(jsonData), &messageDelta); err != nil {
		return nil
	}

	// Check if this message_delta contains usage information
	if messageDelta.Usage == nil {
		// ⚠️ 兼容性处理：对于非Claude端点，message_delta可能不包含usage信息
		// 这种情况下需要fallback机制来标记请求完成
		if tp.requestID != "" {
			// Use "default" as model name if no model was extracted from message_start
			modelName := tp.modelName
			if modelName == "" {
				modelName = "default"
			}

			slog.Info(fmt.Sprintf("🎯 [无Token响应] [%s] message_delta事件不包含token信息，标记为完成 - 模型: %s",
				tp.requestID, modelName))
		}
		return nil
	}

	// Convert to our TokenUsage format
	tokenUsage := &monitor.TokenUsage{
		InputTokens:         messageDelta.Usage.InputTokens,
		OutputTokens:        messageDelta.Usage.OutputTokens,
		CacheCreationTokens: messageDelta.Usage.CacheCreationInputTokens,
		CacheReadTokens:     messageDelta.Usage.CacheReadInputTokens,
	}

	// ✅ 设置finalUsage供GetFinalUsage()方法使用
	tp.finalUsage = &tracking.TokenUsage{
		InputTokens:         messageDelta.Usage.InputTokens,
		OutputTokens:        messageDelta.Usage.OutputTokens,
		CacheCreationTokens: messageDelta.Usage.CacheCreationInputTokens,
		CacheReadTokens:     messageDelta.Usage.CacheReadInputTokens,
	}

	// 移除重复的日志记录 - 由StreamProcessor统一处理
	// if tp.requestID != "" {
	//	slog.Info(fmt.Sprintf("🪙 [Token使用统计] [%s] 从message_delta事件中提取完整令牌使用情况 -%s 输入: %d, 输出: %d, 缓存创建: %d, 缓存读取: %d",
	//		tp.requestID, modelInfo, tokenUsage.InputTokens, tokenUsage.OutputTokens, tokenUsage.CacheCreationTokens, tokenUsage.CacheReadTokens))
	// } else {
	//	slog.Info(fmt.Sprintf("🪙 [Token使用统计] 从message_delta事件中提取完整令牌使用情况 -%s 输入: %d, 输出: %d, 缓存创建: %d, 缓存读取: %d",
	//		modelInfo, tokenUsage.InputTokens, tokenUsage.OutputTokens, tokenUsage.CacheCreationTokens, tokenUsage.CacheReadTokens))
	// }

	// ✅ TokenParser只负责解析，不直接调用usage tracker
	// usage tracker的调用由上层（StreamProcessor或Handler）统一管理
	// if tp.usageTracker != nil && tp.requestID != "" {
	//	// Calculate duration since parser creation
	//	duration := time.Since(tp.startTime)
	//
	//	// Convert monitor.TokenUsage to tracking.TokenUsage
	//	trackingTokens := &tracking.TokenUsage{
	//		InputTokens:         tokenUsage.InputTokens,
	//		OutputTokens:        tokenUsage.OutputTokens,
	//		CacheCreationTokens: tokenUsage.CacheCreationTokens,
	//		CacheReadTokens:     tokenUsage.CacheReadTokens,
	//	}
	//
	//	// Record the completion with token usage and cost information
	//	tp.usageTracker.RecordRequestComplete(tp.requestID, tp.modelName, trackingTokens, duration)
	// }

	return tokenUsage
}

// Reset clears the parser state
func (tp *TokenParser) Reset() {
	tp.eventBuffer.Reset()
	tp.currentEvent = ""
	tp.collectingData = false
	tp.finalUsage = nil
	tp.partialUsage = nil
	tp.startTime = time.Now()
}

// parseErrorEventV2 新版本的错误事件解析方法
// 返回 ParseResult 而不是直接调用 usageTracker
func (tp *TokenParser) parseErrorEventV2() *ParseResult {
	defer func() {
		tp.eventBuffer.Reset()
		tp.collectingData = false
		tp.currentEvent = ""
	}()

	jsonData := tp.eventBuffer.String()
	if jsonData == "" {
		return nil
	}

	// Parse the error JSON data
	var errorData SSEErrorData
	if err := json.Unmarshal([]byte(jsonData), &errorData); err != nil {
		if tp.requestID != "" {
			slog.Info(fmt.Sprintf("⚠️ [SSE错误解析] [%s] 无法解析错误数据: %s", tp.requestID, jsonData))
		}
		return nil
	}

	// Extract error type and message
	errorType := errorData.Error.Type
	errorMessage := errorData.Error.Message
	if errorType == "" {
		errorType = "unknown_error"
	}
	if errorMessage == "" {
		errorMessage = "Unknown error"
	}

	// Log the API error
	if tp.requestID != "" {
		slog.Info(fmt.Sprintf("❌ [API错误] [%s] 错误类型: %s, 错误信息: %s",
			tp.requestID, errorType, errorMessage))
	} else {
		slog.Info(fmt.Sprintf("❌ [API错误] 错误类型: %s, 错误信息: %s",
			errorType, errorMessage))
	}

	// 返回解析结果而不是直接记录到 usageTracker
	errorModelName := fmt.Sprintf("error:%s", errorType)

	return &ParseResult{
		TokenUsage: &tracking.TokenUsage{
			InputTokens:         0,
			OutputTokens:        0,
			CacheCreationTokens: 0,
			CacheReadTokens:     0,
		},
		ModelName:   errorModelName,
		ErrorInfo:   &ErrorInfo{Type: errorType, Message: errorMessage},
		IsCompleted: true,
		Status:      StatusErrorAPI,
	}
}

// parseErrorEvent parses SSE error events and records them as API errors
func (tp *TokenParser) parseErrorEvent() {
	defer func() {
		tp.eventBuffer.Reset()
		tp.collectingData = false
		tp.currentEvent = ""
	}()

	jsonData := tp.eventBuffer.String()
	if jsonData == "" {
		return
	}

	// Parse the error JSON data
	var errorData SSEErrorData
	if err := json.Unmarshal([]byte(jsonData), &errorData); err != nil {
		if tp.requestID != "" {
			slog.Info(fmt.Sprintf("⚠️ [SSE错误解析] [%s] 无法解析错误数据: %s", tp.requestID, jsonData))
		}
		return
	}

	// Extract error type and message
	errorType := errorData.Error.Type
	errorMessage := errorData.Error.Message
	if errorType == "" {
		errorType = "unknown_error"
	}
	if errorMessage == "" {
		errorMessage = "Unknown error"
	}

	// Log the API error
	if tp.requestID != "" {
		slog.Info(fmt.Sprintf("❌ [API错误] [%s] 错误类型: %s, 错误信息: %s",
			tp.requestID, errorType, errorMessage))
	} else {
		slog.Info(fmt.Sprintf("❌ [API错误] 错误类型: %s, 错误信息: %s",
			errorType, errorMessage))
	}

	// ℹ️ 返回错误信息，由Handler通过生命周期管理器记录
	// 不再直接调用usageTracker，遵循架构原则
	// TokenParser只负责解析和返回结果，不直接记录到数据库
	if tp.requestID != "" {
		slog.Info(fmt.Sprintf("🏁 [API错误解析] [%s] 错误信息已解析: %s - %s", tp.requestID, errorType, errorMessage))
	}

	// 更新内部状态，由上层通过生命周期管理器记录完成状态
	tp.finalUsage = &tracking.TokenUsage{
		InputTokens:         0,
		OutputTokens:        0,
		CacheCreationTokens: 0,
		CacheReadTokens:     0,
	}
	tp.modelName = fmt.Sprintf("error:%s", errorType)
	// TokenParser不需要维护status字段，由上层处理
}

// SetModelName allows setting the model name directly (useful for JSON response parsing)
func (tp *TokenParser) SetModelName(modelName string) {
	tp.modelName = modelName
}

// ParseMessageStart 实现接口方法 - 解析message_start事件提取模型信息
func (tp *TokenParser) ParseMessageStart(line string) *ModelInfo {
	if !strings.HasPrefix(line, "data: ") {
		return nil
	}

	data := line[6:] // Remove "data: "
	if strings.TrimSpace(data) == "[DONE]" {
		return nil
	}

	var event map[string]interface{}
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return nil
	}

	if event["type"] == "message_start" {
		if message, ok := event["message"].(map[string]interface{}); ok {
			if model, ok := message["model"].(string); ok {
				tp.SetModel(model)
				return &ModelInfo{Model: model}
			}
		}
	}

	return nil
}

// ParseMessageDelta 实现接口方法 - 解析message_delta事件提取Token使用统计
func (tp *TokenParser) ParseMessageDelta(line string) *tracking.TokenUsage {
	if !strings.HasPrefix(line, "data: ") {
		return nil
	}

	data := line[6:] // Remove "data: "
	if strings.TrimSpace(data) == "[DONE]" {
		return nil
	}

	var event map[string]interface{}
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return nil
	}

	if event["type"] == "message_delta" {
		if usage, ok := event["usage"].(map[string]interface{}); ok {
			tokenUsage := &tracking.TokenUsage{}

			if inputTokens, ok := usage["input_tokens"].(float64); ok {
				tokenUsage.InputTokens = int64(inputTokens)
			}
			if outputTokens, ok := usage["output_tokens"].(float64); ok {
				tokenUsage.OutputTokens = int64(outputTokens)
			}
			if cacheCreation, ok := usage["cache_creation_input_tokens"].(float64); ok {
				tokenUsage.CacheCreationTokens = int64(cacheCreation)
			}
			if cacheRead, ok := usage["cache_read_input_tokens"].(float64); ok {
				tokenUsage.CacheReadTokens = int64(cacheRead)
			}

			// 保存最终使用统计
			tp.finalUsage = tokenUsage

			return tokenUsage
		}
	}

	return nil
}

// SetModel 实现接口方法 - 设置模型名称
func (tp *TokenParser) SetModel(modelName string) {
	tp.modelName = modelName
}

// GetFinalUsage 实现接口方法 - 获取最终Token使用统计
func (tp *TokenParser) GetFinalUsage() *tracking.TokenUsage {
	return tp.finalUsage
}

// GetModelName 获取模型名称
func (tp *TokenParser) GetModelName() string {
	return tp.modelName
}

// GetPartialUsage 获取部分Token使用统计（用于网络中断恢复）
func (tp *TokenParser) GetPartialUsage() *tracking.TokenUsage {
	if tp.partialUsage != nil {
		return tp.partialUsage
	}
	return tp.finalUsage
}

// FlushPendingEvent 强制解析缓存中的待处理事件
// 在流结束或连接中断时调用，确保不会因为缺少终止空行而丢失 usage 信息
func (tp *TokenParser) FlushPendingEvent() *ParseResult {
	// 只有在收集数据且缓存非空时才需要flush
	if !tp.collectingData || tp.eventBuffer.Len() == 0 {
		return nil
	}

	// 根据当前事件类型调用相应的解析方法
	switch tp.currentEvent {
	case "message_delta":
		if tp.requestID != "" {
			slog.Info(fmt.Sprintf("🔄 [事件Flush] [%s] 强制解析缓存的message_delta事件", tp.requestID))
		}
		return tp.parseMessageDeltaV2()
	case "message_start":
		if tp.requestID != "" {
			slog.Info(fmt.Sprintf("🔄 [事件Flush] [%s] 强制解析缓存的message_start事件", tp.requestID))
		}
		tp.parseMessageStart()
		return nil
	case "error":
		if tp.requestID != "" {
			slog.Info(fmt.Sprintf("🔄 [事件Flush] [%s] 强制解析缓存的error事件", tp.requestID))
		}
		return tp.parseErrorEventV2()
	default:
		if tp.requestID != "" {
			slog.Info(fmt.Sprintf("⚠️ [事件Flush] [%s] 未知事件类型: %s, 跳过解析", tp.requestID, tp.currentEvent))
		}
		return nil
	}
}
