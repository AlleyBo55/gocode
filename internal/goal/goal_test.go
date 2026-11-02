package goal

import "testing"

func newTempTracker(t *testing.T) *Tracker {
	t.Helper()
	tr, err := NewTracker(t.TempDir())
	if err != nil {
		t.Fatalf("NewTracker: %v", err)
	}
	return tr
}

func TestSetAndActive(t *testing.T) {
	tr := newTempTracker(t)
	if tr.Active() {
		t.Fatal("no goal set but Active() true")
	}
	if err := tr.Set("add tests to auth", 0); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if !tr.Active() {
		t.Fatal("goal set but Active() false")
	}
	if tr.Get().Objective != "add tests to auth" {
		t.Fatalf("objective = %q", tr.Get().Objective)
	}
}

func TestSetEmpty(t *testing.T) {
	tr := newTempTracker(t)
	if err := tr.Set("   ", 0); err == nil {
		t.Fatal("empty objective accepted")
	}
}

func TestPauseResume(t *testing.T) {
	tr := newTempTracker(t)
	_ = tr.Set("task", 0)
	if err := tr.Pause(); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if tr.Active() {
		t.Fatal("paused goal still active")
	}
	if tr.DrivePrompt() != "" {
		t.Fatal("paused goal still drives")
	}
	if err := tr.Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if !tr.Active() {
		t.Fatal("resumed goal not active")
	}
}

func TestBudgetAutoPause(t *testing.T) {
	tr := newTempTracker(t)
	_ = tr.Set("task", 1000)
	_ = tr.AddUsage(400)
	if !tr.Active() {
		t.Fatal("under budget but paused")
	}
	_ = tr.AddUsage(700)
	if tr.Active() {
		t.Fatal("budget exceeded but still active")
	}
	if !tr.BudgetExceeded() {
		t.Fatal("BudgetExceeded() false after exceeding")
	}
}

func TestNoBudgetNeverAutoPauses(t *testing.T) {
	tr := newTempTracker(t)
	_ = tr.Set("task", 0)
	_ = tr.AddUsage(999999)
	if !tr.Active() {
		t.Fatal("no-budget goal auto-paused")
	}
}

func TestCompleteAndBlock(t *testing.T) {
	tr := newTempTracker(t)
	_ = tr.Set("task", 0)
	_ = tr.Complete("shipped")
	if tr.Get().Status != StatusDone {
		t.Fatalf("status = %q", tr.Get().Status)
	}
	_ = tr.Set("task2", 0)
	_ = tr.Block("missing API key")
	if tr.Get().Status != StatusBlocked {
		t.Fatalf("status = %q", tr.Get().Status)
	}
}

func TestClear(t *testing.T) {
	tr := newTempTracker(t)
	_ = tr.Set("task", 0)
	if err := tr.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if tr.Get() != nil {
		t.Fatal("goal still set after Clear")
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	tr, _ := NewTracker(dir)
	_ = tr.Set("persist me", 5000)
	_ = tr.AddUsage(123)

	tr2, err := NewTracker(dir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	g := tr2.Get()
	if g == nil || g.Objective != "persist me" || g.TokenBudget != 5000 || g.TokensUsed != 123 {
		t.Fatalf("reloaded goal = %+v", g)
	}
}

func TestDrivePromptContent(t *testing.T) {
	tr := newTempTracker(t)
	_ = tr.Set("refactor auth", 10000)
	_ = tr.AddUsage(2500)
	p := tr.DrivePrompt()
	if p == "" {
		t.Fatal("active goal produced empty drive prompt")
	}
	for _, want := range []string{"refactor auth", "GOAL_DONE", "GOAL_BLOCKED", "7500"} {
		if !contains(p, want) {
			t.Fatalf("drive prompt missing %q:\n%s", want, p)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
