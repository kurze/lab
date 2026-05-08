package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kurze/lab/agentcore"
)

const (
	defaultMaxIter   = 12
	defaultMaxTokens = 3000
	agentName        = "wlx-review-agent"
)

func runAgent(ctx context.Context, llm *agentcore.LLMClient, model ModelDef, root, artifactPath, focus string, maxIter int) (*ReviewResult, error) {
	systemPrompt := buildSystemPrompt(artifactPath, focus, root)

	lr, err := agentcore.RunLoop(ctx, llm, agentcore.LoopConfig{
		ModelID:        model.ID,
		ContextSize:    model.ContextSize,
		TokenCeiling:   model.TokenCeiling,
		Root:           root,
		Temperature:    0.3,
		MaxIter:        maxIter,
		MaxTokens:      defaultMaxTokens,
		AgentName:      agentName,
		TracerTag:      artifactPath,
		Tools:          agentcore.StandardToolDefs(),
		ToolDispatcher: agentcore.StandardToolDispatch,
		Messages: []agentcore.ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: fmt.Sprintf("Review the artifact at %q with focus: %s\n\nStart by reading the artifact, then explore the workspace as needed to build context. When you have enough information, produce your final findings.", artifactPath, focus)},
		},
	})
	if err != nil {
		return nil, err
	}

	defer lr.Tracer.Close()

	if lr.Truncated {
		return collectPartial(model.ID, lr), nil
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
- Be concise. Short descriptions, minimal evidence quotes. No preamble or explanation outside the JSON.
- When you are done exploring, stop calling tools and output ONLY your JSON.`, artifactPath, focus, root)
}

func parseReviewOutput(ctx context.Context, llm *agentcore.LLMClient, model ModelDef, lr *agentcore.LoopResult) (*ReviewResult, error) {
	content := agentcore.ExtractJSON(lr.FinalMessage.Content)

	var raw struct {
		Findings     []Finding `json:"findings"`
		OpenQuestions []string `json:"open_questions"`
	}

	if err := json.Unmarshal([]byte(content), &raw); err == nil && raw.Findings != nil {
		return &ReviewResult{
			Findings:       raw.Findings,
			OpenQuestions:  raw.OpenQuestions,
			ContextPulled:  lr.ContextPulled,
			IterationsUsed: lr.Iteration,
			ModelUsed:      model.ID,
			ElapsedSec:     lr.ElapsedSec,
			TokensUsed:     lr.TokensUsed,
		}, nil
	}

	repaired, err := agentcore.RepairJSON[struct {
		Findings     []Finding `json:"findings"`
		OpenQuestions []string `json:"open_questions"`
	}](ctx, llm, model.ID, lr.Messages, content,
		"Your response was not valid JSON. Please output ONLY a JSON object with 'findings' (array of objects with category/severity/location/description/evidence) and 'open_questions' (array of strings). No markdown, no explanation, just the JSON.",
		lr.Tracer, lr.Iteration)

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

func collectPartial(modelID string, lr *agentcore.LoopResult) *ReviewResult {
	return &ReviewResult{
		Findings:       []Finding{},
		OpenQuestions:  []string{"review was truncated before completion"},
		ContextPulled:  lr.ContextPulled,
		IterationsUsed: lr.Iteration,
		Truncated:      true,
		ModelUsed:      modelID,
		ElapsedSec:     lr.ElapsedSec,
		TokensUsed:     lr.TokensUsed,
	}
}
