package repl

import (
	"fmt"
	"strings"

	"github.com/AlleyBo55/gocode/internal/apiclient"
)

// MissingCredentialsHelp is what a new user sees instead of a bare
// "missing credentials" error. It gives every kind of user a path that works
// in one command: no signup, one key, or the key they already have. When a
// local server is running, its installed models are named so the first
// command can be copied as-is.
func MissingCredentialsHelp(model string, cause error, local apiclient.LocalServer, localFound bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "gocode needs a model to talk to, and none is set up for %q.\n", model)
	if cause != nil {
		fmt.Fprintf(&b, "  (%v)\n", cause)
	}
	b.WriteString("\nThree ways to start. Pick one:\n\n")

	// 1. Local.
	switch {
	case localFound && len(local.Models) > 0:
		suggested := apiclient.SuggestLocalModel(local.Models)
		fmt.Fprintf(&b, "  Free, private, no signup — %s is running with %s:\n", local.Name, joinModels(local.Models, 4))
		fmt.Fprintf(&b, "    gocode chat --model %s\n\n", suggested)
	case localFound:
		fmt.Fprintf(&b, "  Free, private, no signup — %s is running but has no models yet:\n", local.Name)
		b.WriteString("    ollama pull qwen2.5-coder:7b\n")
		b.WriteString("    gocode chat --model qwen2.5-coder:7b\n\n")
	default:
		b.WriteString("  Free, private, no signup — install Ollama (https://ollama.com), then:\n")
		b.WriteString("    ollama pull qwen2.5-coder:7b\n")
		b.WriteString("    gocode chat --model qwen2.5-coder:7b\n\n")
	}

	// 2. One key.
	b.WriteString("  One key, every hosted model — https://openrouter.ai/keys\n")
	b.WriteString("    export OPENROUTER_API_KEY=sk-or-...\n")
	b.WriteString("    gocode chat --model anthropic/claude-sonnet-4-6\n\n")

	// 3. Own key.
	b.WriteString("  Your own provider key:\n")
	b.WriteString("    export ANTHROPIC_API_KEY=sk-ant-...   # or OPENAI_API_KEY, GEMINI_API_KEY, GROQ_API_KEY, ...\n")
	b.WriteString("    gocode chat\n\n")

	b.WriteString("Then:  gocode doctor                 shows which keys and tools are set up\n")
	b.WriteString("       gocode doctor --model NAME    checks what a model can actually do\n")
	return b.String()
}

func joinModels(models []string, limit int) string {
	if len(models) <= limit {
		return strings.Join(models, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(models[:limit], ", "), len(models)-limit)
}
