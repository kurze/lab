package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kurze/lab/agentcore"
)

type digestInput struct {
	Commit   string    `json:"commit"`
	Message  string    `json:"message"`
	Findings []Finding `json:"findings"`
}

func digestFindings(ctx context.Context, llm *agentcore.LLMClient, model string, commitResults []CommitReviewResult, tracer *agentcore.Tracer) (string, error) {
	var inputs []digestInput
	for _, cr := range commitResults {
		if cr.Result == nil || len(cr.Result.Findings) == 0 {
			continue
		}
		sha := cr.Commit.SHA
		if len(sha) > 8 {
			sha = sha[:8]
		}
		msg := firstline(cr.Commit.Message)
		inputs = append(inputs, digestInput{
			Commit:   sha,
			Message:  msg,
			Findings: cr.Result.Findings,
		})
	}

	if len(inputs) == 0 {
		return "No findings from per-commit review.", nil
	}

	inputJSON, err := json.Marshal(inputs)
	if err != nil {
		return digestFindingsPlain(commitResults), nil
	}

	if tracer != nil {
		tracer.Log(agentcore.TraceEntry{Role: "system", Content: "[digest] sending per-commit findings for thematic digest"})
	}

	start := time.Now()
	resp, err := llm.Chat(ctx, agentcore.ChatRequest{
		Model: model,
		Messages: []agentcore.ChatMessage{
			{Role: "system", Content: BuildDigestPrompt()},
			{Role: "user", Content: fmt.Sprintf("Here are the per-commit review findings:\n\n```json\n%s\n```\n\nProduce a thematic digest.", string(inputJSON))},
		},
		Temperature: 0.2,
		MaxTokens:   1500,
	})
	if err != nil {
		return digestFindingsPlain(commitResults), nil
	}

	if tracer != nil && len(resp.Choices) > 0 {
		tracer.Log(agentcore.TraceEntry{
			Role:             "assistant",
			Content:          "[digest] " + resp.Choices[0].Message.Content,
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
			LatencyMs:        time.Since(start).Milliseconds(),
		})
	}

	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == "" {
		return digestFindingsPlain(commitResults), nil
	}

	return resp.Choices[0].Message.Content, nil
}

func digestFindingsPlain(commitResults []CommitReviewResult) string {
	var b strings.Builder
	for _, cr := range commitResults {
		if cr.Result == nil || len(cr.Result.Findings) == 0 {
			continue
		}
		sha := cr.Commit.SHA
		if len(sha) > 8 {
			sha = sha[:8]
		}
		msg := firstline(cr.Commit.Message)
		fmt.Fprintf(&b, "Commit %s (%s):\n", sha, msg)
		for _, f := range cr.Result.Findings {
			fmt.Fprintf(&b, "  - [%s] %s: %s", f.Severity, f.Category, f.Description)
			if f.Location != "" {
				fmt.Fprintf(&b, " (%s)", f.Location)
			}
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}
	if b.Len() == 0 {
		return "No findings from per-commit review."
	}
	return b.String()
}
