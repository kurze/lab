package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"maestro/internal/agent"
	"maestro/internal/agent/claude"
	"maestro/internal/agent/local"
	"maestro/internal/coder"
	"maestro/internal/config"
	"maestro/internal/fsm"
	"maestro/internal/grill"
	"maestro/internal/planner"
	"maestro/internal/reviewer"
	"maestro/internal/task"
	"maestro/internal/tracker"
	jiratracker "maestro/internal/tracker/jira"
	"maestro/internal/workspace"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func loadConfigAndStore(configPath string) (config.Config, *task.Store, string, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return config.Config{}, nil, "", fmt.Errorf("load config: %w", err)
	}

	root, err := repoRoot()
	if err != nil {
		return config.Config{}, nil, "", err
	}

	maestroDir := filepath.Join(root, ".maestro")
	store := task.NewStore(maestroDir)
	return cfg, store, root, nil
}

func makeTracker(cfg config.Config, noJira bool) tracker.Tracker {
	if noJira || cfg.Jira.APIToken == "" {
		return tracker.NoopTracker{}
	}
	return jiratracker.New(cfg.Jira)
}

func makeAgent(cfg config.AgentConfig, mode agent.Mode) agent.Agent {
	switch cfg.Type {
	case "claude-code":
		return claude.New(mode)
	case "local-llm":
		return local.New(cfg.Endpoint, cfg.Model, cfg.MaxTokens)
	default:
		return claude.New(mode)
	}
}

func makeGrillAgent(agentType string, cfg config.Config) agent.Agent {
	switch agentType {
	case "local":
		return local.New(cfg.Agents.Reviewer.Endpoint, cfg.Agents.Reviewer.Model, cfg.Agents.Reviewer.MaxTokens)
	default:
		return claude.New(agent.Interactive)
	}
}

func taskDir(maestroDir, taskID string) string {
	return filepath.Join(maestroDir, taskID)
}

func transitionJira(ctx context.Context, trk tracker.Tracker, t *task.Task, cfg config.Config) {
	if t.JiraID == nil {
		return
	}
	mapping := jiratracker.BuildMapping(cfg.Jira.StatusMapping)
	status := jiratracker.MapState(t.State, mapping)
	if status == "" {
		return
	}
	// Best-effort: if Jira is unreachable, log and continue.
	if err := trk.Transition(ctx, *t.JiraID, status); err != nil {
		fmt.Fprintf(os.Stderr, "warning: jira transition failed: %v\n", err)
	}
}

func savePlan(dir string, g *planner.Graph) error {
	data, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal plan: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, "plan.json"), data, 0o644)
}

func loadPlan(dir string) (*planner.Graph, error) {
	data, err := os.ReadFile(filepath.Join(dir, "plan.json"))
	if err != nil {
		return nil, fmt.Errorf("load plan: %w", err)
	}
	var g planner.Graph
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, fmt.Errorf("parse plan: %w", err)
	}
	return &g, nil
}

func slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return '-'
	}, s)
	// Collapse multiple dashes.
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	s = strings.Trim(s, "-")
	if len(s) > 30 {
		s = s[:30]
	}
	return s
}

// ---------------------------------------------------------------------------
// Subcommands
// ---------------------------------------------------------------------------

