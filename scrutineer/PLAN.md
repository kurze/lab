# scrutineer: commit-by-commit review with inline comments

## Phase 1: Commit-by-commit review

Review an MR one commit at a time instead of as a single diff.

- [x] Extend `Forge` interface with `ListCommits(ctx, id) []Commit` (sha, message, author, date)
- [x] Implement for GitLab (`ListMergeRequestCommits`) and GitHub (`ListCommits` on PR)
- [x] For each commit, get its diff via `git show <sha>` in the worktree, run `reviewer.Review()` with that single-commit diff
- [x] Adapt `ReviewResult` to carry commit metadata (sha, message)
- [x] Format output as per-commit sections in the comment
- [x] Wire into `run()` with a `--mode full|commits` flag (default: `full` for backwards compat)

## Phase 2: Full-branch repass

After commit-by-commit review, optionally run a second pass on the full MR diff for cross-cutting concerns.

- [x] **Intermediate LLM pass**: summarize, deduplicate, categorize per-commit findings into a compact digest
- [x] Inject the digest into the repass prompt so the agent knows what was already caught
- [x] Agent focuses on: cross-commit interactions, architectural impact, branch-scope patterns
- [x] Add `--mode both` that runs phase 1 → digest → phase 2
- [x] Format output adds a "branch-level" section after the per-commit sections

## Phase 3: Inline comments

Post findings as inline comments on the diff instead of a single MR comment.

- [x] Extend `Forge` interface with `PostInlineComment(ctx, id, file, line, sha, body)` or equivalent
- [x] GitLab: use `CreateMergeRequestDiffDiscussion` (supports file, line, commit)
- [x] GitHub: use the pull request review API — create a pending review, add comments, submit
- [x] Parse finding `location` field (`file:line`) into structured file + line number
- [x] Route findings: those with a valid file:line → inline comment, others → general comment
- [x] Post a lightweight summary comment with finding counts

## Phase 4: Remove TUI, improve CLI

Replace the bubbletea TUI with CLI subcommands that cover the same functionality.

- [x] Add `list` subcommand: show MRs with ID, author, age, title, review status
- [x] Support `--filter all|unreviewed|reviewed` (default: all)
- [ ] Mark posted status in output (`[P]` marker)
- [x] Accept multiple MRs: `--mr 1,2,5` or `--mr 1 --mr 2 --mr 5`
- [x] Combine with `--batch` to review a subset instead of all-or-nothing
- [x] Allow per-MR mode override: `--mr 42:commits --mr 43:full`
- [x] Review without posting by default (current `--post` opt-in stays)
- [x] Persist results to state so they survive between invocations
- [x] Print findings to stdout after review so the user can inspect them
- [x] Add `post` subcommand: post previously reviewed findings without re-running the review
- [x] Add `show` subcommand: display stored findings for a given MR/branch/commit without posting
- [x] Print per-commit progress to stderr during batch mode
- [ ] Show spinner or status line for long-running full-diff reviews
- [x] Replace `log.Printf` with colored output helpers with TTY detection
- [x] ANSI colors: dim timestamps, green/red/cyan markers, bold MR IDs, dim SHAs
- [x] Summary table at end of batch reviews: per-MR findings, posted count, tokens, duration, status
- [x] Surface token usage from agentcore `LoopResult` through `ReviewResult`
- [x] Suppress expected "branch not found" warnings during worktree cleanup
- [x] Remove `tui.go` and bubbletea/lipgloss dependencies
- [x] Remove `programRef` plumbing from model and main

## Phase 5: Standalone branch/commit review

Review local branches and individual commits outside of MR context, with optional forge commit comments.

- [ ] Extend `--branch` to post findings via forge commit comments when `--post` is set
- [ ] Add `--commit <sha>` to review a single commit (get diff via `git show`, review, optionally post)
- [ ] Extend `Forge` interface with `PostCommitComment(ctx, sha, file, line, body)` for both GitLab and GitHub
  - [ ] GitLab: `POST /projects/:id/repository/commits/:sha/comments` (supports `path`, `line`)
  - [ ] GitHub: `POST /repos/{owner}/{repo}/commits/{sha}/comments` (supports `path`, `line`)
- [ ] When `--post` is set on `--branch`, post inline findings on each commit's SHA
- [ ] When `--post` is set on `--commit`, post findings on that SHA
- [ ] Without `--post`, output to stdout as today

## Phase 6: Polish and configuration

- [x] Config option for default review mode (full vs commit-by-commit)
- [x] Config option for comment style (inline vs summary vs both)
- [ ] Rate limiting / batching for inline comments (avoid spamming the MR)
- [x] Deduplication: don't re-review commits that were already reviewed (track by sha in state)
- [x] Severity threshold: only post inline comments for major+ findings

## Phase 7: Debug and LLM exchange logging

Capture all LLM request/response exchanges so they can be reviewed after the fact.

- [ ] Log each LLM call (prompt, response, token counts, latency) to a structured file (JSONL or similar)
- [ ] Include metadata: commit SHA, MR ID, review mode, timestamp
- [ ] Store logs in a configurable directory (default: `~/.local/share/scrutineer/logs/`)
- [ ] Add `logs` subcommand to list, inspect, and replay past exchanges
  - [ ] `logs list` — show recent review sessions with date, MR, commit count
  - [ ] `logs show <id>` — display the full prompt/response for a session
  - [ ] `logs tail` — stream logs during a running review
