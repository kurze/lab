package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func runDiffReview(ctx context.Context, llm *LLMClient, model ModelDef, root, diffRef, focus string, maxIter int) (*ReviewResult, error) {
	diff, err := gitDiff(root, diffRef)
	if err != nil {
		return nil, fmt.Errorf("git diff: %w", err)
	}
	if strings.TrimSpace(diff) == "" {
		return nil, fmt.Errorf("no diff output for %q", diffRef)
	}

	systemPrompt := buildDiffSystemPrompt(diffRef, focus, root)

	lr, err := runLoop(ctx, llm, LoopConfig{
		Model:       model,
		Root:        root,
		Temperature: 0.3,
		MaxIter:     maxIter,
		MaxTokens:   defaultMaxTokens,
		TracerTag:   "diff-" + sanitizeTracerTag(diffRef),
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: fmt.Sprintf("Here is the diff to review:\n\n```diff\n%s\n```\n\nFocus: %s\n\nExplore the workspace for context, then produce your findings.", diff, focus)},
		},
	})
	if err != nil {
		return nil, err
	}

	defer lr.Tracer.Close()

	if lr.Truncated {
		return collectPartial(model.ID, lr), nil
	}

	return parseReviewOutput(ctx, llm, model, lr)
}

func buildDiffSystemPrompt(diffRef, focus, root string) string {
	return fmt.Sprintf(`You are a code review agent. Your task is to review a git diff and produce structured findings.

Diff reference: %s
Focus area: %s
Workspace root: %s

You have three tools: read_file, grep, list_dir. All paths are relative to the workspace root.
Use these tools to understand the surrounding code context for the changes in the diff.

Process:
1. Analyze the diff provided in the user message
2. Explore the workspace to understand the context around the changed code
3. When ready, produce your final output as a JSON object

Your final output MUST be a JSON object with exactly these fields:
{
  "findings": [{"category": "missing|inconsistent|risk|assumption|pattern_match", "severity": "info|minor|major", "location": "file:line or section", "description": "what you found", "evidence": "what led you to this"}],
  "open_questions": ["questions that need human input"]
}

Rules:
- Focus on the CHANGES in the diff, not pre-existing issues.
- Be descriptive, never prescriptive. Say what you found, not what to do about it.
- Every finding needs concrete evidence from the diff or codebase.
- Pay attention to: missing error handling in new code, broken invariants, inconsistencies with existing patterns, untested edge cases, security implications.
- Be concise. Short descriptions, minimal evidence quotes. No preamble or explanation outside the JSON.
- When you are done exploring, stop calling tools and output ONLY your JSON.`, diffRef, focus, root)
}

func gitDiff(root, ref string) (string, error) {
	var args []string
	switch {
	case ref == "" || ref == "HEAD":
		args = []string{"diff", "HEAD"}
	case strings.Contains(ref, ".."):
		parts := strings.SplitN(ref, "..", 2)
		args = []string{"diff", parts[0], parts[1]}
	default:
		args = []string{"diff", ref}
	}
	args = append(args, "--no-color", "--unified=5")

	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git diff failed: %s", string(exitErr.Stderr))
		}
		return "", err
	}

	const maxDiffSize = 100_000
	s := string(out)
	if len(s) > maxDiffSize {
		s = s[:maxDiffSize] + "\n... diff truncated at 100KB ..."
	}
	return s, nil
}

func sanitizeTracerTag(s string) string {
	r := strings.NewReplacer("/", "-", "..", "-", " ", "-")
	return r.Replace(s)
}

func parseDiffRef(ref string) string {
	if ref == "" {
		return "HEAD"
	}
	for _, c := range ref {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '.' || c == '/' || c == '-' || c == '_' || c == '~' || c == '^') {
			return ""
		}
	}
	return ref
}