// cmdNew creates a new task, runs the grill session, and on /done: creates
// Jira ticket and generates the plan.
func cmdNew(configPath, title, jiraKey string, noJira bool, agentType string) error {
	cfg, store, root, err := loadConfigAndStore(configPath)
	if err != nil {
		return err
	}
	ctx := context.Background()

	// Use title from Jira key if not provided.
	if title == "" && jiraKey != "" {
		title = jiraKey
	}

	t, err := store.Create(title)
	if err != nil {
		return fmt.Errorf("create task: %w", err)
	}

	// Attach existing Jira key if provided.
	if jiraKey != "" {
		t.JiraID = &jiraKey
		if err := store.Save(t); err != nil {
			return fmt.Errorf("save task: %w", err)
		}
	}

	maestroDir := filepath.Join(root, ".maestro")
	tDir := taskDir(maestroDir, t.ID)

	fmt.Printf("Created task %s\n", t.ID)
	fmt.Println("Starting grill session...")

	// Run grill.
	ag := makeGrillAgent(agentType, cfg)
	result, err := grill.RunGrill(ctx, ag, tDir, title)
	if err != nil {
		return fmt.Errorf("grill: %w", err)
	}

	if result.Abandoned {
		if err := t.Transition(fsm.Abandoned); err != nil {
			return err
		}
		if err := store.Save(t); err != nil {
			return err
		}
		fmt.Printf("Task %s abandoned.\n", t.ID)
		return nil
	}

	// Read PRD content for planner.
	prdContent, err := os.ReadFile(result.PRDPath)
	if err != nil {
		return fmt.Errorf("read prd: %w", err)
	}

	// Create Jira ticket (if not attaching to existing and not --no-jira).
	trk := makeTracker(cfg, noJira)
	if t.JiraID == nil && !noJira {
		key, err := trk.Create(ctx, t.Title, string(prdContent))
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: jira create failed: %v\n", err)
		} else if key != "" {
			t.JiraID = &key
			fmt.Printf("Created Jira ticket: %s\n", key)
		}
	}

	// Run planner.
	fmt.Println("Generating task graph...")
	plannerAgent := makeAgent(cfg.Agents.Planner, agent.Batch)
	graph, err := planner.Plan(ctx, plannerAgent, string(prdContent), cfg.Agents.Planner.TokenBudget)
	if err != nil {
		return fmt.Errorf("planner: %w", err)
	}

	if err := savePlan(tDir, graph); err != nil {
		return fmt.Errorf("save plan: %w", err)
	}

	// Transition GRILL -> PLAN.
	if err := t.Transition(fsm.Plan); err != nil {
		return err
	}
	if err := store.Save(t); err != nil {
		return err
	}

	transitionJira(ctx, trk, t, cfg)

	// Print plan summary.
	printPlanSummary(graph)
	fmt.Printf("\nTask %s is in PLAN state. Review and run: maestro approve %s\n", t.ID, t.ID)

	return nil
}

func cmdStatus(configPath string, noJira bool, agentType string) error {
	_, store, _, err := loadConfigAndStore(configPath)
	if err != nil {
		return err
	}

	tasks, err := store.List()
	if err != nil {
		return fmt.Errorf("list tasks: %w", err)
	}

	return runTUI(tasks, store, configPath, noJira, agentType)
}

// cmdPlan pretty-prints the task graph for a task.
func cmdPlan(configPath, id string) error {
	_, store, root, err := loadConfigAndStore(configPath)
	if err != nil {
		return err
	}

	t, err := store.Resolve(id)
	if err != nil {
		return err
	}

	maestroDir := filepath.Join(root, ".maestro")
	tDir := taskDir(maestroDir, t.ID)
	graph, err := loadPlan(tDir)
	if err != nil {
		return err
	}

	fmt.Printf("Task: %s — %s [%s]\n\n", t.ID, t.Title, t.State)
	printPlanSummary(graph)
	return nil
}

