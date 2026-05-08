package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-github/v72/github"
)

type GitHubClient struct {
	client *github.Client
	owner  string
	repo   string
}

func NewGitHubClient(cfg Config) (*GitHubClient, error) {
	client := github.NewClient(nil).WithAuthToken(cfg.Token)

	if cfg.ForgeURL != "" && cfg.ForgeURL != "https://github.com" {
		var err error
		client, err = github.NewClient(nil).WithEnterpriseURLs(cfg.ForgeURL, cfg.ForgeURL)
		if err != nil {
			return nil, fmt.Errorf("github enterprise: %w", err)
		}
		client = client.WithAuthToken(cfg.Token)
	}

	owner, repo, err := splitOwnerRepo(cfg.Project)
	if err != nil {
		return nil, err
	}

	return &GitHubClient{client: client, owner: owner, repo: repo}, nil
}

func (g *GitHubClient) Name() string { return "github" }

func (g *GitHubClient) ListUnreviewed(ctx context.Context, reviewLabel string) ([]PullRequest, error) {
	prs, _, err := g.client.PullRequests.List(ctx, g.owner, g.repo, &github.PullRequestListOptions{
		State:       "open",
		ListOptions: github.ListOptions{PerPage: 100},
	})
	if err != nil {
		return nil, err
	}

	var result []PullRequest
	for _, pr := range prs {
		if hasGitHubLabel(pr.Labels, reviewLabel) {
			continue
		}
		result = append(result, PullRequest{
			ID:     int64(pr.GetNumber()),
			Title:  pr.GetTitle(),
			Labels: gitHubLabelNames(pr.Labels),
		})
	}
	return result, nil
}

func (g *GitHubClient) Get(ctx context.Context, id int64) (PullRequest, error) {
	pr, _, err := g.client.PullRequests.Get(ctx, g.owner, g.repo, int(id))
	if err != nil {
		return PullRequest{}, err
	}
	return PullRequest{
		ID:     int64(pr.GetNumber()),
		Title:  pr.GetTitle(),
		Labels: gitHubLabelNames(pr.Labels),
	}, nil
}

func (g *GitHubClient) GetDiff(ctx context.Context, id int64) (string, error) {
	diff, _, err := g.client.PullRequests.GetRaw(ctx, g.owner, g.repo, int(id), github.RawOptions{Type: github.Diff})
	if err != nil {
		return "", err
	}
	return diff, nil
}

func (g *GitHubClient) PostComment(ctx context.Context, id int64, body string) error {
	_, _, err := g.client.Issues.CreateComment(ctx, g.owner, g.repo, int(id), &github.IssueComment{
		Body: &body,
	})
	return err
}

func (g *GitHubClient) AddLabel(ctx context.Context, id int64, label string) error {
	_, _, err := g.client.Issues.AddLabelsToIssue(ctx, g.owner, g.repo, int(id), []string{label})
	return err
}

func splitOwnerRepo(project string) (string, string, error) {
	parts := strings.SplitN(project, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("project must be owner/repo, got: %q", project)
	}
	return parts[0], parts[1], nil
}

func hasGitHubLabel(labels []*github.Label, target string) bool {
	for _, l := range labels {
		if l.GetName() == target {
			return true
		}
	}
	return false
}

func gitHubLabelNames(labels []*github.Label) []string {
	names := make([]string, len(labels))
	for i, l := range labels {
		names[i] = l.GetName()
	}
	return names
}