- [ ] Add `--verbose` / `--debug` flag to also print exchanges to stderr in real time
- [ ] Rotate / prune old logs based on age or size (configurable)

## Phase 8: Shell completions

Generate shell completion scripts for bash, zsh, and fish.

- [ ] Add `completion` subcommand: `scrutineer completion bash|zsh|fish`
- [ ] Complete subcommands, flags, and flag values (e.g. `--mode full|commits|both`)
- [ ] Dynamic completion for MR IDs where feasible (query forge for open MRs)
- [ ] Include installation instructions in `--help` output for each shell
- [ ] Consider using cobra/ff completion generation if the CLI framework supports it, otherwise hand-written scripts

## Phase 9: System prompt optimization

Improve the review quality by refining the system prompt sent to the LLM. Details TBD at implementation time.

- [ ] Audit current prompt for verbosity, ambiguity, and missed instructions
- [ ] Structure prompt with clear sections (role, task, constraints, output format)
- [ ] Add configurable prompt templates or overlays for different review styles (security-focused, performance-focused, etc.)
- [ ] Benchmark prompt changes against a set of known MRs to measure finding quality
- [ ] Allow user-provided prompt fragments via config (e.g. project-specific review guidelines)

## Phase 10: Agent presets and provider support

Support external CLI review agents (Claude Code, Codex, Gemini, etc.) and hosted LLM providers.

### Agent presets
- [x] Add `[agent]` config section with `name` and `command` fields
- [x] Built-in presets: `claude`, `codex`, `gemini`, `vibe`, `opencode`, `pi` (extensible via `agentPresets` map)
- [x] `CLIReviewer` implementation: sends review prompt to CLI agent, captures raw text output
- [x] Backward compat: `review_command` still works (treated as `agent.name = "custom"`)
- [x] Raw output passthrough: CLI agents produce free text, posted as summary comment
- [x] `RawOutput` field on `ReviewResult` and `StoredResult` for unstructured agent output
- [ ] Test each agent preset with a real review (verify flags and invocation)

### LLM provider presets
- [x] `WithAPIKey` option on agentcore `LLMClient` — sets `Authorization: Bearer` header
- [x] Provider preset system: `provider = "mistral"` sets default URL + model
- [x] Built-in presets: `lmstudio`, `ollama`, `mistral`, `openai`, `openrouter`
- [x] Config fields: `provider`, `api_key` in `[llm]` section; `REVIEW_LLM_API_KEY` env var
- [x] Validate agent name and provider name against known presets
- [x] Updated `config.example.toml` with agent and provider examples
- [ ] Test with hosted providers (Mistral, OpenRouter) on real MRs

## Phase 11: Auto-fix via fixup commits

Automatically generate fixup commits for findings above a configurable severity threshold.

- [ ] Add `--fix` flag to `review` subcommand: after review, attempt to generate fixes for qualifying findings
- [ ] Add `fix_threshold` config option (default: `"minor"`) — only generate fixes for findings at or above this severity
- [ ] For each qualifying finding, send the finding + relevant file context to the LLM and ask for a concrete patch
- [ ] Apply each patch as its own `fixup!` commit targeting the original commit (`git commit --fixup=<sha>`) — one fixup per finding for clarity, user squashes after review
- [ ] Add `--fix-dry-run` flag to preview generated patches without committing
- [ ] Add `fix` subcommand to generate fixes from previously stored review results (without re-running the review)
- [ ] Handle conflicts gracefully: skip fix if patch doesn't apply cleanly, report to user

---

## Ideas

Unscoped ideas for future consideration. Not yet planned or prioritized.

- **Configurable guidelines files** — allow a list of guideline files (code style, commit message conventions, etc.) to be referenced in config and injected into the review prompt. Open question: how best to use them — full injection, summarized, or as a checklist the LLM scores against.
- **Multi-model consensus** — run the same review on two or more models, only surface findings that both agree on. Higher precision, fewer false positives, at the cost of N× tokens. Open question: how to match/deduplicate findings across models (semantic similarity? same file:line? LLM-assisted merge?).
- **Diff-aware context window management** — for large MRs, split the diff by file or logical unit, review chunks in parallel, then merge findings. Avoids truncation and lets smaller models handle big PRs.
- **Incremental re-review** — when a branch gets updated after review, only re-review commits that changed (force-push detection via reflog or forge events). Avoids re-reviewing the whole MR after a small fix.
- **Configurable review personas** — named profiles (e.g. `security`, `perf`, `api-review`) that bundle a system prompt + severity threshold + comment style. Run multiple personas in one pass for thorough coverage.
- **Forge webhook / CI integration** — listen for MR events (opened, updated) and auto-trigger reviews. Makes scrutineer a proper bot. Could be a lightweight HTTP server, GitHub Action, or GitLab CI job.
- **Finding deduplication across runs** — if the same finding (same file, line, category) was already posted in a previous review, skip it. Avoids spamming the MR with repeated comments after re-review.
- **Confidence scoring** — have the LLM self-rate confidence on each finding. Low-confidence findings get demoted or hidden behind a flag to reduce noise.
