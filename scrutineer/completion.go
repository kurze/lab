package main

import (
	_ "embed"
	"fmt"
	"os"
)

//go:embed completions/scrutineer.bash
var completionBash string

//go:embed completions/scrutineer.zsh
var completionZsh string

//go:embed completions/scrutineer.fish
var completionFish string

var completionUsage = `Usage: scrutineer completion <shell>

Generate shell completion scripts.

Supported shells: bash, zsh, fish

Installation:

  Bash:
    scrutineer completion bash > ~/.local/share/bash-completion/completions/scrutineer

  Zsh:
    scrutineer completion zsh > "${fpath[1]}/_scrutineer"
    # then restart your shell or run: compinit

  Fish:
    scrutineer completion fish > ~/.config/fish/completions/scrutineer.fish
`

func cmdCompletion(args []string) {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
		fmt.Fprint(os.Stderr, completionUsage)
		os.Exit(0)
	}
	if len(args) != 1 {
		fmt.Fprint(os.Stderr, completionUsage)

		os.Exit(1)
	}

	switch args[0] {
	case "bash":
		fmt.Print(completionBash)
	case "zsh":
		fmt.Print(completionZsh)
	case "fish":
		fmt.Print(completionFish)
	default:
		fmt.Fprintf(os.Stderr, "unsupported shell: %s (valid: bash, zsh, fish)\n", args[0])
		os.Exit(1)
	}
}
