package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type StoredResult struct {
	Key        string    `json:"key"`
	Title      string    `json:"title"`
	Mode       string    `json:"mode"`
	Findings   []Finding `json:"findings"`
	RawOutput  string    `json:"raw_output"`
	Model      string    `json:"model,omitempty"`
	ReviewedAt time.Time `json:"reviewed_at"`
}

type State struct {
	Reviewed        map[string]map[string]time.Time          `json:"reviewed"`
	ReviewedCommits map[string]map[string]time.Time          `json:"reviewed_commits,omitempty"`
	Results         map[string]map[string]*StoredResult      `json:"results,omitempty"`
	mu              sync.Mutex                               `json:"-"`
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
		Results:         make(map[string]map[string]*StoredResult),
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
	s.mu.Lock()
	defer s.mu.Unlock()
	proj, ok := s.ReviewedCommits[project]
	if !ok {
		return false
	}
	_, ok = proj[sha]
	return ok
}

func (s *State) MarkCommitReviewed(project, sha string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ReviewedCommits == nil {
		s.ReviewedCommits = make(map[string]map[string]time.Time)
	}
	if s.ReviewedCommits[project] == nil {
		s.ReviewedCommits[project] = make(map[string]time.Time)
	}
	s.ReviewedCommits[project][sha] = time.Now()
}

func ResultKeyMR(id int64) string       { return fmt.Sprintf("mr:%d", id) }
func ResultKeyBranch(name string) string { return "branch:" + name }
func ResultKeyCommit(sha string) string  { return "commit:" + sha }

func (s *State) StoreResult(project string, sr *StoredResult) {
	if s.Results == nil {
		s.Results = make(map[string]map[string]*StoredResult)
	}
	if s.Results[project] == nil {
		s.Results[project] = make(map[string]*StoredResult)
	}
	s.Results[project][sr.Key] = sr
}

func (s *State) GetResult(project, key string) *StoredResult {
	if s.Results == nil {
		return nil
	}
	proj, ok := s.Results[project]
	if !ok {
		return nil
	}
	return proj[key]
}

func (s *State) ListResults(project string) []*StoredResult {
	if s.Results == nil {
		return nil
	}
	proj, ok := s.Results[project]
	if !ok {
		return nil
	}
	results := make([]*StoredResult, 0, len(proj))
	for _, r := range proj {
		results = append(results, r)
	}
	return results
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
