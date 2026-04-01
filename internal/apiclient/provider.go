package apiclient

import (
	"context"

	"github.com/AlleyBo55/gocode/internal/apitypes"
)

// Provider is the core abstraction for LLM API communication.
type Provider interface {
	// SendMessage sends a non-streaming request and returns the full response.
	SendMessage(ctx context.Context, req apitypes.MessageRequest) (*apitypes.MessageResponse, error)

	// StreamMessage sends a streaming request and returns a channel of events.
	// The channel is closed when the stream completes or an error occurs.
	StreamMessage(ctx context.Context, req apitypes.MessageRequest) (<-chan apitypes.StreamEvent, error)

	// Kind returns the provider type.
	Kind() ProviderKind
}

// ResolveProvider selects a Provider based on model name and available credentials.
// Returns the provider, the resolved model name, and any error.
func ResolveProvider(model string, apiKeyFlag string) (Provider, string, error) {
	resolvedModel := ResolveModelAlias(model)
	kind := DetectProviderKind(resolvedModel)

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
