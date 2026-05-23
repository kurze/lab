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
	Concurrency  int     `toml:"concurrency"`
}

type Config struct {
	ForgeType     string    `toml:"forge"`
	ForgeURL      string    `toml:"forge_url"`
	Token         string    `toml:"token"`
	Project       string    `toml:"project"`
	ReviewCommand string    `toml:"review_command"`
	ReviewAgent   string    `toml:"review_agent"`
	RepoPath      string    `toml:"repo_path"`
	ReviewMode    string    `toml:"review_mode"`
	Concurrency   int       `toml:"concurrency"`
	LLM           LLMConfig `toml:"llm"`
	CommentStyle   string   `toml:"comment_style"`
	InlineSeverity string   `toml:"inline_severity"`
	DryRun         bool     `toml:"-"`
}

func loadConfig(path string) Config {
	cfg := Config{
		RepoPath:    ".",
		Concurrency: 1,
		LLM: LLMConfig{
			ContextSize:  200_000,
			TokenCeiling: 150_000,
			Temperature:  0.3,
			Concurrency:  1,
		},
	}

	if path == "" {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, ".config", "scrutineer", "config.toml")
		}
	}

	if path != "" {
		if _, err := toml.DecodeFile(path, &cfg); err != nil && !os.IsNotExist(err) {
			log.Printf("warning: config %s: %v", path, err)
		}
	}

	if v := os.Getenv("FORGE_URL"); v != "" {
		cfg.ForgeURL = v
	}
	if v := os.Getenv("FORGE_TOKEN"); v != "" {
		cfg.Token = v
	}
	if v := os.Getenv("FORGE_PROJECT"); v != "" {
		cfg.Project = v
	}
	if v := os.Getenv("REVIEW_COMMAND"); v != "" {
		cfg.ReviewCommand = v
	}
	if v := os.Getenv("REVIEW_REPO_PATH"); v != "" {
		cfg.RepoPath = v
	}
	if v := os.Getenv("REVIEW_LLM_URL"); v != "" {
		cfg.LLM.URL = v
	}
	if v := os.Getenv("REVIEW_LLM_MODEL"); v != "" {
		cfg.LLM.Model = v
	}
	if v := os.Getenv("REVIEW_MODE"); v != "" {
		cfg.ReviewMode = v
	}

	if cfg.ForgeType == "" || cfg.ForgeURL == "" || cfg.Project == "" {
		info := DetectRemote(cfg.RepoPath)
		if cfg.ForgeType == "" && info.ForgeType != "" {
			cfg.ForgeType = info.ForgeType
		}
		if cfg.ForgeURL == "" && info.BaseURL != "" {
			cfg.ForgeURL = info.BaseURL
		}
		if cfg.Project == "" && info.Project != "" {
			cfg.Project = info.Project
		}
	}

	return cfg
}

func (c Config) CommitConcurrency() int {
	var n int
	if c.ReviewCommand != "" {
		n = c.Concurrency
	} else {
		n = c.LLM.Concurrency
	}
	if n <= 0 {
		return 1
	}
	return n
}

func (c Config) Validate() error {
	if c.Token == "" {
		return fmt.Errorf("token required: set token in config or FORGE_TOKEN env var")
	}
	if c.Project == "" {
		return fmt.Errorf("project required: set project in config or use --project flag")
	}
	if c.ReviewCommand == "" && c.LLM.URL == "" {
		return fmt.Errorf("review engine required: set review_command or [llm] url in config")
	}
	switch c.ReviewMode {
	case "", "full", "commits", "both":
	default:
		return fmt.Errorf("invalid review_mode %q (valid: full, commits, both)", c.ReviewMode)
	}
	switch c.CommentStyle {
	case "", "summary", "inline", "both":
	default:
		return fmt.Errorf("invalid comment_style %q (valid: summary, inline, both)", c.CommentStyle)
	}
	switch c.InlineSeverity {
	case "", "info", "minor", "major", "critical":
	default:
		return fmt.Errorf("invalid inline_severity %q (valid: info, minor, major, critical)", c.InlineSeverity)
	}
	return nil
}
