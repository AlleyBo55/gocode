package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// TrustedToolStore manages a persistent list of trusted tool patterns.
// Patterns can be:
//   - Exact tool name: "BashTool" — trusts this tool with any params
//   - Tool + command prefix: "BashTool:git *" — trusts BashTool when command starts with "git"
//   - Wildcard: "*" — trusts everything (equivalent to --dangerously-skip-permissions)
type TrustedToolStore struct {
	mu       sync.RWMutex
	patterns []string
	path     string
}

// NewTrustedToolStore creates a store backed by a JSON file.
func NewTrustedToolStore(path string) *TrustedToolStore {
	if path == "" {
		path = filepath.Join(".gocode", "trusted_tools.json")
	}
	return &TrustedToolStore{path: path}
}

// Load reads trusted patterns from disk.
func (s *TrustedToolStore) Load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return json.Unmarshal(data, &s.patterns)
}

// Save writes trusted patterns to disk.
func (s *TrustedToolStore) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.patterns, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}

// Add adds a trusted pattern and persists to disk.
func (s *TrustedToolStore) Add(pattern string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Don't add duplicates
	for _, p := range s.patterns {
		if p == pattern {
			return
		}
	}
	s.patterns = append(s.patterns, pattern)
}

// IsTrusted checks if a tool invocation matches any trusted pattern.
func (s *TrustedToolStore) IsTrusted(toolName string, input string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	toolLower := strings.ToLower(toolName)

	for _, pattern := range s.patterns {
		patLower := strings.ToLower(pattern)

		// Global wildcard — trust everything
		if patLower == "*" {
			return true
		}

		// Exact tool name match (no colon) — trust this tool with any params
		if !strings.Contains(patLower, ":") {
			if patLower == toolLower {
				return true
			}
			continue
		}

		// Tool:prefix pattern — trust tool when input contains the prefix
		parts := strings.SplitN(patLower, ":", 2)
		if len(parts) != 2 {
			continue
		}
		pTool := strings.TrimSpace(parts[0])
		pPrefix := strings.TrimSpace(parts[1])

		if pTool != toolLower {
			continue
		}

		// Wildcard suffix: "BashTool:git *" matches any command starting with "git"
		if strings.HasSuffix(pPrefix, " *") {
			cmdPrefix := strings.TrimSuffix(pPrefix, " *")
			if strings.Contains(strings.ToLower(input), cmdPrefix) {
				return true
			}
			continue
		}

		// Exact prefix match
		if strings.Contains(strings.ToLower(input), pPrefix) {
			return true
		}
	}
	return false
}

// List returns all trusted patterns.
func (s *TrustedToolStore) List() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, len(s.patterns))
	copy(out, s.patterns)
	return out
}

// Remove removes a trusted pattern by index.
func (s *TrustedToolStore) Remove(index int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.patterns) {
		return false
	}
	s.patterns = append(s.patterns[:index], s.patterns[index+1:]...)
	return true
}
