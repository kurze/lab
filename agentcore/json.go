package agentcore

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var codeBlockRe = regexp.MustCompile("(?s)```(?:json)?\\s*\n?(.*?)\\s*```")

func ExtractJSON(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "{") {
		return s
	}
	if m := codeBlockRe.FindStringSubmatch(s); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	if start := strings.Index(s, "{"); start >= 0 {
		return s[start:]
	}
	return s
}

func RepairJSON[T any](ctx context.Context, llm *LLMClient, modelID string, messages []ChatMessage, content string, repairPrompt string, tracer *Tracer, iter int) (*T, error) {
	for attempt := range 2 {
		repairMessages := make([]ChatMessage, len(messages), len(messages)+2)
		copy(repairMessages, messages)
		repairMessages = append(repairMessages, ChatMessage{Role: "assistant", Content: content})
		repairMessages = append(repairMessages, ChatMessage{
			Role:    "user",
			Content: repairPrompt,
		})

		req := ChatRequest{
			Model:       modelID,
			Messages:    repairMessages,
			Temperature: 0.1,
		}

		resp, err := llm.Chat(ctx, req)
		if err != nil {
			tracer.Log(TraceEntry{Iteration: iter, Role: "error", Content: fmt.Sprintf("repair attempt %d: %s", attempt, err)})
			continue
		}
		if len(resp.Choices) == 0 {
			continue
		}

		repaired := ExtractJSON(resp.Choices[0].Message.Content)
		tracer.Log(TraceEntry{Iteration: iter, Role: "repair", Content: repaired})

		var result T
		if err := json.Unmarshal([]byte(repaired), &result); err == nil {
			return &result, nil
		}
	}
	return nil, fmt.Errorf("failed to parse output after repair attempts")
}
