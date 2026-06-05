package main

import (
	"fmt"
	"strings"
)

const fence = "```"

type PromptConfig struct {
	Root         string
	PriorContext string
	Focus        string
	Guidelines   string
}

func BuildCommitReviewPrompt(pc PromptConfig) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf(`You are an expert code reviewer analyzing a single commit.

The commit message describes the author's intent. Your job is to verify whether the diff achieves that intent correctly and completely. Discrepancies between stated intent and actual changes are high-priority findings.

Workspace root: %s

Tools (paths relative to root):
- read_file: read file contents (with optional line range)
- grep: regex search
- list_dir: list directory

Process: read the diff, optionally read 1-2 changed files for context, then write your review.

Focus on:
- What this commit is actually trying to do (intent)
- Whether it accomplishes that correctly (correctness)
- What it breaks or risks breaking (regressions, security, edge cases)
- What is subtly wrong that a tired reviewer would miss

Do not waste attention on:
- Formatting and naming conventions
- Things that are fine
- Restating what the code does line by line
- Hedging ("this might be an issue" — either it is or it isn't)
- Preamble, disclaimers, or summary of what you're about to do
- Commits prefixed with "fixup!" or "squash!" are cleanup commits — only check whether the fix is correct, skip everything else

Be opinionated. If something is wrong, say so directly and explain why.
If the commit is clean, say it's clean and why in two sentences. Do not pad with filler.

Output: for each finding state file:line, severity (info/minor/major/critical), what you found.`, pc.Root))
	b.WriteString(guidelinesSection(pc.Guidelines))
	return b.String()
}

func BuildMRReviewPrompt(pc PromptConfig) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf(`You are an expert code reviewer analyzing a merge request.

The MR diff represents a complete set of changes. Your job is to find real problems — bugs, security holes, performance traps, broken invariants. Verify that the changes are correct and complete for what they claim to do.

Workspace root: %s

Tools (paths relative to root):
- read_file, grep, glob, list_dir, git_log, git_diff, git_blame, git_show
- fork: split into parallel sub-tasks (use this!)

Process:
1. EXPLORE: read the diff carefully. Use grep/read_file to understand the surrounding code — call sites, types, invariants. Build context before judging.
2. Call the fork tool with these four tasks:
%s

Do not waste attention on:
- Formatting and naming conventions
- Things that are fine
- Restating what the code does line by line
- Hedging ("this might be an issue" — either it is or it isn't)
- Preamble, disclaimers, or summary of what you're about to do
- Commits prefixed with "fixup!" or "squash!" are cleanup commits — only check whether the fix is correct, skip everything else

Be opinionated. If something is wrong, say so directly and explain why.

Output: for each finding state file:line, severity (info/minor/major/critical), what you found, short evidence quote. If a category has no findings, skip it.`, pc.Root, forkTasks(pc.Focus)))
	b.WriteString(guidelinesSection(pc.Guidelines))
	return b.String()
}

func BuildMRRepassPrompt(pc PromptConfig) string {
	var b strings.Builder
	b.WriteString("You are an expert code reviewer. Second pass — DO NOT repeat prior findings.\n")
	prior := strings.TrimSpace(pc.PriorContext)
	if prior != "" {
		fmt.Fprintf(&b, "\nPrior findings (already reported):\n<prior_findings>\n%s\n</prior_findings>\n", prior)
	}
	b.WriteString(fmt.Sprintf(`
Workspace root: %s

Tools (paths relative to root):
- read_file, grep, glob, list_dir, git_log, git_diff, git_blame, git_show
- fork: split into parallel sub-tasks (use this!)

Process:
1. EXPLORE: read the full diff. Use grep/read_file to trace how changes across commits interact — shared state, call chains, data flow.
2. Call the fork tool with these four tasks (focus on cross-commit interactions):
%s

Do not waste attention on:
- Anything already in the prior findings above
- Formatting and naming conventions
- Things that are fine
- Hedging or preamble
- Commits prefixed with "fixup!" or "squash!" are cleanup commits — only check whether the fix is correct, skip everything else

Focus on what spans multiple commits or emerges from the full picture. Be opinionated.

Output: for each finding state file:line, severity (info/minor/major/critical), what you found, short evidence quote. If no new issues, say "No cross-cutting issues found."`, pc.Root, repassForkTasks(pc.Focus)))
	b.WriteString(guidelinesSection(pc.Guidelines))
	return b.String()
}

