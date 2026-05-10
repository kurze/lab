package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initBareRepo creates a git repo in dir with an initial commit so that
// worktree branches can be created.
func initBareRepo(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "commit", "--allow-empty", "-m", "initial"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
	}
}

// branchExists checks whether a branch exists in the repo at dir.
func branchExists(t *testing.T, dir, branch string) bool {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--verify", "refs/heads/"+branch)
	cmd.Dir = dir
	return cmd.Run() == nil
}

func TestCreateAndRemove(t *testing.T) {
	repoDir := t.TempDir()
	initBareRepo(t, repoDir)

	taskID := "m-20260510-abcd"
	branchName := BranchName(taskID, "", "")
	worktreeDir := ".worktrees"

	// Create worktree
	wtPath, err := Create(repoDir, taskID, branchName, worktreeDir)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Verify the worktree directory exists
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		t.Fatalf("worktree directory does not exist: %s", wtPath)
	}

	// Verify the expected path
	expectedPath := filepath.Join(repoDir, worktreeDir, taskID)
	if wtPath != expectedPath {
		t.Fatalf("unexpected worktree path: got %s, want %s", wtPath, expectedPath)
	}

	// Verify branch was created
	if !branchExists(t, repoDir, branchName) {
		t.Fatalf("branch %s does not exist after Create", branchName)
	}

	// Remove worktree
	if err := Remove(wtPath); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Verify the worktree directory is gone
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Fatalf("worktree directory still exists after Remove: %s", wtPath)
	}
}

func TestCreateWithJiraKey(t *testing.T) {
	repoDir := t.TempDir()
	initBareRepo(t, repoDir)

	taskID := "m-20260510-ef01"
	jiraKey := "TRUS-42"
	slug := "add-auth"
	branchName := BranchName(taskID, jiraKey, slug)
	worktreeDir := ".worktrees"

	expectedBranch := "maestro/TRUS-42-add-auth"
	if branchName != expectedBranch {
		t.Fatalf("BranchName: got %s, want %s", branchName, expectedBranch)
	}

	wtPath, err := Create(repoDir, taskID, branchName, worktreeDir)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer Remove(wtPath)

	if !branchExists(t, repoDir, expectedBranch) {
		t.Fatalf("branch %s does not exist", expectedBranch)
	}
}

func TestResolve(t *testing.T) {
	repoDir := t.TempDir()
	taskID := "m-20260510-1234"
	worktreeDir := ".worktrees"

	wtPath, err := Resolve(repoDir, taskID, worktreeDir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	expected := filepath.Join(repoDir, worktreeDir, taskID)
	if wtPath != expected {
		t.Fatalf("Resolve: got %s, want %s", wtPath, expected)
	}
}

func TestBranchName(t *testing.T) {
	tests := []struct {
		localID  string
		jiraKey  string
		slug     string
		expected string
	}{
		{"m-20260510-abcd", "", "", "maestro/m-20260510-abcd"},
		{"m-20260510-abcd", "TRUS-42", "add-auth", "maestro/TRUS-42-add-auth"},
		{"m-20260510-abcd", "TRUS-42", "", "maestro/TRUS-42"},
	}

	for _, tt := range tests {
		got := BranchName(tt.localID, tt.jiraKey, tt.slug)
		if got != tt.expected {
			t.Errorf("BranchName(%q, %q, %q) = %q, want %q",
				tt.localID, tt.jiraKey, tt.slug, got, tt.expected)
		}
	}
}

func TestCreateDuplicateFails(t *testing.T) {
	repoDir := t.TempDir()
	initBareRepo(t, repoDir)

	taskID := "m-20260510-dup1"
	branchName := BranchName(taskID, "", "")
	worktreeDir := ".worktrees"

	wtPath, err := Create(repoDir, taskID, branchName, worktreeDir)
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}
	defer Remove(wtPath)

	// Second create with same taskID should fail
	_, err = Create(repoDir, taskID, branchName, worktreeDir)
	if err == nil {
		t.Fatal("expected error creating duplicate worktree, got nil")
	}
	if !strings.Contains(err.Error(), "git worktree add") {
		t.Fatalf("unexpected error: %v", err)
	}
}
