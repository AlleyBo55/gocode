package structout

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// StructuredOutput is the JSON envelope for --output-format json.
type StructuredOutput struct {
	Result        string          `json:"result"`
	ToolCalls     []ToolCallEntry `json:"tool_calls"`
	FilesModified []string        `json:"files_modified"`
	ExitCode      int             `json:"exit_code"`
	Usage         UsageSummary    `json:"usage"`
}

// ToolCallEntry records a single tool invocation.
type ToolCallEntry struct {
	Name   string                 `json:"name"`
	Input  map[string]interface{} `json:"input"`
	Output string                 `json:"output"`
	Error  bool                   `json:"error"`
}

// UsageSummary holds token counts and cost.
type UsageSummary struct {
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	TotalCost    float64 `json:"total_cost"`
}

// Writer collects tool calls and file modifications, then produces the final JSON output.
type Writer struct {
	mu         sync.Mutex
	output     StructuredOutput
	schemaPath string
	schema     map[string]interface{} // parsed JSON Schema (nil if no schema)
}

// NewWriter creates a Writer, optionally loading a JSON Schema for validation.
// If schemaPath is empty, no schema validation is performed.
func NewWriter(schemaPath string) *Writer {
	w := &Writer{
		schemaPath: schemaPath,
		output: StructuredOutput{
			ToolCalls:     []ToolCallEntry{},
			FilesModified: []string{},
		},
	}
	if schemaPath != "" {
		data, err := os.ReadFile(schemaPath)
		if err == nil {
			var s map[string]interface{}
			if jsonErr := json.Unmarshal(data, &s); jsonErr == nil {
				w.schema = s
			}
		}
	}
	return w
}

// RecordToolCall adds a tool call entry.
func (w *Writer) RecordToolCall(name string, input map[string]interface{}, output string, isError bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.output.ToolCalls = append(w.output.ToolCalls, ToolCallEntry{
		Name:   name,
		Input:  input,
		Output: output,
		Error:  isError,
	})
}

// RecordFileModified adds a file to the modified list, deduplicating paths.
func (w *Writer) RecordFileModified(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, existing := range w.output.FilesModified {
		if existing == path {
			return
		}
	}
	w.output.FilesModified = append(w.output.FilesModified, path)
}

// Finalize sets the result text and usage, then produces JSON bytes.
// If a schema was loaded, it validates the output against the schema.
func (w *Writer) Finalize(result string, usage UsageSummary) ([]byte, error) {
	w.mu.Lock()
	w.output.Result = result
	w.output.Usage = usage
	w.mu.Unlock()

	data, err := json.MarshalIndent(w.output, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling structured output: %w", err)
	}

	if w.schema != nil {
		if valErr := w.ValidateAgainstSchema(data); valErr != nil {
			return data, valErr
		}
	}

	return data, nil
}

// ValidateAgainstSchema validates JSON data against the loaded schema.
// It performs basic validation: checks that all required fields exist in the data.
func (w *Writer) ValidateAgainstSchema(data []byte) error {
	if w.schema == nil {
		return nil
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return fmt.Errorf("schema validation: invalid JSON: %w", err)
	}

	// Check required fields from schema
	if required, ok := w.schema["required"]; ok {
		if reqList, ok := required.([]interface{}); ok {
			for _, r := range reqList {
				fieldName, ok := r.(string)
				if !ok {
					continue
				}
				if _, exists := parsed[fieldName]; !exists {
					return fmt.Errorf("schema validation failed: missing required field %q", fieldName)
				}
			}
		}
	}

	// Check property types if "properties" is defined
	properties, ok := w.schema["properties"]
	if !ok {
		return nil
	}

	propMap, ok := properties.(map[string]interface{})
	if !ok {
		return nil
	}

	for fieldName, propDef := range propMap {
		val, exists := parsed[fieldName]
		if !exists {
			continue
		}
		prop, ok := propDef.(map[string]interface{})
		if !ok {
			continue
		}
		expectedType, ok := prop["type"].(string)
		if !ok {
			continue
		}
		if err := checkType(fieldName, val, expectedType); err != nil {
			return err
		}
	}

	return nil
}

// checkType validates that a value matches the expected JSON Schema type.
func checkType(field string, val interface{}, expectedType string) error {
	switch expectedType {
	case "string":
		if _, ok := val.(string); !ok {
			return fmt.Errorf("schema validation failed: field %q expected string", field)
		}
	case "integer":
		switch v := val.(type) {
		case float64:
			if v != float64(int(v)) {
				return fmt.Errorf("schema validation failed: field %q expected integer", field)
			}
		default:
			return fmt.Errorf("schema validation failed: field %q expected integer", field)
		}
	case "number":
		if _, ok := val.(float64); !ok {
			return fmt.Errorf("schema validation failed: field %q expected number", field)
		}
	case "array":
		if _, ok := val.([]interface{}); !ok {
			return fmt.Errorf("schema validation failed: field %q expected array", field)
		}
	case "object":
		if _, ok := val.(map[string]interface{}); !ok {
			return fmt.Errorf("schema validation failed: field %q expected object", field)
		}
	case "boolean":
		if _, ok := val.(bool); !ok {
			return fmt.Errorf("schema validation failed: field %q expected boolean", field)
		}
	}
	return nil
}

// ToolCallbackWrapper wraps a Writer to record tool calls during agent execution.
// It implements agent.ToolCallback by tracking tool start/end events.
type ToolCallbackWrapper struct {
	Writer    *Writer
	pending   map[string]map[string]interface{} // tool name -> input for in-flight calls
	pendingMu sync.Mutex
}

// NewToolCallbackWrapper creates a callback wrapper around a Writer.
func NewToolCallbackWrapper(w *Writer) *ToolCallbackWrapper {
	return &ToolCallbackWrapper{
		Writer:  w,
		pending: make(map[string]map[string]interface{}),
	}
}

// OnToolStart records the tool name and input for later recording.
func (c *ToolCallbackWrapper) OnToolStart(name string, input map[string]interface{}) {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	c.pending[name] = input

	// Track file modifications for write/edit tools
	if name == "filewritetool" || name == "fileedittool" || name == "notebookedittool" {
		if path, ok := input["path"].(string); ok && path != "" {
			c.Writer.RecordFileModified(path)
		}
	}
}

// OnToolEnd records the completed tool call.
func (c *ToolCallbackWrapper) OnToolEnd(name string, success bool) {
	c.pendingMu.Lock()
	input := c.pending[name]
	delete(c.pending, name)
	c.pendingMu.Unlock()

	c.Writer.RecordToolCall(name, input, "", !success)
}
