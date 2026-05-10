package task

import (
	"maestro/internal/fsm"
	"path/filepath"
	"strings"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	return NewStore(filepath.Join(t.TempDir(), ".maestro"))
}

func TestCreateAndLoad(t *testing.T) {
	s := newTestStore(t)

	task, err := s.Create("test task")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if !strings.HasPrefix(task.ID, "m-") {
		t.Errorf("id format: got %s, want m-YYYYMMDD-XXXX", task.ID)
	}
	if task.Title != "test task" {
		t.Errorf("title: got %q, want %q", task.Title, "test task")
	}
	if task.State != fsm.Grill {
		t.Errorf("state: got %s, want %s", task.State, fsm.Grill)
	}
	if task.MaxReviewIterations != 2 {
		t.Errorf("max_review_iterations: got %d, want 2", task.MaxReviewIterations)
	}

	loaded, err := s.Load(task.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.ID != task.ID {
		t.Errorf("loaded id: got %s, want %s", loaded.ID, task.ID)
	}
	if loaded.Title != task.Title {
		t.Errorf("loaded title: got %q, want %q", loaded.Title, task.Title)
	}
}

func TestList(t *testing.T) {
	s := newTestStore(t)

	for _, title := range []string{"alpha", "beta", "gamma"} {
		if _, err := s.Create(title); err != nil {
			t.Fatalf("create %s: %v", title, err)
		}
	}

	tasks, err := s.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(tasks) != 3 {
		t.Errorf("list count: got %d, want 3", len(tasks))
	}
}

func TestResolveByID(t *testing.T) {
	s := newTestStore(t)

	task, err := s.Create("by id")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	resolved, err := s.Resolve(task.ID)
	if err != nil {
		t.Fatalf("resolve by id: %v", err)
	}
	if resolved.ID != task.ID {
		t.Errorf("resolved id: got %s, want %s", resolved.ID, task.ID)
	}
}

func TestResolveByJiraKey(t *testing.T) {
	s := newTestStore(t)

	task, err := s.Create("by jira")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	jiraKey := "TRUS-42"
	task.JiraID = &jiraKey
	if err := s.Save(task); err != nil {
		t.Fatalf("save with jira_id: %v", err)
	}

	resolved, err := s.Resolve("TRUS-42")
	if err != nil {
		t.Fatalf("resolve by jira key: %v", err)
	}
	if resolved.ID != task.ID {
		t.Errorf("resolved id: got %s, want %s", resolved.ID, task.ID)
	}
}

func TestResolveNotFound(t *testing.T) {
	s := newTestStore(t)

	_, err := s.Resolve("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent task")
	}
}

func TestTransitionValid(t *testing.T) {
	s := newTestStore(t)

	task, err := s.Create("transitions")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := task.Transition(fsm.Plan); err != nil {
		t.Fatalf("grill→plan: %v", err)
	}
	if task.State != fsm.Plan {
		t.Errorf("state after transition: got %s, want %s", task.State, fsm.Plan)
	}
	if len(task.Transitions) != 1 {
		t.Fatalf("transitions count: got %d, want 1", len(task.Transitions))
	}
	if task.Transitions[0].From != fsm.Grill || task.Transitions[0].To != fsm.Plan {
		t.Errorf("transition record: got %s→%s, want %s→%s",
			task.Transitions[0].From, task.Transitions[0].To, fsm.Grill, fsm.Plan)
	}

	if err := s.Save(task); err != nil {
		t.Fatalf("save after transition: %v", err)
	}
	loaded, err := s.Load(task.ID)
	if err != nil {
		t.Fatalf("load after transition: %v", err)
	}
	if loaded.State != fsm.Plan {
		t.Errorf("loaded state: got %s, want %s", loaded.State, fsm.Plan)
	}
}

func TestTransitionInvalid(t *testing.T) {
	s := newTestStore(t)

	task, err := s.Create("invalid transition")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := task.Transition(fsm.Code); err == nil {
		t.Fatal("expected error for grill→code")
	}
	if task.State != fsm.Grill {
		t.Errorf("state should not change on invalid transition: got %s", task.State)
	}
}

func TestTransitionChain(t *testing.T) {
	s := newTestStore(t)

	task, err := s.Create("full chain")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	chain := []fsm.State{fsm.Plan, fsm.Code, fsm.AIReview, fsm.AIFix, fsm.AIReview, fsm.LocalReview, fsm.Push}
	for _, next := range chain {
		if err := task.Transition(next); err != nil {
			t.Fatalf("%s→%s: %v", task.State, next, err)
		}
	}

	if err := s.Save(task); err != nil {
		t.Fatalf("save: %v", err)
	}
	if len(task.Transitions) != len(chain) {
		t.Errorf("transitions count: got %d, want %d", len(task.Transitions), len(chain))
	}
}

func TestListEmpty(t *testing.T) {
	s := newTestStore(t)

	tasks, err := s.List()
	if err != nil {
		t.Fatalf("list empty: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected empty list, got %d", len(tasks))
	}
}
