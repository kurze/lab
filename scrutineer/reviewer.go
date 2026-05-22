package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type Reviewer interface {
	Review(ctx context.Context, workDir string, diff string) (*ReviewResult, error)
	ReviewFull(ctx context.Context, workDir string, diff string) (*ReviewResult, error)
	ReviewWithContext(ctx context.Context, workDir string, diff string, priorContext string) (*ReviewResult, error)
}

type CommandReviewer struct {
	Command string
	Agent   string
}

func (r *CommandReviewer) Review(ctx context.Context, workDir string, diff string) (*ReviewResult, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", r.Command)
	cmd.Dir = workDir
	cmd.Stdin = strings.NewReader(diff)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("review command failed: %w\nstderr: %s", err, stderr.String())
	}

	var result ReviewResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("parse review output: %w\nraw output: %s", err, stdout.String())
	}

	result.Model = r.Agent
	return &result, nil
}

func (r *CommandReviewer) ReviewFull(ctx context.Context, workDir string, diff string) (*ReviewResult, error) {
	return r.Review(ctx, workDir, diff)
}

func (r *CommandReviewer) ReviewWithContext(ctx context.Context, workDir string, diff string, priorContext string) (*ReviewResult, error) {
	input := fmt.Sprintf("PRIOR FINDINGS DIGEST:\n%s\n\n---\n\n%s", priorContext, diff)
	return r.Review(ctx, workDir, input)
}
