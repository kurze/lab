package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type PullRequest struct {
	ID     int64
	Title  string
	Labels []string
}

type Forge interface {
	Name() string
	ListUnreviewed(ctx context.Context, reviewLabel string) ([]PullRequest, error)
	Get(ctx context.Context, id int64) (PullRequest, error)
	GetDiff(ctx context.Context, id int64) (string, error)
	PostComment(ctx context.Context, id int64, body string) error
	AddLabel(ctx context.Context, id int64, label string) error
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
