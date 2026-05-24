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
  review      Review merge/pull requests or local branches
  list        List merge/pull requests and their review status
  show        Display stored review findings
  post        Post stored review findings to the forge
  fix         Generate fixup commits from stored review findings
  logs        Browse and manage LLM exchange traces
  completion  Generate shell completion scripts (bash, zsh, fish)

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
	case "fix":
		cmdFix(os.Args[2:])
	case "logs":
		cmdLogs(os.Args[2:])
	case "completion":
		cmdCompletion(os.Args[2:])
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
	formatFlag := fs.String("format", "", "output format (ids: one ID per line, for shell completion)")
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

	if *formatFlag != "" && *formatFlag != "ids" {
		fmt.Fprintf(os.Stderr, "error: invalid format %q (valid: ids)\n", *formatFlag)
		os.Exit(1)
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

		if *formatFlag == "ids" {
			fmt.Println(pr.ID)
			continue
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
			count := fmt.Sprintf("%d finding(s)", len(r.Findings))
			if r.RawOutput != "" {
				count = "raw output"
			}
			fmt.Printf("%-20s %-6s %-14s %s  %s\n",
				r.Key, r.Mode, count, formatAge(r.ReviewedAt), r.Title)
		}
		return
	}

	for _, key := range keys {
		r := state.GetResult(cfg.Project, key)
		if r == nil {
			fmt.Fprintf(os.Stderr, "no stored result for %s\n", key)
			continue
		}
		if r.RawOutput != "" {
			fmt.Printf("--- %s: %s (raw output, mode: %s) ---\n%s\n\n", r.Key, r.Title, r.Mode, r.RawOutput)
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
	branch := fs.String("branch", "", "post results for a branch")
	commit := fs.String("commit", "", "post results for a commit SHA")
	all := fs.Bool("all", false, "post all stored results")
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
	}
	if *branch != "" {
		keys = append(keys, ResultKeyBranch(*branch))
	}
	if *commit != "" {
		keys = append(keys, ResultKeyCommit(*commit))
	}
	if *all && len(keys) == 0 {
		for _, r := range state.ListResults(cfg.Project) {
			keys = append(keys, r.Key)
		}
	}

	if len(keys) == 0 {
		fmt.Fprintf(os.Stderr, "nothing to post: specify --mr, --branch, --commit, or --all\n")
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

		switch {
		case strings.HasPrefix(key, "mr:"):
			postMRResult(ctx, forge, sr, key, style, cfg)
		case strings.HasPrefix(key, "branch:"):
			postBranchResult(ctx, forge, sr, key, cfg)
		case strings.HasPrefix(key, "commit:"):
			postCommitResult(ctx, forge, sr, key, cfg)
		default:
			fmt.Fprintf(os.Stderr, "unknown key type: %s\n", key)
		}
	}
}

