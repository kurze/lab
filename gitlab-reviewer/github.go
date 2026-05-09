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

func (g *GitHubClient) ListAll(ctx context.Context) ([]PullRequest, error) {
	prs, _, err := g.client.PullRequests.List(ctx, g.owner, g.repo, &github.PullRequestListOptions{
		State:       "open",
		ListOptions: github.ListOptions{PerPage: 100},
	})
	if err != nil {
		return nil, err
	}

	var result []PullRequest
	for _, pr := range prs {
		result = append(result, PullRequest{
			ID:     int64(pr.GetNumber()),
			Title:  pr.GetTitle(),
			Author: pr.GetUser().GetLogin(),
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
		Author: pr.GetUser().GetLogin(),
	}, nil
}

func (g *GitHubClient) ListCommits(ctx context.Context, id int64) ([]Commit, error) {
	commits, _, err := g.client.PullRequests.ListCommits(ctx, g.owner, g.repo, int(id), &github.ListOptions{PerPage: 100})
	if err != nil {
		return nil, err
	}

	result := make([]Commit, len(commits))
	for i, c := range commits {
		result[i] = Commit{
			SHA:     c.GetSHA(),
			Message: c.GetCommit().GetMessage(),
			Author:  c.GetCommit().GetAuthor().GetName(),
		}
	}
	return result, nil
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

func splitOwnerRepo(project string) (string, string, error) {
	parts := strings.SplitN(project, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("project must be owner/repo, got: %q", project)
	}
	return parts[0], parts[1], nil
}
