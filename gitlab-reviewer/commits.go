package main

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
)

func reviewByCommits(ctx context.Context, forge Forge, reviewer Reviewer, worktreeDir string, pr PullRequest) ([]CommitReviewResult, error) {
	commits, err := forge.ListCommits(ctx, pr.ID)
	if err != nil {
		return nil, fmt.Errorf("list commits: %w", err)
	}

	if len(commits) == 0 {
		return nil, nil
	}

	var results []CommitReviewResult
	for i, commit := range commits {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		if isMergeCommit(worktreeDir, commit.SHA) {
			log.Printf("  skip merge commit %d/%d: %s", i+1, len(commits), commit.SHA[:8])
			continue
		}

		diff, err := commitDiff(worktreeDir, commit.SHA)
		if err != nil {
			log.Printf("  skip commit %d/%d %s: git show: %v", i+1, len(commits), commit.SHA[:8], err)
			continue
		}
		if strings.TrimSpace(diff) == "" {
			continue
		}

		sha := commit.SHA
		if len(sha) > 8 {
			sha = sha[:8]
		}
		msg := commit.Message
		if idx := strings.IndexByte(msg, '\n'); idx > 0 {
			msg = msg[:idx]
		}

		log.Printf("  reviewing commit %d/%d: %s %s", i+1, len(commits), sha, msg)

		taggedDiff := fmt.Sprintf("Commit: %s — %s\n\n%s", sha, msg, diff)
		result, err := reviewer.Review(ctx, worktreeDir, taggedDiff)
		if err != nil {
			log.Printf("  commit %s review failed: %v", sha, err)
			continue
		}

		results = append(results, CommitReviewResult{
			Commit: commit,
			Result: result,
		})
	}

	return results, nil
}

func commitDiff(worktreeDir, sha string) (string, error) {
	cmd := exec.Command("git", "show", sha, "--no-color", "--unified=5", "--format=")
	cmd.Dir = worktreeDir
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("%s", string(exitErr.Stderr))
		}
		return "", err
	}
	return string(out), nil
}

func isMergeCommit(worktreeDir, sha string) bool {
	cmd := exec.Command("git", "cat-file", "-p", sha)
	cmd.Dir = worktreeDir
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	parents := 0
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "parent ") {
			parents++
		}
		if line == "" {
			break
		}
	}
	return parents > 1
}

func mergeCommitResults(crs []CommitReviewResult) *ReviewResult {
	merged := &ReviewResult{}
	for _, cr := range crs {
		if cr.Result == nil {
			continue
		}
		if merged.Model == "" {
			merged.Model = cr.Result.Model
		}
		sha := cr.Commit.SHA
		if len(sha) > 8 {
			sha = sha[:8]
		}
		for _, f := range cr.Result.Findings {
			f.Location = fmt.Sprintf("[%s] %s", sha, f.Location)
			merged.Findings = append(merged.Findings, f)
		}
	}
	return merged
}
