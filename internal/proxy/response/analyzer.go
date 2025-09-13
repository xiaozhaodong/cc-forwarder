package response

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"cc-forwarder/internal/monitor"
	"cc-forwarder/internal/tracking"
)

// RequestLifecycleManager 定义生命周期管理器接口
type RequestLifecycleManager interface {
	GetDuration() time.Duration
}

// TokenParser 定义token parser接口
type TokenParser interface {
	ParseSSELine(line string) *monitor.TokenUsage
	SetModelName(model string)
}

// TokenParserProvider 提供token parser创建功能
type TokenParserProvider interface {
	NewTokenParser() TokenParser
	NewTokenParserWithUsageTracker(requestID string, usageTracker *tracking.UsageTracker) TokenParser
}

// TokenAnalyzer 负责分析响应中的Token使用信息
type TokenAnalyzer struct {
	usageTracker         *tracking.UsageTracker
	monitoringMiddleware interface{}
	tokenParserProvider  TokenParserProvider
}

// NewTokenAnalyzer 创建新的TokenAnalyzer实例
func NewTokenAnalyzer(usageTracker *tracking.UsageTracker, monitoringMiddleware interface{}, provider TokenParserProvider) *TokenAnalyzer {
	return &TokenAnalyzer{
		usageTracker:         usageTracker,
		monitoringMiddleware: monitoringMiddleware,
		tokenParserProvider:  provider,
	}
}

// AnalyzeResponseForTokens analyzes the complete response body for token usage information
func (a *TokenAnalyzer) AnalyzeResponseForTokens(ctx context.Context, responseBody, endpointName string, r *http.Request) {
	
	// Get connection ID from request context
	connID := ""
	if connIDValue, ok := r.Context().Value("conn_id").(string); ok {
		connID = connIDValue
	}
	
	// Add entry log for debugging
	slog.DebugContext(ctx, fmt.Sprintf("🎯 [Token分析入口] [%s] 端点: %s, 响应长度: %d字节", 
		connID, endpointName, len(responseBody)))
	
	// Method 1: Try to find SSE format in the response (for streaming responses that were buffered)
	// Check for error events first before checking for token events
	if strings.Contains(responseBody, "event:error") || strings.Contains(responseBody, "event: error") {
		a.ParseSSETokens(ctx, responseBody, endpointName, connID)
		return
	}
	
	// Check for both message_start and message_delta events as token info can be in either
	if strings.Contains(responseBody, "event:message_start") || 
	   strings.Contains(responseBody, "event: message_start") ||
	   strings.Contains(responseBody, "event:message_delta") || 
	   strings.Contains(responseBody, "event: message_delta") {
		a.ParseSSETokens(ctx, responseBody, endpointName, connID)
		return
	}
	
	// Method 2: Try to parse as single JSON response
	if strings.HasPrefix(strings.TrimSpace(responseBody), "{") && strings.Contains(responseBody, "usage") {
		a.ParseJSONTokens(ctx, responseBody, endpointName, connID)
		return
	}

	// Fallback: No token information found, mark request as completed with non_token_response model
	slog.InfoContext(ctx, fmt.Sprintf("🎯 [无Token响应] 端点: %s, 连接: %s - 响应不包含token信息，标记为完成", endpointName, connID))
	
	// ⚠️ 注意：这个方法不再直接记录到数据库，由调用方决定如何处理
	// 但为了兼容现有调用，我们保留日志记录功能
	if connID != "" {
		slog.InfoContext(ctx, fmt.Sprintf("✅ [无Token完成] 连接: %s 检测为无Token响应，模型: non_token_response", connID))
	}
}

