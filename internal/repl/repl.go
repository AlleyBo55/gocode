package repl

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
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
		case CmdCompact:
			before := len(r.runtime.GetSession())
			r.runtime.CompactSession(10)
			after := len(r.runtime.GetSession())
			fmt.Fprintf(r.writer, "Compacted: %d → %d messages\n", before, after)
			continue
		case CmdHelp:
			fmt.Fprintln(r.writer, "Available commands:")
			fmt.Fprintln(r.writer, "  /help        Show this help")
			fmt.Fprintln(r.writer, "  /exit        Quit session")
			fmt.Fprintln(r.writer, "  /clear       Reset conversation")
			fmt.Fprintln(r.writer, "  /compact     Compact conversation history")
			fmt.Fprintln(r.writer, "  /cost        Show token usage and cost")
			fmt.Fprintln(r.writer, "  /model       Show or switch model")
			fmt.Fprintln(r.writer, "  /skill       List or activate skills")
			fmt.Fprintln(r.writer, "  /plan        Start planning session")
			fmt.Fprintln(r.writer, "  /init-deep   Generate AGENTS.md files")
			fmt.Fprintln(r.writer, "  /diff        Show git diff of changes")
			fmt.Fprintln(r.writer, "  /undo        Revert uncommitted changes")
			fmt.Fprintln(r.writer, "  /status      Show session stats")
			fmt.Fprintln(r.writer, "  /review      Ask agent to review changes")
			fmt.Fprintln(r.writer, "  /permissions Show permission mode")
			fmt.Fprintln(r.writer, "  /doctor      Check environment")
			continue
		case CmdModel:
			r.handleModelCommand(input)
			continue
		case CmdDiff:
			r.handleDiffCommand()
			continue
		case CmdUndo:
			out, err := exec.Command("git", "diff", "--stat").Output()
			if err != nil || strings.TrimSpace(string(out)) == "" {
				fmt.Fprintln(r.writer, "Nothing to undo.")
				continue
			}
			fmt.Fprintf(r.writer, "Reverting:\n%s", string(out))
			exec.Command("git", "checkout", "--", ".").Run()
			fmt.Fprintln(r.writer, "Changes reverted.")
			continue
		case CmdStatus:
			cwd, _ := os.Getwd()
			usage := r.runtime.GetUsage()
			msgs := len(r.runtime.GetSession())
			fmt.Fprintf(r.writer, "Model:    %s\n", r.config.Model)
			fmt.Fprintf(r.writer, "Messages: %d\n", msgs)
			fmt.Fprintf(r.writer, "Tokens:   %d in / %d out\n", usage.InputTokens, usage.OutputTokens)
			fmt.Fprintf(r.writer, "Turns:    %d / %d max\n", usage.Turns, r.config.MaxTurns)
			fmt.Fprintf(r.writer, "CWD:      %s\n", cwd)
			if branch, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
				fmt.Fprintf(r.writer, "Branch:   %s\n", strings.TrimSpace(string(branch)))
			}
			continue
		case CmdReview:
			diff, _ := exec.Command("git", "diff").Output()
			if len(diff) == 0 {
				fmt.Fprintln(r.writer, "No changes to review.")
				continue
			}
			// Inject the diff as a user message asking for review — fall through to message handler
			input = fmt.Sprintf("Review these changes I just made and point out any issues:\n\n```diff\n%s\n```", string(diff))
		case CmdPermissions:
			fmt.Fprintln(r.writer, "Permission mode: workspace-write (prompts before tool execution)")
			fmt.Fprintln(r.writer, "Use --dangerously-skip-permissions to disable all prompts.")
			continue
		case CmdDoctor:
			fmt.Fprintln(r.writer, "Checking environment...")
			checks := []struct{ name, cmd string }{
				{"git", "git --version"},
				{"go", "go version"},
				{"tmux", "tmux -V"},
				{"ast-grep", "ast-grep --version"},
			}
			for _, c := range checks {
				parts := strings.Fields(c.cmd)
				if out, err := exec.Command(parts[0], parts[1:]...).Output(); err == nil {
					fmt.Fprintf(r.writer, "  ✓ %s: %s\n", c.name, strings.TrimSpace(string(out)))
				} else {
					fmt.Fprintf(r.writer, "  ✗ %s: not found\n", c.name)
				}
			}
			for _, env := range []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GEMINI_API_KEY", "XAI_API_KEY"} {
				if os.Getenv(env) != "" {
					fmt.Fprintf(r.writer, "  ✓ %s: set\n", env)
				} else {
					fmt.Fprintf(r.writer, "  - %s: not set\n", env)
				}
			}
			for _, name := range []string{"GOCODE.md", "CLAUDE.md"} {
				if _, err := os.Stat(name); err == nil {
					fmt.Fprintf(r.writer, "  ✓ %s: found\n", name)
				}
			}
			continue
		}

		// Show spinner while waiting for first token
		fmt.Fprintln(r.writer)
		spin := NewSpinner(r.writer, "Thinking...")

		// Wire spinner to tool callback so it stops during tool execution
		if tcb, ok := r.runtime.GetToolCb().(*TerminalToolCallback); ok {
			tcb.Spinner = spin
		}

		// Set up Ctrl+C to cancel the current request without killing the process
		msgCtx, cancel := context.WithCancel(ctx)
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt)
		go func() {
			select {
			case <-sigCh:
				cancel()
			case <-msgCtx.Done():
			}
			signal.Stop(sigCh)
		}()

		spin.Start()
		eventCh, err := r.runtime.StreamUserMessage(msgCtx, input)
		if err != nil {
			spin.Stop()
			cancel()
			r.display.Error(err)
			fmt.Fprintln(r.writer)
			continue
		}

		firstToken := true
		for ev := range eventCh {
			if firstToken && ev.Kind == "content_block_delta" && ev.BlockDelta != nil && ev.BlockDelta.Kind == "text_delta" {
				spin.Stop()
				fmt.Fprintf(r.writer, "%sassistant>%s ", cBlue+ansiBold, ansiReset)
				firstToken = false
			}
			r.display.StreamEvent(ev)
		}
		if firstToken {
			spin.Stop()
		} // no text was streamed
		cancel()
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

