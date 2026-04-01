package repl

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/AlleyBo55/gocode/internal/agent"
	"github.com/AlleyBo55/gocode/internal/apitypes"
)

// REPL provides the interactive terminal chat interface.
type REPL struct {
	runtime *agent.ConversationRuntime
	reader  io.Reader
	writer  io.Writer
	display *Display
}

// NewREPL creates a new REPL.
func NewREPL(rt *agent.ConversationRuntime, r io.Reader, w io.Writer) *REPL {
	return &REPL{
		runtime: rt,
		reader:  r,
		writer:  w,
		display: NewDisplay(w),
	}
}

// Run starts the interactive REPL loop.
func (r *REPL) Run(ctx context.Context) error {
	scanner := bufio.NewScanner(r.reader)
	fmt.Fprintln(r.writer, "gocode agent — type /exit to quit, /clear to reset, /cost for usage")
	fmt.Fprintln(r.writer)

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

		// Handle slash commands
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

		// Stream the response
		fmt.Fprint(r.writer, "\nassistant> ")
		eventCh, err := r.runtime.StreamUserMessage(ctx, input)
		if err != nil {
			r.display.Error(err)
			continue
		}

		for ev := range eventCh {
			r.display.StreamEvent(ev)
		}
		fmt.Fprintln(r.writer)
	}
}

// TerminalPermissionPrompter prompts the user in the terminal for permission.
type TerminalPermissionPrompter struct {
	Reader  io.Reader
	Writer  io.Writer
	Display *Display
}

// Prompt asks the user for permission.
func (p *TerminalPermissionPrompter) Prompt(toolName string, operation string) (bool, error) {
	p.Display.PermissionPrompt(toolName, operation)
	scanner := bufio.NewScanner(p.Reader)
	if !scanner.Scan() {
		return false, nil
	}
	answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
	return answer == "y" || answer == "yes", nil
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
