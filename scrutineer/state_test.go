package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	s, err := LoadState(path)
	if err != nil {
		t.Fatalf("load empty state: %v", err)
	}

	s.MarkReviewed("owner/repo", 42)
	s.MarkCommitReviewed("owner/repo", "abc123")

	if err := s.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	s2, err := LoadState(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}

	if !s2.IsReviewed("owner/repo", 42) {
		t.Error("expected MR 42 to be reviewed")
	}
	if s2.IsReviewed("owner/repo", 99) {
		t.Error("expected MR 99 to not be reviewed")
	}
	if !s2.IsCommitReviewed("owner/repo", "abc123") {
		t.Error("expected commit abc123 to be reviewed")
	}
	if s2.IsCommitReviewed("owner/repo", "def456") {
		t.Error("expected commit def456 to not be reviewed")
	}
}

func TestStateLoadNonExistent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.json")
	s, err := LoadState(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.IsReviewed("x", 1) {
		t.Error("fresh state should have no reviews")
	}
}

func TestStateLoadCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("{invalid"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadState(path)
	if err == nil {
		t.Error("expected error on corrupt state file")
	}
}
