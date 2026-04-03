package notebook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func sampleNotebook() *Notebook {
	return &Notebook{
		Metadata:    map[string]interface{}{"kernelspec": map[string]interface{}{"name": "python3"}},
		NBFormat:    4,
		NBFormatMin: 5,
		Cells: []Cell{
			{
				CellType: "code",
				Source:   []string{"import pandas as pd\n"},
				Metadata: map[string]interface{}{"collapsed": false},
				Outputs:  []interface{}{map[string]interface{}{"output_type": "stream", "text": []interface{}{"hello"}}},
			},
			{
				CellType: "markdown",
				Source:   []string{"# Title\n"},
				Metadata: map[string]interface{}{},
			},
			{
				CellType: "code",
				Source:   []string{"x = 1\n"},
				Metadata: map[string]interface{}{"scrolled": true},
				Outputs:  []interface{}{},
			},
		},
	}
}

func writeNotebook(t *testing.T, nb *Notebook) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.ipynb")
	if err := nb.WriteToFile(path); err != nil {
		t.Fatalf("WriteToFile: %v", err)
	}
	return path
}

func TestEditPreservesMetadata(t *testing.T) {
	nb := sampleNotebook()
	path := writeNotebook(t, nb)

	tool := &NotebookEditTool{}
	result := tool.Execute(map[string]interface{}{
		"path":       path,
		"action":     "edit",
		"cell_index": float64(0),
		"new_source": "import numpy as np\n",
	})
	if !result.Success {
		t.Fatalf("edit failed: %s", result.Error)
	}

	// Re-read and verify
	edited, err := ParseNotebook(path)
	if err != nil {
		t.Fatalf("ParseNotebook: %v", err)
	}

	// Source should be updated
	if len(edited.Cells[0].Source) == 0 || edited.Cells[0].Source[0] != "import numpy as np\n" {
		t.Errorf("source not updated: %v", edited.Cells[0].Source)
	}

	// Metadata should be preserved
	if edited.Cells[0].Metadata == nil {
		t.Fatal("metadata was lost")
	}
	if _, ok := edited.Cells[0].Metadata["collapsed"]; !ok {
		t.Error("cell metadata 'collapsed' was lost")
	}

	// Outputs should be preserved
	if len(edited.Cells[0].Outputs) != 1 {
		t.Errorf("outputs were lost: got %d, want 1", len(edited.Cells[0].Outputs))
	}

	// Notebook-level metadata preserved
	if edited.NBFormat != 4 || edited.NBFormatMin != 5 {
		t.Errorf("notebook format changed: %d.%d", edited.NBFormat, edited.NBFormatMin)
	}
	if edited.Metadata == nil {
		t.Error("notebook metadata was lost")
	}
}

func TestAddCellCount(t *testing.T) {
	nb := sampleNotebook()
	originalCount := len(nb.Cells)
	path := writeNotebook(t, nb)

	tool := &NotebookEditTool{}
	result := tool.Execute(map[string]interface{}{
		"path":       path,
		"action":     "add",
		"cell_index": float64(1),
		"cell_type":  "code",
		"new_source": "y = 2\n",
	})
	if !result.Success {
		t.Fatalf("add failed: %s", result.Error)
	}

	added, err := ParseNotebook(path)
	if err != nil {
		t.Fatalf("ParseNotebook: %v", err)
	}
	if len(added.Cells) != originalCount+1 {
		t.Errorf("cell count: got %d, want %d", len(added.Cells), originalCount+1)
	}
	// Verify the new cell is at index 1
	if added.Cells[1].CellType != "code" {
		t.Errorf("new cell type: got %q, want %q", added.Cells[1].CellType, "code")
	}
}

func TestRemoveCellCount(t *testing.T) {
	nb := sampleNotebook()
	originalCount := len(nb.Cells)
	path := writeNotebook(t, nb)

	tool := &NotebookEditTool{}
	result := tool.Execute(map[string]interface{}{
		"path":       path,
		"action":     "remove",
		"cell_index": float64(1),
	})
	if !result.Success {
		t.Fatalf("remove failed: %s", result.Error)
	}

	removed, err := ParseNotebook(path)
	if err != nil {
		t.Fatalf("ParseNotebook: %v", err)
	}
	if len(removed.Cells) != originalCount-1 {
		t.Errorf("cell count: got %d, want %d", len(removed.Cells), originalCount-1)
	}
}

