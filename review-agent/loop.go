package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type LoopConfig struct {
	Model       ModelDef
	Root        string
	Messages    []chatMessage
	Temperature float64
	MaxIter     int
	MaxTokens   int
	TracerTag   string
}

type LoopResult struct {
	FinalMessage  chatMessage
	Messages      []chatMessage
	ContextPulled []string
	Iteration     int
	Truncated     bool
	Tracer        *Tracer
	ElapsedSec    float64
	TokensUsed    int
}

func runLoop(ctx context.Context, llm *LLMClient, cfg LoopConfig) (lr *LoopResult, retErr error) {
	maxIter := cfg.MaxIter
	if maxIter <= 0 {
		maxIter = defaultMaxIter
	}

	start := time.Now()

	tracer, err := newTracer(cfg.TracerTag)
	if err != nil {
		return nil, fmt.Errorf("init tracer: %w", err)
	}
	defer func() {
		if retErr != nil {
			tracer.Close()
		}
	}()

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
	lastTokens := 0
	nudged := false

	for iter := 1; iter <= maxIter; iter++ {
		iterCtx, iterCancel := context.WithTimeout(ctx, perIterTimeout)

		req := chatRequest{
			Model:       cfg.Model.ID,
			Messages:    messages,
			Tools:       agentTools,
			Temperature: cfg.Temperature,
			MaxTokens:   cfg.MaxTokens,
		}

		resp, err := llm.Chat(iterCtx, req)
		if err != nil && ctx.Err() == nil {
			tracer.Log(TraceEntry{Iteration: iter, Role: "error", Content: fmt.Sprintf("retrying: %s", err)})
			time.Sleep(2 * time.Second)
			resp, err = llm.Chat(iterCtx, req)
		}
		iterCancel()

		if err != nil {
			tracer.Log(TraceEntry{Iteration: iter, Role: "error", Content: err.Error()})
			if ctx.Err() != nil {
				elapsed := time.Since(start).Seconds()
				return &LoopResult{ContextPulled: contextPulled, Iteration: iter, Truncated: true, Tracer: tracer, ElapsedSec: elapsed, TokensUsed: lastTokens}, nil
			}
			return nil, fmt.Errorf("iteration %d: %w", iter, err)
		}

		if len(resp.Choices) == 0 {
			return nil, fmt.Errorf("iteration %d: empty response from LLM", iter)
		}

		lastTokens = resp.Usage.TotalTokens
		msg := resp.Choices[0].Message
		tracer.Log(TraceEntry{Iteration: iter, Role: "assistant", Content: msg.Content, ToolCalls: msg.ToolCalls})

		if len(msg.ToolCalls) == 0 || resp.Choices[0].FinishReason == "stop" {
			if strings.TrimSpace(msg.Content) == "" && iter < maxIter && !nudged {
				nudged = true
				tracer.Log(TraceEntry{Iteration: iter, Role: "system", Content: "empty final response, nudging model"})
				messages = append(messages, chatMessage{Role: "assistant", Content: msg.Content})
				messages = append(messages, chatMessage{Role: "user", Content: "You stopped without producing output. Please produce your final JSON now."})
				continue
			}
			elapsed := time.Since(start).Seconds()
			return &LoopResult{
				FinalMessage:  msg,
				Messages:      messages,
				ContextPulled: contextPulled,
				Iteration:     iter,
				Truncated:     false,
				Tracer:        tracer,
				ElapsedSec:    elapsed,
				TokensUsed:    lastTokens,
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
			elapsed := time.Since(start).Seconds()
			return &LoopResult{ContextPulled: contextPulled, Iteration: iter, Truncated: true, Tracer: tracer, ElapsedSec: elapsed, TokensUsed: lastTokens}, nil
		}

		if resp.Usage.TotalTokens > cfg.Model.TokenCeiling {
			tracer.Log(TraceEntry{Iteration: iter, Role: "system", Content: fmt.Sprintf("token ceiling reached: %d", resp.Usage.TotalTokens)})
			elapsed := time.Since(start).Seconds()
			return &LoopResult{ContextPulled: contextPulled, Iteration: iter, Truncated: true, Tracer: tracer, ElapsedSec: elapsed, TokensUsed: lastTokens}, nil
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

		messages = compactMessages(messages, cfg.Model.ContextSize)
	}

	elapsed := time.Since(start).Seconds()
	return &LoopResult{ContextPulled: contextPulled, Iteration: maxIter, Truncated: true, Tracer: tracer, ElapsedSec: elapsed, TokensUsed: lastTokens}, nil
}

func estimateTokens(messages []chatMessage) int {
	total := 0
	for _, m := range messages {
		total += len(m.Content)
		for _, tc := range m.ToolCalls {
			total += len(tc.Function.Arguments)
		}
	}
	return total / 4
}

func compactMessages(messages []chatMessage, contextSize int) []chatMessage {
	threshold := contextSize * 3 / 4
	if estimateTokens(messages) < threshold {
		return messages
	}

	// Keep system + user (first 2) and the last 6 messages intact.
	// Compress older tool results to a one-line summary.
	protect := 6
	if len(messages) <= 2+protect {
		return messages
	}

	for i := 2; i < len(messages)-protect; i++ {
		if messages[i].Role == "tool" && len(messages[i].Content) > 200 {
			first := messages[i].Content
			if nl := strings.Index(first, "\n"); nl > 0 {
				first = first[:nl]
			}
			if len(first) > 200 {
				first = first[:200]
			}
			messages[i].Content = first + "\n[compacted]"
		}
	}
	return messages
}

func repairJSON[T any](ctx context.Context, llm *LLMClient, model ModelDef, messages []chatMessage, content string, repairPrompt string, tracer *Tracer, iter int) (*T, error) {
	for attempt := 0; attempt < 2; attempt++ {
		repairMessages := make([]chatMessage, len(messages), len(messages)+2)
		copy(repairMessages, messages)
		repairMessages = append(repairMessages, chatMessage{Role: "assistant", Content: content})
		repairMessages = append(repairMessages, chatMessage{
			Role:    "user",
			Content: repairPrompt,
		})

		req := chatRequest{
			Model:       model.ID,
			Messages:    repairMessages,
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

		repaired := extractJSON(resp.Choices[0].Message.Content)
		tracer.Log(TraceEntry{Iteration: iter, Role: "repair", Content: repaired})

		var result T
		if err := json.Unmarshal([]byte(repaired), &result); err == nil {
			return &result, nil
		}
	}
	return nil, fmt.Errorf("failed to parse output after repair attempts")
}
