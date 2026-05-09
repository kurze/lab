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

	if lr.Truncated {
		return &ReviewResult{Findings: []Finding{}, Model: r.Model}, nil
	}

	result, err := parseLLMOutput(lr)
	if err != nil {
		return nil, err
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

	if lr.Truncated {
		return &ReviewResult{Findings: []Finding{}, Model: r.Model}, nil
	}

	result, err := parseLLMOutput(lr)
	if err != nil {
		return nil, err
	}
	result.Model = r.Model
	return result, nil
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

Output MUST be JSON: {"findings": [{"category": "...", "severity": "info|minor|major|critical", "location": "file:line", "description": "...", "evidence": "..."}]}

Rules: never repeat prior findings. Descriptive, not prescriptive. Descriptions under 2 sentences. If no new cross-cutting issues found, return {"findings": []}.`, priorContext, root)
}

func buildCommitReviewPrompt(root string) string {
	return fmt.Sprintf(`You are a code review agent. Review the commit diff. Be brief — short tool calls, minimal exploration.

Workspace root: %s

Tools (paths relative to root):
- read_file: read file contents (with optional line range)
- grep: regex search
- list_dir: list directory

Process: read the diff, optionally read 1-2 changed files for context, then output findings.
If the commit is trivial (rename, formatting, comments only), return {"findings": []}.

Output MUST be JSON: {"findings": [{"category": "...", "severity": "info|minor|major|critical", "location": "file:line", "description": "...", "evidence": "..."}]}

Rules: focus on changes only, not pre-existing issues. Be descriptive, not prescriptive. Keep descriptions under 2 sentences. Keep evidence to a single short quote or omit.`, root)
}

func buildMRReviewPrompt(root string) string {
	return fmt.Sprintf(`You are a code review agent. Review the merge request diff. Be brief throughout — short tool calls, concise findings.

Workspace root: %s

Tools (paths relative to root):
- read_file, grep, glob, list_dir, git_log, git_diff, git_blame, git_show
- fork: split into parallel sub-tasks

Process:
1. Read the diff, explore workspace briefly for context
2. Fork into parallel reviews: correctness, security, consistency
3. Combine results into final output

Output MUST be JSON: {"findings": [{"category": "...", "severity": "info|minor|major|critical", "location": "file:line", "description": "...", "evidence": "..."}]}

Rules: focus on changes only. Descriptive, not prescriptive. Descriptions under 2 sentences. Evidence: single short quote or omit.`, root)
}

func parseLLMOutput(lr *agentcore.LoopResult) (*ReviewResult, error) {
	content := agentcore.ExtractJSON(lr.FinalMessage.Content)

	var result ReviewResult
	if err := json.Unmarshal([]byte(content), &result); err == nil {
		return &result, nil
	}

	// Output may be truncated — salvage complete findings
	result.Findings = salvageFindings(content)
	if len(result.Findings) > 0 {
		log.Printf("warning: output was truncated, salvaged %d complete finding(s)", len(result.Findings))
		return &result, nil
	}

	log.Printf("warning: could not parse review output, returning empty findings")
	return &ReviewResult{}, nil
}

func salvageFindings(content string) []Finding {
	// Find each complete JSON object in the findings array
	var findings []Finding
	depth := 0
	start := -1
	for i, c := range content {
		switch c {
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			depth--
			if depth == 0 && start >= 0 {
				var f Finding
				if err := json.Unmarshal([]byte(content[start:i+1]), &f); err == nil && f.Description != "" {
					findings = append(findings, f)
				}
				start = -1
			}
		}
	}
	// Skip the first match — it's the outer {"findings": ...} wrapper
	if len(findings) > 1 {
		return findings[1:]
	}
	return nil
}
