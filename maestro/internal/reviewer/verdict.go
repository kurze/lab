package reviewer

import (
	"bufio"
	"fmt"
	"strings"
)

// Verdict represents the outcome of an AI review.
type Verdict int

const (
	Pass     Verdict = iota // No issues found, proceed to local review.
	NeedsFix               // Issues found that the fix agent should address.
	Blocker                // Severe issues that require human intervention.
)

func (v Verdict) String() string {
	switch v {
	case Pass:
		return "PASS"
	case NeedsFix:
		return "NEEDS_FIX"
	case Blocker:
		return "BLOCKER"
	default:
		return fmt.Sprintf("Verdict(%d)", int(v))
	}
}

// ParseVerdict scans markdown text for a line containing "VERDICT: <value>"
// and returns the corresponding Verdict. The match is case-insensitive for
// the value. Returns an error if no verdict line is found or the value is
// unrecognised.
func ParseVerdict(markdown string) (Verdict, error) {
	scanner := bufio.NewScanner(strings.NewReader(markdown))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Strip leading markdown bold/heading markers so lines like
		// "**VERDICT: PASS**" or "## VERDICT: PASS" still match.
		line = strings.TrimLeft(line, "#* ")

		if !strings.HasPrefix(strings.ToUpper(line), "VERDICT:") {
			continue
		}

		// Extract the value after the colon.
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		val := strings.TrimSpace(parts[1])
		// Strip trailing markdown bold markers.
		val = strings.TrimRight(val, "* ")
		val = strings.ToUpper(val)

		switch val {
		case "PASS":
			return Pass, nil
		case "NEEDS_FIX":
			return NeedsFix, nil
		case "BLOCKER":
			return Blocker, nil
		default:
			return 0, fmt.Errorf("unrecognised verdict value: %q", val)
		}
	}

	return 0, fmt.Errorf("no VERDICT line found in review output")
}
