package task

import (
	"encoding/json"
	"fmt"
	"maestro/internal/fsm"
	"time"
)

type TransitionRecord struct {
	From fsm.State `json:"from"`
	To   fsm.State `json:"to"`
	At   time.Time `json:"at"`
}

type Task struct {
	ID                  string             `json:"id"`
	JiraID              *string            `json:"jira_id"`
	Title               string             `json:"title"`
	State               fsm.State          `json:"state"`
	WorktreePath        *string            `json:"worktree_path"`
	BranchName          *string            `json:"branch_name"`
	ReviewIteration     int                `json:"review_iteration"`
	MaxReviewIterations int                `json:"max_review_iterations"`
	CreatedAt           time.Time          `json:"created_at"`
	Transitions         []TransitionRecord `json:"transitions"`
}

func (t *Task) Transition(newState fsm.State) error {
	if err := fsm.Transition(t.State, newState); err != nil {
		return fmt.Errorf("task %s: %w", t.ID, err)
	}
	t.Transitions = append(t.Transitions, TransitionRecord{
		From: t.State,
		To:   newState,
		At:   time.Now().UTC(),
	})
	t.State = newState
	return nil
}

func (t *Task) MarshalJSON() ([]byte, error) {
	type Alias Task
	return json.Marshal((*Alias)(t))
}

func (t *Task) UnmarshalJSON(data []byte) error {
	type Alias Task
	return json.Unmarshal(data, (*Alias)(t))
}
