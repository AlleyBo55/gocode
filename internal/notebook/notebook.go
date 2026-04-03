package notebook

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/AlleyBo55/gocode/internal/toolimpl"
)

// Notebook represents a parsed .ipynb file.
type Notebook struct {
	Metadata    map[string]interface{} `json:"metadata"`
	NBFormat    int                    `json:"nbformat"`
	NBFormatMin int                    `json:"nbformat_minor"`
	Cells       []Cell                 `json:"cells"`
}

// Cell represents a single notebook cell.
type Cell struct {
	CellType string                 `json:"cell_type"`
	Source   []string               `json:"source"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	Outputs  []interface{}          `json:"outputs,omitempty"`
}

// ParseNotebook reads and parses a .ipynb file.
func ParseNotebook(path string) (*Notebook, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading notebook: %w", err)
	}
	return ParseNotebookBytes(data)
}

// ParseNotebookBytes parses notebook JSON bytes.
func ParseNotebookBytes(data []byte) (*Notebook, error) {
	var nb Notebook
	if err := json.Unmarshal(data, &nb); err != nil {
		return nil, fmt.Errorf("parsing notebook JSON: %w", err)
	}
	return &nb, nil
}

// Serialize writes the notebook back to JSON bytes.
func (nb *Notebook) Serialize() ([]byte, error) {
	return json.MarshalIndent(nb, "", " ")
}

// WriteToFile serializes and writes the notebook to a file.
func (nb *Notebook) WriteToFile(path string) error {
	data, err := nb.Serialize()
	if err != nil {
		return fmt.Errorf("serializing notebook: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// NotebookEditTool implements toolimpl.ToolExecutor for cell-level editing.
type NotebookEditTool struct{}

// Execute handles notebook edit actions: "edit", "add", "remove", "reorder".
// Params: path, cell_index, action, new_source, cell_type
func (t *NotebookEditTool) Execute(params map[string]interface{}) toolimpl.ToolResult {
	path, _ := params["path"].(string)
	if path == "" {
		return toolimpl.ToolResult{Success: false, Error: "missing required parameter: path"}
	}
	action, _ := params["action"].(string)
	if action == "" {
		return toolimpl.ToolResult{Success: false, Error: "missing required parameter: action"}
	}

	nb, err := ParseNotebook(path)
	if err != nil {
		return toolimpl.ToolResult{Success: false, Error: err.Error()}
	}

	switch action {
	case "edit":
		return t.editCell(nb, path, params)
	case "add":
		return t.addCell(nb, path, params)
	case "remove":
		return t.removeCell(nb, path, params)
	case "reorder":
		return t.reorderCell(nb, path, params)
	default:
		return toolimpl.ToolResult{Success: false, Error: fmt.Sprintf("unknown action: %s", action)}
	}
}

func (t *NotebookEditTool) editCell(nb *Notebook, path string, params map[string]interface{}) toolimpl.ToolResult {
	idx, err := cellIndex(params, len(nb.Cells))
	if err != nil {
		return toolimpl.ToolResult{Success: false, Error: err.Error()}
	}
	newSource, ok := params["new_source"].(string)
	if !ok {
		return toolimpl.ToolResult{Success: false, Error: "missing required parameter: new_source"}
	}

	// Preserve metadata and outputs, only change source
	nb.Cells[idx].Source = splitSource(newSource)

	if err := nb.WriteToFile(path); err != nil {
		return toolimpl.ToolResult{Success: false, Error: err.Error()}
	}
	return toolimpl.ToolResult{Success: true, Output: fmt.Sprintf("edited cell %d", idx)}
}

func (t *NotebookEditTool) addCell(nb *Notebook, path string, params map[string]interface{}) toolimpl.ToolResult {
	cellType, _ := params["cell_type"].(string)
	if cellType == "" {
		cellType = "code"
	}
	newSource, _ := params["new_source"].(string)

	idx := len(nb.Cells) // default: append
	if v, ok := params["cell_index"]; ok {
		i, err := toInt(v)
		if err == nil {
			if i < 0 || i > len(nb.Cells) {
				return toolimpl.ToolResult{Success: false, Error: fmt.Sprintf("cell index %d out of range (0..%d)", i, len(nb.Cells))}
			}
			idx = i
		}
	}

	cell := Cell{
		CellType: cellType,
		Source:   splitSource(newSource),
		Metadata: map[string]interface{}{},
	}
	if cellType == "code" {
		cell.Outputs = []interface{}{}
	}

	// Insert at idx
	nb.Cells = append(nb.Cells, Cell{})
	copy(nb.Cells[idx+1:], nb.Cells[idx:])
	nb.Cells[idx] = cell

	if err := nb.WriteToFile(path); err != nil {
		return toolimpl.ToolResult{Success: false, Error: err.Error()}
	}
	return toolimpl.ToolResult{Success: true, Output: fmt.Sprintf("added %s cell at index %d", cellType, idx)}
}

func (t *NotebookEditTool) removeCell(nb *Notebook, path string, params map[string]interface{}) toolimpl.ToolResult {
	idx, err := cellIndex(params, len(nb.Cells))
	if err != nil {
		return toolimpl.ToolResult{Success: false, Error: err.Error()}
	}

	nb.Cells = append(nb.Cells[:idx], nb.Cells[idx+1:]...)

	if err := nb.WriteToFile(path); err != nil {
		return toolimpl.ToolResult{Success: false, Error: err.Error()}
	}
	return toolimpl.ToolResult{Success: true, Output: fmt.Sprintf("removed cell %d", idx)}
}

func (t *NotebookEditTool) reorderCell(nb *Notebook, path string, params map[string]interface{}) toolimpl.ToolResult {
	fromIdx, err := cellIndex(params, len(nb.Cells))
	if err != nil {
		return toolimpl.ToolResult{Success: false, Error: err.Error()}
	}

	newIdxVal, ok := params["new_index"]
	if !ok {
		return toolimpl.ToolResult{Success: false, Error: "missing required parameter: new_index for reorder"}
	}
	newIdx, err := toInt(newIdxVal)
	if err != nil || newIdx < 0 || newIdx >= len(nb.Cells) {
		return toolimpl.ToolResult{Success: false, Error: fmt.Sprintf("new_index %v out of range (0..%d)", newIdxVal, len(nb.Cells)-1)}
	}

	cell := nb.Cells[fromIdx]
	// Remove from old position
	nb.Cells = append(nb.Cells[:fromIdx], nb.Cells[fromIdx+1:]...)
	// Insert at new position
	nb.Cells = append(nb.Cells, Cell{})
	copy(nb.Cells[newIdx+1:], nb.Cells[newIdx:])
	nb.Cells[newIdx] = cell

	if err := nb.WriteToFile(path); err != nil {
		return toolimpl.ToolResult{Success: false, Error: err.Error()}
	}
	return toolimpl.ToolResult{Success: true, Output: fmt.Sprintf("moved cell from %d to %d", fromIdx, newIdx)}
}

// cellIndex extracts and validates the cell_index parameter.
func cellIndex(params map[string]interface{}, cellCount int) (int, error) {
	v, ok := params["cell_index"]
	if !ok {
		return 0, fmt.Errorf("missing required parameter: cell_index")
	}
	idx, err := toInt(v)
	if err != nil {
		return 0, fmt.Errorf("invalid cell_index: %w", err)
	}
	if idx < 0 || idx >= cellCount {
		return 0, fmt.Errorf("cell index %d out of range (0..%d)", idx, cellCount-1)
	}
	return idx, nil
}

// splitSource splits a source string into lines for the notebook format.
func splitSource(s string) []string {
	if s == "" {
		return []string{}
	}
	var lines []string
	remaining := s
	for {
		idx := indexOf(remaining, '\n')
		if idx == -1 {
			lines = append(lines, remaining)
			break
		}
		lines = append(lines, remaining[:idx+1])
		remaining = remaining[idx+1:]
	}
	return lines
}

func indexOf(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

// toInt converts an interface{} to int (handles float64 from JSON).
func toInt(v interface{}) (int, error) {
	switch val := v.(type) {
	case float64:
		return int(val), nil
	case int:
		return val, nil
	case json.Number:
		i, err := val.Int64()
		return int(i), err
	default:
		return 0, fmt.Errorf("cannot convert %T to int", v)
	}
}

// RegisterNotebookTool registers the NotebookEditTool in the registry,
// replacing the existing notebookedittool alias.
func RegisterNotebookTool(registry *toolimpl.Registry) {
	registry.Set("notebookedittool", &NotebookEditTool{})
}
