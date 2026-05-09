package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type DiffRefs struct {
	BaseSHA  string
	HeadSHA  string
	StartSHA string
}

type InlineComment struct {
	File string
	Line int
	Body string
}

type PullRequest struct {
	ID        int64
	Title     string
	Author    string
	DiffRefs  DiffRefs
	UpdatedAt time.Time
}

type Commit struct {
	SHA     string
	Message string
	Author  string
}

type Forge interface {
	Name() string
	ListAll(ctx context.Context) ([]PullRequest, error)
	Get(ctx context.Context, id int64) (PullRequest, error)
	GetDiff(ctx context.Context, id int64) (string, error)
	ListCommits(ctx context.Context, id int64) ([]Commit, error)
	PostComment(ctx context.Context, id int64, body string) error
	PostInlineComments(ctx context.Context, pr PullRequest, comments []InlineComment) error
}

func DetectForge(repoPath string) string {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	url := strings.TrimSpace(string(out))

	if strings.Contains(url, "github.com") {
		return "github"
	}
	return "gitlab"
}

func firstline(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx > 0 {
		return s[:idx]
	}
	return s
}

func NewForge(cfg Config) (Forge, error) {
	forgeType := cfg.ForgeType
	if forgeType == "" {
		forgeType = DetectForge(cfg.RepoPath)
	}

	switch forgeType {
	case "github":
		return NewGitHubClient(cfg)
	case "gitlab":
		return NewGitLabClient(cfg)
	default:
		return nil, fmt.Errorf("unknown forge type: %s", forgeType)
	}
}
