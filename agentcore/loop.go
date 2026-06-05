package agentcore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
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
	LLM            *LLMClient
	Temperature    float64
	MaxIter        int
	MaxTokens      int
	MaxForkDepth   int
	MaxForkIter    int
	AgentName      string
	TracerTag      string
	TraceMeta      map[string]string
	TraceDir       string
	OnTrace        func(TraceEntry)
	NudgeMessage   string
	StuckThreshold int
	PerIterTimeout time.Duration
	TotalTimeout   time.Duration
}

type LoopResult struct {
	FinalMessage     ChatMessage
	Messages         []ChatMessage
	ContextPulled    []string
	Iteration        int
	Truncated        bool
	Tracer           *Tracer
	ElapsedSec       float64
	TokensUsed       int
	GeneratedTokens  int
	PeakContextTokens int
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
	if llm == nil {
		llm = cfg.LLM
	}
	if cfg.LLM == nil {
		cfg.LLM = llm
	}
	cfg.defaults()

	start := time.Now()

	var (
		tracer *Tracer
		err    error
	)
	if cfg.TraceDir != "" {
		tracer, err = NewTracerInDir(cfg.TraceDir, cfg.TracerTag)
	} else {
		tracer, err = NewTracer(cfg.AgentName, cfg.TracerTag)
	}
	if err != nil {
		return nil, fmt.Errorf("init tracer: %w", err)
	}
	for k, v := range cfg.TraceMeta {
		tracer.SetMeta(k, v)
	}
	if cfg.OnTrace != nil {
		tracer.OnLog = cfg.OnTrace
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

	tools := cfg.Tools
	if cfg.MaxForkDepth > 0 {
		tools = append(append([]any{}, tools...), forkToolDef())
	}

	var contextPulled []string
	agentsMDSeen := make(map[string]bool)
	var lastToolSig string
	stuckCount := 0
	lastTokens := 0
	totalGenerated := 0
	peakContext := 0
	nudged := false

	for iter := 1; iter <= cfg.MaxIter; iter++ {
		iterCtx, iterCancel := context.WithTimeout(ctx, cfg.PerIterTimeout)

		req := ChatRequest{
			Model:       cfg.ModelID,
			Messages:    messages,
			Tools:       tools,
			Temperature: cfg.Temperature,
			MaxTokens:   cfg.MaxTokens,
		}

		iterStart := time.Now()
		resp, err := llm.Chat(iterCtx, req)
		if err != nil && ctx.Err() == nil {
			tracer.Log(TraceEntry{Iteration: iter, Role: "error", Content: fmt.Sprintf("retrying: %s", err)})
			time.Sleep(2 * time.Second)
			iterStart = time.Now()
			resp, err = llm.Chat(iterCtx, req)
		}
		iterCancel()
		iterLatency := time.Since(iterStart).Milliseconds()

		if err != nil {
			tracer.Log(TraceEntry{Iteration: iter, Role: "error", Content: err.Error()})
			if ctx.Err() != nil {
				elapsed := time.Since(start).Seconds()
				return &LoopResult{ContextPulled: contextPulled, Iteration: iter, Truncated: true, Tracer: tracer, ElapsedSec: elapsed, TokensUsed: lastTokens, GeneratedTokens: totalGenerated, PeakContextTokens: peakContext}, nil
			}
			return nil, fmt.Errorf("iteration %d: %w", iter, err)
		}

		if len(resp.Choices) == 0 {
			return nil, fmt.Errorf("iteration %d: empty response from LLM", iter)
		}

		lastTokens = resp.Usage.TotalTokens
		totalGenerated += resp.Usage.CompletionTokens
		if resp.Usage.PromptTokens > peakContext {
			peakContext = resp.Usage.PromptTokens
		}
		msg := resp.Choices[0].Message
		tracer.Log(TraceEntry{
			Iteration:        iter,
			Role:             "assistant",
			Content:          msg.Content,
			ToolCalls:        msg.ToolCalls,
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
			LatencyMs:        iterLatency,
		})

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
				FinalMessage:      msg,
				Messages:          messages,
				ContextPulled:     contextPulled,
				Iteration:         iter,
				Truncated:         false,
				Tracer:            tracer,
				ElapsedSec:        elapsed,
				TokensUsed:        lastTokens,
				GeneratedTokens:   totalGenerated,
				PeakContextTokens: peakContext,
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
			return &LoopResult{ContextPulled: contextPulled, Iteration: iter, Truncated: true, Tracer: tracer, ElapsedSec: elapsed, TokensUsed: lastTokens, GeneratedTokens: totalGenerated, PeakContextTokens: peakContext}, nil
		}

		if cfg.TokenCeiling > 0 && resp.Usage.TotalTokens > cfg.TokenCeiling {
			tracer.Log(TraceEntry{Iteration: iter, Role: "system", Content: fmt.Sprintf("token ceiling reached: %d", resp.Usage.TotalTokens)})
			elapsed := time.Since(start).Seconds()
			return &LoopResult{ContextPulled: contextPulled, Iteration: iter, Truncated: true, Tracer: tracer, ElapsedSec: elapsed, TokensUsed: lastTokens, GeneratedTokens: totalGenerated, PeakContextTokens: peakContext}, nil
		}

		messages = append(messages, ChatMessage{Role: "assistant", Content: msg.Content, ToolCalls: msg.ToolCalls})

		for _, tc := range msg.ToolCalls {
			var result ToolResult
			if tc.Function.Name == "fork" && cfg.MaxForkDepth > 0 {
				result = execFork(ctx, llm, cfg, messages, tc, agentsMDSeen)
			} else {
				result = cfg.ToolDispatcher(cfg.Root, tc, &contextPulled, agentsMDSeen)
			}
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
	return &LoopResult{ContextPulled: contextPulled, Iteration: cfg.MaxIter, Truncated: true, Tracer: tracer, ElapsedSec: elapsed, TokensUsed: lastTokens, GeneratedTokens: totalGenerated, PeakContextTokens: peakContext}, nil
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

func forkToolDef() any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        "fork",
			"description": "Fork the current context into multiple parallel sub-tasks. All sub-tasks inherit your current conversation history and run concurrently. Use this when you have gathered enough context and want to analyze it from multiple angles simultaneously. Provide ALL sub-tasks in a single call.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"tasks": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"id":     map[string]any{"type": "string", "description": "Short identifier for this sub-task (e.g. 'security', 'perf')"},
								"prompt": map[string]any{"type": "string", "description": "What this sub-task should focus on and produce"},
							},
							"required": []string{"id", "prompt"},
						},
						"minItems": 2,
						"description": "Sub-tasks to run in parallel. Each inherits the full conversation context.",
					},
				},
				"required": []string{"tasks"},
			},
		},
	}
}

