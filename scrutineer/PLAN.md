# scrutineer: commit-by-commit review with inline comments

## Phase 1: Commit-by-commit review

Review an MR one commit at a time instead of as a single diff.

- Extend `Forge` interface with `ListCommits(ctx, id) []Commit` (sha, message, author, date)
- Implement for GitLab (`ListMergeRequestCommits`) and GitHub (`ListCommits` on PR)
- For each commit, get its diff via `git show <sha>` in the worktree, run `reviewer.Review()` with that single-commit diff
- Adapt `ReviewResult` to carry commit metadata (sha, message)
- Format output as per-commit sections in the comment
- Wire into `run()` with a `--mode full|commits` flag (default: `full` for backwards compat)

## Phase 2: Full-branch repass

After commit-by-commit review, optionally run a second pass on the full MR diff for cross-cutting concerns.

- **Intermediate LLM pass**: use the model to process per-commit findings before the repass — summarize, deduplicate overlapping findings, categorize by theme, flag contradictions between commits. This produces a compact digest that's cheaper to inject than raw findings.
- Inject the digest into the repass prompt so the agent knows what was already caught
- Agent focuses on: cross-commit interactions, architectural impact, things only visible at branch scope, patterns that emerge across commits
- Add `--mode both` that runs phase 1 → digest → phase 2
- Format output adds a "branch-level" section after the per-commit sections

## Phase 3: Inline comments

Post findings as inline comments on the diff instead of a single MR comment.

- Extend `Forge` interface with `PostInlineComment(ctx, id, file, line, sha, body)` or equivalent
- GitLab: use `CreateMergeRequestDiffDiscussion` (supports file, line, commit)
- GitHub: use the pull request review API — create a pending review, add comments, submit
- Parse finding `location` field (`file:line`) into structured file + line number
- Route findings: those with a valid file:line → inline comment, others → general comment
- Post a lightweight summary comment with finding counts

## Phase 4: Remove TUI, improve CLI

Replace the bubbletea TUI with CLI subcommands that cover the same functionality.

### List MRs
- Add `list` subcommand: show MRs with ID, author, age, title, review status
- Support `--filter all|unreviewed|reviewed` (default: all)
- Mark reviewed/posted status in output (e.g. `[R]`, `[P]`)

### Selective MR targeting
- Accept multiple MRs: `--mr 1,2,5` or `--mr 1 --mr 2 --mr 5`
- Combine with `--batch` to review a subset instead of all-or-nothing

### Per-MR review mode
- Allow per-MR mode override: `--mr 42:commits --mr 43:full`
- Fall back to `--mode` flag or config default when no per-MR mode given

### Review-then-post workflow
- Review without posting by default (current `--post` opt-in stays)
- After review, persist results to state (keyed by MR ID, branch name, or commit SHA) so they survive between invocations
- Print findings to stdout after review so the user can inspect them
- Add `post` subcommand: post previously reviewed findings without re-running the review
  - `post --mr 42,43` or `post --all` for MR findings
  - `post --branch <name>` or `post --commit <sha>` for commit comments
- Add `show` subcommand: display stored findings for a given MR/branch/commit without posting

### Progress output
- Print per-commit progress to stderr during commit-by-commit reviews in batch mode
- Show spinner or status line for long-running full-diff reviews

### Standalone branch/commit review with commit comments
- `--branch` already reviews a local branch; extend it to post findings via forge commit comments
- Add `--commit <sha>` to review a single commit (get diff via `git show`, review, optionally post)
- Extend `Forge` interface with `PostCommitComment(ctx, sha, file, line, body)` for both GitLab and GitHub
  - GitLab: `POST /projects/:id/repository/commits/:sha/comments` (supports `path`, `line`)
  - GitHub: `POST /repos/{owner}/{repo}/commits/{sha}/comments` (supports `path`, `line`)
- When `--post` is set on `--branch`, post inline findings on each commit's SHA
- When `--post` is set on `--commit`, post findings on that SHA
- Without `--post`, output to stdout as today

### Cleanup
- Remove `tui.go` and bubbletea/lipgloss dependencies
- Remove `programRef` plumbing from model and main

## Phase 5: Polish and configuration

- Config option for default review mode (full vs commit-by-commit)
- Config option for comment style (inline vs summary vs both)
- Rate limiting / batching for inline comments (avoid spamming the MR)
- Deduplication: don't re-review commits that were already reviewed (track by sha in state)
- Severity threshold: only post inline comments for major+ findings
