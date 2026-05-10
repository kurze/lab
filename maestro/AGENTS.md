# Maestro

AI coding workflow orchestrator. Go module at `maestro/`.

## Build and test

```bash
go build ./cmd/maestro/
go test ./...
```

## Module layout

- `cmd/maestro/` — CLI entrypoint, TUI, orchestrator logic
- `internal/fsm/` — state machine (GRILL -> PLAN -> CODE -> AI_REVIEW -> AI_FIX -> LOCAL_REVIEW -> PUSH -> ABANDONED)
- `internal/task/` — task persistence in `.maestro/` directory, JSON serialization
- `internal/config/` — YAML config loading from `~/.config/maestro/config.yaml`
- `internal/workspace/` — git worktree create/remove/resolve
- `internal/agent/` — `Agent` interface with `claude/` and `local/` implementations
- `internal/grill/` — interactive interview session producing a PRD
- `internal/planner/` — PRD to DAG decomposition, topological sort, cycle detection
- `internal/coder/` — executes sub-tasks in dependency order, commits per task
- `internal/reviewer/` — AI review with PASS/NEEDS_FIX/BLOCKER verdict, fix loop
- `internal/tracker/` — Jira REST API client + NoopTracker for offline mode

## Key interfaces

- `agent.Agent`: `Run(ctx, workDir, prompt) -> (string, error)` — used by grill, planner, coder, reviewer, fixer
- `tracker.Tracker`: `Create`, `Update`, `Transition`, `GetStatus` — Jira or noop
- `fsm.Transition(from, to)` — validates state machine edges

## Dependencies

- `agentcore` (local module at `../agentcore`) — LLM client, agent loop, tool dispatch, JSON extraction
- `bubbletea` + `lipgloss` — TUI
- `gopkg.in/yaml.v3` — config

## Conventions

- Task IDs: `m-YYYYMMDD-XXXXXX` (date + 6 random hex chars)
- Branch naming: `maestro/<jira-key>-<slug>` or `maestro/<local-id>`
- Plan saved to `.maestro/<id>/plan.json`, not the worktree
- All Jira calls are best-effort — errors logged, never block the FSM
- ABANDONED is reachable from any non-terminal state
