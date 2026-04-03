package ultraplan

import (
	"context"
	"testing"
	"time"

	"github.com/AlleyBo55/gocode/internal/apiclient"
	"github.com/AlleyBo55/gocode/internal/apitypes"
)

func TestHasKeyword(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"I need an ultraplan for this refactor", true},
		{"let's do ULTRAPLAN", true},
		{"run UltraPlan on the auth module", true},
		{"ultraplan", true},
		{"/ultraplan", false},       // slash command — handled by REPL
		{"  /ultraplan foo", false},  // slash command with leading space
		{"no trigger here", false},
		{"ultraplanning is not a word we match", false}, // \b boundary
		{"", false},
	}
	for _, tt := range tests {
		got := HasKeyword(tt.input)
		if got != tt.want {
			t.Errorf("HasKeyword(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestExtractText(t *testing.T) {
	// nil response
	if got := extractText(nil); got != "" {
		t.Errorf("extractText(nil) = %q, want empty", got)
	}
}

func TestExtractText_WithContent(t *testing.T) {
	t.Run("single text block", func(t *testing.T) {
		resp := &apitypes.MessageResponse{
			Content: []apitypes.OutputContentBlock{
				{Kind: "text", Text: "hello world"},
			},
		}
		got := extractText(resp)
		if got != "hello world" {
			t.Errorf("extractText = %q, want %q", got, "hello world")
		}
	})

	t.Run("multiple text blocks joined with newline", func(t *testing.T) {
		resp := &apitypes.MessageResponse{
			Content: []apitypes.OutputContentBlock{
				{Kind: "text", Text: "line one"},
				{Kind: "text", Text: "line two"},
			},
		}
		got := extractText(resp)
		want := "line one\nline two"
		if got != want {
			t.Errorf("extractText = %q, want %q", got, want)
		}
	})

	t.Run("mixed content skips non-text blocks", func(t *testing.T) {
		resp := &apitypes.MessageResponse{
			Content: []apitypes.OutputContentBlock{
				{Kind: "text", Text: "before"},
				{Kind: "tool_use", Name: "bash", Text: ""},
				{Kind: "text", Text: "after"},
			},
		}
		got := extractText(resp)
		want := "before\nafter"
		if got != want {
			t.Errorf("extractText = %q, want %q", got, want)
		}
	})

	t.Run("empty text blocks are skipped", func(t *testing.T) {
		resp := &apitypes.MessageResponse{
			Content: []apitypes.OutputContentBlock{
				{Kind: "text", Text: ""},
				{Kind: "text", Text: "only this"},
				{Kind: "text", Text: ""},
			},
		}
		got := extractText(resp)
		if got != "only this" {
			t.Errorf("extractText = %q, want %q", got, "only this")
		}
	})

	t.Run("empty content slice returns empty string", func(t *testing.T) {
		resp := &apitypes.MessageResponse{
			Content: []apitypes.OutputContentBlock{},
		}
		got := extractText(resp)
		if got != "" {
			t.Errorf("extractText = %q, want empty", got)
		}
	})
}

func TestConstants(t *testing.T) {
	if DefaultTimeout != 30*time.Minute {
		t.Errorf("DefaultTimeout = %v, want %v", DefaultTimeout, 30*time.Minute)
	}
	if MaxTokens != 32768 {
		t.Errorf("MaxTokens = %d, want 32768", MaxTokens)
	}
	if MaxIterations != 60 {
		t.Errorf("MaxIterations = %d, want 60", MaxIterations)
	}
}

func TestNewPlanner(t *testing.T) {
	p := NewPlanner(nil, nil)
	if p == nil {
		t.Fatal("NewPlanner(nil, nil) returned nil")
	}
}

func TestPlanBackground_ChannelSemantics(t *testing.T) {
	// Empty router (no routes configured) — Route returns an error
	// instead of panicking on a nil receiver.
	emptyRouter := apiclient.NewModelRouter(map[apiclient.TaskCategory]*apiclient.FallbackProvider{})
	p := NewPlanner(emptyRouter, nil)
	ch := p.PlanBackground(context.Background(), "test task")

	// Should receive exactly one result.
	result, ok := <-ch
	if !ok {
		t.Fatal("channel closed before sending a result")
	}

	// Result should have a non-nil error (no ultrabrain route configured).
	if result.Err == nil {
		t.Fatal("expected non-nil Err from empty-router Planner")
	}

	// Channel should be closed after the single result.
	_, ok = <-ch
	if ok {
		t.Fatal("expected channel to be closed after single result")
	}
}

func TestResult_ZeroValue(t *testing.T) {
	var r Result
	if r.Output != "" {
		t.Errorf("zero Result.Output = %q, want empty", r.Output)
	}
	if r.Err != nil {
		t.Errorf("zero Result.Err = %v, want nil", r.Err)
	}
}
