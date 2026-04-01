package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AlleyBo55/gocode/internal/apitypes"
	"github.com/AlleyBo55/gocode/internal/models"
	"github.com/AlleyBo55/gocode/internal/toolimpl"
	"github.com/AlleyBo55/gocode/internal/tools"
)

// ToolExecutor dispatches tool-use requests to implementations.
type ToolExecutor interface {
	Execute(name string, input map[string]interface{}) apitypes.ToolResult
	ListTools() []apitypes.ToolDef
}

// RegistryExecutor bridges the existing toolimpl.Registry and tools.ToolRegistry
// to the agent's ToolExecutor interface.
type RegistryExecutor struct {
	implRegistry *toolimpl.Registry
	toolRegistry *tools.ToolRegistry
}

// NewRegistryExecutor creates a ToolExecutor backed by existing registries.
func NewRegistryExecutor(impl *toolimpl.Registry, toolReg *tools.ToolRegistry) *RegistryExecutor {
	return &RegistryExecutor{implRegistry: impl, toolRegistry: toolReg}
}

// Execute dispatches a tool call to the appropriate implementation.
func (e *RegistryExecutor) Execute(name string, input map[string]interface{}) apitypes.ToolResult {
	// Serialize input to JSON for the existing toolimpl interface
	payload, err := json.Marshal(input)
	if err != nil {
		return apitypes.ToolResult{Output: fmt.Sprintf("failed to serialize input: %v", err), IsError: true}
	}

	result := e.implRegistry.ExecuteTool(name, string(payload))
	if !result.Success {
		errMsg := result.Error
		if errMsg == "" {
			errMsg = result.Output
		}
		return apitypes.ToolResult{Output: errMsg, IsError: true}
	}
	return apitypes.ToolResult{Output: result.Output, IsError: false}
}

// ListTools converts all registered tool definitions to apitypes.ToolDef.
func (e *RegistryExecutor) ListTools() []apitypes.ToolDef {
	defs := e.toolRegistry.GetToolDefinitions()
	result := make([]apitypes.ToolDef, 0, len(defs))
	for _, d := range defs {
		result = append(result, convertToolDef(d))
	}
	return result
}

// convertToolDef converts a models.ToolDefinition to apitypes.ToolDef.
func convertToolDef(d models.ToolDefinition) apitypes.ToolDef {
	schema, _ := json.Marshal(d.InputSchema)
	return apitypes.ToolDef{
		Name:        d.Name,
		Description: d.Description,
		InputSchema: schema,
	}
}

// StaticExecutor is a simple ToolExecutor for testing with registered handlers.
type StaticExecutor struct {
	handlers map[string]func(map[string]interface{}) apitypes.ToolResult
	tools    []apitypes.ToolDef
}

// NewStaticExecutor creates a StaticExecutor.
func NewStaticExecutor() *StaticExecutor {
	return &StaticExecutor{handlers: make(map[string]func(map[string]interface{}) apitypes.ToolResult)}
}

// Register adds a tool handler.
func (e *StaticExecutor) Register(name string, handler func(map[string]interface{}) apitypes.ToolResult) *StaticExecutor {
	e.handlers[name] = handler
	e.tools = append(e.tools, apitypes.ToolDef{Name: name})
	return e
}

// Execute dispatches to the registered handler.
func (e *StaticExecutor) Execute(name string, input map[string]interface{}) apitypes.ToolResult {
	handler, ok := e.handlers[strings.ToLower(name)]
	if !ok {
		return apitypes.ToolResult{Output: fmt.Sprintf("unknown tool: %s", name), IsError: true}
	}
	return handler(input)
}

// ListTools returns registered tool definitions.
func (e *StaticExecutor) ListTools() []apitypes.ToolDef { return e.tools }
