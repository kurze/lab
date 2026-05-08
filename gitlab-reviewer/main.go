package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/kurze/lab/agentcore"
)

func main() {
	project := flag.String("project", "", "project path (owner/repo)")
	mrIID := flag.Int64("mr", 0, "review a single merge/pull request by number")
	post := flag.Bool("post", false, "post findings as comments and add label (default: dry-run)")
	configPath := flag.String("config", "", "path to config file")
	repoPath := flag.String("repo", "", "path to local repo clone")
	flag.Parse()

	cfg := loadConfig(*configPath)
	if *project != "" {
		cfg.Project = *project
	}
	if *repoPath != "" {
		cfg.RepoPath = *repoPath
	}
	cfg.DryRun = !*post

	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := run(ctx, cfg, *mrIID); err != nil {
		log.Fatalf("error: %v", err)
	}
}

func run(ctx context.Context, cfg Config, singleMR int64) error {
	forge, err := NewForge(cfg)
	if err != nil {
		return fmt.Errorf("%s client: %w", forge.Name(), err)
	}

	log.Printf("using %s forge", forge.Name())

	var reviewer Reviewer
	if cfg.ReviewCommand != "" {
		reviewer = &CommandReviewer{Command: cfg.ReviewCommand, Agent: cfg.ReviewAgent}
	} else {
		reviewer = &LLMReviewer{
			LLM:          agentcore.NewLLMClient(cfg.LLM.URL),
			Model:        cfg.LLM.Model,
			ContextSize:  cfg.LLM.ContextSize,
			TokenCeiling: cfg.LLM.TokenCeiling,
			Temperature:  cfg.LLM.Temperature,
		}
	}

	var prs []PullRequest
	if singleMR > 0 {
		pr, err := forge.Get(ctx, singleMR)
		if err != nil {
			return fmt.Errorf("get #%d: %w", singleMR, err)
		}
		prs = []PullRequest{pr}
	} else {
		prs, err = forge.ListUnreviewed(ctx, cfg.ReviewLabel)
		if err != nil {
			return fmt.Errorf("list PRs: %w", err)
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

		diff, err := forge.GetDiff(ctx, pr.ID)
		if err != nil {
			log.Printf("skip #%d: get diff: %v", pr.ID, err)
			continue
		}

		worktreeDir, cleanup, err := CreateWorktree(ctx, cfg.RepoPath, pr.ID, forge.Name())
		if err != nil {
			log.Printf("skip #%d: worktree: %v", pr.ID, err)
			continue
		}

		result, err := reviewer.Review(ctx, worktreeDir, diff)
		cleanup()
		if err != nil {
			log.Printf("skip #%d: review: %v", pr.ID, err)
			continue
		}

		comment := FormatComment(result, pr.Title)

		if cfg.DryRun {
			fmt.Printf("--- #%d: %s ---\n%s\n\n", pr.ID, pr.Title, comment)
			continue
		}

		if err := forge.PostComment(ctx, pr.ID, comment); err != nil {
			log.Printf("skip #%d: post comment: %v", pr.ID, err)
			continue
		}

		if err := forge.AddLabel(ctx, pr.ID, cfg.ReviewLabel); err != nil {
			log.Printf("warning: #%d: add label: %v", pr.ID, err)
		}

		log.Printf("reviewed #%d: %d finding(s)", pr.ID, len(result.Findings))
	}

	return nil
}
