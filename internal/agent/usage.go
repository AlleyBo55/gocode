package agent

import (
	"fmt"

	"github.com/AlleyBo55/gocode/internal/apitypes"
)

// UsageTracker accumulates token usage across turns.
type UsageTracker struct {
	InputTokens              int
	CacheCreationInputTokens int
	CacheReadInputTokens     int
	OutputTokens             int
	Turns                    int
}

// Add accumulates usage from a single API response.
func (u *UsageTracker) Add(usage apitypes.Usage) {
	u.InputTokens += usage.InputTokens
	u.CacheCreationInputTokens += usage.CacheCreationInputTokens
	u.CacheReadInputTokens += usage.CacheReadInputTokens
	u.OutputTokens += usage.OutputTokens
	u.Turns++
}

// TotalTokens returns the sum of input and output tokens.
func (u *UsageTracker) TotalTokens() int {
	return u.InputTokens + u.OutputTokens
}

// Render returns a human-readable summary.
func (u UsageTracker) Render() string {
	return fmt.Sprintf("Tokens: %d in / %d out (%d total, %d turns, %d cache-create, %d cache-read)",
		u.InputTokens, u.OutputTokens, u.TotalTokens(), u.Turns,
		u.CacheCreationInputTokens, u.CacheReadInputTokens)
}