// cmdApprove transitions PLAN -> CODE, creates worktree, runs coder,
// then runs the review/fix loop.
func cmdApprove(configPath, id string, noJira bool) error {
	cfg, store, root, err := loadConfigAndStore(configPath)
	if err != nil {
		return err
	}
	ctx := context.Background()

	t, err := store.Resolve(id)
	if err != nil {
		return err
	}

	if t.State != fsm.Plan {
		return fmt.Errorf("task %s is in %s state, expected PLAN", t.ID, t.State)
	}

	maestroDir := filepath.Join(root, ".maestro")
	tDir := taskDir(maestroDir, t.ID)
	trk := makeTracker(cfg, noJira)

	// Create worktree.
	jk := ""
	if t.JiraID != nil {
		jk = *t.JiraID
	}
	branchName := workspace.BranchName(t.ID, jk, slugify(t.Title))
	worktreePath, err := workspace.Create(root, t.ID, branchName, cfg.Workspace.WorktreeDir)
	if err != nil {
		return fmt.Errorf("create worktree: %w", err)
	}
	t.WorktreePath = &worktreePath
	t.BranchName = &branchName

	// Transition PLAN -> CODE.
	if err := t.Transition(fsm.Code); err != nil {
		return err
	}
	if err := store.Save(t); err != nil {
		return err
	}
	transitionJira(ctx, trk, t, cfg)

	fmt.Printf("Worktree created: %s\n", worktreePath)
	fmt.Printf("Branch: %s\n", branchName)

	// Load plan and run coder.
	graph, err := loadPlan(tDir)
	if err != nil {
		return err
	}

	prdContent, err := os.ReadFile(filepath.Join(tDir, "prd.md"))
	if err != nil {
		return fmt.Errorf("read prd: %w", err)
	}

	fmt.Println("Running coder...")
	coderAgent := makeAgent(cfg.Agents.Coder, agent.Batch)
	if err := coder.RunCode(ctx, coderAgent, worktreePath, tDir, graph, string(prdContent)); err != nil {
		return fmt.Errorf("coder: %w", err)
	}

	// Save updated plan with sub-task statuses.
	if err := savePlan(tDir, graph); err != nil {
		return fmt.Errorf("save plan: %w", err)
	}

	fmt.Println("Coding complete. Starting review...")

	// CODE -> AI_REVIEW.
	if err := t.Transition(fsm.AIReview); err != nil {
		return err
	}
	if err := store.Save(t); err != nil {
		return err
	}
	transitionJira(ctx, trk, t, cfg)

	// Review/fix loop.
	if err := runReviewLoop(ctx, cfg, store, t, tDir, worktreePath, trk); err != nil {
		return err
	}

	return nil
}

// runReviewLoop runs the AI review -> fix loop, bounded by max_review_iterations.
func runReviewLoop(
	ctx context.Context,
	cfg config.Config,
	store *task.Store,
	t *task.Task,
	tDir, worktreePath string,
	trk tracker.Tracker,
) error {
	maxIter := cfg.Review.MaxIterations
	if t.MaxReviewIterations > 0 {
		maxIter = t.MaxReviewIterations
	}

	reviewerAgent := makeAgent(cfg.Agents.Reviewer, agent.Batch)
	fixerAgent := makeAgent(cfg.Agents.Coder, agent.Batch)

	for t.ReviewIteration < maxIter {
		t.ReviewIteration++

		fmt.Printf("Review iteration %d/%d...\n", t.ReviewIteration, maxIter)

		skillPath := cfg.Review.SkillPath
		verdict, reportPath, err := reviewer.RunReview(
			ctx, reviewerAgent, worktreePath, tDir, skillPath, t.ReviewIteration, cfg.Review.BaseBranch,
		)
		if err != nil {
			return fmt.Errorf("review iteration %d: %w", t.ReviewIteration, err)
		}

		fmt.Printf("Review verdict: %s (report: %s)\n", verdict, reportPath)

		if verdict == reviewer.Pass {
			break
		}

		if verdict == reviewer.Blocker {
			fmt.Println("BLOCKER verdict — stopping review loop, requires human intervention.")
			break
		}

		if t.ReviewIteration >= maxIter {
			fmt.Println("Max review iterations reached.")
			break
		}

		// AI_REVIEW -> AI_FIX.
		if err := t.Transition(fsm.AIFix); err != nil {
			return err
		}
		if err := store.Save(t); err != nil {
			return err
		}
		transitionJira(ctx, trk, t, cfg)

		// Read review report for fix prompt.
		report, err := os.ReadFile(reportPath)
		if err != nil {
			return fmt.Errorf("read review report: %w", err)
		}

		fmt.Printf("Running fix iteration %d...\n", t.ReviewIteration)
		if err := reviewer.RunFix(ctx, fixerAgent, worktreePath, tDir, string(report), t.ReviewIteration); err != nil {
			return fmt.Errorf("fix iteration %d: %w", t.ReviewIteration, err)
		}

		// Commit fix changes.
		commitCmd := exec.CommandContext(ctx, "git", "add", "-A")
		commitCmd.Dir = worktreePath
		if out, err := commitCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git add after fix: %w\n%s", err, out)
		}
		commitMsg := fmt.Sprintf("fix: address review findings (iteration %d)", t.ReviewIteration)
		commitCmd = exec.CommandContext(ctx, "git", "commit", "-m", commitMsg, "--allow-empty")
		commitCmd.Dir = worktreePath
		_ = commitCmd.Run() // skip if nothing to commit

		// AI_FIX -> AI_REVIEW.
		if err := t.Transition(fsm.AIReview); err != nil {
			return err
		}
		if err := store.Save(t); err != nil {
			return err
		}
		transitionJira(ctx, trk, t, cfg)
	}

	// Transition to LOCAL_REVIEW.
	if err := t.Transition(fsm.LocalReview); err != nil {
		return err
	}
	if err := store.Save(t); err != nil {
		return err
	}
	transitionJira(ctx, trk, t, cfg)

	fmt.Printf("Task %s is ready for local review.\n", t.ID)
	fmt.Printf("Review the diff: maestro review %s\n", t.ID)
	return nil
}

