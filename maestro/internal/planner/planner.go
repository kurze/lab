package planner

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kurze/lab/agentcore"

	"maestro/internal/agent"
)

const plannerPrompt = `You are a planner agent. Your job is to decompose a PRD (Product Requirements Document) into a task graph that a coding agent will execute.

## Instructions

1. Read the PRD below carefully.
2. Inspect the codebase file structure to produce accurate file hints.
3. Decompose the work into sub-tasks. Each sub-task must:
   - Have a unique short ID (t1, t2, t3, ...).
   - Include a clear title and description.
   - List dependencies on other sub-tasks (by ID).
   - Include a files_hint array listing the files the coding agent will need to read or create.
   - Include an estimated_tokens count (prompt + file contents + output headroom). Each task MUST stay under the token budget of %d tokens.
4. The tasks must form a valid DAG (directed acyclic graph) — no cycles.
5. Order dependencies so independent tasks can run in parallel where possible.

## Output format

Respond with ONLY a JSON object matching this schema (no markdown, no explanation):

{
  "token_budget": %d,
  "tasks": [
    {
      "id": "t1",
      "title": "string",
      "description": "string",
      "depends_on": [],
      "status": "pending",
      "files_hint": ["path/to/file.go"],
      "estimated_tokens": 45000
    }
  ]
}

## PRD

%s`

// Plan invokes the planner agent to decompose a PRD into a task graph.
// It parses the agent output into a Graph, validates it, and returns the result.
func Plan(ctx context.Context, a agent.Agent, prdContent string, tokenBudget int) (*Graph, error) {
	prompt := fmt.Sprintf(plannerPrompt, tokenBudget, tokenBudget, prdContent)

	output, err := a.Run(ctx, "", prompt)
	if err != nil {
		return nil, fmt.Errorf("planner agent failed: %w", err)
	}

	graph, err := parseGraph(output)
	if err != nil {
		return nil, fmt.Errorf("failed to parse planner output: %w", err)
	}

	// Override token_budget with the caller's value in case the model changed it.
	graph.TokenBudget = tokenBudget

	if err := graph.Validate(); err != nil {
		return nil, fmt.Errorf("planner produced invalid graph: %w", err)
	}

	return graph, nil
}

// parseGraph extracts a Graph from raw agent output. It uses
// agentcore.ExtractJSON to handle markdown code fences and leading text,
// then falls back to agentcore.RepairJSON-style manual cleanup.
func parseGraph(output string) (*Graph, error) {
	extracted := agentcore.ExtractJSON(output)

	var g Graph
	if err := json.Unmarshal([]byte(extracted), &g); err != nil {
		return nil, fmt.Errorf("json unmarshal failed: %w\nraw output:\n%s", err, truncate(output, 500))
	}

	// Set default status for tasks that don't have one.
	for i := range g.Tasks {
		if g.Tasks[i].Status == "" {
			g.Tasks[i].Status = StatusPending
		}
		if g.Tasks[i].DependsOn == nil {
			g.Tasks[i].DependsOn = []string{}
		}
		if g.Tasks[i].FilesHint == nil {
			g.Tasks[i].FilesHint = []string{}
		}
	}

	return &g, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
