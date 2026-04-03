package dream

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAcquireLock(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "test.lock")

	release, err := acquireLock(lockPath)
	if err != nil {
		t.Fatalf("acquireLock: %v", err)
	}

	// Lock file should exist with PID.
	data, _ := os.ReadFile(lockPath)
	if len(data) == 0 {
		t.Error("lock file should contain PID")
	}

	// Second acquire should fail.
	_, err2 := acquireLock(lockPath)
	if err2 == nil {
		t.Error("expected second acquireLock to fail")
	}

	// Release and re-acquire.
	release()
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Error("lock file should be removed after release")
	}

	release2, err3 := acquireLock(lockPath)
	if err3 != nil {
		t.Fatalf("re-acquire after release: %v", err3)
	}
	release2()
}

func TestShouldDream(t *testing.T) {
	d := &Dreamer{
		lastRun: time.Now().Add(-1 * time.Hour), // 1 hour ago > MinInterval
	}
	if !d.ShouldDream() {
		t.Error("should dream after MinInterval has passed")
	}

	d.lastRun = time.Now() // just now
	if d.ShouldDream() {
		t.Error("should not dream immediately after last run")
	}

	d.running = true
	d.lastRun = time.Time{} // long ago
	if d.ShouldDream() {
		t.Error("should not dream while already running")
	}
}

func TestBuildConsolidationPrompt(t *testing.T) {
	prompt := BuildConsolidationPrompt("/mem", "/sess", "")
	if prompt == "" {
		t.Fatal("prompt should not be empty")
	}
	if len(prompt) < 200 {
		t.Error("prompt seems too short")
	}

	// With extra context.
	prompt2 := BuildConsolidationPrompt("/mem", "/sess", "focus on auth module")
	if prompt2 == prompt {
		t.Error("extra context should change the prompt")
	}
}

func TestStartStopStatus(t *testing.T) {
	d := &Dreamer{
		idleDuration: time.Minute,
		stopCh:       make(chan struct{}),
	}

	if s := d.Status(); s != "idle" {
		t.Errorf("expected idle, got %s", s)
	}

	d.Start()
	if s := d.Status(); s != "idle" {
		t.Errorf("expected idle after Start, got %s", s)
	}

	d.Stop()
	if s := d.Status(); s != "stopped" {
		t.Errorf("expected stopped, got %s", s)
	}

	// Double stop should not panic.
	d.Stop()
}

func TestResetIdleTimer(t *testing.T) {
	d := &Dreamer{
		idleDuration: 50 * time.Millisecond,
		stopCh:       make(chan struct{}),
		// Set lastRun to now so ShouldDream returns false — we just want to
		// verify the timer mechanics, not actually run a dream.
		lastRun: time.Now(),
	}

	d.ResetIdleTimer()
	if s := d.Status(); s != "idle-armed" {
		t.Errorf("expected idle-armed, got %s", s)
	}

	// Reset again — should not panic.
	d.ResetIdleTimer()

	// Stop should cancel the timer.
	d.Stop()
	if s := d.Status(); s != "stopped" {
		t.Errorf("expected stopped after Stop, got %s", s)
	}
}

func TestResetIdleTimerNoOpWhenStopped(t *testing.T) {
	d := &Dreamer{
		idleDuration: time.Minute,
		stopCh:       make(chan struct{}),
	}
	d.Stop()
	d.ResetIdleTimer()
	// Timer should not be set.
	d.mu.Lock()
	hasTimer := d.idleTimer != nil
	d.mu.Unlock()
	if hasTimer {
		t.Error("idle timer should not be set after Stop")
	}
}

func TestRunFinalConsolidationSkipsWhenRecent(t *testing.T) {
	d := &Dreamer{
		idleDuration: time.Minute,
		stopCh:       make(chan struct{}),
		lastRun:      time.Now(), // just ran — ShouldDream returns false
	}

	summary, err := d.RunFinalConsolidation(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary != "" {
		t.Errorf("expected empty summary when skipped, got %q", summary)
	}
}

func TestSetIdleDuration(t *testing.T) {
	d := &Dreamer{
		idleDuration: time.Minute,
		stopCh:       make(chan struct{}),
	}
	d.SetIdleDuration(10 * time.Second)
	d.mu.Lock()
	dur := d.idleDuration
	d.mu.Unlock()
	if dur != 10*time.Second {
		t.Errorf("expected 10s, got %v", dur)
	}
}
