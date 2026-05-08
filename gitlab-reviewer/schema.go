package main

type Finding struct {
	Category    string `json:"category"`
	Severity    string `json:"severity"`
	Location    string `json:"location"`
	Description string `json:"description"`
	Evidence    string `json:"evidence"`
}

type ReviewResult struct {
	Findings []Finding `json:"findings"`
}