func cmdFix(args []string) {
	fs := flag.NewFlagSet("fix", flag.ExitOnError)
	project := fs.String("project", "", "project path (owner/repo)")
	configPath := fs.String("config", "", "path to config file")
	repoPath := fs.String("repo", "", "path to local repo clone")
	mrFlag := fs.String("mr", "", "fix findings for MR ID(s) (comma-separated)")
	branch := fs.String("branch", "", "fix findings for a branch")
	commit := fs.String("commit", "", "fix findings for a commit SHA")
	dryRun := fs.Bool("dry-run", false, "preview patches without committing")
	model := fs.String("model", "", "LLM model (overrides config)")
	fs.Parse(args)

	cfg := loadConfig(*configPath)
	if *project != "" {
		cfg.Project = *project
	}
	if *repoPath != "" {
		cfg.RepoPath = *repoPath
	}
	if *model != "" {
		cfg.LLM.Model = *model
	}

	if err := validateReviewEngine(cfg); err != nil {
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
	}
	if *branch != "" {
		keys = append(keys, ResultKeyBranch(*branch))
	}
	if *commit != "" {
		keys = append(keys, ResultKeyCommit(*commit))
	}

	if len(keys) == 0 {
		fmt.Fprintf(os.Stderr, "nothing to fix: specify --mr, --branch, or --commit\n")
		os.Exit(1)
	}

	reviewer := newReviewer(cfg)
	llmr, ok := reviewer.(*LLMReviewer)
	if !ok {
		log.Fatal("fix subcommand requires builtin agent")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	for _, key := range keys {
		sr := state.GetResult(cfg.Project, key)
		if sr == nil {
			warnf("no stored result for %s", key)
			continue
		}
		if len(sr.Findings) == 0 && sr.RawOutput == "" {
			logf("%s: no findings to fix", key)
			continue
		}
		if sr.RawOutput != "" {
			warnf("%s: raw output results cannot be fixed (requires structured findings)", key)
			continue
		}

		logf("fixing %s: %s", cl(ansiBold, key), sr.Title)
		results, err := generateFixes(ctx, llmr.LLM, llmr.Model, sr.Findings, FixOptions{
			DryRun:    *dryRun,
			Threshold: cfg.FixThreshold,
			WorkDir:   cfg.RepoPath,
		})
		if err != nil {
			warnf("%s: fix generation failed: %v", key, err)
			continue
		}
		printFixResults(results)
	}
}

func postMRResult(ctx context.Context, forge Forge, sr *StoredResult, key, style string, cfg Config) {
	idStr := strings.TrimPrefix(key, "mr:")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid key %s\n", key)
		return
	}

	pr, err := forge.Get(ctx, id)
	if err != nil {
		warnf("skip %s: get PR: %v", key, err)
		return
	}

	result := &ReviewResult{Findings: sr.Findings, Model: sr.Model}
	comment := FormatComment(result, pr.Title)

	postInline := (style == "inline" || style == "both") && len(result.Findings) > 0
	postSummary := style == "summary" || style == "both"

	if postInline {
		inlineComments, _ := routeFindings(result.Findings, cfg.InlineSeverity)
		if len(inlineComments) > 0 {
			if err := forge.PostInlineComments(ctx, pr, inlineComments); err != nil {
				errf("#%d: inline comments failed: %v", id, err)
			} else {
				logf("%s posted %s inline comment(s)", cl(ansiBold, fmt.Sprintf("#%d", id)), cl(ansiGreen, fmt.Sprintf("%d", len(inlineComments))))
			}
		}
	}
	if postSummary {
		if err := forge.PostComment(ctx, id, comment); err != nil {
			errf("skip #%d: post comment: %v", id, err)
			return
		}
	}

	logf("%s %s %d finding(s)", cl(ansiBold, fmt.Sprintf("#%d", id)), cl(ansiGreen, "✓"), len(result.Findings))
}

func postBranchResult(ctx context.Context, forge Forge, sr *StoredResult, key string, cfg Config) {
	branchName := strings.TrimPrefix(key, "branch:")

	if len(sr.Findings) == 0 {
		logf("%s: no findings to post", key)
		return
	}

	baseBranch := detectBaseBranch(cfg.RepoPath)
	commits, err := branchCommits(cfg.RepoPath, branchName, baseBranch)
	if err != nil {
		warnf("skip %s: list commits: %v", key, err)
		return
	}

	posted := postFindingsToCommits(ctx, forge, sr.Findings, commits, branchName, cfg)
	logf("%s %s %d commit comment(s)", cl(ansiBold, branchName), cl(ansiGreen, "posted"), posted)
}

func postFindingsToCommits(ctx context.Context, forge Forge, findings []Finding, commits []Commit, label string, cfg Config) int {
	shaMap := make(map[string]string, len(commits))
	for _, c := range commits {
		shaMap[c.SHA] = c.SHA
		if len(c.SHA) >= 8 {
			shaMap[c.SHA[:8]] = c.SHA
		}
	}

	minSeverity := cfg.InlineSeverity
	if minSeverity == "" {
		minSeverity = "minor"
	}
	minRank := severityRank[strings.ToLower(minSeverity)]

	posted := 0
	for _, f := range findings {
		if f.CommitSHA == "" {
			continue
		}
		if severityRank[strings.ToLower(f.Severity)] < minRank {
			continue
		}
		file, line, ok := parseLocation(f.Location)
		if !ok {
			continue
		}
		sha, ok := shaMap[f.CommitSHA]
		if !ok {
			warnf("commit %s not found on %s, skipping", f.CommitSHA, label)
			continue
		}
		body := formatInlineBody(f)
		if err := forge.PostCommitComment(ctx, sha, file, line, body); err != nil {
			warnf("commit comment on %s %s:%d failed: %v", f.CommitSHA, file, line, err)
		} else {
			posted++
		}
	}
	return posted
}

