package tracker_test

import (
	"context"
	"testing"

	"maestro/internal/fsm"
	"maestro/internal/tracker"
	"maestro/internal/tracker/jira"
)

// ---------------------------------------------------------------------------
// NoopTracker
// ---------------------------------------------------------------------------

func TestNoopTracker_Create(t *testing.T) {
	var tr tracker.Tracker = tracker.NoopTracker{}
	key, err := tr.Create(context.Background(), "title", "desc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "" {
		t.Fatalf("expected empty key, got %q", key)
	}
}

func TestNoopTracker_Update(t *testing.T) {
	var tr tracker.Tracker = tracker.NoopTracker{}
	if err := tr.Update(context.Background(), "X-1", map[string]any{"summary": "x"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNoopTracker_Transition(t *testing.T) {
	var tr tracker.Tracker = tracker.NoopTracker{}
	if err := tr.Transition(context.Background(), "X-1", "Done"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNoopTracker_GetStatus(t *testing.T) {
	var tr tracker.Tracker = tracker.NoopTracker{}
	status, err := tr.GetStatus(context.Background(), "X-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != "" {
		t.Fatalf("expected empty status, got %q", status)
	}
}

// ---------------------------------------------------------------------------
// MapState
// ---------------------------------------------------------------------------

func TestMapState_DefaultMapping(t *testing.T) {
	tests := []struct {
		state fsm.State
		want  string
	}{
		{fsm.Grill, "In Progress"},
		{fsm.Plan, "In Progress"},
		{fsm.Code, "In Progress"},
		{fsm.AIReview, "In Progress"},
		{fsm.AIFix, "In Progress"},
		{fsm.LocalReview, "In Review"},
		{fsm.Push, "In Review"},
		{fsm.Abandoned, ""},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			got := jira.MapState(tt.state, nil)
			if got != tt.want {
				t.Errorf("MapState(%s, nil) = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}

func TestMapState_CustomMapping(t *testing.T) {
	custom := map[fsm.State]string{
		fsm.Grill: "Backlog",
		fsm.Code:  "Development",
		fsm.Push:  "Done",
	}

	tests := []struct {
		state fsm.State
		want  string
	}{
		{fsm.Grill, "Backlog"},
		{fsm.Code, "Development"},
		{fsm.Push, "Done"},
		{fsm.Plan, ""},  // not in custom mapping
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			got := jira.MapState(tt.state, custom)
			if got != tt.want {
				t.Errorf("MapState(%s, custom) = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}

func TestBuildMapping_Empty(t *testing.T) {
	m := jira.BuildMapping(nil)
	if m == nil {
		t.Fatal("expected default mapping, got nil")
	}
	if m[fsm.Grill] != "In Progress" {
		t.Errorf("expected default for GRILL, got %q", m[fsm.Grill])
	}
}

func TestBuildMapping_FromConfig(t *testing.T) {
	raw := map[string]string{
		"GRILL":        "In Progress",
		"PLAN":         "In Progress",
		"CODE":         "In Progress",
		"AI_REVIEW":    "In Progress",
		"AI_FIX":       "In Progress",
		"LOCAL_REVIEW": "In Review",
		"PUSH":         "In Review",
	}
	m := jira.BuildMapping(raw)
	if m[fsm.LocalReview] != "In Review" {
		t.Errorf("expected 'In Review' for LOCAL_REVIEW, got %q", m[fsm.LocalReview])
	}
	if m[fsm.Code] != "In Progress" {
		t.Errorf("expected 'In Progress' for CODE, got %q", m[fsm.Code])
	}
}
