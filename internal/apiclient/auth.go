package apiclient

import (
	"net/http"
	"os"
	"strings"

	"github.com/AlleyBo55/gocode/internal/apitypes"
)

// ResolveAuthSource resolves credentials from CLI flags, env vars, and provider type.
// apiKeyFlag takes precedence over environment variables.
func ResolveAuthSource(providerKind ProviderKind, apiKeyFlag string) (apitypes.AuthSource, error) {
	// CLI flag takes highest precedence
	if apiKeyFlag != "" {
		return apitypes.AuthApiKey(apiKeyFlag), nil
	}

	switch providerKind {
	case ProviderAnthropic:
		return resolveAnthropicAuth()
	case ProviderXai:
		return resolveEnvAuth("XAI_API_KEY", "xAI", "XAI_API_KEY")
	case ProviderOpenAi:
		return resolveEnvAuth("OPENAI_API_KEY", "OpenAI", "OPENAI_API_KEY")
	case ProviderGemini:
		return resolveGeminiAuth()
	default:
		return resolveAnthropicAuth()
	}
}

// resolveAnthropicAuth resolves Anthropic credentials from env vars.
func resolveAnthropicAuth() (apitypes.AuthSource, error) {
	apiKey := readEnvNonEmpty("ANTHROPIC_API_KEY")
	authToken := readEnvNonEmpty("ANTHROPIC_AUTH_TOKEN")

	switch {
	case apiKey != "" && authToken != "":
		return apitypes.AuthApiKeyAndBearer(apiKey, authToken), nil
	case apiKey != "":
		return apitypes.AuthApiKey(apiKey), nil
	case authToken != "":
		return apitypes.AuthBearer(authToken), nil
	default:
		return apitypes.AuthSource{}, apitypes.NewMissingCredentials("Anthropic", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_API_KEY")
	}
}

// resolveEnvAuth resolves a single API key from an env var.
func resolveEnvAuth(envVar, providerName string, envVars ...string) (apitypes.AuthSource, error) {
	key := readEnvNonEmpty(envVar)
	if key != "" {
		return apitypes.AuthApiKey(key), nil
	}
	return apitypes.AuthSource{}, apitypes.NewMissingCredentials(providerName, envVars...)
}

// ApplyAuth sets authentication headers on an HTTP request based on the AuthSource.
func ApplyAuth(req *http.Request, auth apitypes.AuthSource) {
	if auth.ApiKey != "" {
		req.Header.Set("x-api-key", auth.ApiKey)
	}
	if auth.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+auth.BearerToken)
	}
}

// readEnvNonEmpty reads an env var and returns "" if not set or empty.
func readEnvNonEmpty(key string) string {
	val := os.Getenv(key)
	if strings.TrimSpace(val) == "" {
		return ""
	}
	return val
}

// envNonEmpty returns true if the env var is set and non-empty.
func envNonEmpty(key string) bool {
	return readEnvNonEmpty(key) != ""
}

// resolveGeminiAuth resolves Google Gemini credentials from env vars.
func resolveGeminiAuth() (apitypes.AuthSource, error) {
	// Try GEMINI_API_KEY first, then GOOGLE_API_KEY
	key := readEnvNonEmpty("GEMINI_API_KEY")
	if key != "" {
		return apitypes.AuthApiKey(key), nil
	}
	key = readEnvNonEmpty("GOOGLE_API_KEY")
	if key != "" {
		return apitypes.AuthApiKey(key), nil
	}
	return apitypes.AuthSource{}, apitypes.NewMissingCredentials("Google Gemini", "GEMINI_API_KEY", "GOOGLE_API_KEY")
}
