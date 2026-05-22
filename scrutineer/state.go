package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type State struct {
	Reviewed        map[string]map[string]time.Time `json:"reviewed"`
	ReviewedCommits map[string]map[string]time.Time `json:"reviewed_commits,omitempty"`
	path            string
}

func defaultStatePath() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".config", "scrutineer", "state.json")
	}
	return "state.json"
}

func LoadState(path string) (*State, error) {
	if path == "" {
		path = defaultStatePath()
	}
	s := &State{
		Reviewed:        make(map[string]map[string]time.Time),
		ReviewedCommits: make(map[string]map[string]time.Time),
		path:            path,
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	s.path = path
	return s, nil
}

func (s *State) IsReviewed(project string, id int64) bool {
	proj, ok := s.Reviewed[project]
	if !ok {
		return false
	}
	_, ok = proj[fmt.Sprintf("%d", id)]
	return ok
}

func (s *State) MarkReviewed(project string, id int64) {
	if s.Reviewed[project] == nil {
		s.Reviewed[project] = make(map[string]time.Time)
	}
	s.Reviewed[project][fmt.Sprintf("%d", id)] = time.Now()
}

func (s *State) IsCommitReviewed(project, sha string) bool {
	proj, ok := s.ReviewedCommits[project]
	if !ok {
		return false
	}
	_, ok = proj[sha]
	return ok
}

func (s *State) MarkCommitReviewed(project, sha string) {
	if s.ReviewedCommits == nil {
		s.ReviewedCommits = make(map[string]map[string]time.Time)
	}
	if s.ReviewedCommits[project] == nil {
		s.ReviewedCommits[project] = make(map[string]time.Time)
	}
	s.ReviewedCommits[project][sha] = time.Now()
}

func (s *State) Save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	return os.WriteFile(s.path, data, 0o644)
}
