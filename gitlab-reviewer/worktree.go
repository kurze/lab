package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func CreateWorktree(ctx context.Context, repoPath string, mrIID int64) (dir string, cleanup func(), err error) {
	absRepo, err := filepath.Abs(repoPath)
	if err != nil {
		return "", nil, fmt.Errorf("resolve repo path: %w", err)
	}

	branchName := fmt.Sprintf("mr-%d", mrIID)
	ref := fmt.Sprintf("+refs/merge-requests/%d/head:refs/heads/%s", mrIID, branchName)

	fetchCmd := exec.CommandContext(ctx, "git", "fetch", "origin", ref)
	fetchCmd.Dir = absRepo
	if out, err := fetchCmd.CombinedOutput(); err != nil {
		return "", nil, fmt.Errorf("git fetch: %w\n%s", err, out)
	}

	dir, err = os.MkdirTemp("", fmt.Sprintf("gitlab-reviewer-%d-", mrIID))
	if err != nil {
		return "", nil, fmt.Errorf("create temp dir: %w", err)
	}

	addCmd := exec.CommandContext(ctx, "git", "worktree", "add", dir, branchName)
	addCmd.Dir = absRepo
	if out, err := addCmd.CombinedOutput(); err != nil {
		os.RemoveAll(dir)
		return "", nil, fmt.Errorf("git worktree add: %w\n%s", err, out)
	}

	cleanup = func() {
		rmCmd := exec.Command("git", "worktree", "remove", "--force", dir)
		rmCmd.Dir = absRepo
		rmCmd.Run()

		brCmd := exec.Command("git", "branch", "-D", branchName)
		brCmd.Dir = absRepo
		brCmd.Run()
	}

	return dir, cleanup, nil
}
