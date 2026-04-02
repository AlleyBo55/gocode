package lsp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AlleyBo55/gocode/internal/toolimpl"
)

// ---------------------------------------------------------------------------
// LSP Tool implementations — each wraps the LSP Client and implements
// toolimpl.ToolExecutor so they can be registered in the tool registry.
// ---------------------------------------------------------------------------

// LspRenameTool renames a symbol at a given file position via the language server.
// Params: file (string), line (int, 0-based), col (int, 0-based), new_name (string).
type LspRenameTool struct {
	Client *Client
}

func (t *LspRenameTool) Execute(params map[string]interface{}) toolimpl.ToolResult {
	file, _ := params["file"].(string)
	if file == "" {
		return toolimpl.ToolResult{Success: false, Error: "missing required param: file"}
	}
	line, err := toIntParam(params, "line")
	if err != nil {
		return toolimpl.ToolResult{Success: false, Error: "missing or invalid param: line (0-based int)"}
	}
	col, err := toIntParam(params, "col")
	if err != nil {
		return toolimpl.ToolResult{Success: false, Error: "missing or invalid param: col (0-based int)"}
	}
	newName, _ := params["new_name"].(string)
	if newName == "" {
		return toolimpl.ToolResult{Success: false, Error: "missing required param: new_name"}
	}

	edits, err := t.Client.Rename(file, line, col, newName)
	if err != nil {
		return toolimpl.ToolResult{Success: false, Error: fmt.Sprintf("lsp_rename: %v", err)}
	}

	out, err := json.Marshal(edits)
	if err != nil {
		return toolimpl.ToolResult{Success: false, Error: fmt.Sprintf("lsp_rename: marshal result: %v", err)}
	}
	return toolimpl.ToolResult{Success: true, Output: string(out)}
}

// LspGotoDefinitionTool returns the definition location(s) for a symbol.
// Params: file (string), line (int, 0-based), col (int, 0-based).
type LspGotoDefinitionTool struct {
	Client *Client
}

func (t *LspGotoDefinitionTool) Execute(params map[string]interface{}) toolimpl.ToolResult {
	file, _ := params["file"].(string)
	if file == "" {
		return toolimpl.ToolResult{Success: false, Error: "missing required param: file"}
	}
	line, err := toIntParam(params, "line")
	if err != nil {
		return toolimpl.ToolResult{Success: false, Error: "missing or invalid param: line (0-based int)"}
	}
	col, err := toIntParam(params, "col")
	if err != nil {
		return toolimpl.ToolResult{Success: false, Error: "missing or invalid param: col (0-based int)"}
	}

	locations, err := t.Client.GotoDefinition(file, line, col)
	if err != nil {
		return toolimpl.ToolResult{Success: false, Error: fmt.Sprintf("lsp_goto_definition: %v", err)}
	}

	return toolimpl.ToolResult{Success: true, Output: formatLocations(locations)}
}

// LspFindReferencesTool returns all reference locations for a symbol.
// Params: file (string), line (int, 0-based), col (int, 0-based).
type LspFindReferencesTool struct {
	Client *Client
}

func (t *LspFindReferencesTool) Execute(params map[string]interface{}) toolimpl.ToolResult {
	file, _ := params["file"].(string)
	if file == "" {
		return toolimpl.ToolResult{Success: false, Error: "missing required param: file"}
	}
	line, err := toIntParam(params, "line")
	if err != nil {
		return toolimpl.ToolResult{Success: false, Error: "missing or invalid param: line (0-based int)"}
	}
	col, err := toIntParam(params, "col")
	if err != nil {
		return toolimpl.ToolResult{Success: false, Error: "missing or invalid param: col (0-based int)"}
	}

	locations, err := t.Client.FindReferences(file, line, col)
	if err != nil {
		return toolimpl.ToolResult{Success: false, Error: fmt.Sprintf("lsp_find_references: %v", err)}
	}

	return toolimpl.ToolResult{Success: true, Output: formatLocations(locations)}
}

// LspDiagnosticsTool returns diagnostics (errors, warnings) for a file.
// Params: file (string).
type LspDiagnosticsTool struct {
	Client *Client
}

func (t *LspDiagnosticsTool) Execute(params map[string]interface{}) toolimpl.ToolResult {
	file, _ := params["file"].(string)
	if file == "" {
		return toolimpl.ToolResult{Success: false, Error: "missing required param: file"}
	}

	diags, err := t.Client.Diagnostics(file)
	if err != nil {
		return toolimpl.ToolResult{Success: false, Error: fmt.Sprintf("lsp_diagnostics: %v", err)}
	}

	out, err := json.Marshal(diags)
	if err != nil {
		return toolimpl.ToolResult{Success: false, Error: fmt.Sprintf("lsp_diagnostics: marshal result: %v", err)}
	}
	return toolimpl.ToolResult{Success: true, Output: string(out)}
}

// ---------------------------------------------------------------------------
// Registration
// ---------------------------------------------------------------------------

// RegisterLSPTools registers all four LSP tools in the given registry,
// each backed by the provided Client instance.
func RegisterLSPTools(r *toolimpl.Registry, client *Client) {
	r.Set("lsp_rename", &LspRenameTool{Client: client})
	r.Set("lsp_goto_definition", &LspGotoDefinitionTool{Client: client})
	r.Set("lsp_find_references", &LspFindReferencesTool{Client: client})
	r.Set("lsp_diagnostics", &LspDiagnosticsTool{Client: client})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// toIntParam extracts an integer parameter from the params map, handling
// float64 (default JSON number type) and json.Number.
func toIntParam(params map[string]interface{}, key string) (int, error) {
	v, ok := params[key]
	if !ok {
		return 0, fmt.Errorf("missing param: %s", key)
	}
	switch n := v.(type) {
	case float64:
		return int(n), nil
	case int:
		return n, nil
	case json.Number:
		i, err := n.Int64()
		return int(i), err
	}
	return 0, fmt.Errorf("invalid type for %s: %T", key, v)
}

// formatLocations renders a slice of Location into a human-readable string.
func formatLocations(locs []Location) string {
	if len(locs) == 0 {
		return "No locations found."
	}
	var sb strings.Builder
	for i, loc := range locs {
		path := uriToPath(loc.URI)
		sb.WriteString(fmt.Sprintf("%s:%d:%d",
			path,
			loc.Range.Start.Line+1,   // display as 1-based
			loc.Range.Start.Character+1,
		))
		if i < len(locs)-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}
