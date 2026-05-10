package grill

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"maestro/internal/agent"
)

const grillPrompt = `Interview me relentlessly about every aspect of this plan until we reach a shared understanding. Walk down each branch of the design tree, resolving dependencies between decisions one by one. If a question can be answered by exploring the codebase, explore the codebase instead.

When I say /done, stop asking questions and produce a PRD with these sections:
- Title
- Problem Statement
- User Stories
- Acceptance Criteria
- Scope Exclusions
- Implementation Notes

When I say /abandon, acknowledge and stop.`

// Result holds the outcome of a grill session.
type Result struct {
	PRDPath   string // path to the written prd.md (empty if abandoned)
	Abandoned bool
}

// RunGrill orchestrates the interactive grill interview loop.
// It sends the grill prompt to the agent, then relays user input and agent
// responses until the user types /done or /abandon.
//
// On /done the agent is instructed to produce a PRD, which is written to
// taskDir/prd.md. On /abandon the session ends immediately.
func RunGrill(ctx context.Context, ag agent.Agent, taskDir string, title string) (*Result, error) {
	// For Claude Code in interactive mode, exec into the claude process
	// and let the user interact directly.
	if ag.Type() == agent.ClaudeCode {
		return runGrillClaude(ctx, ag, taskDir, title)
	}

	return runGrillLocal(ctx, ag, taskDir, title)
}

// runGrillClaude delegates the entire session to Claude Code in interactive mode.
// The user converses directly with claude. On return (after the claude process
// exits), we check taskDir for a prd.md that claude was instructed to write.
func runGrillClaude(ctx context.Context, ag agent.Agent, taskDir string, title string) (*Result, error) {
	prompt := fmt.Sprintf("Task: %s\n\n%s\n\nWrite the PRD to: %s/prd.md", title, grillPrompt, taskDir)

	_, err := ag.Run(ctx, "", prompt)
	if err != nil {
		return nil, fmt.Errorf("claude grill session failed: %w", err)
	}

	// Check if a PRD was produced.
	prdPath := filepath.Join(taskDir, "prd.md")
	if _, err := os.Stat(prdPath); err == nil {
		return &Result{PRDPath: prdPath}, nil
	}

	// Claude session ended without producing a PRD — treat as abandoned.
	return &Result{Abandoned: true}, nil
}

// runGrillLocal runs the grill loop with a local LLM, reading user input from
// stdin and sending it to the agent turn by turn.
func runGrillLocal(ctx context.Context, ag agent.Agent, taskDir string, title string) (*Result, error) {
	scanner := bufio.NewScanner(os.Stdin)

	// Start the conversation with the grill prompt.
	conversation := fmt.Sprintf("Task: %s\n\n%s", title, grillPrompt)

	// Get the first question from the agent.
	response, err := ag.Run(ctx, "", conversation)
	if err != nil {
		return nil, fmt.Errorf("grill initial prompt failed: %w", err)
	}

	fmt.Println(response)

	// Track the full conversation for PRD generation.
	var history strings.Builder
	history.WriteString("System: " + conversation + "\n\n")
	history.WriteString("Agent: " + response + "\n\n")

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		fmt.Print("\n> ")
		if !scanner.Scan() {
			return &Result{Abandoned: true}, nil
		}

		input := strings.TrimSpace(scanner.Text())

		switch input {
		case "/abandon":
			return &Result{Abandoned: true}, nil

		case "/done":
			return finishGrill(ctx, ag, taskDir, history.String())

		default:
			history.WriteString("User: " + input + "\n\n")

			// Send the full conversation history so the agent has context.
			turnPrompt := history.String() + "\nContinue the interview. Ask your next question."
			response, err = ag.Run(ctx, "", turnPrompt)
			if err != nil {
				return nil, fmt.Errorf("grill turn failed: %w", err)
			}

			fmt.Println(response)
			history.WriteString("Agent: " + response + "\n\n")
		}
	}
}

// finishGrill instructs the agent to produce a PRD from the conversation
// and writes it to taskDir/prd.md.
func finishGrill(ctx context.Context, ag agent.Agent, taskDir string, conversationHistory string) (*Result, error) {
	prdPrompt := conversationHistory + "\n\n" + prdGenerationPrompt

	output, err := ag.Run(ctx, "", prdPrompt)
	if err != nil {
		return nil, fmt.Errorf("PRD generation failed: %w", err)
	}

	prd := FormatPRD(output)

	prdPath := filepath.Join(taskDir, "prd.md")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating task directory: %w", err)
	}

	if err := os.WriteFile(prdPath, []byte(prd), 0o644); err != nil {
		return nil, fmt.Errorf("writing prd.md: %w", err)
	}

	return &Result{PRDPath: prdPath}, nil
}
