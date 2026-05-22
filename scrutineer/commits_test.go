package main

import (
	"testing"
)

func TestMergeCommitResults_Empty(t *testing.T) {
	result := mergeCommitResults(nil)
	if len(result.Findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(result.Findings))
	}
}

func TestMergeCommitResults_NilResult(t *testing.T) {
	crs := []CommitReviewResult{
		{Commit: Commit{SHA: "abc12345"}, Result: nil},
	}
	result := mergeCommitResults(crs)
	if len(result.Findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(result.Findings))
	}
}

func TestMergeCommitResults(t *testing.T) {
	crs := []CommitReviewResult{
		{
			Commit: Commit{SHA: "aaa1111100000000"},
			Result: &ReviewResult{
				Model:    "test-model",
				Findings: []Finding{{Severity: "major", Location: "file.go:1", Description: "bug1"}},
			},
		},
		{
			Commit: Commit{SHA: "bbb2222200000000"},
			Result: &ReviewResult{
				Findings: []Finding{
					{Severity: "minor", Location: "other.go:5", Description: "style1"},
					{Severity: "info", Location: "other.go:10", Description: "note1"},
				},
			},
		},
	}

	result := mergeCommitResults(crs)
	if result.Model != "test-model" {
		t.Errorf("expected model 'test-model', got %q", result.Model)
	}
	if len(result.Findings) != 3 {
		t.Fatalf("expected 3 findings, got %d", len(result.Findings))
	}
	if result.Findings[0].Location != "[aaa11111] file.go:1" {
		t.Errorf("unexpected location prefix: %s", result.Findings[0].Location)
	}
	if result.Findings[1].Location != "[bbb22222] other.go:5" {
		t.Errorf("unexpected location prefix: %s", result.Findings[1].Location)
	}
}