// cmdReplan re-invokes the planner with optional instructions.
func cmdReplan(configPath, id, instructions string, noJira bool) error {
	cfg, store, root, err := loadConfigAndStore(configPath)
	if err != nil {
		return err
	}
	ctx := context.Background()

	t, err := store.Resolve(id)
	if err != nil {
		return err
	}

	if t.State != fsm.Plan {
		return fmt.Errorf("task %s is in %s state, expected PLAN", t.ID, t.State)
	}

	maestroDir := filepath.Join(root, ".maestro")
	tDir := taskDir(maestroDir, t.ID)

	prdContent, err := os.ReadFile(filepath.Join(tDir, "prd.md"))
	if err != nil {
		return fmt.Errorf("read prd: %w", err)
	}

	prd := string(prdContent)
	if instructions != "" {
		prd += "\n\n## Replan Instructions\n\n" + instructions
	}

	fmt.Println("Re-generating task graph...")
	plannerAgent := makeAgent(cfg.Agents.Planner, agent.Batch)
	graph, err := planner.Plan(ctx, plannerAgent, prd, cfg.Agents.Planner.TokenBudget)
	if err != nil {
		return fmt.Errorf("planner: %w", err)
	}

	if err := savePlan(tDir, graph); err != nil {
		return fmt.Errorf("save plan: %w", err)
	}

	// PLAN -> PLAN transition.
	if err := t.Transition(fsm.Plan); err != nil {
		return err
	}
	if err := store.Save(t); err != nil {
		return err
	}

	_ = noJira // PLAN -> PLAN does not change Jira status.

	printPlanSummary(graph)
	fmt.Printf("\nPlan updated. Review and run: maestro approve %s\n", t.ID)
	return nil
}

// cmdReview opens the diff for local review.
func cmdReview(configPath, id string) error {
	cfg, store, root, err := loadConfigAndStore(configPath)
	if err != nil {
		return err
	}

	t, err := store.Resolve(id)
	if err != nil {
		return err
	}

	if t.State != fsm.LocalReview {
		return fmt.Errorf("task %s is in %s state, expected LOCAL_REVIEW", t.ID, t.State)
	}

	if t.WorktreePath == nil {
		return fmt.Errorf("task %s has no worktree", t.ID)
	}

	maestroDir := filepath.Join(root, ".maestro")
	tDir := taskDir(maestroDir, t.ID)

	// Print review reports.
	for i := 1; i <= t.ReviewIteration; i++ {
		reportPath := filepath.Join(tDir, fmt.Sprintf("review-%d.md", i))
		if data, err := os.ReadFile(reportPath); err == nil {
			fmt.Printf("=== Review %d ===\n%s\n\n", i, string(data))
		}
	}

	// Show the diff.
	fmt.Println("=== Diff ===")
	baseBranch := cfg.Review.BaseBranch
	if baseBranch == "" {
		baseBranch = "origin/main"
	}
	cmd := exec.Command("git", "diff", "--merge-base", baseBranch, "HEAD", "--stat")
	cmd.Dir = *t.WorktreePath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	_ = cmd.Run()

	fmt.Printf("\nActions:\n")
	fmt.Printf("  maestro push %s      — push and create MR\n", t.ID)
	fmt.Printf("  maestro rework %s    — send back to AI fix\n", t.ID)
	fmt.Printf("  maestro abandon %s   — discard\n", t.ID)

	return nil
}