// handleModelCommand processes the /model slash command.
func (r *REPL) handleModelCommand(input string) {
	trimmed := strings.TrimSpace(input)
	arg := strings.TrimSpace(strings.TrimPrefix(trimmed, "/model"))
	if arg == "" {
		fmt.Fprintf(r.writer, "Current model: %s%s%s\n", cGreen+ansiBold, r.config.Model, ansiReset)
		return
	}
	fmt.Fprintf(r.writer, "Model switching mid-session requires restart. Start a new session with: gocode chat --model %s\n", arg)
}

// handleDiffCommand runs git diff and prints the output.
func (r *REPL) handleDiffCommand() {
	out, err := exec.Command("git", "diff").Output()
	if err != nil {
		fmt.Fprintf(r.writer, "Error running git diff: %v\n", err)
		return
	}
	diff := strings.TrimSpace(string(out))
	if diff == "" {
		fmt.Fprintln(r.writer, "No changes detected.")
		return
	}
	fmt.Fprintln(r.writer, diff)
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

// SkipProjectConfig disables loading GOCODE.md/CLAUDE.md into the system prompt.
var SkipProjectConfig bool

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

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`You are gocode, an interactive CLI tool that helps users with software engineering tasks. Use the instructions below and the tools available to you to assist the user.

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

%s`, cwd, osName, shell, toolList.String()))

	// Git context
	if branch, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
		gitBranch := strings.TrimSpace(string(branch))
		// also get short status
		status, _ := exec.Command("git", "status", "--short").Output()
		statusLines := strings.Split(strings.TrimSpace(string(status)), "\n")
		changedFiles := len(statusLines)
		if statusLines[0] == "" {
			changedFiles = 0
		}
		sb.WriteString(fmt.Sprintf("\n# Git\n- Branch: %s\n- Changed files: %d\n", gitBranch, changedFiles))
	}

	// Load project config (CLAUDE.md or GOCODE.md)
	if !SkipProjectConfig {
		for _, name := range []string{"GOCODE.md", "CLAUDE.md"} {
			if data, err := os.ReadFile(name); err == nil {
				sb.WriteString("\n\n# Project Instructions (from " + name + ")\n\n")
				sb.WriteString(string(data))
				break
			}
		}
	}

	return sb.String()
}
