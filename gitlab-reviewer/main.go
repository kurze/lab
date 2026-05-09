package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kurze/lab/agentcore"
)

func main() {
	project := flag.String("project", "", "project path (owner/repo)")
	mrIID := flag.Int64("mr", 0, "review a single merge/pull request by number")
	post := flag.Bool("post", false, "post findings as comments (default: dry-run)")
	configPath := flag.String("config", "", "path to config file")
	repoPath := flag.String("repo", "", "path to local repo clone")
	batch := flag.Bool("batch", false, "batch mode: review all unreviewed MRs without TUI")
	mode := flag.String("mode", "", "review mode: full, commits, or both")
	inline := flag.Bool("inline", false, "post findings as inline comments on the diff")
	branch := flag.String("branch", "", "review a local branch (commits since base branch)")
	flag.Parse()

	cfg := loadConfig(*configPath)
	if *project != "" {
		cfg.Project = *project
	}
	if *repoPath != "" {
		cfg.RepoPath = *repoPath
	}
	cfg.DryRun = !*post
	if *mode != "" {
		cfg.ReviewMode = *mode
	}
	if *inline {
		cfg.InlineComments = true
	}

	if *branch != "" {
		if cfg.ReviewCommand == "" && cfg.LLM.URL == "" {
			fmt.Fprintf(os.Stderr, "error: review engine required: set review_command or [llm] url in config\n")
			os.Exit(1)
		}
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()
		if err := runBranch(ctx, cfg, *branch); err != nil {
			log.Fatalf("error: %v", err)
		}
		return
	}

	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	state, err := LoadState("")
	if err != nil {
		log.Fatalf("state: %v", err)
	}

	if *mrIID > 0 || *batch {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()
		if err := run(ctx, cfg, state, *mrIID); err != nil {
			log.Fatalf("error: %v", err)
		}
		return
	}

	forge, err := NewForge(cfg)
	if err != nil {
		log.Fatalf("forge: %v", err)
	}

	m := newModel(forge, newReviewer(cfg), cfg, state)
	var prog *tea.Program
	m.programRef = &prog
	p := tea.NewProgram(m, tea.WithAltScreen())
	prog = p
	if _, err := p.Run(); err != nil {
		log.Fatalf("tui: %v", err)
	}
}

