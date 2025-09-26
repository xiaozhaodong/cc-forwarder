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

// UsageData 表示Claude API SSE事件中的usage字段
type UsageData struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
}

// MessageStartData 表示message_start事件中的message对象
type MessageStartData struct {
	ID      string        `json:"id"`
	Type    string        `json:"type"`
	Role    string        `json:"role"`
	Model   string        `json:"model"`
	Content []interface{} `json:"content"`
	Usage   *UsageData    `json:"usage,omitempty"`
}

// MessageStart 表示message_start事件的结构
type MessageStart struct {
	Type    string            `json:"type"`
	Message *MessageStartData `json:"message"`
}

// MessageDelta 表示message_delta事件的结构
type MessageDelta struct {
	Type  string      `json:"type"`
	Delta interface{} `json:"delta"`
	Usage *UsageData  `json:"usage,omitempty"`
}

// SSEErrorData 表示SSE流中error事件的结构
type SSEErrorData struct {
	Type  string `json:"type"`
	Error struct {
		Type      string `json:"type"`
		Message   string `json:"message"`
		RequestID string `json:"request_id,omitempty"`
	} `json:"error"`
}

// 请求处理状态常量
const (
	StatusCompleted    = "completed"     // 真正成功（有Token或正常响应）
	StatusErrorAPI     = "error_api"     // API层错误（overloaded等）
	StatusErrorNetwork = "error_network" // 网络层错误（超时等）
	StatusProcessing   = "processing"    // 处理中
)

// TokenParser 处理SSE事件的解析以提取token使用信息
// 实现TokenParserInterface接口
type TokenParser struct {
	// 用于收集多行JSON数据的缓冲区
	eventBuffer    strings.Builder
	currentEvent   string
	collectingData bool
	// 用于日志记录的请求ID
	requestID string
	// 从message_start事件中提取的模型名称
	modelName string
	// 用于记录token使用和成本的跟踪器
	usageTracker *tracking.UsageTracker
	// 用于计算持续时间的开始时间
	startTime time.Time
	// 用于累积的最终token使用量
	finalUsage *tracking.TokenUsage
	// 用于处理中断的部分使用量
	partialUsage *tracking.TokenUsage
}

// fixMalformedEventType 修复格式错误的事件类型
// 处理如 "content_event: message_delta" 这样的格式错误，提取最后一个有效的事件名称
func (tp *TokenParser) fixMalformedEventType(eventType string) string {
	// 🔧 [格式错误修复] 处理格式错误的事件行，如 "event: content_event: message_delta"
	// 从事件类型中提取最后一个有效的事件名称
	if strings.Contains(eventType, ":") {
		parts := strings.Split(eventType, ":")
		// 取最后一个非空部分作为真正的事件类型
		for i := len(parts) - 1; i >= 0; i-- {
			cleanPart := strings.TrimSpace(parts[i])
			if cleanPart != "" {
				if tp.requestID != "" {
					slog.Warn(fmt.Sprintf("⚠️ [格式错误修复] [%s] 检测到格式错误的事件行，修正为: %s", tp.requestID, cleanPart))
				}
				return cleanPart
			}
		}
	}
	return eventType
}

// NewTokenParser 创建新的token解析器实例
func NewTokenParser() *TokenParser {
	return &TokenParser{
		startTime: time.Now(),
	}
}

// NewTokenParserWithRequestID 创建带请求ID的新token解析器实例
func NewTokenParserWithRequestID(requestID string) *TokenParser {
	return &TokenParser{
		requestID: requestID,
		startTime: time.Now(),
	}
}

// NewTokenParserWithUsageTracker 创建带使用跟踪器的新token解析器实例
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

	// 处理事件类型行 - 支持 "event: " 和 "event:" 两种格式
	if strings.HasPrefix(line, "event:") {
		var eventType string
		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
		} else {
			eventType = strings.TrimPrefix(line, "event:")
		}

		// 使用公共方法修复格式错误的事件类型
		eventType = tp.fixMalformedEventType(eventType)

		tp.currentEvent = eventType
		// 为message_start（模型信息）、message_delta（使用量）和error事件收集数据
		tp.collectingData = (eventType == "message_delta" || eventType == "message_start" || eventType == "error")
		tp.eventBuffer.Reset()
		return nil
	}

	// 处理数据行 - 支持 "data: " 和 "data:" 两种格式
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

	// 处理表示SSE事件结束的空行
	if line == "" && tp.collectingData && tp.eventBuffer.Len() > 0 {
		if tp.currentEvent == "message_start" {
			// 仅解析message_start以获取模型信息（不需要ParseResult）
			tp.parseMessageStart()
			return nil
		} else if tp.currentEvent == "message_delta" {
			// 使用新的V2方法解析message_delta
			return tp.parseMessageDeltaV2()
		} else if tp.currentEvent == "error" {
			// 使用新的V2方法解析error事件
			return tp.parseErrorEventV2()
		}
	}

	return nil
}

