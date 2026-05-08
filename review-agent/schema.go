package main

type Finding struct {
	Category    string `json:"category"`
	Severity    string `json:"severity"`
	Location    string `json:"location"`
	Description string `json:"description"`
	Evidence    string `json:"evidence"`
}

type ReviewResult struct {
	Findings       []Finding `json:"findings"`
	OpenQuestions  []string  `json:"open_questions"`
	ContextPulled  []string  `json:"context_pulled"`
	IterationsUsed int       `json:"iterations_used"`
	Truncated      bool      `json:"truncated"`
}

var findingsJSONSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"findings": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"category": map[string]any{
						"type": "string",
						"enum": []string{"missing", "inconsistent", "risk", "assumption", "pattern_match"},
					},
					"severity": map[string]any{
						"type": "string",
						"enum": []string{"info", "minor", "major"},
					},
					"location":    map[string]any{"type": "string"},
					"description": map[string]any{"type": "string"},
					"evidence":    map[string]any{"type": "string"},
				},
				"required":             []string{"category", "severity", "location", "description", "evidence"},
				"additionalProperties": false,
			},
		},
		"open_questions": map[string]any{
			"type":  "array",
			"items": map[string]any{"type": "string"},
		},
	},
	"required":             []string{"findings", "open_questions"},
	"additionalProperties": false,
}
