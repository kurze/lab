package planner

import (
	"testing"
)

func TestValidDAG(t *testing.T) {
	g := &Graph{
		TokenBudget: 150000,
		Tasks: []Task{
			{ID: "t1", Title: "First", DependsOn: []string{}, EstimatedTokens: 40000},
			{ID: "t2", Title: "Second", DependsOn: []string{"t1"}, EstimatedTokens: 80000},
			{ID: "t3", Title: "Third", DependsOn: []string{"t1"}, EstimatedTokens: 50000},
			{ID: "t4", Title: "Fourth", DependsOn: []string{"t2", "t3"}, EstimatedTokens: 60000},
		},
	}
	if err := g.Validate(); err != nil {
		t.Fatalf("expected valid DAG, got error: %v", err)
	}
}

func TestCycleDetection(t *testing.T) {
	g := &Graph{
		TokenBudget: 150000,
		Tasks: []Task{
			{ID: "t1", Title: "First", DependsOn: []string{"t3"}, EstimatedTokens: 40000},
			{ID: "t2", Title: "Second", DependsOn: []string{"t1"}, EstimatedTokens: 40000},
			{ID: "t3", Title: "Third", DependsOn: []string{"t2"}, EstimatedTokens: 40000},
		},
	}
	err := g.Validate()
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
	if got := err.Error(); !contains(got, "cycle") {
		t.Fatalf("expected cycle error, got: %s", got)
	}
}

func TestMissingDependency(t *testing.T) {
	g := &Graph{
		TokenBudget: 150000,
		Tasks: []Task{
			{ID: "t1", Title: "First", DependsOn: []string{"t99"}, EstimatedTokens: 40000},
		},
	}
	err := g.Validate()
	if err == nil {
		t.Fatal("expected missing dependency error, got nil")
	}
	if got := err.Error(); !contains(got, "unknown task t99") {
		t.Fatalf("expected unknown task error, got: %s", got)
	}
}

func TestOverBudgetTask(t *testing.T) {
	g := &Graph{
		TokenBudget: 100000,
		Tasks: []Task{
			{ID: "t1", Title: "Too big", DependsOn: []string{}, EstimatedTokens: 200000},
		},
	}
	err := g.Validate()
	if err == nil {
		t.Fatal("expected over-budget error, got nil")
	}
	if got := err.Error(); !contains(got, "exceeds budget") {
		t.Fatalf("expected budget error, got: %s", got)
	}
}

func TestDuplicateTaskID(t *testing.T) {
	g := &Graph{
		TokenBudget: 150000,
		Tasks: []Task{
			{ID: "t1", Title: "First", DependsOn: []string{}, EstimatedTokens: 40000},
			{ID: "t1", Title: "Duplicate", DependsOn: []string{}, EstimatedTokens: 40000},
		},
	}
	err := g.Validate()
	if err == nil {
		t.Fatal("expected duplicate id error, got nil")
	}
	if got := err.Error(); !contains(got, "duplicate") {
		t.Fatalf("expected duplicate error, got: %s", got)
	}
}

func TestTopologicalSort(t *testing.T) {
	g := &Graph{
		TokenBudget: 150000,
		Tasks: []Task{
			{ID: "t1", Title: "First", DependsOn: []string{}, EstimatedTokens: 40000},
			{ID: "t2", Title: "Second", DependsOn: []string{"t1"}, EstimatedTokens: 80000},
			{ID: "t3", Title: "Third", DependsOn: []string{"t1"}, EstimatedTokens: 50000},
			{ID: "t4", Title: "Fourth", DependsOn: []string{"t2", "t3"}, EstimatedTokens: 60000},
		},
	}
	sorted, err := g.TopologicalSort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sorted) != 4 {
		t.Fatalf("expected 4 tasks, got %d", len(sorted))
	}

	// Build position map to check ordering constraints.
	pos := make(map[string]int, len(sorted))
	for i, task := range sorted {
		pos[task.ID] = i
	}

	// t1 must come before t2, t3.
	if pos["t1"] >= pos["t2"] {
		t.Errorf("t1 (pos %d) should come before t2 (pos %d)", pos["t1"], pos["t2"])
	}
	if pos["t1"] >= pos["t3"] {
		t.Errorf("t1 (pos %d) should come before t3 (pos %d)", pos["t1"], pos["t3"])
	}
	// t2 and t3 must come before t4.
	if pos["t2"] >= pos["t4"] {
		t.Errorf("t2 (pos %d) should come before t4 (pos %d)", pos["t2"], pos["t4"])
	}
	if pos["t3"] >= pos["t4"] {
		t.Errorf("t3 (pos %d) should come before t4 (pos %d)", pos["t3"], pos["t4"])
	}
}

func TestTopologicalSortEmpty(t *testing.T) {
	g := &Graph{TokenBudget: 100000}
	sorted, err := g.TopologicalSort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sorted != nil {
		t.Fatalf("expected nil, got %v", sorted)
	}
}

func TestTopologicalSortCycle(t *testing.T) {
	g := &Graph{
		TokenBudget: 150000,
		Tasks: []Task{
			{ID: "a", Title: "A", DependsOn: []string{"b"}, EstimatedTokens: 10000},
			{ID: "b", Title: "B", DependsOn: []string{"a"}, EstimatedTokens: 10000},
		},
	}
	_, err := g.TopologicalSort()
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
