// Package dream implements autonomous memory consolidation.
//
// Ported from Claude Code's src/services/autoDream/ — the "dreaming" system
// that runs as a background agent to consolidate memories. The cycle is:
//   1. Orient  — read existing memory files, understand current state
//   2. Gather  — find new signals from session transcripts and daily logs
//   3. Consolidate — update durable memory files with new knowledge
//   4. Prune   — keep the memory index compact and contradiction-free
//
// Improvement over Claude Code: we use the ModelRouter's "deep" category
// so the dream agent gets a strong model with automatic fallback, and we
// add a file-based lock to prevent concurrent dreams.
package dream

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/AlleyBo55/gocode/internal/agent"
	"github.com/AlleyBo55/gocode/internal/apiclient"
)

// DefaultTimeout is the max duration for a dream cycle.
const DefaultTimeout = 10 * time.Minute

// MinInterval is the minimum time between dream cycles.
// Prevents thrashing if sessions are short.
const MinInterval = 30 * time.Minute

// DefaultIdleDuration is the default idle time before triggering a dream.
const DefaultIdleDuration = 5 * time.Minute

// Dreamer manages the autonomous memory consolidation lifecycle.
type Dreamer struct {
	mu           sync.Mutex
	router       *apiclient.ModelRouter
	executor     agent.ToolExecutor
	memDir       string // .gocode/memory/
	sessDir      string // .port_sessions/
	lockPath     string // .gocode/dream.lock
	lastRun      time.Time
	running      bool
	idleTimer    *time.Timer
	idleDuration time.Duration
	stopCh       chan struct{}
	stopped      bool
}

// NewDreamer creates a dreamer wired to the model router.
func NewDreamer(router *apiclient.ModelRouter, executor agent.ToolExecutor) *Dreamer {
	return &Dreamer{
		router:       router,
		executor:     executor,
		memDir:       filepath.Join(".gocode", "memory"),
		sessDir:      ".port_sessions",
		lockPath:     filepath.Join(".gocode", "dream.lock"),
		idleDuration: DefaultIdleDuration,
		stopCh:       make(chan struct{}),
	}
}

// SetIdleDuration configures the idle duration before a dream is triggered.
func (d *Dreamer) SetIdleDuration(dur time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.idleDuration = dur
}

// Start initializes the dreamer lifecycle. It does not immediately trigger a
// dream — call ResetIdleTimer after each user interaction to arm the timer.
func (d *Dreamer) Start() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stopped = false
	d.stopCh = make(chan struct{})
}

// Stop tears down the dreamer, cancelling any pending idle timer.
func (d *Dreamer) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopped {
		return
	}
	d.stopped = true
	close(d.stopCh)
	if d.idleTimer != nil {
		d.idleTimer.Stop()
		d.idleTimer = nil
	}
}

// Status returns a human-readable status string.
func (d *Dreamer) Status() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.running {
		return "running"
	}
	if d.stopped {
		return "stopped"
	}
	if d.idleTimer != nil {
		return "idle-armed"
	}
	return "idle"
}

// ResetIdleTimer resets (or starts) the idle timer. When the timer fires,
// a consolidation cycle runs as a background goroutine.
func (d *Dreamer) ResetIdleTimer() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopped {
		return
	}
	if d.idleTimer != nil {
		d.idleTimer.Stop()
	}
	d.idleTimer = time.AfterFunc(d.idleDuration, func() {
		if !d.ShouldDream() {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
		defer cancel()
		// Fire and forget — errors are logged internally.
		d.Dream(ctx) //nolint:errcheck
	})
}

// RunFinalConsolidation runs one synchronous consolidation cycle.
// Intended to be called on session end before shutdown.
func (d *Dreamer) RunFinalConsolidation(ctx context.Context) (string, error) {
	// Stop the idle timer so it doesn't fire concurrently.
	d.mu.Lock()
	if d.idleTimer != nil {
		d.idleTimer.Stop()
		d.idleTimer = nil
	}
	d.mu.Unlock()

	if !d.ShouldDream() {
		return "", nil
	}
	return d.Dream(ctx)
}

// ShouldDream returns true if enough time has passed since the last dream.
func (d *Dreamer) ShouldDream() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.running {
		return false
	}
	return time.Since(d.lastRun) >= MinInterval
}

