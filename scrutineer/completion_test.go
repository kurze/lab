package main

import (
	"strings"
	"testing"
)

func TestCompletionScriptsNonEmpty(t *testing.T) {
	shells := []struct {
		name    string
		content string
		marker  string
	}{
		{"bash", completionBash, "complete -F _scrutineer scrutineer"},
		{"zsh", completionZsh, "#compdef scrutineer"},
		{"fish", completionFish, "complete -c scrutineer"},
	}

	for _, sh := range shells {
		t.Run(sh.name, func(t *testing.T) {
			if len(sh.content) == 0 {
				t.Fatalf("completion script for %s is empty", sh.name)
			}
			if !strings.Contains(sh.content, sh.marker) {
				t.Errorf("completion script for %s missing expected marker %q", sh.name, sh.marker)
			}
		})
	}
}

func TestCompletionScriptsContainSubcommands(t *testing.T) {
	subcommands := []string{"review", "list", "show", "post", "completion"}

	shells := []struct {
		name    string
		content string
	}{
		{"bash", completionBash},
		{"zsh", completionZsh},
		{"fish", completionFish},
	}

	for _, sh := range shells {
		t.Run(sh.name, func(t *testing.T) {
			for _, cmd := range subcommands {
				if !strings.Contains(sh.content, cmd) {
					t.Errorf("%s completion missing subcommand %q", sh.name, cmd)
				}
			}
		})
	}
}

func TestCompletionScriptsContainFlagValues(t *testing.T) {
	flagValues := []struct {
		flag   string
		values []string
	}{
		{"--mode", []string{"full", "commits", "both"}},
		{"--filter", []string{"all", "unreviewed", "reviewed"}},
		{"--agent", []string{"builtin", "claude", "codex", "gemini"}},
		{"--comments", []string{"summary", "inline"}},
	}

	shells := []struct {
		name    string
		content string
	}{
		{"bash", completionBash},
		{"zsh", completionZsh},
		{"fish", completionFish},
	}

	for _, sh := range shells {
		t.Run(sh.name, func(t *testing.T) {
			for _, fv := range flagValues {
				for _, val := range fv.values {
					if !strings.Contains(sh.content, val) {
						t.Errorf("%s completion missing value %q for flag %s", sh.name, val, fv.flag)
					}
				}
			}
		})
	}
}
