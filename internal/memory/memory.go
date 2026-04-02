package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Memory is a single key-value pair.
type Memory struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Store manages persistent memories on disk.
type Store struct {
	path     string
	memories []Memory
}

// NewStore creates a new memory store at the given path.
func NewStore(path string) *Store {
	if path == "" {
		path = filepath.Join(".gocode", "memory.json")
	}
	return &Store{path: path}
}

// Load reads memories from disk.
func (s *Store) Load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(data, &s.memories)
}

// Save writes memories to disk.
func (s *Store) Save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.memories, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}

// Set adds or updates a memory.
func (s *Store) Set(key, value string) {
	for i, m := range s.memories {
		if m.Key == key {
			s.memories[i].Value = value
			return
		}
	}
	s.memories = append(s.memories, Memory{Key: key, Value: value})
}

// Get retrieves a memory by key.
func (s *Store) Get(key string) (string, bool) {
	for _, m := range s.memories {
		if m.Key == key {
			return m.Value, true
		}
	}
	return "", false
}

// Delete removes a memory by key.
func (s *Store) Delete(key string) {
	for i, m := range s.memories {
		if m.Key == key {
			s.memories = append(s.memories[:i], s.memories[i+1:]...)
			return
		}
	}
}

// All returns all memories.
func (s *Store) All() []Memory { return s.memories }

// Render returns all memories as a string for system prompt injection.
func (s *Store) Render() string {
	if len(s.memories) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, m := range s.memories {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", m.Key, m.Value))
	}
	return sb.String()
}
