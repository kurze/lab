package main

import (
	"fmt"
	"os/exec"
	"strings"
)

func detectBaseBranch(repoPath string) string {
	// Try the remote HEAD first (most reliable for non-standard default branches).
	cmd := exec.Command("git", "symbolic-ref", "refs/remotes/origin/HEAD")
	cmd.Dir = repoPath
	if out, err := cmd.Output(); err == nil {
		ref := strings.TrimSpace(string(out))
		// "refs/remotes/origin/integration" → "origin/integration"
		if short := strings.TrimPrefix(ref, "refs/remotes/"); short != ref {
			return short
		}
	}

	for _, name := range []string{"main", "master", "origin/main", "origin/master"} {
		if resolveRef(repoPath, name) {
			return name
		}
	}
	return "main"
}

func resolveRef(repoPath, ref string) bool {
	cmd := exec.Command("git", "rev-parse", "--verify", ref)
	cmd.Dir = repoPath
	return cmd.Run() == nil
}

func resolveBranch(repoPath, branchName string) (string, error) {
	if resolveRef(repoPath, branchName) {
		return branchName, nil
	}
	remote := "origin/" + branchName
	if resolveRef(repoPath, remote) {
		return remote, nil
	}
	return "", fmt.Errorf("branch %q not found (tried %q and %q)", branchName, branchName, remote)
}

func branchCommits(repoPath, branchName, baseBranch string) ([]Commit, error) {
	ref, err := resolveBranch(repoPath, branchName)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command("git", "log", "--format=%H\t%an\t%s", baseBranch+".."+ref)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil, nil
	}

	var commits []Commit
	for _, line := range lines {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		commits = append(commits, Commit{
			SHA:     parts[0],
			Author:  parts[1],
			Message: parts[2],
		})
	}
	return commits, nil
}

func branchDiff(repoPath, branchName, baseBranch string) (string, error) {
	ref, err := resolveBranch(repoPath, branchName)
	if err != nil {
		return "", err
	}

	cmd := exec.Command("git", "diff", baseBranch+"..."+ref)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git diff: %w", err)
	}
	return string(out), nil
}
