package apiclient

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/AlleyBo55/gocode/internal/apitypes"
)

// fakeOllama answers /api/tags like a running Ollama with the given models,
// and /v1/chat/completions with a fixed reply so a full round trip works.
func fakeOllama(t *testing.T, models ...string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, r *http.Request) {
		var b strings.Builder
		b.WriteString(`{"models":[`)
		for i, m := range models {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString(`{"name":"` + m + `","size":1}`)
		}
		b.WriteString(`]}`)
		writeJSON(w, 200, b.String())
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("local server must not receive credentials, got %q", r.Header.Get("Authorization"))
		}
		writeJSON(w, 200, `{"id":"x","model":"`+models[0]+`","choices":[{"index":0,"message":{"role":"assistant","content":"hello from ollama"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":4}}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestLooksLocal(t *testing.T) {
	for model, want := range map[string]bool{
		"qwen2.5-coder:7b":                       true,
		"llama3.3:70b":                           true,
		"llama":                                  true, // alias -> llama3.3:70b
		"qwen-coder":                             true,
		"claude-sonnet-4-6":                      false,
		"gpt-4o":                                 false,
		"anthropic/claude-sonnet-4-6:nitro":      false, // OpenRouter suffix, not a local tag
		"meta-llama/llama-3.3-70b-instruct:free": false,
		"":                                       false,
	} {
		if got := LooksLocal(model); got != want {
			t.Errorf("LooksLocal(%q) = %v, want %v", model, got, want)
		}
	}
}

func TestDetectLocalServerFindsOllamaViaOllamaHost(t *testing.T) {
	clearProviderEnv(t)
	srv := fakeOllama(t, "qwen2.5-coder:7b", "llama3.1:8b")
	// The ollama CLI accepts host:port without a scheme; so do we.
	t.Setenv("OLLAMA_HOST", strings.TrimPrefix(srv.URL, "http://"))

	local, ok := DetectLocalServer()
	if !ok {
		t.Fatal("running Ollama not detected")
	}
	if local.Name != "Ollama" || local.BaseURL != srv.URL+"/v1" {
		t.Errorf("server = %+v", local)
	}
	if want := []string{"llama3.1:8b", "qwen2.5-coder:7b"}; !reflect.DeepEqual(local.Models, want) {
		t.Errorf("models = %v, want sorted %v", local.Models, want)
	}
}

// noLocalServers points both probes at a closed port so the test outcome
// does not depend on whether the developer happens to run Ollama or LM Studio.
func noLocalServers(t *testing.T) {
	t.Helper()
	t.Setenv("OLLAMA_HOST", "http://127.0.0.1:1")
	old := lmStudioBase
	lmStudioBase = "http://127.0.0.1:1"
	t.Cleanup(func() { lmStudioBase = old })
}

func TestDetectLocalServerNothingRunning(t *testing.T) {
	clearProviderEnv(t)
	noLocalServers(t)
	if _, ok := DetectLocalServer(); ok {
		t.Error("detected a server on a closed port")
	}
}

func TestDetectLocalServerFindsLMStudio(t *testing.T) {
	clearProviderEnv(t)
	noLocalServers(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, `{"data":[{"id":"qwen2.5-coder-7b-instruct"},{"id":"gemma-2-9b"}]}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	lmStudioBase = srv.URL

	local, ok := DetectLocalServer()
	if !ok || local.Name != "LM Studio" || local.BaseURL != srv.URL+"/v1" {
		t.Fatalf("server = %+v ok=%v", local, ok)
	}
	if want := []string{"gemma-2-9b", "qwen2.5-coder-7b-instruct"}; !reflect.DeepEqual(local.Models, want) {
		t.Errorf("models = %v", local.Models)
	}
}