// ParseSSETokens parses SSE format response for token usage or error events
func (a *TokenAnalyzer) ParseSSETokens(ctx context.Context, responseBody, endpointName, connID string) {
	tokenParser := a.tokenParserProvider.NewTokenParserWithUsageTracker(connID, a.usageTracker)
	lines := strings.Split(responseBody, "\n")
	
	foundTokenUsage := false
	hasErrorEvent := false
	
	// Check if response contains error events first
	if strings.Contains(responseBody, "event:error") || strings.Contains(responseBody, "event: error") {
		hasErrorEvent = true
		slog.InfoContext(ctx, fmt.Sprintf("❌ [SSE错误检测] [%s] 端点: %s - 检测到error事件", connID, endpointName))
	}
	
	for _, line := range lines {
		if tokenUsage := tokenParser.ParseSSELine(line); tokenUsage != nil {
			foundTokenUsage = true
			// 获取模型名称(需要类型断言获取TokenParser的模型信息)
			modelName := "unknown"
			if tp, ok := tokenParser.(interface{ GetModelName() string }); ok {
				modelName = tp.GetModelName()
			}
			
			// 详细显示token使用信息
			slog.InfoContext(ctx, fmt.Sprintf("✅ [SSE解析成功] [%s] 端点: %s - 模型: %s, 输入: %d, 输出: %d, 缓存创建: %d, 缓存读取: %d", 
				connID, endpointName, modelName, 
				tokenUsage.InputTokens, tokenUsage.OutputTokens, 
				tokenUsage.CacheCreationTokens, tokenUsage.CacheReadTokens))
			
			// Record token usage in monitoring middleware if available
			if mm, ok := a.monitoringMiddleware.(interface{
				RecordTokenUsage(connID string, endpoint string, tokens *monitor.TokenUsage)
			}); ok && connID != "" {
				mm.RecordTokenUsage(connID, endpointName, tokenUsage)
			}
			
			// ⚠️ 重要：TokenParser现在只负责解析，不直接记录到数据库
			// 数据库记录由上层AnalyzeResponseForTokensWithLifecycle统一处理
			return
		}
	}
	
	// If we found an error event, the parseErrorEvent method would have already handled it
	if hasErrorEvent {
		slog.InfoContext(ctx, fmt.Sprintf("❌ [SSE错误处理] [%s] 端点: %s - 错误事件已处理", connID, endpointName))
		return
	}
	
	if !foundTokenUsage {
		slog.InfoContext(ctx, fmt.Sprintf("🚫 [SSE解析] [%s] 端点: %s - 未找到token usage信息", connID, endpointName))
	}
}

// ParseJSONTokens parses single JSON response for token usage
func (a *TokenAnalyzer) ParseJSONTokens(ctx context.Context, responseBody, endpointName, connID string) {
	// Simulate SSE parsing for a single JSON response
	tokenParser := a.tokenParserProvider.NewTokenParserWithUsageTracker(connID, a.usageTracker)
	
	slog.InfoContext(ctx, fmt.Sprintf("🔍 [JSON解析] [%s] 尝试解析JSON响应", connID))
	
	// 🆕 First extract model information directly from JSON
	var jsonResp map[string]interface{}
	if err := json.Unmarshal([]byte(responseBody), &jsonResp); err == nil {
		if model, ok := jsonResp["model"].(string); ok && model != "" {
			tokenParser.SetModelName(model)
			slog.InfoContext(ctx, "📋 [JSON解析] 提取到模型信息", "model", model)
		}
	}
	
	// Wrap JSON as SSE message_delta event
	tokenParser.ParseSSELine("event: message_delta")
	tokenParser.ParseSSELine("data: " + responseBody)
	if tokenUsage := tokenParser.ParseSSELine(""); tokenUsage != nil {
		// Record token usage
		if mm, ok := a.monitoringMiddleware.(interface{
			RecordTokenUsage(connID string, endpoint string, tokens *monitor.TokenUsage)
		}); ok && connID != "" {
			mm.RecordTokenUsage(connID, endpointName, tokenUsage)
			slog.InfoContext(ctx, "✅ [JSON解析] 成功记录token使用", 
				"endpoint", endpointName, 
				"inputTokens", tokenUsage.InputTokens, 
				"outputTokens", tokenUsage.OutputTokens,
				"cacheCreation", tokenUsage.CacheCreationTokens,
				"cacheRead", tokenUsage.CacheReadTokens)
		}
	} else {
		slog.DebugContext(ctx, fmt.Sprintf("🚫 [JSON解析] [%s] JSON中未找到token usage信息", connID))
	}
}

