package config

import (
	"os"
	"path/filepath"
	"testing"
)

const validConfig = `
jira:
  base_url: "https://wallix.atlassian.net"
  email: "simon@wallix.com"
  api_token_env: "JIRA_API_TOKEN"
  project_key: "TRUS"
  status_mapping:
    GRILL: "In Progress"
    PLAN: "In Progress"
    CODE: "In Progress"
    LOCAL_REVIEW: "In Review"

agents:
  planner:
    type: "claude-code"
    token_budget: 200000
  coder:
    type: "claude-code"
  reviewer:
    type: "local-llm"
    endpoint: "http://localhost:8080"
    model: "qwen3.5-9b"

review:
  max_iterations: 3
  skill_path: ".claude/skills/review-branch.md"

workspace:
  worktree_dir: ".wt"
`

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadValidConfig(t *testing.T) {
	path := writeConfig(t, validConfig)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Jira.BaseURL != "https://wallix.atlassian.net" {
		t.Errorf("base_url = %q, want %q", cfg.Jira.BaseURL, "https://wallix.atlassian.net")
	}
	if cfg.Jira.ProjectKey != "TRUS" {
		t.Errorf("project_key = %q, want %q", cfg.Jira.ProjectKey, "TRUS")
	}
	if cfg.Agents.Planner.TokenBudget != 200000 {
		t.Errorf("token_budget = %d, want 200000", cfg.Agents.Planner.TokenBudget)
	}
	if cfg.Agents.Reviewer.Endpoint != "http://localhost:8080" {
		t.Errorf("reviewer endpoint = %q, want %q", cfg.Agents.Reviewer.Endpoint, "http://localhost:8080")
	}
	if cfg.Review.MaxIterations != 3 {
		t.Errorf("max_iterations = %d, want 3", cfg.Review.MaxIterations)
	}
	if cfg.Workspace.WorktreeDir != ".wt" {
		t.Errorf("worktree_dir = %q, want %q", cfg.Workspace.WorktreeDir, ".wt")
	}
	if cfg.Jira.StatusMapping["GRILL"] != "In Progress" {
		t.Errorf("status_mapping[GRILL] = %q, want %q", cfg.Jira.StatusMapping["GRILL"], "In Progress")
	}
}

func TestDefaults(t *testing.T) {
	minimal := `
jira:
  base_url: "https://example.atlassian.net"
  project_key: "TEST"
agents:
  coder:
    type: "claude-code"
`
	path := writeConfig(t, minimal)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Review.MaxIterations != 2 {
		t.Errorf("default max_iterations = %d, want 2", cfg.Review.MaxIterations)
	}
	if cfg.Workspace.WorktreeDir != ".worktrees" {
		t.Errorf("default worktree_dir = %q, want %q", cfg.Workspace.WorktreeDir, ".worktrees")
	}
	if cfg.Agents.Planner.TokenBudget != 150000 {
		t.Errorf("default token_budget = %d, want 150000", cfg.Agents.Planner.TokenBudget)
	}
}

func TestEnvVarTokenLoading(t *testing.T) {
	t.Setenv("TEST_JIRA_TOKEN", "secret-token-123")
	config := `
jira:
  base_url: "https://example.atlassian.net"
  project_key: "TEST"
  api_token_env: "TEST_JIRA_TOKEN"
agents:
  planner:
    type: "claude-code"
`
	path := writeConfig(t, config)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Jira.APIToken != "secret-token-123" {
		t.Errorf("api_token = %q, want %q", cfg.Jira.APIToken, "secret-token-123")
	}
}

func TestValidationMissingBaseURL(t *testing.T) {
	config := `
jira:
  project_key: "TEST"
agents:
  coder:
    type: "claude-code"
`
	path := writeConfig(t, config)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing base_url")
	}
}

func TestValidationMissingProjectKey(t *testing.T) {
	config := `
jira:
  base_url: "https://example.atlassian.net"
agents:
  coder:
    type: "claude-code"
`
	path := writeConfig(t, config)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing project_key")
	}
}

func TestValidationNoAgents(t *testing.T) {
	config := `
jira:
  base_url: "https://example.atlassian.net"
  project_key: "TEST"
`
	path := writeConfig(t, config)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for no agent config")
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
