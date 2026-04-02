package apiclient

import (
	"os"
	"strings"
)

// ProviderKind identifies an LLM provider backend.
type ProviderKind int

const (
	ProviderAnthropic ProviderKind = iota
	ProviderXai
	ProviderOpenAi
	ProviderGemini
	ProviderOpenRouter
	ProviderTogether
	ProviderGroq
	ProviderMistral
	ProviderAzure
	ProviderDeepSeek
	ProviderCodex
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
	// DeepSeek
	"deepseek":      "deepseek-chat",
	"deepseek-r1":   "deepseek-reasoner",
	"deepseek-code": "deepseek-coder",
	// Mistral
	"mistral":       "mistral-large-latest",
	"mistral-small": "mistral-small-latest",
	"mistral-nemo":  "open-mistral-nemo",
	"codestral":     "codestral-latest",
	"pixtral":       "pixtral-large-latest",
	// Meta Llama (via Ollama / Together / Groq)
	"llama":     "llama3.3:70b",
	"llama-8b":  "llama3.1:8b",
	"llama-70b": "llama3.3:70b",
	"llama-405": "meta-llama/Meta-Llama-3.1-405B-Instruct-Turbo",
	// Qwen (via Ollama / Together)
	"qwen":      "qwen2.5:72b",
	"qwen-coder":"qwen2.5-coder:32b",
	// Cohere
	"command-r":      "command-r-plus",
	"command-r-plus": "command-r-plus",
	// Google via OpenRouter
	"palm": "google/palm-2-chat-bison",
	// Perplexity
	"pplx":       "llama-3.1-sonar-large-128k-online",
	"pplx-small": "llama-3.1-sonar-small-128k-online",
	// Together AI popular
	"together-llama": "meta-llama/Meta-Llama-3.1-70B-Instruct-Turbo",
	"together-qwen":  "Qwen/Qwen2.5-72B-Instruct-Turbo",
	// Groq popular
	"groq-llama":   "llama-3.3-70b-versatile",
	"groq-mixtral": "mixtral-8x7b-32768",
	"groq-gemma":   "gemma2-9b-it",
}

// proxyProviderConfigs maps proxy provider kinds to their OpenAI-compat configs.
var proxyProviderConfigs = map[ProviderKind]struct {
	Name    string
	BaseEnv string
	Default string
	AuthEnv string
}{
	ProviderOpenRouter: {"OpenRouter", "OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1", "OPENROUTER_API_KEY"},
	ProviderTogether:   {"Together AI", "TOGETHER_BASE_URL", "https://api.together.xyz/v1", "TOGETHER_API_KEY"},
	ProviderGroq:       {"Groq", "GROQ_BASE_URL", "https://api.groq.com/openai/v1", "GROQ_API_KEY"},
	ProviderMistral:    {"Mistral", "MISTRAL_BASE_URL", "https://api.mistral.ai/v1", "MISTRAL_API_KEY"},
	ProviderDeepSeek:   {"DeepSeek", "DEEPSEEK_BASE_URL", "https://api.deepseek.com/v1", "DEEPSEEK_API_KEY"},
	ProviderAzure:      {"Azure OpenAI", "AZURE_OPENAI_ENDPOINT", "", "AZURE_OPENAI_API_KEY"},
	ProviderCodex:      {"Codex", "CODEX_BASE_URL", "https://api.openai.com/v1", ""},
}

// ResolveModelAlias maps short names to full model identifiers.
func ResolveModelAlias(alias string) string {
	lower := strings.ToLower(strings.TrimSpace(alias))
	if full, ok := modelAliases[lower]; ok {
		return full
	}
	return strings.TrimSpace(alias)
}

// RecommendModel returns a model alias based on a goal.
func RecommendModel(goal string) string {
	switch goal {
	case "coding":
		return "sonnet"
	case "latency":
		return "haiku"
	case "balanced":
		return "gpt5"
	default:
		return "sonnet"
	}
}

