package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

type CommitStatus string

const (
	CommitStarted CommitStatus = "started"
	CommitDone    CommitStatus = "done"
	CommitFailed  CommitStatus = "failed"
)

type CommitProgressEvent struct {
	Index   int
	Total   int
	SHA     string
	Message string
	Status  CommitStatus
	Err     error
	Result  *ReviewResult
}

type ProgressFunc func(CommitProgressEvent)

type ReviewByCommitsOpts struct {
	State       CommitReviewStore
	Project     string
	OnProgress  ProgressFunc
	Concurrency int
}

type workItem struct {
	commit Commit
	sha    string
	msg    string
	diff   string
}

func reviewByCommits(ctx context.Context, reviewer Reviewer, worktreeDir string, commits []Commit, opts *ReviewByCommitsOpts) ([]CommitReviewResult, error) {
	if len(commits) == 0 {
		return nil, nil
	}

	var items []workItem
	for i, commit := range commits {
		if opts != nil && opts.State != nil && opts.State.IsCommitReviewed(opts.Project, commit.SHA) {
			logf("  skip already reviewed commit %d/%d: %s", i+1, len(commits), cl(ansiDim, commit.SHA[:8]))
			continue
		}

		if isMergeCommit(worktreeDir, commit.SHA) {
			logf("  skip merge commit %d/%d: %s", i+1, len(commits), cl(ansiDim, commit.SHA[:8]))
			continue
		}

		diff, err := commitDiff(worktreeDir, commit.SHA)
		if err != nil {
			warnf("  skip commit %d/%d %s: git show: %v", i+1, len(commits), commit.SHA[:8], err)
			continue
		}
		if strings.TrimSpace(diff) == "" {
			continue
		}

		sha := commit.SHA
		if len(sha) > 8 {
			sha = sha[:8]
		}

		items = append(items, workItem{
			commit: commit,
			sha:    sha,
			msg:    firstline(commit.Message),
			diff:   diff,
		})
	}

	if len(items) == 0 {
		return nil, nil
	}

	concurrency := 1
	if opts != nil && opts.Concurrency > 1 {
		concurrency = opts.Concurrency
	}

	total := len(items)
	results := make([]*CommitReviewResult, len(items))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for wi, item := range items {
		if ctx.Err() != nil {
			break
		}

		wg.Add(1)
		wi, item := wi, item
		go func() {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()

			if ctx.Err() != nil {
				return
			}

			if llmr, ok := reviewer.(*LLMReviewer); ok {
				llmr.mu.Lock()
				if llmr.TraceMeta == nil {
					llmr.TraceMeta = make(map[string]string)
				}
				llmr.TraceMeta["commit"] = item.commit.SHA
				llmr.mu.Unlock()
			}

			logf("  reviewing commit %d/%d: %s %s", wi+1, total, cl(ansiDim, item.sha), item.msg)
			if opts != nil && opts.OnProgress != nil {
				opts.OnProgress(CommitProgressEvent{
					Index:   wi + 1,
					Total:   total,
					SHA:     item.sha,
					Message: item.msg,
					Status:  CommitStarted,
				})
			}

			wtDir, wtCleanup, wtErr := createWorktree(worktreeDir, item.commit.SHA)
			if wtErr != nil {
				errf("  commit %s worktree failed: %v", item.sha, wtErr)
				return
			}
			defer wtCleanup()

			taggedDiff := fmt.Sprintf("Commit: %s — %s\n\n%s", item.sha, item.msg, item.diff)
			if isFixupCommit(item.commit.Message) {
				taggedDiff = fmt.Sprintf("Commit: %s — %s\n[NOTE: This is a fixup/squash commit. Focus on correctness of the fix, not style.]\n\n%s", item.sha, item.msg, item.diff)
			}
			result, err := reviewer.Review(ctx, wtDir, taggedDiff)
			if err != nil {
				errf("  commit %s review failed: %v", item.sha, err)
				if opts != nil && opts.OnProgress != nil {
					opts.OnProgress(CommitProgressEvent{
						Index:   wi + 1,
						Total:   total,
						SHA:     item.sha,
						Message: item.msg,
						Status:  CommitFailed,
						Err:     err,
					})
				}
				return
			}

			results[wi] = &CommitReviewResult{
				Commit: item.commit,
				Result: result,
			}
			if opts != nil && opts.State != nil {
				opts.State.MarkCommitReviewed(opts.Project, item.commit.SHA)
			}
			if opts != nil && opts.OnProgress != nil {
				opts.OnProgress(CommitProgressEvent{
					Index:   wi + 1,
					Total:   total,
					SHA:     item.sha,
					Message: item.msg,
					Status:  CommitDone,
					Result:  result,
				})
			}
		}()
	}

	wg.Wait()

	collected := make([]CommitReviewResult, 0, len(results))
	for _, r := range results {
		if r != nil {
			collected = append(collected, *r)
		}
	}

	if ctx.Err() != nil {
		return collected, ctx.Err()
	}

	return collected, nil
}

func createWorktree(repoDir, sha string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "scrutineer-wt-*")
	if err != nil {
		return "", nil, fmt.Errorf("mkdtemp: %w", err)
	}
	cleanup := func() {
		exec.Command("git", "-C", repoDir, "worktree", "remove", "--force", dir).Run()
		os.RemoveAll(dir)
	}

	cmd := exec.Command("git", "worktree", "add", "--detach", dir, sha)
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("git worktree add: %s", strings.TrimSpace(string(out)))
	}
	return dir, cleanup, nil
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

func isFixupCommit(message string) bool {
	msg := strings.TrimSpace(message)
	return strings.HasPrefix(msg, "fixup! ") || strings.HasPrefix(msg, "squash! ")
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
		merged.TokensUsed += cr.Result.TokensUsed
		merged.GeneratedTokens += cr.Result.GeneratedTokens
		if cr.Result.PeakContextTokens > merged.PeakContextTokens {
			merged.PeakContextTokens = cr.Result.PeakContextTokens
		}
		sha := cr.Commit.SHA
		if len(sha) > 8 {
			sha = sha[:8]
		}
		for _, f := range cr.Result.Findings {
			f.CommitSHA = sha
			merged.Findings = append(merged.Findings, f)
		}
	}
	return merged
}
