# scrutineer: commit-by-commit review with inline comments

## Phase 1: Commit-by-commit review

Review an MR one commit at a time instead of as a single diff.

- [x] Extend `Forge` interface with `ListCommits(ctx, id) []Commit` (sha, message, author, date)
- [x] Implement for GitLab (`ListMergeRequestCommits`)
- [x] Implement for GitHub (`ListCommits` on PR)
- [x] For each commit, get its diff via `git show <sha>` in the worktree, run `reviewer.Review()` with that single-commit diff
- [x] Adapt `ReviewResult` to carry commit metadata (sha, message)
- [x] Format output as per-commit sections in the comment
- [x] Wire into `run()` with a `--mode full|commits` flag (default: `full` for backwards compat)

## Phase 2: Full-branch repass

After commit-by-commit review, optionally run a second pass on the full MR diff for cross-cutting concerns.

- [x] **Intermediate LLM pass**: use the model to process per-commit findings before the repass — summarize, deduplicate overlapping findings, categorize by theme, flag contradictions between commits. This produces a compact digest that's cheaper to inject than raw findings.
- [x] Inject the digest into the repass prompt so the agent knows what was already caught
- [x] Agent focuses on: cross-commit interactions, architectural impact, things only visible at branch scope, patterns that emerge across commits
- [x] Add `--mode both` that runs phase 1 → digest → phase 2
- [x] Format output adds a "branch-level" section after the per-commit sections

## Phase 3: Inline comments

Post findings as inline comments on the diff instead of a single MR comment.

- [x] Extend `Forge` interface with `PostInlineComments(ctx, pr, comments)` (batch version)
- [x] GitLab: use `CreateMergeRequestDiffDiscussion` (supports file, line, commit)
- [x] GitHub: use the pull request review API — create a pending review, add comments, submit
- [x] Parse finding `location` field (`file:line`) into structured file + line number
- [x] Route findings: those with a valid file:line → inline comment, others → general comment
- [x] Post a lightweight summary comment with finding counts

## Phase 4: Remove TUI, improve CLI

Replace the bubbletea TUI with CLI subcommands that cover the same functionality.

### List MRs
- [x] Add `list` subcommand: show MRs with ID, author, age, title, review status
- [x] Support `--filter all|unreviewed|reviewed` (default: all)
- [ ] Mark posted status `[P]` in output (reviewed `[R]` is done)

### Selective MR targeting
- [x] Accept multiple MRs: `--mr 1,2,5` or `--mr 1 --mr 2 --mr 5`
- [x] Combine with `--batch` to review a subset instead of all-or-nothing

### Per-MR review mode
- [x] Allow per-MR mode override: `--mr 42:commits --mr 43:full`
- [x] Fall back to `--mode` flag or config default when no per-MR mode given

### Review-then-post workflow
- [x] Review without posting by default (current `--post` opt-in stays)
- [x] After review, persist results to state (keyed by MR ID, branch name, or commit SHA) so they survive between invocations
- [x] Print findings to stdout after review so the user can inspect them
- [x] Add `post` subcommand: post previously reviewed findings without re-running the review
- [x] Add `show` subcommand: display stored findings for a given MR/branch/commit without posting

### Progress output
- [x] Print per-commit progress to stderr during commit-by-commit reviews in batch mode

### Standalone branch/commit review with commit comments
- [x] `--branch` reviews a local branch and posts findings via forge commit comments
- [x] Add `--commit <sha>` to review a single commit (get diff via `git show`, review, optionally post)
- [x] Extend `Forge` interface with `PostCommitComment(ctx, sha, file, line, body)` for both GitLab and GitHub

### Cleanup
- [x] Remove `tui.go` and bubbletea/lipgloss dependencies
- [x] Remove `programRef` plumbing from model and main

## Phase 5: Polish and configuration

- [x] Config option for default review mode (full vs commit-by-commit)
- [x] Config option for comment style (inline vs summary vs both)
- [ ] Rate limiting / batching for inline comments (avoid spamming the MR)
- [x] Deduplication: don't re-review commits that were already reviewed (track by sha in state)
- [x] Severity threshold: only post inline comments for major+ findings

## Phase 6: Debug and LLM exchange logging

Capture all LLM request/response exchanges so they can be reviewed after the fact.

- [x] Log each LLM call (prompt, response, token counts, latency) to a structured file (JSONL or similar)
- [x] Include metadata: commit SHA, MR ID, review mode, timestamp
- [x] Store logs in a configurable directory (default: `~/.local/state/scrutineer/traces/`)
- [x] Add `logs` subcommand to list, inspect, and replay past exchanges
  - [x] `logs list` — show recent review sessions with date, MR, commit count
  - [x] `logs show <id>` — display the full prompt/response for a session
  - [ ] `logs tail` — stream logs during a running review
- [ ] Add `--verbose` / `--debug` flag to also print exchanges to stderr in real time
- [ ] Rotate / prune old logs based on age or size (configurable)

## Phase 7: Shell completions

Generate shell completion scripts for bash, zsh, and fish.

- [ ] Add `completion` subcommand: `scrutineer completion bash|zsh|fish`
- [ ] Complete subcommands, flags, and flag values (e.g. `--mode full|commits|both`)
- [ ] Dynamic completion for MR IDs where feasible (query forge for open MRs)
- [ ] Include installation instructions in `--help` output for each shell
- [ ] Consider using cobra/ff completion generation if the CLI framework supports it, otherwise hand-written scripts

## Phase 8: System prompt optimization

Improve the review quality by refining the system prompt sent to the LLM. Details TBD at implementation time.

- [ ] Audit current prompt for verbosity, ambiguity, and missed instructions
- [ ] Structure prompt with clear sections (role, task, constraints, output format)
- [ ] Add configurable prompt templates or overlays for different review styles (security-focused, performance-focused, etc.)
- [ ] Benchmark prompt changes against a set of known MRs to measure finding quality
- [ ] Allow user-provided prompt fragments via config (e.g. project-specific review guidelines)

## Phase 9: Mistral Vibe support

Add Mistral's Vibe coding model as a first-class agent option alongside the existing LLM backend.

- [ ] Extend agentcore LLMClient (or add a new client) to support API key authentication (`Authorization: Bearer` header) — currently the client sends no auth headers, which only works for local/unauthenticated endpoints
- [ ] Add Mistral API endpoint support — Mistral uses an OpenAI-compatible API so the request/response format should work as-is
- [ ] Add config fields for provider selection and API key:
  - [ ] `provider`: `"openai-compat"` (default, current behavior) or `"mistral"`
  - [ ] `api_key`: API key string, or `REVIEW_LLM_API_KEY` env var
  - [ ] Default URL and model for Mistral: `mistral-vibe-latest`
- [ ] Handle Mistral-specific quirks if any (tool call format differences, token counting, rate limits)
- [ ] Validate that tool use works correctly with Mistral Vibe (function calling support)
- [ ] Update `config.example.toml` with a Mistral Vibe example block
- [ ] Test with Mistral Vibe on a representative set of MRs to tune temperature and token limits
