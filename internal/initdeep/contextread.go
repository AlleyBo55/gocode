package initdeep

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/AlleyBo55/gocode/internal/toolimpl"
)

// ContextAwareReadTool wraps an existing FileReadTool (or any read tool) and
// automatically prepends the nearest AGENTS.md content to the output when a
// file is read. This gives the agent automatic project-context awareness.
// Implements toolimpl.ToolExecutor.
type ContextAwareReadTool struct {
	Inner toolimpl.ToolExecutor
}

// Execute delegates to the inner read tool, then looks up the nearest
// AGENTS.md relative to the file's directory. If found, the AGENTS.md
// content is prepended to the output separated by a header.
func (c *ContextAwareReadTool) Execute(params map[string]interface{}) toolimpl.ToolResult {
	result := c.Inner.Execute(params)
	if !result.Success || result.Output == "" {
		return result
	}

	path, _ := params["path"].(string)
	if path == "" {
		return result
	}

	dir := filepath.Dir(path)
	agentsPath, err := FindNearestAgentsMD(dir)
	if err != nil {
		// No AGENTS.md found — return inner result unchanged.
		return result
	}

	agentsContent, err := os.ReadFile(agentsPath)
	if err != nil {
		// Can't read the file — return inner result unchanged.
		return result
	}

	combined := fmt.Sprintf("--- AGENTS.md (%s) ---\n%s\n--- End AGENTS.md ---\n\n%s", agentsPath, string(agentsContent), result.Output)
	return toolimpl.ToolResult{Success: true, Output: combined}
}

// RegisterContextAwareRead wraps the existing filereadtool in the registry
// with a ContextAwareReadTool that prepends nearest AGENTS.md content.
func RegisterContextAwareRead(r *toolimpl.Registry) {
	origRead := r.Get("filereadtool")
	if origRead != nil {
		r.Set("filereadtool", &ContextAwareReadTool{Inner: origRead})
	}
}
