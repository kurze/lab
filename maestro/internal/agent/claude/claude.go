package claude

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"maestro/internal/agent"
)

var _ agent.Agent = (*ClaudeAgent)(nil)

// ClaudeAgent wraps the `claude` CLI as an Agent implementation.
type ClaudeAgent struct {
	mode agent.Mode
}

// New creates a ClaudeAgent operating in the given mode.
func New(mode agent.Mode) *ClaudeAgent {
	return &ClaudeAgent{mode: mode}
}

func (a *ClaudeAgent) Type() agent.AgentType {
	return agent.ClaudeCode
}

// Run executes the claude CLI.
//
// In interactive mode (grill): exec replaces the current process so the user
// gets a direct terminal session with Claude Code. The caller should fork
// before calling Run in interactive mode, or accept that Run will not return.
//
// In batch mode (code/fix): runs `claude --print` with the prompt on stdin
// and captures stdout.
func (a *ClaudeAgent) Run(ctx context.Context, worktree string, prompt string) (string, error) {
	switch a.mode {
	case agent.Interactive:
		return a.runInteractive(ctx, worktree, prompt)
	case agent.Batch:
		return a.runBatch(ctx, worktree, prompt)
	default:
		return "", fmt.Errorf("unknown claude agent mode: %s", a.mode)
	}
}

// runInteractive runs the claude CLI as a child process with inherited stdio,
// giving the user a direct terminal session.
func (a *ClaudeAgent) runInteractive(ctx context.Context, worktree string, prompt string) (string, error) {
	var args []string
	if prompt != "" {
		args = append(args, prompt)
	}

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if worktree != "" {
		cmd.Dir = worktree
	}

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("claude session failed: %w", err)
	}
	return "", nil
}

// runBatch runs `claude --print` with the prompt on stdin, captures stdout.
func (a *ClaudeAgent) runBatch(ctx context.Context, worktree string, prompt string) (string, error) {
	args := []string{"--print"}

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Stdin = strings.NewReader(prompt)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if worktree != "" {
		cmd.Dir = worktree
	}

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("claude --print failed: %w\nstderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}