func postCommitResult(ctx context.Context, forge Forge, sr *StoredResult, key string, cfg Config) {
	sha := strings.TrimPrefix(key, "commit:")
	shortSHA := sha
	if len(shortSHA) > 8 {
		shortSHA = shortSHA[:8]
	}

	if len(sr.Findings) == 0 {
		logf("%s: no findings to post", key)
		return
	}

	inlineComments, _ := routeFindings(sr.Findings, cfg.InlineSeverity)
	posted := 0
	for _, ic := range inlineComments {
		if err := forge.PostCommitComment(ctx, sha, ic.File, ic.Line, ic.Body); err != nil {
			warnf("commit comment on %s:%d failed: %v", ic.File, ic.Line, err)
		} else {
			posted++
		}
	}
	logf("%s %s %d commit comment(s)", cl(ansiBold, shortSHA), cl(ansiGreen, "posted"), posted)
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
	agent := fs.String("agent", "", "review agent: builtin, claude, codex, gemini, vibe, opencode, pi, custom")
	model := fs.String("model", "", "LLM model (overrides config)")
	verbose := fs.Bool("verbose", false, "print LLM exchanges to stderr in real time")
	fix := fs.Bool("fix", false, "generate fixup commits for qualifying findings after review")
	fixDryRun := fs.Bool("fix-dry-run", false, "preview generated patches without committing")
	fs.Parse(args)

	cfg := loadConfig(*configPath)
	if *agent != "" {
		cfg.Agent.Name = *agent
	}
	if *model != "" {
		cfg.LLM.Model = *model
	}
	if *project != "" {
		cfg.Project = *project
	}
	if *repoPath != "" {
		cfg.RepoPath = *repoPath
	}
	cfg.DryRun = !*post
	cfg.Verbose = *verbose
	cfg.Fix = *fix
	cfg.FixDryRun = *fixDryRun
	if *mode != "" {
		cfg.ReviewMode = *mode
	}
	if *comments != "" {
		cfg.CommentStyle = *comments
	}

	go pruneTraces(resolveTracesDir(cfg), cfg.LogMaxAgeDays, cfg.LogMaxSizeMB)

	state, err := LoadState("")
	if err != nil {
		log.Fatalf("state: %v", err)
	}

	if *branch != "" {
		if err := validateReviewEngine(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
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
		if err := validateReviewEngine(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
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
		logf("no commits found on %s since %s", branchName, baseBranch)
		return nil
	}

	logf("branch %s: %d commit(s) since %s", cl(ansiBold, branchName), len(commits), baseBranch)

	mode := cfg.ReviewMode
	if mode == "" {
		mode = "full"
	}

	setReviewerMeta(reviewer, map[string]string{
		"branch":  branchName,
		"project": cfg.Project,
		"mode":    mode,
	})

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
				digest, _ = digestFindings(ctx, llmr.LLM, llmr.Model, commitResults, nil)
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
		RawOutput:  result.RawOutput,
		Model:      result.Model,
		ReviewedAt: time.Now(),
	})
	if err := state.Save(); err != nil {
		warnf("save state: %v", err)
	}

	if result.RawOutput != "" {
		fmt.Printf("--- %s (raw output) ---\n%s\n", branchName, result.RawOutput)
	} else {
		fmt.Printf("--- %s (%d finding(s)) ---\n%s\n", branchName, len(result.Findings), comment)
	}

	if result.RawOutput == "" {
		tryGenerateFixes(ctx, cfg, reviewer, result.Findings, cfg.RepoPath, branchName)
	}

	if !cfg.DryRun && result.RawOutput == "" && len(result.Findings) > 0 {
		if err := cfg.ValidateForge(); err == nil {
			forge, fErr := NewForge(cfg)
			if fErr == nil {
				shaMap := make(map[string]string, len(commits))
				for _, c := range commits {
					shaMap[c.SHA] = c.SHA
					if len(c.SHA) >= 8 {
						shaMap[c.SHA[:8]] = c.SHA
					}
				}

				minSeverity := cfg.InlineSeverity
				if minSeverity == "" {
					minSeverity = "minor"
				}
				minRank := severityRank[strings.ToLower(minSeverity)]

				posted := 0
				for _, f := range result.Findings {
					if f.CommitSHA == "" {
						continue
					}
					if severityRank[strings.ToLower(f.Severity)] < minRank {
						continue
					}
					file, line, ok := parseLocation(f.Location)
					if !ok {
						continue
					}
					sha, ok := shaMap[f.CommitSHA]
					if !ok {
						continue
					}
					body := formatInlineBody(f)
					if err := forge.PostCommitComment(ctx, sha, file, line, body); err != nil {
						warnf("commit comment on %s %s:%d failed: %v", f.CommitSHA, file, line, err)
					} else {
						posted++
					}
				}
				if posted > 0 {
					logf("posted %d commit comment(s) on branch %s", posted, branchName)
				}
			}
		}
	}

	return nil
}

