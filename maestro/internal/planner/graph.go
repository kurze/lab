package planner

import (
	"fmt"
	"strings"
)

// TaskStatus represents the execution state of a sub-task in the plan.
type TaskStatus string

const (
	StatusPending    TaskStatus = "pending"
	StatusInProgress TaskStatus = "in_progress"
	StatusDone       TaskStatus = "done"
	StatusFailed     TaskStatus = "failed"
)

// Task is a single sub-task in the plan DAG, matching the plan.json schema.
type Task struct {
	ID              string     `json:"id"`
	Title           string     `json:"title"`
	Description     string     `json:"description"`
	DependsOn       []string   `json:"depends_on"`
	Status          TaskStatus `json:"status"`
	FilesHint       []string   `json:"files_hint"`
	EstimatedTokens int        `json:"estimated_tokens"`
}

// Graph is a DAG of sub-tasks decomposed from a PRD.
type Graph struct {
	TokenBudget int    `json:"token_budget"`
	Tasks       []Task `json:"tasks"`
}

// Validate checks that the graph is well-formed:
//   - No cycles in the dependency graph
//   - All depends_on references point to existing task IDs
//   - No task's estimated_tokens exceeds the token budget
func (g *Graph) Validate() error {
	ids := make(map[string]int, len(g.Tasks))
	for i, t := range g.Tasks {
		if _, dup := ids[t.ID]; dup {
			return fmt.Errorf("duplicate task id: %s", t.ID)
		}
		ids[t.ID] = i
	}

	// Check all depends_on references exist.
	for _, t := range g.Tasks {
		for _, dep := range t.DependsOn {
			if _, ok := ids[dep]; !ok {
				return fmt.Errorf("task %s depends on unknown task %s", t.ID, dep)
			}
		}
	}

	// Check no estimated_tokens exceeds budget.
	for _, t := range g.Tasks {
		if t.EstimatedTokens > g.TokenBudget {
			return fmt.Errorf("task %s estimated_tokens (%d) exceeds budget (%d)", t.ID, t.EstimatedTokens, g.TokenBudget)
		}
	}

	// Check for cycles using Kahn's algorithm (also used by TopologicalSort).
	if _, err := g.topologicalSort(); err != nil {
		return err
	}

	return nil
}

// TopologicalSort returns the tasks in dependency order. Tasks with no
// unresolved dependencies come first. Returns an error if the graph
// contains a cycle.
func (g *Graph) TopologicalSort() ([]Task, error) {
	return g.topologicalSort()
}

func (g *Graph) topologicalSort() ([]Task, error) {
	n := len(g.Tasks)
	if n == 0 {
		return nil, nil
	}

	idIdx := make(map[string]int, n)
	for i, t := range g.Tasks {
		idIdx[t.ID] = i
	}

	inDegree := make([]int, n)
	adj := make([][]int, n)
	for i, t := range g.Tasks {
		for _, dep := range t.DependsOn {
			j, ok := idIdx[dep]
			if !ok {
				return nil, fmt.Errorf("task %s depends on unknown task %s", t.ID, dep)
			}
			adj[j] = append(adj[j], i)
			inDegree[i]++
		}
	}

	// Seed queue with zero in-degree nodes.
	queue := make([]int, 0, n)
	for i, d := range inDegree {
		if d == 0 {
			queue = append(queue, i)
		}
	}

	sorted := make([]Task, 0, n)
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		sorted = append(sorted, g.Tasks[cur])
		for _, next := range adj[cur] {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}

	if len(sorted) != n {
		// Find tasks involved in the cycle for a helpful error message.
		var cycled []string
		for i, d := range inDegree {
			if d > 0 {
				cycled = append(cycled, g.Tasks[i].ID)
			}
		}
		return nil, fmt.Errorf("cycle detected involving tasks: %s", strings.Join(cycled, ", "))
	}

	return sorted, nil
}
