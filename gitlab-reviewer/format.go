package main

import (
	"fmt"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func formatFooter(model string) string {
	footer := "*Automated review by [gitlab-reviewer](https://github.com/kurze/lab/tree/main/gitlab-reviewer)"
	if model != "" {
		footer += fmt.Sprintf(" · agent: %s", model)
	}
	footer += "*"
	return footer
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

	for _, cr := range commitResults {
		sha := cr.Commit.SHA
		if len(sha) > 8 {
			sha = sha[:8]
		}
		msg := cr.Commit.Message
		if idx := strings.IndexByte(msg, '\n'); idx > 0 {
			msg = msg[:idx]
		}
		fmt.Fprintf(&b, "\n### Commit `%s` — %s\n", sha, msg)

		if cr.Result == nil || len(cr.Result.Findings) == 0 {
			b.WriteString("\n_No findings_\n")
			continue
		}

		groups := map[string][]Finding{}
		order := []string{"critical", "major", "minor", "info"}
		for _, f := range cr.Result.Findings {
			sev := strings.ToLower(f.Severity)
			groups[sev] = append(groups[sev], f)
		}

		for _, sev := range order {
			findings, ok := groups[sev]
			if !ok {
				continue
			}
			fmt.Fprintf(&b, "\n#### %s\n\n", cases.Title(language.English).String(sev))
			for _, f := range findings {
				fmt.Fprintf(&b, "- **%s**", f.Category)
				if f.Location != "" {
					fmt.Fprintf(&b, " `%s`", f.Location)
				}
				fmt.Fprintf(&b, " — %s", f.Description)
				if f.Evidence != "" {
					fmt.Fprintf(&b, " (%s)", f.Evidence)
				}
				b.WriteByte('\n')
			}
		}
	}

	b.WriteString("\n---\n")
	b.WriteString(formatFooter(model))
	b.WriteByte('\n')
	return b.String()
}

func FormatComment(result *ReviewResult, mrTitle string) string {
	if len(result.Findings) == 0 {
		return "## AI Review\n\nNo findings for this merge request.\n\n---\n" + formatFooter(result.Model)
	}

	groups := map[string][]Finding{}
	order := []string{"critical", "major", "minor", "info"}
	for _, f := range result.Findings {
		sev := strings.ToLower(f.Severity)
		groups[sev] = append(groups[sev], f)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "## AI Review\n\n**%d finding(s)** for \"%s\"\n", len(result.Findings), mrTitle)

	for _, sev := range order {
		findings, ok := groups[sev]
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "\n### %s\n\n", cases.Title(language.English).String(sev))
		for _, f := range findings {
			fmt.Fprintf(&b, "- **%s**", f.Category)
			if f.Location != "" {
				fmt.Fprintf(&b, " `%s`", f.Location)
			}
			fmt.Fprintf(&b, " — %s", f.Description)
			if f.Evidence != "" {
				fmt.Fprintf(&b, " (%s)", f.Evidence)
			}
			b.WriteByte('\n')
		}
	}

	b.WriteString("\n---\n")
	b.WriteString(formatFooter(result.Model))
	b.WriteByte('\n')
	return b.String()
}
