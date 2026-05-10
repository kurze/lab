package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

var reviewResultSchema = `{"type":"object","required":["findings"],"properties":{"findings":{"type":"array","items":{"type":"object","properties":{"category":{"type":"string"},"severity":{"type":"string","enum":["info","minor","major","critical"]},"location":{"type":"string"},"description":{"type":"string"},"evidence":{"type":"string"}},"required":["category","severity","location","description"]}}}}`

const claudeReadOnlyTools = "Read,Bash(grep *),Bash(git log *),Bash(git diff *),Bash(git show *),Bash(git blame *)"

type ClaudeCodeReviewer struct {
	Model string
}

func (r *ClaudeCodeReviewer) Review(ctx context.Context, workDir string, diff string) (*ReviewResult, error) {
	prompt := "Review this commit diff (provided on stdin). For each finding, note the file and line, severity (info/minor/major/critical), category, and a one-sentence description. Focus on changes only, not pre-existing issues. Be descriptive, not prescriptive."
	return r.run(ctx, workDir, prompt, diff)
}

func (r *ClaudeCodeReviewer) ReviewFull(ctx context.Context, workDir string, diff string) (*ReviewResult, error) {
	prompt := "Review this merge request diff (provided on stdin). Explore the codebase for context. For each finding, note the file and line, severity (info/minor/major/critical), category, and a one-sentence description with evidence. Focus on changes only, not pre-existing issues."
	return r.run(ctx, workDir, prompt, diff)
}

func (r *ClaudeCodeReviewer) ReviewWithContext(ctx context.Context, workDir string, diff string, priorContext string) (*ReviewResult, error) {
	prompt := fmt.Sprintf(`Second-pass review of a merge request. The diff is provided on stdin. DO NOT repeat prior findings.

Prior findings:
%s

Focus only on cross-cutting concerns: cross-commit interactions, architectural impact, patterns across commits. If no new issues, return empty findings.`, priorContext)
	return r.run(ctx, workDir, prompt, diff)
}

func (r *ClaudeCodeReviewer) run(ctx context.Context, workDir string, prompt string, diff string) (*ReviewResult, error) {
	args := []string{
		"-p", prompt,
		"--output-format", "json",
		"--json-schema", reviewResultSchema,
		"--bare",
		"--dangerously-skip-permissions",
		"--tools", claudeReadOnlyTools,
	}
	if r.Model != "" {
		args = append(args, "--model", r.Model)
	}

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = workDir
	cmd.Stdin = strings.NewReader(diff)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("claude-code failed: %w\nstderr: %s", err, stderr.String())
	}

	var envelope struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		return nil, fmt.Errorf("parse claude-code envelope: %w\nraw: %s", err, stdout.String())
	}

	var result ReviewResult
	if err := json.Unmarshal([]byte(envelope.Result), &result); err != nil {
		return nil, fmt.Errorf("parse claude-code findings: %w\nraw result: %s", err, envelope.Result)
	}

	model := "claude-code"
	if r.Model != "" {
		model = r.Model
	}
	result.Model = model
	return &result, nil
}
