package repl

import (
	"bytes"
	"strings"
	"testing"

	"github.com/AlleyBo55/gocode/internal/goal"
)

// newGoalREPL returns a REPL with only the pieces handleGoalCommand needs.
func newGoalREPL(t *testing.T) (*REPL, *bytes.Buffer) {
	t.Helper()
	buf := &bytes.Buffer{}
	tr, err := goal.NewTracker(t.TempDir())
	if err != nil {
		t.Fatalf("NewTracker: %v", err)
	}
	r := &REPL{writer: buf, goalTracker: tr}
	return r, buf
}

func TestGoalCommandSetAndStatus(t *testing.T) {
	r, buf := newGoalREPL(t)
	r.handleGoalCommand("/goal add tests to auth --budget 50000")
	if !strings.Contains(buf.String(), "budget 50000") {
		t.Fatalf("set output: %q", buf.String())
	}
	if r.goalTracker.Get() == nil || r.goalTracker.Get().TokenBudget != 50000 {
		t.Fatalf("goal not stored: %+v", r.goalTracker.Get())
	}

	buf.Reset()
	r.handleGoalCommand("/goal status")
	out := buf.String()
	for _, want := range []string{"add tests to auth", "active", "50000"} {
		if !strings.Contains(out, want) {
			t.Fatalf("status missing %q:\n%s", want, out)
		}
	}
}

func TestGoalCommandPauseResumeClear(t *testing.T) {
	r, buf := newGoalREPL(t)
	r.handleGoalCommand("/goal do the thing")

	r.handleGoalCommand("/goal pause")
	if r.goalTracker.Active() {
		t.Fatal("still active after pause")
	}
	r.handleGoalCommand("/goal resume")
	if !r.goalTracker.Active() {
		t.Fatal("not active after resume")
	}
	r.handleGoalCommand("/goal clear")
	if r.goalTracker.Get() != nil {
		t.Fatal("goal not cleared")
	}
	_ = buf
}

func TestGoalCommandEmptyShowsStatus(t *testing.T) {
	r, buf := newGoalREPL(t)
	r.handleGoalCommand("/goal")
	if !strings.Contains(buf.String(), "No goal set") {
		t.Fatalf("empty /goal output: %q", buf.String())
	}
}
