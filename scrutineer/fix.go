package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kurze/lab/agentcore"
)

type FixResult struct {
	Finding    Finding
	Patch      string
	Applied    bool
	Committed  bool
	SkipReason string
	CommitSHA  string
}

type FixOptions struct {
	DryRun    bool
	Threshold string
	WorkDir   string
}

func filterFixable(findings []Finding, threshold string) []Finding {
	if threshold == "" {
		threshold = "minor"
	}
	minRank := severityRank[strings.ToLower(threshold)]

	var out []Finding
	for _, f := range findings {
		if severityRank[strings.ToLower(f.Severity)] < minRank {
			continue
		}
		if _, _, ok := parseLocation(f.Location); !ok {
			continue
		}
		out = append(out, f)
	}
	return out
}

func extractPatch(response string) (string, error) {
	if idx := strings.Index(response, "```diff"); idx >= 0 {
		start := idx + len("```diff")
		if nl := strings.IndexByte(response[start:], '\n'); nl >= 0 {
			start += nl + 1
		}
		end := strings.Index(response[start:], "```")
		if end >= 0 {
			patch := strings.TrimSpace(response[start : start+end])
			if patch != "" {
				return patch + "\n", nil
			}
		}
	}

	if idx := strings.Index(response, "```"); idx >= 0 {
		start := idx + len("```")
		if nl := strings.IndexByte(response[start:], '\n'); nl >= 0 {
			start += nl + 1
		}
		end := strings.Index(response[start:], "```")
		if end >= 0 {
			block := strings.TrimSpace(response[start : start+end])
			if strings.Contains(block, "---") && strings.Contains(block, "+++") {
				return block + "\n", nil
			}
		}
	}

	var lines []string
	inPatch := false
	for _, line := range strings.Split(response, "\n") {
		if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "diff --git") {
			inPatch = true
		}
		if inPatch {
			lines = append(lines, line)
		}
	}
	if len(lines) > 0 {
		return strings.Join(lines, "\n") + "\n", nil
	}

	return "", fmt.Errorf("no unified diff patch found in response")
}

func applyPatch(ctx context.Context, workDir string, patch string) error {
	f, err := os.CreateTemp("", "scrutineer-patch-*.patch")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(f.Name())

	if _, err := f.WriteString(patch); err != nil {
		f.Close()
		return fmt.Errorf("write patch: %w", err)
	}
	f.Close()

	check := exec.CommandContext(ctx, "git", "apply", "--check", f.Name())
	check.Dir = workDir
	if out, err := check.CombinedOutput(); err != nil {
		return fmt.Errorf("patch does not apply cleanly: %s", strings.TrimSpace(string(out)))
	}

	apply := exec.CommandContext(ctx, "git", "apply", f.Name())
	apply.Dir = workDir
	if out, err := apply.CombinedOutput(); err != nil {
		return fmt.Errorf("git apply: %w\n%s", err, out)
	}

	return nil
}

func validateFilePath(file string) error {
	if filepath.IsAbs(file) {
		return fmt.Errorf("absolute path not allowed: %s", file)
	}
	cleaned := filepath.Clean(file)
	if strings.HasPrefix(cleaned, "..") {
		return fmt.Errorf("path escapes working directory: %s", file)
	}
	return nil
}

func createFixupCommit(ctx context.Context, workDir string, finding Finding, targetSHA string) (string, error) {
	add := exec.CommandContext(ctx, "git", "add", "-A")
	add.Dir = workDir
	if out, err := add.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git add: %w\n%s", err, out)
	}

	commit := exec.CommandContext(ctx, "git", "commit", "--fixup="+targetSHA)
	commit.Dir = workDir
	if out, err := commit.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git commit --fixup: %w\n%s", err, out)
	}

	rev := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	rev.Dir = workDir
	out, err := rev.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func findTargetCommit(ctx context.Context, workDir string, file string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "log", "-1", "--format=%H", "--", file)
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git log for %s: %w", file, err)
	}
	sha := strings.TrimSpace(string(out))
	if sha == "" {
		return "", fmt.Errorf("no commit found for %s", file)
	}
	return sha, nil
}

func rollbackFile(ctx context.Context, workDir string, file string) {
	cmd := exec.CommandContext(ctx, "git", "checkout", "--", file)
	cmd.Dir = workDir
	cmd.Run()
}

