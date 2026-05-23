package main

type Finding struct {
	Category    string `json:"category"`
	Severity    string `json:"severity"`
	Location    string `json:"location"`
	Description string `json:"description"`
	Evidence    string `json:"evidence"`
	CommitSHA   string `json:"commit_sha,omitempty"`
}

type ReviewResult struct {
	Findings   []Finding `json:"findings"`
	Model      string    `json:"-"`
	TokensUsed int       `json:"-"`
}

type CommitReviewResult struct {
	Commit Commit
	Result *ReviewResult
}
