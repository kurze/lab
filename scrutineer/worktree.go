package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func cleanupStaleWorktree(ctx context.Context, absRepo, branchName string) {
	listCmd := exec.CommandContext(ctx, "git", "worktree", "list", "--porcelain")
	listCmd.Dir = absRepo
	out, err := listCmd.Output()
	if err != nil {
		return
	}

	var worktreePath string
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "worktree ") {
			worktreePath = strings.TrimPrefix(line, "worktree ")
		}
		if line == "branch refs/heads/"+branchName && worktreePath != "" {
			rmCmd := exec.CommandContext(ctx, "git", "worktree", "remove", "--force", worktreePath)
			rmCmd.Dir = absRepo
			if out, err := rmCmd.CombinedOutput(); err != nil {
				log.Printf("warning: remove stale worktree %s: %v\n%s", worktreePath, err, out)
			}
			break
		}
	}

	delCmd := exec.CommandContext(ctx, "git", "branch", "-D", branchName)
	delCmd.Dir = absRepo
	if out, err := delCmd.CombinedOutput(); err != nil {
		log.Printf("warning: delete stale branch %s: %v\n%s", branchName, err, out)
	}
}

func fetchRef(forgeName string, id int64, branchName string) string {
	switch forgeName {
	case "github":
		return fmt.Sprintf("+refs/pull/%d/head:refs/heads/%s", id, branchName)
	default:
		return fmt.Sprintf("+refs/merge-requests/%d/head:refs/heads/%s", id, branchName)
	}
}

func CreateWorktree(ctx context.Context, repoPath string, id int64, forgeName string) (dir string, cleanup func(), err error) {
	absRepo, err := filepath.Abs(repoPath)
	if err != nil {
		return "", nil, fmt.Errorf("resolve repo path: %w", err)
	}

	branchName := fmt.Sprintf("review-%d", id)

	cleanupStaleWorktree(ctx, absRepo, branchName)

	ref := fetchRef(forgeName, id, branchName)

	fetchCmd := exec.CommandContext(ctx, "git", "fetch", "origin", ref)
	fetchCmd.Dir = absRepo
	if out, err := fetchCmd.CombinedOutput(); err != nil {
		return "", nil, fmt.Errorf("git fetch: %w\n%s", err, out)
	}

	dir, err = os.MkdirTemp("", fmt.Sprintf("review-%d-", id))
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
		if out, err := rmCmd.CombinedOutput(); err != nil {
			log.Printf("warning: remove worktree %s: %v\n%s", dir, err, out)
		}

		brCmd := exec.Command("git", "branch", "-D", branchName)
		brCmd.Dir = absRepo
		if out, err := brCmd.CombinedOutput(); err != nil {
			log.Printf("warning: delete branch %s: %v\n%s", branchName, err, out)
		}
	}

	return dir, cleanup, nil
}
