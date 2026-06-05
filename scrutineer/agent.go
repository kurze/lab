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
	ReviewCfg    ReviewPromptConfig
	mu           sync.Mutex
}

func (r *LLMReviewer) Review(ctx context.Context, workDir string, diff string) (*ReviewResult, error) {
	return r.review(ctx, workDir, diff, false)
}

func (r *LLMReviewer) ReviewFull(ctx context.Context, workDir string, diff string) (*ReviewResult, error) {
	return r.review(ctx, workDir, diff, true)
}

func (r *LLMReviewer) review(ctx context.Context, workDir string, diff string, full bool) (*ReviewResult, error) {
	pc := PromptConfig{
		Root:       workDir,
		Focus:      r.ReviewCfg.Focus,
		Guidelines: r.ReviewCfg.Guidelines,
	}

	var systemPrompt string
	maxIter := 6
	maxForkDepth := 0
	tracerTag := "commit-review"

	if full {
		systemPrompt = BuildMRReviewPrompt(pc)
		maxIter = 12
		maxForkDepth = 1
		tracerTag = "mr-review"
	} else {
		systemPrompt = BuildCommitReviewPrompt(pc)
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
	pc := PromptConfig{
		Root:         workDir,
		PriorContext: priorContext,
		Focus:        r.ReviewCfg.Focus,
		Guidelines:   r.ReviewCfg.Guidelines,
	}
	systemPrompt := BuildMRRepassPrompt(pc)

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
			{Role: "system", Content: BuildExtractionPrompt()},
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