func generatePatch(ctx context.Context, llm *agentcore.LLMClient, model string, workDir string, f Finding) (string, error) {
	file, line, _ := parseLocation(f.Location)

	if err := validateFilePath(file); err != nil {
		return "", err
	}

	path := filepath.Join(workDir, file)
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", file, err)
	}

	lines := strings.Split(string(content), "\n")
	start := line - 50
	if start < 0 {
		start = 0
	}
	end := line + 50
	if end > len(lines) {
		end = len(lines)
	}

	var numbered strings.Builder
	for i := start; i < end; i++ {
		fmt.Fprintf(&numbered, "%d\t%s\n", i+1, lines[i])
	}

	prompt := fmt.Sprintf(`You are a code fixer. Given a code review finding and the relevant source file, produce a minimal unified diff patch that fixes the issue.

Finding:
- Category: %s
- Severity: %s
- Location: %s
- Description: %s
- Evidence: %s

File: %s (lines %d-%d)
%s

Output ONLY a unified diff patch (starting with --- and +++). The patch must apply cleanly with "git apply". Make the minimal change needed to fix the finding. Do not refactor unrelated code.`, f.Category, f.Severity, f.Location, f.Description, f.Evidence, file, start+1, end, numbered.String())

	resp, err := llm.Chat(ctx, agentcore.ChatRequest{
		Model: model,
		Messages: []agentcore.ChatMessage{
			{Role: "user", Content: prompt},
		},
		Temperature: 0.2,
		MaxTokens:   4000,
	})
	if err != nil {
		return "", fmt.Errorf("LLM call: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("empty LLM response")
	}

	return extractPatch(resp.Choices[0].Message.Content)
}

func generateFixes(ctx context.Context, llm *agentcore.LLMClient, model string, findings []Finding, opts FixOptions) ([]FixResult, error) {
	fixable := filterFixable(findings, opts.Threshold)
	if len(fixable) == 0 {
		return nil, nil
	}

	logf("generating fixes for %s qualifying finding(s)", cl(ansiBold, fmt.Sprintf("%d", len(fixable))))

	var results []FixResult
	for i, f := range fixable {
		file, _, _ := parseLocation(f.Location)
		shortLoc := f.Location
		logf("  fix %d/%d: %s %s", i+1, len(fixable), cl(ansiDim, shortLoc), f.Description)

		patch, err := generatePatch(ctx, llm, model, opts.WorkDir, f)
		if err != nil {
			logf("  %s patch generation failed: %v", cl(ansiRed, "✗"), err)
			results = append(results, FixResult{Finding: f, SkipReason: fmt.Sprintf("patch generation: %v", err)})
			continue
		}

		fr := FixResult{Finding: f, Patch: patch}

		if opts.DryRun {
			fmt.Printf("--- patch for %s ---\n%s\n", f.Location, patch)
			results = append(results, fr)
			continue
		}

		if err := applyPatch(ctx, opts.WorkDir, patch); err != nil {
			logf("  %s %v", cl(ansiRed, "✗"), err)
			fr.SkipReason = err.Error()
			results = append(results, fr)
			continue
		}
		fr.Applied = true

		targetSHA := f.CommitSHA
		if targetSHA == "" {
			targetSHA, err = findTargetCommit(ctx, opts.WorkDir, file)
			if err != nil {
				logf("  %s no target commit: %v", cl(ansiRed, "✗"), err)
				rollbackFile(ctx, opts.WorkDir, file)
				fr.SkipReason = fmt.Sprintf("no target commit: %v", err)
				results = append(results, fr)
				continue
			}
		}

		commitSHA, err := createFixupCommit(ctx, opts.WorkDir, f, targetSHA)
		if err != nil {
			logf("  %s commit failed: %v", cl(ansiRed, "✗"), err)
			rollbackFile(ctx, opts.WorkDir, file)
			fr.SkipReason = fmt.Sprintf("commit: %v", err)
			results = append(results, fr)
			continue
		}

		fr.Committed = true
		fr.CommitSHA = commitSHA
		logf("  %s fixup commit %s", cl(ansiGreen, "✓"), cl(ansiDim, commitSHA[:8]))
		results = append(results, fr)
	}

	return results, nil
}

func printFixResults(results []FixResult) {
	if len(results) == 0 {
		return
	}

	applied := 0
	skipped := 0
	dryRun := 0
	for _, r := range results {
		if r.Committed {
			applied++
		} else if r.Patch != "" && r.SkipReason == "" {
			dryRun++
		} else if r.SkipReason != "" {
			skipped++
		}
	}

	fmt.Fprintf(os.Stderr, "\n%s\n", cl(ansiBold, "Fix Summary"))
	fmt.Fprintf(os.Stderr, "%s\n", cl(ansiDim, strings.Repeat("─", 60)))

	for _, r := range results {
		loc := r.Finding.Location
		if r.Committed {
			fmt.Fprintf(os.Stderr, "%s %s — %s\n", cl(ansiGreen, "✓"), loc, cl(ansiDim, r.CommitSHA[:8]))
		} else if r.Patch != "" && r.SkipReason == "" {
			fmt.Fprintf(os.Stderr, "%s %s — dry-run\n", cl(ansiCyan, "●"), loc)
		} else {
			fmt.Fprintf(os.Stderr, "%s %s — %s\n", cl(ansiRed, "✗"), loc, r.SkipReason)
		}
	}

	fmt.Fprintf(os.Stderr, "%s\n", cl(ansiDim, strings.Repeat("─", 60)))
	parts := []string{fmt.Sprintf("%d finding(s)", len(results))}
	if applied > 0 {
		parts = append(parts, fmt.Sprintf("%d fixed", applied))
	}
	if dryRun > 0 {
		parts = append(parts, fmt.Sprintf("%d patched (dry-run)", dryRun))
	}
	if skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", skipped))
	}
	fmt.Fprintf(os.Stderr, "Total: %s\n", strings.Join(parts, ", "))
}
