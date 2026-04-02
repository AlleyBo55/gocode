package orchestrator

import (
	"context"
	"testing"

	"github.com/AlleyBo55/gocode/internal/agent"
	"github.com/AlleyBo55/gocode/internal/apiclient"
	"github.com/AlleyBo55/gocode/internal/apitypes"
)

// --- Test helpers ---

// fakeProvider is a minimal Provider that returns a canned text response.
type fakeProvider struct {
	response string
}

func (f *fakeProvider) SendMessage(_ context.Context, req apitypes.MessageRequest) (*apitypes.MessageResponse, error) {
	return &apitypes.MessageResponse{
		Role: "assistant",
		Content: []apitypes.OutputContentBlock{
			{Kind: "text", Text: f.response},
		},
	}, nil
}

func (f *fakeProvider) StreamMessage(_ context.Context, _ apitypes.MessageRequest) (<-chan apitypes.StreamEvent, error) {
	ch := make(chan apitypes.StreamEvent)
	close(ch)
	return ch, nil
}

func (f *fakeProvider) Kind() apiclient.ProviderKind {
	return apiclient.ProviderAnthropic
}

func newTestRouter(response string) *apiclient.ModelRouter {
	fp := apiclient.NewFallbackProvider([]apiclient.FallbackEntry{
		{Model: "test-model", Provider: &fakeProvider{response: response}},
	}, nil)
	return apiclient.NewModelRouter(map[apiclient.TaskCategory]*apiclient.FallbackProvider{
		apiclient.CategoryQuick: fp,
		apiclient.CategoryDeep:  fp,
	})
}

// --- Tests ---

func TestNewOrchestrator_BuiltinProfiles(t *testing.T) {
	router := newTestRouter("ok")
	exec := agent.NewStaticExecutor()
	o := NewOrchestrator(router, exec)

	expected := []string{"coordinator", "deep-worker", "planner", "debugger"}
	for _, name := range expected {
		if _, ok := o.registry[name]; !ok {
			t.Errorf("missing built-in profile: %s", name)
		}
	}
}

func TestNewOrchestrator_BuiltinCategories(t *testing.T) {
	router := newTestRouter("ok")
	exec := agent.NewStaticExecutor()
	o := NewOrchestrator(router, exec)

	tests := []struct {
		name     string
		category apiclient.TaskCategory
	}{
		{"coordinator", apiclient.CategoryQuick},
		{"deep-worker", apiclient.CategoryDeep},
		{"planner", apiclient.CategoryQuick},
		{"debugger", apiclient.CategoryDeep},
	}
	for _, tt := range tests {
		def := o.registry[tt.name]
		if def.Category != tt.category {
			t.Errorf("profile %q: got category %q, want %q", tt.name, def.Category, tt.category)
		}
	}
}

func TestDelegate_Success(t *testing.T) {
	router := newTestRouter("deep analysis result")
	exec := agent.NewStaticExecutor()
	o := NewOrchestrator(router, exec)

	output, err := o.Delegate(context.Background(), "deep-worker", "analyze the codebase")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output != "deep analysis result" {
		t.Errorf("got output %q, want %q", output, "deep analysis result")
	}
}

func TestDelegate_UnknownAgent(t *testing.T) {
	router := newTestRouter("ok")
	exec := agent.NewStaticExecutor()
	o := NewOrchestrator(router, exec)

	_, err := o.Delegate(context.Background(), "nonexistent", "task")
	if err == nil {
		t.Fatal("expected error for unknown agent")
	}
}

func TestDelegateBackground_ReturnsResult(t *testing.T) {
	router := newTestRouter("bg result")
	exec := agent.NewStaticExecutor()
	o := NewOrchestrator(router, exec)

	ch := o.DelegateBackground(context.Background(), "coordinator", "do something")
	result := <-ch
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if result.AgentName != "coordinator" {
		t.Errorf("got agent name %q, want %q", result.AgentName, "coordinator")
	}
	if result.Output != "bg result" {
		t.Errorf("got output %q, want %q", result.Output, "bg result")
	}
}

func TestListTools_ReturnsDelegationTools(t *testing.T) {
	router := newTestRouter("ok")
	exec := agent.NewStaticExecutor()
	o := NewOrchestrator(router, exec)

	tools := o.ListTools()
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}

	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Name] = true
	}
	if !names["orchestrator_delegate"] {
		t.Error("missing orchestrator_delegate tool")
	}
	if !names["orchestrator_delegate_bg"] {
		t.Error("missing orchestrator_delegate_bg tool")
	}
}

func TestExecute_Delegate(t *testing.T) {
	router := newTestRouter("delegated output")
	exec := agent.NewStaticExecutor()
	o := NewOrchestrator(router, exec)

	result := o.Execute("orchestrator_delegate", map[string]interface{}{
		"agent_name": "planner",
		"task":       "plan the refactor",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if result.Output != "delegated output" {
		t.Errorf("got %q, want %q", result.Output, "delegated output")
	}
}

func TestExecute_MissingParams(t *testing.T) {
	router := newTestRouter("ok")
	exec := agent.NewStaticExecutor()
	o := NewOrchestrator(router, exec)

	result := o.Execute("orchestrator_delegate", map[string]interface{}{})
	if !result.IsError {
		t.Error("expected error for missing agent_name")
	}

	result = o.Execute("orchestrator_delegate", map[string]interface{}{
		"agent_name": "planner",
	})
	if !result.IsError {
		t.Error("expected error for missing task")
	}
}

func TestExecute_UnknownTool(t *testing.T) {
	router := newTestRouter("ok")
	exec := agent.NewStaticExecutor()
	o := NewOrchestrator(router, exec)

	result := o.Execute("unknown_tool", map[string]interface{}{})
	if !result.IsError {
		t.Error("expected error for unknown tool")
	}
}

func TestRegister_CustomAgent(t *testing.T) {
	router := newTestRouter("custom output")
	exec := agent.NewStaticExecutor()
	o := NewOrchestrator(router, exec)

	o.Register(SubAgentDef{
		Name:         "custom",
		SystemPrompt: "You are a custom agent.",
		Category:     apiclient.CategoryQuick,
	})

	output, err := o.Delegate(context.Background(), "custom", "custom task")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output != "custom output" {
		t.Errorf("got %q, want %q", output, "custom output")
	}
}

func TestFilteredExecutor_RestrictsTools(t *testing.T) {
	inner := agent.NewStaticExecutor()
	inner.Register("allowed_tool", func(input map[string]interface{}) apitypes.ToolResult {
		return apitypes.ToolResult{Output: "ok"}
	})
	inner.Register("blocked_tool", func(input map[string]interface{}) apitypes.ToolResult {
		return apitypes.ToolResult{Output: "should not reach"}
	})

	filtered := &permFilteredExecutor{
		inner:   inner,
		allowed: map[string]bool{"allowed_tool": true},
	}

	// Allowed tool works
	result := filtered.Execute("allowed_tool", nil)
	if result.IsError {
		t.Errorf("allowed tool should succeed: %s", result.Output)
	}

	// Blocked tool is rejected
	result = filtered.Execute("blocked_tool", nil)
	if !result.IsError {
		t.Error("blocked tool should be rejected")
	}

	// ListTools only returns allowed tools
	tools := filtered.ListTools()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name != "allowed_tool" {
		t.Errorf("expected allowed_tool, got %s", tools[0].Name)
	}
}
