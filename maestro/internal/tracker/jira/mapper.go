package jira

import "maestro/internal/fsm"

// DefaultStatusMapping is the PRD-specified mapping from FSM states to
// Jira workflow status names.
var DefaultStatusMapping = map[fsm.State]string{
	fsm.Grill:       "In Progress",
	fsm.Plan:        "In Progress",
	fsm.Code:        "In Progress",
	fsm.AIReview:    "In Progress",
	fsm.AIFix:       "In Progress",
	fsm.LocalReview: "In Review",
	fsm.Push:        "In Review",
}

// MapState converts an FSM state to a Jira workflow status name using
// the provided mapping.  If the mapping is nil the default is used.
// Returns empty string for states not present in the mapping (e.g.
// ABANDONED).
func MapState(state fsm.State, mapping map[fsm.State]string) string {
	if mapping == nil {
		mapping = DefaultStatusMapping
	}
	return mapping[state]
}

// BuildMapping converts the string-keyed status_mapping from the config
// file into a typed map keyed by fsm.State.
func BuildMapping(raw map[string]string) map[fsm.State]string {
	if len(raw) == 0 {
		m := make(map[fsm.State]string, len(DefaultStatusMapping))
		for k, v := range DefaultStatusMapping {
			m[k] = v
		}
		return m
	}
	m := make(map[fsm.State]string, len(raw))
	for k, v := range raw {
		m[fsm.State(k)] = v
	}
	return m
}
