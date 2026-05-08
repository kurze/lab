package main

import (
	"context"
	"encoding/json"
	"fmt"
)

func runGrill(ctx context.Context, llm *LLMClient, model ModelDef, root, artifactPath, focus string, maxIter int) (*GrillResult, error) {
	systemPrompt := buildGrillSystemPrompt(artifactPath, focus, root)

	lr, err := runLoop(ctx, llm, LoopConfig{
		Model:       model,
		Root:        root,
		Temperature: 0.5,
		MaxIter:     maxIter,
		MaxTokens:   defaultMaxTokens,
		TracerTag:   "grill-" + artifactPath,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: fmt.Sprintf("Read the artifact at %q and the surrounding workspace, then produce your toughest questions. Focus: %s", artifactPath, focus)},
		},
	})
	if err != nil {
		return nil, err
	}

	defer lr.Tracer.Close()

	if lr.Truncated {
		return collectGrillPartial(model.ID, lr), nil
	}

	return parseGrillOutput(ctx, llm, model, lr)
}

func buildGrillSystemPrompt(artifactPath, focus, root string) string {
	return fmt.Sprintf(`You are a senior technical interviewer. Your job is to read an artifact and its surrounding codebase, then generate the hardest, most useful questions a reviewer should ask the author.

Artifact: %s
Focus area: %s
Workspace root: %s

You have three tools: read_file, grep, list_dir. All paths are relative to the workspace root.

Process:
1. Read the artifact file thoroughly
2. Explore the workspace to understand the implementation context
3. Identify assumptions, gaps, risks, ambiguities, dependencies, and tradeoffs
4. When ready, produce your questions as a JSON object

Your final output MUST be a JSON object with exactly this field:
{
  "questions": [{"question": "the question itself", "why": "why this question matters — what's at stake", "category": "assumption|gap|risk|ambiguity|dependency|tradeoff"}]
}

Rules:
- Each question must be specific and grounded in something you observed in the artifact or code.
- Prioritize questions that expose hidden assumptions or things that could go wrong.
- Ask "what happens when..." and "how do you know that..." questions.
- Do not ask obvious or generic questions. Every question should make the author think.
- Be concise. One-sentence questions, brief "why" explanations. No preamble outside the JSON.
- Aim for 5-15 questions, ranked by importance.
- When ready, output ONLY your JSON. No other text.`, artifactPath, focus, root)
}

func parseGrillOutput(ctx context.Context, llm *LLMClient, model ModelDef, lr *LoopResult) (*GrillResult, error) {
	content := extractJSON(lr.FinalMessage.Content)

	var raw struct {
		Questions []GrillQuestion `json:"questions"`
	}

	if err := json.Unmarshal([]byte(content), &raw); err == nil && raw.Questions != nil {
		return &GrillResult{
			Questions:      raw.Questions,
			ContextPulled:  lr.ContextPulled,
			IterationsUsed: lr.Iteration,
			ModelUsed:      model.ID,
			ElapsedSec:     lr.ElapsedSec,
			TokensUsed:     lr.TokensUsed,
		}, nil
	}

	repaired, err := repairJSON[struct {
		Questions []GrillQuestion `json:"questions"`
	}](ctx, llm, model, lr.Messages, content,
		"Your response was not valid JSON. Please output ONLY a JSON object with a 'questions' array. Each question needs 'question' (string), 'why' (string), and 'category' (one of: assumption, gap, risk, ambiguity, dependency, tradeoff). No markdown, no explanation, just the JSON.",
		lr.Tracer, lr.Iteration)

	if err != nil {
		return nil, fmt.Errorf("failed to parse grill output after repair attempts")
	}

	return &GrillResult{
		Questions:      repaired.Questions,
		ContextPulled:  lr.ContextPulled,
		IterationsUsed: lr.Iteration,
		ModelUsed:      model.ID,
	}, nil
}

func collectGrillPartial(modelID string, lr *LoopResult) *GrillResult {
	return &GrillResult{
		Questions:      []GrillQuestion{},
		ContextPulled:  lr.ContextPulled,
		IterationsUsed: lr.Iteration,
		Truncated:      true,
		ModelUsed:      modelID,
		ElapsedSec:     lr.ElapsedSec,
		TokensUsed:     lr.TokensUsed,
	}
}