// AnalyzeResponseForTokensWithLifecycle analyzes response with accurate duration from lifecycle manager
func (a *TokenAnalyzer) AnalyzeResponseForTokensWithLifecycle(ctx context.Context, responseBody, endpointName string, r *http.Request, lifecycleManager RequestLifecycleManager) {
	// Get connection ID from request context
	connID := ""
	if connIDValue, ok := r.Context().Value("conn_id").(string); ok {
		connID = connIDValue
	}
	
	// Add entry log for debugging
	slog.DebugContext(ctx, fmt.Sprintf("🎯 [Token分析入口] [%s] 端点: %s, 响应长度: %d字节", 
		connID, endpointName, len(responseBody)))
	
	// Method 1: Try to find SSE format in the response (for streaming responses that were buffered)
	// Check for error events first before checking for token events
	if strings.Contains(responseBody, "event:error") || strings.Contains(responseBody, "event: error") {
		a.ParseSSETokens(ctx, responseBody, endpointName, connID)
		return
	}
	
	// Check for both message_start and message_delta events as token info can be in either
	if strings.Contains(responseBody, "event:message_start") || 
	   strings.Contains(responseBody, "event:message_delta") ||
	   strings.Contains(responseBody, "event: message_start") ||
	   strings.Contains(responseBody, "event: message_delta") {
		a.ParseSSETokens(ctx, responseBody, endpointName, connID)
		
		// ℹ️ 返回Token信息，由Handler通过生命周期管理器记录
		// 不再直接调用usageTracker.RecordRequestComplete，遵循架构原则
		if tokenUsage, modelName := a.parseSSEForTokens(responseBody, connID, endpointName); tokenUsage != nil {
			slog.InfoContext(ctx, fmt.Sprintf("💾 [SSEToken解析] [%s] 模型: %s, Token信息已解析完成", connID, modelName))
			// 返回Token信息，由上层Handler调用生命周期管理器记录
			return // TokenAnalyzer不再直接记录，返回给上层处理
		}
		
		return
	}
	
	// Method 2: Direct JSON analysis for non-SSE responses
	slog.InfoContext(ctx, fmt.Sprintf("🔍 [JSON解析] [%s] 尝试解析JSON响应", connID))
	
	// Try to parse as JSON and extract model information
	var jsonData map[string]interface{}
	var model string
	
	if err := json.Unmarshal([]byte(responseBody), &jsonData); err == nil {
		// Extract model information if available
		if modelValue, exists := jsonData["model"]; exists {
			if modelStr, ok := modelValue.(string); ok {
				model = modelStr
				slog.InfoContext(ctx, "📋 [JSON解析] 提取到模型信息", "model", model)
			}
		}
	}
	
	// Wrap JSON as SSE message_delta event
	tokenParser := a.tokenParserProvider.NewTokenParser()
	tokenParser.ParseSSELine("event: message_delta")
	tokenParser.ParseSSELine("data: " + responseBody)
	if tokenUsage := tokenParser.ParseSSELine(""); tokenUsage != nil {
		// Record token usage to monitoring middleware
		if mm, ok := a.monitoringMiddleware.(interface{
			RecordTokenUsage(connID string, endpoint string, tokens *monitor.TokenUsage)
		}); ok && connID != "" {
			mm.RecordTokenUsage(connID, endpointName, tokenUsage)
			slog.InfoContext(ctx, "✅ [JSON解析] 成功记录token使用", 
				"endpoint", endpointName, 
				"inputTokens", tokenUsage.InputTokens, 
				"outputTokens", tokenUsage.OutputTokens,
				"cacheCreation", tokenUsage.CacheCreationTokens,
				"cacheRead", tokenUsage.CacheReadTokens)
		}
		
		// ℹ️ 返回Token信息，由Handler通过生命周期管理器记录
		// 不再直接调用usageTracker.RecordRequestComplete，遵循架构原则
		if tokenUsage, modelName := a.parseJSONForTokens(responseBody, connID, endpointName); tokenUsage != nil {
			slog.InfoContext(ctx, "💾 [JSON数据库保存] JSON解析的Token信息已解析完成",
				"request_id", connID, "model", modelName, 
				"inputTokens", tokenUsage.InputTokens, "outputTokens", tokenUsage.OutputTokens)
			// TokenAnalyzer不再直接记录，返回给上层处理
		} else {
			slog.DebugContext(ctx, fmt.Sprintf("🚫 [JSON解析] [%s] JSON中未找到token usage信息", connID))
			
			// ℹ️ Fallback: 返回空的Token信息，由Handler通过生命周期管理器记录
			// 不再直接调用usageTracker.RecordRequestComplete，遵循架构原则
			slog.InfoContext(ctx, fmt.Sprintf("✅ [无Token完成] 连接: %s 将由Handler标记为完成状态，模型: non_token_response", connID))
		}
	}
}

// AnalyzeResponseForTokensUnified 简化版本的Token分析（用于统一接口）
// 返回值: (tokenUsage, modelName) - tokenUsage为nil表示无Token信息
func (a *TokenAnalyzer) AnalyzeResponseForTokensUnified(responseBytes []byte, connID, endpointName string) (*tracking.TokenUsage, string) {
	if len(responseBytes) == 0 {
		return nil, "empty_response"
	}
	
	responseStr := string(responseBytes)
	
	// Method 1: 检查是否为SSE格式响应
	if strings.Contains(responseStr, "event:error") || strings.Contains(responseStr, "event: error") {
		return a.parseSSEForTokens(responseStr, connID, endpointName)
	}
	
	// Check for both message_start and message_delta events
	if strings.Contains(responseStr, "event:message_start") || 
	   strings.Contains(responseStr, "event: message_start") ||
	   strings.Contains(responseStr, "event:message_delta") || 
	   strings.Contains(responseStr, "event: message_delta") {
		return a.parseSSEForTokens(responseStr, connID, endpointName)
	}
	
	// Method 2: 尝试解析JSON响应
	if strings.HasPrefix(strings.TrimSpace(responseStr), "{") && strings.Contains(responseStr, "usage") {
		return a.parseJSONForTokens(responseStr, connID, endpointName)
	}
	
	// Fallback: 无Token信息
	slog.Info(fmt.Sprintf("🎯 [无Token响应] 端点: %s, 连接: %s - 响应不包含token信息", endpointName, connID))
	return nil, "non_token_response"
}