func TestResolveProviderRoutesLocalModelsWithoutAnyKey(t *testing.T) {
	clearProviderEnv(t)
	srv := fakeOllama(t, "qwen2.5-coder:7b")
	t.Setenv("OLLAMA_HOST", srv.URL)

	p, model, err := ResolveProvider("qwen2.5-coder:7b", "")
	if err != nil {
		t.Fatalf("a local model with Ollama running must not need credentials: %v", err)
	}
	compat, ok := p.(*OpenAiCompatProvider)
	if !ok || compat.BaseURL != srv.URL+"/v1" || compat.Config.ProviderName != "Ollama (local)" {
		t.Fatalf("provider = %T %+v", p, compat)
	}
	if model != "qwen2.5-coder:7b" {
		t.Errorf("model = %s", model)
	}
	// And it actually talks to the server.
	resp, err := p.SendMessage(t.Context(), userRequest(model))
	if err != nil || resp.Content[0].Text != "hello from ollama" {
		t.Errorf("round trip: %+v %v", resp, err)
	}

	// Aliases that resolve to local ids take the same path.
	if _, _, err := ResolveProvider("llama", ""); err != nil {
		t.Errorf("alias 'llama' should route locally: %v", err)
	}
}

func TestResolveProviderLocalModelWithNoServerExplainsItself(t *testing.T) {
	clearProviderEnv(t)
	noLocalServers(t)
	_, _, err := ResolveProvider("qwen2.5-coder:7b", "")
	var apiErr *apitypes.ApiError
	if !errors.As(err, &apiErr) || apiErr.Kind != apitypes.ErrMissingCredentials {
		t.Fatalf("want MissingCredentials so the CLI shows onboarding, got %v", err)
	}
	for _, want := range []string{"qwen2.5-coder:7b", "looks like a local model", "Ollama", "LM Studio", "OLLAMA_HOST"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "ANTHROPIC") {
		t.Errorf("a local model must not be blamed on missing Anthropic keys: %q", err)
	}
}

func TestExplicitBaseURLStillWinsOverLocalDetection(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv("OPENAI_BASE_URL", "http://my-proxy:9999/v1")
	p, _, err := ResolveProvider("llama3.3:70b", "")
	if err != nil {
		t.Fatal(err)
	}
	if b := p.(*OpenAiCompatProvider).BaseURL; b != "http://my-proxy:9999/v1" {
		t.Errorf("OPENAI_BASE_URL must take precedence over auto-detection, got %s", b)
	}
}

func TestHostedModelsAreNotRoutedLocally(t *testing.T) {
	clearProviderEnv(t)
	srv := fakeOllama(t, "qwen2.5-coder:7b")
	t.Setenv("OLLAMA_HOST", srv.URL)
	// Ollama running must not hijack a hosted model id; that would 404 with a
	// confusing "model not found" instead of a clear credentials message.
	_, _, err := ResolveProvider("sonnet", "")
	var apiErr *apitypes.ApiError
	if !errors.As(err, &apiErr) || apiErr.Kind != apitypes.ErrMissingCredentials || !strings.Contains(err.Error(), "Anthropic") {
		t.Errorf("got %v", err)
	}
}

func TestSuggestLocalModelPrefersCoders(t *testing.T) {
	if got := SuggestLocalModel([]string{"llama3.1:8b", "qwen2.5-coder:7b", "gemma2:9b"}); got != "qwen2.5-coder:7b" {
		t.Errorf("got %q", got)
	}
	if got := SuggestLocalModel([]string{"gemma2:9b", "llama3.1:8b"}); got != "gemma2:9b" {
		t.Errorf("no coder installed: got %q, want first", got)
	}
	if got := SuggestLocalModel(nil); got != "" {
		t.Errorf("empty list: got %q", got)
	}
}

func TestHasAnyProviderCredentials(t *testing.T) {
	clearProviderEnv(t)
	if HasAnyProviderCredentials() {
		t.Fatal("clean env reported credentials")
	}
	for _, env := range []string{"GROQ_API_KEY", "OPENAI_BASE_URL", "ANTHROPIC_AUTH_TOKEN"} {
		clearProviderEnv(t)
		t.Setenv(env, "x")
		if !HasAnyProviderCredentials() {
			t.Errorf("%s not counted as credentials", env)
		}
	}
}

func TestOllamaHostForms(t *testing.T) {
	for in, want := range map[string]string{
		"":                     "http://localhost:11434",
		"127.0.0.1:11434":      "http://127.0.0.1:11434",
		"http://gpu-box:11434": "http://gpu-box:11434",
		"https://ollama.lan/":  "https://ollama.lan",
		"  0.0.0.0:11434  ":    "http://0.0.0.0:11434",
	} {
		t.Setenv("OLLAMA_HOST", in)
		if got := ollamaHost(); got != want {
			t.Errorf("OLLAMA_HOST=%q -> %q, want %q", in, got, want)
		}
	}
}
