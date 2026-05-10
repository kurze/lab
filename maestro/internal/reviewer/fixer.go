package reviewer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"maestro/internal/agent"
)

const fixPromptTemplate = `## Review Findings

The following is the AI review report for the current branch. Fix the issues
described below. Do not introduce unrelated changes.

%s
`

// RunFix invokes the fix agent with the review report as prompt, working in
// the same worktree. It writes a fix summary to fix-N.md in the task directory.
func RunFix(
	ctx context.Context,
	a agent.Agent,
	worktree string,
	taskDir string,
	reviewReport string,
	iteration int,
) error {
	prompt := fmt.Sprintf(fixPromptTemplate, reviewReport)

	output, err := a.Run(ctx, worktree, prompt)
	if err != nil {
		return fmt.Errorf("fix agent: %w", err)
	}

	// Write fix summary.
	filename := fmt.Sprintf("fix-%d.md", iteration)
	path := filepath.Join(taskDir, filename)
	if err := os.WriteFile(path, []byte(output), 0o644); err != nil {
		return fmt.Errorf("write fix summary: %w", err)
	}

	return nil
}
