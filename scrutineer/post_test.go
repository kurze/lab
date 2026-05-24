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
