package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gocode/gocode/internal/models"
)

// ErrCommandNotFound is returned when a command lookup fails.
var ErrCommandNotFound = errors.New("command not found")

// CommandExecution holds the result of executing a command.
type CommandExecution struct {
	Name       string `json:"name"`
	SourceHint string `json:"source_hint"`
	Prompt     string `json:"prompt"`
	Handled    bool   `json:"handled"`
	Message    string `json:"message"`
}

// CommandLookup defines the interface for command registry operations.
type CommandLookup interface {
	GetCommand(name string) (*models.PortingModule, error)
	FindCommands(query string, limit int) []models.PortingModule
	GetCommands(includePlugins, includeSkills bool) []models.PortingModule
	ExecuteCommand(name, prompt string) CommandExecution
	RenderIndex(limit int, query string) string
}

// CommandRegistry implements CommandLookup by loading commands from JSON.
type CommandRegistry struct {
	commands []models.PortingModule
	index    map[string]*models.PortingModule
}

// commandJSON matches the JSON shape in commands.json.
type commandJSON struct {
	Name           string `json:"name"`
	Responsibility string `json:"responsibility"`
	SourceHint     string `json:"source_hint"`
}

// NewCommandRegistry parses a JSON array of command objects into a CommandRegistry.
func NewCommandRegistry(jsonData []byte) (*CommandRegistry, error) {
	var raw []commandJSON
	if err := json.Unmarshal(jsonData, &raw); err != nil {
		return nil, fmt.Errorf("parsing commands JSON: %w", err)
	}

	cmds := make([]models.PortingModule, len(raw))
	idx := make(map[string]*models.PortingModule, len(raw))

	for i, r := range raw {
		cmds[i] = models.PortingModule{
			Name:           r.Name,
			Responsibility: r.Responsibility,
			SourceHint:     r.SourceHint,
			Status:         "mirrored",
		}
		idx[strings.ToLower(cmds[i].Name)] = &cmds[i]
	}

	return &CommandRegistry{
		commands: cmds,
		index:    idx,
	}, nil
}

// GetCommand returns the command matching name (case-insensitive), or ErrCommandNotFound.
func (cr *CommandRegistry) GetCommand(name string) (*models.PortingModule, error) {
	cmd, ok := cr.index[strings.ToLower(name)]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrCommandNotFound, name)
	}
	return cmd, nil
}

// FindCommands returns commands whose name or source_hint contain query (case-insensitive),
// limited to at most limit results. If limit <= 0, all matches are returned.
func (cr *CommandRegistry) FindCommands(query string, limit int) []models.PortingModule {
	if query == "" {
		if limit > 0 && limit < len(cr.commands) {
			result := make([]models.PortingModule, limit)
			copy(result, cr.commands[:limit])
			return result
		}
		result := make([]models.PortingModule, len(cr.commands))
		copy(result, cr.commands)
		return result
	}

	q := strings.ToLower(query)
	var results []models.PortingModule
	for _, cmd := range cr.commands {
		if limit > 0 && len(results) >= limit {
			break
		}
		if strings.Contains(strings.ToLower(cmd.Name), q) ||
			strings.Contains(strings.ToLower(cmd.SourceHint), q) {
			results = append(results, cmd)
		}
	}
	return results
}

// GetCommands returns commands filtered by source_hint category.
// includePlugins includes commands with "plugin" in source_hint.
// includeSkills includes commands with "skills" in source_hint.
// If both are false, only built-in commands (no plugin/skill hint) are returned.
func (cr *CommandRegistry) GetCommands(includePlugins, includeSkills bool) []models.PortingModule {
	var results []models.PortingModule
	for _, cmd := range cr.commands {
		hint := strings.ToLower(cmd.SourceHint)
		isPlugin := strings.Contains(hint, "plugin")
		isSkill := strings.Contains(hint, "skill")

		if isPlugin && !includePlugins {
			continue
		}
		if isSkill && !includeSkills {
			continue
		}
		if !isPlugin && !isSkill {
			// Built-in: always included
			results = append(results, cmd)
			continue
		}
		results = append(results, cmd)
	}
	return results
}

// BuiltInCommandNames returns the names of commands that are not plugins or skills.
func (cr *CommandRegistry) BuiltInCommandNames() []string {
	var names []string
	for _, cmd := range cr.commands {
		hint := strings.ToLower(cmd.SourceHint)
		if !strings.Contains(hint, "plugin") && !strings.Contains(hint, "skill") {
			names = append(names, cmd.Name)
		}
	}
	return names
}

// ExecuteCommand returns a CommandExecution result for the named command.
// If the command is not found, Handled is false and Message describes the error.
func (cr *CommandRegistry) ExecuteCommand(name, prompt string) CommandExecution {
	cmd, err := cr.GetCommand(name)
	if err != nil {
		return CommandExecution{
			Name:    name,
			Prompt:  prompt,
			Handled: false,
			Message: fmt.Sprintf("command not found: %s", name),
		}
	}
	return CommandExecution{
		Name:       cmd.Name,
		SourceHint: cmd.SourceHint,
		Prompt:     prompt,
		Handled:    true,
		Message:    fmt.Sprintf("executed command: %s", cmd.Name),
	}
}

// RenderIndex returns a Markdown-formatted command index.
// If query is non-empty, only matching commands are shown.
// limit controls the max number of entries (0 = unlimited).
func (cr *CommandRegistry) RenderIndex(limit int, query string) string {
	cmds := cr.FindCommands(query, limit)

	var sb strings.Builder
	sb.WriteString("# Command Index\n\n")
	if len(cmds) == 0 {
		sb.WriteString("No commands found.\n")
		return sb.String()
	}

	for _, cmd := range cmds {
		sb.WriteString(fmt.Sprintf("- **%s** — %s (source: %s)\n", cmd.Name, cmd.Responsibility, cmd.SourceHint))
	}
	return sb.String()
}