// isCustomBaseURL returns true if OPENAI_BASE_URL points to a non-default endpoint.
func isCustomBaseURL() bool {
	u := os.Getenv("OPENAI_BASE_URL")
	if u == "" {
		return false
	}
	lower := strings.ToLower(u)
	return strings.Contains(lower, "localhost") ||
		strings.Contains(lower, "127.0.0.1") ||
		(!strings.Contains(lower, "api.openai.com") && !strings.Contains(lower, "api.x.ai") &&
			!strings.Contains(lower, "generativelanguage.googleapis.com"))
}

// DetectProviderKind determines the provider from a model name and env vars.
func DetectProviderKind(model string) ProviderKind {
	// If OPENAI_BASE_URL is set to a custom endpoint, always use OpenAI-compat
	if isCustomBaseURL() {
		return ProviderOpenAi
	}

	resolved := strings.ToLower(ResolveModelAlias(model))

	// Proxy provider detection by model prefix or env var
	if strings.HasPrefix(resolved, "deepseek") {
		if envNonEmpty("DEEPSEEK_API_KEY") {
			return ProviderDeepSeek
		}
	}
	if strings.HasPrefix(resolved, "mistral") || strings.HasPrefix(resolved, "codestral") ||
		strings.HasPrefix(resolved, "pixtral") || strings.HasPrefix(resolved, "open-mistral") {
		if envNonEmpty("MISTRAL_API_KEY") {
			return ProviderMistral
		}
	}
	if strings.HasPrefix(resolved, "meta-llama/") || strings.HasPrefix(resolved, "qwen/") ||
		strings.Contains(resolved, "-turbo") {
		if envNonEmpty("TOGETHER_API_KEY") {
			return ProviderTogether
		}
	}
	if strings.Contains(resolved, "versatile") || strings.Contains(resolved, "32768") ||
		strings.HasPrefix(resolved, "groq-") {
		if envNonEmpty("GROQ_API_KEY") {
			return ProviderGroq
		}
	}
	if strings.Contains(resolved, "/") && envNonEmpty("OPENROUTER_API_KEY") {
		return ProviderOpenRouter
	}

	// Native provider detection
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

	// Fallback: check env vars in priority order
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
	if envNonEmpty("OPENROUTER_API_KEY") {
		return ProviderOpenRouter
	}
	if envNonEmpty("TOGETHER_API_KEY") {
		return ProviderTogether
	}
	if envNonEmpty("GROQ_API_KEY") {
		return ProviderGroq
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
	case strings.Contains(canonical, "deepseek"):
		return 65536
	case strings.Contains(canonical, "mistral-large"):
		return 131072
	case strings.Contains(canonical, "llama"):
		return 131072
	case strings.Contains(canonical, "qwen"):
		return 131072
	default:
		return 16384
	}
}

// ContextWindowForModel returns the context window size (input tokens) for a model.
func ContextWindowForModel(model string) int {
	canonical := strings.ToLower(ResolveModelAlias(model))
	switch {
	case strings.Contains(canonical, "gpt-5.4"):
		return 1000000
	case strings.Contains(canonical, "gpt-4o"):
		return 128000
	case strings.Contains(canonical, "opus"):
		return 200000
	case strings.Contains(canonical, "sonnet"):
		return 200000
	case strings.Contains(canonical, "haiku"):
		return 200000
	case strings.Contains(canonical, "gemini-3"), strings.Contains(canonical, "gemini-2"):
		return 1000000
	case strings.Contains(canonical, "grok"):
		return 131072
	case strings.Contains(canonical, "o3"), strings.Contains(canonical, "o4"):
		return 200000
	case strings.Contains(canonical, "deepseek"):
		return 128000
	case strings.Contains(canonical, "mistral-large"):
		return 128000
	case strings.Contains(canonical, "llama"):
		return 128000
	case strings.Contains(canonical, "qwen"):
		return 128000
	default:
		return 128000
	}
}
