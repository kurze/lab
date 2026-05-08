package main

import (
	"strings"
	"testing"
)

func TestCompactMessagesUnderThreshold(t *testing.T) {
	msgs := []chatMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "usr"},
		{Role: "tool", Content: strings.Repeat("x", 500)},
	}
	result := compactMessages(msgs, 200_000)
	if len(result[2].Content) != 500 {
		t.Error("should not compact when under threshold")
	}
}

func TestCompactMessagesOverThreshold(t *testing.T) {
	big := strings.Repeat("line one\n", 5000)
	msgs := []chatMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "usr"},
		{Role: "tool", Content: big},
		{Role: "assistant", Content: "ok"},
		{Role: "tool", Content: big},
		{Role: "assistant", Content: "ok"},
		{Role: "tool", Content: big},
		{Role: "assistant", Content: "ok"},
		{Role: "tool", Content: big},
		{Role: "assistant", Content: "ok"},
		{Role: "tool", Content: big},
		{Role: "assistant", Content: "latest"},
	}
	result := compactMessages(msgs, 1000)

	// First tool messages should be compacted
	if !strings.Contains(result[2].Content, "[compacted]") {
		t.Error("expected early tool message to be compacted")
	}

	// Last messages should be preserved
	last := result[len(result)-1]
	if last.Content != "latest" {
		t.Errorf("expected last message preserved, got: %s", last.Content)
	}
}

func TestEstimateTokens(t *testing.T) {
	msgs := []chatMessage{
		{Role: "user", Content: strings.Repeat("x", 400)},
	}
	tokens := estimateTokens(msgs)
	if tokens != 100 {
		t.Errorf("expected ~100 tokens, got %d", tokens)
	}
}
