package tasks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Task is a single task item.
type Task struct {
	ID   int    `json:"id"`
	Text string `json:"text"`
	Done bool   `json:"done"`
}

// Store manages persistent tasks on disk.
type Store struct {
	path  string
	tasks []Task
	next  int
}

// NewStore creates a new task store at the given path.
func NewStore(path string) *Store {
	if path == "" {
		path = filepath.Join(".gocode", "tasks.json")
	}
	return &Store{path: path, next: 1}
}

// Load reads tasks from disk.
func (s *Store) Load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := json.Unmarshal(data, &s.tasks); err != nil {
		return err
	}
	for _, t := range s.tasks {
		if t.ID >= s.next {
			s.next = t.ID + 1
		}
	}
	return nil
}

// Save writes tasks to disk.
func (s *Store) Save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.tasks, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}

// Add creates a new task and returns it.
func (s *Store) Add(text string) Task {
	t := Task{ID: s.next, Text: text}
	s.next++
	s.tasks = append(s.tasks, t)
	return t
}

// Complete marks a task as done.
func (s *Store) Complete(id int) error {
	for i, t := range s.tasks {
		if t.ID == id {
			s.tasks[i].Done = true
			return nil
		}
	}
	return fmt.Errorf("task %d not found", id)
}

// List returns all tasks.
func (s *Store) List() []Task { return s.tasks }

// Render returns tasks as a display string.
func (s *Store) Render() string {
	if len(s.tasks) == 0 {
		return "No tasks."
	}
	var sb strings.Builder
	for _, t := range s.tasks {
		mark := "[ ]"
		if t.Done {
			mark = "[x]"
		}
		sb.WriteString(fmt.Sprintf("  %s #%d %s\n", mark, t.ID, t.Text))
	}
	return sb.String()
}
