package main

import (
	"testing"
	"time"
)

func TestParseTraceFilename(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantTag string
		wantTS  bool
	}{
		{"mr review", "20260523-233210-mr-review.jsonl", "mr-review", true},
		{"fork", "20260523-233538-mr-review-fork-bugs.jsonl", "mr-review-fork-bugs", true},
		{"commit review", "20260524-120000-commit-review.jsonl", "commit-review", true},
		{"short name", "short.jsonl", "short", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, tag := parseTraceFilename(tt.input)
			if tag != tt.wantTag {
				t.Errorf("tag = %q, want %q", tag, tt.wantTag)
			}
			if tt.wantTS && ts.IsZero() {
				t.Error("expected non-zero timestamp")
			}
			if !tt.wantTS && !ts.IsZero() {
				t.Error("expected zero timestamp")
			}
		})
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{500, "500B"},
		{1024, "1.0K"},
		{80700, "78.8K"},
		{1048576, "1.0M"},
		{1572864, "1.5M"},
	}
	for _, tt := range tests {
		got := formatSize(tt.bytes)
		if got != tt.want {
			t.Errorf("formatSize(%d) = %q, want %q", tt.bytes, got, tt.want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{500 * time.Millisecond, "500ms"},
		{5 * time.Second, "5.0s"},
		{90 * time.Second, "1.5m"},
	}
	for _, tt := range tests {
		got := formatDuration(tt.d)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}
