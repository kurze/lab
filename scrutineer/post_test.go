package main

import (
	"context"
	"fmt"
	"testing"
)

type fakeForge struct {
	commitComments []commitComment
	inlineComments []InlineComment
	summaryBodies  []string
	getPR          PullRequest
	getErr         error
}

type commitComment struct {
	SHA  string
	File string
	Line int
	Body string
}

func (f *fakeForge) Name() string { return "fake" }
func (f *fakeForge) ListAll(_ context.Context) ([]PullRequest, error) {
	return nil, nil
}
func (f *fakeForge) Get(_ context.Context, _ int64) (PullRequest, error) {
	return f.getPR, f.getErr
}
func (f *fakeForge) GetDiff(_ context.Context, _ int64) (string, error) { return "", nil }
func (f *fakeForge) ListCommits(_ context.Context, _ int64) ([]Commit, error) {
	return nil, nil
}
func (f *fakeForge) PostComment(_ context.Context, _ int64, body string) error {
	f.summaryBodies = append(f.summaryBodies, body)
	return nil
}
func (f *fakeForge) PostInlineComments(_ context.Context, _ PullRequest, comments []InlineComment) error {
	f.inlineComments = append(f.inlineComments, comments...)
	return nil
}
func (f *fakeForge) PostCommitComment(_ context.Context, sha string, file string, line int, body string) error {
	f.commitComments = append(f.commitComments, commitComment{sha, file, line, body})
	return nil
}

func TestPostCommitResult(t *testing.T) {
	ctx := context.Background()
	sha := "abc123def456"

	t.Run("posts inline findings", func(t *testing.T) {
		forge := &fakeForge{}
		sr := &StoredResult{
			Key: ResultKeyCommit(sha),
			Findings: []Finding{
				{Severity: "major", Location: "main.go:10", Category: "bug", Description: "null deref"},
				{Severity: "critical", Location: "util.go:5", Category: "security", Description: "injection"},
			},
		}
		postCommitResult(ctx, forge, sr, sr.Key, Config{})

		if len(forge.commitComments) != 2 {
			t.Fatalf("expected 2 commit comments, got %d", len(forge.commitComments))
		}
		for _, cc := range forge.commitComments {
			if cc.SHA != sha {
				t.Errorf("expected SHA %s, got %s", sha, cc.SHA)
			}
		}
		if forge.commitComments[0].File != "main.go" || forge.commitComments[0].Line != 10 {
			t.Errorf("unexpected first comment: %+v", forge.commitComments[0])
		}
		if forge.commitComments[1].File != "util.go" || forge.commitComments[1].Line != 5 {
			t.Errorf("unexpected second comment: %+v", forge.commitComments[1])
		}
	})

	t.Run("skips below severity threshold", func(t *testing.T) {
		forge := &fakeForge{}
		sr := &StoredResult{
			Key: ResultKeyCommit(sha),
			Findings: []Finding{
				{Severity: "info", Location: "main.go:10", Category: "note", Description: "fyi"},
			},
		}
		postCommitResult(ctx, forge, sr, sr.Key, Config{InlineSeverity: "major"})

		if len(forge.commitComments) != 0 {
			t.Errorf("expected 0 commit comments for low-severity finding, got %d", len(forge.commitComments))
		}
	})

	t.Run("skips findings without location", func(t *testing.T) {
		forge := &fakeForge{}
		sr := &StoredResult{
			Key: ResultKeyCommit(sha),
			Findings: []Finding{
				{Severity: "major", Location: "", Category: "bug", Description: "no location"},
				{Severity: "major", Location: "valid.go:1", Category: "bug", Description: "has location"},
			},
		}
		postCommitResult(ctx, forge, sr, sr.Key, Config{})

		if len(forge.commitComments) != 1 {
			t.Fatalf("expected 1 commit comment, got %d", len(forge.commitComments))
		}
	})

	t.Run("no findings is a no-op", func(t *testing.T) {
		forge := &fakeForge{}
		sr := &StoredResult{Key: ResultKeyCommit(sha)}
		postCommitResult(ctx, forge, sr, sr.Key, Config{})

		if len(forge.commitComments) != 0 {
			t.Errorf("expected 0 commit comments, got %d", len(forge.commitComments))
		}
	})
}