func cmdRebase(configPath, id string) error {
	cfg, store, _, err := loadConfigAndStore(configPath)
	if err != nil {
		return err
	}

	t, err := store.Resolve(id)
	if err != nil {
		return err
	}

	if t.WorktreePath == nil {
		return fmt.Errorf("task %s has no worktree", t.ID)
	}

	baseBranch := cfg.Review.BaseBranch
	if baseBranch == "" {
		baseBranch = "origin/main"
	}

	// Fetch latest.
	fmt.Printf("Fetching %s...\n", baseBranch)
	fetch := exec.Command("git", "fetch", strings.SplitN(baseBranch, "/", 2)[0])
	fetch.Dir = *t.WorktreePath
	fetch.Stdout = os.Stdout
	fetch.Stderr = os.Stderr
	if err := fetch.Run(); err != nil {
		return fmt.Errorf("git fetch: %w", err)
	}

	// Rebase.
	fmt.Printf("Rebasing on %s...\n", baseBranch)
	rebase := exec.Command("git", "rebase", baseBranch)
	rebase.Dir = *t.WorktreePath
	rebase.Stdout = os.Stdout
	rebase.Stderr = os.Stderr
	if err := rebase.Run(); err != nil {
		return fmt.Errorf("git rebase failed — resolve conflicts in %s and run: git rebase --continue", *t.WorktreePath)
	}

	fmt.Println("Rebase complete.")
	return nil
}

func cmdPushWithBranch(configPath, id, newBranch string, noJira bool) error {
	_, store, _, err := loadConfigAndStore(configPath)
	if err != nil {
		return err
	}

	t, err := store.Resolve(id)
	if err != nil {
		return err
	}

	if t.WorktreePath == nil || t.BranchName == nil {
		return fmt.Errorf("task %s has no worktree or branch", t.ID)
	}

	// Rename branch if different.
	if newBranch != *t.BranchName {
		fmt.Printf("Renaming branch %s → %s\n", *t.BranchName, newBranch)
		rename := exec.Command("git", "branch", "-m", newBranch)
		rename.Dir = *t.WorktreePath
		if out, err := rename.CombinedOutput(); err != nil {
			return fmt.Errorf("git branch -m: %w\n%s", err, out)
		}
		t.BranchName = &newBranch
		if err := store.Save(t); err != nil {
			return err
		}
	}

	return cmdPush(configPath, id, noJira)
}

// cmdPush pushes the branch and prints the MR URL.
func cmdPush(configPath, id string, noJira bool) error {
	cfg, store, _, err := loadConfigAndStore(configPath)
	if err != nil {
		return err
	}
	ctx := context.Background()

	t, err := store.Resolve(id)
	if err != nil {
		return err
	}

	if t.State != fsm.LocalReview {
		return fmt.Errorf("task %s is in %s state, expected LOCAL_REVIEW", t.ID, t.State)
	}

	if t.WorktreePath == nil || t.BranchName == nil {
		return fmt.Errorf("task %s has no worktree or branch", t.ID)
	}

	// git push -u origin <branch>.
	fmt.Printf("Pushing branch %s...\n", *t.BranchName)
	cmd := exec.Command("git", "push", "-u", "origin", *t.BranchName)
	cmd.Dir = *t.WorktreePath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git push: %w", err)
	}

	// Transition LOCAL_REVIEW -> PUSH.
	trk := makeTracker(cfg, noJira)
	if err := t.Transition(fsm.Push); err != nil {
		return err
	}
	if err := store.Save(t); err != nil {
		return err
	}
	transitionJira(ctx, trk, t, cfg)

	fmt.Printf("Task %s pushed.\n", t.ID)

	// Detect forge and offer MR/PR creation.
	remote := getRemoteURL(*t.WorktreePath)
	if remote != "" {
		mrURL := buildMRURL(remote, *t.BranchName)
		if mrURL != "" {
			fmt.Printf("\nMR/PR URL: %s\n", mrURL)
		}
	}

	return nil
}

