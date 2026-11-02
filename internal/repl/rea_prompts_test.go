package repl

import (
	"strings"
	"testing"

	"github.com/AlleyBo55/gocode/internal/apitypes"
)

func reaTools() []apitypes.ToolDef {
	return []apitypes.ToolDef{{Name: "mcp_rea_open_binary"}}
}

func TestHasREATools(t *testing.T) {
	if !hasREATools(reaTools()) {
		t.Fatal("mcp_rea_ tool not detected")
	}
	if hasREATools([]apitypes.ToolDef{{Name: "mcp_other_x"}, {Name: "bash"}}) {
		t.Fatal("false positive on non-REA tools")
	}
	if hasREATools(nil) {
		t.Fatal("false positive on empty tools")
	}
}

func TestREAWorkflowForModel(t *testing.T) {
	cases := []struct{ model, wantKey string }{
		{"opus", "opus"}, {"claude-opus-4-6", "opus"}, {"sonnet", "opus"},
		{"anthropic/claude-sonnet-4-6", "opus"}, {"haiku", "opus"},
		{"grok", "grok"}, {"grok-4.20-beta", "grok"}, {"xai/grok-3", "grok"},
		{"gpt5", "sol"}, {"gpt-5.4", "sol"}, {"codex", "sol"},
		{"openai/o3", "sol"}, {"o4-mini", "sol"},
	}
	for _, c := range cases {
		got := reaWorkflowForModel(c.model)
		if got == "" {
			t.Errorf("reaWorkflowForModel(%q) = empty, want %s workflow", c.model, c.wantKey)
			continue
		}
		if got != reaWorkflowPrompts[c.wantKey] {
			t.Errorf("reaWorkflowForModel(%q) returned wrong workflow, want %s", c.model, c.wantKey)
		}
	}
}

func TestREAWorkflowForModelUnknown(t *testing.T) {
	for _, m := range []string{"gemini-3-flash", "qwen2.5-coder", "llama-3.3-70b", "deepseek-chat"} {
		if got := reaWorkflowForModel(m); got != "" {
			t.Errorf("reaWorkflowForModel(%q) = non-empty, want none", m)
		}
	}
}

func TestBuildSystemPromptREAAutoActivation(t *testing.T) {
	// REA tools present + a workflow model -> the REA mode prompt is injected.
	p := BuildSystemPrompt(reaTools(), "gpt5")
	if !strings.Contains(p, "REA MODE") {
		t.Fatal("REA tools + gpt5 did not inject REA MODE prompt")
	}
	if !strings.Contains(p, "Rebuilder") {
		t.Fatal("gpt5 should get the SOL rebuilder workflow")
	}

	// An opus-family model gets the investigator workflow.
	p = BuildSystemPrompt(reaTools(), "claude-opus-4-6")
	if !strings.Contains(p, "Deep Investigator") {
		t.Fatal("opus should get the investigator workflow")
	}

	// A grok model gets the adversarial reviewer workflow.
	p = BuildSystemPrompt(reaTools(), "grok-3")
	if !strings.Contains(p, "Adversarial Reviewer") {
		t.Fatal("grok should get the adversarial reviewer workflow")
	}
}

func TestBuildSystemPromptNoREANoInjection(t *testing.T) {
	plain := []apitypes.ToolDef{{Name: "bash"}, {Name: "fileread"}}
	p := BuildSystemPrompt(plain, "gpt5")
	if strings.Contains(p, "REA MODE") {
		t.Fatal("REA MODE injected without REA tools")
	}
	// A model with no workflow + REA tools -> still no generic injection.
	p = BuildSystemPrompt(reaTools(), "llama-3.3-70b")
	if strings.Contains(p, "REA MODE") {
		t.Fatal("REA MODE injected for a model with no defined workflow")
	}
}