func BuildExtractionPrompt() string {
	return `Extract structured findings from the code review below. Output ONLY a JSON object:
{"findings": [{"category": "string", "severity": "info|minor|major|critical", "location": "file:line", "description": "one sentence", "evidence": "short quote or empty"}]}
If the review found no issues, return {"findings": []}.
Preserve the exact file:line locations from the review text. Do not invent or modify locations.`
}

func BuildDigestPrompt() string {
	return `You are a code review assistant. You will receive findings from a per-commit review of a merge request. Produce a compact digest of these findings that another reviewer can use to avoid repeating them.

Produce a concise bulleted summary organized by theme (not by commit). For each bullet:
- State what was found and where (file/location)
- Note the severity
- If multiple commits touch the same concern, mention that
- If findings across commits contradict each other (one adds validation, another removes it), flag that explicitly

Keep the digest under 500 words. Do not add new findings. Do not suggest fixes. Just summarize what was found.`
}

func BuildCLIReviewPrompt(diff string, mode string, pc PromptConfig) string {
	scope := "commit"
	if mode == "full" {
		scope = "merge request"
	}
	var b strings.Builder
	fmt.Fprintf(&b, `Review the following %s diff. For each issue found, state the file and line, severity (info/minor/major/critical), and a brief description.

Focus on correctness, regressions, and security. Do not flag style or formatting issues.
If the commit is clean, say so in one sentence. Do not pad with filler observations.

If no issues are found, say so.`, scope)
	b.WriteString(guidelinesSection(pc.Guidelines))
	fmt.Fprintf(&b, "\n\n%sdiff\n%s\n%s", fence, diff, fence)
	return b.String()
}

func BuildCLIRepassPrompt(diff string, priorContext string, pc PromptConfig) string {
	var b strings.Builder
	b.WriteString("Review the following merge request diff. Focus on cross-cutting concerns that span multiple commits.\n")
	if priorContext = strings.TrimSpace(priorContext); priorContext != "" {
		fmt.Fprintf(&b, "\nPrior findings (already reported — do NOT repeat these):\n<prior_findings>\n%s\n</prior_findings>\n", priorContext)
	}
	b.WriteString(guidelinesSection(pc.Guidelines))
	fmt.Fprintf(&b, "\n\n%sdiff\n%s\n%s", fence, diff, fence)
	b.WriteString("\n\nReport only NEW findings not covered above. For each issue, state the file and line, severity (info/minor/major/critical), and a brief description. If no new issues are found, say so.")
	return b.String()
}

