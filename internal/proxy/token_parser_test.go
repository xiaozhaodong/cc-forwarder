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