// Package plugins provides a plugin system for gocode.
// Plugins are JSON directories in .gocode/plugins/<name>/plugin.json.
package plugins

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Plugin represents an installable plugin.
type Plugin struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Hooks       []Hook   `json:"hooks,omitempty"`
	Skills      []string `json:"skills,omitempty"`
	Tools       []string `json:"tools,omitempty"`
}

// Hook defines a plugin hook that intercepts tool calls.
type Hook struct {
	Event   string `json:"event"`
	Pattern string `json:"pattern"`
	Action  string `json:"action"`
	Message string `json:"message,omitempty"`
}

// PluginManager loads and manages plugins.
type PluginManager struct {
	dir     string
	plugins []Plugin
}

// NewPluginManager creates a new PluginManager for the given directory.
func NewPluginManager(dir string) *PluginManager {
	return &PluginManager{dir: dir}
}

// builtinPlugins returns the bundled plugins that are always available.
func builtinPlugins() []Plugin {
	return []Plugin{
		{
			Name: "safety-guard", Version: "1.0.0",
			Description: "Blocks dangerous shell commands",
			Hooks: []Hook{
				{Event: "pre_tool", Pattern: "bashtool", Action: "log", Message: "Checking command safety..."},
			},
		},
		{
			Name: "git-auto-commit", Version: "1.0.0",
			Description: "Reminds to commit after file changes",
			Hooks: []Hook{
				{Event: "post_tool", Pattern: "fileedittool", Action: "log", Message: "File edited — consider committing your changes."},
				{Event: "post_tool", Pattern: "filewritetool", Action: "log", Message: "File written — consider committing your changes."},
			},
		},
	}
}

// LoadAll loads built-in plugins plus user-installed plugins from the directory.
func (pm *PluginManager) LoadAll() ([]Plugin, []error) {
	var all []Plugin
	var errs []error
	all = append(all, builtinPlugins()...)

	entries, err := os.ReadDir(pm.dir)
	if err != nil {
		if os.IsNotExist(err) {
			pm.plugins = all
			return all, nil
		}
		pm.plugins = all
		return all, []error{fmt.Errorf("reading plugins dir: %w", err)}
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(pm.dir, entry.Name(), "plugin.json")
		data, err := os.ReadFile(path)
		if err != nil {
			errs = append(errs, fmt.Errorf("reading %s: %w", path, err))
			continue
		}
		var p Plugin
		if err := json.Unmarshal(data, &p); err != nil {
			errs = append(errs, fmt.Errorf("parsing %s: %w", path, err))
			continue
		}
		all = append(all, p)
	}
	pm.plugins = all
	return all, errs
}

// List returns all loaded plugins.
func (pm *PluginManager) List() []Plugin { return pm.plugins }

// Get returns a plugin by name.
func (pm *PluginManager) Get(name string) (Plugin, bool) {
	for _, p := range pm.plugins {
		if p.Name == name {
			return p, true
		}
	}
	return Plugin{}, false
}

// Install writes a plugin.json into the plugins directory.
func (pm *PluginManager) Install(name string, plugin Plugin) error {
	dir := filepath.Join(pm.dir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating plugin dir: %w", err)
	}
	data, err := json.MarshalIndent(plugin, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling plugin: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, "plugin.json"), data, 0o644)
}

// Uninstall removes a plugin directory.
func (pm *PluginManager) Uninstall(name string) error {
	return os.RemoveAll(filepath.Join(pm.dir, name))
}

// matchGlob checks if a tool name matches a glob pattern.
func matchGlob(pattern, name string) bool {
	if pattern == "*" {
		return true
	}
	if strings.Contains(pattern, "*") {
		matched, _ := filepath.Match(pattern, name)
		return matched
	}
	return pattern == name
}
