package repl

import (
	"fmt"
	"io"
	"sync"
	"time"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

const spinnerColor = "\033[38;5;39m" // blue
const spinnerReset = "\033[0m"
const clearLine = "\r\033[K"

// Spinner shows an animated loading indicator.
// It is restartable — Stop/Start can be called multiple times.
type Spinner struct {
	w       io.Writer
	message string
	mu      sync.Mutex
	running bool
	stopCh  chan struct{}
	doneCh  chan struct{}
}

// NewSpinner creates a spinner that writes to w.
func NewSpinner(w io.Writer, message string) *Spinner {
	return &Spinner{
		w:       w,
		message: message,
	}
}

// Start begins the spinner animation in a goroutine.
// Safe to call multiple times — restarts if already stopped.
func (s *Spinner) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.stopCh = make(chan struct{})
	s.doneCh = make(chan struct{})
	stopCh := s.stopCh
	doneCh := s.doneCh
	s.mu.Unlock()

	go func() {
		defer close(doneCh)
		i := 0
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				fmt.Fprint(s.w, clearLine)
				return
			case <-ticker.C:
				s.mu.Lock()
				frame := spinnerFrames[i%len(spinnerFrames)]
				fmt.Fprintf(s.w, "%s%s%s %s", clearLine, spinnerColor, frame, s.message+spinnerReset)
				s.mu.Unlock()
				i++
			}
		}
	}()
}

// Stop halts the spinner and clears the line.
// Safe to call multiple times or when not running.
func (s *Spinner) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.stopCh)
	doneCh := s.doneCh
	s.mu.Unlock()
	<-doneCh
}

// UpdateMessage changes the spinner text while running.
func (s *Spinner) UpdateMessage(msg string) {
	s.mu.Lock()
	s.message = msg
	s.mu.Unlock()
}
