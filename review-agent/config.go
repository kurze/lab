package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type ModelDef struct {
	ID           string `toml:"id"`
	ContextSize  int    `toml:"context_size"`
	TokenCeiling int    `toml:"token_ceiling"`
}

type Config struct {
	LLMURL       string              `toml:"llm_url"`
	DefaultModel string              `toml:"default_model"`
	Models       map[string]ModelDef `toml:"models"`
}

var builtinModels = map[string]ModelDef{
	"precise": {
		ID:           "qwen/qwen3.6-27b",
		ContextSize:  200_000,
		TokenCeiling: 150_000,
	},
	"fast": {
		ID:           "qwen/qwen3.6-35b-a3b",
		ContextSize:  262_000,
		TokenCeiling: 196_000,
	},
}

func loadConfig() Config {
	cfg := Config{
		LLMURL:       "http://192.168.1.8:1234/v1/chat/completions",
		DefaultModel: "fast",
		Models:       make(map[string]ModelDef),
	}

	if home, err := os.UserHomeDir(); err == nil {
		path := filepath.Join(home, ".config", "wlx-review-agent", "config.toml")
		if _, err := toml.DecodeFile(path, &cfg); err != nil && !os.IsNotExist(err) {
			log.Printf("warning: config %s: %v", path, err)
		}
	}

	if v := os.Getenv("WLX_REVIEW_AGENT_LLM_URL"); v != "" {
		cfg.LLMURL = v
	}
	if v := os.Getenv("WLX_REVIEW_AGENT_DEFAULT_MODEL"); v != "" {
		cfg.DefaultModel = v
	}

	for k, v := range builtinModels {
		if _, exists := cfg.Models[k]; !exists {
			cfg.Models[k] = v
		}
	}

	return cfg
}

func (c Config) ResolveModel(name string) (ModelDef, error) {
	if name == "" {
		name = c.DefaultModel
	}
	if m, ok := c.Models[name]; ok {
		return m, nil
	}
	names := make([]string, 0, len(c.Models))
	for k := range c.Models {
		names = append(names, k)
	}
	return ModelDef{}, fmt.Errorf("unknown model %q, available: %v", name, names)
}
