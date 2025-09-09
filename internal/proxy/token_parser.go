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

// UsageData represents the usage field in Claude API SSE events
type UsageData struct {
	InputTokens            int64 `json:"input_tokens"`
	OutputTokens           int64 `json:"output_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens    int64 `json:"cache_read_input_tokens"`
}

// MessageStartData represents the message object in message_start events
type MessageStartData struct {
	ID       string     `json:"id"`
	Type     string     `json:"type"`
	Role     string     `json:"role"`
	Model    string     `json:"model"`
	Content  []interface{} `json:"content"`
	Usage    *UsageData `json:"usage,omitempty"`
}

// MessageStart represents the structure of message_start events
type MessageStart struct {
	Type    string             `json:"type"`
	Message *MessageStartData  `json:"message"`
}

// MessageDelta represents the structure of message_delta events
type MessageDelta struct {
	Type      string     `json:"type"`
	Delta     interface{} `json:"delta"`
	Usage     *UsageData  `json:"usage,omitempty"`
}

// SSEErrorData represents the structure of error events in SSE streams
type SSEErrorData struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"` 
		Message string `json:"message"`
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
type TokenParser struct {
	// Buffer to collect multi-line JSON data
	eventBuffer     strings.Builder
	currentEvent    string
	collectingData  bool
	// Request ID for logging purposes
	requestID       string
	// Model name extracted from message_start event
	modelName       string
	// Usage tracker for recording token usage and costs
	usageTracker    *tracking.UsageTracker
	// Start time for duration calculation
	startTime       time.Time
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
			tp.parseErrorEvent()
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
		
		// Log model extraction (不处理token usage)
		if tp.requestID != "" {
			slog.Info(fmt.Sprintf("🎯 [模型提取] [%s] 从message_start事件中提取模型信息: %s", 
				tp.requestID, tp.modelName))
		} else {
			slog.Info(fmt.Sprintf("🎯 [模型提取] 从message_start事件中提取模型信息: %s", 
				tp.modelName))
		}
	}
	
	// ⚠️ 重要：message_start事件不处理token usage信息
	// Token usage信息应该从message_delta事件中获取，该事件包含完整的使用统计
	
	return nil
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
		if tp.usageTracker != nil && tp.requestID != "" {
			// Calculate duration since parser creation
			duration := time.Since(tp.startTime)
			
			// Create empty token usage for non-Claude endpoints
			emptyTokens := &tracking.TokenUsage{
				InputTokens:         0,
				OutputTokens:        0,
				CacheCreationTokens: 0,
				CacheReadTokens:     0,
			}
			
			// Use "default" as model name if no model was extracted from message_start
			modelName := tp.modelName
			if modelName == "" {
				modelName = "default"
			}
			
			// Record completion for non-token response
			tp.usageTracker.RecordRequestComplete(tp.requestID, modelName, emptyTokens, duration)
			
			if tp.requestID != "" {
				slog.Info(fmt.Sprintf("🎯 [无Token响应] [%s] message_delta事件不包含token信息，标记为完成 - 模型: %s", 
					tp.requestID, modelName))
			}
		}
		return nil
	}
	
	// Convert to our TokenUsage format
	tokenUsage := &monitor.TokenUsage{
		InputTokens:            messageDelta.Usage.InputTokens,
		OutputTokens:           messageDelta.Usage.OutputTokens,
		CacheCreationTokens:    messageDelta.Usage.CacheCreationInputTokens,
		CacheReadTokens:        messageDelta.Usage.CacheReadInputTokens,
	}

	modelInfo := ""
	if tp.modelName != "" {
		modelInfo = fmt.Sprintf(" 模型: %s,", tp.modelName)
	}

	if tp.requestID != "" {
		slog.Info(fmt.Sprintf("🪙 [Token使用统计] [%s] 从message_delta事件中提取完整令牌使用情况 -%s 输入: %d, 输出: %d, 缓存创建: %d, 缓存读取: %d",
			tp.requestID, modelInfo, tokenUsage.InputTokens, tokenUsage.OutputTokens, tokenUsage.CacheCreationTokens, tokenUsage.CacheReadTokens))
	} else {
		slog.Info(fmt.Sprintf("🪙 [Token使用统计] 从message_delta事件中提取完整令牌使用情况 -%s 输入: %d, 输出: %d, 缓存创建: %d, 缓存读取: %d",
			modelInfo, tokenUsage.InputTokens, tokenUsage.OutputTokens, tokenUsage.CacheCreationTokens, tokenUsage.CacheReadTokens))
	}

	// ✅ 只在message_delta中记录请求完成和成本计算
	// 这是正确的时机，因为message_delta包含最终的完整token usage信息
	if tp.usageTracker != nil && tp.requestID != "" {
		// Calculate duration since parser creation
		duration := time.Since(tp.startTime)
		
		// Convert monitor.TokenUsage to tracking.TokenUsage
		trackingTokens := &tracking.TokenUsage{
			InputTokens:         tokenUsage.InputTokens,
			OutputTokens:        tokenUsage.OutputTokens,
			CacheCreationTokens: tokenUsage.CacheCreationTokens,
			CacheReadTokens:     tokenUsage.CacheReadTokens,
		}
		
		// Record the completion with token usage and cost information
		tp.usageTracker.RecordRequestComplete(tp.requestID, tp.modelName, trackingTokens, duration)
	}

	return tokenUsage
}

// Reset clears the parser state
func (tp *TokenParser) Reset() {
	tp.eventBuffer.Reset()
	tp.currentEvent = ""
	tp.collectingData = false
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
	
	// Record as API error in usage tracking
	if tp.usageTracker != nil && tp.requestID != "" {
		// Calculate duration since parser creation
		duration := time.Since(tp.startTime)
		
		// Create error model name with error type
		errorModelName := fmt.Sprintf("error:%s", errorType)
		
		// Record with empty token usage but error status
		emptyTokens := &tracking.TokenUsage{
			InputTokens:         0,
			OutputTokens:        0,
			CacheCreationTokens: 0,
			CacheReadTokens:     0,
		}
		
		// Record the completion with error model name
		tp.usageTracker.RecordRequestComplete(tp.requestID, errorModelName, emptyTokens, duration)
		
		// Update request status to error_api
		tp.usageTracker.RecordRequestUpdate(tp.requestID, "", "", StatusErrorAPI, 0, 0)
	}
}

// SetModelName allows setting the model name directly (useful for JSON response parsing)
func (tp *TokenParser) SetModelName(modelName string) {
	tp.modelName = modelName
}