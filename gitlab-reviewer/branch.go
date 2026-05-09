package main

import (
	"fmt"
	"os/exec"
	"strings"
)

func detectBaseBranch(repoPath string) string {
	for _, name := range []string{"main", "master"} {
		cmd := exec.Command("git", "rev-parse", "--verify", name)
		cmd.Dir = repoPath
		if err := cmd.Run(); err == nil {
			return name
		}
	}
	return "main"
}

func branchCommits(repoPath, branchName, baseBranch string) ([]Commit, error) {
	cmd := exec.Command("git", "log", "--format=%H\t%an\t%s", baseBranch+".."+branchName)
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
	cmd := exec.Command("git", "diff", baseBranch+"..."+branchName)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git diff: %w", err)
	}
	return string(out), nil
}
