package agent

import (
	"math"
	"strings"
	"testing"

	"github.com/AlleyBo55/gocode/internal/apitypes"
)

func TestUsageTrackerAggregatesAndSplitsPerModel(t *testing.T) {
	var u UsageTracker
	u.AddForModel("claude-sonnet-4-6", apitypes.Usage{InputTokens: 100, OutputTokens: 10, CacheReadInputTokens: 50})
	u.AddForModel("gpt-4o-mini", apitypes.Usage{InputTokens: 20, OutputTokens: 5})
	u.AddForModel("claude-sonnet-4-6", apitypes.Usage{InputTokens: 200, OutputTokens: 20})

	// The aggregate is what /status and the TUI status line already read.
	if u.InputTokens != 320 || u.OutputTokens != 35 || u.CacheReadInputTokens != 50 || u.Turns != 3 || u.TotalTokens() != 355 {
		t.Errorf("aggregate = %+v", u)
	}

	models := u.Models()
	if len(models) != 2 || models[0].Model != "claude-sonnet-4-6" || models[1].Model != "gpt-4o-mini" {
		t.Fatalf("models in first-seen order = %v", names(models))
	}
	if s := models[0]; s.Usage.InputTokens != 300 || s.Usage.OutputTokens != 30 || s.Usage.CacheReadInputTokens != 50 || s.Turns != 2 {
		t.Errorf("sonnet = %+v", *s)
	}
	if g := models[1]; g.Usage.InputTokens != 20 || g.Turns != 1 {
		t.Errorf("mini = %+v", *g)
	}

	// Per-model rows always sum to the aggregate.
	var in, out, turns int
	for _, m := range models {
		in += m.Usage.InputTokens
		out += m.Usage.OutputTokens
		turns += m.Turns
	}
	if in != u.InputTokens || out != u.OutputTokens || turns != u.Turns {
		t.Errorf("ledger (%d/%d/%d) does not sum to aggregate (%d/%d/%d)", in, out, turns, u.InputTokens, u.OutputTokens, u.Turns)
	}
}

func TestUsageTrackerAddWithoutModelIsStillCounted(t *testing.T) {
	var u UsageTracker
	u.Add(apitypes.Usage{InputTokens: 7, OutputTokens: 3})
	u.AddForModel("  ", apitypes.Usage{InputTokens: 1})
	if u.Turns != 2 || u.InputTokens != 8 {
		t.Errorf("aggregate = %+v", u)
	}
	models := u.Models()
	if len(models) != 1 || models[0].Model != unknownModel || models[0].Turns != 2 {
		t.Errorf("unattributed turns should share one row: %v", names(models))
	}
	if _, unpriced := u.Cost(); unpriced != 1 {
		t.Errorf("the unknown row must be reported as unpriced, got %d", unpriced)
	}
}

func TestUsageTrackerCostUsesRealPricesPerModel(t *testing.T) {
	var u UsageTracker
	// 1M in + 1M out on Sonnet ($3/$15) = $18; on gpt-4o-mini ($0.15/$0.60) = $0.75.
	u.AddForModel("claude-sonnet-4-6", apitypes.Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000})
	u.AddForModel("gpt-4o-mini", apitypes.Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000})
	usd, unpriced := u.Cost()
	if unpriced != 0 || math.Abs(usd-18.75) > 1e-9 {
		t.Errorf("cost = %v (%d unpriced), want 18.75", usd, unpriced)
	}

	// Unpriced models are excluded and counted, never silently charged zero.
	u.AddForModel("my-finetune", apitypes.Usage{InputTokens: 1_000_000})
	usd, unpriced = u.Cost()
	if unpriced != 1 || math.Abs(usd-18.75) > 1e-9 {
		t.Errorf("with unpriced model: cost = %v (%d unpriced)", usd, unpriced)
	}

	// Local models are known and free, not unpriced.
	u.AddForModel("llama3.3:70b", apitypes.Usage{InputTokens: 1_000_000})
	if _, unpriced = u.Cost(); unpriced != 1 {
		t.Errorf("local model should not count as unpriced, got %d", unpriced)
	}
}

func TestUsageTrackerRenderShape(t *testing.T) {
	var u UsageTracker
	if got := u.Render(); got != "Tokens: 0 in / 0 out (0 total, 0 turns) — est. $0.0000" {
		t.Errorf("empty render = %q", got)
	}

	u.AddForModel("claude-sonnet-4-6", apitypes.Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000})
	u.AddForModel("gpt-4o-mini", apitypes.Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000})
	u.AddForModel("my-finetune", apitypes.Usage{InputTokens: 10})
	got := u.Render()
	lines := strings.Split(got, "\n")
	if len(lines) != 4 {
		t.Fatalf("want 1 total line + 3 model lines, got %d:\n%s", len(lines), got)
	}
	// The first line keeps the shape the REPL and TUI have always printed.
	if !strings.HasPrefix(lines[0], "Tokens: 2000010 in / 2000000 out (4000010 total, 3 turns) — est. $18.7500") {
		t.Errorf("total line = %q", lines[0])
	}
	if !strings.Contains(lines[0], "(1 model unpriced)") {
		t.Errorf("total line should flag the unpriced model: %q", lines[0])
	}
	// Sonnet is $18 of $18.75 = 96%.
	if !strings.Contains(lines[1], "claude-sonnet-4-6") || !strings.Contains(lines[1], "$18.0000") || !strings.Contains(lines[1], "96%") {
		t.Errorf("sonnet line = %q", lines[1])
	}
	if !strings.Contains(lines[2], "gpt-4o-mini") || !strings.Contains(lines[2], "$0.7500") || !strings.Contains(lines[2], "4%") {
		t.Errorf("mini line = %q", lines[2])
	}
	if !strings.Contains(lines[3], "my-finetune") || !strings.Contains(lines[3], "no price data") || strings.Contains(lines[3], "%") {
		t.Errorf("unpriced line must say so and carry no share: %q", lines[3])
	}
	for _, l := range lines[1:] {
		if !strings.HasPrefix(l, "  ") {
			t.Errorf("model lines are indented under the total: %q", l)
		}
	}
}

func TestUsageTrackerRenderSingleFreeModelHasNoShareColumn(t *testing.T) {
	var u UsageTracker
	u.AddForModel("llama3.3:70b", apitypes.Usage{InputTokens: 10, OutputTokens: 5})
	got := u.Render()
	if !strings.Contains(got, "est. $0.0000") || strings.Contains(got, "unpriced") {
		t.Errorf("free local model: %q", got)
	}
	// With zero total there is no meaningful share, so none is printed.
	if strings.Contains(got, "%") {
		t.Errorf("share printed against a zero total: %q", got)
	}
}

func TestUsageTrackerCopyIsReadable(t *testing.T) {
	// GetUsage returns the tracker by value; the copy must still see the
	// ledger, since /cost is answered from that copy.
	var u UsageTracker
	u.AddForModel("gpt-4o", apitypes.Usage{InputTokens: 1})
	cp := u
	if len(cp.Models()) != 1 || cp.Models()[0].Model != "gpt-4o" {
		t.Errorf("copy lost the ledger: %v", names(cp.Models()))
	}
}

func names(ms []*ModelUsage) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Model
	}
	return out
}
