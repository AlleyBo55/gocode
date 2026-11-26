package cron

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// A day that never comes (Feb 30, Apr 31) has no next run. Scheduling it must
// fail instead of firing over and over.
func TestCreateRejectsExpressionThatNeverMatches(t *testing.T) {
	for _, expr := range []string{"0 0 30 2 *", "0 9 31 4 *"} {
		var fired atomic.Int32
		s := NewScheduler(func(*Task) { fired.Add(1) })
		id, err := s.Create(expr, "never", true)
		time.Sleep(300 * time.Millisecond)
		s.StopAll()
		if err == nil {
			t.Errorf("Create(%q) = %s, nil; want an error (fired %d times in 300ms)", expr, id, fired.Load())
		}
		if n := fired.Load(); n != 0 {
			t.Errorf("Create(%q): fired %d times in 300ms, want 0", expr, n)
		}
	}
}

// A never-matching entry already saved in cron.json must not fire at startup.
func TestLoadSchedulesSkipsExpressionThatNeverMatches(t *testing.T) {
	dir := t.TempDir()
	data := `[{"id":"cron-1","expression":"0 0 30 2 *","description":"never","recurring":true}]`
	if err := os.WriteFile(filepath.Join(dir, "cron.json"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	var fired atomic.Int32
	s := NewScheduler(func(*Task) { fired.Add(1) })
	n, err := s.LoadSchedules(dir)
	time.Sleep(300 * time.Millisecond)
	s.StopAll()
	if err != nil || n != 0 {
		t.Errorf("LoadSchedules = %d, %v; want 0, nil", n, err)
	}
	if got := fired.Load(); got != 0 {
		t.Errorf("fired %d times in 300ms, want 0", got)
	}
}