func TestPostFindingsToCommits(t *testing.T) {
	ctx := context.Background()
	commits := []Commit{
		{SHA: "aaaa1111bbbb2222"},
		{SHA: "cccc3333dddd4444"},
	}

	t.Run("posts matching findings", func(t *testing.T) {
		forge := &fakeForge{}
		findings := []Finding{
			{Severity: "major", Location: "main.go:10", CommitSHA: "aaaa1111bbbb2222", Category: "bug", Description: "null deref"},
			{Severity: "critical", Location: "util.go:5", CommitSHA: "cccc3333dddd4444", Category: "security", Description: "injection"},
		}
		posted := postFindingsToCommits(ctx, forge, findings, commits, "test-branch", Config{})
		if posted != 2 {
			t.Fatalf("expected 2 posted, got %d", posted)
		}
		if len(forge.commitComments) != 2 {
			t.Fatalf("expected 2 commit comments, got %d", len(forge.commitComments))
		}
		if forge.commitComments[0].SHA != "aaaa1111bbbb2222" {
			t.Errorf("expected full SHA, got %s", forge.commitComments[0].SHA)
		}
	})

	t.Run("resolves short SHA to full SHA", func(t *testing.T) {
		forge := &fakeForge{}
		findings := []Finding{
			{Severity: "major", Location: "main.go:10", CommitSHA: "aaaa1111", Category: "bug", Description: "short sha"},
		}
		posted := postFindingsToCommits(ctx, forge, findings, commits, "test-branch", Config{})
		if posted != 1 {
			t.Fatalf("expected 1 posted, got %d", posted)
		}
		if forge.commitComments[0].SHA != "aaaa1111bbbb2222" {
			t.Errorf("expected full SHA aaaa1111bbbb2222, got %s", forge.commitComments[0].SHA)
		}
	})

	t.Run("skips below severity threshold", func(t *testing.T) {
		forge := &fakeForge{}
		findings := []Finding{
			{Severity: "info", Location: "main.go:10", CommitSHA: "aaaa1111bbbb2222", Category: "note", Description: "fyi"},
			{Severity: "minor", Location: "main.go:20", CommitSHA: "aaaa1111bbbb2222", Category: "style", Description: "naming"},
			{Severity: "major", Location: "main.go:30", CommitSHA: "aaaa1111bbbb2222", Category: "bug", Description: "real bug"},
		}
		posted := postFindingsToCommits(ctx, forge, findings, commits, "test-branch", Config{InlineSeverity: "major"})
		if posted != 1 {
			t.Fatalf("expected 1 posted (only major+), got %d", posted)
		}
	})

	t.Run("skips unknown commit SHA", func(t *testing.T) {
		forge := &fakeForge{}
		findings := []Finding{
			{Severity: "major", Location: "main.go:10", CommitSHA: "deadbeef12345678", Category: "bug", Description: "unknown sha"},
		}
		posted := postFindingsToCommits(ctx, forge, findings, commits, "test-branch", Config{})
		if posted != 0 {
			t.Fatalf("expected 0 posted for unknown SHA, got %d", posted)
		}
	})

	t.Run("skips findings without commit SHA", func(t *testing.T) {
		forge := &fakeForge{}
		findings := []Finding{
			{Severity: "major", Location: "main.go:10", CommitSHA: "", Category: "bug", Description: "no sha"},
		}
		posted := postFindingsToCommits(ctx, forge, findings, commits, "test-branch", Config{})
		if posted != 0 {
			t.Fatalf("expected 0 posted for empty SHA, got %d", posted)
		}
	})

	t.Run("skips findings without parseable location", func(t *testing.T) {
		forge := &fakeForge{}
		findings := []Finding{
			{Severity: "major", Location: "", CommitSHA: "aaaa1111bbbb2222", Category: "bug", Description: "no loc"},
			{Severity: "major", Location: "no-line", CommitSHA: "aaaa1111bbbb2222", Category: "bug", Description: "bad loc"},
		}
		posted := postFindingsToCommits(ctx, forge, findings, commits, "test-branch", Config{})
		if posted != 0 {
			t.Fatalf("expected 0 posted for unparseable locations, got %d", posted)
		}
	})

	t.Run("default severity threshold is minor", func(t *testing.T) {
		forge := &fakeForge{}
		findings := []Finding{
			{Severity: "info", Location: "main.go:10", CommitSHA: "aaaa1111bbbb2222", Category: "note", Description: "info only"},
			{Severity: "minor", Location: "main.go:20", CommitSHA: "aaaa1111bbbb2222", Category: "style", Description: "minor issue"},
		}
		posted := postFindingsToCommits(ctx, forge, findings, commits, "test-branch", Config{})
		if posted != 1 {
			t.Fatalf("expected 1 posted (minor+ with default threshold), got %d", posted)
		}
	})
}