func forkTasks(focus string) string {
	bugs := `   - id:"bugs", prompt:"Hunt for logic errors, nil derefs, off-by-one, race conditions, missing error handling, broken invariants. Read changed files and their callers for evidence."`
	security := `   - id:"security", prompt:"Hunt for injection, auth bypass, secrets exposure, unsafe input handling, path traversal. Trace data flow from inputs to sensitive operations."`
	perf := `   - id:"perf", prompt:"Hunt for unnecessary allocations, O(n²) loops, missing caching, unbounded growth, blocking calls. Check hot paths."`
	style := `   - id:"style", prompt:"Hunt for dead code, unclear control flow, missing or misleading abstractions."`

	switch focus {
	case "security":
		security = `   - id:"security", prompt:"Deep security review. Hunt for: injection (SQL, command, template), auth/authz bypass, secrets in code or logs, unsafe deserialization, SSRF, path traversal, TOCTOU races, missing input validation on trust boundaries. Trace every external input to where it's consumed. Check error paths for information leaks."`
		bugs = `   - id:"bugs", prompt:"Hunt for logic errors and broken invariants that could have security impact."`
		perf = `   - id:"perf", prompt:"Check for denial-of-service vectors: unbounded allocations, missing timeouts, algorithmic complexity attacks."`
		style = `   - id:"style", prompt:"Check for security anti-patterns: magic strings, hardcoded credentials, overly broad permissions."`
	case "performance":
		perf = `   - id:"perf", prompt:"Deep performance review. Hunt for: unnecessary allocations and copies, O(n²) or worse loops, missing caching on repeated lookups, unbounded growth in maps/slices, blocking calls in hot paths, unnecessary serialization, N+1 query patterns. Profile-think through the hot path. Check memory layout and GC pressure."`
		bugs = `   - id:"bugs", prompt:"Hunt for logic errors, especially those that cause unnecessary work or resource leaks."`
		security = `   - id:"security", prompt:"Check for resource exhaustion and denial-of-service vectors."`
		style = `   - id:"style", prompt:"Check for clarity issues that obscure performance characteristics."`
	case "style":
		style = `   - id:"style", prompt:"Deep code quality review. Hunt for: dead code, unclear control flow, misleading names, leaky abstractions, inconsistent error handling patterns, missing documentation on non-obvious behavior, overly complex functions that should be split, duplicated logic that should be extracted."`
		bugs = `   - id:"bugs", prompt:"Hunt for logic errors and broken invariants."`
		security = `   - id:"security", prompt:"Hunt for obvious security issues: injection, auth bypass, secrets exposure."`
		perf = `   - id:"perf", prompt:"Hunt for obvious performance issues: O(n²) loops, unbounded growth."`
	}

	return bugs + "\n" + security + "\n" + perf + "\n" + style
}

func repassForkTasks(focus string) string {
	bugs := `   - id:"bugs", prompt:"Hunt for logic errors that emerge from cross-commit interactions: shared state broken by different commits, inconsistent error handling, assumptions in one commit violated by another."`
	security := `   - id:"security", prompt:"Hunt for security issues spanning multiple commits: auth gaps, input validation missing on new paths, secrets exposure, unsafe data flow across changed boundaries."`
	perf := `   - id:"perf", prompt:"Hunt for performance issues at branch scale: redundant work across commits, new hot paths without caching, unbounded growth introduced by combined changes."`
	style := `   - id:"style", prompt:"Hunt for branch-wide consistency: repeated anti-patterns, inconsistent naming or conventions, dead code left behind."`

	switch focus {
	case "security":
		security = `   - id:"security", prompt:"Deep cross-commit security review. Trace trust boundaries across all changes. Hunt for: auth added in one commit but bypassable via another, validation gaps at new boundaries, secrets flowing through newly connected paths, privilege escalation via combined changes."`
	case "performance":
		perf = `   - id:"perf", prompt:"Deep cross-commit performance review. Trace hot paths across all changes. Hunt for: redundant work introduced by separate commits, new allocation patterns that compound, caching invalidated by one commit but not updated by another, combined changes creating O(n²) behavior."`
	case "style":
		style = `   - id:"style", prompt:"Deep cross-commit quality review. Hunt for: inconsistent patterns across commits, dead code left by one commit after another refactored, duplicated logic that should share an abstraction, missing tests for new code paths."`
	}

	return bugs + "\n" + security + "\n" + perf + "\n" + style
}

func guidelinesSection(guidelines string) string {
	guidelines = strings.TrimSpace(guidelines)
	if guidelines == "" {
		return ""
	}
	return fmt.Sprintf("\n\nProject-specific guidelines (apply these during review):\n%s", guidelines)
}
