package main

import (
	"reflect"
	"testing"
)

func TestParseRemoteURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want RemoteInfo
	}{
		{
			"github https",
			"https://github.com/owner/repo.git",
			RemoteInfo{ForgeType: "github", BaseURL: "https://github.com", Project: "owner/repo"},
		},
		{
			"github ssh",
			"git@github.com:owner/repo.git",
			RemoteInfo{ForgeType: "github", BaseURL: "https://github.com", Project: "owner/repo"},
		},
		{
			"gitlab https",
			"https://gitlab.example.com/group/project.git",
			RemoteInfo{ForgeType: "gitlab", BaseURL: "https://gitlab.example.com", Project: "group/project"},
		},
		{
			"gitlab ssh",
			"git@gitlab.example.com:group/subgroup/project.git",
			RemoteInfo{ForgeType: "gitlab", BaseURL: "https://gitlab.example.com", Project: "group/subgroup/project"},
		},
		{
			"no .git suffix",
			"https://github.com/owner/repo",
			RemoteInfo{ForgeType: "github", BaseURL: "https://github.com", Project: "owner/repo"},
		},
		{
			"trailing slash",
			"https://gitlab.com/owner/repo.git/",
			RemoteInfo{ForgeType: "gitlab", BaseURL: "https://gitlab.com", Project: "owner/repo"},
		},
		{
			"github enterprise",
			"https://github.corp.com/team/service.git",
			RemoteInfo{ForgeType: "github", BaseURL: "https://github.corp.com", Project: "team/service"},
		},
		{
			"self-hosted gitlab",
			"git@git.company.io:infra/deploy-tools.git",
			RemoteInfo{ForgeType: "gitlab", BaseURL: "https://git.company.io", Project: "infra/deploy-tools"},
		},
		{
			"ssh with port",
			"ssh://git@gitlab.example.com:2222/owner/repo.git",
			RemoteInfo{ForgeType: "gitlab", BaseURL: "https://gitlab.example.com", Project: "owner/repo"},
		},
		{
			"http scheme preserved",
			"http://git.local/team/project.git",
			RemoteInfo{ForgeType: "gitlab", BaseURL: "http://git.local", Project: "team/project"},
		},
		{
			"empty string",
			"",
			RemoteInfo{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseRemoteURL(tt.url)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseRemoteURL(%q)\n  got  %+v\n  want %+v", tt.url, got, tt.want)
			}
		})
	}
}

func TestFirstline(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"hello\nworld", "hello"},
		{"single line", "single line"},
		{"", ""},
		{"\nleading newline", "\nleading newline"},
		{"trailing\n", "trailing"},
	}
	for _, tt := range tests {
		if got := firstline(tt.in); got != tt.want {
			t.Errorf("firstline(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
