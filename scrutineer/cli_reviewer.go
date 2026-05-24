package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type CLIReviewer struct {
	Command   string
	Args      []string
	Agent     string
	ReviewCfg ReviewPromptConfig
}

func (r *CLIReviewer) Review(ctx context.Context, workDir string, diff string) (*ReviewResult, error) {
	pc := PromptConfig{Focus: r.ReviewCfg.Focus, Guidelines: r.ReviewCfg.Guidelines}
	prompt := BuildCLIReviewPrompt(diff, "commit", pc)
	return r.execute(ctx, workDir, prompt)
}

func (r *CLIReviewer) ReviewFull(ctx context.Context, workDir string, diff string) (*ReviewResult, error) {
	pc := PromptConfig{Focus: r.ReviewCfg.Focus, Guidelines: r.ReviewCfg.Guidelines}
	prompt := BuildCLIReviewPrompt(diff, "full", pc)
	return r.execute(ctx, workDir, prompt)
}

func (r *CLIReviewer) ReviewWithContext(ctx context.Context, workDir string, diff string, priorContext string) (*ReviewResult, error) {
	pc := PromptConfig{Focus: r.ReviewCfg.Focus, Guidelines: r.ReviewCfg.Guidelines}
	prompt := BuildCLIRepassPrompt(diff, priorContext, pc)
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
