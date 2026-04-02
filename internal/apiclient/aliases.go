package apiclient

import "strings"

// ProviderKind identifies an LLM provider backend.
type ProviderKind int

const (
	ProviderAnthropic ProviderKind = iota
	ProviderXai
	ProviderOpenAi
	ProviderGemini
)

// modelAliases maps short names to full model identifiers.
var modelAliases = map[string]string{
	// Anthropic Claude (2026)
	"opus":   "claude-opus-4-6",
	"sonnet": "claude-sonnet-4-6",
	"haiku":  "claude-haiku-4-5-20251213",
	// OpenAI GPT (2026)
	"gpt5":       "gpt-5.4",
	"gpt54":      "gpt-5.4",
	"gpt5-mini":  "gpt-5.4-mini",
	"gpt54-mini": "gpt-5.4-mini",
	"gpt54-nano": "gpt-5.4-nano",
	"gpt4":       "gpt-4o",
	"gpt4o":      "gpt-4o",
	"gpt4-mini":  "gpt-4o-mini",
	"gpt":        "gpt-5.4",
	"o1":         "o1",
	"o1-mini":    "o1-mini",
	"o3":         "o3",
	"o3-mini":    "o3-mini",
	"o4-mini":    "o4-mini",
	"codex":      "codex-mini-latest",
	// Google Gemini (2026)
	"gemini":       "gemini-3.1-pro-preview",
	"gemini-pro":   "gemini-3.1-pro-preview",
	"gemini-flash": "gemini-3-flash",
	"gemini-2.5":   "gemini-2.5-pro",
	// xAI Grok (2026)
	"grok":      "grok-4.20-beta",
	"grok-mini": "grok-3-mini",
	"grok-3":    "grok-3",
	"grok-2":    "grok-2",
}

// ResolveModelAlias maps short names to full model identifiers.
// Returns the input unchanged if it's not a known alias.
func ResolveModelAlias(alias string) string {
	lower := strings.ToLower(strings.TrimSpace(alias))
	if full, ok := modelAliases[lower]; ok {
		return full
	}
	return strings.TrimSpace(alias)
}

// DetectProviderKind determines the provider from a model name.
func DetectProviderKind(model string) ProviderKind {
	resolved := strings.ToLower(ResolveModelAlias(model))

	// Model-prefix-based detection
	if strings.HasPrefix(resolved, "claude") {
		return ProviderAnthropic
	}
	if strings.HasPrefix(resolved, "gpt") || strings.HasPrefix(resolved, "o1") ||
		strings.HasPrefix(resolved, "o3") || strings.HasPrefix(resolved, "o4") ||
		strings.HasPrefix(resolved, "codex") || strings.HasPrefix(resolved, "gpt-5") {
		return ProviderOpenAi
	}
	if strings.HasPrefix(resolved, "gemini") {
		return ProviderGemini
	}
	if strings.HasPrefix(resolved, "grok") {
		return ProviderXai
	}

	// Fallback: check env vars
	if envNonEmpty("ANTHROPIC_API_KEY") || envNonEmpty("ANTHROPIC_AUTH_TOKEN") {
		return ProviderAnthropic
	}
	if envNonEmpty("OPENAI_API_KEY") {
		return ProviderOpenAi
	}
	if envNonEmpty("GEMINI_API_KEY") || envNonEmpty("GOOGLE_API_KEY") {
		return ProviderGemini
	}
	if envNonEmpty("XAI_API_KEY") {
		return ProviderXai
	}
	return ProviderAnthropic
}

// MaxTokensForModel returns the max output tokens for a model.
func MaxTokensForModel(model string) int {
	canonical := strings.ToLower(ResolveModelAlias(model))
	switch {
	case strings.Contains(canonical, "gpt-5.4"):
		return 128000
	case strings.Contains(canonical, "gpt-4o"):
		return 16384
	case strings.Contains(canonical, "opus"):
		return 32000
	case strings.Contains(canonical, "sonnet"):
		return 64000
	case strings.Contains(canonical, "haiku"):
		return 8192
	case strings.Contains(canonical, "gemini-3"):
		return 65536
	case strings.Contains(canonical, "gemini-2"):
		return 65536
	case strings.Contains(canonical, "grok"):
		return 131072
	case strings.Contains(canonical, "o3"), strings.Contains(canonical, "o4"):
		return 100000
	case strings.Contains(canonical, "codex"):
		return 16384
	default:
		return 16384
	}
}
