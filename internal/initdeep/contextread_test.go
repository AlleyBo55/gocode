package initdeep

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlleyBo55/gocode/internal/toolimpl"
)

// fakeReadTool is a stub ToolExecutor that returns a fixed result.
type fakeReadTool struct {
	result toolimpl.ToolResult
}

func (f *fakeReadTool) Execute(params map[string]interface{}) toolimpl.ToolResult {
	return f.result
}

func TestContextAwareReadTool_WithAgentsMD(t *testing.T) {
	// Create a temp directory with an AGENTS.md and a source file.
	dir := t.TempDir()
	agentsContent := "# mypackage\n\nThis is the agents context.\n"
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(agentsContent), 0644); err != nil {
		t.Fatal(err)
	}
	srcFile := filepath.Join(dir, "main.go")
	if err := os.WriteFile(srcFile, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	inner := &fakeReadTool{result: toolimpl.ToolResult{Success: true, Output: "1: package main\n"}}
	tool := &ContextAwareReadTool{Inner: inner}

	result := tool.Execute(map[string]interface{}{"path": srcFile})
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "--- AGENTS.md") {
		t.Errorf("expected AGENTS.md header in output, got:\n%s", result.Output)
	}
	if !strings.Contains(result.Output, agentsContent) {
		t.Errorf("expected AGENTS.md content in output, got:\n%s", result.Output)
	}
	if !strings.Contains(result.Output, "1: package main") {
		t.Errorf("expected original file content in output, got:\n%s", result.Output)
	}
}

func TestContextAwareReadTool_NoAgentsMD(t *testing.T) {
	// Create a temp directory without AGENTS.md.
	dir := t.TempDir()
	srcFile := filepath.Join(dir, "main.go")
	if err := os.WriteFile(srcFile, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	innerOutput := "1: package main\n"
	inner := &fakeReadTool{result: toolimpl.ToolResult{Success: true, Output: innerOutput}}
	tool := &ContextAwareReadTool{Inner: inner}

	result := tool.Execute(map[string]interface{}{"path": srcFile})
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	if result.Output != innerOutput {
		t.Errorf("expected unchanged output %q, got %q", innerOutput, result.Output)
	}
}

func TestContextAwareReadTool_InnerFailure(t *testing.T) {
	inner := &fakeReadTool{result: toolimpl.ToolResult{Success: false, Error: "file not found"}}
	tool := &ContextAwareReadTool{Inner: inner}

	result := tool.Execute(map[string]interface{}{"path": "/nonexistent/file.go"})
	if result.Success {
		t.Fatal("expected failure")
	}
	if result.Error != "file not found" {
		t.Errorf("expected inner error, got: %s", result.Error)
	}
}

func TestContextAwareReadTool_AncestorAgentsMD(t *testing.T) {
	// AGENTS.md in parent, source file in child.
	parent := t.TempDir()
	child := filepath.Join(parent, "sub")
	if err := os.Mkdir(child, 0755); err != nil {
		t.Fatal(err)
	}
	agentsContent := "# parent context\n"
	if err := os.WriteFile(filepath.Join(parent, "AGENTS.md"), []byte(agentsContent), 0644); err != nil {
		t.Fatal(err)
	}
	srcFile := filepath.Join(child, "lib.go")
	if err := os.WriteFile(srcFile, []byte("package sub\n"), 0644); err != nil {
		t.Fatal(err)
	}

	inner := &fakeReadTool{result: toolimpl.ToolResult{Success: true, Output: "1: package sub\n"}}
	tool := &ContextAwareReadTool{Inner: inner}

	result := tool.Execute(map[string]interface{}{"path": srcFile})
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "--- AGENTS.md") {
		t.Errorf("expected AGENTS.md header from ancestor, got:\n%s", result.Output)
	}
	if !strings.Contains(result.Output, agentsContent) {
		t.Errorf("expected ancestor AGENTS.md content in output")
	}
}

func TestRegisterContextAwareRead(t *testing.T) {
	r := toolimpl.NewRegistry()
	origRead := r.Get("filereadtool")
	if origRead == nil {
		t.Fatal("expected filereadtool to exist in registry")
	}

	RegisterContextAwareRead(r)

	wrapped := r.Get("filereadtool")
	if wrapped == nil {
		t.Fatal("expected filereadtool to still exist after registration")
	}

	// The wrapped tool should be a ContextAwareReadTool.
	car, ok := wrapped.(*ContextAwareReadTool)
	if !ok {
		t.Fatalf("expected *ContextAwareReadTool, got %T", wrapped)
	}
	if car.Inner == nil {
		t.Fatal("expected Inner to be set")
	}
}
