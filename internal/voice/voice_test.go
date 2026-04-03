package voice

import (
	"errors"
	"testing"
)

// mockSTTEngine is a test double for STTEngine.
type mockSTTEngine struct {
	startErr error
	stopErr  error
	started  bool
	stopped  bool
	ch       chan string
}

func newMockEngine() *mockSTTEngine {
	return &mockSTTEngine{ch: make(chan string, 10)}
}

func (m *mockSTTEngine) Start() error {
	if m.startErr != nil {
		return m.startErr
	}
	m.started = true
	m.stopped = false
	return nil
}

func (m *mockSTTEngine) Stop() error {
	if m.stopErr != nil {
		return m.stopErr
	}
	m.stopped = true
	m.started = false
	return nil
}

func (m *mockSTTEngine) Results() <-chan string {
	return m.ch
}

func TestNewListener(t *testing.T) {
	engine := newMockEngine()
	l := NewListener(engine)
	if l == nil {
		t.Fatal("expected non-nil listener")
	}
	if l.IsActive() {
		t.Error("new listener should not be active")
	}
}

func TestToggleOn(t *testing.T) {
	engine := newMockEngine()
	l := NewListener(engine)

	active, err := l.Toggle()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !active {
		t.Error("expected active=true after toggle on")
	}
	if !l.IsActive() {
		t.Error("IsActive should return true after toggle on")
	}
	if !engine.started {
		t.Error("engine.Start should have been called")
	}
}

func TestToggleOff(t *testing.T) {
	engine := newMockEngine()
	l := NewListener(engine)

	// Toggle on first
	l.Toggle()

	// Toggle off
	active, err := l.Toggle()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if active {
		t.Error("expected active=false after toggle off")
	}
	if l.IsActive() {
		t.Error("IsActive should return false after toggle off")
	}
	if !engine.stopped {
		t.Error("engine.Stop should have been called")
	}
}

func TestToggleInvolution(t *testing.T) {
	// Double toggle returns to original state
	engine := newMockEngine()
	l := NewListener(engine)

	before := l.IsActive()

	l.Toggle()
	l.Toggle()

	after := l.IsActive()
	if before != after {
		t.Errorf("double toggle should return to original state: before=%v, after=%v", before, after)
	}
}

func TestToggleOnFailsNoMicrophone(t *testing.T) {
	engine := newMockEngine()
	engine.startErr = errors.New("device not found")
	l := NewListener(engine)

	active, err := l.Toggle()
	if err == nil {
		t.Fatal("expected error when engine.Start fails")
	}
	if !errors.Is(err, ErrNoMicrophone) {
		t.Errorf("expected ErrNoMicrophone, got: %v", err)
	}
	if active {
		t.Error("expected active=false when toggle fails")
	}
	if l.IsActive() {
		t.Error("IsActive should be false when toggle fails")
	}
}

func TestIsActiveReflectsState(t *testing.T) {
	engine := newMockEngine()
	l := NewListener(engine)

	// Initially inactive
	if l.IsActive() {
		t.Error("should start inactive")
	}

	// After toggle on
	l.Toggle()
	if !l.IsActive() {
		t.Error("should be active after toggle on")
	}

	// After toggle off
	l.Toggle()
	if l.IsActive() {
		t.Error("should be inactive after toggle off")
	}
}

func TestResults(t *testing.T) {
	engine := newMockEngine()
	l := NewListener(engine)

	ch := l.engine.Results()
	if ch == nil {
		t.Fatal("Results channel should not be nil")
	}

	// Send a value through the engine channel
	engine.ch <- "hello world"
	got := <-ch
	if got != "hello world" {
		t.Errorf("expected 'hello world', got %q", got)
	}
}
