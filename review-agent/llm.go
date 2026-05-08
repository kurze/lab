package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type LLMClient struct {
	url    string
	client *http.Client
}

func newLLMClient(cfg Config) *LLMClient {
	return &LLMClient{
		url: cfg.LLMURL,
		client: &http.Client{
			Timeout: 300 * time.Second,
		},
	}
}

type chatMessage struct {
	Role       string    `json:"role"`
	Content    string    `json:"content,omitempty"`
	ToolCalls  []llmTool `json:"tool_calls,omitempty"`
	ToolCallID string    `json:"tool_call_id,omitempty"`
}

type llmTool struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Function llmToolFunction `json:"function"`
}

type llmToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type chatRequest struct {
	Model          string         `json:"model"`
	Messages       []chatMessage  `json:"messages"`
	Tools          []any          `json:"tools,omitempty"`
	ResponseFormat map[string]any `json:"response_format,omitempty"`
	Temperature    float64        `json:"temperature"`
}

type chatResponse struct {
	Choices []struct {
		Message      chatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func (c *LLMClient) Chat(ctx context.Context, req chatRequest) (*chatResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("LLM request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("LLM returned %d: %s", resp.StatusCode, string(b))
	}

	var result chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode LLM response: %w", err)
	}

	return &result, nil
}

var agentTools = []any{
	map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        "read_file",
			"description": "Read a file's contents. Path is relative to workspace root.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":  map[string]any{"type": "string", "description": "File path relative to workspace root"},
					"start": map[string]any{"type": "integer", "description": "Start line (1-indexed, optional)"},
					"end":   map[string]any{"type": "integer", "description": "End line (inclusive, optional)"},
				},
				"required": []string{"path"},
			},
		},
	},
	map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        "grep",
			"description": "Search for a regex pattern in files under a path.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern": map[string]any{"type": "string", "description": "Regex pattern to search for"},
					"path":    map[string]any{"type": "string", "description": "Directory or file to search, relative to workspace root"},
					"glob":    map[string]any{"type": "string", "description": "Filename glob filter (e.g. *.go), optional"},
				},
				"required": []string{"pattern", "path"},
			},
		},
	},
	map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        "list_dir",
			"description": "List entries in a directory.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string", "description": "Directory path relative to workspace root"},
				},
				"required": []string{"path"},
			},
		},
	},
}
