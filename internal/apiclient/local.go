package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/AlleyBo55/gocode/internal/apitypes"
)

// LocalServer is an OpenAI-compatible inference server on this machine.
type LocalServer struct {
	Name    string   // "Ollama" or "LM Studio"
	BaseURL string   // OpenAI-compatible base, e.g. http://localhost:11434/v1
	Models  []string // installed model ids, sorted
}

// localProbeTimeout bounds the whole detection. Two loopback connections
// either answer in a few milliseconds or refuse immediately; the timeout only
// matters when a firewall drops packets instead of refusing them.
const localProbeTimeout = 400 * time.Millisecond

// DetectLocalServer looks for a running Ollama or LM Studio and returns the
// first one found. Ollama's own OLLAMA_HOST convention is honoured, so a
// non-default port or a server on another machine works the same way it does
// for the ollama CLI.
func DetectLocalServer() (LocalServer, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), localProbeTimeout)
	defer cancel()

	if srv, ok := detectOllama(ctx, ollamaHost()); ok {
		return srv, true
	}
	if srv, ok := detectLMStudio(ctx, lmStudioBase); ok {
		return srv, true
	}
	return LocalServer{}, false
}

// lmStudioBase is LM Studio's default server address. It has no environment
// convention of its own; a variable so tests can point it at a closed port.
var lmStudioBase = "http://localhost:1234"

// ollamaHost returns Ollama's base URL from OLLAMA_HOST, accepting the forms
// the ollama CLI accepts ("127.0.0.1:11434", "http://host:port"), with the
// same default.
func ollamaHost() string {
	h := strings.TrimSpace(os.Getenv("OLLAMA_HOST"))
	if h == "" {
		return "http://localhost:11434"
	}
	if !strings.Contains(h, "://") {
		h = "http://" + h
	}
	return strings.TrimRight(h, "/")
}

func detectOllama(ctx context.Context, base string) (LocalServer, bool) {
	var tags struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if !getJSON(ctx, base+"/api/tags", &tags) {
		return LocalServer{}, false
	}
	srv := LocalServer{Name: "Ollama", BaseURL: base + "/v1"}
	for _, m := range tags.Models {
		if m.Name != "" {
			srv.Models = append(srv.Models, m.Name)
		}
	}
	sort.Strings(srv.Models)
	return srv, true
}

func detectLMStudio(ctx context.Context, base string) (LocalServer, bool) {
	var list struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if !getJSON(ctx, base+"/v1/models", &list) {
		return LocalServer{}, false
	}
	srv := LocalServer{Name: "LM Studio", BaseURL: base + "/v1"}
	for _, m := range list.Data {
		if m.ID != "" {
			srv.Models = append(srv.Models, m.ID)
		}
	}
	sort.Strings(srv.Models)
	return srv, true
}

func getJSON(ctx context.Context, url string, into any) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	return json.NewDecoder(resp.Body).Decode(into) == nil
}

// LooksLocal reports whether a model id names a locally served model: an
// Ollama-style "name:tag" with no vendor prefix. OpenRouter ids also use a
// colon for routing suffixes, but always carry a "vendor/" prefix.
func LooksLocal(model string) bool {
	id := ResolveModelAlias(model)
	return strings.Contains(id, ":") && !strings.Contains(id, "/")
}

// HasAnyProviderCredentials reports whether any hosted provider is configured,
// by key or by a custom OpenAI-compatible endpoint.
func HasAnyProviderCredentials() bool {
	for _, env := range []string{
		"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "OPENAI_API_KEY", "OPENAI_BASE_URL",
		"GEMINI_API_KEY", "GOOGLE_API_KEY", "XAI_API_KEY", "OPENROUTER_API_KEY",
		"TOGETHER_API_KEY", "GROQ_API_KEY", "MISTRAL_API_KEY", "DEEPSEEK_API_KEY",
		"AZURE_OPENAI_API_KEY",
	} {
		if envNonEmpty(env) {
			return true
		}
	}
	return false
}

// SuggestLocalModel picks the model to default to from a local server's
// list: a coding model when one is installed, otherwise the first by name.
// Returns "" when there is nothing installed.
func SuggestLocalModel(models []string) string {
	if len(models) == 0 {
		return ""
	}
	for _, m := range models {
		lower := strings.ToLower(m)
		if strings.Contains(lower, "coder") || strings.Contains(lower, "code") {
			return m
		}
	}
	return models[0]
}

// newLocalProvider returns an OpenAI-compatible provider for a local server.
// Local servers ignore credentials, so none are sent.
func newLocalProvider(srv LocalServer) *OpenAiCompatProvider {
	p := NewOpenAiCompatProvider(OpenAiCompatConfig{
		ProviderName: srv.Name + " (local)",
		BaseURLEnv:   "GOCODE_LOCAL_BASE_URL_UNUSED",
		DefaultBase:  srv.BaseURL,
	}, apitypes.AuthSource{})
	p.BaseURL = srv.BaseURL
	return p
}
