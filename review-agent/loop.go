package main

import (
	"context"
	"encoding/json"
	"fmt"
)

type LoopConfig struct {
	Model       ModelDef
	Root        string
	Messages    []chatMessage
	Temperature float64
	MaxIter     int
	TracerTag   string
}

type LoopResult struct {
	FinalMessage  chatMessage
	Messages      []chatMessage
	ContextPulled []string
	Iteration     int
	Truncated     bool
	Tracer        *Tracer
}

func runLoop(ctx context.Context, llm *LLMClient, cfg LoopConfig) (*LoopResult, error) {
	maxIter := cfg.MaxIter
	if maxIter <= 0 {
		maxIter = defaultMaxIter
	}

	tracer, err := newTracer(cfg.TracerTag)
	if err != nil {
		return nil, fmt.Errorf("init tracer: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, totalTimeout)
	defer cancel()

	messages := make([]chatMessage, len(cfg.Messages))
	copy(messages, cfg.Messages)

	for _, m := range messages {
		tracer.Log(TraceEntry{Iteration: 0, Role: m.Role, Content: m.Content})
	}

	var contextPulled []string
	var lastToolSig string
	stuckCount := 0

	for iter := 1; iter <= maxIter; iter++ {
		iterCtx, iterCancel := context.WithTimeout(ctx, perIterTimeout)

		req := chatRequest{
			Model:       cfg.Model.ID,
			Messages:    messages,
			Tools:       agentTools,
			Temperature: cfg.Temperature,
		}

		resp, err := llm.Chat(iterCtx, req)
		iterCancel()

		if err != nil {
			tracer.Log(TraceEntry{Iteration: iter, Role: "error", Content: err.Error()})
			if ctx.Err() != nil {
				return &LoopResult{ContextPulled: contextPulled, Iteration: iter, Truncated: true, Tracer: tracer}, nil
			}
			return nil, fmt.Errorf("iteration %d: %w", iter, err)
		}

		if len(resp.Choices) == 0 {
			return nil, fmt.Errorf("iteration %d: empty response from LLM", iter)
		}

		msg := resp.Choices[0].Message
		tracer.Log(TraceEntry{Iteration: iter, Role: "assistant", Content: msg.Content, ToolCalls: msg.ToolCalls})

		if len(msg.ToolCalls) == 0 || resp.Choices[0].FinishReason == "stop" {
			return &LoopResult{
				FinalMessage:  msg,
				Messages:      messages,
				ContextPulled: contextPulled,
				Iteration:     iter,
				Truncated:     false,
				Tracer:        tracer,
			}, nil
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
			return &LoopResult{ContextPulled: contextPulled, Iteration: iter, Truncated: true, Tracer: tracer}, nil
		}

		if resp.Usage.TotalTokens > cfg.Model.TokenCeiling {
			tracer.Log(TraceEntry{Iteration: iter, Role: "system", Content: fmt.Sprintf("token ceiling reached: %d", resp.Usage.TotalTokens)})
			return &LoopResult{ContextPulled: contextPulled, Iteration: iter, Truncated: true, Tracer: tracer}, nil
		}

		messages = append(messages, chatMessage{Role: "assistant", Content: msg.Content, ToolCalls: msg.ToolCalls})

		for _, tc := range msg.ToolCalls {
			result := dispatchTool(cfg.Root, tc, &contextPulled)
			tracer.Log(TraceEntry{Iteration: iter, Role: "tool", Content: result.Content, ToolResults: tc.ID})
			messages = append(messages, chatMessage{
				Role:       "tool",
				Content:    result.Content,
				ToolCallID: tc.ID,
			})
		}
	}

	return &LoopResult{ContextPulled: contextPulled, Iteration: maxIter, Truncated: true, Tracer: tracer}, nil
}

func repairJSON[T any](ctx context.Context, llm *LLMClient, model ModelDef, messages []chatMessage, content string, schema map[string]any, schemaName string, repairPrompt string, tracer *Tracer, iter int) (*T, error) {
	for attempt := 0; attempt < 2; attempt++ {
		repairMessages := append(messages, chatMessage{Role: "assistant", Content: content})
		repairMessages = append(repairMessages, chatMessage{
			Role:    "user",
			Content: repairPrompt,
		})

		req := chatRequest{
			Model:    model.ID,
			Messages: repairMessages,
			ResponseFormat: map[string]any{
				"type":        "json_schema",
				"json_schema": map[string]any{"name": schemaName, "schema": schema},
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

		var result T
		if err := json.Unmarshal([]byte(repaired), &result); err == nil {
			return &result, nil
		}
	}
	return nil, fmt.Errorf("failed to parse output after repair attempts")
}
