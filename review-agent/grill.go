package main

import (
	"context"
	"encoding/json"
	"fmt"
)

func runGrill(ctx context.Context, llm *LLMClient, model ModelDef, root, artifactPath, focus string, maxIter int) (*GrillResult, error) {
	if maxIter <= 0 {
		maxIter = defaultMaxIter
	}

	tracer, err := newTracer("grill-" + artifactPath)
	if err != nil {
		return nil, fmt.Errorf("init tracer: %w", err)
	}
	defer tracer.Close()

	ctx, cancel := context.WithTimeout(ctx, totalTimeout)
	defer cancel()

	systemPrompt := buildGrillSystemPrompt(artifactPath, focus, root)

	messages := []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: fmt.Sprintf("Read the artifact at %q and the surrounding workspace, then produce your toughest questions. Focus: %s", artifactPath, focus)},
	}

	tracer.Log(TraceEntry{Iteration: 0, Role: "system", Content: systemPrompt})
	tracer.Log(TraceEntry{Iteration: 0, Role: "user", Content: messages[1].Content})

	var contextPulled []string
	var lastToolSig string
	stuckCount := 0
	totalTokens := 0

	for iter := 1; iter <= maxIter; iter++ {
		iterCtx, iterCancel := context.WithTimeout(ctx, perIterTimeout)

		req := chatRequest{
			Model:       model.ID,
			Messages:    messages,
			Tools:       agentTools,
			Temperature: 0.5,
		}

		resp, err := llm.Chat(iterCtx, req)
		iterCancel()

		if err != nil {
			tracer.Log(TraceEntry{Iteration: iter, Role: "error", Content: err.Error()})
			if ctx.Err() != nil {
				return collectGrillPartial(model.ID, contextPulled, iter, true), nil
			}
			return nil, fmt.Errorf("iteration %d: %w", iter, err)
		}

		if len(resp.Choices) == 0 {
			return nil, fmt.Errorf("iteration %d: empty response from LLM", iter)
		}

		totalTokens += resp.Usage.TotalTokens
		msg := resp.Choices[0].Message
		tracer.Log(TraceEntry{Iteration: iter, Role: "assistant", Content: msg.Content, ToolCalls: msg.ToolCalls})

		if len(msg.ToolCalls) == 0 || resp.Choices[0].FinishReason == "stop" {
			return parseGrillResponse(ctx, llm, model, messages, msg, tracer, contextPulled, iter)
		}

		sig := toolCallSignature(msg.ToolCalls)
		if sig == lastToolSig {
			stuckCount++
		} else {
			stuckCount = 0
		}
		lastToolSig = sig

		if stuckCount >= stuckThreshold {
			tracer.Log(TraceEntry{Iteration: iter, Role: "system", Content: "stuck detection triggered"})
			return collectGrillPartial(model.ID, contextPulled, iter, true), nil
		}

		if totalTokens > model.TokenCeiling {
			tracer.Log(TraceEntry{Iteration: iter, Role: "system", Content: fmt.Sprintf("token ceiling reached: %d", totalTokens)})
			return collectGrillPartial(model.ID, contextPulled, iter, true), nil
		}

		messages = append(messages, chatMessage{Role: "assistant", Content: msg.Content, ToolCalls: msg.ToolCalls})

		for _, tc := range msg.ToolCalls {
			result := dispatchTool(root, tc, &contextPulled)
			tracer.Log(TraceEntry{Iteration: iter, Role: "tool", Content: result.Content, ToolResults: tc.ID})
			messages = append(messages, chatMessage{
				Role:       "tool",
				Content:    result.Content,
				ToolCallID: tc.ID,
			})
		}
	}

	return collectGrillPartial(model.ID, contextPulled, maxIter, true), nil
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
- Aim for 5-15 questions, ranked by importance.`, artifactPath, focus, root)
}

func parseGrillResponse(ctx context.Context, llm *LLMClient, model ModelDef, messages []chatMessage, msg chatMessage, tracer *Tracer, contextPulled []string, iter int) (*GrillResult, error) {
	content := msg.Content

	var raw struct {
		Questions []GrillQuestion `json:"questions"`
	}

	if err := json.Unmarshal([]byte(content), &raw); err == nil && len(raw.Questions) > 0 {
		return &GrillResult{
			Questions:      raw.Questions,
			ContextPulled:  contextPulled,
			IterationsUsed: iter,
			Truncated:      false,
			ModelUsed:      model.ID,
		}, nil
	}

	for attempt := 0; attempt < 2; attempt++ {
		repairMessages := append(messages, chatMessage{Role: "assistant", Content: content})
		repairMessages = append(repairMessages, chatMessage{
			Role:    "user",
			Content: "Your response was not valid JSON matching the required schema. Please output ONLY a JSON object with a 'questions' array. Each question needs 'question', 'why', and 'category' fields. No other text.",
		})

		req := chatRequest{
			Model:    model.ID,
			Messages: repairMessages,
			ResponseFormat: map[string]any{
				"type":        "json_schema",
				"json_schema": map[string]any{"name": "grill_output", "schema": grillQuestionsJSONSchema},
			},
			Temperature: 0.3,
		}

		resp, err := llm.Chat(ctx, req)
		if err != nil {
			tracer.Log(TraceEntry{Iteration: iter, Role: "error", Content: fmt.Sprintf("repair attempt %d: %s", attempt, err)})
			continue
		}
		if len(resp.Choices) == 0 {
			continue
		}

		repaired := resp.Choices[0].Message.Content
		tracer.Log(TraceEntry{Iteration: iter, Role: "repair", Content: repaired})

		if err := json.Unmarshal([]byte(repaired), &raw); err == nil && raw.Questions != nil {
			return &GrillResult{
				Questions:      raw.Questions,
				ContextPulled:  contextPulled,
				IterationsUsed: iter,
				Truncated:      false,
				ModelUsed:      model.ID,
			}, nil
		}
	}

	return nil, fmt.Errorf("failed to parse grill output after repair attempts")
}

func collectGrillPartial(modelID string, contextPulled []string, iter int, truncated bool) *GrillResult {
	return &GrillResult{
		Questions:      []GrillQuestion{},
		ContextPulled:  contextPulled,
		IterationsUsed: iter,
		Truncated:      truncated,
		ModelUsed:      modelID,
	}
}
