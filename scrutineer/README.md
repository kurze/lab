# scrutineer

Scrutineer is an automated code review tool that reviews merge requests and pull requests using an LLM or an external review command. It connects to GitLab or GitHub, fetches diffs, runs them through a review engine, and posts structured findings back as comments. It supports reviewing full MR diffs, individual commits, or both in a two-pass approach that first reviews each commit then does a holistic pass over the entire change.

## Installation

```
go install github.com/kurze/lab/scrutineer@latest
```

## Usage

```
scrutineer <command> [flags]

Commands:
  review    Review merge/pull requests or local branches
  list      List merge/pull requests and their review status
  show      Display stored review findings
  post      Post stored review findings to the forge
```

### Review MRs

```bash
# Dry-run review of a single MR (prints findings, does not post)
scrutineer review --mr 42

# Review and post comments to the forge
scrutineer review --mr 42 --post

# Review multiple MRs, with per-MR mode override
scrutineer review --mr 42,43,44
scrutineer review --mr 42:commits,43:full

# Batch review all unreviewed MRs
scrutineer review --batch

# Review a local branch (no forge token needed)
scrutineer review --branch feature/foo

# Review a single commit
scrutineer review --commit abc1234
```

### List MRs

```bash
scrutineer list                      # all open MRs
scrutineer list --filter unreviewed  # only unreviewed
scrutineer list --filter reviewed    # only reviewed
```

### Show stored results

```bash
scrutineer show                # list all stored results
scrutineer show --mr 42        # show findings for MR 42
scrutineer show --branch foo   # show findings for a branch
scrutineer show --commit abc   # show findings for a commit
```

### Post stored results

```bash
scrutineer post --mr 42        # post stored findings for MR 42
scrutineer post --all           # post all stored MR results
scrutineer post --mr 42 --comments inline  # inline comments only
```

## Configuration

Config is loaded in layers, each overriding the previous:

1. **Global**: `~/.config/scrutineer/config.toml`
2. **Local**: `./config.toml` in the working directory (or `--config path/to/file.toml`)
3. **Environment variables** (see below)

```toml
# Forge type: "gitlab" or "github" (auto-detected from git remote if omitted)
forge = "gitlab"

# Forge base URL (default: https://gitlab.com or https://github.com)
forge_url = "https://gitlab.example.com"

# API token (or set FORGE_TOKEN env var)
token = "glpat-..."

# Project path (or set FORGE_PROJECT env var)
project = "owner/repo"

# Path to local repo clone (default: current directory)
repo_path = "/path/to/repo"

# Review mode: "full", "commits", or "both" (default: "full")
review_mode = "full"

# Comment style: "summary", "inline", or "both" (default: "both")
comment_style = "both"

# Minimum severity for inline comments: "info", "minor", "major", "critical"
inline_severity = "minor"

# External review command (receives diff on stdin, outputs JSON)
# review_command = "my-reviewer"

# LLM configuration (alternative to review_command)
[llm]
url = "http://localhost:1234"
model = "model-name"
context_size = 200000
token_ceiling = 150000
temperature = 0.3
concurrency = 2
```

### Environment variables

| Variable | Overrides |
|---|---|
| `FORGE_TOKEN` | `token` |
| `FORGE_URL` | `forge_url` |
| `FORGE_PROJECT` | `project` |
| `REVIEW_COMMAND` | `review_command` |
| `REVIEW_REPO_PATH` | `repo_path` |
| `REVIEW_LLM_URL` | `[llm] url` |
| `REVIEW_LLM_MODEL` | `[llm] model` |
| `REVIEW_MODE` | `review_mode` |

## Review modes

- **full** -- reviews the entire MR diff as a single pass. Good for small MRs.
- **commits** -- reviews each commit individually. Better signal on large MRs with logical commits.
- **both** -- two-pass: reviews each commit, digests the findings, then does a holistic review of the full diff with the digest as context. Most thorough but uses more tokens.

Per-MR overrides: `--mr 42:commits,43:full`.

## Forge support

Scrutineer supports GitLab and GitHub. The forge type is auto-detected from the `origin` remote URL. Set `forge = "gitlab"` or `forge = "github"` in the config to override.

Both forges support listing MRs/PRs, fetching diffs, listing commits, posting summary comments, and posting inline comments on diff lines.
