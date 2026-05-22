package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kurze/lab/agentcore"
)

type digestInput struct {
	Commit   string    `json:"commit"`
	Message  string    `json:"message"`
	Findings []Finding `json:"findings"`
}

func digestFindings(ctx context.Context, llm *agentcore.LLMClient, model string, commitResults []CommitReviewResult) (string, error) {
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

	resp, err := llm.Chat(ctx, agentcore.ChatRequest{
		Model: model,
		Messages: []agentcore.ChatMessage{
			{Role: "system", Content: `You are a code review assistant. You will receive findings from a per-commit review of a merge request. Produce a compact digest of these findings that another reviewer can use to avoid repeating them.

Produce a concise bulleted summary organized by theme (not by commit). For each bullet:
- State what was found and where (file/location)
- Note the severity
- If multiple commits touch the same concern, mention that

Keep the digest under 500 words. Do not add new findings. Do not suggest fixes. Just summarize what was found.`},
			{Role: "user", Content: fmt.Sprintf("Here are the per-commit review findings:\n\n```json\n%s\n```\n\nProduce a thematic digest.", string(inputJSON))},
		},
		Temperature: 0.2,
		MaxTokens:   1500,
	})
	if err != nil {
		return digestFindingsPlain(commitResults), nil
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