// Dream runs a single consolidation cycle. It acquires a file lock to
// prevent concurrent dreams (e.g., multiple gocode instances).
// Returns the dream summary or an error.
func (d *Dreamer) Dream(ctx context.Context) (string, error) {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return "", fmt.Errorf("dream already in progress")
	}
	d.running = true
	d.mu.Unlock()

	defer func() {
		d.mu.Lock()
		d.running = false
		d.lastRun = time.Now()
		d.mu.Unlock()
	}()

	// Acquire file lock.
	release, err := acquireLock(d.lockPath)
	if err != nil {
		return "", fmt.Errorf("dream lock: %w", err)
	}
	defer release()

	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	// Route to a strong model for consolidation.
	provider, err := d.router.Route(apiclient.CategoryDeep)
	if err != nil {
		return "", fmt.Errorf("dream: no deep provider: %w", err)
	}

	prompt := BuildConsolidationPrompt(d.memDir, d.sessDir, "")

	rt := agent.NewConversationRuntime(agent.RuntimeOptions{
		Provider:      provider,
		Executor:      d.executor,
		Model:         "",
		MaxTokens:     8192,
		MaxIterations: 20,
		SystemPrompt:  consolidationSystemPrompt,
		PermMode:      agent.DangerFullAccess,
	})

	resp, err := rt.SendUserMessage(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("dream failed: %w", err)
	}

	var parts []string
	for _, block := range resp.Content {
		if block.Kind == "text" && block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n"), nil
}

// DreamBackground runs the dream cycle in a background goroutine.
func (d *Dreamer) DreamBackground(ctx context.Context) <-chan DreamResult {
	ch := make(chan DreamResult, 1)
	go func() {
		defer close(ch)
		summary, err := d.Dream(ctx)
		ch <- DreamResult{Summary: summary, Err: err}
	}()
	return ch
}

// DreamResult is the outcome of a background dream.
type DreamResult struct {
	Summary string
	Err     error
}

// consolidationSystemPrompt tells the dream agent what it is.
const consolidationSystemPrompt = `You are a memory consolidation agent. Your job is to organize and maintain the user's memory files so future sessions can orient quickly. You have full tool access to read files, search the codebase, and write memory files. Be thorough but concise.`

// BuildConsolidationPrompt constructs the dream prompt.
// Ported from Claude Code's consolidationPrompt.ts.
func BuildConsolidationPrompt(memDir, sessDir, extra string) string {
	var sb strings.Builder
	sb.WriteString(`# Dream: Memory Consolidation

You are performing a dream — a reflective pass over your memory files. Synthesize what you've learned recently into durable, well-organized memories so that future sessions can orient quickly.

Memory directory: ` + "`" + memDir + "`" + `
Session transcripts: ` + "`" + sessDir + "`" + ` (JSON files — grep narrowly, don't read whole files)

---

## Phase 1 — Orient

- ls the memory directory to see what already exists
- Read the index file (if any) to understand the current state
- Skim existing topic files so you improve them rather than creating duplicates

## Phase 2 — Gather recent signal

Look for new information worth persisting:
1. Recent session transcripts — grep for key decisions, corrections, and discoveries
2. Existing memories that drifted — facts that contradict what you see in the codebase now
3. Don't exhaustively read transcripts. Look only for things you suspect matter.

## Phase 3 — Consolidate

For each thing worth remembering, write or update a memory file:
- Merge new signal into existing topic files rather than creating near-duplicates
- Convert relative dates ("yesterday") to absolute dates
- Delete contradicted facts — if new evidence disproves an old memory, fix it

## Phase 4 — Prune and index

Update the index so it stays compact:
- Remove pointers to stale or superseded memories
- Add pointers to newly important memories
- Resolve contradictions between files

Return a brief summary of what you consolidated, updated, or pruned.`)

	if extra != "" {
		sb.WriteString("\n\n## Additional context\n\n")
		sb.WriteString(extra)
	}
	return sb.String()
}

// acquireLock creates a lock file with O_CREATE|O_EXCL.
// Returns a release function that removes the lock.
func acquireLock(path string) (func(), error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("lock already held or cannot create: %w", err)
	}
	// Write PID for debugging.
	fmt.Fprintf(f, "%d\n", os.Getpid())
	f.Close()
	return func() { os.Remove(path) }, nil
}
