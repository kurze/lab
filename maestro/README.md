# maestro

CLI orchestrator that chains AI coding phases: grill (interview) -> plan (DAG) -> code -> review -> fix -> push. Persists state in `.maestro/`, optionally syncs with Jira.

## Install

```bash
cd maestro && go build -o maestro ./cmd/maestro/
```

## Quick start

```bash
# Start a new task (no Jira)
maestro new --no-jira "add health check endpoint"

# Interactive grill session runs, then produces a PRD and task graph.
# Review the plan:
maestro plan <id>

# Approve and run coding + review loop:
maestro approve <id>

# Check status (TUI):
maestro status

# Push when satisfied:
maestro push <id>
```

## Workflow

```
GRILL --> PLAN --> CODE --> AI_REVIEW --> AI_FIX --> LOCAL_REVIEW --> PUSH
(interactive) (gate)  (auto)   (auto)       (auto)     (gate)         (auto)
```

- **GRILL**: Interactive interview to produce a PRD. `/done` to proceed, `/abandon` to discard.
- **PLAN**: Agent decomposes PRD into a DAG of sub-tasks with token budgets and file hints. Manual gate: `maestro approve` or `maestro replan`.
- **CODE**: Sub-tasks executed in topological order, committed individually.
- **AI_REVIEW/AI_FIX**: Automated review/fix loop, bounded by `max_iterations`.
- **LOCAL_REVIEW**: Human reviews the diff. `maestro push` or `maestro rework`.

## Commands

| Command | Description |
|---------|-------------|
| `new "<title>"` | Start grill session for a new task |
| `new --jira KEY` | Attach to existing Jira ticket |
| `status` | TUI showing all tasks, DAG, details |
| `plan <id>` | Print task graph |
| `approve <id>` | Approve plan, start coding |
| `replan <id> [feedback]` | Regenerate plan with optional instructions |
| `review <id>` | Show diff and review reports |
| `push <id>` | Push branch, print MR URL |
| `rework <id> [feedback]` | Send back to AI fix |
| `abandon <id>` | Remove worktree, cancel task |
| `resume <id>` | Re-enter current phase |

## Configuration

`~/.config/maestro/config.yaml`:

```yaml
jira:
  base_url: "https://your-instance.atlassian.net"
  email: "you@company.com"
  api_token_env: "JIRA_API_TOKEN"
  project_key: "PROJ"

agents:
  planner:
    type: "claude-code"
    token_budget: 150000
  coder:
    type: "claude-code"
  reviewer:
    type: "local-llm"
    endpoint: "http://localhost:8080"
    model: "qwen3.5-9b"

review:
  max_iterations: 2
  base_branch: "origin/main"
  skill_path: ".claude/skills/review-branch.md"

workspace:
  worktree_dir: ".worktrees"
```

Jira config is optional. Omit it entirely or use `--no-jira` to skip Jira integration.

## Local state

All state lives in `.maestro/` (gitignored), one directory per task:

```
.maestro/
  m-20260510-a3f2e1/
    task.json       # FSM state, timestamps, transitions
    prd.md          # PRD from grill phase
    plan.json       # Sub-task DAG
    review-1.md     # AI review report
    fix-1.md        # AI fix summary
```

## Architecture

| Package | Purpose |
|---------|---------|
| `internal/fsm` | State machine with transition validation |
| `internal/task` | Task persistence in `.maestro/` |
| `internal/config` | YAML config with defaults and env var resolution |
| `internal/workspace` | Git worktree lifecycle |
| `internal/agent` | Agent interface + Claude Code and local LLM adapters |
| `internal/grill` | Interactive grill session |
| `internal/planner` | PRD to DAG decomposition with validation |
| `internal/coder` | Sub-task execution in topological order |
| `internal/reviewer` | AI review with verdict parsing + fix loop |
| `internal/tracker` | Jira integration with noop fallback |
| `cmd/maestro` | CLI, TUI (Bubble Tea), orchestrator |

Dependencies: `agentcore` (local, agent loop engine), `bubbletea`/`lipgloss` (TUI), `gopkg.in/yaml.v3`.
