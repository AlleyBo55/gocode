package voice

import (
	"errors"
	"sync"
)

// ErrNoMicrophone is returned when the STT engine fails to start (e.g. no mic).
var ErrNoMicrophone = errors.New("no microphone available")

// STTEngine abstracts the speech-to-text backend.
type STTEngine interface {
	// Start begins capturing audio and processing speech-to-text.
	Start() error
	// Stop ends audio capture.
	Stop() error
	// Results returns a channel of transcribed text segments.
	Results() <-chan string
}

// Listener manages microphone capture and speech-to-text.
type Listener struct {
	mu     sync.Mutex
	active bool
	engine STTEngine
}

// NewListener creates a voice listener with the given STT engine.
func NewListener(engine STTEngine) *Listener {
	return &Listener{engine: engine}
}

// Toggle enables or disables voice input. Returns the new active state.
// Returns an error if enabling fails (e.g. no microphone).
func (l *Listener) Toggle() (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.active {
		// Turning off
		if err := l.engine.Stop(); err != nil {
			// Best-effort stop; still mark inactive
			l.active = false
			return false, nil
		}
		l.active = false
		return false, nil
	}

	// Turning on
	if err := l.engine.Start(); err != nil {
		return false, ErrNoMicrophone
	}
	l.active = true
	return true, nil
}

// IsActive returns whether voice input is currently active.
func (l *Listener) IsActive() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.active
}
