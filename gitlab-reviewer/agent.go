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
	return fmt.Sprintf(`You are a code review agent performing a second-pass review of a merge request.
A first pass already reviewed each commit individually and found these issues:

<prior_findings>
%s
</prior_findings>

DO NOT repeat any finding already covered above. Focus exclusively on:
- Cross-commit interactions: does a change in one commit break an assumption made in another?
- Architectural impact: does the overall change introduce structural problems not visible per-commit?
- Patterns across commits: repeated mistakes, inconsistent approaches, missing coordination
- Branch-scope concerns: API surface changes, migration ordering, test coverage gaps across the whole change

Workspace root: %s

Tools available (all paths relative to workspace root):
- read_file: read file contents (with optional line range)
- grep: regex search inside files
- glob: find files by name pattern (e.g. **/*.go)
- list_dir: list directory entries
- git_log: recent commit history
- git_diff: compare two refs or see changes to a specific file
- git_blame: see who last modified each line and when
- git_show: inspect a specific commit
- fork: split into parallel sub-tasks sharing your current context

Process:
1. Read the full diff and the prior findings digest
2. Explore the workspace to understand cross-cutting context
3. Produce findings ONLY for issues not already covered

Your final output MUST be a JSON object with exactly this field:
{
  "findings": [{"category": "string", "severity": "info|minor|major|critical", "location": "file:line or section", "description": "what you found", "evidence": "what led you to this"}]
}

Rules:
- Focus on cross-cutting concerns the per-commit review could not catch.
- Never repeat a finding from the prior digest, even rephrased.
- Be descriptive, never prescriptive.
- Every finding needs concrete evidence from the diff or codebase.
- Be concise. Short descriptions, minimal evidence quotes.
- When you are done exploring, stop calling tools and output ONLY your JSON.`, priorContext, root)
}

func buildCommitReviewPrompt(root string) string {
	return fmt.Sprintf(`You are a code review agent. Review the commit diff and produce structured findings.

Workspace root: %s

Tools available (all paths relative to workspace root):
- read_file: read file contents (with optional line range)
- grep: regex search inside files
- list_dir: list directory entries

Process:
1. Read the diff
2. If needed, read one or two changed files for context
3. Produce your findings

Your final output MUST be a JSON object with exactly this field:
{
  "findings": [{"category": "string", "severity": "info|minor|major|critical", "location": "file:line or section", "description": "what you found", "evidence": "what led you to this"}]
}

Rules:
- Focus on the CHANGES in the diff, not pre-existing issues.
- Be descriptive, never prescriptive.
- Every finding needs concrete evidence from the diff or codebase.
- Be concise. Short descriptions, minimal evidence quotes.
- If the commit is trivial (rename, formatting, comments only), return {"findings": []}.
- When you are done, output ONLY your JSON.`, root)
}

func buildMRReviewPrompt(root string) string {
	return fmt.Sprintf(`You are a code review agent. Your task is to review a merge request diff and produce structured findings.

Workspace root: %s

Tools available (all paths relative to workspace root):
- read_file: read file contents (with optional line range)
- grep: regex search inside files
- glob: find files by name pattern (e.g. **/*.go)
- list_dir: list directory entries
- git_log: recent commit history
- git_diff: compare two refs or see changes to a specific file
- git_blame: see who last modified each line and when
- git_show: inspect a specific commit
- fork: split into parallel sub-tasks sharing your current context

Process:
1. Analyze the diff provided in the user message
2. Explore the workspace to understand context: read changed files, grep for related usage, use git_blame/git_diff to understand change history
3. When you have enough context, use fork to run parallel focused reviews:
   - One sub-task for correctness and logic errors
   - One sub-task for security implications
   - One sub-task for consistency with existing patterns
   Each fork sub-task should produce its own findings JSON.
4. Combine fork results into your final output

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
