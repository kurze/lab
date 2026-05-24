package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/kurze/lab/agentcore"
)

const agentName = "scrutineer"

type LLMReviewer struct {
	LLM          *agentcore.LLMClient
	Model        string
	ContextSize  int
	TokenCeiling int
	Temperature  float64
	TraceMeta    map[string]string
	TraceDir     string
	Verbose      bool
	mu           sync.Mutex
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

	r.mu.Lock()
	meta := cloneMap(r.TraceMeta)
	r.mu.Unlock()

	cfg := agentcore.LoopConfig{
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
		TraceMeta:      meta,
		TraceDir:       r.TraceDir,
		Tools:          agentcore.StandardToolDefs(),
		ToolDispatcher: agentcore.StandardToolDispatch,
		Messages: []agentcore.ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: fmt.Sprintf("Here is the diff to review:\n\n```diff\n%s\n```\n\nReview it and produce your findings.", diff)},
		},
	}
	if r.Verbose {
		cfg.OnTrace = verboseTraceFunc()
	}

	lr, err := agentcore.RunLoop(ctx, r.LLM, cfg)
	if err != nil {
		return nil, err
	}
	defer lr.Tracer.Close()

	reviewText := lr.FinalMessage.Content
	if lr.Truncated || reviewText == "" {
		return &ReviewResult{Findings: []Finding{}, Model: r.Model, TokensUsed: lr.TokensUsed}, nil
	}

	result, err := r.extractFindings(ctx, reviewText, lr.Tracer)
	if err != nil {
		warnf("extraction failed, returning empty findings: %v", err)
		return &ReviewResult{Findings: []Finding{}, Model: r.Model, TokensUsed: lr.TokensUsed}, nil
	}
	result.Model = r.Model
	result.TokensUsed = lr.TokensUsed
	return result, nil
}

func (r *LLMReviewer) ReviewWithContext(ctx context.Context, workDir string, diff string, priorContext string) (*ReviewResult, error) {
	systemPrompt := buildMRRepassPrompt(workDir, priorContext)

	temp := r.Temperature
	if temp == 0 {
		temp = 0.3
	}

	r.mu.Lock()
	meta := cloneMap(r.TraceMeta)
	r.mu.Unlock()

	cfg := agentcore.LoopConfig{
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
		TraceMeta:      meta,
		TraceDir:       r.TraceDir,
		Tools:          agentcore.StandardToolDefs(),
		ToolDispatcher: agentcore.StandardToolDispatch,
		Messages: []agentcore.ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: fmt.Sprintf("Here is the full merge request diff to review:\n\n```diff\n%s\n```\n\nThe prior findings digest above covers what was found per-commit. Focus on cross-cutting concerns only.", diff)},
		},
	}
	if r.Verbose {
		cfg.OnTrace = verboseTraceFunc()
	}

	lr, err := agentcore.RunLoop(ctx, r.LLM, cfg)
	if err != nil {
		return nil, err
	}
	defer lr.Tracer.Close()

	reviewText := lr.FinalMessage.Content
	if lr.Truncated || reviewText == "" {
		return &ReviewResult{Findings: []Finding{}, Model: r.Model, TokensUsed: lr.TokensUsed}, nil
	}

	result, err := r.extractFindings(ctx, reviewText, lr.Tracer)
	if err != nil {
		warnf("extraction failed, returning empty findings: %v", err)
		return &ReviewResult{Findings: []Finding{}, Model: r.Model, TokensUsed: lr.TokensUsed}, nil
	}
	result.Model = r.Model
	result.TokensUsed = lr.TokensUsed
	return result, nil
}