func TestReorderPreservesCells(t *testing.T) {
	nb := sampleNotebook()
	originalCount := len(nb.Cells)
	// Record original cell contents
	originalSources := make([]string, len(nb.Cells))
	for i, c := range nb.Cells {
		if len(c.Source) > 0 {
			originalSources[i] = c.Source[0]
		}
	}
	path := writeNotebook(t, nb)

	tool := &NotebookEditTool{}
	result := tool.Execute(map[string]interface{}{
		"path":       path,
		"action":     "reorder",
		"cell_index": float64(0),
		"new_index":  float64(2),
	})
	if !result.Success {
		t.Fatalf("reorder failed: %s", result.Error)
	}

	reordered, err := ParseNotebook(path)
	if err != nil {
		t.Fatalf("ParseNotebook: %v", err)
	}

	// Same number of cells
	if len(reordered.Cells) != originalCount {
		t.Errorf("cell count changed: got %d, want %d", len(reordered.Cells), originalCount)
	}

	// Same set of sources (just reordered)
	reorderedSources := make(map[string]bool)
	for _, c := range reordered.Cells {
		if len(c.Source) > 0 {
			reorderedSources[c.Source[0]] = true
		}
	}
	for _, src := range originalSources {
		if src != "" && !reorderedSources[src] {
			t.Errorf("source %q missing after reorder", src)
		}
	}
}

func TestInvalidCellIndex(t *testing.T) {
	nb := sampleNotebook()
	path := writeNotebook(t, nb)

	tool := &NotebookEditTool{}

	// Index too high
	result := tool.Execute(map[string]interface{}{
		"path":       path,
		"action":     "edit",
		"cell_index": float64(99),
		"new_source": "x",
	})
	if result.Success {
		t.Error("expected failure for out-of-range index")
	}

	// Negative index
	result = tool.Execute(map[string]interface{}{
		"path":       path,
		"action":     "edit",
		"cell_index": float64(-1),
		"new_source": "x",
	})
	if result.Success {
		t.Error("expected failure for negative index")
	}

	// Missing index
	result = tool.Execute(map[string]interface{}{
		"path":   path,
		"action": "remove",
	})
	if result.Success {
		t.Error("expected failure for missing cell_index")
	}
}

func TestInvalidNotebookJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.ipynb")
	if err := os.WriteFile(path, []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := &NotebookEditTool{}
	result := tool.Execute(map[string]interface{}{
		"path":       path,
		"action":     "edit",
		"cell_index": float64(0),
		"new_source": "x",
	})
	if result.Success {
		t.Error("expected failure for invalid JSON")
	}
}

func TestEmptyNotebook(t *testing.T) {
	nb := &Notebook{
		Metadata:    map[string]interface{}{},
		NBFormat:    4,
		NBFormatMin: 5,
		Cells:       []Cell{},
	}
	path := writeNotebook(t, nb)

	tool := &NotebookEditTool{}

	// Edit on empty notebook should fail
	result := tool.Execute(map[string]interface{}{
		"path":       path,
		"action":     "edit",
		"cell_index": float64(0),
		"new_source": "x",
	})
	if result.Success {
		t.Error("expected failure for edit on empty notebook")
	}

	// Add to empty notebook should work
	result = tool.Execute(map[string]interface{}{
		"path":       path,
		"action":     "add",
		"cell_type":  "code",
		"new_source": "print('hello')\n",
	})
	if !result.Success {
		t.Fatalf("add to empty notebook failed: %s", result.Error)
	}

	added, err := ParseNotebook(path)
	if err != nil {
		t.Fatalf("ParseNotebook: %v", err)
	}
	if len(added.Cells) != 1 {
		t.Errorf("expected 1 cell, got %d", len(added.Cells))
	}
}

func TestParseSerializeRoundTrip(t *testing.T) {
	nb := sampleNotebook()
	data, err := nb.Serialize()
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}

	parsed, err := ParseNotebookBytes(data)
	if err != nil {
		t.Fatalf("ParseNotebookBytes: %v", err)
	}

	// Verify structural equality via JSON
	data1, _ := json.Marshal(nb)
	data2, _ := json.Marshal(parsed)
	if string(data1) != string(data2) {
		t.Error("round-trip produced different notebook")
	}
}
