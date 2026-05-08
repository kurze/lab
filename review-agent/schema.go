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
	ModelUsed      string    `json:"model_used"`
	ElapsedSec     float64   `json:"elapsed_sec"`
	TokensUsed     int       `json:"tokens_used"`
}

type GrillQuestion struct {
	Question   string `json:"question"`
	Why        string `json:"why"`
	Category   string `json:"category"`
	FollowedUp bool   `json:"followed_up"`
}

type GrillResult struct {
	Questions      []GrillQuestion `json:"questions"`
	ContextPulled  []string        `json:"context_pulled"`
	IterationsUsed int             `json:"iterations_used"`
	Truncated      bool            `json:"truncated"`
	ModelUsed      string          `json:"model_used"`
	ElapsedSec     float64         `json:"elapsed_sec"`
	TokensUsed     int             `json:"tokens_used"`
}

