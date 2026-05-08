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
	project := flag.String("project", "", "GitLab project path (group/project)")
	mrIID := flag.Int64("mr", 0, "review a single merge request by IID")
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
	gl, err := NewGitLabClient(cfg)
	if err != nil {
		return fmt.Errorf("gitlab client: %w", err)
	}

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

	var mrs []MergeRequest
	if singleMR > 0 {
		mr, err := gl.GetMR(ctx, singleMR)
		if err != nil {
			return fmt.Errorf("get MR !%d: %w", singleMR, err)
		}
		mrs = []MergeRequest{mr}
	} else {
		mrs, err = gl.ListUnreviewedMRs(ctx, cfg.ReviewLabel)
		if err != nil {
			return fmt.Errorf("list MRs: %w", err)
		}
	}

	if len(mrs) == 0 {
		log.Println("no unreviewed merge requests found")
		return nil
	}

	log.Printf("reviewing %d merge request(s)", len(mrs))

	for _, mr := range mrs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		log.Printf("reviewing MR !%d: %s", mr.IID, mr.Title)

		diff, err := gl.GetMRDiff(ctx, mr.IID)
		if err != nil {
			log.Printf("skip MR !%d: get diff: %v", mr.IID, err)
			continue
		}

		worktreeDir, cleanup, err := CreateWorktree(ctx, cfg.RepoPath, mr.IID)
		if err != nil {
			log.Printf("skip MR !%d: worktree: %v", mr.IID, err)
			continue
		}

		result, err := reviewer.Review(ctx, worktreeDir, diff)
		cleanup()
		if err != nil {
			log.Printf("skip MR !%d: review: %v", mr.IID, err)
			continue
		}

		comment := FormatComment(result, mr.Title)

		if cfg.DryRun {
			fmt.Printf("--- MR !%d: %s ---\n%s\n\n", mr.IID, mr.Title, comment)
			continue
		}

		if err := gl.PostComment(ctx, mr.IID, comment); err != nil {
			log.Printf("skip MR !%d: post comment: %v", mr.IID, err)
			continue
		}

		if err := gl.AddLabel(ctx, mr.IID, cfg.ReviewLabel); err != nil {
			log.Printf("warning: MR !%d: add label: %v", mr.IID, err)
		}

		log.Printf("reviewed MR !%d: %d finding(s)", mr.IID, len(result.Findings))
	}

	return nil
}
