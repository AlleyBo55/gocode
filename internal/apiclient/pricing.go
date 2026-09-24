package apiclient

import (
	"strings"

	"github.com/AlleyBo55/gocode/internal/apitypes"
)

// ModelPrice is a model's list price in USD per one million tokens.
type ModelPrice struct {
	Input  float64
	Output float64
}

// Prompt-cache multipliers on the input price. Only Anthropic reports cache
// tokens in the fields Cost reads, and these are Anthropic's rates: a cache
// write costs 25% more than a plain input token, a cache read 90% less.
const (
	cacheWriteMultiplier = 1.25
	cacheReadMultiplier  = 0.10
)

// Cost estimates the spend for one usage record at this price.
func (p ModelPrice) Cost(u apitypes.Usage) float64 {
	in := float64(u.InputTokens) +
		cacheWriteMultiplier*float64(u.CacheCreationInputTokens) +
		cacheReadMultiplier*float64(u.CacheReadInputTokens)
	return (in*p.Input + float64(u.OutputTokens)*p.Output) / 1_000_000
}

type pricingRule struct {
	prefix string // matched against the lowercased id after the vendor prefix is removed
	price  ModelPrice
}

// pricingRules are standard-tier list prices at the time of writing, checked
// in order with first match winning, so a more specific id must precede the
// family it belongs to ("gpt-4o-mini" before "gpt-4o"). Long-context
// surcharges, batch discounts and off-peak rates are ignored: this is an
// estimate for the session ledger, not an invoice.
//
// When a vendor changes a price, edit the number here. There is deliberately
// no override file yet; add one when a second person asks for it.
var pricingRules = []pricingRule{
	// Anthropic. Opus 4 and 4.1 were $15/$75; 4.5 onward is $5/$25.
	{"claude-opus-4-1", ModelPrice{15, 75}},
	{"claude-opus-4-2", ModelPrice{15, 75}}, // claude-opus-4-20250514
	{"claude-3-opus", ModelPrice{15, 75}},
	{"claude-opus", ModelPrice{5, 25}},
	{"claude-sonnet", ModelPrice{3, 15}},
	{"claude-3-7-sonnet", ModelPrice{3, 15}},
	{"claude-3-5-sonnet", ModelPrice{3, 15}},
	{"claude-haiku", ModelPrice{1, 5}},
	{"claude-3-5-haiku", ModelPrice{0.80, 4}},
	{"claude-3-haiku", ModelPrice{0.25, 1.25}},

	// OpenAI.
	{"gpt-5.4-mini", ModelPrice{0.75, 4.50}},
	{"gpt-5.4-nano", ModelPrice{0.20, 1.25}},
	{"gpt-5.4", ModelPrice{2.50, 15}},
	{"gpt-5.2", ModelPrice{1.75, 14}},
	{"gpt-5-mini", ModelPrice{0.25, 2}},
	{"gpt-5-nano", ModelPrice{0.05, 0.40}},
	{"gpt-5", ModelPrice{1.25, 10}},
	{"gpt-4.1-nano", ModelPrice{0.10, 0.40}},
	{"gpt-4.1-mini", ModelPrice{0.40, 1.60}},
	{"gpt-4.1", ModelPrice{2, 8}},
	{"gpt-4o-mini", ModelPrice{0.15, 0.60}},
	{"gpt-4o", ModelPrice{2.50, 10}},
	{"o4-mini", ModelPrice{1.10, 4.40}},
	{"o3-mini", ModelPrice{1.10, 4.40}},
	{"o3-pro", ModelPrice{20, 80}},
	{"o3", ModelPrice{2, 8}},
	{"o1-mini", ModelPrice{1.10, 4.40}},
	{"o1-pro", ModelPrice{150, 600}},
	{"o1", ModelPrice{15, 60}},
	{"codex-mini", ModelPrice{1.50, 6}},

	// Google. Pro tiers are the <=200k-token rate.
	{"gemini-3.1-pro", ModelPrice{2, 12}},
	{"gemini-3-pro", ModelPrice{2, 12}},
	{"gemini-3-flash", ModelPrice{0.50, 3}},
	{"gemini-2.5-pro", ModelPrice{1.25, 10}},
	{"gemini-2.5-flash-lite", ModelPrice{0.10, 0.40}},
	{"gemini-2.5-flash", ModelPrice{0.30, 2.50}},
	{"gemini-2.0-flash", ModelPrice{0.10, 0.40}},

	// xAI.
	{"grok-4", ModelPrice{3, 15}},
	{"grok-3-mini", ModelPrice{0.30, 0.50}},
	{"grok-3", ModelPrice{3, 15}},
	{"grok-2", ModelPrice{2, 10}},

	// DeepSeek. The legacy chat/reasoner ids routed to V4-Flash.
	{"deepseek-v4-pro", ModelPrice{1.74, 3.48}},
	{"deepseek-v4-flash", ModelPrice{0.14, 0.28}},
	{"deepseek-chat", ModelPrice{0.14, 0.28}},
	{"deepseek-reasoner", ModelPrice{0.14, 0.28}},
	{"deepseek-coder", ModelPrice{0.14, 0.28}},

	// Mistral.
	{"mistral-large", ModelPrice{2, 6}},
	{"mistral-medium", ModelPrice{0.40, 2}},
	{"mistral-small", ModelPrice{0.10, 0.30}},
	{"codestral", ModelPrice{0.30, 0.90}},
	{"pixtral-large", ModelPrice{2, 6}},
	{"open-mistral-nemo", ModelPrice{0.15, 0.15}},
	{"mistral-nemo", ModelPrice{0.15, 0.15}},

	// Groq.
	{"llama-3.3-70b-versatile", ModelPrice{0.59, 0.79}},
	{"llama-3.1-8b-instant", ModelPrice{0.05, 0.08}},
	{"mixtral-8x7b-32768", ModelPrice{0.24, 0.24}},
	{"gemma2-9b-it", ModelPrice{0.20, 0.20}},

	// Together.
	{"meta-llama-3.1-405b", ModelPrice{3.50, 3.50}},
	{"meta-llama-3.1-70b", ModelPrice{0.88, 0.88}},
	{"llama-3.3-70b-instruct-turbo", ModelPrice{0.88, 0.88}},
	{"qwen2.5-72b-instruct-turbo", ModelPrice{1.20, 1.20}},
}

// PriceForModel returns the list price for a model, resolving aliases first.
// ok is false when no rule matches, in which case the caller should report
// the model as unpriced rather than charge zero for it. Two shapes are priced
// at zero with ok true: Ollama-style "name:tag" ids, which are local, and
// OpenRouter ":free" variants.
func PriceForModel(model string) (ModelPrice, bool) {
	id := strings.ToLower(strings.TrimSpace(ResolveModelAlias(model)))
	if id == "" {
		return ModelPrice{}, false
	}

	// OpenRouter and Together ids carry a vendor prefix, and OpenRouter adds
	// routing suffixes after a colon. Strip both before matching so
	// "anthropic/claude-sonnet-4:nitro" prices as "claude-sonnet-4".
	if i := strings.LastIndex(id, "/"); i >= 0 {
		id = id[i+1:]
		if j := strings.Index(id, ":"); j >= 0 {
			if id[j+1:] == "free" {
				return ModelPrice{}, true
			}
			id = id[:j]
		}
	} else if strings.Contains(id, ":") {
		// No vendor prefix and a tag: an Ollama or LM Studio local model.
		return ModelPrice{}, true
	}

	for _, rule := range pricingRules {
		if strings.HasPrefix(id, rule.prefix) {
			return rule.price, true
		}
	}
	return ModelPrice{}, false
}
