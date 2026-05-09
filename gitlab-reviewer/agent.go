package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

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
	return r.review(ctx, workDir, diff, false)
}

func (r *LLMReviewer) ReviewFull(ctx context.Context, workDir string, diff string) (*ReviewResult, error) {
	return r.review(ctx, workDir, diff, true)
}

func (r *LLMReviewer) review(ctx context.Context, workDir string, diff string, full bool) (*ReviewResult, error) {
	var systemPrompt string
	maxIter := 6
	maxForkDepth := 0
	tracerTag := "commit-review"

	if full {
		systemPrompt = buildMRReviewPrompt(workDir)
		maxIter = 12
		maxForkDepth = 1
		tracerTag = "mr-review"
	} else {
		systemPrompt = buildCommitReviewPrompt(workDir)
		maxForkDepth = 1
	}

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
		MaxIter:        maxIter,
		MaxTokens:      8000,
		MaxForkDepth:   maxForkDepth,
		AgentName:      agentName,
		TracerTag:      tracerTag,
		Tools:          agentcore.StandardToolDefs(),
		ToolDispatcher: agentcore.StandardToolDispatch,
		Messages: []agentcore.ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: fmt.Sprintf("Here is the diff to review:\n\n```diff\n%s\n```\n\nReview it and produce your findings.", diff)},
		},
	})
	if err != nil {
		return nil, err
	}
	defer lr.Tracer.Close()

	reviewText := lr.FinalMessage.Content
	if lr.Truncated || reviewText == "" {
		return &ReviewResult{Findings: []Finding{}, Model: r.Model}, nil
	}

	result, err := r.extractFindings(ctx, reviewText)
	if err != nil {
		log.Printf("warning: extraction failed, returning empty findings: %v", err)
		return &ReviewResult{Findings: []Finding{}, Model: r.Model}, nil
	}
	result.Model = r.Model
	return result, nil
}

func (r *LLMReviewer) ReviewWithContext(ctx context.Context, workDir string, diff string, priorContext string) (*ReviewResult, error) {
	systemPrompt := buildMRRepassPrompt(workDir, priorContext)

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
		MaxTokens:      8000,
		MaxForkDepth:   1,
		AgentName:      agentName,
		TracerTag:      "mr-repass",
		Tools:          agentcore.StandardToolDefs(),
		ToolDispatcher: agentcore.StandardToolDispatch,
		Messages: []agentcore.ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: fmt.Sprintf("Here is the full merge request diff to review:\n\n```diff\n%s\n```\n\nThe prior findings digest above covers what was found per-commit. Focus on cross-cutting concerns only.", diff)},
		},
	})
	if err != nil {
		return nil, err
	}
	defer lr.Tracer.Close()

	reviewText := lr.FinalMessage.Content
	if lr.Truncated || reviewText == "" {
		return &ReviewResult{Findings: []Finding{}, Model: r.Model}, nil
	}

	result, err := r.extractFindings(ctx, reviewText)
	if err != nil {
		log.Printf("warning: extraction failed, returning empty findings: %v", err)
		return &ReviewResult{Findings: []Finding{}, Model: r.Model}, nil
	}
	result.Model = r.Model
	return result, nil
}

func (r *LLMReviewer) extractFindings(ctx context.Context, reviewText string) (*ReviewResult, error) {
	resp, err := r.LLM.Chat(ctx, agentcore.ChatRequest{
		Model: r.Model,
		Messages: []agentcore.ChatMessage{
			{Role: "system", Content: `Extract structured findings from the code review below. Output ONLY a JSON object:
{"findings": [{"category": "string", "severity": "info|minor|major|critical", "location": "file:line", "description": "one sentence", "evidence": "short quote or empty"}]}
If the review found no issues, return {"findings": []}.`},
			{Role: "user", Content: reviewText},
		},
		Temperature: 0.1,
		MaxTokens:   4000,
	})
	if err != nil {
		return nil, fmt.Errorf("extraction LLM call: %w", err)
	}

	if len(resp.Choices) == 0 {
		return &ReviewResult{}, nil
	}

	content := agentcore.ExtractJSON(resp.Choices[0].Message.Content)
	var result ReviewResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("parse extraction output: %w", err)
	}
	return &result, nil
}

func buildMRRepassPrompt(root string, priorContext string) string {
	return fmt.Sprintf(`You are a code review agent. Second pass — DO NOT repeat prior findings. Be brief.

Prior findings:
<prior_findings>
%s
</prior_findings>

Focus only on: cross-commit interactions, architectural impact, patterns across commits, branch-scope concerns.

Workspace root: %s

Tools (paths relative to root):
- read_file, grep, glob, list_dir, git_log, git_diff, git_blame, git_show, fork

Write your review as plain text. For each finding state: the file and line, severity (info/minor/major/critical), what you found, and a short evidence quote. If no new issues, just say "No cross-cutting issues found."

Rules: never repeat prior findings. Descriptive, not prescriptive. Keep each finding to 1-2 sentences.`, priorContext, root)
}

func buildCommitReviewPrompt(root string) string {
	return fmt.Sprintf(`You are a code review agent. Review the commit diff. Be brief — short tool calls, minimal exploration.

Workspace root: %s

Tools (paths relative to root):
- read_file: read file contents (with optional line range)
- grep: regex search
- list_dir: list directory

Process: read the diff, optionally read 1-2 changed files for context, then write your review.

Write your review as plain text. For each finding state: the file and line, severity (info/minor/major/critical), what you found. If the commit is trivial or has no issues, just say "No issues found."

Rules: focus on changes only, not pre-existing issues. Be descriptive, not prescriptive. Keep each finding to 1-2 sentences.`, root)
}

func buildMRReviewPrompt(root string) string {
	return fmt.Sprintf(`You are a code review agent. Review the merge request diff. Be brief — short tool calls, concise findings.

Workspace root: %s

Tools (paths relative to root):
- read_file, grep, glob, list_dir, git_log, git_diff, git_blame, git_show
- fork: split into parallel sub-tasks

Process:
1. Read the diff, explore workspace briefly for context
2. Fork into parallel reviews: correctness, security, consistency
3. Combine results into final output

Write your review as plain text. For each finding state: the file and line, severity (info/minor/major/critical), what you found, and a short evidence quote.

Rules: focus on changes only. Descriptive, not prescriptive. Keep each finding to 1-2 sentences.`, root)
}

