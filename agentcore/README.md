# agentcore

Shared library for building LLM-powered agents with tool use. Used by `review-agent` and `gitlab-reviewer`.

## What it provides

| File | Purpose |
|------|---------|
| `llm.go` | HTTP client for OpenAI-compatible chat completions APIs (`LLMClient`, `ChatMessage`, `ChatRequest/Response`) |
| `loop.go` | Agentic loop engine — repeatedly calls the LLM, dispatches tool calls, handles stuck detection, token ceiling, message compaction, and context forking |
| `tools.go` | Standard tool definitions and execution: `read_file`, `grep`, `list_dir`, `git_log`. Also handles `AGENTS.md` discovery |
| `pathsafe.go` | Path validation to prevent directory traversal (`SafePath`, `CanonicalRoot`) |
| `json.go` | Extract JSON from LLM output (code blocks, raw), with LLM-assisted repair (`RepairJSON`) |
| `trace.go` | JSONL tracing to `~/.local/state/<agent>/traces/` |

## Usage

```go
llm := agentcore.NewLLMClient("http://localhost:1234/v1/chat/completions")

lr, err := agentcore.RunLoop(ctx, llm, agentcore.LoopConfig{
    ModelID:        "qwen/qwen3-32b",
    ContextSize:    200_000,
    TokenCeiling:   150_000,
    Root:           "/path/to/workspace",
    Tools:          agentcore.StandardToolDefs(),
    ToolDispatcher: agentcore.StandardToolDispatch,
    Temperature:    0.3,
    MaxIter:        12,
    MaxTokens:      3000,
    AgentName:      "my-agent",
    TracerTag:      "task-123",
    Messages: []agentcore.ChatMessage{
        {Role: "system", Content: "You are a code review agent..."},
        {Role: "user", Content: "Review this diff: ..."},
    },
})
```

## Features

### Standard tools

The agent gets five tools to explore the workspace, all path-sandboxed to the root:

- **read_file** — read a file (optionally a line range), max 1MB
- **grep** — regex search for text inside files, max 200 matches
- **glob** — find files by name pattern (`**/*.go`, `src/**/test_*.py`), supports `**` for recursive matching
- **list_dir** — list directory entries (flat, single directory)
- **git_log** — recent commit history

### AGENTS.md

When the agent reads a file or lists a directory, `agentcore` checks for `AGENTS.md` files in that directory and all ancestors up to the workspace root. Content is appended to the tool result, ordered root-to-leaf, and deduplicated across calls.

### Fork

Set `MaxForkDepth: 1` (or higher) to enable the `fork` tool. The agent can call:

```json
{"name": "fork", "arguments": {"tasks": [
  {"id": "security", "prompt": "Focus on security vulnerabilities..."},
  {"id": "perf", "prompt": "Focus on performance issues..."}
]}}
```

This clones the current conversation history and runs all sub-tasks as parallel goroutines. Each child inherits the full context (messages, AGENTS.md state) without re-exploring. Results are collected and returned to the parent loop.

Depth is decremented on each fork — children at depth 0 don't see the fork tool.

### Loop control

- **Stuck detection** — aborts if the same tool call signature repeats N times (default 3)
- **Token ceiling** — stops if total tokens exceed the configured limit
- **Message compaction** — truncates old tool results when approaching context size
- **Nudge** — if the model produces empty output, prompts it once to produce its final answer
- **Timeouts** — per-iteration (5m) and total (20m) defaults
