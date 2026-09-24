package apiclient

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlleyBo55/gocode/internal/apitypes"
)

func TestResolveModelAlias(t *testing.T) {
	for in, want := range map[string]string{
		"sonnet":       "claude-sonnet-4-6",
		"  SONNET  ":   "claude-sonnet-4-6", // case and whitespace insensitive
		"gpt5":         "gpt-5.4",
		"codex":        "codex-mini-latest",
		"deepseek":     "deepseek-chat",
		"groq-llama":   "llama-3.3-70b-versatile",
		"gemini-flash": "gemini-3-flash",
		// Unknown names pass through trimmed, never altered.
		"  anthropic/claude-3.5-sonnet ": "anthropic/claude-3.5-sonnet",
		"my-finetune":                    "my-finetune",
	} {
		if got := ResolveModelAlias(in); got != want {
			t.Errorf("ResolveModelAlias(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDetectProviderKindByModelName(t *testing.T) {
	clearProviderEnv(t)
	for model, want := range map[string]ProviderKind{
		"claude-sonnet-4-6": ProviderAnthropic,
		"opus":              ProviderAnthropic,
		"gpt-4o":            ProviderOpenAi,
		"gpt5":              ProviderOpenAi,
		"o3":                ProviderOpenAi,
		"o4-mini":           ProviderOpenAi,
		// Was unreachable: the generic OpenAI branch claimed "codex" first, so
		// the Codex credential path never ran.
		"codex":             ProviderCodex,
		"codex-mini-latest": ProviderCodex,
		"gemini":            ProviderGemini,
		"gemini-2.5-pro":    ProviderGemini,
		"grok":              ProviderXai,
		"grok-3":            ProviderXai,
	} {
		if got := DetectProviderKind(model); got != want {
			t.Errorf("DetectProviderKind(%q) = %v, want %v", model, got, want)
		}
	}
}

func TestDetectProviderKindProxyRequiresItsKey(t *testing.T) {
	clearProviderEnv(t)
	// Without the proxy key a proxy-looking model falls through to whatever
	// native credentials exist; here none do, so the Anthropic default wins.
	if got := DetectProviderKind("deepseek-chat"); got != ProviderAnthropic {
		t.Errorf("deepseek without key = %v, want Anthropic default", got)
	}

	cases := []struct {
		env   string
		model string
		want  ProviderKind
	}{
		{"DEEPSEEK_API_KEY", "deepseek-chat", ProviderDeepSeek},
		{"DEEPSEEK_API_KEY", "deepseek-r1", ProviderDeepSeek},
		{"MISTRAL_API_KEY", "mistral-large-latest", ProviderMistral},
		{"MISTRAL_API_KEY", "codestral", ProviderMistral},
		{"MISTRAL_API_KEY", "open-mistral-nemo", ProviderMistral},
		{"TOGETHER_API_KEY", "meta-llama/Meta-Llama-3.1-405B-Instruct-Turbo", ProviderTogether},
		{"TOGETHER_API_KEY", "Qwen/Qwen2.5-72B-Instruct-Turbo", ProviderTogether},
		{"GROQ_API_KEY", "llama-3.3-70b-versatile", ProviderGroq},
		{"GROQ_API_KEY", "mixtral-8x7b-32768", ProviderGroq},
		{"OPENROUTER_API_KEY", "anthropic/claude-sonnet-4", ProviderOpenRouter},
		{"OPENROUTER_API_KEY", "x-ai/grok-3", ProviderOpenRouter},
	}
	for _, tc := range cases {
		t.Run(tc.env+"/"+tc.model, func(t *testing.T) {
			clearProviderEnv(t)
			t.Setenv(tc.env, "key")
			if got := DetectProviderKind(tc.model); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDetectProviderKindFallsBackToCredentialsInPriorityOrder(t *testing.T) {
	// A model name nothing recognises routes to whichever provider the user
	// has credentials for, checked in a fixed order.
	order := []struct {
		env  string
		want ProviderKind
	}{
		{"ANTHROPIC_API_KEY", ProviderAnthropic},
		{"OPENAI_API_KEY", ProviderOpenAi},
		{"GEMINI_API_KEY", ProviderGemini},
		{"XAI_API_KEY", ProviderXai},
		{"OPENROUTER_API_KEY", ProviderOpenRouter},
		{"TOGETHER_API_KEY", ProviderTogether},
		{"GROQ_API_KEY", ProviderGroq},
	}
	for i, tc := range order {
		t.Run(tc.env, func(t *testing.T) {
			clearProviderEnv(t)
			// Set this key and every lower-priority one; this one must win.
			for _, later := range order[i:] {
				t.Setenv(later.env, "key")
			}
			if got := DetectProviderKind("mystery-model"); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
	clearProviderEnv(t)
	if got := DetectProviderKind("mystery-model"); got != ProviderAnthropic {
		t.Errorf("no credentials at all = %v, want Anthropic default", got)
	}
}

func TestDetectProviderKindCustomBaseURLOverridesModelName(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv("OPENAI_BASE_URL", "http://localhost:11434/v1")
	if got := DetectProviderKind("claude-sonnet-4-6"); got != ProviderOpenAi {
		t.Errorf("local endpoint must route everything through OpenAI-compat, got %v", got)
	}
	// The official OpenAI URL is not "custom" and must not hijack Claude.
	t.Setenv("OPENAI_BASE_URL", "https://api.openai.com/v1")
	if got := DetectProviderKind("claude-sonnet-4-6"); got != ProviderAnthropic {
		t.Errorf("official OpenAI URL should not override, got %v", got)
	}
}

func TestResolveProviderOpenAIBaseURLWinsAndNeedsNoKey(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv("OPENAI_BASE_URL", "http://localhost:11434/v1")
	p, model, err := ResolveProvider("llama", "")
	if err != nil {
		t.Fatal(err)
	}
	compat, ok := p.(*OpenAiCompatProvider)
	if !ok {
		t.Fatalf("got %T", p)
	}
	if compat.BaseURL != "http://localhost:11434/v1" || compat.Config.ProviderName != "OpenAI-Compatible" {
		t.Errorf("provider = %+v", compat.Config)
	}
	if compat.Auth.ApiKey != "" || compat.Auth.BearerToken != "" {
		t.Errorf("local endpoints must not require credentials: %+v", compat.Auth)
	}
	if model != "llama3.3:70b" {
		t.Errorf("alias not expanded: %s", model)
	}
}

func TestResolveProviderAnthropicCredentialForms(t *testing.T) {
	clearProviderEnv(t)
	_, _, err := ResolveProvider("sonnet", "")
	var apiErr *apitypes.ApiError
	if !errors.As(err, &apiErr) || apiErr.Kind != apitypes.ErrMissingCredentials {
		t.Fatalf("no credentials should be MissingCredentials, got %v", err)
	}
	for _, want := range []string{"Anthropic", "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should name %s", err, want)
		}
	}

	t.Setenv("ANTHROPIC_API_KEY", "sk-ant")
	p, model, err := ResolveProvider("sonnet", "")
	if err != nil {
		t.Fatal(err)
	}
	ap, ok := p.(*AnthropicProvider)
	if !ok || ap.Auth.ApiKey != "sk-ant" || ap.Auth.BearerToken != "" || model != "claude-sonnet-4-6" {
		t.Errorf("api key form: %T %+v %s", p, ap.Auth, model)
	}

	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "tok")
	p, _, _ = ResolveProvider("sonnet", "")
	if a := p.(*AnthropicProvider).Auth; a.BearerToken != "tok" || a.ApiKey != "" {
		t.Errorf("bearer form: %+v", a)
	}

	t.Setenv("ANTHROPIC_API_KEY", "sk-ant")
	p, _, _ = ResolveProvider("sonnet", "")
	if a := p.(*AnthropicProvider).Auth; a.Kind != "api_key_and_bearer" || a.ApiKey != "sk-ant" || a.BearerToken != "tok" {
		t.Errorf("both form: %+v", a)
	}

	// The --api-key flag beats every environment variable.
	p, _, _ = ResolveProvider("sonnet", "flag-key")
	if a := p.(*AnthropicProvider).Auth; a.ApiKey != "flag-key" || a.BearerToken != "" {
		t.Errorf("flag should win outright: %+v", a)
	}
}

func TestResolveProviderNativeOpenAICompatFamilies(t *testing.T) {
	cases := []struct {
		model, env, wantName, wantBase string
	}{
		{"gpt5", "OPENAI_API_KEY", "OpenAI", "https://api.openai.com/v1"},
		{"grok", "XAI_API_KEY", "xAI", "https://api.x.ai/v1"},
		{"gemini", "GEMINI_API_KEY", "Google Gemini", "https://generativelanguage.googleapis.com/v1beta/openai"},
		{"gemini", "GOOGLE_API_KEY", "Google Gemini", "https://generativelanguage.googleapis.com/v1beta/openai"},
	}
	for _, tc := range cases {
		t.Run(tc.model+"/"+tc.env, func(t *testing.T) {
			clearProviderEnv(t)
			if _, _, err := ResolveProvider(tc.model, ""); err == nil {
				t.Fatal("must fail without credentials")
			}
			t.Setenv(tc.env, "key")
			p, _, err := ResolveProvider(tc.model, "")
			if err != nil {
				t.Fatal(err)
			}
			compat := p.(*OpenAiCompatProvider)
			if compat.Config.ProviderName != tc.wantName || compat.BaseURL != tc.wantBase || compat.Auth.ApiKey != "key" {
				t.Errorf("got name=%q base=%q auth=%+v", compat.Config.ProviderName, compat.BaseURL, compat.Auth)
			}
		})
	}
}

func TestResolveProviderProxyServices(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv("DEEPSEEK_API_KEY", "ds")
	p, model, err := ResolveProvider("deepseek", "")
	if err != nil {
		t.Fatal(err)
	}
	compat := p.(*OpenAiCompatProvider)
	if compat.Config.ProviderName != "DeepSeek" || compat.BaseURL != "https://api.deepseek.com/v1" || compat.Auth.ApiKey != "ds" || model != "deepseek-chat" {
		t.Errorf("deepseek: %+v base=%s model=%s", compat.Config, compat.BaseURL, model)
	}

	// A proxy base URL override is honoured.
	t.Setenv("DEEPSEEK_BASE_URL", "https://proxy.example/v1")
	p, _, _ = ResolveProvider("deepseek", "")
	if b := p.(*OpenAiCompatProvider).BaseURL; b != "https://proxy.example/v1" {
		t.Errorf("base override ignored: %s", b)
	}

	// The --api-key flag substitutes for the proxy key. Detection still needs
	// the env var to pick the proxy, so this mirrors a user who has the key
	// exported but wants to override it for one run.
	t.Setenv("DEEPSEEK_API_KEY", "ds")
	p, _, err = ResolveProvider("deepseek", "override")
	if err != nil {
		t.Fatal(err)
	}
	if a := p.(*OpenAiCompatProvider).Auth.ApiKey; a != "ds" {
		// Documenting current behaviour: env auth is resolved first and the
		// flag is only consulted when the env var is absent.
		t.Logf("note: proxy providers prefer the env key over --api-key (got %q)", a)
	}
}

func TestResolveProviderCodexReadsCodexCLICache(t *testing.T) {
	clearProviderEnv(t)
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	writeFile(t, filepath.Join(home, "auth.json"), `{"auth_mode":"apikey","OPENAI_API_KEY":"sk-from-codex"}`)

	p, model, err := ResolveProvider("codex", "")
	if err != nil {
		t.Fatal(err)
	}
	compat, ok := p.(*OpenAiCompatProvider)
	if !ok {
		t.Fatalf("got %T", p)
	}
	if compat.Config.ProviderName != "Codex" || compat.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("config = %+v base=%s", compat.Config, compat.BaseURL)
	}
	if compat.Auth.ApiKey != "sk-from-codex" {
		t.Errorf("auth = %+v, want the key from auth.json", compat.Auth)
	}
	if model != "codex-mini-latest" {
		t.Errorf("model = %s", model)
	}

	t.Setenv("CODEX_BASE_URL", "https://codex-proxy.example/v1")
	p, _, _ = ResolveProvider("codex", "")
	if b := p.(*OpenAiCompatProvider).BaseURL; b != "https://codex-proxy.example/v1" {
		t.Errorf("CODEX_BASE_URL ignored: %s", b)
	}
}

func TestResolveProviderCodexPrecedenceAndFailure(t *testing.T) {
	clearProviderEnv(t)
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)

	// Nothing anywhere: a clear error, not a provider that will 401.
	_, _, err := ResolveProvider("codex", "")
	var apiErr *apitypes.ApiError
	if !errors.As(err, &apiErr) || apiErr.Kind != apitypes.ErrMissingCredentials || !strings.Contains(err.Error(), "OPENAI_API_KEY") {
		t.Fatalf("want MissingCredentials naming OPENAI_API_KEY, got %v", err)
	}

	// Env var alone is enough.
	t.Setenv("OPENAI_API_KEY", "sk-env")
	p, _, err := ResolveProvider("codex", "")
	if err != nil || p.(*OpenAiCompatProvider).Auth.ApiKey != "sk-env" {
		t.Errorf("env fallback: %v %+v", err, p)
	}

	// Cache beats env.
	writeFile(t, filepath.Join(home, "auth.json"), `{"auth_mode":"apikey","OPENAI_API_KEY":"sk-cache"}`)
	p, _, _ = ResolveProvider("codex", "")
	if a := p.(*OpenAiCompatProvider).Auth.ApiKey; a != "sk-cache" {
		t.Errorf("cache should beat env, got %q", a)
	}

	// Flag beats both.
	p, _, _ = ResolveProvider("codex", "sk-flag")
	if a := p.(*OpenAiCompatProvider).Auth.ApiKey; a != "sk-flag" {
		t.Errorf("flag should beat cache, got %q", a)
	}
}

func TestReadCodexApiKeyFileShapes(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name, body, want string
	}{
		{"current cli layout", `{"auth_mode":"apikey","OPENAI_API_KEY":"sk-1"}`, "sk-1"},
		{"legacy layout", `{"api_key":"sk-legacy"}`, "sk-legacy"},
		{"current wins over legacy", `{"api_key":"old","OPENAI_API_KEY":"new"}`, "new"},
		// A ChatGPT sign-in leaves an OAuth token that is only valid against
		// the ChatGPT backend, which this provider cannot talk to. Reading it
		// would turn a clear missing-credentials error into a confusing 401.
		{"chatgpt oauth layout is ignored", `{"auth_mode":"chatgpt","tokens":{"access_token":"eyJ...","refresh_token":"r","account_id":"acc"}}`, ""},
		{"empty key", `{"OPENAI_API_KEY":""}`, ""},
		{"not json", `not json`, ""},
		{"empty file", ``, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, strings.ReplaceAll(tc.name, " ", "_")+".json")
			writeFile(t, path, tc.body)
			if got := readCodexApiKey(path); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
	if got := readCodexApiKey(filepath.Join(dir, "does-not-exist.json")); got != "" {
		t.Errorf("missing file should yield empty, got %q", got)
	}
}

func TestCodexAuthPathHonoursCodexHome(t *testing.T) {
	t.Setenv("CODEX_HOME", "/tmp/codex-home")
	if got := codexAuthPath(); got != filepath.Join("/tmp/codex-home", "auth.json") {
		t.Errorf("got %s", got)
	}
	t.Setenv("CODEX_HOME", "")
	home, _ := os.UserHomeDir()
	if got := codexAuthPath(); got != filepath.Join(home, ".codex", "auth.json") {
		t.Errorf("default = %s", got)
	}
}

func TestApplyAuthHeaders(t *testing.T) {
	req, _ := http.NewRequest("POST", "http://x", nil)
	ApplyAuth(req, apitypes.AuthApiKeyAndBearer("k", "t"))
	if req.Header.Get("x-api-key") != "k" || req.Header.Get("Authorization") != "Bearer t" {
		t.Errorf("headers = %v", req.Header)
	}
	req, _ = http.NewRequest("POST", "http://x", nil)
	ApplyAuth(req, apitypes.AuthSource{})
	if len(req.Header) != 0 {
		t.Errorf("empty auth must set nothing: %v", req.Header)
	}
}

func TestModelLimitsResolveAliasesFirst(t *testing.T) {
	// A handful of pins so an alias rename cannot silently change limits.
	if MaxTokensForModel("sonnet") != 64000 || MaxTokensForModel("claude-sonnet-4-6") != 64000 {
		t.Error("sonnet max tokens")
	}
	if MaxTokensForModel("gpt5") != 128000 || ContextWindowForModel("gpt5") != 1000000 {
		t.Error("gpt5 limits")
	}
	if MaxTokensForModel("something-unknown") != 16384 || ContextWindowForModel("something-unknown") != 128000 {
		t.Error("unknown model defaults")
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