func runBranch(ctx context.Context, cfg Config, branchName string) error {
	baseBranch := detectBaseBranch(cfg.RepoPath)
	reviewer := newReviewer(cfg)

	commits, err := branchCommits(cfg.RepoPath, branchName, baseBranch)
	if err != nil {
		return fmt.Errorf("list branch commits: %w", err)
	}
	if len(commits) == 0 {
		log.Printf("no commits found on %s since %s", branchName, baseBranch)
		return nil
	}

	log.Printf("branch %s: %d commit(s) since %s", branchName, len(commits), baseBranch)

	mode := cfg.ReviewMode
	if mode == "" {
		mode = "full"
	}

	pr := PullRequest{
		Title:  fmt.Sprintf("branch: %s", branchName),
		Author: "local",
	}

	var result *ReviewResult
	var comment string

	switch mode {
	case "commits", "both":
		var commitResults []CommitReviewResult
		for i, commit := range commits {
			sha := commit.SHA
			if len(sha) > 8 {
				sha = sha[:8]
			}
			msg := commit.Message
			if idx := strings.IndexByte(msg, '\n'); idx > 0 {
				msg = msg[:idx]
			}
			log.Printf("  reviewing commit %d/%d: %s %s", i+1, len(commits), sha, msg)

			diff, err := commitDiff(cfg.RepoPath, commit.SHA)
			if err != nil {
				log.Printf("  skip commit %s: %v", sha, err)
				continue
			}
			if strings.TrimSpace(diff) == "" {
				continue
			}

			taggedDiff := fmt.Sprintf("Commit: %s — %s\n\n%s", sha, msg, diff)
			r, err := reviewer.Review(ctx, cfg.RepoPath, taggedDiff)
			if err != nil {
				log.Printf("  commit %s review failed: %v", sha, err)
				continue
			}
			commitResults = append(commitResults, CommitReviewResult{Commit: commit, Result: r})
		}

		if mode == "both" {
			var digest string
			if llmr, ok := reviewer.(*LLMReviewer); ok {
				digest, _ = digestFindings(ctx, llmr.LLM, llmr.Model, commitResults)
			} else {
				digest = digestFindingsPlain(commitResults)
			}

			diff, err := branchDiff(cfg.RepoPath, branchName, baseBranch)
			if err != nil {
				return fmt.Errorf("branch diff: %w", err)
			}
			branchResult, err := reviewer.ReviewWithContext(ctx, cfg.RepoPath, diff, digest)
			if err != nil {
				return fmt.Errorf("branch repass: %w", err)
			}

			merged := mergeCommitResults(commitResults)
			result = &ReviewResult{
				Findings: append(merged.Findings, branchResult.Findings...),
				Model:    merged.Model,
			}
			comment = FormatBothReviewComment(commitResults, branchResult, pr.Title, merged.Model)
		} else {
			result = mergeCommitResults(commitResults)
			comment = FormatCommitReviewComment(commitResults, pr.Title, result.Model)
		}

	default:
		diff, err := branchDiff(cfg.RepoPath, branchName, baseBranch)
		if err != nil {
			return fmt.Errorf("branch diff: %w", err)
		}
		result, err = reviewer.Review(ctx, cfg.RepoPath, diff)
		if err != nil {
			return fmt.Errorf("review: %w", err)
		}
		comment = FormatComment(result, pr.Title)
	}

	fmt.Printf("--- %s (%d finding(s)) ---\n%s\n", branchName, len(result.Findings), comment)
	return nil
}

func newReviewer(cfg Config) Reviewer {
	if cfg.ReviewCommand != "" {
		return &CommandReviewer{Command: cfg.ReviewCommand, Agent: cfg.ReviewAgent}
	}
	return &LLMReviewer{
		LLM:          agentcore.NewLLMClient(cfg.LLM.URL),
		Model:        cfg.LLM.Model,
		ContextSize:  cfg.LLM.ContextSize,
		TokenCeiling: cfg.LLM.TokenCeiling,
		Temperature:  cfg.LLM.Temperature,
	}
}

