package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	defaultMaxIter = 12
	perIterTimeout = 180 * time.Second
	totalTimeout   = 5 * time.Minute
	stuckThreshold = 3
)

func runAgent(ctx context.Context, llm *LLMClient, model ModelDef, root, artifactPath, focus string, maxIter int) (*ReviewResult, error) {
	if maxIter <= 0 {
		maxIter = defaultMaxIter
	}

	tracer, err := newTracer(artifactPath)
	if err != nil {
		return nil, fmt.Errorf("init tracer: %w", err)
	}
	defer tracer.Close()

	ctx, cancel := context.WithTimeout(ctx, totalTimeout)
	defer cancel()

	systemPrompt := buildSystemPrompt(artifactPath, focus, root)

	messages := []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: fmt.Sprintf("Review the artifact at %q with focus: %s\n\nStart by reading the artifact, then explore the workspace as needed to build context. When you have enough information, produce your final findings.", artifactPath, focus)},
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
			Temperature: 0.3,
		}

		resp, err := llm.Chat(iterCtx, req)
		iterCancel()

		if err != nil {
			tracer.Log(TraceEntry{Iteration: iter, Role: "error", Content: err.Error()})
			if ctx.Err() != nil {
				return collectPartial(model.ID, contextPulled, iter, true), nil
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
			return parseFinalResponse(ctx, llm, model, messages, msg, tracer, contextPulled, iter)
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
			return collectPartial(model.ID, contextPulled, iter, true), nil
		}

		if totalTokens > model.TokenCeiling {
			tracer.Log(TraceEntry{Iteration: iter, Role: "system", Content: fmt.Sprintf("token ceiling reached: %d", totalTokens)})
			return collectPartial(model.ID, contextPulled, iter, true), nil
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

	return collectPartial(model.ID, contextPulled, maxIter, true), nil
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

func dispatchTool(root string, tc llmTool, contextPulled *[]string) ToolResult {
	var args map[string]any
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		return ToolResult{Content: fmt.Sprintf("invalid arguments: %s", err), IsError: true}
	}

	switch tc.Function.Name {
	case "read_file":
		path, _ := args["path"].(string)
		start, _ := args["start"].(float64)
		end, _ := args["end"].(float64)
		*contextPulled = append(*contextPulled, path)
		return execReadFile(root, path, int(start), int(end))

	case "grep":
		pattern, _ := args["pattern"].(string)
		path, _ := args["path"].(string)
		glob, _ := args["glob"].(string)
		*contextPulled = append(*contextPulled, fmt.Sprintf("grep:%s in %s", pattern, path))
		return execGrep(root, pattern, path, glob)

	case "list_dir":
		path, _ := args["path"].(string)
		*contextPulled = append(*contextPulled, fmt.Sprintf("ls:%s", path))
		return execListDir(root, path)

	default:
		return ToolResult{Content: fmt.Sprintf("unknown tool: %s", tc.Function.Name), IsError: true}
	}
}

func toolCallSignature(calls []llmTool) string {
	var parts []string
	for _, c := range calls {
		parts = append(parts, c.Function.Name+":"+c.Function.Arguments)
	}
	return strings.Join(parts, "|")
}

func parseFinalResponse(ctx context.Context, llm *LLMClient, model ModelDef, messages []chatMessage, msg chatMessage, tracer *Tracer, contextPulled []string, iter int) (*ReviewResult, error) {
	content := msg.Content

	var raw struct {
		Findings      []Finding `json:"findings"`
		OpenQuestions []string  `json:"open_questions"`
	}

	if err := json.Unmarshal([]byte(content), &raw); err == nil && len(raw.Findings) > 0 {
		return &ReviewResult{
			Findings:       raw.Findings,
			OpenQuestions:  raw.OpenQuestions,
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
			Content: "Your response was not valid JSON matching the required schema. Please output ONLY a JSON object with 'findings' (array) and 'open_questions' (array). No other text.",
		})

		req := chatRequest{
			Model:    model.ID,
			Messages: repairMessages,
			ResponseFormat: map[string]any{
				"type":        "json_schema",
				"json_schema": map[string]any{"name": "review_output", "schema": findingsJSONSchema},
			},
			Temperature: 0.1,
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

		if err := json.Unmarshal([]byte(repaired), &raw); err == nil && raw.Findings != nil {
			return &ReviewResult{
				Findings:       raw.Findings,
				OpenQuestions:  raw.OpenQuestions,
				ContextPulled:  contextPulled,
				IterationsUsed: iter,
				Truncated:      false,
				ModelUsed:      model.ID,
			}, nil
		}
	}

	return nil, fmt.Errorf("failed to parse final output after repair attempts")
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
