package task

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"maestro/internal/fsm"
)

type Store struct {
	root string // path to .maestro/ directory
}

func NewStore(root string) *Store {
	return &Store{root: root}
}

// Root returns the path to the .maestro/ directory managed by this store.
func (s *Store) Root() string {
	return s.root
}

func (s *Store) Create(title string) (*Task, error) {
	id, err := generateID()
	if err != nil {
		return nil, fmt.Errorf("generate id: %w", err)
	}

	t := &Task{
		ID:                  id,
		Title:               title,
		State:               fsm.Grill,
		ReviewIteration:     0,
		MaxReviewIterations: 2,
		CreatedAt:           time.Now().UTC(),
		Transitions:         []TransitionRecord{},
	}

	if err := s.Save(t); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *Store) Load(id string) (*Task, error) {
	path := filepath.Join(s.root, id, "task.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load task %s: %w", id, err)
	}
	var t Task
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parse task %s: %w", id, err)
	}
	return &t, nil
}

func (s *Store) List() ([]*Task, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list tasks: %w", err)
	}

	var tasks []*Task
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		t, err := s.Load(e.Name())
		if err != nil {
			continue
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

func (s *Store) Save(t *Task) error {
	dir := filepath.Join(s.root, t.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create task dir: %w", err)
	}

	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal task: %w", err)
	}

	path := filepath.Join(dir, "task.json")
	return os.WriteFile(path, data, 0o644)
}

func (s *Store) Resolve(idOrJiraKey string) (*Task, error) {
	t, err := s.Load(idOrJiraKey)
	if err == nil {
		return t, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	// Not found by ID — scan for jira_id match.
	tasks, err := s.List()
	if err != nil {
		return nil, err
	}
	for _, t := range tasks {
		if t.JiraID != nil && *t.JiraID == idOrJiraKey {
			return t, nil
		}
	}
	return nil, fmt.Errorf("task not found: %s", idOrJiraKey)
}

func generateID() (string, error) {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	now := time.Now().UTC()
	return fmt.Sprintf("m-%s-%s", now.Format("20060102"), hex.EncodeToString(b)), nil
}
