package repl

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/AlleyBo55/gocode/internal/agent"
	"github.com/AlleyBo55/gocode/internal/apitypes"
)

// REPLConfig holds configuration for the REPL display.
type REPLConfig struct {
	Version  string
	Model    string
	MaxTurns int
}

// REPL provides the interactive terminal chat interface.
type REPL struct {
	runtime *agent.ConversationRuntime
	reader  io.Reader
	writer  io.Writer
	display *Display
	config  REPLConfig
}

// NewREPL creates a new REPL.
func NewREPL(rt *agent.ConversationRuntime, r io.Reader, w io.Writer, cfg REPLConfig) *REPL {
	return &REPL{
		runtime: rt,
		reader:  r,
		writer:  w,
		display: NewDisplay(w),
		config:  cfg,
	}
}

// Run starts the interactive REPL loop.
func (r *REPL) Run(ctx context.Context) error {
	scanner := bufio.NewScanner(r.reader)
	PrintBanner(r.writer, BannerConfig{
		Version:  r.config.Version,
		Model:    r.config.Model,
		MaxTurns: r.config.MaxTurns,
	})

	for {
		fmt.Fprint(r.writer, "you> ")
		input, err := ReadInput(scanner)
		if err != nil {
			if err == io.EOF {
				fmt.Fprintln(r.writer, "\nGoodbye!")
				return nil
			}
			return err
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		switch ParseSlashCommand(input) {
		case CmdExit:
			fmt.Fprintln(r.writer, "Goodbye!")
			return nil
		case CmdClear:
			r.runtime.RestoreSession(nil)
			fmt.Fprintln(r.writer, "Session cleared.")
			continue
		case CmdCost:
			r.display.Usage(r.runtime.GetUsage().Render())
			continue
		}

		// Use non-streaming SendUserMessage so tool execution
		// happens synchronously and permission prompts work
		fmt.Fprint(r.writer, "\nassistant> ")
		resp, err := r.runtime.SendUserMessage(ctx, input)
		if err != nil {
			r.display.Error(err)
			fmt.Fprintln(r.writer)
			continue
		}

		for _, block := range resp.Content {
			switch block.Kind {
			case "text":
				fmt.Fprint(r.writer, block.Text)
			case "tool_use":
				inputStr := "{}"
				if len(block.Input) > 0 {
					inputStr = string(block.Input)
				}
				fmt.Fprintf(r.writer, "\n⚡ Tool: %s(%s)\n", block.Name, summarizeJSON(inputStr))
			}
		}
		fmt.Fprintln(r.writer)
	}
}

// TerminalPermissionPrompter prompts the user in the terminal for permission.
type TerminalPermissionPrompter struct {
	Scanner *bufio.Scanner
	Writer  io.Writer
}

// Prompt asks the user for permission, showing the tool name and params.
func (p *TerminalPermissionPrompter) Prompt(toolName string, operation string) (bool, error) {
	summary := summarizeJSON(operation)
	fmt.Fprintf(p.Writer, "🔒 %s wants to run: %s\n", toolName, summary)
	fmt.Fprint(p.Writer, "   Allow? [y/N]: ")
	if !p.Scanner.Scan() {
		return false, nil
	}
	answer := strings.TrimSpace(strings.ToLower(p.Scanner.Text()))
	return answer == "y" || answer == "yes", nil
}

// summarizeJSON returns a compact summary of a JSON string for display.
func summarizeJSON(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || s == "{}" || s == "null" {
		return "(no params)"
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return s
	}
	parts := make([]string, 0, len(m))
	for k, v := range m {
		vs := fmt.Sprintf("%v", v)
		if len(vs) > 60 {
			vs = vs[:57] + "..."
		}
		parts = append(parts, fmt.Sprintf("%s=%s", k, vs))
	}
	result := strings.Join(parts, ", ")
	if len(result) > 120 {
		result = result[:117] + "..."
	}
	return result
}

// RunOneShot runs a single prompt through the agent and prints the result.
func RunOneShot(ctx context.Context, rt *agent.ConversationRuntime, prompt string, stream bool, w io.Writer) error {
	if stream {
		eventCh, err := rt.StreamUserMessage(ctx, prompt)
		if err != nil {
			return err
		}
		display := NewDisplay(w)
		for ev := range eventCh {
			display.StreamEvent(ev)
		}
		fmt.Fprintln(w)
		return nil
	}

	resp, err := rt.SendUserMessage(ctx, prompt)
	if err != nil {
		return err
	}
	for _, block := range resp.Content {
		if block.Kind == "text" {
			fmt.Fprintln(w, block.Text)
		}
	}
	return nil
}

// BuildSystemPrompt constructs the system prompt for the agent.
func BuildSystemPrompt(tools []apitypes.ToolDef) string {
	var sb strings.Builder
	sb.WriteString("You are gocode, an AI coding agent. You have access to the following tools:\n\n")
	for _, t := range tools {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", t.Name, t.Description))
	}
	sb.WriteString("\nUse tools to help the user with coding tasks. Be concise and helpful.")
	return sb.String()
}
