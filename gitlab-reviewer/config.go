package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type LLMConfig struct {
	URL          string  `toml:"url"`
	Model        string  `toml:"model"`
	ContextSize  int     `toml:"context_size"`
	TokenCeiling int     `toml:"token_ceiling"`
	Temperature  float64 `toml:"temperature"`
}

type Config struct {
	GitLabURL     string    `toml:"gitlab_url"`
	Token         string    `toml:"token"`
	Project       string    `toml:"project"`
	ReviewLabel   string    `toml:"review_label"`
	ReviewCommand string    `toml:"review_command"`
	RepoPath      string    `toml:"repo_path"`
	LLM           LLMConfig `toml:"llm"`
	DryRun        bool      `toml:"-"`
}

func loadConfig(path string) Config {
	cfg := Config{
		GitLabURL:   "https://gitlab.com",
		ReviewLabel: "ai-reviewed",
		RepoPath:    ".",
		LLM: LLMConfig{
			ContextSize:  200_000,
			TokenCeiling: 150_000,
			Temperature:  0.3,
		},
	}

	if path == "" {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, ".config", "gitlab-reviewer", "config.toml")
		}
	}

	if path != "" {
		if _, err := toml.DecodeFile(path, &cfg); err != nil && !os.IsNotExist(err) {
			log.Printf("warning: config %s: %v", path, err)
		}
	}

	if v := os.Getenv("GITLAB_REVIEWER_URL"); v != "" {
		cfg.GitLabURL = v
	}
	if v := os.Getenv("GITLAB_REVIEWER_TOKEN"); v != "" {
		cfg.Token = v
	}
	if v := os.Getenv("GITLAB_REVIEWER_PROJECT"); v != "" {
		cfg.Project = v
	}
	if v := os.Getenv("GITLAB_REVIEWER_COMMAND"); v != "" {
		cfg.ReviewCommand = v
	}
	if v := os.Getenv("GITLAB_REVIEWER_REPO_PATH"); v != "" {
		cfg.RepoPath = v
	}
	if v := os.Getenv("GITLAB_REVIEWER_LLM_URL"); v != "" {
		cfg.LLM.URL = v
	}
	if v := os.Getenv("GITLAB_REVIEWER_LLM_MODEL"); v != "" {
		cfg.LLM.Model = v
	}

	return cfg
}

func (c Config) Validate() error {
	if c.Token == "" {
		return fmt.Errorf("gitlab token required: set token in config or GITLAB_REVIEWER_TOKEN env var")
	}
	if c.Project == "" {
		return fmt.Errorf("gitlab project required: set project in config or use --project flag")
	}
	if c.ReviewCommand == "" && c.LLM.URL == "" {
		return fmt.Errorf("review engine required: set review_command or [llm] url in config")
	}
	return nil
}