func runCommit(ctx context.Context, cfg Config, state *State, sha string) error {
	reviewer := newReviewer(cfg)
	setReviewerMeta(reviewer, map[string]string{
		"commit":  sha,
		"project": cfg.Project,
	})

	diff, err := commitDiff(cfg.RepoPath, sha)
	if err != nil {
		return fmt.Errorf("git show %s: %w", sha, err)
	}
	if strings.TrimSpace(diff) == "" {
		logf("commit %s has no diff", sha)
		return nil
	}

	shortSHA := sha
	if len(shortSHA) > 8 {
		shortSHA = shortSHA[:8]
	}

	logf("reviewing commit %s...", cl(ansiBold, shortSHA))

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
		RawOutput:  result.RawOutput,
		Model:      result.Model,
		ReviewedAt: time.Now(),
	})
	if err := state.Save(); err != nil {
		warnf("save state: %v", err)
	}

	if result.RawOutput != "" {
		fmt.Printf("--- commit %s (raw output) ---\n%s\n", shortSHA, result.RawOutput)
	} else {
		comment := FormatComment(result, fmt.Sprintf("commit %s", shortSHA))
		fmt.Printf("--- commit %s (%d finding(s)) ---\n%s\n", shortSHA, len(result.Findings), comment)
	}

	if result.RawOutput == "" {
		tryGenerateFixes(ctx, cfg, reviewer, result.Findings, cfg.RepoPath, shortSHA)
	}

	if !cfg.DryRun && result.RawOutput == "" && len(result.Findings) > 0 {
		forge, fErr := NewForge(cfg)
		if fErr != nil {
			return fmt.Errorf("forge: %w", fErr)
		}
		inlineComments, _ := routeFindings(result.Findings, cfg.InlineSeverity)
		for _, ic := range inlineComments {
			if err := forge.PostCommitComment(ctx, sha, ic.File, ic.Line, ic.Body); err != nil {
				warnf("commit comment on %s:%d failed: %v", ic.File, ic.Line, err)
			}
		}
		logf("posted %d commit comment(s) on %s", len(inlineComments), shortSHA)
	}

	return nil
}

func cliProgress(prefix string) ProgressFunc {
	return func(ev CommitProgressEvent) {
		p := ""
		if prefix != "" {
			p = cl(ansiBold, prefix) + " "
		}
		sha := cl(ansiDim, ev.SHA)
		switch ev.Status {
		case CommitStarted:
			logf("%s%s commit %d/%d: %s %s", p, cl(ansiCyan, "⟳"), ev.Index, ev.Total, sha, ev.Message)
		case CommitDone:
			logf("%s%s commit %d/%d: %s %s", p, cl(ansiGreen, "✓"), ev.Index, ev.Total, sha, ev.Message)
		case CommitFailed:
			logf("%s%s commit %d/%d: %s %s — %v", p, cl(ansiRed, "✗"), ev.Index, ev.Total, sha, ev.Message, ev.Err)
		}
	}
}

