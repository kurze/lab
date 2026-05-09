package main

import (
	"fmt"
	"strconv"
	"strings"
)

func parseLocation(loc string) (file string, line int, ok bool) {
	idx := strings.LastIndexByte(loc, ':')
	if idx <= 0 || idx == len(loc)-1 {
		return "", 0, false
	}
	n, err := strconv.Atoi(loc[idx+1:])
	if err != nil || n <= 0 {
		return "", 0, false
	}
	f := loc[:idx]
	if strings.ContainsAny(f, " \t") {
		return "", 0, false
	}
	return f, n, true
}

func routeFindings(findings []Finding) (inline []InlineComment, summary []Finding) {
	for _, f := range findings {
		file, line, ok := parseLocation(f.Location)
		if !ok {
			summary = append(summary, f)
			continue
		}
		inline = append(inline, InlineComment{
			File: file,
			Line: line,
			Body: formatInlineBody(f),
		})
	}
	return
}

func formatInlineBody(f Finding) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**[%s] %s** — %s", f.Severity, f.Category, f.Description)
	if f.Evidence != "" {
		fmt.Fprintf(&b, "\n\n> %s", f.Evidence)
	}
	return b.String()
}
