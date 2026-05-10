package jira

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"maestro/internal/config"
	"maestro/internal/fsm"
)

// JiraClient implements tracker.Tracker using the Jira REST API v3 with
// Basic auth (email + API token / PAT).
type JiraClient struct {
	baseURL    string
	email      string
	apiToken   string
	projectKey string
	httpClient *http.Client
	mapping    map[fsm.State]string
}

// New creates a JiraClient from the application config.
func New(cfg config.JiraConfig) *JiraClient {
	return &JiraClient{
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		email:      cfg.Email,
		apiToken:   cfg.APIToken,
		projectKey: cfg.ProjectKey,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		mapping:    BuildMapping(cfg.StatusMapping),
	}
}

// ---------------------------------------------------------------------------
// Tracker interface
// ---------------------------------------------------------------------------

// Create opens a new Jira issue and returns its key.
func (c *JiraClient) Create(ctx context.Context, title, description string) (string, error) {
	body := map[string]any{
		"fields": map[string]any{
			"project":     map[string]string{"key": c.projectKey},
			"summary":     title,
			"description": adfText(description),
			"issuetype":   map[string]string{"name": "Task"},
		},
	}

	var resp struct {
		Key string `json:"key"`
	}
	if err := c.do(ctx, http.MethodPost, "/rest/api/3/issue", body, &resp); err != nil {
		return "", fmt.Errorf("jira create: %w", err)
	}
	return resp.Key, nil
}

// Update patches fields on an existing issue.
func (c *JiraClient) Update(ctx context.Context, jiraKey string, fields map[string]any) error {
	body := map[string]any{"fields": fields}
	if err := c.do(ctx, http.MethodPut, "/rest/api/3/issue/"+jiraKey, body, nil); err != nil {
		return fmt.Errorf("jira update %s: %w", jiraKey, err)
	}
	return nil
}

// Transition moves the issue to targetStatus by first fetching
// available transitions and selecting the one whose name matches.
func (c *JiraClient) Transition(ctx context.Context, jiraKey, targetStatus string) error {
	tid, err := c.findTransitionID(ctx, jiraKey, targetStatus)
	if err != nil {
		return err
	}

	body := map[string]any{
		"transition": map[string]string{"id": tid},
	}
	if err := c.do(ctx, http.MethodPost, "/rest/api/3/issue/"+jiraKey+"/transitions", body, nil); err != nil {
		return fmt.Errorf("jira transition %s → %s: %w", jiraKey, targetStatus, err)
	}
	return nil
}

// GetStatus returns the current workflow status name.
func (c *JiraClient) GetStatus(ctx context.Context, jiraKey string) (string, error) {
	var resp struct {
		Fields struct {
			Status struct {
				Name string `json:"name"`
			} `json:"status"`
		} `json:"fields"`
	}
	if err := c.do(ctx, http.MethodGet, "/rest/api/3/issue/"+jiraKey, nil, &resp); err != nil {
		return "", fmt.Errorf("jira get status %s: %w", jiraKey, err)
	}
	return resp.Fields.Status.Name, nil
}

// StatusMapping returns the FSM-to-Jira status mapping used by this
// client, so the orchestrator can call MapState with the same mapping.
func (c *JiraClient) StatusMapping() map[fsm.State]string {
	return c.mapping
}

// ---------------------------------------------------------------------------
// Internals
// ---------------------------------------------------------------------------

func (c *JiraClient) findTransitionID(ctx context.Context, jiraKey, targetStatus string) (string, error) {
	var resp struct {
		Transitions []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			To   struct {
				Name string `json:"name"`
			} `json:"to"`
		} `json:"transitions"`
	}
	if err := c.do(ctx, http.MethodGet, "/rest/api/3/issue/"+jiraKey+"/transitions", nil, &resp); err != nil {
		return "", fmt.Errorf("jira list transitions %s: %w", jiraKey, err)
	}

	target := strings.ToLower(targetStatus)
	for _, t := range resp.Transitions {
		if strings.ToLower(t.To.Name) == target || strings.ToLower(t.Name) == target {
			return t.ID, nil
		}
	}
	return "", fmt.Errorf("no transition to %q found for %s", targetStatus, jiraKey)
}

func (c *JiraClient) do(ctx context.Context, method, path string, payload any, dst any) error {
	var bodyReader io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.email, c.apiToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("jira %s %s: %d %s", method, path, resp.StatusCode, string(respBody))
	}

	if dst != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, dst); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}
	return nil
}

// adfText wraps plain text into Atlassian Document Format (ADF) for the
// v3 API description field.
func adfText(text string) map[string]any {
	return map[string]any{
		"type":    "doc",
		"version": 1,
		"content": []map[string]any{
			{
				"type": "paragraph",
				"content": []map[string]any{
					{
						"type": "text",
						"text": text,
					},
				},
			},
		},
	}
}