// ParseSSELine 处理SSE流中的单行数据，如果找到则提取token使用信息
func (tp *TokenParser) ParseSSELine(line string) *monitor.TokenUsage {
	line = strings.TrimSpace(line)

	// 处理事件类型行 - 支持 "event: " 和 "event:" 两种格式
	if strings.HasPrefix(line, "event:") {
		var eventType string
		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
		} else {
			eventType = strings.TrimPrefix(line, "event:")
		}

		// 使用公共方法修复格式错误的事件类型
		eventType = tp.fixMalformedEventType(eventType)

		tp.currentEvent = eventType
		// 为message_start（模型信息）、message_delta（使用量）和error事件收集数据
		tp.collectingData = (eventType == "message_delta" || eventType == "message_start" || eventType == "error")
		tp.eventBuffer.Reset()
		return nil
	}

	// 处理数据行 - 支持 "data: " 和 "data:" 两种格式
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

	// 处理表示SSE事件结束的空行
	if line == "" && tp.collectingData && tp.eventBuffer.Len() > 0 {
		if tp.currentEvent == "message_start" {
			// 解析message_start以获取模型信息和token使用量
			return tp.parseMessageStart()
		} else if tp.currentEvent == "message_delta" {
			// 解析message_delta以获取使用信息
			return tp.parseMessageDelta()
		} else if tp.currentEvent == "error" {
			// 解析error事件并记录为API错误
			// 🚫 修复：注释掉违规的直接usageTracker调用，让生命周期管理器处理
			// tp.parseErrorEvent()
			slog.Info(fmt.Sprintf("❌ [错误事件] [%s] 检测到API错误事件", tp.requestID))
			return nil // error事件不返回TokenUsage
		}
	}

	return nil
}

// parseMessageStart 解析收集的message_start JSON数据以仅提取模型信息
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

	// 解析JSON数据
	var messageStart MessageStart
	if err := json.Unmarshal([]byte(jsonData), &messageStart); err != nil {
		return nil
	}

	// 如果可用，提取模型名称
	if messageStart.Message != nil && messageStart.Message.Model != "" {
		tp.modelName = messageStart.Message.Model

		// 记录模型提取（不处理token usage） - 始终包含requestID
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

	// 解析JSON数据
	var messageDelta MessageDelta
	if err := json.Unmarshal([]byte(jsonData), &messageDelta); err != nil {
		return nil
	}

	// 检查此message_delta是否包含使用信息
	if messageDelta.Usage == nil {
		// ⚠️ 兼容性处理：对于非Claude端点，message_delta可能不包含usage信息
		// 这种情况下需要fallback机制来标记请求完成
		if tp.requestID != "" {
			// 如果未从message_start提取模型，则使用"default"作为模型名称
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

// parseMessageDelta 解析收集的message_delta JSON数据以获取完整的token使用信息
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

	// 解析JSON数据
	var messageDelta MessageDelta
	if err := json.Unmarshal([]byte(jsonData), &messageDelta); err != nil {
		return nil
	}

	// 检查此message_delta是否包含使用信息
	if messageDelta.Usage == nil {
		// ⚠️ 兼容性处理：对于非Claude端点，message_delta可能不包含usage信息
		// 这种情况下需要fallback机制来标记请求完成
		if tp.requestID != "" {
			// 如果未从message_start提取模型，则使用"default"作为模型名称
			modelName := tp.modelName
			if modelName == "" {
				modelName = "default"
			}

			slog.Info(fmt.Sprintf("🎯 [无Token响应] [%s] message_delta事件不包含token信息，标记为完成 - 模型: %s",
				tp.requestID, modelName))
		}
		return nil
	}

	// 转换为我们的TokenUsage格式
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
	//	// 计算创建解析器以来的持续时间
	//	duration := time.Since(tp.startTime)
	//
	//	// 转换monitor.TokenUsage为tracking.TokenUsage
	//	trackingTokens := &tracking.TokenUsage{
	//		InputTokens:         tokenUsage.InputTokens,
	//		OutputTokens:        tokenUsage.OutputTokens,
	//		CacheCreationTokens: tokenUsage.CacheCreationTokens,
	//		CacheReadTokens:     tokenUsage.CacheReadTokens,
	//	}
	//
	//	// 记录完成的token使用和成本信息
	//	tp.usageTracker.RecordRequestComplete(tp.requestID, tp.modelName, trackingTokens, duration)
	// }

	return tokenUsage
}

// Reset 清除解析器状态
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

	// 解析错误JSON数据
	var errorData SSEErrorData
	if err := json.Unmarshal([]byte(jsonData), &errorData); err != nil {
		if tp.requestID != "" {
			slog.Info(fmt.Sprintf("⚠️ [SSE错误解析] [%s] 无法解析错误数据: %s", tp.requestID, jsonData))
		}
		return nil
	}

	// 提取错误类型和消息
	errorType := errorData.Error.Type
	errorMessage := errorData.Error.Message
	if errorType == "" {
		errorType = "unknown_error"
	}
	if errorMessage == "" {
		errorMessage = "Unknown error"
	}

	// 记录API错误
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

// parseErrorEvent 解析SSE错误事件并将其记录为API错误
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

	// 解析错误JSON数据
	var errorData SSEErrorData
	if err := json.Unmarshal([]byte(jsonData), &errorData); err != nil {
		if tp.requestID != "" {
			slog.Info(fmt.Sprintf("⚠️ [SSE错误解析] [%s] 无法解析错误数据: %s", tp.requestID, jsonData))
		}
		return
	}

	// 提取错误类型和消息
	errorType := errorData.Error.Type
	errorMessage := errorData.Error.Message
	if errorType == "" {
		errorType = "unknown_error"
	}
	if errorMessage == "" {
		errorMessage = "Unknown error"
	}

	// 记录API错误
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

// SetModelName 允许直接设置模型名称（用于JSON响应解析）
func (tp *TokenParser) SetModelName(modelName string) {
	tp.modelName = modelName
}

// ParseMessageStart 实现接口方法 - 解析message_start事件提取模型信息
func (tp *TokenParser) ParseMessageStart(line string) *ModelInfo {
	if !strings.HasPrefix(line, "data: ") {
		return nil
	}

	data := line[6:] // 移除 "data: "
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

	data := line[6:] // 移除 "data: "
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
