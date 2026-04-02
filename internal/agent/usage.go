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
	// Rough pricing per 1M tokens (blended average across providers)
	inputCostPer1M := 3.0   // $3 per 1M input tokens
	outputCostPer1M := 15.0 // $15 per 1M output tokens

	inputCost := float64(u.InputTokens) / 1_000_000 * inputCostPer1M
	outputCost := float64(u.OutputTokens) / 1_000_000 * outputCostPer1M
	totalCost := inputCost + outputCost

	return fmt.Sprintf("Tokens: %d in / %d out (%d total, %d turns) — est. $%.4f",
		u.InputTokens, u.OutputTokens, u.TotalTokens(), u.Turns, totalCost)
}
