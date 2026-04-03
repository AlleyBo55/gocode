package migrations

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Migration represents a single versioned migration step.
type Migration struct {
	Version int
	Name    string
	Migrate func(baseDir string) error
}

// completedState tracks which migrations have been applied.
type completedState struct {
	Completed []int `json:"completed"`
}

// Runner manages and executes migrations, tracking completed ones in a JSON file.
type Runner struct {
	migrations []Migration
	baseDir    string // e.g. ~/.gocode or a test temp dir
}

// NewRunner creates a Runner that persists state under baseDir.
func NewRunner(baseDir string) *Runner {
	r := &Runner{baseDir: baseDir}
	r.registerDefaults()
	return r
}

// NewRunnerWithMigrations creates a Runner with explicit migrations (for testing).
func NewRunnerWithMigrations(baseDir string, migrations []Migration) *Runner {
	return &Runner{baseDir: baseDir, migrations: migrations}
}

// Register adds a migration to the runner.
func (r *Runner) Register(m Migration) {
	r.migrations = append(r.migrations, m)
}

func (r *Runner) stateFile() string {
	return filepath.Join(r.baseDir, "migrations.json")
}

func (r *Runner) loadState() (completedState, error) {
	var state completedState
	data, err := os.ReadFile(r.stateFile())
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return state, fmt.Errorf("read migrations state: %w", err)
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, fmt.Errorf("parse migrations state: %w", err)
	}
	return state, nil
}

func (r *Runner) saveState(state completedState) error {
	if err := os.MkdirAll(r.baseDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.stateFile(), data, 0o644)
}

// Run executes all pending migrations in version order, skipping already-completed ones.
func (r *Runner) Run() error {
	state, err := r.loadState()
	if err != nil {
		return err
	}

	completed := make(map[int]bool)
	for _, v := range state.Completed {
		completed[v] = true
	}

	// Sort migrations by version.
	sorted := make([]Migration, len(r.migrations))
	copy(sorted, r.migrations)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Version < sorted[j].Version
	})

	for _, m := range sorted {
		if completed[m.Version] {
			continue
		}
		if err := m.Migrate(r.baseDir); err != nil {
			return fmt.Errorf("migration %d (%s) failed: %w", m.Version, m.Name, err)
		}
		state.Completed = append(state.Completed, m.Version)
		if err := r.saveState(state); err != nil {
			return fmt.Errorf("save state after migration %d: %w", m.Version, err)
		}
	}
	return nil
}

// Pending returns the list of migrations that have not yet been applied.
func (r *Runner) Pending() ([]Migration, error) {
	state, err := r.loadState()
	if err != nil {
		return nil, err
	}
	completed := make(map[int]bool)
	for _, v := range state.Completed {
		completed[v] = true
	}
	var pending []Migration
	for _, m := range r.migrations {
		if !completed[m.Version] {
			pending = append(pending, m)
		}
	}
	sort.Slice(pending, func(i, j int) bool {
		return pending[i].Version < pending[j].Version
	})
	return pending, nil
}

// --- default migrations ---

func (r *Runner) registerDefaults() {
	r.Register(Migration{
		Version: 1,
		Name:    "rename-old-config-paths",
		Migrate: migrateRenameConfigPaths,
	})
	r.Register(Migration{
		Version: 2,
		Name:    "update-model-aliases",
		Migrate: migrateUpdateModelAliases,
	})
}

// migrateRenameConfigPaths renames legacy config file paths to the new layout.
func migrateRenameConfigPaths(baseDir string) error {
	renames := map[string]string{
		"config.json":  "settings.json",
		"prompts.json": "skills/custom-prompts.json",
	}
	for old, newName := range renames {
		oldPath := filepath.Join(baseDir, old)
		newPath := filepath.Join(baseDir, newName)
		if _, err := os.Stat(oldPath); err == nil {
			if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
				return err
			}
			if err := os.Rename(oldPath, newPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// migrateUpdateModelAliases updates old model alias names in the settings file.
func migrateUpdateModelAliases(baseDir string) error {
	settingsPath := filepath.Join(baseDir, "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // nothing to migrate
		}
		return err
	}
	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return err
	}

	aliases := map[string]string{
		"claude-3":   "claude-3.5-sonnet",
		"gpt4":       "gpt-4o",
		"gpt-4-turbo": "gpt-4o",
	}

	changed := false
	if model, ok := settings["model"].(string); ok {
		if newModel, found := aliases[model]; found {
			settings["model"] = newModel
			changed = true
		}
	}

	if !changed {
		return nil
	}

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(settingsPath, out, 0o644)
}
