package main

import (
	"strings"
	"testing"
)

func TestBuildCommitReviewPrompt_ContainsRoot(t *testing.T) {
	p := BuildCommitReviewPrompt(PromptConfig{Root: "/tmp/test-repo"})
	if !strings.Contains(p, "/tmp/test-repo") {
		t.Error("prompt should contain workspace root")
	}
}

func TestBuildCommitReviewPrompt_IntentDriven(t *testing.T) {
	p := BuildCommitReviewPrompt(PromptConfig{Root: "/r"})
	for _, want := range []string{"intent", "correctness", "regressions"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt should mention %q", want)
		}
	}
}

func TestBuildCommitReviewPrompt_NoHedging(t *testing.T) {
	p := BuildCommitReviewPrompt(PromptConfig{Root: "/r"})
	if !strings.Contains(p, "Hedging") {
		t.Error("prompt should instruct against hedging")
	}
}

func TestBuildCommitReviewPrompt_FixupClause(t *testing.T) {
	p := BuildCommitReviewPrompt(PromptConfig{Root: "/r"})
	if !strings.Contains(p, "fixup!") || !strings.Contains(p, "squash!") {
		t.Error("prompt should contain fixup/squash awareness clause")
	}
}

func TestBuildCommitReviewPrompt_WithGuidelines(t *testing.T) {
	p := BuildCommitReviewPrompt(PromptConfig{Root: "/r", Guidelines: "All SQL must use parameterized queries"})
	if !strings.Contains(p, "Project-specific guidelines") {
		t.Error("prompt should contain guidelines section header")
	}
	if !strings.Contains(p, "parameterized queries") {
		t.Error("prompt should contain the guidelines content")
	}
}

func TestBuildCommitReviewPrompt_WithoutGuidelines(t *testing.T) {
	p := BuildCommitReviewPrompt(PromptConfig{Root: "/r"})
	if strings.Contains(p, "Project-specific guidelines") {
		t.Error("prompt should not contain guidelines section when empty")
	}
}

func TestBuildCommitReviewPrompt_Tools(t *testing.T) {
	p := BuildCommitReviewPrompt(PromptConfig{Root: "/r"})
	if !strings.Contains(p, "read_file") {
		t.Error("commit prompt should list read_file tool")
	}
	if strings.Contains(p, "fork") {
		t.Error("commit prompt should not have fork tool")
	}
}

func TestBuildMRReviewPrompt_ContainsRoot(t *testing.T) {
	p := BuildMRReviewPrompt(PromptConfig{Root: "/workspace"})
	if !strings.Contains(p, "/workspace") {
		t.Error("prompt should contain workspace root")
	}
}

func TestBuildMRReviewPrompt_HasFork(t *testing.T) {
	p := BuildMRReviewPrompt(PromptConfig{Root: "/r"})
	if !strings.Contains(p, "fork") {
		t.Error("MR prompt should include fork tool")
	}
}

func TestBuildMRReviewPrompt_DefaultForkTasks(t *testing.T) {
	p := BuildMRReviewPrompt(PromptConfig{Root: "/r"})
	for _, id := range []string{`id:"bugs"`, `id:"security"`, `id:"perf"`, `id:"style"`} {
		if !strings.Contains(p, id) {
			t.Errorf("prompt should contain fork task %s", id)
		}
	}
}

func TestBuildMRReviewPrompt_FixupClause(t *testing.T) {
	p := BuildMRReviewPrompt(PromptConfig{Root: "/r"})
	if !strings.Contains(p, "fixup!") {
		t.Error("MR prompt should contain fixup awareness clause")
	}
}

func TestBuildMRReviewPrompt_WithGuidelines(t *testing.T) {
	p := BuildMRReviewPrompt(PromptConfig{Root: "/r", Guidelines: "Use error wrapping"})
	if !strings.Contains(p, "Use error wrapping") {
		t.Error("MR prompt should include guidelines")
	}
}

func TestBuildMRRepassPrompt_ContainsPriorContext(t *testing.T) {
	p := BuildMRRepassPrompt(PromptConfig{Root: "/r", PriorContext: "found nil deref in handler.go:42"})
	if !strings.Contains(p, "found nil deref in handler.go:42") {
		t.Error("repass prompt should contain prior context")
	}
	if !strings.Contains(p, "<prior_findings>") {
		t.Error("repass prompt should wrap prior context in XML tags")
	}
}

func TestBuildMRRepassPrompt_FixupClause(t *testing.T) {
	p := BuildMRRepassPrompt(PromptConfig{Root: "/r", PriorContext: "none"})
	if !strings.Contains(p, "fixup!") {
		t.Error("repass prompt should contain fixup awareness clause")
	}
}

func TestBuildMRRepassPrompt_NoRepeat(t *testing.T) {
	p := BuildMRRepassPrompt(PromptConfig{Root: "/r", PriorContext: "x"})
	if !strings.Contains(p, "DO NOT repeat") {
		t.Error("repass prompt should instruct not to repeat prior findings")
	}
}

