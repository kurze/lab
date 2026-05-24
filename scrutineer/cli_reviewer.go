package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type CLIReviewer struct {
	Command string
	Args    []string
	Agent   string
}

func (r *CLIReviewer) Review(ctx context.Context, workDir string, diff string) (*ReviewResult, error) {
	prompt := buildCLIReviewPrompt(diff, "commit")
	return r.execute(ctx, workDir, prompt)
}

func (r *CLIReviewer) ReviewFull(ctx context.Context, workDir string, diff string) (*ReviewResult, error) {
	prompt := buildCLIReviewPrompt(diff, "full")
	return r.execute(ctx, workDir, prompt)
}

func (r *CLIReviewer) ReviewWithContext(ctx context.Context, workDir string, diff string, priorContext string) (*ReviewResult, error) {
	prompt := buildCLIRepassPrompt(diff, priorContext)
	return r.execute(ctx, workDir, prompt)
}

func (r *CLIReviewer) execute(ctx context.Context, workDir string, prompt string) (*ReviewResult, error) {
	cmd := exec.CommandContext(ctx, r.Command, r.Args...)
	cmd.Dir = workDir
	cmd.Stdin = strings.NewReader(prompt)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("agent %s failed: %w\nstderr: %s", r.Agent, err, stderr.String())
	}

	output := strings.TrimSpace(stdout.String())
	return &ReviewResult{RawOutput: output, Model: r.Agent}, nil
}

func buildCLIReviewPrompt(diff string, mode string) string {
	scope := "commit"
	if mode == "full" {
		scope = "merge request"
	}
	return fmt.Sprintf("Review the following %s diff. For each issue found, state the file and line, severity (info/minor/major/critical), and a brief description.\n\nIf no issues are found, say so.\n\n%sdiff\n%s\n%s", scope, fence, diff, fence)
}

func buildCLIRepassPrompt(diff string, priorContext string) string {
	return fmt.Sprintf("Review the following merge request diff. Focus on cross-cutting concerns that span multiple commits.\n\nPrior findings (already reported — do NOT repeat these):\n%s\n\n---\n\n%sdiff\n%s\n%s\n\nReport only NEW findings not covered above. For each issue, state the file and line, severity (info/minor/major/critical), and a brief description. If no new issues are found, say so.", priorContext, fence, diff, fence)
}
