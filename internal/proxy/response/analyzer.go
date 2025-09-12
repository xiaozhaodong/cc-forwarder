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
	
	// Update request status to completed and set model name to "non_token_response"
	if a.usageTracker != nil && connID != "" {
		// Create empty token usage for consistent completion tracking
		emptyTokens := &tracking.TokenUsage{
			InputTokens:         0,
			OutputTokens:        0,
			CacheCreationTokens: 0,
			CacheReadTokens:     0,
		}
		
		// Record completion with non_token_response model name and zero duration (since we don't track start time here)
		a.usageTracker.RecordRequestComplete(connID, "non_token_response", emptyTokens, 0)
		
		slog.InfoContext(ctx, fmt.Sprintf("✅ [无Token完成] 连接: %s 已标记为完成状态，模型: non_token_response", connID))
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
		
		// ⚠️ 补充：ParseSSETokens不再记录到数据库，需要在此处补充
		// 重新解析获取token信息并记录到UsageTracker
		if a.usageTracker != nil && connID != "" {
			tokenParser := a.tokenParserProvider.NewTokenParserWithUsageTracker(connID, a.usageTracker)
			lines := strings.Split(responseBody, "\n")
			
			for _, line := range lines {
				if tokenUsage := tokenParser.ParseSSELine(line); tokenUsage != nil {
					// 获取模型名称
					modelName := "unknown"
					if tp, ok := tokenParser.(interface{ GetModelName() string }); ok {
						modelName = tp.GetModelName()
					}
					
					// 转换为tracking.TokenUsage格式
					trackingTokens := &tracking.TokenUsage{
						InputTokens:         tokenUsage.InputTokens,
						OutputTokens:        tokenUsage.OutputTokens,
						CacheCreationTokens: tokenUsage.CacheCreationTokens,
						CacheReadTokens:     tokenUsage.CacheReadTokens,
					}
					
					// 获取准确的持续时间
					duration := lifecycleManager.GetDuration()
					
					// 记录到数据库
					a.usageTracker.RecordRequestComplete(connID, modelName, trackingTokens, duration)
					break // 找到token usage后退出循环
				}
			}
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
		
		// 🔧 修复：同时保存到数据库，使用准确的处理时间
		if a.usageTracker != nil && connID != "" && lifecycleManager != nil {
			// 转换Token格式
			dbTokens := &tracking.TokenUsage{
				InputTokens:         tokenUsage.InputTokens,
				OutputTokens:        tokenUsage.OutputTokens,
				CacheCreationTokens: tokenUsage.CacheCreationTokens,
				CacheReadTokens:     tokenUsage.CacheReadTokens,
			}
			
			// 使用提取的模型名称，如果没有则使用default
			modelName := "default"
			if model != "" {
				modelName = model
			}
			
			// 🎯 使用lifecycleManager获取准确的处理时间
			duration := lifecycleManager.GetDuration()
			
			// 保存到数据库
			a.usageTracker.RecordRequestComplete(connID, modelName, dbTokens, duration)
			slog.InfoContext(ctx, "💾 [数据库保存] JSON解析的Token信息已保存到数据库",
				"request_id", connID, "model", modelName, 
				"inputTokens", dbTokens.InputTokens, "outputTokens", dbTokens.OutputTokens,
				"duration", duration)
		}
	} else {
		slog.DebugContext(ctx, fmt.Sprintf("🚫 [JSON解析] [%s] JSON中未找到token usage信息", connID))
		
		// Fallback: No token information found, mark request as completed with default model
		if a.usageTracker != nil && connID != "" && lifecycleManager != nil {
			emptyTokens := &tracking.TokenUsage{
				InputTokens: 0, OutputTokens: 0, 
				CacheCreationTokens: 0, CacheReadTokens: 0,
			}
			duration := lifecycleManager.GetDuration()
			a.usageTracker.RecordRequestComplete(connID, "non_token_response", emptyTokens, duration)
			slog.InfoContext(ctx, fmt.Sprintf("✅ [无Token完成] 连接: %s 已标记为完成状态，模型: non_token_response, 处理时间: %v", 
				connID, duration))
		}
	}
}

// AnalyzeResponseForTokensUnified 简化版本的Token分析（用于统一接口）
func (a *TokenAnalyzer) AnalyzeResponseForTokensUnified(responseBytes []byte, connID, endpointName string, lifecycleManager RequestLifecycleManager) {
	if len(responseBytes) == 0 {
		return
	}
	
	responseStr := string(responseBytes)
	
	// 使用现有的Token分析方法（创建一个临时的Request对象）
	req := &http.Request{} // 创建一个空的request对象
	req = req.WithContext(context.WithValue(context.Background(), "conn_id", connID))
	
	// 调用现有的分析方法，传入lifecycleManager以获取准确的duration
	a.AnalyzeResponseForTokensWithLifecycle(req.Context(), responseStr, endpointName, req, lifecycleManager)
}