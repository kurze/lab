# `wlx-review-agent` — Delegation Brief

**Status:** Proposed, ready for implementation
**Owner:** Simon
**Date:** 2026-05-08
**Audience:** Claude Code instance picking up the implementation

---

## Context

Wallix engineering continuously produces architectural artifacts: ADRs, RFCs, spec documents, design proposals (current examples: RFC 8693 Token Exchange spec, `wlxctl`/`wlxd` architecture). Senior review of these artifacts is a bottleneck and benefits from a divergent second read — a model with different training data and different failure modes than Claude.

Constraints driving the design:

- **Air-gapped / on-prem is first-class.** Customer code, configs, and audit artifacts often cannot leave the local machine. Local-only inference is mandatory for some workloads.
- **Human reviewer is authoritative.** The local model is an attention amplifier, not a peer reviewer. It does not vote, does not approve, does not override project-history context that only the human has.
- **Latency tolerance varies by workload.** Plan / ADR / spec review is async-ish; inline code review is not. This brief covers the async case only.

Serving stack:

- **Model:** `qwen/qwen3.6-27b`
- **Runtime:** LM Studio
- **Endpoint:** `http://192.168.1.8:1234` (LAN host, **not colocated** with the Claude Code / agent process)
- **Throughput:** ~30 tk/s generation
- **Context window:** 200k tokens

LM Studio exposes three endpoint surfaces: native (`/api/v1/...`), OpenAI-compatible (`/v1/...`), Anthropic-compatible (`/v1/messages`). Endpoint choice is documented in Implementation Notes.

## Decision

Build an **MCP server, `wlx-review-agent`**, that exposes local-model-driven review to Claude Code. The local model operates as an **agent**: it receives the artifact path and a focus area, then iterates with a constrained tool surface (`read_file`, `grep`, `list_dir`) to build its own context before producing structured findings.

Rationale: for plan-review use cases, the value is decorrelated errors and independent reading. A direct model-as-tool call would feed the local model context curated by Claude, contaminating the second-opinion property. Agent-as-tool preserves independence at the cost of latency, which is acceptable for this workload.

## Scope

**In scope (v1):**

- One primary MCP tool: `independent_review`
- Agent loop with bounded iterations and a tight internal tool surface
- LM Studio backend over OpenAI-compatible endpoint
- JSON-schema-constrained output
- File-based logging of agent traces for debugging

**Out of scope (deferred):**

- Inline code review on diffs (use direct-call pattern instead — separate brief)
- Embedding / RAG over the codebase (separate effort, separate model)
- Multi-model consensus or voting
- Streaming partial findings
- Vendor-agnostic backend (v1 targets LM Studio specifically)

**Non-goals (will not build):**

- A tool that approves, certifies, or rejects artifacts. Output is *findings*, not verdicts.
- Anything that lets the local model take actions outside its read-only tool surface.
- General-purpose agent harness. This server does one thing.

## Architecture

### MCP Tool Surface (external contract)

```
independent_review(
  artifact_path:    string,
  focus:            string,
  workspace_root?:  string,    // defaults to dirname(artifact_path)
  max_iterations?:  number     // default 12
) -> {
  findings:        Finding[],
  open_questions:  string[],
  context_pulled:  string[],   // files/queries the agent consulted
  iterations_used: number,
  truncated:       boolean     // true if hit a stop condition before completing
}

Finding = {
  category:    "missing" | "inconsistent" | "risk" | "assumption" | "pattern_match",
  severity:    "info" | "minor" | "major",
  location:    string,         // file:line or section reference
  description: string,
  evidence:    string          // what in the artifact / context led to this
}
```

Output is descriptive, never prescriptive. No "you should do X." Findings surface things; the human decides what to do about them.

### Agent's Internal Tools

The local-model agent gets exactly these, no more:

```
read_file(path: string, range?: [start, end]) -> string
grep(pattern: string, path: string, glob?: string) -> matches[]
list_dir(path: string) -> entries[]
```

All scoped under `workspace_root`. Path traversal outside root is rejected at the tool layer, not at the model. Resolve to canonical absolute paths and prefix-check.

### Loop Control

- Hard cap: `max_iterations` (default 12). Each iteration = one model turn that may produce tool calls or final output.
- Stuck detection: three consecutive iterations producing identical tool calls → terminate, return with `truncated: true`.
- Per-iteration timeout: 60s. Exceeding aborts the iteration with a tool error fed back to the model.
- Total wall-clock cap: 5 minutes. Exceeding terminates and returns whatever findings have accumulated.

### Output Constraints

- Use structured output: `response_format: {type: "json_schema", json_schema: {...}}` on the OpenAI-compatible endpoint. This is the cleanest path on LM Studio.
- If structured output produces invalid or empty content, fall back to: parse, validate, and on failure repair-and-retry up to 2 times with the validation error injected into the next prompt.
- Final output must validate against the `independent_review` return schema.
- On final-attempt validation failure, return a tool error to the MCP caller. **Do not fabricate a passing response.**

## Implementation Notes

**Language:** Go. Single static binary, stdio transport, easy to ship into air-gapped environments alongside `wlxctl`.

**Agent loop:** Do **not** roll a generic agent harness. Two acceptable options:

1. Use an existing minimal Go agent library if one exists with constrained-output support that the implementer can vouch for.
2. Write a deliberately narrow loop (~200 lines) that handles only the three internal tools, JSON schema validation, and the stop conditions above.

Option 2 is preferred. Generality is a tax this project cannot afford.

**LM Studio integration:** Use the **OpenAI-compatible endpoint** (`POST /v1/chat/completions`).

Default config:

