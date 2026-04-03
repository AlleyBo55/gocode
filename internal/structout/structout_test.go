package structout

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRecordToolCall(t *testing.T) {
	w := NewWriter("")

	w.RecordToolCall("bashtool", map[string]interface{}{"cmd": "ls"}, "file1\nfile2", false)
	w.RecordToolCall("filewritetool", map[string]interface{}{"path": "main.go"}, "wrote 100 bytes", false)
	w.RecordToolCall("greptool", nil, "", true)

	if len(w.output.ToolCalls) != 3 {
		t.Fatalf("expected 3 tool calls, got %d", len(w.output.ToolCalls))
	}

	tc := w.output.ToolCalls[0]
	if tc.Name != "bashtool" {
		t.Errorf("expected name bashtool, got %s", tc.Name)
	}
	if tc.Output != "file1\nfile2" {
		t.Errorf("unexpected output: %s", tc.Output)
	}
	if tc.Error {
		t.Error("expected error=false for first call")
	}

	if !w.output.ToolCalls[2].Error {
		t.Error("expected error=true for third call")
	}
}

func TestRecordFileModified_Deduplication(t *testing.T) {
	w := NewWriter("")

	w.RecordFileModified("main.go")
	w.RecordFileModified("go.mod")
	w.RecordFileModified("main.go") // duplicate
	w.RecordFileModified("go.sum")
	w.RecordFileModified("go.mod") // duplicate

	if len(w.output.FilesModified) != 3 {
		t.Fatalf("expected 3 unique files, got %d: %v", len(w.output.FilesModified), w.output.FilesModified)
	}

	expected := []string{"main.go", "go.mod", "go.sum"}
	for i, f := range expected {
		if w.output.FilesModified[i] != f {
			t.Errorf("index %d: expected %s, got %s", i, f, w.output.FilesModified[i])
		}
	}
}

func TestFinalize_ProducesValidJSON(t *testing.T) {
	w := NewWriter("")

	w.RecordToolCall("filewritetool", map[string]interface{}{"path": "main.go"}, "wrote 120 bytes", false)
	w.RecordFileModified("main.go")

	usage := UsageSummary{
		InputTokens:  1500,
		OutputTokens: 800,
		TotalCost:    0.012,
	}

	data, err := w.Finalize("Created 3 files for the new feature", usage)
	if err != nil {
		t.Fatalf("Finalize error: %v", err)
	}

	var out StructuredOutput
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	if out.Result != "Created 3 files for the new feature" {
		t.Errorf("unexpected result: %s", out.Result)
	}
	if len(out.ToolCalls) != 1 {
		t.Errorf("expected 1 tool call, got %d", len(out.ToolCalls))
	}
	if len(out.FilesModified) != 1 {
		t.Errorf("expected 1 file modified, got %d", len(out.FilesModified))
	}
	if out.Usage.InputTokens != 1500 {
		t.Errorf("expected 1500 input tokens, got %d", out.Usage.InputTokens)
	}
	if out.Usage.OutputTokens != 800 {
		t.Errorf("expected 800 output tokens, got %d", out.Usage.OutputTokens)
	}
	if out.Usage.TotalCost != 0.012 {
		t.Errorf("expected total cost 0.012, got %f", out.Usage.TotalCost)
	}
}

func TestFinalize_AllRequiredFieldsPresent(t *testing.T) {
	w := NewWriter("")

	data, err := w.Finalize("done", UsageSummary{})
	if err != nil {
		t.Fatalf("Finalize error: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	requiredFields := []string{"result", "tool_calls", "files_modified", "exit_code", "usage"}
	for _, field := range requiredFields {
		if _, ok := raw[field]; !ok {
			t.Errorf("missing required field: %s", field)
		}
	}

	// Verify tool_calls and files_modified are arrays (not null)
	if _, ok := raw["tool_calls"].([]interface{}); !ok {
		t.Error("tool_calls should be a non-null array")
	}
	if _, ok := raw["files_modified"].([]interface{}); !ok {
		t.Error("files_modified should be a non-null array")
	}

	// Verify usage is an object with expected sub-fields
	usageMap, ok := raw["usage"].(map[string]interface{})
	if !ok {
		t.Fatal("usage should be an object")
	}
	for _, sub := range []string{"input_tokens", "output_tokens", "total_cost"} {
		if _, ok := usageMap[sub]; !ok {
			t.Errorf("usage missing sub-field: %s", sub)
		}
	}
}

func TestSchemaValidation_Pass(t *testing.T) {
	schema := `{
		"type": "object",
		"required": ["result", "tool_calls", "files_modified", "exit_code", "usage"],
		"properties": {
			"result": {"type": "string"},
			"tool_calls": {"type": "array"},
			"files_modified": {"type": "array"},
			"exit_code": {"type": "integer"},
			"usage": {"type": "object"}
		}
	}`

	schemaPath := writeTemp(t, "schema.json", schema)
	w := NewWriter(schemaPath)

	w.RecordToolCall("bashtool", map[string]interface{}{"cmd": "ls"}, "ok", false)
	w.RecordFileModified("main.go")

	_, err := w.Finalize("done", UsageSummary{InputTokens: 100, OutputTokens: 50, TotalCost: 0.001})
	if err != nil {
		t.Fatalf("expected schema validation to pass, got: %v", err)
	}
}

func TestSchemaValidation_Fail_MissingRequired(t *testing.T) {
	schema := `{
		"type": "object",
		"required": ["result", "custom_field"]
	}`

	schemaPath := writeTemp(t, "schema.json", schema)
	w := NewWriter(schemaPath)

	_, err := w.Finalize("done", UsageSummary{})
	if err == nil {
		t.Fatal("expected schema validation to fail for missing custom_field")
	}
}

func TestSchemaValidation_Fail_WrongType(t *testing.T) {
	// Schema expects result to be an integer, but it's a string
	schema := `{
		"type": "object",
		"properties": {
			"result": {"type": "integer"}
		}
	}`

	schemaPath := writeTemp(t, "schema.json", schema)
	w := NewWriter(schemaPath)

	_, err := w.Finalize("done", UsageSummary{})
	if err == nil {
		t.Fatal("expected schema validation to fail for wrong type")
	}
}

func TestNewWriter_NoSchema(t *testing.T) {
	w := NewWriter("")
	if w.schema != nil {
		t.Error("expected nil schema when no path provided")
	}
	if w.schemaPath != "" {
		t.Error("expected empty schemaPath")
	}
}

func TestNewWriter_InvalidSchemaPath(t *testing.T) {
	w := NewWriter("/nonexistent/path/schema.json")
	if w.schema != nil {
		t.Error("expected nil schema for invalid path")
	}
}

func TestValidateAgainstSchema_NoSchema(t *testing.T) {
	w := NewWriter("")
	err := w.ValidateAgainstSchema([]byte(`{"result": "ok"}`))
	if err != nil {
		t.Errorf("expected no error with no schema, got: %v", err)
	}
}

func TestValidateAgainstSchema_InvalidJSON(t *testing.T) {
	schema := `{"type": "object", "required": ["result"]}`
	schemaPath := writeTemp(t, "schema.json", schema)
	w := NewWriter(schemaPath)

	err := w.ValidateAgainstSchema([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// writeTemp creates a temporary file with the given content and returns its path.
func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}
	return path
}
