package agentcore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type TraceEntry struct {
	Timestamp        time.Time `json:"ts"`
	Iteration        int       `json:"iteration"`
	Role             string    `json:"role"`
	Content          string    `json:"content,omitempty"`
	ToolCalls        any       `json:"tool_calls,omitempty"`
	ToolResults      any       `json:"tool_results,omitempty"`
	PromptTokens     int       `json:"prompt_tokens,omitempty"`
	CompletionTokens int       `json:"completion_tokens,omitempty"`
	TotalTokens      int       `json:"total_tokens,omitempty"`
	LatencyMs        int64     `json:"latency_ms,omitempty"`
}

type Tracer struct {
	f        *os.File
	meta     map[string]string
	path     string
	metaSent bool
	OnLog    func(TraceEntry)
}

func NewTracer(agentName, tag string) (*Tracer, error) {
	stateDir := os.Getenv("XDG_STATE_HOME")
	if stateDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		stateDir = filepath.Join(home, ".local", "state")
	}

	dir := filepath.Join(stateDir, agentName, "traces")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create trace dir: %w", err)
	}

	base := strings.TrimSuffix(filepath.Base(tag), filepath.Ext(tag))
	name := fmt.Sprintf("%s-%s.jsonl", time.Now().Format("20060102-150405"), base)
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return nil, fmt.Errorf("create trace file: %w", err)
	}

	return &Tracer{f: f, path: filepath.Join(dir, name)}, nil
}

func NewTracerInDir(dir, tag string) (*Tracer, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create trace dir: %w", err)
	}

	base := strings.TrimSuffix(filepath.Base(tag), filepath.Ext(tag))
	name := fmt.Sprintf("%s-%s.jsonl", time.Now().Format("20060102-150405"), base)
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return nil, fmt.Errorf("create trace file: %w", err)
	}

	return &Tracer{f: f, path: filepath.Join(dir, name)}, nil
}

func (t *Tracer) SetMeta(key, value string) {
	if t.meta == nil {
		t.meta = make(map[string]string)
	}
	t.meta[key] = value
}

func (t *Tracer) Path() string { return t.path }

func TracesDir(agentName string) string {
	stateDir := os.Getenv("XDG_STATE_HOME")
	if stateDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		stateDir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(stateDir, agentName, "traces")
}

type traceEntryWithMeta struct {
	TraceEntry
	Meta map[string]string `json:"meta,omitempty"`
}

func (t *Tracer) Log(entry TraceEntry) {
	entry.Timestamp = time.Now()
	em := traceEntryWithMeta{TraceEntry: entry}
	if !t.metaSent && entry.Role == "system" && len(t.meta) > 0 {
		em.Meta = t.meta
		t.metaSent = true
	}
	data, err := json.Marshal(em)
	if err != nil {
		return
	}
	_, _ = t.f.Write(data)
	_, _ = t.f.Write([]byte("\n"))
	_ = t.f.Sync()
	if t.OnLog != nil {
		t.OnLog(entry)
	}
}

func (t *Tracer) Close() {
	if t.f != nil {
		t.f.Close()
	}
}
