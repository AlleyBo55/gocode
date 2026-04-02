package repl

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/AlleyBo55/gocode/internal/agent"
	"github.com/AlleyBo55/gocode/internal/apitypes"
	"github.com/AlleyBo55/gocode/internal/initdeep"
	"github.com/AlleyBo55/gocode/internal/skills"
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
	skills  []skills.Skill
}

// NewREPL creates a new REPL.
func NewREPL(rt *agent.ConversationRuntime, r io.Reader, w io.Writer, cfg REPLConfig, sk []skills.Skill) *REPL {
	return &REPL{
		runtime: rt,
		reader:  r,
		writer:  w,
		display: NewDisplay(w),
		config:  cfg,
		skills:  sk,
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
		case CmdSkill:
			r.handleSkillCommand(input)
			continue
		}

		// Show spinner while waiting for LLM response
		fmt.Fprintln(r.writer)
		spin := NewSpinner(r.writer, "Thinking...")

		// Wire spinner to tool callback so it stops during tool execution
		if tcb, ok := r.runtime.GetToolCb().(*TerminalToolCallback); ok {
			tcb.Spinner = spin
		}

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

// handleSkillCommand processes the /skill slash command.
// With no arguments it lists all available skills.
// With a skill name it activates that skill by injecting its system prompt.
func (r *REPL) handleSkillCommand(input string) {
	trimmed := strings.TrimSpace(input)
	arg := strings.TrimSpace(strings.TrimPrefix(trimmed, "/skill"))

	if arg == "" {
		// List all available skills.
		if len(r.skills) == 0 {
			fmt.Fprintln(r.writer, "No skills available.")
			return
		}
		fmt.Fprintln(r.writer, "Available skills:")
		for _, s := range r.skills {
			fmt.Fprintf(r.writer, "  %s%-20s%s %s\n", cGreen, s.Name, ansiReset, truncateSkillDesc(s.SystemPrompt, 80))
		}
		return
	}

	// Look up the skill by name.
	var found *skills.Skill
	for i := range r.skills {
		if r.skills[i].Name == arg {
			found = &r.skills[i]
			break
		}
	}
	if found == nil {
		fmt.Fprintf(r.writer, "Unknown skill: %s. Use /skill to list available skills.\n", arg)
		return
	}

	// Activate by injecting the skill's system prompt as a user message.
	activationMsg := fmt.Sprintf("The following skill has been activated: %s. Apply these guidelines:\n\n%s", found.Name, found.SystemPrompt)
	_, err := r.runtime.SendUserMessage(context.Background(), activationMsg)
	if err != nil {
		fmt.Fprintf(r.writer, "Error activating skill: %v\n", err)
		return
	}
	fmt.Fprintf(r.writer, "Skill %s%s%s activated.\n", cGreen+ansiBold, found.Name, ansiReset)
}

// truncateSkillDesc shortens a string to maxLen characters, appending "..." if truncated.
func truncateSkillDesc(s string, maxLen int) string {
	// Collapse to single line for display.
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// TerminalToolCallback updates the terminal during tool execution.
// It stops the spinner before showing tool progress.
type TerminalToolCallback struct {
	Writer  io.Writer
	Spinner *Spinner
}

func (t *TerminalToolCallback) OnToolStart(name string, input map[string]interface{}) {
	if t.Spinner != nil {
		t.Spinner.Stop()
	}
	// Show tool name with key params for visibility
	summary := summarizeToolInput(name, input)
	fmt.Fprintf(t.Writer, "  %s⚡ %s%s%s %s\n", cBlue, cWhite+ansiBold, name, ansiReset, summary)
}

func (t *TerminalToolCallback) OnToolEnd(name string, success bool) {
	if success {
		fmt.Fprintf(t.Writer, "  %s✓ %s%s\n", cGreen, name, ansiReset)
	} else {
		fmt.Fprintf(t.Writer, "  %s✗ %s%s\n", cRed, name, ansiReset)
	}
	// Restart spinner for the next LLM call
	if t.Spinner != nil {
		t.Spinner.Start()
	}
}

// summarizeToolInput extracts the most useful param for display.
func summarizeToolInput(name string, input map[string]interface{}) string {
	if input == nil {
		return ""
	}
	// Show the most relevant param based on tool type
	for _, key := range []string{"path", "pattern", "command", "file", "name"} {
		if v, ok := input[key]; ok {
			s := fmt.Sprintf("%v", v)
			if len(s) > 60 {
				s = s[:57] + "..."
			}
			return cGray + s + ansiReset
		}
	}
	return ""
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
// Modeled after Claude Code's agentic behavior — proactive, autonomous, thorough.
func BuildSystemPrompt(tools []apitypes.ToolDef) string {
	cwd, _ := os.Getwd()
	osName := os.Getenv("OSTYPE")
	if osName == "" {
		osName = "unix"
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}

	var toolList strings.Builder
	for _, t := range tools {
		toolList.WriteString(fmt.Sprintf("- %s: %s\n", t.Name, t.Description))
	}

	return fmt.Sprintf(`You are gocode, an interactive CLI tool that helps users with software engineering tasks. Use the instructions below and the tools available to you to assist the user.

IMPORTANT: You should be proactive in accomplishing the task, not reactive. Do not wait for the user to ask you to do something that you can anticipate.

# Tool Use

You have tools at your disposal to solve the coding task. Follow these rules regarding tool calls:

1. ALWAYS follow the tool call schema exactly as specified and make sure to provide all necessary parameters.
2. The conversation may reference tools that are no longer available. NEVER call tools that are not explicitly provided.
3. **NEVER refer to tool names when speaking to the user.** For example, instead of saying "I need to use the BashTool to run a command", just say "Let me run that command" or simply run it.
4. Only call tools when they are necessary. If the user's task is conversational or does not require tool use, respond without calling tools.
5. When you need information, prefer using tools over asking the user.

# Tool Use Best Practices

- When doing file search, prefer GrepTool for searching file contents and GlobTool for finding files by name pattern.
- When reading files, always read the full file first unless you know the exact line range needed.
- When you need to edit a file, ALWAYS read it first so you have the exact content for old_text matching.
- When running commands with BashTool, prefer non-interactive commands. Avoid interactive commands that require user input.
- When using BashTool, do not use commands that produce very large outputs. If needed, pipe to head or tail.

# Making Code Changes

When making code changes:

1. Read the relevant file(s) first to understand the current code.
2. Make the minimal necessary changes to accomplish the task.
3. Ensure your changes are syntactically correct and follow the existing code style.
4. After making changes, verify them if possible (e.g., run tests, check for syntax errors).
5. NEVER leave placeholder comments like "// rest of code here" or "// existing code". Always include the complete code.

# Communication Style

1. Be concise and direct. Avoid unnecessary preamble or filler.
2. When you have completed a task, briefly summarize what you did. Do not list every step.
3. If something fails, explain what went wrong and what you tried.
4. Use markdown formatting in responses when it improves readability.
5. NEVER say "Let me know if you'd like me to..." or "Would you like me to..." — just do it.
6. NEVER ask "Shall I proceed?" or "Should I continue?" — just proceed.

# Environment

- Working directory: %s
- OS: %s
- Shell: %s

# Available Tools

%s`, cwd, osName, shell, toolList.String())
}
