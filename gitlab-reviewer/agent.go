package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kurze/lab/agentcore"
)

const agentName = "gitlab-reviewer"

type LLMReviewer struct {
	LLM          *agentcore.LLMClient
	Model        string
	ContextSize  int
	TokenCeiling int
	Temperature  float64
}

func (r *LLMReviewer) Review(ctx context.Context, workDir string, diff string) (*ReviewResult, error) {
	systemPrompt := buildMRReviewPrompt(workDir)

	temp := r.Temperature
	if temp == 0 {
		temp = 0.3
	}

	lr, err := agentcore.RunLoop(ctx, r.LLM, agentcore.LoopConfig{
		ModelID:        r.Model,
		ContextSize:    r.ContextSize,
		TokenCeiling:   r.TokenCeiling,
		Root:           workDir,
		Temperature:    temp,
		MaxIter:        12,
		MaxTokens:      3000,
		AgentName:      agentName,
		TracerTag:      "mr-review",
		Tools:          agentcore.StandardToolDefs(),
		ToolDispatcher: agentcore.StandardToolDispatch,
		Messages: []agentcore.ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: fmt.Sprintf("Here is the merge request diff to review:\n\n```diff\n%s\n```\n\nExplore the codebase for context, then produce your findings.", diff)},
		},
	})
	if err != nil {
		return nil, err
	}
	defer lr.Tracer.Close()

	if lr.Truncated {
		return &ReviewResult{Findings: []Finding{}}, nil
	}

	return parseLLMOutput(lr)
}

func buildMRReviewPrompt(root string) string {
	return fmt.Sprintf(`You are a code review agent. Your task is to review a GitLab merge request diff and produce structured findings.

Workspace root: %s

You have four tools: read_file, grep, list_dir, git_log. All paths are relative to the workspace root.
Use these tools to understand the surrounding code context for the changes in the diff.

Process:
1. Analyze the diff provided in the user message
2. Explore the workspace to understand the context around the changed code
3. When ready, produce your final output as a JSON object

Your final output MUST be a JSON object with exactly this field:
{
  "findings": [{"category": "string", "severity": "info|minor|major|critical", "location": "file:line or section", "description": "what you found", "evidence": "what led you to this"}]
}

Rules:
- Focus on the CHANGES in the diff, not pre-existing issues.
- Be descriptive, never prescriptive. Say what you found, not what to do about it.
- Every finding needs concrete evidence from the diff or codebase.
- Pay attention to: missing error handling, broken invariants, inconsistencies with existing patterns, untested edge cases, security implications.
- Be concise. Short descriptions, minimal evidence quotes.
- When you are done exploring, stop calling tools and output ONLY your JSON.`, root)
}

func parseLLMOutput(lr *agentcore.LoopResult) (*ReviewResult, error) {
	content := agentcore.ExtractJSON(lr.FinalMessage.Content)

	var result ReviewResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("parse review output: %w\nraw: %s", err, content)
	}
	return &result, nil
}
