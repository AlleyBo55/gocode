package repl

import (
	"errors"
	"strings"
	"testing"

	"github.com/AlleyBo55/gocode/internal/apiclient"
)

func TestMissingCredentialsHelpNamesInstalledLocalModels(t *testing.T) {
	local := apiclient.LocalServer{Name: "Ollama", BaseURL: "http://localhost:11434/v1", Models: []string{"gemma2:9b", "llama3.1:8b", "qwen2.5-coder:7b"}}
	out := MissingCredentialsHelp("sonnet", errors.New("missing Anthropic credentials"), local, true)

	for _, want := range []string{
		`none is set up for "sonnet"`,
		"missing Anthropic credentials",
		"Ollama is running with gemma2:9b, llama3.1:8b, qwen2.5-coder:7b",
		"gocode chat --model qwen2.5-coder:7b", // the coder model is suggested, not the first alphabetically
		"OPENROUTER_API_KEY",
		"ANTHROPIC_API_KEY",
		"gocode doctor --model",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("help should contain %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "ollama pull") {
		t.Error("should not tell a user with models installed to pull one")
	}
}

func TestMissingCredentialsHelpWithoutLocalServer(t *testing.T) {
	out := MissingCredentialsHelp("sonnet", nil, apiclient.LocalServer{}, false)
	for _, want := range []string{"install Ollama", "ollama pull qwen2.5-coder:7b", "gocode chat --model qwen2.5-coder:7b", "openrouter.ai/keys"} {
		if !strings.Contains(out, want) {
			t.Errorf("help should contain %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "(") && strings.Contains(out, "(<nil>)") {
		t.Error("nil cause must not be printed")
	}
}

func TestMissingCredentialsHelpLocalServerWithNoModels(t *testing.T) {
	out := MissingCredentialsHelp("llama", nil, apiclient.LocalServer{Name: "Ollama"}, true)
	if !strings.Contains(out, "running but has no models yet") || !strings.Contains(out, "ollama pull") {
		t.Errorf("empty server should be told to pull:\n%s", out)
	}
}

func TestJoinModelsTruncates(t *testing.T) {
	if got := joinModels([]string{"a", "b", "c", "d", "e", "f"}, 4); got != "a, b, c, d and 2 more" {
		t.Errorf("got %q", got)
	}
	if got := joinModels([]string{"a"}, 4); got != "a" {
		t.Errorf("got %q", got)
	}
}