func cmdCreateMR(configPath, id string) error {
	_, store, _, err := loadConfigAndStore(configPath)
	if err != nil {
		return err
	}

	t, err := store.Resolve(id)
	if err != nil {
		return err
	}

	if t.WorktreePath == nil || t.BranchName == nil {
		return fmt.Errorf("task %s has no worktree or branch", t.ID)
	}

	remote := getRemoteURL(*t.WorktreePath)

	if strings.Contains(remote, "github") {
		fmt.Println("Creating PR with gh...")
		cmd := exec.Command("gh", "pr", "create", "--fill", "--head", *t.BranchName)
		cmd.Dir = *t.WorktreePath
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("gh pr create: %w", err)
		}
	} else if strings.Contains(remote, "gitlab") {
		fmt.Println("Creating MR with glab...")
		cmd := exec.Command("glab", "mr", "create", "--fill", "--source-branch", *t.BranchName)
		cmd.Dir = *t.WorktreePath
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("glab mr create: %w", err)
		}
	} else {
		mrURL := buildMRURL(remote, *t.BranchName)
		fmt.Printf("Open manually: %s\n", mrURL)
	}

	return nil
}

// cmdRework sends the task back to AI_FIX with optional instructions.
func cmdRework(configPath, id, instructions string, noJira bool) error {
	cfg, store, root, err := loadConfigAndStore(configPath)
	if err != nil {
		return err
	}
	ctx := context.Background()

	t, err := store.Resolve(id)
	if err != nil {
		return err
	}

	if t.State != fsm.LocalReview {
		return fmt.Errorf("task %s is in %s state, expected LOCAL_REVIEW", t.ID, t.State)
	}

	maestroDir := filepath.Join(root, ".maestro")
	tDir := taskDir(maestroDir, t.ID)
	trk := makeTracker(cfg, noJira)

	if t.WorktreePath == nil {
		return fmt.Errorf("task %s has no worktree", t.ID)
	}

	// Read last review report before resetting iteration counter.
	fixPrompt := ""
	lastReview := filepath.Join(tDir, fmt.Sprintf("review-%d.md", t.ReviewIteration))
	if data, err := os.ReadFile(lastReview); err == nil {
		fixPrompt = string(data)
	}

	// LOCAL_REVIEW -> AI_FIX.
	t.ReviewIteration = 0
	if err := t.Transition(fsm.AIFix); err != nil {
		return err
	}
	if err := store.Save(t); err != nil {
		return err
	}
	transitionJira(ctx, trk, t, cfg)
	if instructions != "" {
		fixPrompt += "\n\n## Human Instructions\n\n" + instructions
	}
	if fixPrompt == "" {
		fixPrompt = "Please review and fix any remaining issues."
	}

	fmt.Println("Running AI fix...")
	fixerAgent := makeAgent(cfg.Agents.Coder, agent.Batch)
	if err := reviewer.RunFix(ctx, fixerAgent, *t.WorktreePath, tDir, fixPrompt, t.ReviewIteration+1); err != nil {
		return fmt.Errorf("fix: %w", err)
	}

	// AI_FIX -> AI_REVIEW, then run review loop.
	if err := t.Transition(fsm.AIReview); err != nil {
		return err
	}
	if err := store.Save(t); err != nil {
		return err
	}
	transitionJira(ctx, trk, t, cfg)

	return runReviewLoop(ctx, cfg, store, t, tDir, *t.WorktreePath, trk)
}

