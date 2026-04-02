package apiclient

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/AlleyBo55/gocode/internal/apitypes"
)

// Provider is the core abstraction for LLM API communication.
type Provider interface {
	// SendMessage sends a non-streaming request and returns the full response.
	SendMessage(ctx context.Context, req apitypes.MessageRequest) (*apitypes.MessageResponse, error)

	// StreamMessage sends a streaming request and returns a channel of events.
	StreamMessage(ctx context.Context, req apitypes.MessageRequest) (<-chan apitypes.StreamEvent, error)

	// Kind returns the provider type.
	Kind() ProviderKind
}

// ResolveProvider selects a Provider based on model name and available credentials.
// Supports 4 native providers + 7 proxy services via OpenAI-compatible shim.
func ResolveProvider(model string, apiKeyFlag string) (Provider, string, error) {
	resolvedModel := ResolveModelAlias(model)

	// If OPENAI_BASE_URL is set, always use OpenAI-compatible provider with that URL
	// This enables Ollama, LM Studio, and any local/custom endpoint.
	if baseURL := os.Getenv("OPENAI_BASE_URL"); baseURL != "" {
		auth, err := ResolveAuthSource(ProviderOpenAi, apiKeyFlag)
		if err != nil {
			// For local models, auth may not be required
			auth = apitypes.AuthSource{}
		}
		return NewOpenAiCompatProvider(OpenAiCompatConfig{
			ProviderName: "OpenAI-Compatible",
			BaseURLEnv:   "OPENAI_BASE_URL",
			DefaultBase:  "https://api.openai.com/v1",
		}, auth), resolvedModel, nil
	}

	kind := DetectProviderKind(resolvedModel)

	// Codex backend: load auth from ~/.codex/auth.json
	if kind == ProviderCodex {
		auth := resolveCodexAuth(apiKeyFlag)
		return NewOpenAiCompatProvider(OpenAiCompatConfig{
			ProviderName: "Codex",
			BaseURLEnv:   "CODEX_BASE_URL",
			DefaultBase:  "https://api.openai.com/v1",
		}, auth), resolvedModel, nil
	}

	// Proxy providers: OpenRouter, Together, Groq, Mistral, DeepSeek, Azure
	if cfg, ok := proxyProviderConfigs[kind]; ok {
		auth, err := resolveEnvAuth(cfg.AuthEnv, cfg.Name, cfg.AuthEnv)
		if err != nil {
			// Try with CLI flag
			if apiKeyFlag != "" {
				auth = apitypes.AuthApiKey(apiKeyFlag)
			} else {
				return nil, resolvedModel, err
			}
		}
		return NewOpenAiCompatProvider(OpenAiCompatConfig{
			ProviderName: cfg.Name,
			BaseURLEnv:   cfg.BaseEnv,
			DefaultBase:  cfg.Default,
		}, auth), resolvedModel, nil
	}

	// Native providers
	auth, err := ResolveAuthSource(kind, apiKeyFlag)
	if err != nil {
		return nil, resolvedModel, err
	}

	switch kind {
	case ProviderXai:
		return NewOpenAiCompatProvider(OpenAiCompatConfig{
			ProviderName: "xAI",
			BaseURLEnv:   "XAI_BASE_URL",
			DefaultBase:  "https://api.x.ai/v1",
		}, auth), resolvedModel, nil
	case ProviderOpenAi:
		return NewOpenAiCompatProvider(OpenAiCompatConfig{
			ProviderName: "OpenAI",
			BaseURLEnv:   "OPENAI_BASE_URL",
			DefaultBase:  "https://api.openai.com/v1",
		}, auth), resolvedModel, nil
	case ProviderGemini:
		return NewOpenAiCompatProvider(OpenAiCompatConfig{
			ProviderName: "Google Gemini",
			BaseURLEnv:   "GEMINI_BASE_URL",
			DefaultBase:  "https://generativelanguage.googleapis.com/v1beta/openai",
		}, auth), resolvedModel, nil
	default:
		return NewAnthropicProvider(auth), resolvedModel, nil
	}
}

// resolveCodexAuth loads Codex auth from ~/.codex/auth.json or falls back to OPENAI_API_KEY.
func resolveCodexAuth(apiKeyFlag string) apitypes.AuthSource {
	if apiKeyFlag != "" {
		return apitypes.AuthApiKey(apiKeyFlag)
	}
	// Try ~/.codex/auth.json
	home, _ := os.UserHomeDir()
	codexPath := filepath.Join(home, ".codex", "auth.json")
	if data, err := os.ReadFile(codexPath); err == nil {
		var codexAuth struct {
			APIKey string `json:"api_key"`
			Token  string `json:"token"`
		}
		if json.Unmarshal(data, &codexAuth) == nil {
			if codexAuth.APIKey != "" {
				return apitypes.AuthApiKey(codexAuth.APIKey)
			}
			if codexAuth.Token != "" {
				return apitypes.AuthBearer(codexAuth.Token)
			}
		}
	}
	// Fall back to OPENAI_API_KEY
	if key := readEnvNonEmpty("OPENAI_API_KEY"); key != "" {
		return apitypes.AuthApiKey(key)
	}
	return apitypes.AuthSource{}
}
