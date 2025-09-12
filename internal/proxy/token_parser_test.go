package proxy

import (
	"testing"
	"cc-forwarder/internal/monitor"
)

func TestTokenParser(t *testing.T) {
	parser := NewTokenParser()
	
	// Test parsing Claude API message_delta event with usage
	lines := []string{
		"event: message_delta",
		"data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\",\"stop_sequence\":null},\"usage\":{\"input_tokens\":5,\"cache_creation_input_tokens\":494,\"cache_read_input_tokens\":110689,\"output_tokens\":582}}",
		"",
	}
	
	var result *monitor.TokenUsage
	for _, line := range lines {
		if tokens := parser.ParseSSELine(line); tokens != nil {
			result = tokens
		}
	}
	
	if result == nil {
		t.Fatal("Expected to parse token usage, got nil")
	}
	
	// Check the values
	if result.InputTokens != 5 {
		t.Errorf("Expected InputTokens=5, got %d", result.InputTokens)
	}
	if result.OutputTokens != 582 {
		t.Errorf("Expected OutputTokens=582, got %d", result.OutputTokens)
	}
	if result.CacheCreationTokens != 494 {
		t.Errorf("Expected CacheCreationTokens=494, got %d", result.CacheCreationTokens)
	}
	if result.CacheReadTokens != 110689 {
		t.Errorf("Expected CacheReadTokens=110689, got %d", result.CacheReadTokens)
	}
}

func TestTokenParserNonUsageEvent(t *testing.T) {
	parser := NewTokenParser()
	
	// Test parsing non-usage message_delta event
	lines := []string{
		"event: message_delta",
		"data: {\"type\":\"message_delta\",\"delta\":{\"text\":\"Hello world\"}}",
		"",
	}
	
	var result *monitor.TokenUsage
	for _, line := range lines {
		if tokens := parser.ParseSSELine(line); tokens != nil {
			result = tokens
		}
	}
	
	if result != nil {
		t.Error("Expected nil for message_delta without usage, got result")
	}
}

func TestTokenParserOtherEvents(t *testing.T) {
	parser := NewTokenParser()
	
	// Test parsing non-message_delta events
	lines := []string{
		"event: ping",
		"data: {\"type\":\"ping\"}",
		"",
	}
	
	var result *monitor.TokenUsage
	for _, line := range lines {
		if tokens := parser.ParseSSELine(line); tokens != nil {
			result = tokens
		}
	}
	
	if result != nil {
		t.Error("Expected nil for non-message_delta events, got result")
	}
}

// ===== V2 职责纯化测试 =====

func TestTokenParserV2_MessageDeltaWithUsage(t *testing.T) {
	parser := NewTokenParserWithRequestID("test-req-123")
	
	// Test parsing Claude API message_delta event with usage using V2 method
	lines := []string{
		"event: message_delta",
		"data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\",\"stop_sequence\":null},\"usage\":{\"input_tokens\":5,\"cache_creation_input_tokens\":494,\"cache_read_input_tokens\":110689,\"output_tokens\":582}}",
		"",
	}
	
	var result *ParseResult
	for _, line := range lines {
		if parseResult := parser.ParseSSELineV2(line); parseResult != nil {
			result = parseResult
		}
	}
	
	if result == nil {
		t.Fatal("Expected to parse token usage with V2 method, got nil")
	}
	
	// Check the ParseResult structure
	if result.TokenUsage == nil {
		t.Fatal("Expected TokenUsage in ParseResult, got nil")
	}
	
	if result.TokenUsage.InputTokens != 5 {
		t.Errorf("Expected InputTokens=5, got %d", result.TokenUsage.InputTokens)
	}
	if result.TokenUsage.OutputTokens != 582 {
		t.Errorf("Expected OutputTokens=582, got %d", result.TokenUsage.OutputTokens)
	}
	if result.TokenUsage.CacheCreationTokens != 494 {
		t.Errorf("Expected CacheCreationTokens=494, got %d", result.TokenUsage.CacheCreationTokens)
	}
	if result.TokenUsage.CacheReadTokens != 110689 {
		t.Errorf("Expected CacheReadTokens=110689, got %d", result.TokenUsage.CacheReadTokens)
	}
	
	if !result.IsCompleted {
		t.Error("Expected IsCompleted=true for message_delta with usage")
	}
	
	if result.Status != "completed" {
		t.Errorf("Expected Status=completed, got %s", result.Status)
	}
}

func TestTokenParserV2_MessageDeltaWithoutUsage(t *testing.T) {
	parser := NewTokenParserWithRequestID("test-req-456")
	
	// Test parsing non-usage message_delta event using V2 method
	lines := []string{
		"event: message_delta",
		"data: {\"type\":\"message_delta\",\"delta\":{\"text\":\"Hello world\"}}",
		"",
	}
	
	var result *ParseResult
	for _, line := range lines {
		if parseResult := parser.ParseSSELineV2(line); parseResult != nil {
			result = parseResult
		}
	}
	
	if result == nil {
		t.Fatal("Expected to get ParseResult for non-usage message_delta, got nil")
	}
	
	// Check the ParseResult for non-token response
	if result.TokenUsage == nil {
		t.Fatal("Expected empty TokenUsage in ParseResult, got nil")
	}
	
	// Should have empty token usage
	if result.TokenUsage.InputTokens != 0 {
		t.Errorf("Expected InputTokens=0 for non-usage, got %d", result.TokenUsage.InputTokens)
	}
	
	if !result.IsCompleted {
		t.Error("Expected IsCompleted=true for non-usage message_delta")
	}
	
	if result.Status != "non_token_response" {
		t.Errorf("Expected Status=non_token_response, got %s", result.Status)
	}
	
	if result.ModelName != "default" {
		t.Errorf("Expected ModelName=default, got %s", result.ModelName)
	}
}

func TestTokenParserV2_ErrorEvent(t *testing.T) {
	parser := NewTokenParserWithRequestID("test-req-error")
	
	// Test parsing error event using V2 method
	lines := []string{
		"event: error",
		"data: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"Server is overloaded\"}}",
		"",
	}
	
	var result *ParseResult
	for _, line := range lines {
		if parseResult := parser.ParseSSELineV2(line); parseResult != nil {
			result = parseResult
		}
	}
	
	if result == nil {
		t.Fatal("Expected to get ParseResult for error event, got nil")
	}
	
	// Check error handling
	if result.ErrorInfo == nil {
		t.Fatal("Expected ErrorInfo in ParseResult, got nil")
	}
	
	if result.ErrorInfo.Type != "overloaded_error" {
		t.Errorf("Expected ErrorInfo.Type=overloaded_error, got %s", result.ErrorInfo.Type)
	}
	
	if result.ErrorInfo.Message != "Server is overloaded" {
		t.Errorf("Expected ErrorInfo.Message=Server is overloaded, got %s", result.ErrorInfo.Message)
	}
	
	if !result.IsCompleted {
		t.Error("Expected IsCompleted=true for error event")
	}
	
	if result.Status != StatusErrorAPI {
		t.Errorf("Expected Status=%s, got %s", StatusErrorAPI, result.Status)
	}
	
	if result.ModelName != "error:overloaded_error" {
		t.Errorf("Expected ModelName=error:overloaded_error, got %s", result.ModelName)
	}
}