// cmdAbandon removes worktree and transitions to ABANDONED.
func cmdAbandon(configPath, id string, noJira bool) error {
	cfg, store, _, err := loadConfigAndStore(configPath)
	if err != nil {
		return err
	}
	ctx := context.Background()

	t, err := store.Resolve(id)
	if err != nil {
		return err
	}

	// Remove worktree if it exists.
	if t.WorktreePath != nil {
		fmt.Printf("Removing worktree %s...\n", *t.WorktreePath)
		if err := workspace.Remove(*t.WorktreePath); err != nil {
			fmt.Fprintf(os.Stderr, "warning: worktree remove failed: %v\n", err)
		}
		t.WorktreePath = nil
	}

	if err := t.Transition(fsm.Abandoned); err != nil {
		return err
	}

	if err := store.Save(t); err != nil {
		return err
	}

	// Transition Jira to cancelled if a ticket exists.
	if t.JiraID != nil && !noJira {
		trk := makeTracker(cfg, noJira)
		if err := trk.Transition(ctx, *t.JiraID, "Cancelled"); err != nil {
			fmt.Fprintf(os.Stderr, "warning: jira transition to cancelled failed: %v\n", err)
		}
	}

	fmt.Printf("Task %s abandoned.\n", t.ID)
	return nil
}

