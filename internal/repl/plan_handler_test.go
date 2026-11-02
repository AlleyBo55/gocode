package repl

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/AlleyBo55/gocode/internal/agent"
)

// TestPlanCommandTogglesReadOnlyMode drives /plan through the real Run loop
// (no provider needed: commands never reach the model) and checks the runtime
// gate flips on and back off.
func TestPlanCommandTogglesReadOnlyMode(t *testing.T) {
	rt := agent.NewConversationRuntime(agent.RuntimeOptions{Model: "test"})
	out := &bytes.Buffer{}
	r := NewREPL(rt, strings.NewReader("/plan\n/plan\n/exit\n"), out, REPLConfig{}, nil)
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Plan mode ON") || !strings.Contains(got, "Plan mode OFF") {
		t.Fatalf("missing toggle messages in output:\n%s", got)
	}
	if rt.PlanMode() {
		t.Fatal("plan mode still on after toggling twice")
	}
}
