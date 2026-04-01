package execution

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gocode/gocode/internal/commands"
	"github.com/gocode/gocode/internal/tools"
)

// ErrExecutableNotFound is returned when a lookup fails.
var ErrExecutableNotFound = errors.New("executable not found")

// Executable is the common interface for commands and tools in the execution registry.
type Executable interface {
	Name() string
	Kind() string // "command" or "tool"
	Execute(payload string) string
}

// MirroredCommand wraps a command for unified execution.
type MirroredCommand struct {
	name       string
	sourceHint string
	registry   *commands.CommandRegistry
}

// Name returns the command name.
func (mc *MirroredCommand) Name() string { return mc.name }

// Kind returns "command".
func (mc *MirroredCommand) Kind() string { return "command" }

// Execute delegates to the command registry's ExecuteCommand and returns the message.
func (mc *MirroredCommand) Execute(payload string) string {
	result := mc.registry.ExecuteCommand(mc.name, payload)
	return result.Message
}

// MirroredTool wraps a tool for unified execution.
type MirroredTool struct {
	name       string
	sourceHint string
	registry   *tools.ToolRegistry
}

// Name returns the tool name.
func (mt *MirroredTool) Name() string { return mt.name }

// Kind returns "tool".
func (mt *MirroredTool) Kind() string { return "tool" }

// Execute delegates to the tool registry's ExecuteTool and returns the message.
func (mt *MirroredTool) Execute(payload string) string {
	result := mt.registry.ExecuteTool(mt.name, payload)
	return result.Message
}

// ExecutionRegistry provides unified lookup for commands and tools.
type ExecutionRegistry struct {
	entries map[string]Executable
}

// BuildExecutionRegistry creates an ExecutionRegistry populated from both registries.
func BuildExecutionRegistry(cmdReg *commands.CommandRegistry, toolReg *tools.ToolRegistry) *ExecutionRegistry {
	er := &ExecutionRegistry{
		entries: make(map[string]Executable),
	}

	// Add all commands
	for _, cmd := range cmdReg.FindCommands("", 0) {
		er.entries[strings.ToLower(cmd.Name)] = &MirroredCommand{
			name:       cmd.Name,
			sourceHint: cmd.SourceHint,
			registry:   cmdReg,
		}
	}

	// Add all tools
	for _, tool := range toolReg.FindTools("", 0) {
		er.entries[strings.ToLower(tool.Name)] = &MirroredTool{
			name:       tool.Name,
			sourceHint: tool.SourceHint,
			registry:   toolReg,
		}
	}

	return er
}

// Lookup returns the Executable matching name (case-insensitive), or an error if not found.
func (er *ExecutionRegistry) Lookup(name string) (Executable, error) {
	entry, ok := er.entries[strings.ToLower(name)]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrExecutableNotFound, name)
	}
	return entry, nil
}
