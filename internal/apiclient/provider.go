package apiclient

import (
	"context"
	"encoding/json"
	"fmt"
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

	// A local model id ("qwen2.5-coder:7b") with no endpoint configured means
	// the user has Ollama or LM Studio running and expects it to be used. Find
	// it rather than failing with a missing-credentials error for a hosted
	// provider they never asked for.
	if LooksLocal(resolvedModel) {
		if srv, ok := DetectLocalServer(); ok {
			return newLocalProvider(srv), resolvedModel, nil
		}
		return nil, resolvedModel, &apitypes.ApiError{
			Kind:     apitypes.ErrMissingCredentials,
			Provider: "local model server",
			EnvVars:  []string{"OLLAMA_HOST", "OPENAI_BASE_URL"},
			Message: fmt.Sprintf("%s looks like a local model, but nothing answered at %s (Ollama) or http://localhost:1234 (LM Studio); "+
				"start the server, or point OLLAMA_HOST or OPENAI_BASE_URL at it", resolvedModel, ollamaHost()),
		}
	}

	kind := DetectProviderKind(resolvedModel)

	// Codex backend: load auth from ~/.codex/auth.json
	if kind == ProviderCodex {
		auth, err := resolveCodexAuth(apiKeyFlag)
		if err != nil {
			return nil, resolvedModel, err
		}
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

// resolveCodexAuth loads Codex auth from the CLI flag, then the Codex CLI's
// cached credentials, then OPENAI_API_KEY.
func resolveCodexAuth(apiKeyFlag string) (apitypes.AuthSource, error) {
	if apiKeyFlag != "" {
		return apitypes.AuthApiKey(apiKeyFlag), nil
	}
	if key := readCodexApiKey(codexAuthPath()); key != "" {
		return apitypes.AuthApiKey(key), nil
	}
	if key := readEnvNonEmpty("OPENAI_API_KEY"); key != "" {
		return apitypes.AuthApiKey(key), nil
	}
	return apitypes.AuthSource{}, apitypes.NewMissingCredentials("Codex", "OPENAI_API_KEY")
}

// codexAuthPath is where the Codex CLI caches credentials. CODEX_HOME
// overrides the default, matching the CLI's own lookup.
func codexAuthPath() string {
	if home := readEnvNonEmpty("CODEX_HOME"); home != "" {
		return filepath.Join(home, "auth.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex", "auth.json")
}

// readCodexApiKey returns the API key from a Codex CLI auth.json, or "".
//
// The file the CLI writes today is {"auth_mode":"apikey","OPENAI_API_KEY":"..."}.
// When the user signed in with ChatGPT instead, the file carries an OAuth
// access token under "tokens". That token is only valid against the ChatGPT
// backend's Responses API, which this provider does not speak, so it is
// deliberately not read: a missing-credentials error is more useful than a 401.
func readCodexApiKey(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var codexAuth struct {
		OpenAIAPIKey string `json:"OPENAI_API_KEY"`
		// Older layouts, kept so an existing file keeps working.
		APIKey string `json:"api_key"`
	}
	if json.Unmarshal(data, &codexAuth) != nil {
		return ""
	}
	if codexAuth.OpenAIAPIKey != "" {
		return codexAuth.OpenAIAPIKey
	}
	return codexAuth.APIKey
}
