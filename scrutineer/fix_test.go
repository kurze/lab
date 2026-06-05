package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kurze/lab/agentcore"
)

func TestFilterFixable(t *testing.T) {
	findings := []Finding{
		{Severity: "critical", Location: "main.go:10", Description: "sql injection"},
		{Severity: "major", Location: "auth.go:20", Description: "missing check"},
		{Severity: "minor", Location: "util.go:5", Description: "unused var"},
		{Severity: "info", Location: "readme.go:1", Description: "typo"},
		{Severity: "major", Location: "", Description: "no location"},
		{Severity: "major", Location: "bad location", Description: "bad loc"},
	}

	tests := []struct {
		name      string
		threshold string
		want      int
	}{
		{"default (minor)", "", 3},
		{"minor", "minor", 3},
		{"major", "major", 2},
		{"critical", "critical", 1},
		{"info", "info", 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterFixable(findings, tt.threshold)
			if len(got) != tt.want {
				t.Errorf("filterFixable(%q) = %d findings, want %d", tt.threshold, len(got), tt.want)
			}
		})
	}
}

func TestFilterFixable_NoValidLocation(t *testing.T) {
	findings := []Finding{
		{Severity: "critical", Location: ""},
		{Severity: "critical", Location: "no-line"},
		{Severity: "critical", Location: "has space:10"},
	}
	got := filterFixable(findings, "info")
	if len(got) != 0 {
		t.Errorf("expected 0 fixable findings, got %d", len(got))
	}
}

func TestValidateFilePath(t *testing.T) {
	tests := []struct {
		path    string
		wantErr bool
	}{
		{"main.go", false},
		{"src/util.go", false},
		{"../etc/passwd", true},
		{"/etc/passwd", true},
		{"../../foo", true},
		{"src/../main.go", false},
		{"src/../../escape.go", true},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			err := validateFilePath(tt.path)
			if tt.wantErr && err == nil {
				t.Errorf("expected error for path %q", tt.path)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error for path %q: %v", tt.path, err)
			}
		})
	}
}

func TestExtractPatch(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		check   func(string) error
	}{
		{
			name: "fenced diff block",
			input: "Here is the fix:\n```diff\n--- a/main.go\n+++ b/main.go\n@@ -1,3 +1,3 @@\n-old\n+new\n```\nDone.",
			check: func(p string) error {
				if !strings.Contains(p, "--- a/main.go") {
					return fmt.Errorf("missing --- line")
				}
				if !strings.Contains(p, "+new") {
					return fmt.Errorf("missing +new line")
				}
				return nil
			},
		},
		{
			name: "plain fenced block with diff content",
			input: "```\n--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-old\n+new\n```",
			check: func(p string) error {
				if !strings.Contains(p, "--- a/main.go") {
					return fmt.Errorf("missing --- line")
				}
				return nil
			},
		},
		{
			name: "raw diff lines",
			input: "Some text\n--- a/file.go\n+++ b/file.go\n@@ -1 +1 @@\n-x\n+y\n",
			check: func(p string) error {
				if !strings.HasPrefix(p, "--- a/file.go") {
					return fmt.Errorf("should start with --- line, got %q", p[:20])
				}
				return nil
			},
		},
		{
			name:    "no patch at all",
			input:   "I can't generate a fix for this.",
			wantErr: true,
		},
		{
			name: "diff --git prefix",
			input: "diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n@@ -1 +1 @@\n-a\n+b\n",
			check: func(p string) error {
				if !strings.HasPrefix(p, "diff --git") {
					return fmt.Errorf("should start with diff --git")
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patch, err := extractPatch(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got patch: %q", patch)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				if err := tt.check(patch); err != nil {
					t.Errorf("patch check failed: %v\npatch: %q", err, patch)
				}
			}
		})
	}
}

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	return dir
}

