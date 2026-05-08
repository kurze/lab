package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var codeBlockRe = regexp.MustCompile("(?s)```(?:json)?\\s*\n?(.*?)\\s*```")

func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "{") {
		return s
	}
	if m := codeBlockRe.FindStringSubmatch(s); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	if start := strings.Index(s, "{"); start >= 0 {
		return s[start:]
	}
	return s
}

const (
	defaultMaxIter = 12
	perIterTimeout = 5 * time.Minute
	totalTimeout   = 20 * time.Minute
	stuckThreshold = 3
)

func runAgent(ctx context.Context, llm *LLMClient, model ModelDef, root, artifactPath, focus string, maxIter int) (*ReviewResult, error) {
	systemPrompt := buildSystemPrompt(artifactPath, focus, root)

	lr, err := runLoop(ctx, llm, LoopConfig{
		Model:       model,
		Root:        root,
		Temperature: 0.3,
		MaxIter:     maxIter,
		TracerTag:   artifactPath,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: fmt.Sprintf("Review the artifact at %q with focus: %s\n\nStart by reading the artifact, then explore the workspace as needed to build context. When you have enough information, produce your final findings.", artifactPath, focus)},
		},
	})
	if err != nil {
		return nil, err
	}

	if lr.Truncated {
		return collectPartial(model.ID, lr.ContextPulled, lr.Iteration, true), nil
	}

	return parseReviewOutput(ctx, llm, model, lr)
}

func buildSystemPrompt(artifactPath, focus, root string) string {
	return fmt.Sprintf(`You are a technical review agent. Your task is to independently review an artifact and produce structured findings.

Artifact: %s
Focus area: %s
Workspace root: %s

You have three tools: read_file, grep, list_dir. All paths are relative to the workspace root.

Process:
1. Read the artifact file
2. Explore the workspace to gather context relevant to the focus area
3. When ready, produce your final output as a JSON object

Your final output MUST be a JSON object with exactly these fields:
{
  "findings": [{"category": "missing|inconsistent|risk|assumption|pattern_match", "severity": "info|minor|major", "location": "file:line or section", "description": "what you found", "evidence": "what led you to this"}],
  "open_questions": ["questions that need human input"]
}

Rules:
- Be descriptive, never prescriptive. Say what you found, not what to do about it.
- Every finding needs concrete evidence from the artifact or codebase.
- When you are done exploring, stop calling tools and output your JSON.`, artifactPath, focus, root)
}

func parseReviewOutput(ctx context.Context, llm *LLMClient, model ModelDef, lr *LoopResult) (*ReviewResult, error) {
	content := extractJSON(lr.FinalMessage.Content)

	var raw struct {
		Findings     []Finding `json:"findings"`
		OpenQuestions []string  `json:"open_questions"`
	}

	if err := json.Unmarshal([]byte(content), &raw); err == nil && len(raw.Findings) > 0 {
		return &ReviewResult{
			Findings:       raw.Findings,
			OpenQuestions:  raw.OpenQuestions,
			ContextPulled:  lr.ContextPulled,
			IterationsUsed: lr.Iteration,
			ModelUsed:      model.ID,
		}, nil
	}

	tracer, _ := newTracer("repair-" + lr.Messages[0].Content[:20])
	defer tracer.Close()

	repaired, err := repairJSON[struct {
		Findings     []Finding `json:"findings"`
		OpenQuestions []string  `json:"open_questions"`
	}](ctx, llm, model, lr.Messages, content, findingsJSONSchema, "review_output",
		"Your response was not valid JSON matching the required schema. Please output ONLY a JSON object with 'findings' (array) and 'open_questions' (array). No other text.",
		tracer, lr.Iteration)

	if err != nil {
		return nil, fmt.Errorf("failed to parse final output after repair attempts")
	}

	return &ReviewResult{
		Findings:       repaired.Findings,
		OpenQuestions:  repaired.OpenQuestions,
		ContextPulled:  lr.ContextPulled,
		IterationsUsed: lr.Iteration,
		ModelUsed:      model.ID,
	}, nil
}

func collectPartial(modelID string, contextPulled []string, iter int, truncated bool) *ReviewResult {
	return &ReviewResult{
		Findings:       []Finding{},
		OpenQuestions:  []string{"review was truncated before completion"},
		ContextPulled:  contextPulled,
		IterationsUsed: iter,
		Truncated:      truncated,
		ModelUsed:      modelID,
	}
}
