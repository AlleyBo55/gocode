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
	"github.com/AlleyBo55/gocode/internal/initdeep"
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
		fmt.Fprintf(r.writer, "%syou>%s ", cGreen+ansiBold, ansiReset)
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
		case CmdPlan:
			fmt.Fprintln(r.writer, "Planning mode is not yet wired to a provider. Use /plan after configuring an orchestrator.")
			continue
		case CmdInitDeep:
			fmt.Fprintln(r.writer, "Generating AGENTS.md context files...")
			gen := initdeep.NewGenerator()
			report, err := gen.Generate(".")
			if err != nil {
				fmt.Fprintf(r.writer, "Error: %v\n", err)
			} else {
				fmt.Fprintf(r.writer, "Created %d AGENTS.md files, skipped %d existing.\n", len(report.Created), len(report.Skipped))
			}
			continue
		}

		// Show spinner while waiting for LLM response
		fmt.Fprintln(r.writer)
		spin := NewSpinner(r.writer, "Thinking...")
		spin.Start()

		resp, err := r.runtime.SendUserMessage(ctx, input)
		spin.Stop()

		if err != nil {
			r.display.Error(err)
			fmt.Fprintln(r.writer)
			continue
		}

		fmt.Fprintf(r.writer, "%sassistant>%s ", cBlue+ansiBold, ansiReset)
		r.display.RenderResponse(resp)
		fmt.Fprintln(r.writer)
	}
}

// TerminalToolCallback updates the spinner during tool execution.
type TerminalToolCallback struct {
	Writer io.Writer
}

func (t *TerminalToolCallback) OnToolStart(name string, input map[string]interface{}) {
	fmt.Fprintf(t.Writer, "\r\033[K  %s⚡ Running %s%s...%s\n", cBlue, cWhite+ansiBold, name, ansiReset)
}

func (t *TerminalToolCallback) OnToolEnd(name string, success bool) {
	if success {
		fmt.Fprintf(t.Writer, "  %s✓ %s%s\n", cGreen, name, ansiReset)
	} else {
		fmt.Fprintf(t.Writer, "  %s✗ %s%s\n", cRed, name, ansiReset)
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
	display := NewDisplay(p.Writer)
	display.PermissionPrompt(toolName, summary)
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
