package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseTraceFilename(t *testing.T) {
	tests := []struct {
		name    string
		wantTag string
		wantOK  bool
	}{
		{"20260524-113348-commit-review", "commit-review", true},
		{"20260524-004019-mr-repass", "mr-repass", true},
		{"20260524-111904-mr-repass-fork-bugs", "mr-repass-fork-bugs", true},
		{"short", "short", false},
	}

	for _, tt := range tests {
		ts, tag := parseTraceFilename(tt.name)
		if tt.wantOK && ts.IsZero() {
			t.Errorf("parseTraceFilename(%q): got zero time, want non-zero", tt.name)
		}
		if !tt.wantOK && !ts.IsZero() {
			t.Errorf("parseTraceFilename(%q): got non-zero time, want zero", tt.name)
		}
		if tag != tt.wantTag {
			t.Errorf("parseTraceFilename(%q): tag = %q, want %q", tt.name, tag, tt.wantTag)
		}
	}
}

func TestFormatTarget(t *testing.T) {
	tests := []struct {
		meta map[string]string
		want string
	}{
		{map[string]string{"mr_id": "42"}, "MR#42"},
		{map[string]string{"branch": "feat/x"}, "branch:feat/x"},
		{map[string]string{"commit": "abc1234567890"}, "commit:abc12345"},
		{map[string]string{}, ""},
		{nil, ""},
	}

	for _, tt := range tests {
		got := formatTarget(tt.meta)
		if got != tt.want {
			t.Errorf("formatTarget(%v) = %q, want %q", tt.meta, got, tt.want)
		}
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{500, "500B"},
		{1024, "1K"},
		{51200, "50K"},
		{1048576, "1.0M"},
		{2621440, "2.5M"},
	}

	for _, tt := range tests {
		got := formatSize(tt.bytes)
		if got != tt.want {
			t.Errorf("formatSize(%d) = %q, want %q", tt.bytes, got, tt.want)
		}
	}
}

func writeTraceFile(t *testing.T, dir, name string, meta map[string]string, entries int) string {
	t.Helper()
	path := filepath.Join(dir, name+".jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create trace file: %v", err)
	}
	defer f.Close()

	type entryWithMeta struct {
		Timestamp time.Time         `json:"ts"`
		Iteration int               `json:"iteration"`
		Role      string            `json:"role"`
		Content   string            `json:"content"`
		Meta      map[string]string `json:"meta,omitempty"`
	}

	for i := 0; i < entries; i++ {
		e := entryWithMeta{
			Timestamp: time.Now(),
			Iteration: i,
			Role:      "system",
			Content:   "test content",
		}
		if i == 0 && meta != nil {
			e.Meta = meta
		}
		data, _ := json.Marshal(e)
		f.Write(data)
		f.Write([]byte("\n"))
	}
	return path
}

func TestReadTraceMeta(t *testing.T) {
	dir := t.TempDir()
	writeTraceFile(t, dir, "20260524-120000-test", map[string]string{
		"mr_id":   "7",
		"project": "owner/repo",
		"mode":    "both",
	}, 3)

	meta := readTraceMeta(filepath.Join(dir, "20260524-120000-test.jsonl"))
	if meta == nil {
		t.Fatal("expected meta, got nil")
	}
	if meta["mr_id"] != "7" {
		t.Errorf("mr_id = %q, want %q", meta["mr_id"], "7")
	}
	if meta["project"] != "owner/repo" {
		t.Errorf("project = %q, want %q", meta["project"], "owner/repo")
	}
}

func TestReadTraceMeta_NoMeta(t *testing.T) {
	dir := t.TempDir()
	writeTraceFile(t, dir, "20260524-120000-nometa", nil, 1)

	meta := readTraceMeta(filepath.Join(dir, "20260524-120000-nometa.jsonl"))
	if len(meta) != 0 {
		t.Errorf("expected empty meta, got %v", meta)
	}
}