func commitFile(t *testing.T, dir, name, content, msg string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", name},
		{"git", "commit", "-m", msg},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func TestApplyPatch(t *testing.T) {
	dir := initTestRepo(t)
	commitFile(t, dir, "hello.go", "package main\n\nfunc hello() {\n\tprintln(\"hello\")\n}\n", "initial")

	t.Run("valid patch", func(t *testing.T) {
		patch := `--- a/hello.go
+++ b/hello.go
@@ -3,3 +3,3 @@
 func hello() {
-	println("hello")
+	println("world")
 }
`
		ctx := context.Background()
		if err := applyPatch(ctx, dir, patch); err != nil {
			t.Fatalf("applyPatch failed: %v", err)
		}

		content, _ := os.ReadFile(filepath.Join(dir, "hello.go"))
		if !strings.Contains(string(content), `"world"`) {
			t.Errorf("patch not applied, content: %s", content)
		}

		// restore for next test
		cmd := exec.Command("git", "checkout", "--", "hello.go")
		cmd.Dir = dir
		cmd.Run()
	})

	t.Run("invalid patch", func(t *testing.T) {
		patch := `--- a/hello.go
+++ b/hello.go
@@ -3,3 +3,3 @@
 func nonexistent() {
-	this line does not exist
+	replacement
 }
`
		ctx := context.Background()
		err := applyPatch(ctx, dir, patch)
		if err == nil {
			t.Fatal("expected error for invalid patch")
		}
		if !strings.Contains(err.Error(), "does not apply cleanly") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestCreateFixupCommit(t *testing.T) {
	dir := initTestRepo(t)
	originalSHA := commitFile(t, dir, "main.go", "package main\n\nfunc main() {}\n", "initial commit")

	// Simulate a patch being applied
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {\n\t// fixed\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	finding := Finding{
		Severity:    "major",
		Location:    "main.go:3",
		Description: "empty function body",
	}

	sha, err := createFixupCommit(ctx, dir, finding, originalSHA)
	if err != nil {
		t.Fatalf("createFixupCommit: %v", err)
	}

	if sha == "" {
		t.Fatal("expected non-empty commit SHA")
	}

	// Verify commit message has fixup! prefix
	cmd := exec.Command("git", "log", "-1", "--format=%s", sha)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	msg := strings.TrimSpace(string(out))
	if !strings.HasPrefix(msg, "fixup!") {
		t.Errorf("expected fixup! prefix, got: %q", msg)
	}
}

func TestFindTargetCommit(t *testing.T) {
	dir := initTestRepo(t)
	sha := commitFile(t, dir, "target.go", "package main\n", "add target")

	ctx := context.Background()
	got, err := findTargetCommit(ctx, dir, "target.go")
	if err != nil {
		t.Fatalf("findTargetCommit: %v", err)
	}
	if got != sha {
		t.Errorf("got %s, want %s", got, sha)
	}

	_, err = findTargetCommit(ctx, dir, "nonexistent.go")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestGenerateFixes_DryRun(t *testing.T) {
	dir := initTestRepo(t)
	commitFile(t, dir, "buggy.go", "package main\n\nfunc buggy() {\n\tvar x int\n\t_ = x\n}\n", "add buggy")

	patch := `--- a/buggy.go
+++ b/buggy.go
@@ -3,4 +3,3 @@
 func buggy() {
-	var x int
-	_ = x
+	println("fixed")
 }
`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": "```diff\n" + patch + "```"}},
			},
			"usage": map[string]int{"total_tokens": 100},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	llm := agentcore.NewLLMClient(srv.URL)
	findings := []Finding{
		{Severity: "major", Location: "buggy.go:4", Description: "unused variable", Category: "style"},
	}

	results, err := generateFixes(context.Background(), llm, "test-model", findings, FixOptions{
		DryRun:    true,
		Threshold: "minor",
		WorkDir:   dir,
	})
	if err != nil {
		t.Fatalf("generateFixes: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Patch == "" {
		t.Error("expected non-empty patch in dry-run result")
	}
	if results[0].Applied {
		t.Error("patch should not be applied in dry-run mode")
	}
	if results[0].Committed {
		t.Error("should not commit in dry-run mode")
	}

	// Verify the file was NOT changed
	content, _ := os.ReadFile(filepath.Join(dir, "buggy.go"))
	if strings.Contains(string(content), "fixed") {
		t.Error("file should not be modified in dry-run mode")
	}
}

func TestGenerateFixes_Apply(t *testing.T) {
	dir := initTestRepo(t)
	targetSHA := commitFile(t, dir, "buggy.go", "package main\n\nfunc buggy() {\n\tvar x int\n\t_ = x\n}\n", "add buggy")

	patch := `--- a/buggy.go
+++ b/buggy.go
@@ -3,4 +3,3 @@
 func buggy() {
-	var x int
-	_ = x
+	println("fixed")
 }
`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": "```diff\n" + patch + "```"}},
			},
			"usage": map[string]int{"total_tokens": 100},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	llm := agentcore.NewLLMClient(srv.URL)
	findings := []Finding{
		{Severity: "major", Location: "buggy.go:4", Description: "unused variable", Category: "style", CommitSHA: targetSHA[:8]},
	}

	results, err := generateFixes(context.Background(), llm, "test-model", findings, FixOptions{
		DryRun:    false,
		Threshold: "minor",
		WorkDir:   dir,
	})
	if err != nil {
		t.Fatalf("generateFixes: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Applied {
		t.Errorf("patch should be applied, skip reason: %s", results[0].SkipReason)
	}
	if !results[0].Committed {
		t.Errorf("should be committed, skip reason: %s", results[0].SkipReason)
	}

	// Verify file was changed
	content, _ := os.ReadFile(filepath.Join(dir, "buggy.go"))
	if !strings.Contains(string(content), "fixed") {
		t.Errorf("file should be modified, got: %s", content)
	}

	// Verify fixup commit exists
	cmd := exec.Command("git", "log", "-1", "--format=%s")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	msg := strings.TrimSpace(string(out))
	if !strings.HasPrefix(msg, "fixup!") {
		t.Errorf("expected fixup! commit, got: %q", msg)
	}
}
