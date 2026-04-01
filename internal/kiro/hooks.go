package kiro

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// HookWhen defines when a hook triggers.
type HookWhen struct {
	Type      string   `json:"type"`
	Patterns  []string `json:"patterns,omitempty"`
	ToolTypes []string `json:"toolTypes,omitempty"`
}

// HookThen defines what a hook does when triggered.
type HookThen struct {
	Type    string `json:"type"`
	Prompt  string `json:"prompt,omitempty"`
	Command string `json:"command,omitempty"`
}

// HookDefinition represents a Kiro hook configuration.
type HookDefinition struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description,omitempty"`
	When        HookWhen `json:"when"`
	Then        HookThen `json:"then"`
}

// LoadHooks reads all hook JSON files from the given directory.
// Returns an empty slice if the directory does not exist.
func LoadHooks(dir string) ([]HookDefinition, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return []HookDefinition{}, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading hooks directory: %w", err)
	}

	var hooks []HookDefinition
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if ext != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("reading hook %s: %w", entry.Name(), err)
		}
		var hook HookDefinition
		if err := json.Unmarshal(data, &hook); err != nil {
			return nil, fmt.Errorf("parsing hook %s: %w", entry.Name(), err)
		}
		hooks = append(hooks, hook)
	}
	return hooks, nil
}