func TestFindTraceFile(t *testing.T) {
	dir := t.TempDir()
	writeTraceFile(t, dir, "20260524-120000-commit-review", nil, 1)
	writeTraceFile(t, dir, "20260524-130000-mr-repass", nil, 1)

	path := findTraceFile(dir, "20260524-120000-commit-review")
	if path == "" {
		t.Fatal("expected to find trace file")
	}

	path = findTraceFile(dir, "20260524-12")
	if path == "" {
		t.Fatal("expected prefix match to find trace file")
	}

	path = findTraceFile(dir, "nonexistent")
	if path != "" {
		t.Fatalf("expected empty path for nonexistent, got %q", path)
	}
}

func TestMostRecentTrace(t *testing.T) {
	dir := t.TempDir()
	writeTraceFile(t, dir, "20260524-120000-old", nil, 1)
	time.Sleep(10 * time.Millisecond)
	writeTraceFile(t, dir, "20260524-130000-new", nil, 1)

	path := mostRecentTrace(dir)
	if filepath.Base(path) != "20260524-130000-new.jsonl" {
		t.Errorf("most recent = %q, want 20260524-130000-new.jsonl", filepath.Base(path))
	}
}

func TestMostRecentTrace_Empty(t *testing.T) {
	dir := t.TempDir()
	if path := mostRecentTrace(dir); path != "" {
		t.Errorf("expected empty, got %q", path)
	}
}

func TestPruneTraces_ByAge(t *testing.T) {
	dir := t.TempDir()

	oldPath := writeTraceFile(t, dir, "20200101-000000-old", nil, 1)
	os.Chtimes(oldPath, time.Now().AddDate(0, 0, -60), time.Now().AddDate(0, 0, -60))
	writeTraceFile(t, dir, "20260524-120000-recent", nil, 1)

	deleted, freed := pruneTraces(dir, 30, 0)
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}
	if freed <= 0 {
		t.Error("expected freed > 0")
	}

	entries, _ := os.ReadDir(dir)
	jsonl := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".jsonl" {
			jsonl++
		}
	}
	if jsonl != 1 {
		t.Errorf("remaining files = %d, want 1", jsonl)
	}
}

func TestPruneTraces_BySize(t *testing.T) {
	dir := t.TempDir()

	for i := 0; i < 5; i++ {
		name := filepath.Join(dir, time.Now().Add(time.Duration(i)*time.Second).Format("20060102-150405")+"-test.jsonl")
		os.WriteFile(name, make([]byte, 1024), 0o644)
		time.Sleep(10 * time.Millisecond)
	}

	// 5 files × 1KB = 5KB, prune to 3KB max
	deleted, _ := pruneTraces(dir, 365, 0)
	if deleted != 0 {
		t.Errorf("age prune should not delete recent files, deleted %d", deleted)
	}

	deleted, _ = pruneTraces(dir, 365, 1) // 1MB, should keep all
	if deleted != 0 {
		t.Errorf("size prune with high limit should not delete, deleted %d", deleted)
	}
}

func TestPruneTraces_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	deleted, freed := pruneTraces(dir, 30, 500)
	if deleted != 0 || freed != 0 {
		t.Errorf("empty dir: deleted=%d freed=%d, want 0/0", deleted, freed)
	}
}

func TestPruneTraces_NoDir(t *testing.T) {
	deleted, freed := pruneTraces("/nonexistent/path", 30, 500)
	if deleted != 0 || freed != 0 {
		t.Errorf("nonexistent dir: deleted=%d freed=%d, want 0/0", deleted, freed)
	}
}

func TestCloneMap(t *testing.T) {
	orig := map[string]string{"a": "1", "b": "2"}
	clone := cloneMap(orig)

	if len(clone) != 2 || clone["a"] != "1" || clone["b"] != "2" {
		t.Errorf("clone mismatch: %v", clone)
	}

	clone["c"] = "3"
	if _, ok := orig["c"]; ok {
		t.Error("mutation of clone affected original")
	}
}

func TestCloneMap_Nil(t *testing.T) {
	clone := cloneMap(nil)
	if clone == nil || len(clone) != 0 {
		t.Errorf("cloneMap(nil) should return empty non-nil map, got %v", clone)
	}
}
