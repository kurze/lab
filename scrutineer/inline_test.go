package main

import (
	"testing"
)

func TestParseLocation(t *testing.T) {
	tests := []struct {
		loc      string
		wantFile string
		wantLine int
		wantOK   bool
	}{
		{"main.go:42", "main.go", 42, true},
		{"src/pkg/foo.go:1", "src/pkg/foo.go", 1, true},
		{"no-colon", "", 0, false},
		{"trailing:", "", 0, false},
		{":5", "", 0, false},
		{"file.go:0", "", 0, false},
		{"file.go:-1", "", 0, false},
		{"file.go:abc", "", 0, false},
		{"path with spaces/file.go:10", "", 0, false},
		{"path\twith\ttabs/file.go:10", "", 0, false},
	}

	for _, tt := range tests {
		file, line, ok := parseLocation(tt.loc)
		if ok != tt.wantOK || file != tt.wantFile || line != tt.wantLine {
			t.Errorf("parseLocation(%q) = (%q, %d, %v), want (%q, %d, %v)",
				tt.loc, file, line, ok, tt.wantFile, tt.wantLine, tt.wantOK)
		}
	}
}

func TestRouteFindings(t *testing.T) {
	findings := []Finding{
		{Severity: "critical", Location: "main.go:10", Category: "bug", Description: "critical bug"},
		{Severity: "major", Location: "main.go:20", Category: "bug", Description: "major bug"},
		{Severity: "minor", Location: "main.go:30", Category: "style", Description: "minor style"},
		{Severity: "info", Location: "main.go:40", Category: "note", Description: "info note"},
		{Severity: "major", Location: "no-location", Category: "bug", Description: "no line"},
	}

	inline, summary := routeFindings(findings, "major")
	if len(inline) != 2 {
		t.Errorf("expected 2 inline comments, got %d", len(inline))
	}
	if len(summary) != 3 {
		t.Errorf("expected 3 summary findings, got %d", len(summary))
	}

	// default threshold is "minor"
	inline2, summary2 := routeFindings(findings, "")
	if len(inline2) != 3 {
		t.Errorf("default threshold: expected 3 inline, got %d", len(inline2))
	}
	if len(summary2) != 2 {
		t.Errorf("default threshold: expected 2 summary, got %d", len(summary2))
	}
}

func TestFormatInlineBody(t *testing.T) {
	f := Finding{Severity: "major", Category: "bug", Description: "null pointer"}
	body := formatInlineBody(f)
	if body != "**[major] bug** — null pointer" {
		t.Errorf("unexpected body: %s", body)
	}

	f.Evidence = "ptr == nil"
	body = formatInlineBody(f)
	want := "**[major] bug** — null pointer\n\n> ptr == nil"
	if body != want {
		t.Errorf("unexpected body with evidence: %s", body)
	}
}
