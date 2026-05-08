package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "adr.md"), []byte("# ADR 001\nDecision: use Go\nReason: portability\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestExecReadFile(t *testing.T) {
	root := setupWorkspace(t)

	r := execReadFile(root, "docs/adr.md", 0, 0)
	if r.IsError {
		t.Fatalf("unexpected error: %s", r.Content)
	}
	if !strings.Contains(r.Content, "ADR 001") {
		t.Error("expected file content")
	}
}

func TestExecReadFileRange(t *testing.T) {
	root := setupWorkspace(t)

	r := execReadFile(root, "docs/adr.md", 2, 3)
	if r.IsError {
		t.Fatalf("unexpected error: %s", r.Content)
	}
	if !strings.Contains(r.Content, "Decision") {
		t.Error("expected line 2 content")
	}
	if strings.Contains(r.Content, "ADR 001") {
		t.Error("should not contain line 1")
	}
}

func TestExecReadFileTraversal(t *testing.T) {
	root := setupWorkspace(t)

	r := execReadFile(root, "../../etc/passwd", 0, 0)
	if !r.IsError {
		t.Error("expected error for path traversal")
	}
}

func TestExecGrep(t *testing.T) {
	root := setupWorkspace(t)

	r := execGrep(root, "portability", ".", "")
	if r.IsError {
		t.Fatalf("unexpected error: %s", r.Content)
	}
	if !strings.Contains(r.Content, "docs/adr.md") {
		t.Error("expected match in adr.md")
	}
}

func TestExecGrepWithGlob(t *testing.T) {
	root := setupWorkspace(t)

	r := execGrep(root, "func", ".", "*.go")
	if r.IsError {
		t.Fatalf("unexpected error: %s", r.Content)
	}
	if !strings.Contains(r.Content, "main.go") {
		t.Error("expected match in main.go")
	}
}

func TestExecListDir(t *testing.T) {
	root := setupWorkspace(t)

	r := execListDir(root, ".")
	if r.IsError {
		t.Fatalf("unexpected error: %s", r.Content)
	}
	if !strings.Contains(r.Content, "docs/") {
		t.Error("expected docs directory")
	}
	if !strings.Contains(r.Content, "main.go") {
		t.Error("expected main.go file")
	}
}
