package orchestrator

import (
	"context"
	"fmt"
	"sync"
)

// BackgroundManager tracks running background agents and enforces
// a concurrency limit. Each spawned agent gets a derived context
// for cancellation, and results are delivered through a buffered channel.
type BackgroundManager struct {
	mu       sync.Mutex
	running  map[string]context.CancelFunc
	results  chan AgentResult
	maxSlots int
}

// NewBackgroundManager creates a manager with the given concurrency limit.
// The results channel is buffered to maxSlots to prevent goroutine leaks.
func NewBackgroundManager(maxSlots int) *BackgroundManager {
	return &BackgroundManager{
		running:  make(map[string]context.CancelFunc),
		results:  make(chan AgentResult, maxSlots),
		maxSlots: maxSlots,
	}
}

// Spawn launches a background agent in a new goroutine with a derived context.
// Returns an error if the concurrency limit (maxSlots) has been reached or if
// an agent with the same name is already running.
func (bm *BackgroundManager) Spawn(ctx context.Context, name string, fn func(ctx context.Context) AgentResult) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if len(bm.running) >= bm.maxSlots {
		return fmt.Errorf("max background slots reached (%d/%d)", len(bm.running), bm.maxSlots)
	}

	if _, exists := bm.running[name]; exists {
		return fmt.Errorf("background agent %q is already running", name)
	}

	childCtx, cancel := context.WithCancel(ctx)
	bm.running[name] = cancel

	go func() {
		result := fn(childCtx)
		result.AgentName = name

		bm.mu.Lock()
		delete(bm.running, name)
		bm.mu.Unlock()

		bm.results <- result
	}()

	return nil
}

// Results returns the channel for receiving completed agent results.
func (bm *BackgroundManager) Results() <-chan AgentResult {
	return bm.results
}

// CancelAll propagates cancellation to all running background agents.
func (bm *BackgroundManager) CancelAll() {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	for name, cancel := range bm.running {
		cancel()
		delete(bm.running, name)
	}
}
