package agent

import "context"

// AgentType identifies the backend used for agent execution.
type AgentType string

const (
	ClaudeCode AgentType = "claude-code"
	LocalLLM   AgentType = "local-llm"
)

// Mode controls how the agent interacts with the user.
type Mode string

const (
	Interactive Mode = "interactive" // stdin/stdout conversation (grill)
	Batch       Mode = "batch"       // headless, capture output (code/fix)
)

// Agent is the common interface for all coding/review/grill agents.
// Implementations wrap either Claude Code CLI or a local LLM HTTP endpoint.
type Agent interface {
	// Run executes the agent with the given prompt.
	// worktree is the working directory (empty string for non-code phases).
	// Returns the agent's text output or an error.
	Run(ctx context.Context, worktree string, prompt string) (result string, err error)

	// Type returns the backend type of this agent.
	Type() AgentType
}
