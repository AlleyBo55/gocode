package agent

import (
	"fmt"
	"strings"

	"github.com/AlleyBo55/gocode/internal/apiclient"
	"github.com/AlleyBo55/gocode/internal/apitypes"
)

// UsageTracker accumulates token usage across turns, in total and per model.
//
// The aggregate fields keep their original meaning, so /status and the TUI
// status line read them unchanged. PerModel is the ledger behind /cost: the
// same counters split by the model that actually served each turn, which
// under a fallback chain is not always the model the user asked for.
type UsageTracker struct {
	InputTokens              int
	CacheCreationInputTokens int
	CacheReadInputTokens     int
	OutputTokens             int
	Turns                    int

	// PerModel is keyed by served model id. Nil until the first turn.
	PerModel map[string]*ModelUsage
	order    []string // first-seen order, so Render is stable across calls
}

// ModelUsage is one model's share of the session.
type ModelUsage struct {
	Model string
	Usage apitypes.Usage
	Turns int
}

// unknownModel labels turns recorded through Add, which carries no model.
const unknownModel = "(unknown model)"

// Add accumulates usage from a single API response with no model attribution.
// Prefer AddForModel; this remains for callers that only have the totals.
func (u *UsageTracker) Add(usage apitypes.Usage) {
	u.AddForModel("", usage)
}

// AddForModel accumulates usage from one turn served by model.
func (u *UsageTracker) AddForModel(model string, usage apitypes.Usage) {
	u.InputTokens += usage.InputTokens
	u.CacheCreationInputTokens += usage.CacheCreationInputTokens
	u.CacheReadInputTokens += usage.CacheReadInputTokens
	u.OutputTokens += usage.OutputTokens
	u.Turns++

	model = strings.TrimSpace(model)
	if model == "" {
		model = unknownModel
	}
	if u.PerModel == nil {
		u.PerModel = make(map[string]*ModelUsage)
	}
	m, ok := u.PerModel[model]
	if !ok {
		m = &ModelUsage{Model: model}
		u.PerModel[model] = m
		u.order = append(u.order, model)
	}
	m.Usage.InputTokens += usage.InputTokens
	m.Usage.CacheCreationInputTokens += usage.CacheCreationInputTokens
	m.Usage.CacheReadInputTokens += usage.CacheReadInputTokens
	m.Usage.OutputTokens += usage.OutputTokens
	m.Turns++
}

// TotalTokens returns the sum of input and output tokens.
func (u *UsageTracker) TotalTokens() int {
	return u.InputTokens + u.OutputTokens
}

// Models returns the per-model ledger in first-seen order.
func (u UsageTracker) Models() []*ModelUsage {
	out := make([]*ModelUsage, 0, len(u.order))
	for _, name := range u.order {
		out = append(out, u.PerModel[name])
	}
	return out
}

// Cost estimates the session's spend in USD from each model's list price.
// unpriced counts models with no price data; they contribute nothing to usd,
// so a non-zero count means the figure is a floor, not a total.
func (u UsageTracker) Cost() (usd float64, unpriced int) {
	for _, m := range u.Models() {
		price, ok := apiclient.PriceForModel(m.Model)
		if !ok {
			unpriced++
			continue
		}
		usd += price.Cost(m.Usage)
	}
	return usd, unpriced
}

// Render returns a human-readable summary: one line of totals, then one line
// per model with its share of the estimated cost.
func (u UsageTracker) Render() string {
	total, unpriced := u.Cost()

	var b strings.Builder
	fmt.Fprintf(&b, "Tokens: %d in / %d out (%d total, %d turns) — est. $%.4f",
		u.InputTokens, u.OutputTokens, u.TotalTokens(), u.Turns, total)
	switch unpriced {
	case 0:
	case 1:
		b.WriteString(" (1 model unpriced)")
	default:
		fmt.Fprintf(&b, " (%d models unpriced)", unpriced)
	}

	models := u.Models()
	if len(models) == 0 {
		return b.String()
	}

	nameWidth := 0
	for _, m := range models {
		nameWidth = max(nameWidth, len(m.Model))
	}
	for _, m := range models {
		fmt.Fprintf(&b, "\n  %-*s  %8d in / %7d out  %3d turn%s  ",
			nameWidth, m.Model, m.Usage.InputTokens, m.Usage.OutputTokens, m.Turns, plural(m.Turns))
		price, ok := apiclient.PriceForModel(m.Model)
		if !ok {
			b.WriteString("     n/a   (no price data)")
			continue
		}
		cost := price.Cost(m.Usage)
		fmt.Fprintf(&b, "$%.4f", cost)
		if total > 0 {
			fmt.Fprintf(&b, "  %3.0f%%", cost/total*100)
		}
	}
	return b.String()
}

func plural(n int) string {
	if n == 1 {
		return " "
	}
	return "s"
}