func (r *LLMReviewer) extractFindings(ctx context.Context, reviewText string, tracer *agentcore.Tracer) (*ReviewResult, error) {
	if tracer != nil {
		tracer.Log(agentcore.TraceEntry{Role: "system", Content: "[extraction] sending review text for structured finding extraction"})
	}

	start := time.Now()
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

	if tracer != nil && len(resp.Choices) > 0 {
		tracer.Log(agentcore.TraceEntry{
			Role:             "assistant",
			Content:          "[extraction] " + resp.Choices[0].Message.Content,
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
			LatencyMs:        time.Since(start).Milliseconds(),
		})
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

Prior findings (already reported):
<prior_findings>
%s
</prior_findings>

Workspace root: %s

Tools (paths relative to root):
- read_file, grep, glob, list_dir, git_log, git_diff, git_blame, git_show
- fork: split into parallel sub-tasks (use this!)

Process:
1. EXPLORE: read the full diff. Use grep/read_file to trace how changes across commits interact — shared state, call chains, data flow.
2. Call the fork tool with these four tasks (same categories, but focus on cross-commit interactions):
   - id:"bugs", prompt:"Hunt for logic errors that emerge from cross-commit interactions: shared state broken by different commits, inconsistent error handling, assumptions in one commit violated by another."
   - id:"security", prompt:"Hunt for security issues spanning multiple commits: auth gaps, input validation missing on new paths, secrets exposure, unsafe data flow across changed boundaries."
   - id:"perf", prompt:"Hunt for performance issues at branch scale: redundant work across commits, new hot paths without caching, unbounded growth introduced by combined changes."
   - id:"style", prompt:"Hunt for branch-wide consistency: repeated anti-patterns, inconsistent naming or conventions, missing tests for new code paths, dead code left behind."

Output: plain text. For each finding: file:line, severity (info/minor/major/critical), what you found, short evidence quote. If no new issues, say "No cross-cutting issues found."

Rules: NEVER repeat prior findings. Focus on what spans multiple commits or emerges from the full picture. Descriptive, not prescriptive. 1-2 sentences per finding.`, priorContext, root)
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
	return fmt.Sprintf(`You are a code review agent. Review the merge request diff. Be brief — concise findings.

Workspace root: %s

Tools (paths relative to root):
- read_file, grep, glob, list_dir, git_log, git_diff, git_blame, git_show
- fork: split into parallel sub-tasks (use this!)

Process:
1. EXPLORE: read the diff carefully. Use grep/read_file to understand the surrounding code — call sites, types, invariants. Build context before judging.
2. Call the fork tool with these four tasks:
   - id:"bugs", prompt:"Hunt for logic errors, nil derefs, off-by-one, race conditions, missing error handling, broken invariants. Read changed files and their callers for evidence."
   - id:"security", prompt:"Hunt for injection, auth bypass, secrets exposure, unsafe input handling, path traversal. Trace data flow from inputs to sensitive operations."
   - id:"perf", prompt:"Hunt for unnecessary allocations, O(n²) loops, missing caching, unbounded growth, blocking calls. Check hot paths."
   - id:"style", prompt:"Hunt for dead code, naming issues, unclear control flow, missing or misleading abstractions."

Output: plain text. For each finding: file:line, severity (info/minor/major/critical), what you found, short evidence quote.

Rules: focus on changes only, not pre-existing issues. Descriptive, not prescriptive. 1-2 sentences per finding. If a category has no findings, skip it.`, root)
}

func cloneMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func verboseTraceFunc() func(agentcore.TraceEntry) {
	return func(e agentcore.TraceEntry) {
		content := e.Content
		if len(content) > 120 {
			content = content[:120] + "…"
		}
		content = strings.ReplaceAll(content, "\n", " ")

		var tokens string
		if e.TotalTokens > 0 {
			tokens = fmt.Sprintf(" %s", formatTokens(e.TotalTokens))
		}
		var latency string
		if e.LatencyMs > 0 {
			latency = fmt.Sprintf(" %dms", e.LatencyMs)
		}

		fmt.Fprintf(os.Stderr, "%s [iter %d] %s: %s%s%s\n",
			cl(ansiDim, time.Now().Format("15:04:05")),
			e.Iteration,
			cl(roleColor(e.Role), e.Role),
			content,
			cl(ansiDim, tokens),
			cl(ansiDim, latency),
		)
	}
}

func roleColor(role string) string {
	switch role {
	case "system":
		return ansiCyan
	case "assistant":
		return ansiGreen
	case "user":
		return ansiBlue
	case "tool":
		return ansiDim
	case "error":
		return ansiRed
	default:
		return ""
	}
}

