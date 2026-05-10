package coder

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"maestro/internal/agent"
	"maestro/internal/planner"
)

const coderPromptTemplate = `## Sub-task

**%s**

%s

## Background — PRD

The following is the full PRD for context. Focus on implementing the sub-task above, not the entire PRD.

%s`

// RunCode walks the plan DAG in topological order and executes each pending
// sub-task using the provided agent. After each successful sub-task it commits
// the changes in the worktree and marks the task as done. The graph is mutated
// in place so callers can observe progress. On failure the current task is
// marked as failed and an error is returned.
func RunCode(ctx context.Context, a agent.Agent, worktree string, taskDir string, g *planner.Graph, prdContent string) error {
	sorted, err := g.TopologicalSort()
	if err != nil {
		return fmt.Errorf("topological sort: %w", err)
	}

	for _, task := range sorted {
		if task.Status == planner.StatusDone {
			continue
		}
		if task.Status == planner.StatusFailed {
			task.Status = planner.StatusPending
		}

		setTaskStatus(g, task.ID, planner.StatusInProgress)
		if err := saveGraph(taskDir, g); err != nil {
			return fmt.Errorf("save plan (in_progress): %w", err)
		}

		prompt := fmt.Sprintf(coderPromptTemplate, task.Title, task.Description, prdContent)

		_, err := a.Run(ctx, worktree, prompt)
		if err != nil {
			setTaskStatus(g, task.ID, planner.StatusFailed)
			_ = saveGraph(taskDir, g)
			return fmt.Errorf("sub-task %s (%s) failed: %w", task.ID, task.Title, err)
		}

		// Commit all changes produced by the agent.
		if err := gitCommit(ctx, worktree, task.Title); err != nil {
			setTaskStatus(g, task.ID, planner.StatusFailed)
			_ = saveGraph(taskDir, g)
			return fmt.Errorf("git commit for sub-task %s: %w", task.ID, err)
		}

		setTaskStatus(g, task.ID, planner.StatusDone)
		if err := saveGraph(taskDir, g); err != nil {
			return fmt.Errorf("save plan (done): %w", err)
		}
	}

	return nil
}

// setTaskStatus updates the status of a task in the graph by ID.
func setTaskStatus(g *planner.Graph, id string, status planner.TaskStatus) {
	for i := range g.Tasks {
		if g.Tasks[i].ID == id {
			g.Tasks[i].Status = status
			return
		}
	}
}

func saveGraph(taskDir string, g *planner.Graph) error {
	data, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal graph: %w", err)
	}

	path := filepath.Join(taskDir, "plan.json")
	return os.WriteFile(path, data, 0o644)
}

// gitCommit stages all changes and creates a commit with the given message.
func gitCommit(ctx context.Context, worktree string, message string) error {
	// Stage all changes.
	add := exec.CommandContext(ctx, "git", "add", "-A")
	add.Dir = worktree
	if out, err := add.CombinedOutput(); err != nil {
		return fmt.Errorf("git add: %w\n%s", err, out)
	}

	// Check if there is anything to commit.
	// Exit 0 = nothing staged, exit 1 = changes staged, other = error.
	diff := exec.CommandContext(ctx, "git", "diff", "--cached", "--quiet")
	diff.Dir = worktree
	if err := diff.Run(); err == nil {
		return nil
	} else if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		// Changes staged — proceed to commit.
	} else {
		return fmt.Errorf("git diff --cached: %w", err)
	}

	// Commit.
	commit := exec.CommandContext(ctx, "git", "commit", "-m", message)
	commit.Dir = worktree
	if out, err := commit.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %w\n%s", err, out)
	}

	return nil
}
