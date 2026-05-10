package grill

import "strings"

const prdGenerationPrompt = `Based on our conversation above, produce a complete PRD in markdown format. Include ALL of these sections:

# Title
(A concise title for the feature/change)

## Problem Statement
(What problem are we solving and why)

## User Stories
(Numbered list of "As a ... I want ... so that ..." stories)

## Acceptance Criteria
(Numbered list of testable criteria)

## Scope Exclusions
(What is explicitly NOT in scope)

## Implementation Notes
(Technical approach, key decisions made during the interview, constraints)

Output ONLY the PRD markdown. No preamble, no commentary.`

// requiredSections lists the headings that a valid PRD must contain.
var requiredSections = []string{
	"# ",
	"## Problem Statement",
	"## User Stories",
	"## Acceptance Criteria",
	"## Scope Exclusions",
	"## Implementation Notes",
}

// FormatPRD validates that the model output contains all required PRD sections.
// If a section is missing, it appends a placeholder. Returns the final PRD text.
func FormatPRD(raw string) string {
	trimmed := strings.TrimSpace(raw)

	// Strip markdown code fences if the model wrapped its output.
	if strings.HasPrefix(trimmed, "```") {
		lines := strings.Split(trimmed, "\n")
		// Remove first line (```markdown or ```)
		if len(lines) > 1 {
			lines = lines[1:]
		}
		// Remove last line if it's a closing fence.
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "```" {
			lines = lines[:len(lines)-1]
		}
		trimmed = strings.Join(lines, "\n")
	}

	// Check for each required section and append placeholders for missing ones.
	for _, section := range requiredSections {
		if section == "# " {
			// Title: just needs any top-level heading.
			if !strings.Contains(trimmed, "\n# ") && !strings.HasPrefix(trimmed, "# ") {
				trimmed = "# Untitled\n\n" + trimmed
			}
			continue
		}
		if !strings.Contains(trimmed, section) {
			trimmed += "\n\n" + section + "\n\nTODO: This section was not generated. Please fill in manually.\n"
		}
	}

	return trimmed + "\n"
}
