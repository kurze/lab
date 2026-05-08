package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type TraceEntry struct {
	Timestamp   time.Time `json:"ts"`
	Iteration   int       `json:"iteration"`
	Role        string    `json:"role"`
	Content     string    `json:"content,omitempty"`
	ToolCalls   any       `json:"tool_calls,omitempty"`
	ToolResults any       `json:"tool_results,omitempty"`
}

type Tracer struct {
	f *os.File
}

func newTracer(artifactPath string) (*Tracer, error) {
	stateDir := os.Getenv("XDG_STATE_HOME")
	if stateDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		stateDir = filepath.Join(home, ".local", "state")
	}

	dir := filepath.Join(stateDir, "wlx-review-agent", "traces")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create trace dir: %w", err)
	}

	base := strings.TrimSuffix(filepath.Base(artifactPath), filepath.Ext(artifactPath))
	name := fmt.Sprintf("%s-%s.jsonl", time.Now().Format("20060102-150405"), base)
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return nil, fmt.Errorf("create trace file: %w", err)
	}

	return &Tracer{f: f}, nil
}

func (t *Tracer) Log(entry TraceEntry) {
	entry.Timestamp = time.Now()
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	t.f.Write(data)
	t.f.Write([]byte("\n"))
}

func (t *Tracer) Close() {
	if t.f != nil {
		t.f.Close()
	}
}
