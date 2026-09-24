package apiclient

import (
	"math"
	"strings"
	"testing"

	"github.com/AlleyBo55/gocode/internal/apitypes"
)

func TestPriceForModelResolvesAliasesAndVendorPrefixes(t *testing.T) {
	cases := []struct {
		model string
		want  ModelPrice
	}{
		{"sonnet", ModelPrice{3, 15}},
		{"claude-sonnet-4-6", ModelPrice{3, 15}},
		{"anthropic/claude-sonnet-4-6", ModelPrice{3, 15}},
		{"anthropic/claude-sonnet-4-6:nitro", ModelPrice{3, 15}},
		{"opus", ModelPrice{5, 25}},
		{"claude-opus-4-1-20250805", ModelPrice{15, 75}}, // the old flagship price, not the new one
		{"haiku", ModelPrice{1, 5}},
		{"claude-3-5-haiku-20241022", ModelPrice{0.80, 4}},
		{"gpt5", ModelPrice{2.50, 15}},
		{"gpt-5.4-2026-03-05", ModelPrice{2.50, 15}},
		{"gpt-5.4-mini", ModelPrice{0.75, 4.50}},
		{"gpt-4o-mini-2024-07-18", ModelPrice{0.15, 0.60}},
		{"openai/gpt-4o", ModelPrice{2.50, 10}},
		{"o3", ModelPrice{2, 8}},
		{"o4-mini", ModelPrice{1.10, 4.40}},
		{"codex", ModelPrice{1.50, 6}},
		{"gemini", ModelPrice{2, 12}},
		{"gemini-flash", ModelPrice{0.50, 3}},
		{"grok", ModelPrice{3, 15}},
		{"grok-mini", ModelPrice{0.30, 0.50}},
		{"deepseek", ModelPrice{0.14, 0.28}},
		{"groq-llama", ModelPrice{0.59, 0.79}},
		{"llama-405", ModelPrice{3.50, 3.50}},
		{"together-qwen", ModelPrice{1.20, 1.20}},
		{"  MISTRAL  ", ModelPrice{2, 6}},
	}
	for _, tc := range cases {
		got, ok := PriceForModel(tc.model)
		if !ok {
			t.Errorf("%q: no price", tc.model)
			continue
		}
		if got != tc.want {
			t.Errorf("%q: got %+v, want %+v", tc.model, got, tc.want)
		}
	}
}

func TestPriceForModelSpecificityAndShortPrefixes(t *testing.T) {
	// A family rule must not shadow its cheaper mini variant.
	mini, _ := PriceForModel("gpt-4o-mini")
	full, _ := PriceForModel("gpt-4o")
	if mini == full {
		t.Error("gpt-4o-mini priced as gpt-4o: rule order is wrong")
	}
	// Two-character reasoning-model prefixes must not fire inside other ids.
	for _, id := range []string{"llama-3.1-sonar-large-128k-online", "custom-o1-finetune"} {
		if p, ok := PriceForModel(id); ok && p == (ModelPrice{15, 60}) {
			t.Errorf("%q was priced as o1", id)
		}
	}
}

func TestPriceForModelLocalAndFreeAreZeroButKnown(t *testing.T) {
	for _, id := range []string{"llama3.3:70b", "qwen2.5-coder:32b", "llama", "meta-llama/llama-3.3-70b-instruct:free"} {
		p, ok := PriceForModel(id)
		if !ok {
			t.Errorf("%q should be known (priced at zero), not unpriced", id)
		}
		if p != (ModelPrice{}) {
			t.Errorf("%q should cost nothing, got %+v", id, p)
		}
	}
}

func TestPriceForModelUnknownIsReportedNotZeroed(t *testing.T) {
	for _, id := range []string{"", "   ", "my-company-finetune", "vendor/never-heard-of-it"} {
		if _, ok := PriceForModel(id); ok {
			t.Errorf("%q should be unpriced", id)
		}
	}
}

func TestModelPriceCostAppliesCacheMultipliers(t *testing.T) {
	p := ModelPrice{Input: 3, Output: 15}
	// 1M plain input + 1M output at list price.
	if got := p.Cost(apitypes.Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000}); math.Abs(got-18) > 1e-9 {
		t.Errorf("plain cost = %v, want 18", got)
	}
	// Cache writes cost 1.25x input, reads 0.1x input.
	if got := p.Cost(apitypes.Usage{CacheCreationInputTokens: 1_000_000}); math.Abs(got-3.75) > 1e-9 {
		t.Errorf("cache write cost = %v, want 3.75", got)
	}
	if got := p.Cost(apitypes.Usage{CacheReadInputTokens: 1_000_000}); math.Abs(got-0.30) > 1e-9 {
		t.Errorf("cache read cost = %v, want 0.30", got)
	}
	if got := (ModelPrice{}).Cost(apitypes.Usage{InputTokens: 5, OutputTokens: 5}); got != 0 {
		t.Errorf("zero price must cost zero, got %v", got)
	}
}

func TestPricingRulesAreLowercaseAndOrderedSpecificFirst(t *testing.T) {
	// If a shorter prefix appears before a longer one it extends, the longer
	// one is dead and its price is never used.
	for i, a := range pricingRules {
		if a.prefix != strings.ToLower(a.prefix) {
			t.Errorf("rule %q must be lowercase; ids are lowercased before matching", a.prefix)
		}
		for _, b := range pricingRules[i+1:] {
			if strings.HasPrefix(b.prefix, a.prefix) && a.price != b.price {
				t.Errorf("rule %q shadows later rule %q with a different price", a.prefix, b.prefix)
			}
		}
	}
}
