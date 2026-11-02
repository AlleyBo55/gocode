// Package goal implements a persistent, budget-capped objective that drives
// the agent across turns until it completes, is blocked, or is paused.
//
// A goal is set once with an objective and an optional token budget. On each
// turn the REPL asks the tracker whether the goal is still active; if so the
// objective is re-injected so the agent keeps working toward it without the
// user re-prompting. State is persisted to disk so a goal survives restarts
// and can be paused and resumed.
package goal

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Status is the lifecycle state of a goal.
type Status string

const (
	// StatusNone means no goal is set.
	StatusNone Status = "none"
	// StatusActive means the goal is driving turns.
	StatusActive Status = "active"
	// StatusPaused means the goal is set but suspended.
	StatusPaused Status = "paused"
	// StatusDone means the agent declared the objective complete.
	StatusDone Status = "done"
	// StatusBlocked means the agent declared it cannot proceed.
	StatusBlocked Status = "blocked"
)

// Goal is a single tracked objective.
type Goal struct {
	Objective   string    `json:"objective"`
	Status      Status    `json:"status"`
	TokenBudget int       `json:"token_budget,omitempty"` // 0 = unlimited
	TokensUsed  int       `json:"tokens_used"`
	Note        string    `json:"note,omitempty"` // completion / blocked reason
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Tracker manages the at-most-one active goal and its persistence.
type Tracker struct {
	path string
	goal *Goal
}

// NewTracker loads (or initializes) the tracker stored under dir.
func NewTracker(dir string) (*Tracker, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	t := &Tracker{path: filepath.Join(dir, "goal.json")}
	data, err := os.ReadFile(t.path)
	if errors.Is(err, os.ErrNotExist) {
		return t, nil
	}
	if err != nil {
		return nil, err
	}
	var g Goal
	if err := json.Unmarshal(data, &g); err != nil {
		// Corrupt state should not block the session; start fresh.
		return t, nil
	}
	t.goal = &g
	return t, nil
}

// Set starts a new goal, replacing any existing one.
func (t *Tracker) Set(objective string, budget int) error {
	objective = strings.TrimSpace(objective)
	if objective == "" {
		return errors.New("objective is empty")
	}
	if budget < 0 {
		return errors.New("budget cannot be negative")
	}
	now := time.Now()
	t.goal = &Goal{
		Objective:   objective,
		Status:      StatusActive,
		TokenBudget: budget,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	return t.save()
}

// Get returns the current goal, or nil if none is set.
func (t *Tracker) Get() *Goal { return t.goal }

// Active reports whether a goal is currently driving turns.
func (t *Tracker) Active() bool {
	return t.goal != nil && t.goal.Status == StatusActive
}

// Pause suspends the active goal.
func (t *Tracker) Pause() error {
	if !t.Active() {
		return errors.New("no active goal to pause")
	}
	t.goal.Status = StatusPaused
	t.goal.UpdatedAt = time.Now()
	return t.save()
}

// Resume re-activates a paused goal.
func (t *Tracker) Resume() error {
	if t.goal == nil || t.goal.Status != StatusPaused {
		return errors.New("no paused goal to resume")
	}
	t.goal.Status = StatusActive
	t.goal.UpdatedAt = time.Now()
	return t.save()
}

// Complete marks the goal done with a note.
func (t *Tracker) Complete(note string) error {
	if t.goal == nil {
		return errors.New("no goal set")
	}
	t.goal.Status = StatusDone
	t.goal.Note = strings.TrimSpace(note)
	t.goal.UpdatedAt = time.Now()
	return t.save()
}

// Block marks the goal blocked with a reason.
func (t *Tracker) Block(reason string) error {
	if t.goal == nil {
		return errors.New("no goal set")
	}
	t.goal.Status = StatusBlocked
	t.goal.Note = strings.TrimSpace(reason)
	t.goal.UpdatedAt = time.Now()
	return t.save()
}

// Clear removes the goal entirely.
func (t *Tracker) Clear() error {
	t.goal = nil
	err := os.Remove(t.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// AddUsage records tokens consumed by a turn against the budget. When the
// budget is reached the goal is auto-paused so it stops driving turns.
func (t *Tracker) AddUsage(tokens int) error {
	if t.goal == nil || tokens <= 0 {
		return nil
	}
	t.goal.TokensUsed += tokens
	t.goal.UpdatedAt = time.Now()
	if t.goal.Status == StatusActive && t.goal.TokenBudget > 0 && t.goal.TokensUsed >= t.goal.TokenBudget {
		t.goal.Status = StatusPaused
		t.goal.Note = "token budget reached"
	}
	return t.save()
}

// BudgetExceeded reports whether the goal paused itself by hitting its budget.
func (t *Tracker) BudgetExceeded() bool {
	return t.goal != nil && t.goal.TokenBudget > 0 &&
		t.goal.TokensUsed >= t.goal.TokenBudget && t.goal.Status == StatusPaused
}

// DrivePrompt returns the text re-injected each turn while the goal is active,
// or "" when the goal should not drive the next turn.
func (t *Tracker) DrivePrompt() string {
	if !t.Active() {
		return ""
	}
	var b strings.Builder
	b.WriteString("[GOAL] ")
	b.WriteString(t.goal.Objective)
	b.WriteString("\nContinue working toward this objective. ")
	if t.goal.TokenBudget > 0 {
		remaining := t.goal.TokenBudget - t.goal.TokensUsed
		fmt.Fprintf(&b, "Token budget remaining: %d. ", remaining)
	}
	b.WriteString("When it is fully complete, say so and call out GOAL_DONE with a one-line summary. ")
	b.WriteString("If you are blocked and cannot proceed, say GOAL_BLOCKED and the reason.")
	return b.String()
}

// Render returns a human-readable summary of the current goal.
func (t *Tracker) Render() string {
	if t.goal == nil {
		return "No goal set. Use /goal <objective> to set one."
	}
	g := t.goal
	var b strings.Builder
	fmt.Fprintf(&b, "Objective: %s\n", g.Objective)
	fmt.Fprintf(&b, "Status:    %s\n", g.Status)
	if g.TokenBudget > 0 {
		fmt.Fprintf(&b, "Budget:    %d / %d tokens used\n", g.TokensUsed, g.TokenBudget)
	} else {
		fmt.Fprintf(&b, "Tokens:    %d used (no budget)\n", g.TokensUsed)
	}
	if g.Note != "" {
		fmt.Fprintf(&b, "Note:      %s\n", g.Note)
	}
	fmt.Fprintf(&b, "Updated:   %s\n", g.UpdatedAt.Format("2006-01-02 15:04:05"))
	return b.String()
}

func (t *Tracker) save() error {
	data, err := json.MarshalIndent(t.goal, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(t.path, data, 0o644)
}
