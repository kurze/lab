package agentcore

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
	apiKey string
	client *http.Client
}

type ClientOption func(*LLMClient)

func WithAPIKey(key string) ClientOption {
	return func(c *LLMClient) { c.apiKey = key }
}

func NewLLMClient(url string, opts ...ClientOption) *LLMClient {
	c := &LLMClient{
		url: url,
		client: &http.Client{
			Timeout: 300 * time.Second,
		},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

type ChatMessage struct {
	Role       string    `json:"role"`
	Content    string    `json:"content,omitempty"`
	ToolCalls  []LLMTool `json:"tool_calls,omitempty"`
	ToolCallID string    `json:"tool_call_id,omitempty"`
}

type LLMTool struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Function LLMToolFunction `json:"function"`
}

type LLMToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ChatRequest struct {
	Model          string         `json:"model"`
	Messages       []ChatMessage  `json:"messages"`
	Tools          []any          `json:"tools,omitempty"`
	ResponseFormat map[string]any `json:"response_format,omitempty"`
	Temperature    float64        `json:"temperature"`
	MaxTokens      int            `json:"max_tokens,omitempty"`
}

type ChatResponse struct {
	Choices []struct {
		Message      ChatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func (c *LLMClient) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("LLM request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, fmt.Errorf("LLM returned %d (body unreadable: %w)", resp.StatusCode, readErr)
		}
		return nil, fmt.Errorf("LLM returned %d: %s", resp.StatusCode, string(b))
	}

	var result ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode LLM response: %w", err)
	}

	return &result, nil
}