// cmdResume re-enters the current phase for a task.
func cmdResume(configPath, id string, noJira bool) error {
	cfg, store, root, err := loadConfigAndStore(configPath)
	if err != nil {
		return err
	}
	ctx := context.Background()

	t, err := store.Resolve(id)
	if err != nil {
		return err
	}

	maestroDir := filepath.Join(root, ".maestro")
	tDir := taskDir(maestroDir, t.ID)
	trk := makeTracker(cfg, noJira)

	fmt.Printf("Resuming task %s in state %s\n", t.ID, t.State)

	switch t.State {
	case fsm.Grill:
		fmt.Println("Re-entering grill session...")
		ag := makeGrillAgent("claude", cfg)
		result, err := grill.RunGrill(ctx, ag, tDir, t.Title)
		if err != nil {
			return fmt.Errorf("grill: %w", err)
		}
		if result.Abandoned {
			if err := t.Transition(fsm.Abandoned); err != nil {
				return err
			}
			return store.Save(t)
		}
		// Continue to plan generation as in cmdNew.
		prdContent, err := os.ReadFile(result.PRDPath)
		if err != nil {
			return fmt.Errorf("read prd: %w", err)
		}
		if t.JiraID == nil && !noJira {
			key, err := trk.Create(ctx, t.Title, string(prdContent))
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: jira create failed: %v\n", err)
			} else if key != "" {
				t.JiraID = &key
			}
		}
		plannerAgent := makeAgent(cfg.Agents.Planner, agent.Batch)
		graph, err := planner.Plan(ctx, plannerAgent, string(prdContent), cfg.Agents.Planner.TokenBudget)
		if err != nil {
			return fmt.Errorf("planner: %w", err)
		}
		if err := savePlan(tDir, graph); err != nil {
			return err
		}
		if err := t.Transition(fsm.Plan); err != nil {
			return err
		}
		if err := store.Save(t); err != nil {
			return err
		}
		transitionJira(ctx, trk, t, cfg)
		printPlanSummary(graph)

	case fsm.Plan:
		graph, err := loadPlan(tDir)
		if err != nil {
			return err
		}
		printPlanSummary(graph)
		fmt.Printf("\nRun: maestro approve %s\n", t.ID)

	case fsm.Code:
		if t.WorktreePath == nil {
			return fmt.Errorf("task %s has no worktree", t.ID)
		}
		graph, err := loadPlan(tDir)
		if err != nil {
			return err
		}
		prdContent, err := os.ReadFile(filepath.Join(tDir, "prd.md"))
		if err != nil {
			return fmt.Errorf("read prd: %w", err)
		}
		coderAgent := makeAgent(cfg.Agents.Coder, agent.Batch)
		if err := coder.RunCode(ctx, coderAgent, *t.WorktreePath, tDir, graph, string(prdContent)); err != nil {
			return fmt.Errorf("coder: %w", err)
		}
		if err := savePlan(tDir, graph); err != nil {
			return err
		}
		if err := t.Transition(fsm.AIReview); err != nil {
			return err
		}
		if err := store.Save(t); err != nil {
			return err
		}
		transitionJira(ctx, trk, t, cfg)
		return runReviewLoop(ctx, cfg, store, t, tDir, *t.WorktreePath, trk)

	case fsm.AIReview:
		if t.WorktreePath == nil {
			return fmt.Errorf("task %s has no worktree", t.ID)
		}
		return runReviewLoop(ctx, cfg, store, t, tDir, *t.WorktreePath, trk)

	case fsm.AIFix:
		if t.WorktreePath == nil {
			return fmt.Errorf("task %s has no worktree", t.ID)
		}
		// Re-enter AI_FIX -> AI_REVIEW loop.
		if err := t.Transition(fsm.AIReview); err != nil {
			return err
		}
		if err := store.Save(t); err != nil {
			return err
		}
		transitionJira(ctx, trk, t, cfg)
		return runReviewLoop(ctx, cfg, store, t, tDir, *t.WorktreePath, trk)

	case fsm.LocalReview:
		return cmdReview(configPath, id)

	case fsm.Push:
		fmt.Println("Task already pushed.")

	case fsm.Abandoned:
		fmt.Println("Task is abandoned.")

	default:
		return fmt.Errorf("unknown state: %s", t.State)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Output helpers
// ---------------------------------------------------------------------------

func printPlanSummary(g *planner.Graph) {
	sorted, err := g.TopologicalSort()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not sort graph: %v\n", err)
		sorted = g.Tasks
	}

	totalTokens := 0
	done := 0
	for _, t := range sorted {
		totalTokens += t.EstimatedTokens
		if t.Status == planner.StatusDone {
			done++
		}
	}

	fmt.Printf("Task Graph (%d tasks, ~%dk tokens, %d/%d done)\n\n",
		len(sorted), totalTokens/1000, done, len(sorted))

	for i, t := range sorted {
		icon := " "
		switch t.Status {
		case planner.StatusDone:
			icon = "✓" // checkmark
		case planner.StatusInProgress:
			icon = "◆" // diamond
		case planner.StatusFailed:
			icon = "✗" // x
		default:
			icon = "○" // circle
		}

		fmt.Printf("  %s %-4s %-50s ~%dk\n", icon, t.ID, t.Title, t.EstimatedTokens/1000)

		if len(t.DependsOn) > 0 {
			fmt.Printf("         deps: %s\n", strings.Join(t.DependsOn, ", "))
		}

		if len(t.FilesHint) > 0 && i < 10 { // Only show file hints for first 10.
			for _, f := range t.FilesHint {
				fmt.Printf("           %s\n", f)
			}
		}
	}
}

// getRemoteURL returns the git remote origin URL.
func getRemoteURL(worktree string) string {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = worktree
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// buildMRURL constructs a merge/pull request creation URL from a remote URL
// and branch name.
func buildMRURL(remoteURL, branch string) string {
	remote := strings.TrimSpace(remoteURL)

	// SSH -> HTTPS conversion.
	if strings.HasPrefix(remote, "git@") {
		remote = strings.TrimPrefix(remote, "git@")
		remote = strings.Replace(remote, ":", "/", 1)
		remote = "https://" + remote
	}
	remote = strings.TrimSuffix(remote, ".git")

	if strings.Contains(remote, "gitlab") {
		return remote + "/-/merge_requests/new?merge_request[source_branch]=" + branch
	}
	if strings.Contains(remote, "github") {
		return remote + "/compare/" + branch + "?expand=1"
	}

	return remote + "/merge_requests/new?source_branch=" + branch
}
