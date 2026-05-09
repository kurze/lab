package main

import (
	"fmt"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var severityOrder = []string{"critical", "major", "minor", "info"}

func formatFooter(model string) string {
	footer := "*Automated review by [gitlab-reviewer](https://github.com/kurze/lab/tree/main/gitlab-reviewer)"
	if model != "" {
		footer += fmt.Sprintf(" · agent: %s", model)
	}
	footer += "*"
	return footer
}

func formatFindingsBody(b *strings.Builder, findings []Finding) {
	groups := map[string][]Finding{}
	for _, f := range findings {
		sev := strings.ToLower(f.Severity)
		groups[sev] = append(groups[sev], f)
	}
	for _, sev := range severityOrder {
		fs, ok := groups[sev]
		if !ok {
			continue
		}
		fmt.Fprintf(b, "\n### %s\n\n", cases.Title(language.English).String(sev))
		for _, f := range fs {
			formatFinding(b, f)
		}
	}
}

func formatCommitResultsBody(b *strings.Builder, commitResults []CommitReviewResult) {
	for _, cr := range commitResults {
		sha := cr.Commit.SHA
		if len(sha) > 8 {
			sha = sha[:8]
		}
		msg := cr.Commit.Message
		if idx := strings.IndexByte(msg, '\n'); idx > 0 {
			msg = msg[:idx]
		}
		fmt.Fprintf(b, "\n### Commit `%s` — %s\n", sha, msg)

		if cr.Result == nil || len(cr.Result.Findings) == 0 {
			b.WriteString("\n_No findings_\n")
			continue
		}

		groups := map[string][]Finding{}
		for _, f := range cr.Result.Findings {
			sev := strings.ToLower(f.Severity)
			groups[sev] = append(groups[sev], f)
		}
		for _, sev := range severityOrder {
			fs, ok := groups[sev]
			if !ok {
				continue
			}
			fmt.Fprintf(b, "\n#### %s\n\n", cases.Title(language.English).String(sev))
			for _, f := range fs {
				formatFinding(b, f)
			}
		}
	}
}

func formatFinding(b *strings.Builder, f Finding) {
	fmt.Fprintf(b, "- **%s**", f.Category)
	if f.Location != "" {
		fmt.Fprintf(b, " `%s`", f.Location)
	}
	fmt.Fprintf(b, " — %s", f.Description)
	if f.Evidence != "" {
		fmt.Fprintf(b, " (%s)", f.Evidence)
	}
	b.WriteByte('\n')
}

func FormatCommitReviewComment(commitResults []CommitReviewResult, mrTitle, model string) string {
	totalFindings := 0
	commitsWithFindings := 0
	for _, cr := range commitResults {
		if cr.Result != nil && len(cr.Result.Findings) > 0 {
			totalFindings += len(cr.Result.Findings)
			commitsWithFindings++
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "## AI Review (commit-by-commit)\n\n")

	if totalFindings == 0 {
		fmt.Fprintf(&b, "No findings across %d commit(s) for \"%s\"\n", len(commitResults), mrTitle)
		b.WriteString("\n---\n")
		b.WriteString(formatFooter(model))
		b.WriteByte('\n')
		return b.String()
	}

	fmt.Fprintf(&b, "**%d finding(s) across %d commit(s)** for \"%s\"\n", totalFindings, commitsWithFindings, mrTitle)
	formatCommitResultsBody(&b, commitResults)

	b.WriteString("\n---\n")
	b.WriteString(formatFooter(model))
	b.WriteByte('\n')
	return b.String()
}

func FormatComment(result *ReviewResult, mrTitle string) string {
	if len(result.Findings) == 0 {
		return "## AI Review\n\nNo findings for this merge request.\n\n---\n" + formatFooter(result.Model)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "## AI Review\n\n**%d finding(s)** for \"%s\"\n", len(result.Findings), mrTitle)
	formatFindingsBody(&b, result.Findings)

	b.WriteString("\n---\n")
	b.WriteString(formatFooter(result.Model))
	b.WriteByte('\n')
	return b.String()
}

func FormatBothReviewComment(commitResults []CommitReviewResult, branchResult *ReviewResult, mrTitle, model string) string {
	commitFindings := 0
	for _, cr := range commitResults {
		if cr.Result != nil {
			commitFindings += len(cr.Result.Findings)
		}
	}
	branchFindings := 0
	if branchResult != nil {
		branchFindings = len(branchResult.Findings)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "## AI Review (commit-by-commit + branch-level)\n\n")
	fmt.Fprintf(&b, "**%d per-commit finding(s), %d branch-level finding(s)** for \"%s\"\n", commitFindings, branchFindings, mrTitle)

	if commitFindings > 0 {
		formatCommitResultsBody(&b, commitResults)
	}

	b.WriteString("\n### Branch-level findings\n")
	if branchFindings > 0 {
		groups := map[string][]Finding{}
		for _, f := range branchResult.Findings {
			sev := strings.ToLower(f.Severity)
			groups[sev] = append(groups[sev], f)
		}
		for _, sev := range severityOrder {
			fs, ok := groups[sev]
			if !ok {
				continue
			}
			fmt.Fprintf(&b, "\n#### %s\n\n", cases.Title(language.English).String(sev))
			for _, f := range fs {
				formatFinding(&b, f)
			}
		}
	} else {
		b.WriteString("\n_No additional findings_\n")
	}

	b.WriteString("\n---\n")
	b.WriteString(formatFooter(model))
	b.WriteByte('\n')
	return b.String()
}
