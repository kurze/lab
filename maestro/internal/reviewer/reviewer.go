package reviewer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"maestro/internal/agent"
)

const maxDiffSize = 200_000

const defaultSkill = `Review the code changes for:
- Correctness: bugs, logic errors, off-by-one, nil/null dereferences
- Security: injection, path traversal, credential exposure
- Performance: unnecessary allocations, O(n²) where O(n) is possible
- Style: naming, dead code, missing error handling

Focus on issues in the changed lines. Do not flag style nits on unchanged code.`

const reviewPromptTemplate = `## Review Skill

%s

## Diff

The following is the three-dot diff (merge-base to HEAD) for this branch:

` + "```diff\n%s\n```" + `

Review the diff according to the skill instructions above and produce a
markdown report. End with a verdict line in the exact format:

VERDICT: PASS | NEEDS_FIX | BLOCKER
`

// RunReview generates the three-dot diff, reads the review skill file, sends
// both to the reviewer agent, parses the verdict, and writes the report to
// review-N.md in taskDir.
//
// It returns the parsed verdict, the path to the written report, and any error.
func RunReview(
	ctx context.Context,
	a agent.Agent,
	worktree string,
	taskDir string,
	reviewSkillPath string,
	iteration int,
	baseBranch string,
) (Verdict, string, error) {
	if baseBranch == "" {
		baseBranch = "origin/main"
	}
	diff, err := threeDotDiff(ctx, worktree, baseBranch)
	if err != nil {
		return 0, "", fmt.Errorf("three-dot diff: %w", err)
	}
	if strings.TrimSpace(diff) == "" {
		return 0, "", fmt.Errorf("empty diff — nothing to review")
	}

	var skillContent []byte
	if reviewSkillPath != "" {
		skillContent, err = os.ReadFile(reviewSkillPath)
		if err != nil {
			return 0, "", fmt.Errorf("read review skill %s: %w", reviewSkillPath, err)
		}
	}
	if len(skillContent) == 0 {
		skillContent = []byte(defaultSkill)
	}

	// Truncate diff if very large.
	if len(diff) > maxDiffSize {
		diff = diff[:maxDiffSize] + "\n... diff truncated ...\n"
	}

	prompt := fmt.Sprintf(reviewPromptTemplate, string(skillContent), diff)

	// Invoke the reviewer agent.
	output, err := a.Run(ctx, worktree, prompt)
	if err != nil {
		return 0, "", fmt.Errorf("reviewer agent: %w", err)
	}

	verdict, err := ParseVerdict(output)
	if err != nil {
		reportPath, _ := writeReport(taskDir, iteration, output)
		if strings.TrimSpace(output) == "" {
			return 0, reportPath, fmt.Errorf("reviewer returned empty output")
		}
		fmt.Fprintf(os.Stderr, "warning: %v — defaulting to NEEDS_FIX\n", err)
		verdict = NeedsFix
	}

	reportPath, err := writeReport(taskDir, iteration, output)
	if err != nil {
		return 0, "", err
	}

	return verdict, reportPath, nil
}

func threeDotDiff(ctx context.Context, worktree string, baseBranch string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "--merge-base", baseBranch, "HEAD", "--no-color", "--unified=5")
	cmd.Dir = worktree
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git diff: %s", string(exitErr.Stderr))
		}
		return "", err
	}
	return string(out), nil
}

func writeReport(taskDir string, iteration int, content string) (string, error) {
	filename := fmt.Sprintf("review-%d.md", iteration)
	path := filepath.Join(taskDir, filename)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write review report: %w", err)
	}
	return path, nil
}
