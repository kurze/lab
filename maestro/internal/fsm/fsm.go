package fsm

import "fmt"

type State string

const (
	Grill       State = "GRILL"
	Plan        State = "PLAN"
	Code        State = "CODE"
	AIReview    State = "AI_REVIEW"
	AIFix       State = "AI_FIX"
	LocalReview State = "LOCAL_REVIEW"
	Push        State = "PUSH"
	Abandoned   State = "ABANDONED"
)

type edge struct{ from, to State }

var allowed = map[edge]bool{
	{Grill, Plan}:           true,
	{Grill, Abandoned}:      true,
	{Plan, Code}:            true,
	{Plan, Plan}:            true,
	{Plan, Abandoned}:       true,
	{Code, AIReview}:        true,
	{Code, Abandoned}:       true,
	{AIReview, AIFix}:       true,
	{AIReview, LocalReview}: true,
	{AIReview, Abandoned}:   true,
	{AIFix, AIReview}:       true,
	{AIFix, Abandoned}:      true,
	{LocalReview, Push}:     true,
	{LocalReview, AIFix}:    true,
	{LocalReview, Abandoned}: true,
}

func Transition(from, to State) error {
	if allowed[edge{from, to}] {
		return nil
	}
	return fmt.Errorf("invalid transition: %s → %s", from, to)
}
