# wlx-review-agent

An MCP server that provides independent code and artifact review using a local LLM. It runs as a stdio-based MCP tool server, giving any MCP client (e.g. Claude Code) access to four review tools that autonomously read files, explore the workspace with git, and produce structured JSON findings.

## How it works

Each tool spawns an agentic loop (`agentcore.RunLoop`) that gives the local LLM a set of read-only workspace tools (read_file, grep, glob, list_dir, git_log, git_diff, git_blame, git_show). The LLM explores the codebase on its own for up to N iterations, then outputs structured JSON. If the output is malformed, a repair pass attempts to fix it.

All findings are **descriptive, never prescriptive** -- the agent says what it found, not what to do about it.

## Tools

| Tool | Purpose |
|---|---|
| `independent_review` | Review an artifact (spec, ADR, design doc) against the codebase. Returns findings with category, severity, location, and evidence. |
| `grill` | Generate tough probing questions about an artifact, like a senior reviewer grilling you on your design. |
| `diff_review` | Review a git diff (uncommitted changes, commit ranges, branch comparisons). Focuses on the changes, not pre-existing issues. |
| `compare_artifacts` | Compare two versions of a document. Flags added/removed/weakened assumptions, contradictions, and new risks. |

## Configuration

Config is loaded from `~/.config/wlx-review-agent/config.toml`, with environment variable overrides.

```toml
llm_url = "http://192.168.1.8:1234/v1/chat/completions"
default_model = "fast"

[models.precise]
id = "qwen/qwen3.6-27b"
context_size = 200000
token_ceiling = 150000

[models.fast]
id = "qwen/qwen3.6-35b-a3b"
context_size = 262000
token_ceiling = 196000
```

Environment variables:
- `WLX_REVIEW_AGENT_LLM_URL` -- override the LLM endpoint
- `WLX_REVIEW_AGENT_DEFAULT_MODEL` -- override the default model name

The two built-in models (`fast` and `precise`) are always available unless overridden in the config file.

## Usage

Build and run as an MCP server:

```bash
go build -o wlx-review-agent .
```

Register it in your MCP client config (e.g. `.mcp.json`):

```json
{
  "mcpServers": {
    "wlxc-review-agent": {
      "command": "/path/to/wlx-review-agent"
    }
  }
}
```

The tools are then available to the MCP client. Each tool accepts:
- **artifact_path** / **workspace_root** -- paths to the artifact or repo
- **focus** -- what to look for (e.g. "security assumptions", "error handling")
- **model** -- which model to use (`fast` or `precise`)
- **max_iterations** -- agent loop cap (default 12)
