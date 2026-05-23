package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/kurze/lab/agentcore"
)

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: scrutineer <command> [flags]

Commands:
  review    Review merge/pull requests or local branches
  list      List merge/pull requests and their review status
  show      Display stored review findings
  post      Post stored review findings to the forge

Run 'scrutineer <command> -h' for command-specific help.
`)
	os.Exit(1)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}

	switch os.Args[1] {
	case "list":
		cmdList(os.Args[2:])
	case "review":
		cmdReview(os.Args[2:])
	case "show":
		cmdShow(os.Args[2:])
	case "post":
		cmdPost(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		usage()
	}
}

func cmdList(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	project := fs.String("project", "", "project path (owner/repo)")
	configPath := fs.String("config", "", "path to config file")
	repoPath := fs.String("repo", "", "path to local repo clone")
	filterFlag := fs.String("filter", "all", "filter: all, unreviewed, reviewed")
	fs.Parse(args)

	cfg := loadConfig(*configPath)
	if *project != "" {
		cfg.Project = *project
	}
	if *repoPath != "" {
		cfg.RepoPath = *repoPath
	}

	if err := cfg.ValidateForge(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	state, err := LoadState("")
	if err != nil {
		log.Fatalf("state: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	forge, err := NewForge(cfg)
	if err != nil {
		log.Fatalf("forge: %v", err)
	}

	prs, err := forge.ListAll(ctx)
	if err != nil {
		log.Fatalf("list: %v", err)
	}

	for _, pr := range prs {
		reviewed := state.IsReviewed(cfg.Project, pr.ID)

		switch *filterFlag {
		case "unreviewed":
			if reviewed {
				continue
			}
		case "reviewed":
			if !reviewed {
				continue
			}
		case "all":
		default:
			fmt.Fprintf(os.Stderr, "error: invalid filter %q (valid: all, unreviewed, reviewed)\n", *filterFlag)
			os.Exit(1)
		}

		status := "  "
		if reviewed {
			status = "R "
		}

		age := formatAge(pr.UpdatedAt)

		fmt.Printf("%s#%-6d %-15s %-6s %s\n", status, pr.ID, pr.Author, age, pr.Title)
	}
}

func cmdShow(args []string) {
	fs := flag.NewFlagSet("show", flag.ExitOnError)
	project := fs.String("project", "", "project path (owner/repo)")
	configPath := fs.String("config", "", "path to config file")
	mrFlag := fs.String("mr", "", "show results for MR ID(s) (comma-separated)")
	branch := fs.String("branch", "", "show results for a branch")
	commit := fs.String("commit", "", "show results for a commit SHA")
	fs.Parse(args)

	cfg := loadConfig(*configPath)
	if *project != "" {
		cfg.Project = *project
	}

	state, err := LoadState("")
	if err != nil {
		log.Fatalf("state: %v", err)
	}

	var keys []string
	if *mrFlag != "" {
		ids, err := parseMRIDs(*mrFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		for _, id := range ids {
			keys = append(keys, ResultKeyMR(id))
		}
	}
	if *branch != "" {
		keys = append(keys, ResultKeyBranch(*branch))
	}
	if *commit != "" {
		keys = append(keys, ResultKeyCommit(*commit))
	}

	if len(keys) == 0 {
		results := state.ListResults(cfg.Project)
		if len(results) == 0 {
			fmt.Println("no stored results")
			return
		}
		for _, r := range results {
			fmt.Printf("%-20s %-6s %d finding(s)  %s  %s\n",
				r.Key, r.Mode, len(r.Findings), formatAge(r.ReviewedAt), r.Title)
		}
		return
	}

	for _, key := range keys {
		r := state.GetResult(cfg.Project, key)
		if r == nil {
			fmt.Fprintf(os.Stderr, "no stored result for %s\n", key)
			continue
		}
		fmt.Printf("--- %s: %s (%d finding(s), mode: %s) ---\n", r.Key, r.Title, len(r.Findings), r.Mode)
		for _, f := range r.Findings {
			loc := ""
			if f.Location != "" {
				loc = " " + f.Location
			}
			fmt.Printf("  [%s] %s%s — %s\n", f.Severity, f.Category, loc, f.Description)
		}
		fmt.Println()
	}
}

func cmdPost(args []string) {
	fs := flag.NewFlagSet("post", flag.ExitOnError)
	project := fs.String("project", "", "project path (owner/repo)")
	configPath := fs.String("config", "", "path to config file")
	repoPath := fs.String("repo", "", "path to local repo clone")
	mrFlag := fs.String("mr", "", "post results for MR ID(s) (comma-separated)")
	all := fs.Bool("all", false, "post all stored MR results")
	comments := fs.String("comments", "", "comment style: summary, inline, or both")
	fs.Parse(args)

	cfg := loadConfig(*configPath)
	if *project != "" {
		cfg.Project = *project
	}
	if *repoPath != "" {
		cfg.RepoPath = *repoPath
	}
	if *comments != "" {
		cfg.CommentStyle = *comments
	}

	if err := cfg.ValidateForge(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	state, err := LoadState("")
	if err != nil {
		log.Fatalf("state: %v", err)
	}

	var keys []string
	if *mrFlag != "" {
		ids, err := parseMRIDs(*mrFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		for _, id := range ids {
			keys = append(keys, ResultKeyMR(id))
		}
	} else if *all {
		for _, r := range state.ListResults(cfg.Project) {
			if strings.HasPrefix(r.Key, "mr:") {
				keys = append(keys, r.Key)
			}
		}
	}

	if len(keys) == 0 {
		fmt.Fprintf(os.Stderr, "nothing to post: specify --mr or --all\n")
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	forge, err := NewForge(cfg)
	if err != nil {
		log.Fatalf("forge: %v", err)
	}

	style := cfg.CommentStyle
	if style == "" {
		style = "both"
	}

	for _, key := range keys {
		sr := state.GetResult(cfg.Project, key)
		if sr == nil {
			fmt.Fprintf(os.Stderr, "no stored result for %s\n", key)
			continue
		}

		idStr := strings.TrimPrefix(key, "mr:")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid key %s\n", key)
			continue
		}

		pr, err := forge.Get(ctx, id)
		if err != nil {
			log.Printf("skip %s: get PR: %v", key, err)
			continue
		}

		result := &ReviewResult{Findings: sr.Findings, Model: sr.Model}
		comment := FormatComment(result, pr.Title)

		postInline := (style == "inline" || style == "both") && len(result.Findings) > 0
		postSummary := style == "summary" || style == "both"

		if postInline {
			inlineComments, _ := routeFindings(result.Findings, cfg.InlineSeverity)
			if len(inlineComments) > 0 {
				if err := forge.PostInlineComments(ctx, pr, inlineComments); err != nil {
					log.Printf("#%d: inline comments failed: %v", id, err)
				} else {
					log.Printf("#%d: posted %d inline comment(s)", id, len(inlineComments))
				}
			}
		}
		if postSummary {
			if err := forge.PostComment(ctx, id, comment); err != nil {
				log.Printf("skip #%d: post comment: %v", id, err)
				continue
			}
		}

		log.Printf("posted #%d: %d finding(s)", id, len(result.Findings))
	}
}

func formatAge(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func cmdReview(args []string) {
	fs := flag.NewFlagSet("review", flag.ExitOnError)
	project := fs.String("project", "", "project path (owner/repo)")
	mrFlag := fs.String("mr", "", "merge/pull request IDs (comma-separated, e.g. 1,2,5)")
	post := fs.Bool("post", false, "post findings as comments (default: dry-run)")
	configPath := fs.String("config", "", "path to config file")
	repoPath := fs.String("repo", "", "path to local repo clone")
	batch := fs.Bool("batch", false, "batch mode: review all unreviewed MRs")
	mode := fs.String("mode", "", "review mode: full, commits, or both")
	comments := fs.String("comments", "", "comment style: summary, inline, or both")
	branch := fs.String("branch", "", "review a local branch (commits since base branch)")
	commitSHA := fs.String("commit", "", "review a single commit by SHA")
	fs.Parse(args)

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
	if *comments != "" {
		cfg.CommentStyle = *comments
	}

	state, err := LoadState("")
	if err != nil {
		log.Fatalf("state: %v", err)
	}

	if *branch != "" {
		if cfg.ReviewCommand == "" && cfg.LLM.URL == "" {
			fmt.Fprintf(os.Stderr, "error: review engine required: set review_command or [llm] url in config\n")
			os.Exit(1)
		}
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()
		if err := runBranch(ctx, cfg, state, *branch); err != nil {
			log.Fatalf("error: %v", err)
		}
		return
	}

	if *commitSHA != "" {
		if cfg.ReviewCommand == "" && cfg.LLM.URL == "" {
			fmt.Fprintf(os.Stderr, "error: review engine required: set review_command or [llm] url in config\n")
			os.Exit(1)
		}
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()
		if err := runCommit(ctx, cfg, state, *commitSHA); err != nil {
			log.Fatalf("error: %v", err)
		}
		return
	}

	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	targets, err := parseMRTargets(*mrFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if len(targets) > 0 || *batch {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()
		if err := run(ctx, cfg, state, targets); err != nil {
			log.Fatalf("error: %v", err)
		}
		return
	}

	fmt.Fprintf(os.Stderr, "Usage: scrutineer review [flags]\n\nFlags:\n")
	fs.PrintDefaults()
	os.Exit(1)
}

func runBranch(ctx context.Context, cfg Config, state *State, branchName string) error {
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
		commitResults, err := reviewByCommits(ctx, reviewer, cfg.RepoPath, commits, &ReviewByCommitsOpts{
			OnProgress:  cliProgress(""),
			Concurrency: cfg.CommitConcurrency(),
		})
		if err != nil {
			return fmt.Errorf("commit review: %w", err)
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
		result, err = reviewer.ReviewFull(ctx, cfg.RepoPath, diff)
		if err != nil {
			return fmt.Errorf("review: %w", err)
		}
		comment = FormatComment(result, pr.Title)
	}

	state.StoreResult(cfg.Project, &StoredResult{
		Key:        ResultKeyBranch(branchName),
		Title:      pr.Title,
		Mode:       mode,
		Findings:   result.Findings,
		Model:      result.Model,
		ReviewedAt: time.Now(),
	})
	if err := state.Save(); err != nil {
		log.Printf("warning: save state: %v", err)
	}

	fmt.Printf("--- %s (%d finding(s)) ---\n%s\n", branchName, len(result.Findings), comment)

	if !cfg.DryRun && len(result.Findings) > 0 {
		if err := cfg.ValidateForge(); err == nil {
			forge, fErr := NewForge(cfg)
			if fErr == nil {
				posted := 0
				for _, f := range result.Findings {
					if f.CommitSHA == "" {
						continue
					}
					file, line, ok := parseLocation(f.Location)
					if !ok {
						continue
					}
					sha := findFullSHA(commits, f.CommitSHA)
					if sha == "" {
						continue
					}
					body := formatInlineBody(f)
					if err := forge.PostCommitComment(ctx, sha, file, line, body); err != nil {
						log.Printf("commit comment on %s %s:%d failed: %v", f.CommitSHA, file, line, err)
					} else {
						posted++
					}
				}
				if posted > 0 {
					log.Printf("posted %d commit comment(s) on branch %s", posted, branchName)
				}
			}
		}
	}

	return nil
}

func runCommit(ctx context.Context, cfg Config, state *State, sha string) error {
	reviewer := newReviewer(cfg)

	diff, err := commitDiff(cfg.RepoPath, sha)
	if err != nil {
		return fmt.Errorf("git show %s: %w", sha, err)
	}
	if strings.TrimSpace(diff) == "" {
		log.Printf("commit %s has no diff", sha)
		return nil
	}

	shortSHA := sha
	if len(shortSHA) > 8 {
		shortSHA = shortSHA[:8]
	}

	fmt.Fprintf(os.Stderr, "reviewing commit %s...\n", shortSHA)

	taggedDiff := fmt.Sprintf("Commit: %s\n\n%s", shortSHA, diff)
	result, err := reviewer.Review(ctx, cfg.RepoPath, taggedDiff)
	if err != nil {
		return fmt.Errorf("review: %w", err)
	}

	state.StoreResult(cfg.Project, &StoredResult{
		Key:        ResultKeyCommit(sha),
		Title:      fmt.Sprintf("commit %s", shortSHA),
		Mode:       "full",
		Findings:   result.Findings,
		Model:      result.Model,
		ReviewedAt: time.Now(),
	})
	if err := state.Save(); err != nil {
		log.Printf("warning: save state: %v", err)
	}

	comment := FormatComment(result, fmt.Sprintf("commit %s", shortSHA))
	fmt.Printf("--- commit %s (%d finding(s)) ---\n%s\n", shortSHA, len(result.Findings), comment)

	if !cfg.DryRun && len(result.Findings) > 0 {
		forge, fErr := NewForge(cfg)
		if fErr != nil {
			return fmt.Errorf("forge: %w", fErr)
		}
		inlineComments, _ := routeFindings(result.Findings, cfg.InlineSeverity)
		for _, ic := range inlineComments {
			if err := forge.PostCommitComment(ctx, sha, ic.File, ic.Line, ic.Body); err != nil {
				log.Printf("commit comment on %s:%d failed: %v", ic.File, ic.Line, err)
			}
		}
		log.Printf("posted %d commit comment(s) on %s", len(inlineComments), shortSHA)
	}

	return nil
}

func findFullSHA(commits []Commit, short string) string {
	for _, c := range commits {
		if strings.HasPrefix(c.SHA, short) {
			return c.SHA
		}
	}
	return ""
}

func cliProgress(prefix string) ProgressFunc {
	return func(ev CommitProgressEvent) {
		switch ev.Status {
		case CommitStarted:
			fmt.Fprintf(os.Stderr, "%s  ⟳ commit %d/%d: %s %s\n", prefix, ev.Index, ev.Total, ev.SHA, ev.Message)
		case CommitDone:
			fmt.Fprintf(os.Stderr, "%s  ✓ commit %d/%d: %s %s\n", prefix, ev.Index, ev.Total, ev.SHA, ev.Message)
		case CommitFailed:
			fmt.Fprintf(os.Stderr, "%s  ✗ commit %d/%d: %s %s — %v\n", prefix, ev.Index, ev.Total, ev.SHA, ev.Message, ev.Err)
		}
	}
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

type MRTarget struct {
	ID   int64
	Mode string
}

func parseMRTargets(s string) ([]MRTarget, error) {
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, ",")
	targets := make([]MRTarget, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		idStr, mode, _ := strings.Cut(p, ":")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid MR ID %q: %w", idStr, err)
		}
		if id <= 0 {
			return nil, fmt.Errorf("invalid MR ID %d: must be positive", id)
		}
		if mode != "" {
			switch mode {
			case "full", "commits", "both":
			default:
				return nil, fmt.Errorf("invalid mode %q for MR %d (valid: full, commits, both)", mode, id)
			}
		}
		targets = append(targets, MRTarget{ID: id, Mode: mode})
	}
	return targets, nil
}

func parseMRIDs(s string) ([]int64, error) {
	targets, err := parseMRTargets(s)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return nil, nil
	}
	ids := make([]int64, len(targets))
	for i, t := range targets {
		ids[i] = t.ID
	}
	return ids, nil
}

func run(ctx context.Context, cfg Config, state *State, targets []MRTarget) error {
	forge, err := NewForge(cfg)
	if err != nil {
		return fmt.Errorf("%s forge client: %w", cfg.ForgeType, err)
	}

	log.Printf("using %s forge", forge.Name())
	reviewer := newReviewer(cfg)

	type prWithMode struct {
		pr   PullRequest
		mode string
	}

	var prs []prWithMode
	if len(targets) > 0 {
		for _, t := range targets {
			pr, err := forge.Get(ctx, t.ID)
			if err != nil {
				return fmt.Errorf("get #%d: %w", t.ID, err)
			}
			prs = append(prs, prWithMode{pr: pr, mode: t.Mode})
		}
	} else {
		all, err := forge.ListAll(ctx)
		if err != nil {
			return fmt.Errorf("list PRs: %w", err)
		}
		for _, pr := range all {
			if !state.IsReviewed(cfg.Project, pr.ID) {
				prs = append(prs, prWithMode{pr: pr})
			}
		}
	}

	if len(prs) == 0 {
		log.Println("no unreviewed merge/pull requests found")
		return nil
	}

	log.Printf("reviewing %d request(s)", len(prs))

	for _, pwm := range prs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		pr := pwm.pr
		reviewMode := pwm.mode
		if reviewMode == "" {
			reviewMode = cfg.ReviewMode
		}

		log.Printf("reviewing #%d: %s (mode: %s)", pr.ID, pr.Title, reviewMode)

		worktreeDir, cleanup, err := CreateWorktree(ctx, cfg.RepoPath, pr.ID, forge.Name())
		if err != nil {
			log.Printf("skip #%d: worktree: %v", pr.ID, err)
			continue
		}

		var comment string
		var result *ReviewResult

		switch reviewMode {
		case "both":
			commits, err := forge.ListCommits(ctx, pr.ID)
			if err != nil {
				cleanup()
				log.Printf("skip #%d: list commits: %v", pr.ID, err)
				continue
			}
			commitResults, err := reviewByCommits(ctx, reviewer, worktreeDir, commits, &ReviewByCommitsOpts{State: state, Project: cfg.Project, OnProgress: cliProgress(fmt.Sprintf("#%d", pr.ID)), Concurrency: cfg.CommitConcurrency()})
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
			commits, err := forge.ListCommits(ctx, pr.ID)
			if err != nil {
				cleanup()
				log.Printf("skip #%d: list commits: %v", pr.ID, err)
				continue
			}
			commitResults, err := reviewByCommits(ctx, reviewer, worktreeDir, commits, &ReviewByCommitsOpts{State: state, Project: cfg.Project, OnProgress: cliProgress(fmt.Sprintf("#%d", pr.ID)), Concurrency: cfg.CommitConcurrency()})
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
			fmt.Fprintf(os.Stderr, "#%d  reviewing full diff...\n", pr.ID)
			result, err = reviewer.ReviewFull(ctx, worktreeDir, diff)
			cleanup()
			if err != nil {
				log.Printf("skip #%d: review: %v", pr.ID, err)
				continue
			}
			comment = FormatComment(result, pr.Title)
		}

		style := cfg.CommentStyle
		if style == "" {
			style = "both"
		}

		postInline := (style == "inline" || style == "both") && len(result.Findings) > 0
		postSummary := style == "summary" || style == "both"

		if cfg.DryRun {
			fmt.Printf("--- #%d: %s ---\n", pr.ID, pr.Title)
			if postInline {
				inlineComments, summaryFindings := routeFindings(result.Findings, cfg.InlineSeverity)
				if len(inlineComments) > 0 {
					fmt.Printf("INLINE (%d):\n", len(inlineComments))
					for _, ic := range inlineComments {
						fmt.Printf("  %s:%d — %s\n", ic.File, ic.Line, ic.Body)
					}
				}
				if len(summaryFindings) > 0 {
					fmt.Printf("SUMMARY-ONLY (%d):\n", len(summaryFindings))
					for _, f := range summaryFindings {
						fmt.Printf("  [%s] %s: %s\n", f.Severity, f.Category, f.Description)
					}
				}
			}
			if postSummary {
				fmt.Printf("SUMMARY:\n%s", comment)
			}
			fmt.Println()
		} else {
			if postInline {
				inlineComments, _ := routeFindings(result.Findings, cfg.InlineSeverity)
				if len(inlineComments) > 0 {
					if err := forge.PostInlineComments(ctx, pr, inlineComments); err != nil {
						log.Printf("#%d: inline comments failed: %v", pr.ID, err)
					} else {
						log.Printf("#%d: posted %d inline comment(s)", pr.ID, len(inlineComments))
					}
				}
			}
			if postSummary {
				if err := forge.PostComment(ctx, pr.ID, comment); err != nil {
					log.Printf("skip #%d: post comment: %v", pr.ID, err)
					continue
				}
			}
		}

		state.StoreResult(cfg.Project, &StoredResult{
			Key:        ResultKeyMR(pr.ID),
			Title:      pr.Title,
			Mode:       reviewMode,
			Findings:   result.Findings,
			Model:      result.Model,
			ReviewedAt: time.Now(),
		})
		state.MarkReviewed(cfg.Project, pr.ID)
		if err := state.Save(); err != nil {
			log.Printf("warning: save state: %v", err)
		}

		log.Printf("reviewed #%d: %d finding(s)", pr.ID, len(result.Findings))
	}

	return nil
}
