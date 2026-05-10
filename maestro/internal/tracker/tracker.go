package tracker

import "context"

// Tracker abstracts issue-tracker operations so the orchestrator can
// create, update, and transition tickets without coupling to a specific
// provider.  The only production implementation today is Jira; the
// NoopTracker covers --no-jira mode and tests.
type Tracker interface {
	// Create opens a new ticket and returns its key (e.g. "TRUS-42").
	Create(ctx context.Context, title, description string) (jiraKey string, err error)

	// Update patches arbitrary fields on an existing ticket.
	Update(ctx context.Context, jiraKey string, fields map[string]any) error

	// Transition moves the ticket to targetStatus by looking up the
	// matching transition ID from the Jira workflow.
	Transition(ctx context.Context, jiraKey string, targetStatus string) error

	// GetStatus returns the current workflow status name of the ticket.
	GetStatus(ctx context.Context, jiraKey string) (status string, err error)
}

// NoopTracker silently succeeds for every operation.  Used when the user
// passes --no-jira or when no Jira configuration is present.
type NoopTracker struct{}

func (NoopTracker) Create(_ context.Context, _, _ string) (string, error) { return "", nil }
func (NoopTracker) Update(_ context.Context, _ string, _ map[string]any) error { return nil }
func (NoopTracker) Transition(_ context.Context, _, _ string) error      { return nil }
func (NoopTracker) GetStatus(_ context.Context, _ string) (string, error) { return "", nil }
