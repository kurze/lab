package main

import (
	"fmt"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func FormatComment(result *ReviewResult, mrTitle string) string {
	if len(result.Findings) == 0 {
		return "## AI Review\n\nNo findings for this merge request.\n\n---\n*Reviewed by gitlab-reviewer*"
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

	b.WriteString("\n---\n*Reviewed by gitlab-reviewer*\n")
	return b.String()
}
