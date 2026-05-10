package fsm

import "testing"

func TestValidTransitions(t *testing.T) {
	valid := []struct{ from, to State }{
		{Grill, Plan},
		{Grill, Abandoned},
		{Plan, Code},
		{Plan, Plan},
		{Plan, Abandoned},
		{Code, AIReview},
		{Code, Abandoned},
		{AIReview, AIFix},
		{AIReview, LocalReview},
		{AIReview, Abandoned},
		{AIFix, AIReview},
		{AIFix, Abandoned},
		{LocalReview, Push},
		{LocalReview, AIFix},
		{LocalReview, Abandoned},
	}
	for _, tc := range valid {
		if err := Transition(tc.from, tc.to); err != nil {
			t.Errorf("expected %s → %s to succeed, got: %v", tc.from, tc.to, err)
		}
	}
}

func TestInvalidTransitions(t *testing.T) {
	invalid := []struct{ from, to State }{
		{Grill, Code},
		{Grill, AIReview},
		{Grill, Push},
		{Plan, AIReview},
		{Plan, Push},
		{Plan, Grill},
		{Code, Plan},
		{Code, Push},
		{Code, Grill},
		{AIReview, Code},
		{AIReview, Push},
		{AIReview, Grill},
		{AIFix, Code},
		{AIFix, Plan},
		{AIFix, Push},
		{LocalReview, Code},
		{LocalReview, Plan},
		{LocalReview, Grill},
		{Push, Grill},
		{Push, Code},
		{Push, AIReview},
		{Push, Abandoned},
		{Abandoned, Grill},
		{Abandoned, Plan},
		{Abandoned, Code},
	}
	for _, tc := range invalid {
		if err := Transition(tc.from, tc.to); err == nil {
			t.Errorf("expected %s → %s to fail, but it succeeded", tc.from, tc.to)
		}
	}
}
