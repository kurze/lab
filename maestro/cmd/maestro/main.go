package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const usage = `maestro — AI coding workflow orchestrator

Usage:
  maestro new "<title>"           Start a new task with a grill session
  maestro new --jira KEY          Attach to an existing Jira ticket
  maestro status                  Show all tasks (TUI)
  maestro plan <id>               Pretty-print the task graph
  maestro approve <id>            Approve plan, create worktree, start coding
  maestro replan <id> [feedback]  Re-invoke planner with optional instructions
  maestro review <id>             Open diff for local review
  maestro push <id>               Push branch, print MR URL
  maestro rebase <id>             Rebase worktree on base branch
  maestro rework <id> [feedback]  Send back to AI fix with optional instructions
  maestro abandon <id>            Remove worktree, archive task
  maestro resume <id>             Re-enter current phase

Flags:
  --jira KEY       Attach to existing Jira ticket (with 'new')
  --no-jira        Skip Jira integration
  --agent TYPE     Agent for grill: claude or local (default: claude)
  --config PATH    Config file path (default: ~/.config/maestro/config.yaml)
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}

	// Parse global flags that can appear anywhere.
	args := os.Args[1:]
	var (
		configPath string
		noJira     bool
		jiraKey    string
		agentType  string
	)

	// Extract flags from args, leaving positional args.
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--config":
			if i+1 < len(args) {
				i++
				configPath = args[i]
			}
		case "--no-jira":
			noJira = true
		case "--jira":
			if i+1 < len(args) {
				i++
				jiraKey = args[i]
			}
		case "--agent":
			if i+1 < len(args) {
				i++
				agentType = args[i]
			}
		case "--help", "-h":
			fmt.Print(usage)
			os.Exit(0)
		default:
			positional = append(positional, args[i])
		}
	}

	if configPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fatalf("cannot determine home directory: %v\n", err)
		}
		configPath = filepath.Join(home, ".config", "maestro", "config.yaml")
	}

	if len(positional) == 0 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}

	subcmd := positional[0]
	subargs := positional[1:]

	switch subcmd {
	case "new":
		title := strings.Join(subargs, " ")
		if title == "" && jiraKey == "" {
			fatalf("maestro new requires a title or --jira KEY\n")
		}
		if err := cmdNew(configPath, title, jiraKey, noJira, agentType); err != nil {
			fatalf("new: %v\n", err)
		}

	case "status":
		if err := cmdStatus(configPath, noJira, agentType); err != nil {
			fatalf("status: %v\n", err)
		}

	case "plan":
		if len(subargs) < 1 {
			fatalf("maestro plan requires a task ID\n")
		}
		if err := cmdPlan(configPath, subargs[0]); err != nil {
			fatalf("plan: %v\n", err)
		}

	case "approve":
		if len(subargs) < 1 {
			fatalf("maestro approve requires a task ID\n")
		}
		if err := cmdApprove(configPath, subargs[0], noJira); err != nil {
			fatalf("approve: %v\n", err)
		}

	case "replan":
		if len(subargs) < 1 {
			fatalf("maestro replan requires a task ID\n")
		}
		instructions := ""
		if len(subargs) > 1 {
			instructions = strings.Join(subargs[1:], " ")
		}
		if err := cmdReplan(configPath, subargs[0], instructions, noJira); err != nil {
			fatalf("replan: %v\n", err)
		}

	case "review":
		if len(subargs) < 1 {
			fatalf("maestro review requires a task ID\n")
		}
		if err := cmdReview(configPath, subargs[0]); err != nil {
			fatalf("review: %v\n", err)
		}

	case "push":
		if len(subargs) < 1 {
			fatalf("maestro push requires a task ID\n")
		}
		if err := cmdPush(configPath, subargs[0], noJira); err != nil {
			fatalf("push: %v\n", err)
		}

	case "rebase":
		if len(subargs) < 1 {
			fatalf("maestro rebase requires a task ID\n")
		}
		if err := cmdRebase(configPath, subargs[0]); err != nil {
			fatalf("rebase: %v\n", err)
		}

	case "rework":
		if len(subargs) < 1 {
			fatalf("maestro rework requires a task ID\n")
		}
		instructions := ""
		if len(subargs) > 1 {
			instructions = strings.Join(subargs[1:], " ")
		}
		if err := cmdRework(configPath, subargs[0], instructions, noJira); err != nil {
			fatalf("rework: %v\n", err)
		}

	case "abandon":
		if len(subargs) < 1 {
			fatalf("maestro abandon requires a task ID\n")
		}
		if err := cmdAbandon(configPath, subargs[0], noJira); err != nil {
			fatalf("abandon: %v\n", err)
		}

	case "resume":
		if len(subargs) < 1 {
			fatalf("maestro resume requires a task ID\n")
		}
		if err := cmdResume(configPath, subargs[0], noJira); err != nil {
			fatalf("resume: %v\n", err)
		}

	default:
		fatalf("unknown command: %s\n%s", subcmd, usage)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format, args...)
	os.Exit(1)
}

func repoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not inside a git repository")
	}
	return strings.TrimSpace(string(out)), nil
}
