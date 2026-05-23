package main

import (
	"context"
	"fmt"
	"net/url"
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
	PostCommitComment(ctx context.Context, sha string, file string, line int, body string) error
}

type RemoteInfo struct {
	ForgeType string
	BaseURL   string
	Project   string
}

func DetectForge(repoPath string) string {
	info := DetectRemote(repoPath)
	return info.ForgeType
}

func DetectRemote(repoPath string) RemoteInfo {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return RemoteInfo{}
	}
	return ParseRemoteURL(strings.TrimSpace(string(out)))
}

func ParseRemoteURL(raw string) RemoteInfo {
	var host, path, scheme string

	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return RemoteInfo{}
		}
		scheme = u.Scheme
		host = u.Hostname()
		path = strings.TrimPrefix(u.Path, "/")
	} else if strings.Contains(raw, ":") {
		// SCP-style: git@host:owner/repo.git
		at := strings.Index(raw, "@")
		if at < 0 {
			return RemoteInfo{}
		}
		rest := raw[at+1:]
		colon := strings.Index(rest, ":")
		if colon < 0 {
			return RemoteInfo{}
		}
		host = rest[:colon]
		path = rest[colon+1:]
		scheme = "https"
	} else {
		return RemoteInfo{}
	}

	if host == "" {
		return RemoteInfo{}
	}

	path = strings.TrimSuffix(path, "/")
	path = strings.TrimSuffix(path, ".git")

	if scheme != "http" {
		scheme = "https"
	}

	forgeType := "gitlab"
	if strings.Contains(host, "github") {
		forgeType = "github"
	}

	return RemoteInfo{
		ForgeType: forgeType,
		BaseURL:   scheme + "://" + host,
		Project:   path,
	}
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