func validateReviewEngine(cfg Config) error {
	agent := resolveAgent(cfg)
	switch agent {
	case "builtin":
		if cfg.LLM.URL == "" {
			return fmt.Errorf("builtin agent requires [llm] url (or set provider)")
		}
	case "custom":
		cmd := cfg.Agent.Command
		if cmd == "" {
			cmd = cfg.ReviewCommand
		}
		if cmd == "" {
			return fmt.Errorf("custom agent requires agent.command or review_command")
		}
	default:
		if _, ok := agentPresets[agent]; !ok {
			return fmt.Errorf("unknown agent %q", agent)
		}
	}
	return nil
}

func newReviewer(cfg Config) Reviewer {
	agent := resolveAgent(cfg)

	switch agent {
	case "builtin":
		var opts []agentcore.ClientOption
		if cfg.LLM.APIKey != "" {
			opts = append(opts, agentcore.WithAPIKey(cfg.LLM.APIKey))
		}
		return &LLMReviewer{
			LLM:          agentcore.NewLLMClient(cfg.LLM.URL, opts...),
			Model:        cfg.LLM.Model,
			ContextSize:  cfg.LLM.ContextSize,
			TokenCeiling: cfg.LLM.TokenCeiling,
			Temperature:  cfg.LLM.Temperature,
			TraceDir:     cfg.LogDir,
			Verbose:      cfg.Verbose,
		}
	case "custom":
		cmd := cfg.Agent.Command
		if cmd == "" {
			cmd = cfg.ReviewCommand
		}
		label := cfg.ReviewAgent
		if label == "" {
			label = "custom"
		}
		return &CommandReviewer{Command: cmd, Agent: label}
	default:
		preset := agentPresets[agent]
		label := cfg.ReviewAgent
		if label == "" {
			label = agent
		}
		return &CLIReviewer{
			Command: preset.Command,
			Args:    preset.Args,
			Agent:   label,
		}
	}
}

func setReviewerMeta(r Reviewer, meta map[string]string) {
	if llmr, ok := r.(*LLMReviewer); ok {
		llmr.mu.Lock()
		llmr.TraceMeta = meta
		llmr.mu.Unlock()
	}
}



