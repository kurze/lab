package local

import (
	"context"
	"fmt"
	"strings"

	"github.com/kurze/lab/agentcore"

	"maestro/internal/agent"
)

// LocalAgent sends prompts to a local LLM via the llama.cpp-compatible
// /v1/chat/completions endpoint, using agentcore.LLMClient.
type LocalAgent struct {
	llm   *agentcore.LLMClient
	model string
}

// New creates a LocalAgent that calls the given endpoint with the specified model name.
func New(endpoint, model string) *LocalAgent {
	url := strings.TrimRight(endpoint, "/") + "/v1/chat/completions"
	return &LocalAgent{
		llm:   agentcore.NewLLMClient(url),
		model: model,
	}
}

func (a *LocalAgent) Type() agent.AgentType {
	return agent.LocalLLM
}

// Run sends the prompt as a user message to the local LLM and returns the
// assistant's response. The worktree parameter is unused for local LLM calls.
func (a *LocalAgent) Run(ctx context.Context, worktree string, prompt string) (string, error) {
	resp, err := a.llm.Chat(ctx, agentcore.ChatRequest{
		Model: a.model,
		Messages: []agentcore.ChatMessage{
			{Role: "user", Content: prompt},
		},
		Temperature: 0.5,
		MaxTokens:   4096,
	})
	if err != nil {
		return "", fmt.Errorf("local LLM call failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("local LLM returned no choices")
	}

	return resp.Choices[0].Message.Content, nil
}