// parseSSEForTokens 解析SSE格式响应获取Token信息（不直接记录）
func (a *TokenAnalyzer) parseSSEForTokens(responseStr, connID, endpointName string) (*tracking.TokenUsage, string) {
	tokenParser := a.tokenParserProvider.NewTokenParserWithUsageTracker(connID, a.usageTracker)
	lines := strings.Split(responseStr, "\n")
	
	var foundTokenUsage *tracking.TokenUsage
	var modelName string = "unknown"
	hasErrorEvent := false
	
	// 检查是否包含错误事件
	if strings.Contains(responseStr, "event:error") || strings.Contains(responseStr, "event: error") {
		hasErrorEvent = true
		slog.Info(fmt.Sprintf("❌ [SSE错误检测] [%s] 端点: %s - 检测到error事件", connID, endpointName))
	}
	
	// 解析每一行
	for _, line := range lines {
		if tokenUsage := tokenParser.ParseSSELine(line); tokenUsage != nil {
			// 获取模型名称
			if tp, ok := tokenParser.(interface{ GetModelName() string }); ok {
				modelName = tp.GetModelName()
			}
			
			// 转换为tracking.TokenUsage格式
			foundTokenUsage = &tracking.TokenUsage{
				InputTokens:         tokenUsage.InputTokens,
				OutputTokens:        tokenUsage.OutputTokens,
				CacheCreationTokens: tokenUsage.CacheCreationTokens,
				CacheReadTokens:     tokenUsage.CacheReadTokens,
			}
			
			slog.Info(fmt.Sprintf("✅ [SSE解析成功] [%s] 端点: %s - 模型: %s, 输入: %d, 输出: %d, 缓存创建: %d, 缓存读取: %d", 
				connID, endpointName, modelName, 
				foundTokenUsage.InputTokens, foundTokenUsage.OutputTokens, 
				foundTokenUsage.CacheCreationTokens, foundTokenUsage.CacheReadTokens))
			
			return foundTokenUsage, modelName
		}
	}
	
	// 如果有错误事件或没找到Token信息
	if hasErrorEvent {
		slog.Info(fmt.Sprintf("❌ [SSE错误处理] [%s] 端点: %s - 错误事件已处理", connID, endpointName))
		return nil, "error_response"
	}
	
	slog.Info(fmt.Sprintf("🚫 [SSE解析] [%s] 端点: %s - 未找到token usage信息", connID, endpointName))
	return nil, "no_token_sse"
}

// parseJSONForTokens 解析JSON格式响应获取Token信息（不直接记录）
func (a *TokenAnalyzer) parseJSONForTokens(responseStr, connID, endpointName string) (*tracking.TokenUsage, string) {
	tokenParser := a.tokenParserProvider.NewTokenParserWithUsageTracker(connID, a.usageTracker)
	
	slog.Info(fmt.Sprintf("🔍 [JSON解析] [%s] 尝试解析JSON响应", connID))
	
	// 首先提取模型信息
	var jsonResp map[string]interface{}
	var modelName string = "default"
	
	if err := json.Unmarshal([]byte(responseStr), &jsonResp); err == nil {
		if model, ok := jsonResp["model"].(string); ok && model != "" {
			modelName = model
			tokenParser.SetModelName(model)
			slog.Info("📋 [JSON解析] 提取到模型信息", "model", model)
		}
	}
	
	// 将JSON包装为SSE message_delta事件进行解析
	tokenParser.ParseSSELine("event: message_delta")
	tokenParser.ParseSSELine("data: " + responseStr)
	if tokenUsage := tokenParser.ParseSSELine(""); tokenUsage != nil {
		// 转换为tracking.TokenUsage格式
		trackingTokenUsage := &tracking.TokenUsage{
			InputTokens:         tokenUsage.InputTokens,
			OutputTokens:        tokenUsage.OutputTokens,
			CacheCreationTokens: tokenUsage.CacheCreationTokens,
			CacheReadTokens:     tokenUsage.CacheReadTokens,
		}
		
		slog.Info("✅ [JSON解析] 成功解析Token使用信息", 
			"endpoint", endpointName, 
			"inputTokens", trackingTokenUsage.InputTokens, 
			"outputTokens", trackingTokenUsage.OutputTokens,
			"cacheCreation", trackingTokenUsage.CacheCreationTokens,
			"cacheRead", trackingTokenUsage.CacheReadTokens)
		
		return trackingTokenUsage, modelName
	}
	
	slog.Debug(fmt.Sprintf("🚫 [JSON解析] [%s] JSON中未找到token usage信息", connID))
	return nil, modelName
}