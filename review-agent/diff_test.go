package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func setupGitRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %s\n%s", args, err, out)
		}
	}

	run("init")
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "initial")

	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	return root
}

func TestGitDiffUncommitted(t *testing.T) {
	root := setupGitRepo(t)

	diff, err := gitDiff(root, "HEAD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(diff, "fmt.Println") {
		t.Error("expected diff to contain new code")
	}
	if !strings.Contains(diff, "+") {
		t.Error("expected diff to contain additions")
	}
}

func TestGitDiffEmpty(t *testing.T) {
	root := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %s\n%s", args, err, out)
		}
	}

	run("init")
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "init")

	diff, err := gitDiff(root, "HEAD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(diff) != "" {
		t.Errorf("expected empty diff, got: %s", diff)
	}
}

func TestParseDiffRef(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"HEAD", "HEAD"},
		{"main..feature", "main..feature"},
		{"HEAD~3", "HEAD~3"},
		{"abc123", "abc123"},
		{"", "HEAD"},
		{"HEAD; rm -rf /", ""},
		{"$(evil)", ""},
	}

	for _, tt := range tests {
		got := parseDiffRef(tt.input)
		if got != tt.want {
			t.Errorf("parseDiffRef(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSanitizeTracerTag(t *testing.T) {
	got := sanitizeTracerTag("main..feature/branch")
	if strings.Contains(got, "/") || strings.Contains(got, "..") {
		t.Errorf("sanitizeTracerTag should remove / and .., got: %s", got)
	}
}
