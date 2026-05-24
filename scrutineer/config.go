package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

type AgentConfig struct {
	Name    string `toml:"name"`
	Command string `toml:"command"`
}

type agentPreset struct {
	Command string
	Args    []string
}

var agentPresets = map[string]agentPreset{
	"claude": {
		Command: "claude",
		Args:    []string{"-p", "--output-format", "text", "--permission-mode", "plan", "--bare"},
	},
	"codex": {
		Command: "codex",
		Args:    []string{"exec", "--sandbox", "read-only", "-a", "never", "--ephemeral"},
	},
	"gemini": {
		Command: "gemini",
		Args:    []string{"-p"},
	},
	"vibe": {
		Command: "vibe",
		Args:    []string{"--prompt"},
	},
	"opencode": {
		Command: "opencode",
		Args:    []string{"run", "-q"},
	},
	"pi": {
		Command: "pi",
		Args:    []string{"-p"},
	},
}

type LLMConfig struct {
	Provider     string  `toml:"provider"`
	URL          string  `toml:"url"`
	Model        string  `toml:"model"`
	APIKey       string  `toml:"api_key"`
	ContextSize  int     `toml:"context_size"`
	TokenCeiling int     `toml:"token_ceiling"`
	Temperature  float64 `toml:"temperature"`
	Concurrency  int     `toml:"concurrency"`
}

type providerPreset struct {
	URL   string
	Model string
}

var providerPresets = map[string]providerPreset{
	"lmstudio": {
		URL:   "http://localhost:1234/v1/chat/completions",
		Model: "default",
	},
	"ollama": {
		URL:   "http://localhost:11434/v1/chat/completions",
		Model: "llama3",
	},
	"mistral": {
		URL:   "https://api.mistral.ai/v1/chat/completions",
		Model: "mistral-small-latest",
	},
	"openai": {
		URL:   "https://api.openai.com/v1/chat/completions",
		Model: "gpt-4o",
	},
	"openrouter": {
		URL:   "https://openrouter.ai/api/v1/chat/completions",
		Model: "anthropic/claude-sonnet-4",
	},
}

type ReviewPromptConfig struct {
	Focus          string `toml:"focus"`
	Guidelines     string `toml:"guidelines"`
	GuidelinesFile string `toml:"guidelines_file"`
}

type Config struct {
	ForgeType      string      `toml:"forge"`
	ForgeURL       string      `toml:"forge_url"`
	Token          string      `toml:"token"`
	Project        string      `toml:"project"`
	ReviewCommand  string      `toml:"review_command"`
	ReviewAgent    string      `toml:"review_agent"`
	RepoPath       string      `toml:"repo_path"`
	ReviewMode     string      `toml:"review_mode"`
	Concurrency    int         `toml:"concurrency"`
	Agent          AgentConfig `toml:"agent"`
	LLM            LLMConfig   `toml:"llm"`
	CommentStyle   string      `toml:"comment_style"`
	InlineSeverity string      `toml:"inline_severity"`
	Review         ReviewPromptConfig `toml:"review"`
	LogDir         string             `toml:"log_dir"`
	LogMaxAgeDays  int                `toml:"log_max_age_days"`
	LogMaxSizeMB   int                `toml:"log_max_size_mb"`
	DryRun         bool        `toml:"-"`
	Verbose        bool        `toml:"-"`
}

