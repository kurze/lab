package main

import (
	"context"
	"encoding/json"
	"fmt"
)

type ComparisonFinding struct {
	Category    string `json:"category"`
	Section     string `json:"section"`
	Description string `json:"description"`
	Impact      string `json:"impact"`
}

type CompareResult struct {
	Findings       []ComparisonFinding `json:"findings"`
	OpenQuestions   []string            `json:"open_questions"`
	ContextPulled   []string            `json:"context_pulled"`
	IterationsUsed  int                 `json:"iterations_used"`
	Truncated       bool                `json:"truncated"`
	ModelUsed       string              `json:"model_used"`
	ElapsedSec      float64             `json:"elapsed_sec"`
	TokensUsed      int                 `json:"tokens_used"`
}

func runCompare(ctx context.Context, llm *LLMClient, model ModelDef, root, oldPath, newPath, focus string, maxIter int) (*CompareResult, error) {
	systemPrompt := buildCompareSystemPrompt(oldPath, newPath, focus, root)

	lr, err := runLoop(ctx, llm, LoopConfig{
		Model:       model,
		Root:        root,
		Temperature: 0.3,
		MaxIter:     maxIter,
		MaxTokens:   defaultMaxTokens,
		TracerTag:   "compare-" + sanitizeTracerTag(newPath),
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: fmt.Sprintf("Compare the old version at %q with the new version at %q. Focus: %s\n\nRead both files, explore the workspace for context, then produce your findings about what the changes mean.", oldPath, newPath, focus)},
		},
	})
	if err != nil {
		return nil, err
	}
	defer lr.Tracer.Close()

	if lr.Truncated {
		return &CompareResult{
			Findings:       []ComparisonFinding{},
			OpenQuestions:   []string{"comparison was truncated before completion"},
			ContextPulled:   lr.ContextPulled,
			IterationsUsed:  lr.Iteration,
			Truncated:       true,
			ModelUsed:       model.ID,
			ElapsedSec:      lr.ElapsedSec,
			TokensUsed:      lr.TokensUsed,
		}, nil
	}

	return parseCompareOutput(ctx, llm, model, lr)
}

func buildCompareSystemPrompt(oldPath, newPath, focus, root string) string {
	return fmt.Sprintf(`You are a technical review agent specializing in artifact revision analysis. Your task is to compare two versions of a document and produce structured findings about what changed and what the changes mean.

Old version: %s
New version: %s
Focus area: %s
Workspace root: %s

You have four tools: read_file, grep, list_dir, git_log. All paths are relative to the workspace root.

Process:
1. Read both the old and new versions
2. Identify what changed: added sections, removed sections, modified claims, changed assumptions
3. Explore the workspace to understand the impact of changes
4. When ready, produce your final output as a JSON object

Your final output MUST be a JSON object with exactly these fields:
{
  "findings": [{"category": "added|removed|changed|weakened|strengthened|contradiction", "section": "which part changed", "description": "what changed", "impact": "what this change means — new risks, resolved issues, or opened questions"}],
  "open_questions": ["questions the revision raises that need human input"]
}

Rules:
- Focus on the MEANING of changes, not just what text differs.
- Flag contradictions between old and new versions.
- Flag assumptions that were quietly dropped or added.
- Be concise. Short descriptions, brief impact assessments. No preamble outside the JSON.
- When done exploring, output ONLY your JSON.`, oldPath, newPath, focus, root)
}

func parseCompareOutput(ctx context.Context, llm *LLMClient, model ModelDef, lr *LoopResult) (*CompareResult, error) {
	content := extractJSON(lr.FinalMessage.Content)

	var raw struct {
		Findings     []ComparisonFinding `json:"findings"`
		OpenQuestions []string            `json:"open_questions"`
	}

	if err := json.Unmarshal([]byte(content), &raw); err == nil && raw.Findings != nil {
		return &CompareResult{
			Findings:       raw.Findings,
			OpenQuestions:   raw.OpenQuestions,
			ContextPulled:   lr.ContextPulled,
			IterationsUsed:  lr.Iteration,
			ModelUsed:       model.ID,
			ElapsedSec:      lr.ElapsedSec,
			TokensUsed:      lr.TokensUsed,
		}, nil
	}

	repaired, err := repairJSON[struct {
		Findings     []ComparisonFinding `json:"findings"`
		OpenQuestions []string            `json:"open_questions"`
	}](ctx, llm, model, lr.Messages, content,
		"Your response was not valid JSON. Please output ONLY a JSON object with 'findings' (array of objects with category/section/description/impact) and 'open_questions' (array of strings). No markdown, no explanation, just the JSON.",
		lr.Tracer, lr.Iteration)

	if err != nil {
		return nil, fmt.Errorf("failed to parse comparison output after repair attempts")
	}

	return &CompareResult{
		Findings:       repaired.Findings,
		OpenQuestions:   repaired.OpenQuestions,
		ContextPulled:   lr.ContextPulled,
		IterationsUsed:  lr.Iteration,
		ModelUsed:       model.ID,
		ElapsedSec:      lr.ElapsedSec,
		TokensUsed:      lr.TokensUsed,
	}, nil
}
