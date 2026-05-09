package agentcore

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	DefaultMaxIter       = 12
	DefaultMaxTokens     = 3000
	DefaultPerIterTimeout = 5 * time.Minute
	DefaultTotalTimeout  = 20 * time.Minute
	DefaultStuckThreshold = 3
	DefaultNudgeMessage  = "You stopped without producing output. Please produce your final JSON now."
)

type LoopConfig struct {
	ModelID        string
	ContextSize    int
	TokenCeiling   int
	Root           string
	Messages       []ChatMessage
	Tools          []any
	ToolDispatcher func(root string, tc LLMTool, contextPulled *[]string, seen map[string]bool) ToolResult
	Temperature    float64
	MaxIter        int
	MaxTokens      int
	AgentName      string
	TracerTag      string
	NudgeMessage   string
	StuckThreshold int
	PerIterTimeout time.Duration
	TotalTimeout   time.Duration
}

type LoopResult struct {
	FinalMessage  ChatMessage
	Messages      []ChatMessage
	ContextPulled []string
	Iteration     int
	Truncated     bool
	Tracer        *Tracer
	ElapsedSec    float64
	TokensUsed    int
}

func (cfg *LoopConfig) defaults() {
	if cfg.MaxIter <= 0 {
		cfg.MaxIter = DefaultMaxIter
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = DefaultMaxTokens
	}
	if cfg.StuckThreshold <= 0 {
		cfg.StuckThreshold = DefaultStuckThreshold
	}
	if cfg.NudgeMessage == "" {
		cfg.NudgeMessage = DefaultNudgeMessage
	}
	if cfg.PerIterTimeout <= 0 {
		cfg.PerIterTimeout = DefaultPerIterTimeout
	}
	if cfg.TotalTimeout <= 0 {
		cfg.TotalTimeout = DefaultTotalTimeout
	}
	if cfg.AgentName == "" {
		cfg.AgentName = "agentcore"
	}
}

func RunLoop(ctx context.Context, llm *LLMClient, cfg LoopConfig) (lr *LoopResult, retErr error) {
	cfg.defaults()

	start := time.Now()

	tracer, err := NewTracer(cfg.AgentName, cfg.TracerTag)
	if err != nil {
		return nil, fmt.Errorf("init tracer: %w", err)
	}
	defer func() {
		if retErr != nil {
			tracer.Close()
		}
	}()

	ctx, cancel := context.WithTimeout(ctx, cfg.TotalTimeout)
	defer cancel()

	messages := make([]ChatMessage, len(cfg.Messages))
	copy(messages, cfg.Messages)

	for _, m := range messages {
		tracer.Log(TraceEntry{Iteration: 0, Role: m.Role, Content: m.Content})
	}

	var contextPulled []string
	agentsMDSeen := make(map[string]bool)
	var lastToolSig string
	stuckCount := 0
	lastTokens := 0
	nudged := false

	for iter := 1; iter <= cfg.MaxIter; iter++ {
		iterCtx, iterCancel := context.WithTimeout(ctx, cfg.PerIterTimeout)

		req := ChatRequest{
			Model:       cfg.ModelID,
			Messages:    messages,
			Tools:       cfg.Tools,
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
			if strings.TrimSpace(msg.Content) == "" && iter < cfg.MaxIter && !nudged {
				nudged = true
				tracer.Log(TraceEntry{Iteration: iter, Role: "system", Content: "empty final response, nudging model"})
				messages = append(messages, ChatMessage{Role: "assistant", Content: msg.Content})
				messages = append(messages, ChatMessage{Role: "user", Content: cfg.NudgeMessage})
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

		sig := ToolCallSignature(msg.ToolCalls)
		if sig == lastToolSig {
			stuckCount++
		} else {
			stuckCount = 0
		}
		lastToolSig = sig

		if stuckCount >= cfg.StuckThreshold {
			tracer.Log(TraceEntry{Iteration: iter, Role: "system", Content: "stuck detection triggered"})
			elapsed := time.Since(start).Seconds()
			return &LoopResult{ContextPulled: contextPulled, Iteration: iter, Truncated: true, Tracer: tracer, ElapsedSec: elapsed, TokensUsed: lastTokens}, nil
		}

		if cfg.TokenCeiling > 0 && resp.Usage.TotalTokens > cfg.TokenCeiling {
			tracer.Log(TraceEntry{Iteration: iter, Role: "system", Content: fmt.Sprintf("token ceiling reached: %d", resp.Usage.TotalTokens)})
			elapsed := time.Since(start).Seconds()
			return &LoopResult{ContextPulled: contextPulled, Iteration: iter, Truncated: true, Tracer: tracer, ElapsedSec: elapsed, TokensUsed: lastTokens}, nil
		}

		messages = append(messages, ChatMessage{Role: "assistant", Content: msg.Content, ToolCalls: msg.ToolCalls})

		for _, tc := range msg.ToolCalls {
			result := cfg.ToolDispatcher(cfg.Root, tc, &contextPulled, agentsMDSeen)
			tracer.Log(TraceEntry{Iteration: iter, Role: "tool", Content: result.Content, ToolResults: tc.ID})
			messages = append(messages, ChatMessage{
				Role:       "tool",
				Content:    result.Content,
				ToolCallID: tc.ID,
			})
		}

		messages = CompactMessages(messages, cfg.ContextSize)
	}

	elapsed := time.Since(start).Seconds()
	return &LoopResult{ContextPulled: contextPulled, Iteration: cfg.MaxIter, Truncated: true, Tracer: tracer, ElapsedSec: elapsed, TokensUsed: lastTokens}, nil
}

func EstimateTokens(messages []ChatMessage) int {
	total := 0
	for _, m := range messages {
		total += len(m.Content)
		for _, tc := range m.ToolCalls {
			total += len(tc.Function.Arguments)
		}
	}
	return total / 4
}

func CompactMessages(messages []ChatMessage, contextSize int) []ChatMessage {
	threshold := contextSize * 3 / 4
	if EstimateTokens(messages) < threshold {
		return messages
	}

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