func TestPostMRResult(t *testing.T) {
	ctx := context.Background()

	t.Run("posts inline and summary", func(t *testing.T) {
		forge := &fakeForge{
			getPR: PullRequest{ID: 42, Title: "test PR"},
		}
		sr := &StoredResult{
			Key: ResultKeyMR(42),
			Findings: []Finding{
				{Severity: "major", Location: "main.go:10", Category: "bug", Description: "null deref"},
			},
		}
		postMRResult(ctx, forge, sr, sr.Key, "both", Config{})

		if len(forge.inlineComments) != 1 {
			t.Errorf("expected 1 inline comment, got %d", len(forge.inlineComments))
		}
		if len(forge.summaryBodies) != 1 {
			t.Errorf("expected 1 summary comment, got %d", len(forge.summaryBodies))
		}
	})

	t.Run("inline only", func(t *testing.T) {
		forge := &fakeForge{
			getPR: PullRequest{ID: 42, Title: "test PR"},
		}
		sr := &StoredResult{
			Key: ResultKeyMR(42),
			Findings: []Finding{
				{Severity: "major", Location: "main.go:10", Category: "bug", Description: "null deref"},
			},
		}
		postMRResult(ctx, forge, sr, sr.Key, "inline", Config{})

		if len(forge.inlineComments) != 1 {
			t.Errorf("expected 1 inline comment, got %d", len(forge.inlineComments))
		}
		if len(forge.summaryBodies) != 0 {
			t.Errorf("expected 0 summary comments, got %d", len(forge.summaryBodies))
		}
	})

	t.Run("summary only", func(t *testing.T) {
		forge := &fakeForge{
			getPR: PullRequest{ID: 42, Title: "test PR"},
		}
		sr := &StoredResult{
			Key: ResultKeyMR(42),
			Findings: []Finding{
				{Severity: "major", Location: "main.go:10", Category: "bug", Description: "null deref"},
			},
		}
		postMRResult(ctx, forge, sr, sr.Key, "summary", Config{})

		if len(forge.inlineComments) != 0 {
			t.Errorf("expected 0 inline comments, got %d", len(forge.inlineComments))
		}
		if len(forge.summaryBodies) != 1 {
			t.Errorf("expected 1 summary comment, got %d", len(forge.summaryBodies))
		}
	})

	t.Run("get PR failure", func(t *testing.T) {
		forge := &fakeForge{
			getErr: fmt.Errorf("not found"),
		}
		sr := &StoredResult{
			Key: ResultKeyMR(99),
			Findings: []Finding{
				{Severity: "major", Location: "main.go:10", Category: "bug", Description: "null deref"},
			},
		}
		postMRResult(ctx, forge, sr, sr.Key, "both", Config{})

		if len(forge.inlineComments) != 0 || len(forge.summaryBodies) != 0 {
			t.Errorf("expected no posts on PR fetch failure")
		}
	})
}