func loadConfig(path string) Config {
	cfg := Config{
		RepoPath:      ".",
		Concurrency:   1,
		LogMaxAgeDays: 30,
		LogMaxSizeMB:  500,
		LLM: LLMConfig{
			ContextSize:  200_000,
			TokenCeiling: 150_000,
			Temperature:  0.3,
			Concurrency:  1,
		},
	}

	if home, err := os.UserHomeDir(); err == nil {
		globalPath := filepath.Join(home, ".config", "scrutineer", "config.toml")
		if _, err := toml.DecodeFile(globalPath, &cfg); err != nil && !os.IsNotExist(err) {
			log.Printf("warning: global config %s: %v", globalPath, err)
		}
	} else {
		log.Printf("warning: cannot determine home directory, skipping global config: %v", err)
	}

	if path == "" {
		path = "config.toml"
	}
	if _, err := toml.DecodeFile(path, &cfg); err != nil && !os.IsNotExist(err) {
		log.Printf("warning: config %s: %v", path, err)
	}

	if p, ok := providerPresets[cfg.LLM.Provider]; ok {
		if cfg.LLM.URL == "" {
			cfg.LLM.URL = p.URL
		}
		if cfg.LLM.Model == "" {
			cfg.LLM.Model = p.Model
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
	if v := os.Getenv("REVIEW_LLM_API_KEY"); v != "" {
		cfg.LLM.APIKey = v
	}
	if v := os.Getenv("REVIEW_MODE"); v != "" {
		cfg.ReviewMode = v
	}
	if v := os.Getenv("SCRUTINEER_LOG_DIR"); v != "" {
		cfg.LogDir = v
	}

	if cfg.Review.GuidelinesFile != "" {
		gpath := cfg.Review.GuidelinesFile
		if !filepath.IsAbs(gpath) {
			gpath = filepath.Join(cfg.RepoPath, gpath)
		}
		gpath = filepath.Clean(gpath)
		repoAbs, _ := filepath.Abs(cfg.RepoPath)
		if repoAbs != "" && !strings.HasPrefix(gpath, repoAbs+string(filepath.Separator)) && gpath != repoAbs {
			log.Printf("warning: guidelines_file %s is outside repo directory, skipping", cfg.Review.GuidelinesFile)
		} else {
			data, err := os.ReadFile(gpath)
			if err != nil {
				log.Printf("warning: guidelines file %s: %v", cfg.Review.GuidelinesFile, err)
			} else {
				const maxGuidelinesSize = 64 * 1024
				if len(data) > maxGuidelinesSize {
					log.Printf("warning: guidelines file %s exceeds %d bytes, truncating", cfg.Review.GuidelinesFile, maxGuidelinesSize)
					data = data[:maxGuidelinesSize]
				}
				if cfg.Review.Guidelines != "" {
					cfg.Review.Guidelines += "\n\n"
				}
				cfg.Review.Guidelines += string(data)
			}
		}
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

func resolveAgent(cfg Config) string {
	if cfg.Agent.Name != "" {
		return cfg.Agent.Name
	}
	if cfg.ReviewCommand != "" {
		return "custom"
	}
	return "builtin"
}

func (c Config) CommitConcurrency() int {
	var n int
	if resolveAgent(c) == "builtin" {
		n = c.LLM.Concurrency
	} else {
		n = c.Concurrency
	}
	if n <= 0 {
		return 1
	}
	return n
}

func (c Config) ValidateForge() error {
	if c.Token == "" {
		return fmt.Errorf("token required: set token in config or FORGE_TOKEN env var")
	}
	if c.Project == "" {
		return fmt.Errorf("project required: set project in config or use --project flag")
	}
	return nil
}

func (c Config) Validate() error {
	if err := c.ValidateForge(); err != nil {
		return err
	}
	agent := resolveAgent(c)
	switch agent {
	case "builtin":
		if c.LLM.URL == "" {
			return fmt.Errorf("builtin agent requires [llm] url (or set provider)")
		}
	case "custom":
		cmd := c.Agent.Command
		if cmd == "" {
			cmd = c.ReviewCommand
		}
		if cmd == "" {
			return fmt.Errorf("custom agent requires agent.command or review_command")
		}
	default:
		if _, ok := agentPresets[agent]; !ok {
			names := []string{"builtin", "custom"}
			for k := range agentPresets {
				names = append(names, k)
			}
			sort.Strings(names)
			return fmt.Errorf("unknown agent %q (known: %s)", agent, strings.Join(names, ", "))
		}
	}
	if agent != "builtin" && agent != "custom" {
		if c.ReviewMode == "commits" || c.ReviewMode == "both" {
			return fmt.Errorf("CLI agent %q only supports review_mode \"full\" (commits/both mode requires structured findings)", agent)
		}
	}
	if c.LLM.Provider != "" {
		if _, ok := providerPresets[c.LLM.Provider]; !ok {
			names := make([]string, 0, len(providerPresets))
			for k := range providerPresets {
				names = append(names, k)
			}
			sort.Strings(names)
			return fmt.Errorf("unknown llm provider %q (known: %s)", c.LLM.Provider, strings.Join(names, ", "))
		}
	}
	switch c.Review.Focus {
	case "", "security", "performance", "style":
	default:
		return fmt.Errorf("invalid review.focus %q (valid: security, performance, style, or omit for default)", c.Review.Focus)
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