func TestBuildExtractionPrompt_StableOutput(t *testing.T) {
	p1 := BuildExtractionPrompt()
	p2 := BuildExtractionPrompt()
	if p1 != p2 {
		t.Error("extraction prompt should be deterministic")
	}
	if !strings.Contains(p1, `"findings"`) {
		t.Error("extraction prompt should reference findings JSON key")
	}
	if !strings.Contains(p1, "Preserve the exact file:line") {
		t.Error("extraction prompt should instruct to preserve locations")
	}
}

func TestBuildDigestPrompt_StableOutput(t *testing.T) {
	p1 := BuildDigestPrompt()
	p2 := BuildDigestPrompt()
	if p1 != p2 {
		t.Error("digest prompt should be deterministic")
	}
	if !strings.Contains(p1, "theme") {
		t.Error("digest prompt should mention organizing by theme")
	}
	if !strings.Contains(p1, "contradict") {
		t.Error("digest prompt should mention contradicting findings")
	}
}

func TestBuildCLIReviewPrompt_CommitMode(t *testing.T) {
	p := BuildCLIReviewPrompt("diff content", "commit", PromptConfig{})
	if !strings.Contains(p, "commit diff") {
		t.Error("should mention commit scope")
	}
	if !strings.Contains(p, "diff content") {
		t.Error("should contain the diff")
	}
}

func TestBuildCLIReviewPrompt_FullMode(t *testing.T) {
	p := BuildCLIReviewPrompt("diff content", "full", PromptConfig{})
	if !strings.Contains(p, "merge request diff") {
		t.Error("should mention merge request scope")
	}
}

func TestBuildCLIReviewPrompt_WithGuidelines(t *testing.T) {
	p := BuildCLIReviewPrompt("diff", "commit", PromptConfig{Guidelines: "Check for SQL injection"})
	if !strings.Contains(p, "SQL injection") {
		t.Error("CLI prompt should include guidelines")
	}
}

func TestBuildCLIRepassPrompt_IncludesPrior(t *testing.T) {
	p := BuildCLIRepassPrompt("diff", "prior findings here", PromptConfig{})
	if !strings.Contains(p, "prior findings here") {
		t.Error("CLI repass should include prior context")
	}
	if !strings.Contains(p, "do NOT repeat") {
		t.Error("CLI repass should instruct not to repeat")
	}
	if !strings.Contains(p, "<prior_findings>") {
		t.Error("CLI repass should wrap prior context in XML tags")
	}
}

func TestBuildCLIRepassPrompt_EmptyPrior(t *testing.T) {
	p := BuildCLIRepassPrompt("diff", "", PromptConfig{})
	if strings.Contains(p, "<prior_findings>") {
		t.Error("CLI repass should omit prior findings section when empty")
	}
}

func TestBuildMRRepassPrompt_EmptyPrior(t *testing.T) {
	p := BuildMRRepassPrompt(PromptConfig{Root: "/r"})
	if strings.Contains(p, "<prior_findings>") {
		t.Error("MR repass should omit prior findings section when empty")
	}
}

func TestForkTasks_DefaultMode(t *testing.T) {
	ft := forkTasks("")
	for _, id := range []string{"bugs", "security", "perf", "style"} {
		if !strings.Contains(ft, id) {
			t.Errorf("default fork tasks should contain %q", id)
		}
	}
}

func TestForkTasks_SecurityFocus(t *testing.T) {
	ft := forkTasks("security")
	if !strings.Contains(ft, "Deep security review") {
		t.Error("security focus should expand security task")
	}
	if !strings.Contains(ft, "SSRF") {
		t.Error("security focus should mention specific attack patterns")
	}
	if strings.Contains(ft, "unnecessary allocations, O(n²) loops, missing caching") {
		t.Error("security focus should shorten perf task")
	}
}

func TestForkTasks_PerformanceFocus(t *testing.T) {
	ft := forkTasks("performance")
	if !strings.Contains(ft, "Deep performance review") {
		t.Error("performance focus should expand perf task")
	}
	if !strings.Contains(ft, "GC pressure") {
		t.Error("performance focus should mention GC pressure")
	}
}

func TestForkTasks_StyleFocus(t *testing.T) {
	ft := forkTasks("style")
	if !strings.Contains(ft, "Deep code quality review") {
		t.Error("style focus should expand style task")
	}
}

func TestGuidelinesSection_Empty(t *testing.T) {
	if s := guidelinesSection(""); s != "" {
		t.Errorf("empty guidelines should return empty string, got %q", s)
	}
	if s := guidelinesSection("   "); s != "" {
		t.Errorf("whitespace-only guidelines should return empty string, got %q", s)
	}
}

func TestGuidelinesSection_NonEmpty(t *testing.T) {
	s := guidelinesSection("Use Go error wrapping")
	if !strings.Contains(s, "Project-specific guidelines") {
		t.Error("should contain section header")
	}
	if !strings.Contains(s, "Use Go error wrapping") {
		t.Error("should contain guidelines text")
	}
}