func run(ctx context.Context, cfg Config, state *State, singleMR int64) error {
	forge, err := NewForge(cfg)
	if err != nil {
		return fmt.Errorf("%s client: %w", forge.Name(), err)
	}

	log.Printf("using %s forge", forge.Name())
	reviewer := newReviewer(cfg)

	var prs []PullRequest
	if singleMR > 0 {
		pr, err := forge.Get(ctx, singleMR)
		if err != nil {
			return fmt.Errorf("get #%d: %w", singleMR, err)
		}
		prs = []PullRequest{pr}
	} else {
		all, err := forge.ListAll(ctx)
		if err != nil {
			return fmt.Errorf("list PRs: %w", err)
		}
		for _, pr := range all {
			if !state.IsReviewed(cfg.Project, pr.ID) {
				prs = append(prs, pr)
			}
		}
	}

	if len(prs) == 0 {
		log.Println("no unreviewed merge/pull requests found")
		return nil
	}

	log.Printf("reviewing %d request(s)", len(prs))

	for _, pr := range prs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		log.Printf("reviewing #%d: %s", pr.ID, pr.Title)

		worktreeDir, cleanup, err := CreateWorktree(ctx, cfg.RepoPath, pr.ID, forge.Name())
		if err != nil {
			log.Printf("skip #%d: worktree: %v", pr.ID, err)
			continue
		}

		var comment string
		var result *ReviewResult

		switch cfg.ReviewMode {
		case "both":
			commitResults, err := reviewByCommits(ctx, forge, reviewer, worktreeDir, pr)
			if err != nil {
				cleanup()
				log.Printf("skip #%d: commit review: %v", pr.ID, err)
				continue
			}

			var digest string
			if llmr, ok := reviewer.(*LLMReviewer); ok {
				digest, err = digestFindings(ctx, llmr.LLM, llmr.Model, commitResults)
				if err != nil {
					log.Printf("#%d: digest failed, using plain fallback: %v", pr.ID, err)
					digest = digestFindingsPlain(commitResults)
				}
			} else {
				digest = digestFindingsPlain(commitResults)
			}
			log.Printf("#%d: digest complete, starting branch repass", pr.ID)

			diff, err := forge.GetDiff(ctx, pr.ID)
			if err != nil {
				cleanup()
				log.Printf("skip #%d: get diff: %v", pr.ID, err)
				continue
			}
			branchResult, err := reviewer.ReviewWithContext(ctx, worktreeDir, diff, digest)
			cleanup()
			if err != nil {
				log.Printf("skip #%d: branch repass: %v", pr.ID, err)
				continue
			}

			merged := mergeCommitResults(commitResults)
			result = &ReviewResult{
				Findings: append(merged.Findings, branchResult.Findings...),
				Model:    merged.Model,
			}
			comment = FormatBothReviewComment(commitResults, branchResult, pr.Title, merged.Model)

		case "commits":
			commitResults, err := reviewByCommits(ctx, forge, reviewer, worktreeDir, pr)
			cleanup()
			if err != nil {
				log.Printf("skip #%d: review: %v", pr.ID, err)
				continue
			}
			result = mergeCommitResults(commitResults)
			comment = FormatCommitReviewComment(commitResults, pr.Title, result.Model)

		default:
			diff, err := forge.GetDiff(ctx, pr.ID)
			if err != nil {
				cleanup()
				log.Printf("skip #%d: get diff: %v", pr.ID, err)
				continue
			}
			result, err = reviewer.Review(ctx, worktreeDir, diff)
			cleanup()
			if err != nil {
				log.Printf("skip #%d: review: %v", pr.ID, err)
				continue
			}
			comment = FormatComment(result, pr.Title)
		}

		if cfg.InlineComments && len(result.Findings) > 0 {
			inlineComments, summaryFindings := routeFindings(result.Findings)
			if cfg.DryRun {
				fmt.Printf("--- #%d: %s ---\n", pr.ID, pr.Title)
				if len(inlineComments) > 0 {
					fmt.Printf("INLINE (%d):\n", len(inlineComments))
					for _, ic := range inlineComments {
						fmt.Printf("  %s:%d — %s\n", ic.File, ic.Line, ic.Body)
					}
				}
				if len(summaryFindings) > 0 {
					fmt.Printf("SUMMARY (%d):\n", len(summaryFindings))
					for _, f := range summaryFindings {
						fmt.Printf("  [%s] %s: %s\n", f.Severity, f.Category, f.Description)
					}
				}
				fmt.Println()
			} else {
				if len(inlineComments) > 0 {
					if err := forge.PostInlineComments(ctx, pr, inlineComments); err != nil {
						log.Printf("#%d: inline comments failed, falling back to summary: %v", pr.ID, err)
					} else {
						log.Printf("#%d: posted %d inline comment(s)", pr.ID, len(inlineComments))
					}
				}
				if err := forge.PostComment(ctx, pr.ID, comment); err != nil {
					log.Printf("skip #%d: post comment: %v", pr.ID, err)
					continue
				}
			}
		} else if cfg.DryRun {
			fmt.Printf("--- #%d: %s ---\n%s\n\n", pr.ID, pr.Title, comment)
		} else {
			if err := forge.PostComment(ctx, pr.ID, comment); err != nil {
				log.Printf("skip #%d: post comment: %v", pr.ID, err)
				continue
			}
		}

		state.MarkReviewed(cfg.Project, pr.ID)
		if err := state.Save(); err != nil {
			log.Printf("warning: save state: %v", err)
		}

		log.Printf("reviewed #%d: %d finding(s)", pr.ID, len(result.Findings))
	}

	return nil
}