type forkTask struct {
	ID     string `json:"id"`
	Prompt string `json:"prompt"`
}

func execFork(ctx context.Context, llm *LLMClient, parentCfg LoopConfig, currentMessages []ChatMessage, tc LLMTool, agentsMDSeen map[string]bool) ToolResult {
	var args struct {
		Tasks []forkTask `json:"tasks"`
	}
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		return ToolResult{Content: fmt.Sprintf("invalid fork arguments: %s", err), IsError: true}
	}
	if len(args.Tasks) < 2 {
		return ToolResult{Content: "fork requires at least 2 tasks", IsError: true}
	}

	maxIter := parentCfg.MaxForkIter
	if maxIter <= 0 {
		maxIter = parentCfg.MaxIter
	}

	type forkResult struct {
		id      string
		content string
		err     error
	}

	results := make([]forkResult, len(args.Tasks))
	var wg sync.WaitGroup

	for i, task := range args.Tasks {
		wg.Add(1)
		go func(idx int, t forkTask) {
			defer wg.Done()

			childMessages := cloneMessages(currentMessages)
			childMessages = append(childMessages, ChatMessage{Role: "user", Content: t.Prompt})

			childSeen := make(map[string]bool, len(agentsMDSeen))
			for k, v := range agentsMDSeen {
				childSeen[k] = v
			}

			childCfg := LoopConfig{
				ModelID:        parentCfg.ModelID,
				ContextSize:    parentCfg.ContextSize,
				TokenCeiling:   parentCfg.TokenCeiling,
				Root:           parentCfg.Root,
				Messages:       childMessages,
				Tools:          parentCfg.Tools,
				ToolDispatcher: parentCfg.ToolDispatcher,
				LLM:            llm,
				Temperature:    parentCfg.Temperature,
				MaxIter:        maxIter,
				MaxTokens:      parentCfg.MaxTokens,
				MaxForkDepth:   parentCfg.MaxForkDepth - 1,
				MaxForkIter:    parentCfg.MaxForkIter,
				AgentName:      parentCfg.AgentName,
				TracerTag:      parentCfg.TracerTag + "-fork-" + t.ID,
				TraceMeta:      parentCfg.TraceMeta,
				NudgeMessage:   parentCfg.NudgeMessage,
				StuckThreshold: parentCfg.StuckThreshold,
				PerIterTimeout: parentCfg.PerIterTimeout,
				TotalTimeout:   parentCfg.TotalTimeout,
			}

			lr, err := RunLoop(ctx, llm, childCfg)
			if err != nil {
				results[idx] = forkResult{id: t.ID, err: err}
				return
			}
			defer lr.Tracer.Close()
			results[idx] = forkResult{id: t.ID, content: lr.FinalMessage.Content}
		}(i, task)
	}

	wg.Wait()

	var b strings.Builder
	for _, r := range results {
		fmt.Fprintf(&b, "=== fork: %s ===\n", r.id)
		if r.err != nil {
			fmt.Fprintf(&b, "error: %v\n", r.err)
		} else {
			b.WriteString(r.content)
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}
	return ToolResult{Content: b.String()}
}

func cloneMessages(msgs []ChatMessage) []ChatMessage {
	out := make([]ChatMessage, len(msgs))
	for i, m := range msgs {
		out[i] = ChatMessage{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		if len(m.ToolCalls) > 0 {
			out[i].ToolCalls = make([]LLMTool, len(m.ToolCalls))
			copy(out[i].ToolCalls, m.ToolCalls)
		}
	}
	return out
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