- URL: `http://192.168.1.8:1234/v1/chat/completions`
- Model: `qwen/qwen3.6-27b`

Override via env vars:

- `WLX_REVIEW_AGENT_LLM_URL` (full URL including path)
- `WLX_REVIEW_AGENT_MODEL`

No API key handling needed. The server is on LAN, not internet — assume reachable, surface clear errors if not (DNS failure, connection refused, timeout). Do not retry indefinitely.

**Endpoint choice rationale.** LM Studio also exposes an Anthropic-compatible endpoint (`/v1/messages`) and a native endpoint (`/api/v1/chat`). OpenAI-compatible is chosen because:

- It is the most stable, widely supported surface across local-inference runtimes (vLLM, Ollama, llama.cpp server, LocalAI). For air-gapped customer deployments where LM Studio may not be the chosen runtime, this matters.
- Vendor portability is a deployment concern, not a dev convenience. Locking to LM-Studio-native or Anthropic-shape APIs creates rework when the runtime changes.
- Tool calling and structured output (`response_format: {type: "json_schema", ...}`) are both supported on this endpoint and well-documented.

If the Anthropic-compatible endpoint later proves to have meaningfully better tool-call quality with this specific model, revisit. Do not switch speculatively.

**Context window:** 200k tokens is generous for the workloads in scope. Do not aggressively summarise or evict tool-call history during the agent loop — the model can hold a full review session in context. Hard ceiling: 150k tokens (~75% of window) before forced termination with `truncated: true`. Don't get clever with sliding windows.

**Logging:** Per-invocation trace file at `$XDG_STATE_HOME/wlx-review-agent/traces/<timestamp>-<artifact-basename>.jsonl`. One JSON object per line: `{ts, iteration, role, content, tool_calls?, tool_results?}`. Mandatory — without traces, debugging wrong findings is hopeless.

**Path safety:** Canonicalise `workspace_root` once at start of invocation. Every tool call resolves its argument and verifies it stays under root via prefix check on the canonicalised result. Reject symlinks that escape root.

**Configuration:** Single TOML file at `~/.config/wlx-review-agent/config.toml` for non-secret defaults. Env vars override. **No customer data in config.**

## Acceptance Criteria

1. `claude mcp add wlx-review-agent -- /path/to/binary` registers successfully.
2. Calling `independent_review` on a sample ADR returns valid JSON matching the schema.
3. Trace file is written for every invocation, including failed ones.
4. Path traversal attempts (e.g. `read_file("../../etc/passwd")`) return a tool error, do not read the file, are logged.
5. Hitting `max_iterations` returns `truncated: true` with whatever findings exist — not an error.
6. Killing LM Studio mid-call surfaces a clear error to the MCP caller within 5s. No hang.
7. Binary runs in an air-gapped environment with only LM Studio as a runtime dependency.
8. End-to-end smoke test: feed the brief you are reading right now back through `independent_review`. Verify findings are produced and schema-valid. (Self-review is a useful smoke test, not a quality benchmark.)

## Open Questions

- **Embedding-based file discovery.** Should the agent be able to do semantic search over the workspace? Probably yes eventually. Not v1. LM Studio exposes `/v1/embeddings` on the same server, so when this is revisited, no separate runtime is required — just load an embedding model alongside the chat model. Revisit after measuring how often agent traces show "I'd need to know about X."
- **Caching.** Same artifact reviewed twice with same focus — reuse? V1: no. Revisit if review-of-review workflows make it matter.
- **Cross-artifact context.** Reviews of an ADR sometimes need the prior ADRs it builds on. V1: human supplies via `workspace_root` containing both. Future: explicit dependency declaration.
- **Failure-mode calibration.** Once shipped, periodically feed both Claude and the local agent known-bad artifacts to check that "both agreed" is not silently masking a shared blind spot. Out of scope for build but flag for ops.

## Alternatives Considered

**Direct model-as-tool** (rejected for plan review). Claude curates context, sends to local model, gets a single-shot review. Cheaper, faster, easier to debug. Rejected because it contaminates the second-opinion property: the local model would review Claude's framing of the artifact, not the artifact. Worth building separately for code-diff review where context curation is appropriate.

**Multiple specialised MCP tools** (`surface_questions`, `match_known_patterns`, `checklist_pass`) (deferred). Cleaner contracts but more surface area. V1 collapses these into one `independent_review` with a `focus` parameter. Specialise later only if usage patterns demand it.

**Subprocess Claude Code with local-model backend** (rejected). Heavier, more moving parts, no clear advantage over a purpose-built loop for this specific workload.

## References

- Claude Code MCP integration: https://docs.claude.com/en/docs/claude-code/mcp
- LM Studio API surfaces (verified from current LM Studio install):
  - LM Studio native: `GET/POST /api/v1/{models,chat,models/load,models/download,...}`
  - OpenAI-compatible: `GET /v1/models`, `POST /v1/{responses,chat/completions,completions,embeddings}`
  - Anthropic-compatible: `POST /v1/messages`
- Companion skills (`review-branch`, `security-reviewer`, `module-audit`) — this tool complements them, does not replace them.

---

## Note to the Implementing Claude Code Instance

The human reviewer (Simon) is a Staff Engineer with 10+ years on this codebase. He has explicit authority over architectural decisions and the project history that informs them.

- Pushback on the **design** of this tool (schema shape, loop control, error handling) is welcome and should be raised.
- Pushback on whether the tool is **needed** is not. That decision is made. Build it.
- When uncertain about Wallix-specific conventions (paths, naming, deployment patterns), ask. Do not invent.
- Prefer fewer features done right over more features done half. The "out of scope" list is a feature.
