package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type JiraConfig struct {
	BaseURL       string            `yaml:"base_url"`
	Email         string            `yaml:"email"`
	APITokenEnv   string            `yaml:"api_token_env"`
	ProjectKey    string            `yaml:"project_key"`
	StatusMapping map[string]string `yaml:"status_mapping"`

	APIToken string `yaml:"-"`
}

type AgentConfig struct {
	Type        string `yaml:"type"`
	Endpoint    string `yaml:"endpoint,omitempty"`
	Model       string `yaml:"model,omitempty"`
	TokenBudget int    `yaml:"token_budget,omitempty"`
}

type AgentsConfig struct {
	Planner  AgentConfig `yaml:"planner"`
	Coder    AgentConfig `yaml:"coder"`
	Reviewer AgentConfig `yaml:"reviewer"`
}

type ReviewConfig struct {
	MaxIterations int    `yaml:"max_iterations"`
	SkillPath     string `yaml:"skill_path"`
	BaseBranch    string `yaml:"base_branch"`
}

type WorkspaceConfig struct {
	WorktreeDir string `yaml:"worktree_dir"`
}

type Config struct {
	Jira      JiraConfig      `yaml:"jira"`
	Agents    AgentsConfig    `yaml:"agents"`
	Review    ReviewConfig    `yaml:"review"`
	Workspace WorkspaceConfig `yaml:"workspace"`
}

func Load(path string) (Config, error) {
	cfg := Config{
		Review: ReviewConfig{
			MaxIterations: 2,
		},
		Workspace: WorkspaceConfig{
			WorktreeDir: ".worktrees",
		},
		Agents: AgentsConfig{
			Planner: AgentConfig{
				TokenBudget: 150_000,
			},
		},
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("reading config: %w", err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing config: %w", err)
	}

	if cfg.Jira.APITokenEnv != "" {
		cfg.Jira.APIToken = os.Getenv(cfg.Jira.APITokenEnv)
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c Config) Validate() error {
	if c.Jira.BaseURL == "" {
		return fmt.Errorf("jira.base_url is required")
	}
	if c.Jira.ProjectKey == "" {
		return fmt.Errorf("jira.project_key is required")
	}
	if !hasAnyAgent(c.Agents) {
		return fmt.Errorf("at least one agent config is required")
	}
	return nil
}

func hasAnyAgent(a AgentsConfig) bool {
	return a.Planner.Type != "" || a.Coder.Type != "" || a.Reviewer.Type != ""
}
