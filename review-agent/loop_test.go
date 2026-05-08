package main

import (
	"strings"
	"testing"

	"github.com/kurze/lab/agentcore"
)

func TestCompactMessagesUnderThreshold(t *testing.T) {
	msgs := []agentcore.ChatMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "usr"},
		{Role: "tool", Content: strings.Repeat("x", 500)},
	}
	result := agentcore.CompactMessages(msgs, 200_000)
	if len(result[2].Content) != 500 {
		t.Error("should not compact when under threshold")
	}
}

func TestCompactMessagesOverThreshold(t *testing.T) {
	big := strings.Repeat("line one\n", 5000)
	msgs := []agentcore.ChatMessage{
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
	result := agentcore.CompactMessages(msgs, 1000)

	if !strings.Contains(result[2].Content, "[compacted]") {
		t.Error("expected early tool message to be compacted")
	}

	last := result[len(result)-1]
	if last.Content != "latest" {
		t.Errorf("expected last message preserved, got: %s", last.Content)
	}
}

func TestEstimateTokens(t *testing.T) {
	msgs := []agentcore.ChatMessage{
		{Role: "user", Content: strings.Repeat("x", 400)},
	}
	tokens := agentcore.EstimateTokens(msgs)
	if tokens != 100 {
		t.Errorf("expected ~100 tokens, got %d", tokens)
	}
}
