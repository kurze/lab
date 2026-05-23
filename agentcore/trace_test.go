package agentcore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTracerWritesJSONL(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)

	tr, err := NewTracer("test-agent", "test-tag")
	if err != nil {
		t.Fatal(err)
	}

	tr.SetMeta("mr_id", "42")
	tr.SetMeta("project", "owner/repo")

	tr.Log(TraceEntry{Iteration: 0, Role: "system", Content: "hello"})
	tr.Log(TraceEntry{
		Iteration:        1,
		Role:             "assistant",
		Content:          "response",
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
		LatencyMs:        1234,
	})
	tr.Close()

	data, err := os.ReadFile(tr.Path())
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	var first traceEntryWithMeta
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatal(err)
	}
	if first.Role != "system" {
		t.Errorf("first entry role = %q, want system", first.Role)
	}
	if first.Meta["mr_id"] != "42" {
		t.Errorf("meta mr_id = %q, want 42", first.Meta["mr_id"])
	}
	if first.Meta["project"] != "owner/repo" {
		t.Errorf("meta project = %q, want owner/repo", first.Meta["project"])
	}

	var second TraceEntry
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatal(err)
	}
	if second.PromptTokens != 100 {
		t.Errorf("prompt_tokens = %d, want 100", second.PromptTokens)
	}
	if second.CompletionTokens != 50 {
		t.Errorf("completion_tokens = %d, want 50", second.CompletionTokens)
	}
	if second.TotalTokens != 150 {
		t.Errorf("total_tokens = %d, want 150", second.TotalTokens)
	}
	if second.LatencyMs != 1234 {
		t.Errorf("latency_ms = %d, want 1234", second.LatencyMs)
	}
}

func TestTracerMetaOnlyOnFirstSystemEntry(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)

	tr, err := NewTracer("test-agent", "meta-test")
	if err != nil {
		t.Fatal(err)
	}

	tr.SetMeta("key", "val")

	tr.Log(TraceEntry{Iteration: 0, Role: "system", Content: "sys"})
	tr.Log(TraceEntry{Iteration: 1, Role: "system", Content: "nudge"})
	tr.Close()

	data, err := os.ReadFile(tr.Path())
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")

	var first, second traceEntryWithMeta
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatal(err)
	}

	if first.Meta == nil || first.Meta["key"] != "val" {
		t.Error("first system entry should have meta")
	}
	if second.Meta != nil {
		t.Error("second system entry should not have meta")
	}
}

func TestTracesDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)

	got := TracesDir("my-agent")
	want := filepath.Join(dir, "my-agent", "traces")
	if got != want {
		t.Errorf("TracesDir = %q, want %q", got, want)
	}
}
