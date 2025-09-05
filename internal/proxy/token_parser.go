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
	
	
	// Handle event type lines
	if strings.HasPrefix(line, "event: ") {
		eventType := strings.TrimPrefix(line, "event: ")
		tp.currentEvent = eventType
		// Collect data for both message_start (for model info) and message_delta (for usage)
		tp.collectingData = (eventType == "message_delta" || eventType == "message_start")
		tp.eventBuffer.Reset()
		return nil
	}
	
	// Handle data lines for message_delta events
	if strings.HasPrefix(line, "data: ") && tp.collectingData {
		dataContent := strings.TrimPrefix(line, "data: ")
		tp.eventBuffer.WriteString(dataContent)
		return nil
	}
	
	// Handle empty lines that signal end of SSE event
	if line == "" && tp.collectingData && tp.eventBuffer.Len() > 0 {
		if tp.currentEvent == "message_start" {
			// Parse message_start for model info, don't return TokenUsage
			tp.parseMessageStart()
			return nil
		} else if tp.currentEvent == "message_delta" {
			// Parse message_delta for usage info
			return tp.parseMessageDelta()
		}
	}
	
	return nil
}

// parseMessageStart parses the collected message_start JSON data to extract model info
func (tp *TokenParser) parseMessageStart() {
	defer func() {
		tp.eventBuffer.Reset()
		tp.collectingData = false
		tp.currentEvent = ""
	}()
	
	jsonData := tp.eventBuffer.String()
	if jsonData == "" {
		return
	}
	
	// Parse the JSON data
	var messageStart MessageStart
	if err := json.Unmarshal([]byte(jsonData), &messageStart); err != nil {
		return
	}
	
	// Extract model name if available
	if messageStart.Message != nil && messageStart.Message.Model != "" {
		tp.modelName = messageStart.Message.Model
	}
}

// parseMessageDelta parses the collected message_delta JSON data
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
		slog.Info(fmt.Sprintf("🪙 [Token Parser] [%s] 从SSE流中提取令牌使用情况 -%s 输入: %d, 输出: %d, 缓存创建: %d, 缓存读取: %d",
			tp.requestID, modelInfo, tokenUsage.InputTokens, tokenUsage.OutputTokens, tokenUsage.CacheCreationTokens, tokenUsage.CacheReadTokens))
	} else {
		slog.Info(fmt.Sprintf("🪙 [Token Parser] 从SSE流中提取令牌使用情况 -%s 输入: %d, 输出: %d, 缓存创建: %d, 缓存读取: %d",
			modelInfo, tokenUsage.InputTokens, tokenUsage.OutputTokens, tokenUsage.CacheCreationTokens, tokenUsage.CacheReadTokens))
	}

	// Record request completion in usage tracking
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