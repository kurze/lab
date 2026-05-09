package main

import (
	"context"
	"fmt"
	"strings"
	"time"

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

func (g *GitLabClient) ListAll(_ context.Context) ([]PullRequest, error) {
	state := "opened"
	opts := &gitlab.ListProjectMergeRequestsOptions{
		State:       &state,
		ListOptions: gitlab.ListOptions{PerPage: 100},
	}

	mrs, _, err := g.client.MergeRequests.ListProjectMergeRequests(g.project, opts)
	if err != nil {
		return nil, err
	}

	var result []PullRequest
	for _, mr := range mrs {
		author := ""
		if mr.Author != nil {
			author = mr.Author.Username
		}
		var updatedAt time.Time
		if mr.UpdatedAt != nil {
			updatedAt = *mr.UpdatedAt
		}
		result = append(result, PullRequest{
			ID:        mr.IID,
			Title:     mr.Title,
			Author:    author,
			UpdatedAt: updatedAt,
		})
	}
	return result, nil
}

func (g *GitLabClient) Get(_ context.Context, id int64) (PullRequest, error) {
	mr, _, err := g.client.MergeRequests.GetMergeRequest(g.project, id, nil)
	if err != nil {
		return PullRequest{}, err
	}
	author := ""
	if mr.Author != nil {
		author = mr.Author.Username
	}
	var updatedAt time.Time
	if mr.UpdatedAt != nil {
		updatedAt = *mr.UpdatedAt
	}
	return PullRequest{
		ID:        mr.IID,
		Title:     mr.Title,
		Author:    author,
		UpdatedAt: updatedAt,
		DiffRefs: DiffRefs{
			BaseSHA:  mr.DiffRefs.BaseSha,
			HeadSHA:  mr.DiffRefs.HeadSha,
			StartSHA: mr.DiffRefs.StartSha,
		},
	}, nil
}

func (g *GitLabClient) ListCommits(_ context.Context, id int64) ([]Commit, error) {
	commits, _, err := g.client.MergeRequests.GetMergeRequestCommits(g.project, id, &gitlab.GetMergeRequestCommitsOptions{
		ListOptions: gitlab.ListOptions{PerPage: 100},
	})
	if err != nil {
		return nil, err
	}

	result := make([]Commit, len(commits))
	for i, c := range commits {
		result[i] = Commit{
			SHA:     c.ID,
			Message: c.Title,
			Author:  c.AuthorName,
		}
	}
	return result, nil
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

func (g *GitLabClient) PostInlineComments(_ context.Context, pr PullRequest, comments []InlineComment) error {
	posType := "text"
	for _, c := range comments {
		line := int64(c.Line)
		_, _, err := g.client.Discussions.CreateMergeRequestDiscussion(g.project, pr.ID, &gitlab.CreateMergeRequestDiscussionOptions{
			Body: &c.Body,
			Position: &gitlab.PositionOptions{
				PositionType: &posType,
				BaseSHA:      &pr.DiffRefs.BaseSHA,
				HeadSHA:      &pr.DiffRefs.HeadSHA,
				StartSHA:     &pr.DiffRefs.StartSHA,
				NewPath:      &c.File,
				OldPath:      &c.File,
				NewLine:      &line,
			},
		})
		if err != nil {
			return fmt.Errorf("inline comment on %s:%d: %w", c.File, c.Line, err)
		}
	}
	return nil
}
