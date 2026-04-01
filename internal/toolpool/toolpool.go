package toolpool

import (
	"fmt"
	"strings"

	"github.com/gocode/gocode/internal/models"
	"github.com/gocode/gocode/internal/permissions"
	"github.com/gocode/gocode/internal/tools"
)

// ToolPool holds the assembled set of tools available for a session,
// filtered by mode flags and permissions.
type ToolPool struct {
	Tools      []models.PortingModule
	SimpleMode bool
	IncludeMCP bool
}

// Render returns a Markdown-formatted summary of the tool pool,
// showing mode flags, tool count, and up to the first 15 tools.
func (tp *ToolPool) Render() string {
	var sb strings.Builder
	sb.WriteString("# Tool Pool\n\n")
	sb.WriteString(fmt.Sprintf("- Simple Mode: %v\n", tp.SimpleMode))
	sb.WriteString(fmt.Sprintf("- Include MCP: %v\n", tp.IncludeMCP))
	sb.WriteString(fmt.Sprintf("- Tool Count: %d\n\n", len(tp.Tools)))

	if len(tp.Tools) == 0 {
		sb.WriteString("No tools in pool.\n")
		return sb.String()
	}

	limit := len(tp.Tools)
	if limit > 15 {
		limit = 15
	}

	for _, t := range tp.Tools[:limit] {
		sb.WriteString(fmt.Sprintf("- **%s** — %s (source: %s)\n", t.Name, t.Responsibility, t.SourceHint))
	}

	if len(tp.Tools) > 15 {
		sb.WriteString(fmt.Sprintf("\n... and %d more tools\n", len(tp.Tools)-15))
	}

	return sb.String()
}

// AssembleToolPool builds a ToolPool by retrieving tools from the registry
// filtered by the given mode flags and permission checker.
func AssembleToolPool(registry *tools.ToolRegistry, simpleMode, includeMCP bool, pc permissions.PermissionChecker) *ToolPool {
	filtered := registry.GetTools(simpleMode, includeMCP, pc)
	return &ToolPool{
		Tools:      filtered,
		SimpleMode: simpleMode,
		IncludeMCP: includeMCP,
	}
}