func tryGenerateFixes(ctx context.Context, cfg Config, reviewer Reviewer, findings []Finding, workDir string, label string) {
	if (!cfg.Fix && !cfg.FixDryRun) || len(findings) == 0 {
		return
	}
	llmr, ok := reviewer.(*LLMReviewer)
	if !ok {
		warnf("--fix requires builtin agent")
		return
	}
	results, err := generateFixes(ctx, llmr.LLM, llmr.Model, findings, FixOptions{
		DryRun:    cfg.FixDryRun,
		Threshold: cfg.FixThreshold,
		WorkDir:   workDir,
	})
	if err != nil {
		warnf("%s fix generation failed: %v", label, err)
		return
	}
	printFixResults(results)
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

type mrSummary struct {
	ID       int64
	Title    string
	Findings int
	Posted   int
	Tokens   int
	Duration time.Duration
	Status   string
}

func formatTokens(n int) string {
	if n <= 0 {
		return "-"
	}
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}

func printSummaryTable(summaries []mrSummary, totalDuration time.Duration) {
	fmt.Fprintf(os.Stderr, "\n%s\n", cl(ansiBold, "Summary"))
	fmt.Fprintf(os.Stderr, "%s\n", cl(ansiDim, strings.Repeat("─", 72)))
	fmt.Fprintf(os.Stderr, "%s %s %s %s %s %s\n",
		cl(ansiDim, fmt.Sprintf("%-7s", "MR")),
		cl(ansiDim, fmt.Sprintf("%-9s", "Findings")),
		cl(ansiDim, fmt.Sprintf("%-8s", "Posted")),
		cl(ansiDim, fmt.Sprintf("%-9s", "Tokens")),
		cl(ansiDim, fmt.Sprintf("%-10s", "Duration")),
		cl(ansiDim, "Status"))
	fmt.Fprintf(os.Stderr, "%s\n", cl(ansiDim, strings.Repeat("─", 72)))

	totalTokens := 0
	for _, s := range summaries {
		totalTokens += s.Tokens
		id := fmt.Sprintf("#%-6d", s.ID)
		findings := fmt.Sprintf("%-9d", s.Findings)
		posted := fmt.Sprintf("%-8d", s.Posted)
		tokens := fmt.Sprintf("%-9s", formatTokens(s.Tokens))
		dur := fmt.Sprintf("%-10s", s.Duration.Round(time.Second))

		var status string
		switch s.Status {
		case "ok":
			status = cl(ansiGreen, "✓")
		case "dry-run":
			status = cl(ansiCyan, "● dry-run")
		default:
			status = cl(ansiRed, "✗ "+s.Status)
		}

		fmt.Fprintf(os.Stderr, "%s %s %s %s %s %s\n", id, findings, posted, tokens, dur, status)
	}

	fmt.Fprintf(os.Stderr, "%s\n", cl(ansiDim, strings.Repeat("─", 72)))
	fmt.Fprintf(os.Stderr, "Total: %d MR(s), %s tokens, %s\n",
		len(summaries), formatTokens(totalTokens), totalDuration.Round(time.Second))
}

func run(ctx context.Context, cfg Config, state *State, targets []MRTarget) error {
	forge, err := NewForge(cfg)
	if err != nil {
		return fmt.Errorf("%s forge client: %w", cfg.ForgeType, err)
	}

	logf("using %s forge", cl(ansiBold, forge.Name()))
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
		logf("no unreviewed merge/pull requests found")
		return nil
	}

	logf("reviewing %s request(s)", cl(ansiBold, fmt.Sprintf("%d", len(prs))))

	runStart := time.Now()
	var summaries []mrSummary

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

		mrStart := time.Now()
		logf("reviewing %s: %s %s", cl(ansiBold, fmt.Sprintf("#%d", pr.ID)), pr.Title, cl(ansiDim, "(mode: "+reviewMode+")"))

		setReviewerMeta(reviewer, map[string]string{
			"mr_id":   fmt.Sprintf("%d", pr.ID),
			"project": cfg.Project,
			"mode":    reviewMode,
		})

		worktreeDir, cleanup, err := CreateWorktree(ctx, cfg.RepoPath, pr.ID, forge.Name())
		if err != nil {
			warnf("skip #%d: worktree: %v", pr.ID, err)
			continue
		}

		var comment string
		var result *ReviewResult

		switch reviewMode {
		case "both":
			commits, err := forge.ListCommits(ctx, pr.ID)
			if err != nil {
				cleanup()
				warnf("skip #%d: list commits: %v", pr.ID, err)
				continue
			}
			commitResults, err := reviewByCommits(ctx, reviewer, worktreeDir, commits, &ReviewByCommitsOpts{State: state, Project: cfg.Project, OnProgress: cliProgress(fmt.Sprintf("#%d", pr.ID)), Concurrency: cfg.CommitConcurrency()})
			if err != nil {
				cleanup()
				warnf("skip #%d: commit review: %v", pr.ID, err)
				continue
			}

			var digest string
			if llmr, ok := reviewer.(*LLMReviewer); ok {
				digest, err = digestFindings(ctx, llmr.LLM, llmr.Model, commitResults, nil)
				if err != nil {
					warnf("#%d: digest failed, using plain fallback: %v", pr.ID, err)
					digest = digestFindingsPlain(commitResults)
				}
			} else {
				digest = digestFindingsPlain(commitResults)
			}
			logf("%s digest complete, starting branch repass", cl(ansiBold, fmt.Sprintf("#%d", pr.ID)))

			diff, err := forge.GetDiff(ctx, pr.ID)
			if err != nil {
				cleanup()
				warnf("skip #%d: get diff: %v", pr.ID, err)
				continue
			}
			branchResult, err := reviewer.ReviewWithContext(ctx, worktreeDir, diff, digest)
			if err != nil {
				cleanup()
				warnf("skip #%d: branch repass: %v", pr.ID, err)
				continue
			}

			merged := mergeCommitResults(commitResults)
			result = &ReviewResult{
				Findings:   append(merged.Findings, branchResult.Findings...),
				Model:      merged.Model,
				TokensUsed: merged.TokensUsed + branchResult.TokensUsed,
			}
			comment = FormatBothReviewComment(commitResults, branchResult, pr.Title, merged.Model)

		case "commits":
			commits, err := forge.ListCommits(ctx, pr.ID)
			if err != nil {
				cleanup()
				warnf("skip #%d: list commits: %v", pr.ID, err)
				continue
			}
			commitResults, err := reviewByCommits(ctx, reviewer, worktreeDir, commits, &ReviewByCommitsOpts{State: state, Project: cfg.Project, OnProgress: cliProgress(fmt.Sprintf("#%d", pr.ID)), Concurrency: cfg.CommitConcurrency()})
			if err != nil {
				cleanup()
				warnf("skip #%d: review: %v", pr.ID, err)
				continue
			}
			result = mergeCommitResults(commitResults)
			comment = FormatCommitReviewComment(commitResults, pr.Title, result.Model)

		default:
			diff, err := forge.GetDiff(ctx, pr.ID)
			if err != nil {
				cleanup()
				warnf("skip #%d: get diff: %v", pr.ID, err)
				continue
			}
			logf("%s %s reviewing full diff...", cl(ansiBold, fmt.Sprintf("#%d", pr.ID)), cl(ansiCyan, "⟳"))
			result, err = reviewer.ReviewFull(ctx, worktreeDir, diff)
			if err != nil {
				cleanup()
				warnf("skip #%d: review: %v", pr.ID, err)
				continue
			}
			comment = FormatComment(result, pr.Title)
		}

		if result.RawOutput == "" {
			tryGenerateFixes(ctx, cfg, reviewer, result.Findings, worktreeDir, fmt.Sprintf("#%d", pr.ID))
		}
		cleanup()

		if result.RawOutput != "" {
			comment = result.RawOutput
		}

		style := cfg.CommentStyle
		if style == "" {
			style = "both"
		}

		postInline := result.RawOutput == "" && (style == "inline" || style == "both") && len(result.Findings) > 0
		postSummary := style == "summary" || style == "both" || result.RawOutput != ""

		s := mrSummary{
			ID:       pr.ID,
			Title:    pr.Title,
			Findings: len(result.Findings),
			Tokens:   result.TokensUsed,
			Duration: time.Since(mrStart),
		}

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
			s.Status = "dry-run"
		} else {
			posted := 0
			if postInline {
				inlineComments, _ := routeFindings(result.Findings, cfg.InlineSeverity)
				if len(inlineComments) > 0 {
					if err := forge.PostInlineComments(ctx, pr, inlineComments); err != nil {
						errf("#%d: inline comments failed: %v", pr.ID, err)
						s.Status = "inline failed"
					} else {
						posted = len(inlineComments)
						logf("%s posted %s inline comment(s)", cl(ansiBold, fmt.Sprintf("#%d", pr.ID)), cl(ansiGreen, fmt.Sprintf("%d", posted)))
					}
				}
			}
			if postSummary {
				if err := forge.PostComment(ctx, pr.ID, comment); err != nil {
					errf("skip #%d: post comment: %v", pr.ID, err)
					s.Status = "post failed"
					s.Posted = posted
					summaries = append(summaries, s)
					continue
				}
			}
			s.Posted = posted
			if s.Status == "" {
				s.Status = "ok"
			}
		}

		state.StoreResult(cfg.Project, &StoredResult{
			Key:        ResultKeyMR(pr.ID),
			Title:      pr.Title,
			Mode:       reviewMode,
			Findings:   result.Findings,
			RawOutput:  result.RawOutput,
			Model:      result.Model,
			ReviewedAt: time.Now(),
		})
		state.MarkReviewed(cfg.Project, pr.ID)
		if err := state.Save(); err != nil {
			warnf("save state: %v", err)
		}

		logf("%s %s %s finding(s) %s",
			cl(ansiBold, fmt.Sprintf("#%d", pr.ID)),
			cl(ansiGreen, "✓"),
			cl(ansiBold, fmt.Sprintf("%d", len(result.Findings))),
			cl(ansiDim, fmt.Sprintf("(%s, %s tokens)", s.Duration.Round(time.Second), formatTokens(result.TokensUsed))))
		summaries = append(summaries, s)
	}

	if len(summaries) > 0 {
		printSummaryTable(summaries, time.Since(runStart))
	}

	return nil
}
