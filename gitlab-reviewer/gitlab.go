package main

import (
	"context"
	"fmt"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type GitLabClient struct {
	client  *gitlab.Client
	project string
}

func NewGitLabClient(cfg Config) (*GitLabClient, error) {
	baseURL := cfg.ForgeURL
	if baseURL == "" {
		baseURL = "https://gitlab.com"
	}
	client, err := gitlab.NewClient(cfg.Token, gitlab.WithBaseURL(baseURL+"/api/v4"))
	if err != nil {
		return nil, err
	}
	return &GitLabClient{client: client, project: cfg.Project}, nil
}

func (g *GitLabClient) Name() string { return "gitlab" }

func (g *GitLabClient) ListUnreviewed(_ context.Context, reviewLabel string) ([]PullRequest, error) {
	state := "opened"
	opts := &gitlab.ListProjectMergeRequestsOptions{
		State: &state,
		ListOptions: gitlab.ListOptions{
			PerPage: 100,
		},
	}

	mrs, _, err := g.client.MergeRequests.ListProjectMergeRequests(g.project, opts)
	if err != nil {
		return nil, err
	}

	var result []PullRequest
	for _, mr := range mrs {
		if hasLabel(mr.Labels, reviewLabel) {
			continue
		}
		result = append(result, PullRequest{
			ID:     mr.IID,
			Title:  mr.Title,
			Labels: []string(mr.Labels),
		})
	}
	return result, nil
}

func (g *GitLabClient) Get(_ context.Context, id int64) (PullRequest, error) {
	mr, _, err := g.client.MergeRequests.GetMergeRequest(g.project, id, nil)
	if err != nil {
		return PullRequest{}, err
	}
	return PullRequest{
		ID:     mr.IID,
		Title:  mr.Title,
		Labels: []string(mr.Labels),
	}, nil
}

func (g *GitLabClient) GetDiff(_ context.Context, id int64) (string, error) {
	unidiff := true
	diffs, _, err := g.client.MergeRequests.ListMergeRequestDiffs(g.project, id, &gitlab.ListMergeRequestDiffsOptions{
		Unidiff: &unidiff,
	})
	if err != nil {
		return "", err
	}

	var b strings.Builder
	for _, d := range diffs {
		fmt.Fprintf(&b, "diff --git a/%s b/%s\n", d.OldPath, d.NewPath)
		b.WriteString(d.Diff)
		if !strings.HasSuffix(d.Diff, "\n") {
			b.WriteByte('\n')
		}
	}
	return b.String(), nil
}

func (g *GitLabClient) PostComment(_ context.Context, id int64, body string) error {
	_, _, err := g.client.Notes.CreateMergeRequestNote(g.project, id, &gitlab.CreateMergeRequestNoteOptions{
		Body: &body,
	})
	return err
}

func (g *GitLabClient) AddLabel(_ context.Context, id int64, label string) error {
	mr, _, err := g.client.MergeRequests.GetMergeRequest(g.project, id, nil)
	if err != nil {
		return err
	}

	labels := append(mr.Labels, label)
	labelOpts := gitlab.LabelOptions(labels)
	_, _, err = g.client.MergeRequests.UpdateMergeRequest(g.project, id, &gitlab.UpdateMergeRequestOptions{
		Labels: &labelOpts,
	})
	return err
}

func hasLabel(labels gitlab.Labels, target string) bool {
	for _, l := range labels {
		if l == target {
			return true
		}
	}
	return false
}
