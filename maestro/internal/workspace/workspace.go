package workspace

import (
	"fmt"
	"os/exec"
	"path/filepath"
)

// Create creates a new git worktree for the given task.
// It runs `git worktree add <worktreeDir>/<taskID> -b <branchName>` from repoRoot.
// Returns the absolute path to the created worktree.
func Create(repoRoot, taskID, branchName, worktreeDir string) (string, error) {
	absRepo, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", fmt.Errorf("resolve repo root: %w", err)
	}

	worktreePath := filepath.Join(absRepo, worktreeDir, taskID)

	cmd := exec.Command("git", "worktree", "add", worktreePath, "-b", branchName)
	cmd.Dir = absRepo
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git worktree add: %w\n%s", err, out)
	}

	return worktreePath, nil
}

// Remove removes an existing git worktree at the given path.
// It runs `git worktree remove --force <worktreePath>` from the directory
// containing the worktree.
func Remove(worktreePath string) error {
	absPath, err := filepath.Abs(worktreePath)
	if err != nil {
		return fmt.Errorf("resolve worktree path: %w", err)
	}

	// Run from the parent of the worktree directory so we're not inside
	// the directory being removed.
	parentDir := filepath.Dir(absPath)

	cmd := exec.Command("git", "worktree", "remove", "--force", absPath)
	cmd.Dir = parentDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree remove: %w\n%s", err, out)
	}

	return nil
}

// Resolve returns the absolute path to a worktree for the given task.
// It does not verify the worktree actually exists on disk.
func Resolve(repoRoot, taskID, worktreeDir string) (string, error) {
	absRepo, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", fmt.Errorf("resolve repo root: %w", err)
	}

	return filepath.Join(absRepo, worktreeDir, taskID), nil
}

// BranchName returns the branch name for a task. If jiraKey is non-empty,
// the format is maestro/<jiraKey>-<slug>. Otherwise it falls back to
// maestro/<localID>.
func BranchName(localID, jiraKey, slug string) string {
	if jiraKey != "" && slug != "" {
		return fmt.Sprintf("maestro/%s-%s", jiraKey, slug)
	}
	if jiraKey != "" {
		return fmt.Sprintf("maestro/%s", jiraKey)
	}
	return fmt.Sprintf("maestro/%s", localID)
}
