package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestStateStoreAndGetResult(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	s, err := LoadState(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	sr := &StoredResult{
		Key:        ResultKeyMR(42),
		Title:      "Fix bug",
		Mode:       "full",
		Findings:   []Finding{{Category: "security", Severity: "major", Description: "SQL injection"}},
		Model:      "test-model",
		ReviewedAt: time.Now(),
	}
	s.StoreResult("owner/repo", sr)

	got := s.GetResult("owner/repo", ResultKeyMR(42))
	if got == nil {
		t.Fatal("expected stored result")
	}
	if got.Title != "Fix bug" {
		t.Errorf("title = %q, want %q", got.Title, "Fix bug")
	}
	if len(got.Findings) != 1 {
		t.Errorf("findings = %d, want 1", len(got.Findings))
	}

	if s.GetResult("owner/repo", ResultKeyMR(99)) != nil {
		t.Error("expected nil for missing result")
	}
	if s.GetResult("other/repo", ResultKeyMR(42)) != nil {
		t.Error("expected nil for wrong project")
	}

	if err := s.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	s2, err := LoadState(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	got2 := s2.GetResult("owner/repo", ResultKeyMR(42))
	if got2 == nil {
		t.Fatal("expected result after reload")
	}
	if len(got2.Findings) != 1 {
		t.Errorf("findings after reload = %d, want 1", len(got2.Findings))
	}
}

func TestStateListResults(t *testing.T) {
	s, err := LoadState(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	results := s.ListResults("owner/repo")
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}

	s.StoreResult("owner/repo", &StoredResult{Key: ResultKeyMR(1), Title: "MR 1", ReviewedAt: time.Now()})
	s.StoreResult("owner/repo", &StoredResult{Key: ResultKeyBranch("feat"), Title: "feat branch", ReviewedAt: time.Now()})
	s.StoreResult("other/repo", &StoredResult{Key: ResultKeyMR(1), Title: "Other MR", ReviewedAt: time.Now()})

	results = s.ListResults("owner/repo")
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestResultKeys(t *testing.T) {
	if got := ResultKeyMR(42); got != "mr:42" {
		t.Errorf("ResultKeyMR(42) = %q, want %q", got, "mr:42")
	}
	if got := ResultKeyBranch("feat/x"); got != "branch:feat/x" {
		t.Errorf("ResultKeyBranch = %q, want %q", got, "branch:feat/x")
	}
	if got := ResultKeyCommit("abc123"); got != "commit:abc123" {
		t.Errorf("ResultKeyCommit = %q, want %q", got, "commit:abc123")
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

func TestLoadStorage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := LoadStorage(path)
	if err != nil {
		t.Fatalf("LoadStorage: %v", err)
	}

	store.MarkReviewed("owner/repo", 42)
	if !store.IsReviewed("owner/repo", 42) {
		t.Fatal("expected reviewed MR to be stored")
	}
}

func TestStoredResultsAreDefensivelyCopied(t *testing.T) {
	s, err := LoadState(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	original := &StoredResult{
		Key:        ResultKeyMR(42),
		Title:      "Original title",
		Mode:       "full",
		Findings:   []Finding{{Category: "security", Severity: "major", Description: "A"}},
		ReviewedAt: time.Now(),
	}
	s.StoreResult("owner/repo", original)

	original.Title = "Mutated by caller"
	original.Findings[0].Description = "Mutated finding"

	got := s.GetResult("owner/repo", ResultKeyMR(42))
	if got == nil {
		t.Fatal("expected stored result")
	}
	if got.Title != "Original title" {
		t.Fatalf("title mutated through caller reference: got %q", got.Title)
	}
	if got.Findings[0].Description != "A" {
		t.Fatalf("findings mutated through caller reference: got %q", got.Findings[0].Description)
	}

	got.Title = "Mutated from getter"
	got.Findings[0].Description = "Mutated from getter"

	again := s.GetResult("owner/repo", ResultKeyMR(42))
	if again == nil {
		t.Fatal("expected stored result on second read")
	}
	if again.Title != "Original title" {
		t.Fatalf("title mutated through getter result: got %q", again.Title)
	}
	if again.Findings[0].Description != "A" {
		t.Fatalf("finding mutated through getter result: got %q", again.Findings[0].Description)
	}

	listed := s.ListResults("owner/repo")
	if len(listed) != 1 {
		t.Fatalf("expected one listed result, got %d", len(listed))
	}
	listed[0].Title = "Mutated from list"

	final := s.GetResult("owner/repo", ResultKeyMR(42))
	if final == nil {
		t.Fatal("expected stored result after list mutation")
	}
	if final.Title != "Original title" {
		t.Fatalf("title mutated through list result: got %q", final.Title)
	}
}
