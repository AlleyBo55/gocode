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
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/AlleyBo55/gocode/internal/agent"
	"github.com/AlleyBo55/gocode/internal/apiclient"
	"github.com/AlleyBo55/gocode/internal/apitypes"
	"github.com/AlleyBo55/gocode/internal/buddy"
	"github.com/AlleyBo55/gocode/internal/checkpoint"
	"github.com/AlleyBo55/gocode/internal/customcmd"
	"github.com/AlleyBo55/gocode/internal/dream"
	"github.com/AlleyBo55/gocode/internal/initdeep"
	"github.com/AlleyBo55/gocode/internal/memory"
	"github.com/AlleyBo55/gocode/internal/outputstyles"
	"github.com/AlleyBo55/gocode/internal/skills"
	"github.com/AlleyBo55/gocode/internal/tasks"
	"github.com/AlleyBo55/gocode/internal/ultraplan"
	"github.com/AlleyBo55/gocode/internal/vim"
	"github.com/AlleyBo55/gocode/internal/voice"
	"github.com/AlleyBo55/gocode/internal/worktree"
)

// REPLConfig holds configuration for the REPL display.
type REPLConfig struct {
	Version  string
	Model    string
	MaxTurns int
}

// REPL provides the interactive terminal chat interface.
type REPL struct {
	runtime      *agent.ConversationRuntime
	reader       io.Reader
	writer       io.Writer
	display      *Display
	config       REPLConfig
	skills       []skills.Skill
	customCmds   *customcmd.Loader
	checkpointMgr *checkpoint.Manager
	worktreeMgr  *worktree.Manager
	voiceListener *voice.Listener
	ultraplanner *ultraplan.Planner
	dreamEngine  *dream.Dreamer
	vimState     *vim.PersistentState
	outputStyle  string
	buddy        *buddy.Companion
}

// NewREPL creates a new REPL.
func NewREPL(rt *agent.ConversationRuntime, r io.Reader, w io.Writer, cfg REPLConfig, sk []skills.Skill) *REPL {
	return &REPL{
		runtime:     rt,
		reader:      r,
		writer:      w,
		display:     NewDisplay(w),
		config:      cfg,
		skills:      sk,
		customCmds:  customcmd.NewLoader(),
		vimState:    vim.NewPersistentState(),
		outputStyle: "markdown",
	}
}

// SetCheckpointManager sets the checkpoint manager for /undo support.
func (r *REPL) SetCheckpointManager(mgr *checkpoint.Manager) {
	r.checkpointMgr = mgr
}

// SetWorktreeManager sets the worktree manager for /worktree support.
func (r *REPL) SetWorktreeManager(mgr *worktree.Manager) {
	r.worktreeMgr = mgr
}

// SetVoiceListener sets the voice listener for /voice support.
func (r *REPL) SetVoiceListener(l *voice.Listener) {
	r.voiceListener = l
}

// SetUltraPlanner sets the ultraplan planner for /ultraplan support.
func (r *REPL) SetUltraPlanner(p *ultraplan.Planner) {
	r.ultraplanner = p
}

// SetDreamEngine sets the dream engine for idle/session-end memory consolidation.
func (r *REPL) SetDreamEngine(d *dream.Dreamer) {
	r.dreamEngine = d
}

// SetOutputStyle sets the active output style.
func (r *REPL) SetOutputStyle(style string) {
	r.outputStyle = style
}

// SetBuddy sets the terminal companion displayed in the banner.
func (r *REPL) SetBuddy(b *buddy.Companion) {
	r.buddy = b
}

// buddyLine returns a short summary string for the banner, or empty if no buddy is set.
func (r *REPL) buddyLine() string {
	if r.buddy == nil {
		return ""
	}
	return fmt.Sprintf("%s — %s (%s)", r.buddy.Name, r.buddy.Species, r.buddy.Rarity)
}

// Run starts the interactive REPL loop.
func (r *REPL) Run(ctx context.Context) error {
	scanner := bufio.NewScanner(r.reader)
	PrintBanner(r.writer, BannerConfig{
		Version:   r.config.Version,
		Model:     r.config.Model,
		MaxTurns:  r.config.MaxTurns,
		BuddyLine: r.buddyLine(),
	})

	for {
		fmt.Fprintf(r.writer, "%syou>%s ", cGreen+ansiBold, ansiReset)
		input, err := ReadInput(scanner)
		if err != nil {
			if err == io.EOF {
				fmt.Fprintln(r.writer, "\nGoodbye!")
				if r.dreamEngine != nil {
					r.dreamEngine.RunFinalConsolidation(ctx) //nolint:errcheck
				}
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
			if r.dreamEngine != nil {
				r.dreamEngine.RunFinalConsolidation(ctx) //nolint:errcheck
			}
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
			fmt.Fprintln(r.writer, "  /undo        Undo changes (checkpoints or stash)")
			fmt.Fprintln(r.writer, "  /undo N      Restore to checkpoint N")
			fmt.Fprintln(r.writer, "  /redo        Restore stashed changes")
			fmt.Fprintln(r.writer, "  /status      Show session stats")
			fmt.Fprintln(r.writer, "  /review      Ask agent to review changes")
			fmt.Fprintln(r.writer, "  /permissions Show permission mode")
			fmt.Fprintln(r.writer, "  /doctor      Check environment")
			fmt.Fprintln(r.writer, "  /connect     Show API key setup guide")
			fmt.Fprintln(r.writer, "  /share       Export session / create gist")
			fmt.Fprintln(r.writer, "  /commit      Auto-commit changes with generated message")
			fmt.Fprintln(r.writer, "  /memory      Manage persistent memories")
			fmt.Fprintln(r.writer, "  /tasks       Manage task list")
			fmt.Fprintln(r.writer, "  /worktree    Manage git worktrees")
			fmt.Fprintln(r.writer, "  /voice       Toggle voice input")
			fmt.Fprintln(r.writer, "  /ultraplan   Deep planning with powerful model")
			fmt.Fprintln(r.writer, "  /vim         Toggle vim keybindings")
			fmt.Fprintln(r.writer, "  /output-style Show or switch output style")
			// Append custom commands to help output.
			if cmds, err := r.customCmds.LoadAll(); err == nil && len(cmds) > 0 {
				fmt.Fprintln(r.writer, "\nCustom commands:")
				for _, cmd := range cmds {
					desc := cmd.Description
					if desc == "" {
						desc = "(no description)"
					}
					fmt.Fprintf(r.writer, "  /%-12s %s\n", cmd.Name, desc)
				}
			}
			continue
		case CmdModel:
			r.handleModelCommand(input)
			continue
		case CmdDiff:
			r.handleDiffCommand()
			continue
		case CmdUndo:
			r.handleUndoCommand(input)
			continue
		case CmdRedo:
			out, err := exec.Command("git", "stash", "list").Output()
			if err != nil || strings.TrimSpace(string(out)) == "" {
				fmt.Fprintln(r.writer, "Nothing to redo.")
				continue
			}
			if !strings.Contains(string(out), "gocode-undo") {
				fmt.Fprintln(r.writer, "No gocode undo stash found.")
				continue
			}
			if popErr := exec.Command("git", "stash", "pop").Run(); popErr != nil {
				fmt.Fprintf(r.writer, "Error restoring stash: %v\n", popErr)
			} else {
				fmt.Fprintln(r.writer, "Changes restored.")
			}
			continue
		case CmdConnect:
			fmt.Fprintln(r.writer, "Available providers:")
			fmt.Fprintln(r.writer, "  1. Anthropic (Claude)  — export ANTHROPIC_API_KEY=...")
			fmt.Fprintln(r.writer, "  2. OpenAI (GPT)        — export OPENAI_API_KEY=...")
			fmt.Fprintln(r.writer, "  3. Google (Gemini)     — export GEMINI_API_KEY=...")
			fmt.Fprintln(r.writer, "  4. xAI (Grok)          — export XAI_API_KEY=...")
			fmt.Fprintln(r.writer, "")
			fmt.Fprintln(r.writer, "Set your API key and restart gocode.")
			continue
		case CmdShare:
			data, err := json.MarshalIndent(r.runtime.GetSession(), "", "  ")
			if err != nil {
				fmt.Fprintf(r.writer, "Error exporting session: %v\n", err)
				continue
			}
			tmpFile := filepath.Join(os.TempDir(), "gocode-session.json")
			if writeErr := os.WriteFile(tmpFile, data, 0644); writeErr != nil {
				fmt.Fprintf(r.writer, "Error writing file: %v\n", writeErr)
				continue
			}
			if _, lookErr := exec.LookPath("gh"); lookErr == nil {
				out, ghErr := exec.Command("gh", "gist", "create", tmpFile, "--desc", "gocode session").Output()
				if ghErr == nil {
					fmt.Fprintf(r.writer, "Shared: %s\n", strings.TrimSpace(string(out)))
					continue
				}
			}
			fmt.Fprintf(r.writer, "Session exported to: %s\n", tmpFile)
			continue
		case CmdCommit:
			// Check both unstaged and staged changes
			stat, _ := exec.Command("git", "diff", "--stat").Output()
			cachedStat, _ := exec.Command("git", "diff", "--cached", "--stat").Output()
			combinedStat := strings.TrimSpace(string(stat)) + strings.TrimSpace(string(cachedStat))
			if combinedStat == "" {
				fmt.Fprintln(r.writer, "No changes to commit.")
				continue
			}
			exec.Command("git", "add", "-A").Run()
			// Re-read staged stat after add -A for the commit message
			finalStat, _ := exec.Command("git", "diff", "--cached", "--stat").Output()
			msg := "gocode: auto-commit"
			if lines := strings.Split(strings.TrimSpace(string(finalStat)), "\n"); len(lines) > 0 && lines[0] != "" {
				last := strings.TrimSpace(lines[len(lines)-1])
				msg = "gocode: " + last
			}
			msg += "\n\nCo-Authored-By: gocoder6969 <gocoder6969@users.noreply.github.com>"
			out, commitErr := exec.Command("git", "commit", "-m", msg).CombinedOutput()
			if commitErr != nil {
				fmt.Fprintf(r.writer, "Commit error: %v\n%s\n", commitErr, string(out))
			} else {
				fmt.Fprintln(r.writer, strings.TrimSpace(string(out)))
			}
			continue
		case CmdMemory:
			r.handleMemoryCommand(input)
			continue
		case CmdTasks:
			r.handleTasksCommand(input)
			continue
		case CmdWorktree:
			r.handleWorktreeCommand(input)
			continue
		case CmdVoice:
			r.handleVoiceCommand()
			continue
		case CmdVim:
			r.vimState.Enabled = !r.vimState.Enabled
			if r.vimState.Enabled {
				fmt.Fprintln(r.writer, "Vim mode: ON")
			} else {
				fmt.Fprintln(r.writer, "Vim mode: OFF")
			}
			continue
		case CmdOutputStyle:
			r.handleOutputStyleCommand(input)
			continue
		case CmdUltraPlan:
			taskDesc := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(input), "/ultraplan"))
			if taskDesc == "" {
				fmt.Fprintln(r.writer, "Usage: /ultraplan <task description>")
				continue
			}
			if r.ultraplanner == nil {
				fmt.Fprintln(r.writer, "ULTRAPLAN not configured. Ensure an ultrabrain provider is set up.")
				continue
			}
			fmt.Fprintln(r.writer, "Starting ULTRAPLAN deep planning session...")
			resultCh := r.ultraplanner.PlanBackground(ctx, taskDesc)
			go func() {
				result := <-resultCh
				if result.Err != nil {
					fmt.Fprintf(r.writer, "\n%s✗ ULTRAPLAN error: %v%s\n", cRed, result.Err, ansiReset)
				} else {
					fmt.Fprintf(r.writer, "\n%s✓ ULTRAPLAN complete:%s\n%s\n", cGreen, ansiReset, result.Output)
				}
			}()
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
			// Check OPENAI_BASE_URL connectivity
			if baseURL := os.Getenv("OPENAI_BASE_URL"); baseURL != "" {
				fmt.Fprintf(r.writer, "  ℹ OPENAI_BASE_URL: %s\n", baseURL)
				if _, _, err := apiclient.ResolveProvider(r.config.Model, ""); err == nil {
					fmt.Fprintf(r.writer, "  ✓ Provider resolved for model %s\n", r.config.Model)
				} else {
					fmt.Fprintf(r.writer, "  ✗ Provider error: %v\n", err)
				}
			}
			for _, name := range []string{"GOCODE.md", "CLAUDE.md"} {
				if _, err := os.Stat(name); err == nil {
					fmt.Fprintf(r.writer, "  ✓ %s: found\n", name)
				}
			}
			continue
		}

		// Try to resolve custom slash commands for unrecognized /commands.
		if strings.HasPrefix(input, "/") {
			parts := strings.Fields(input)
			cmdName := strings.TrimPrefix(parts[0], "/")
			args := parts[1:]
			if resolved, err := r.customCmds.Resolve(cmdName, args); err == nil {
				input = resolved
			} else if !strings.Contains(err.Error(), "unknown command") {
				// Known command but bad args — show error and re-prompt.
				fmt.Fprintf(r.writer, "Error: %v\n", err)
				continue
			}
			// If "unknown command", fall through — treat as normal input.
		}

		// Feature 4: @general subagent inline — rewrite prompt for orchestrator
		if strings.Contains(input, "@general") {
			input = strings.Replace(input, "@general", "", 1)
			input = "Use a sub-agent to handle this complex task: " + strings.TrimSpace(input)
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

		// Check for image paths in input
		var eventCh <-chan apitypes.StreamEvent
		var streamErr error
		imagePath := extractImagePath(input)
		if imagePath != "" {
			textPart := strings.Replace(input, imagePath, "", 1)
			textPart = strings.TrimSpace(textPart)
			if textPart == "" {
				textPart = "What's in this image?"
			}
			imgMsg, imgErr := apitypes.UserImageAndText(textPart, imagePath)
			if imgErr != nil {
				spin.Stop()
				cancel()
				fmt.Fprintf(r.writer, "Error loading image: %v\n", imgErr)
				continue
			}
			eventCh, streamErr = r.runtime.StreamWithMessage(msgCtx, imgMsg)
		} else {
			eventCh, streamErr = r.runtime.StreamUserMessage(msgCtx, input)
		}
		if streamErr != nil {
			spin.Stop()
			cancel()
			r.display.Error(streamErr)
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

		// Context window management: check if session is approaching the limit
		estimatedTokens := r.runtime.EstimateSessionTokens()
		contextLimit := apiclient.ContextWindowForModel(r.config.Model)
		threshold := int(float64(contextLimit) * 0.85) // 85% threshold

		if estimatedTokens > threshold {
			fmt.Fprintf(r.writer, "\n%s⚠ Context window %.0f%% full (%dk / %dk tokens)%s\n",
				cYellow, float64(estimatedTokens)/float64(contextLimit)*100,
				estimatedTokens/1000, contextLimit/1000, ansiReset)
			fmt.Fprintf(r.writer, "%sSummarizing conversation and starting fresh session...%s\n", cCyan, ansiReset)

			summary, sumErr := r.runtime.SummarizeAndReset(ctx)
			if sumErr != nil {
				fmt.Fprintf(r.writer, "%sError summarizing: %v. Use /clear to reset manually.%s\n", cRed, sumErr, ansiReset)
			} else {
				fmt.Fprintf(r.writer, "%s✓ New session started with summarized context.%s\n", cGreen, ansiReset)
				if summary != "" {
					fmt.Fprintf(r.writer, "%sSummary: %s%s\n\n", cGray, summary, ansiReset)
				}
			}
		}

		// Reset dream idle timer after each user interaction.
		if r.dreamEngine != nil {
			r.dreamEngine.ResetIdleTimer()
		}
	}
}

// extractImagePath finds the first word in input that looks like an image file path.
func extractImagePath(input string) string {
	imageExts := []string{".png", ".jpg", ".jpeg", ".gif", ".webp"}
	words := strings.Fields(input)
	for _, word := range words {
		lower := strings.ToLower(word)
		for _, ext := range imageExts {
			if strings.HasSuffix(lower, ext) {
				// Check if file exists
				if _, err := os.Stat(word); err == nil {
					return word
				}
			}
		}
	}
	return ""
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

// handleMemoryCommand processes the /memory slash command.
func (r *REPL) handleMemoryCommand(input string) {
	store := memory.NewStore(filepath.Join(".gocode", "memory.json"))
	_ = store.Load()
	arg := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(input), "/memory"))
	if arg == "" {
		mems := store.All()
		if len(mems) == 0 {
			fmt.Fprintln(r.writer, "No memories stored.")
			return
		}
		for _, m := range mems {
			fmt.Fprintf(r.writer, "  %s: %s\n", m.Key, m.Value)
		}
		return
	}
	if strings.HasPrefix(arg, "set ") {
		parts := strings.SplitN(strings.TrimPrefix(arg, "set "), " ", 2)
		if len(parts) < 2 {
			fmt.Fprintln(r.writer, "Usage: /memory set <key> <value>")
			return
		}
		store.Set(parts[0], parts[1])
		_ = store.Save()
		fmt.Fprintf(r.writer, "Memory set: %s = %s\n", parts[0], parts[1])
		return
	}
	if strings.HasPrefix(arg, "delete ") {
		key := strings.TrimSpace(strings.TrimPrefix(arg, "delete "))
		store.Delete(key)
		_ = store.Save()
		fmt.Fprintf(r.writer, "Memory deleted: %s\n", key)
		return
	}
	fmt.Fprintln(r.writer, "Usage: /memory [set <key> <value> | delete <key>]")
}

// handleTasksCommand processes the /tasks slash command.
func (r *REPL) handleTasksCommand(input string) {
	store := tasks.NewStore(filepath.Join(".gocode", "tasks.json"))
	_ = store.Load()
	arg := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(input), "/tasks"))
	if arg == "" {
		fmt.Fprint(r.writer, store.Render())
		return
	}
	if strings.HasPrefix(arg, "add ") {
		text := strings.TrimSpace(strings.TrimPrefix(arg, "add "))
		t := store.Add(text)
		_ = store.Save()
		fmt.Fprintf(r.writer, "Added task #%d: %s\n", t.ID, t.Text)
		return
	}
	if strings.HasPrefix(arg, "done ") {
		idStr := strings.TrimSpace(strings.TrimPrefix(arg, "done "))
		var id int
		if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
			fmt.Fprintln(r.writer, "Usage: /tasks done <id>")
			return
		}
		if err := store.Complete(id); err != nil {
			fmt.Fprintf(r.writer, "Error: %v\n", err)
			return
		}
		_ = store.Save()
		fmt.Fprintf(r.writer, "Task #%d marked complete.\n", id)
		return
	}
	fmt.Fprintln(r.writer, "Usage: /tasks [add <text> | done <id>]")
}

// handleWorktreeCommand processes the /worktree slash command.
func (r *REPL) handleWorktreeCommand(input string) {
	if r.worktreeMgr == nil {
		// Try to auto-detect repo root for a manager.
		ctx, err := worktree.Detect()
		if err != nil {
			fmt.Fprintln(r.writer, "Not in a git repository.")
			return
		}
		r.worktreeMgr = worktree.NewManager(ctx.RepoRoot)
	}

	arg := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(input), "/worktree"))
	if arg == "" {
		fmt.Fprintln(r.writer, "Usage: /worktree create <branch> | /worktree list")
		return
	}

	if strings.HasPrefix(arg, "create ") {
		branch := strings.TrimSpace(strings.TrimPrefix(arg, "create "))
		if branch == "" {
			fmt.Fprintln(r.writer, "Usage: /worktree create <branch>")
			return
		}
		path := branch // default worktree path is the branch name
		if err := r.worktreeMgr.Create(branch, path); err != nil {
			fmt.Fprintf(r.writer, "Error creating worktree: %v\n", err)
			return
		}
		fmt.Fprintf(r.writer, "Created worktree for branch %s\n", branch)
		return
	}

	if arg == "list" {
		wts, err := r.worktreeMgr.List()
		if err != nil {
			fmt.Fprintf(r.writer, "Error listing worktrees: %v\n", err)
			return
		}
		if len(wts) == 0 {
			fmt.Fprintln(r.writer, "No worktrees found.")
			return
		}
		fmt.Fprintln(r.writer, "Active worktrees:")
		for _, wt := range wts {
			branch := wt.Branch
			if branch == "" {
				branch = "(detached)"
			}
			fmt.Fprintf(r.writer, "  %s  %s  %s\n", wt.Path, branch, wt.HEAD[:min(7, len(wt.HEAD))])
		}
		return
	}

	fmt.Fprintln(r.writer, "Usage: /worktree create <branch> | /worktree list")
}

// handleVoiceCommand toggles voice input mode.
func (r *REPL) handleVoiceCommand() {
	if r.voiceListener == nil {
		fmt.Fprintln(r.writer, "Voice input is not configured. No STT engine available.")
		return
	}

	active, err := r.voiceListener.Toggle()
	if err != nil {
		fmt.Fprintf(r.writer, "Voice error: %v\n", err)
		return
	}

	if active {
		fmt.Fprintln(r.writer, "🎤 Listening...")
	} else {
		fmt.Fprintln(r.writer, "Voice input disabled.")
	}
}

// handleOutputStyleCommand processes the /output-style slash command.
func (r *REPL) handleOutputStyleCommand(input string) {
	arg := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(input), "/output-style"))
	reg := outputstyles.NewRegistry()

	if arg == "" {
		fmt.Fprintf(r.writer, "Current output style: %s%s%s\n", cGreen+ansiBold, r.outputStyle, ansiReset)
		return
	}

	if reg.Get(arg) == nil {
		names := reg.List()
		fmt.Fprintf(r.writer, "Unknown style: %s. Available styles: %s\n", arg, strings.Join(names, ", "))
		return
	}

	r.outputStyle = arg
	fmt.Fprintf(r.writer, "Output style set to: %s%s%s\n", cGreen+ansiBold, r.outputStyle, ansiReset)
}

// handleUndoCommand processes the /undo slash command.
// With a checkpoint manager: /undo lists checkpoints, /undo N restores to checkpoint N.
// Without a checkpoint manager: falls back to git stash behavior.
func (r *REPL) handleUndoCommand(input string) {
	arg := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(input), "/undo"))

	// If no checkpoint manager, fall back to stash-based undo
	if r.checkpointMgr == nil {
		r.undoViaStash()
		return
	}

	if arg == "" {
		// List checkpoints
		checkpoints, err := r.checkpointMgr.List()
		if err != nil {
			fmt.Fprintf(r.writer, "Error listing checkpoints: %v\n", err)
			return
		}
		if len(checkpoints) == 0 {
			fmt.Fprintln(r.writer, "No checkpoints in this session.")
			return
		}
		fmt.Fprintln(r.writer, "Checkpoints:")
		for _, cp := range checkpoints {
			ts := cp.Timestamp.Format("15:04:05")
			fmt.Fprintf(r.writer, "  %d. [%s] %s\n", cp.ID, ts, cp.Message)
		}
		fmt.Fprintln(r.writer, "\nUse /undo N to restore to checkpoint N.")
		return
	}

	// Parse checkpoint ID
	id, err := strconv.Atoi(arg)
	if err != nil {
		fmt.Fprintf(r.writer, "Invalid checkpoint ID: %s\n", arg)
		return
	}

	if err := r.checkpointMgr.Restore(id); err != nil {
		fmt.Fprintf(r.writer, "Error restoring checkpoint: %v\n", err)
		return
	}
	fmt.Fprintf(r.writer, "Restored to checkpoint %d.\n", id)
}

// undoViaStash is the legacy stash-based undo fallback.
func (r *REPL) undoViaStash() {
	out, err := exec.Command("git", "diff", "--stat").Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		fmt.Fprintln(r.writer, "Nothing to undo.")
		return
	}
	fmt.Fprintf(r.writer, "Stashing:\n%s", string(out))
	if stashErr := exec.Command("git", "stash", "push", "-m", "gocode-undo").Run(); stashErr != nil {
		fmt.Fprintf(r.writer, "Error stashing changes: %v\n", stashErr)
	} else {
		fmt.Fprintln(r.writer, "Changes stashed (use /redo to restore).")
	}
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
	if input == nil || len(input) == 0 {
		return ""
	}
	// Show the most relevant param based on tool type
	for _, key := range []string{"command", "path", "pattern", "query", "url", "file", "name", "agent_name", "task"} {
		if v, ok := input[key]; ok {
			s := fmt.Sprintf("%v", v)
			if len(s) > 80 {
				s = s[:77] + "..."
			}
			return cGray + key + "=" + s + ansiReset
		}
	}
	// Fallback: show first key=value
	for k, v := range input {
		s := fmt.Sprintf("%v", v)
		if len(s) > 60 {
			s = s[:57] + "..."
		}
		return cGray + k + "=" + s + ansiReset
	}
	return ""
}

// TerminalPermissionPrompter prompts the user in the terminal for permission.
type TerminalPermissionPrompter struct {
	Scanner *bufio.Scanner
	Writer  io.Writer
	Trusted *agent.TrustedToolStore
}

// Prompt asks the user for permission, showing the tool name and params.
// Options: y=yes once, n=deny, a=always trust this tool, t=trust tool:command prefix
func (p *TerminalPermissionPrompter) Prompt(toolName string, operation string) (bool, error) {
	summary := summarizeJSON(operation)
	display := NewDisplay(p.Writer)
	display.PermissionPromptExtended(toolName, summary)
	if !p.Scanner.Scan() {
		return false, nil
	}
	answer := strings.TrimSpace(strings.ToLower(p.Scanner.Text()))
	switch answer {
	case "y", "yes":
		return true, nil
	case "a", "always":
		// Trust this tool with any params
		if p.Trusted != nil {
			p.Trusted.Add(toolName)
			_ = p.Trusted.Save()
			fmt.Fprintf(p.Writer, "  ✓ Trusted %s (all future calls auto-approved)\n", toolName)
		}
		return true, nil
	case "t", "trust":
		// Trust tool with wildcard command prefix
		if p.Trusted != nil {
			// Extract a meaningful prefix from the operation
			prefix := extractCommandPrefix(toolName, operation)
			if prefix != "" {
				pattern := toolName + ":" + prefix + " *"
				p.Trusted.Add(pattern)
				_ = p.Trusted.Save()
				fmt.Fprintf(p.Writer, "  ✓ Trusted %s (auto-approved)\n", pattern)
			} else {
				p.Trusted.Add(toolName)
				_ = p.Trusted.Save()
				fmt.Fprintf(p.Writer, "  ✓ Trusted %s (all future calls auto-approved)\n", toolName)
			}
		}
		return true, nil
	default:
		return false, nil
	}
}

// extractCommandPrefix pulls a meaningful prefix from tool params for wildcard trust.
func extractCommandPrefix(toolName string, operation string) string {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(operation), &m); err != nil {
		return ""
	}
	// For BashTool, extract the first word of the command
	if cmd, ok := m["command"].(string); ok {
		parts := strings.Fields(cmd)
		if len(parts) > 0 {
			return parts[0]
		}
	}
	// For GrepTool/GlobTool, use the pattern
	if pat, ok := m["pattern"].(string); ok {
		return pat
	}
	// For file tools, use the path directory
	if path, ok := m["path"].(string); ok {
		dir := filepath.Dir(path)
		if dir != "." && dir != "" {
			return dir
		}
	}
	return ""
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
	currentDate := time.Now().Format("January 2, 2006")

	var toolList strings.Builder
	for _, t := range tools {
		toolList.WriteString(fmt.Sprintf("- %s: %s\n", t.Name, t.Description))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`You are gocode, an interactive CLI tool that helps users with software engineering tasks. Use the instructions below and the tools available to you to assist the user.

IMPORTANT: You should be proactive in accomplishing the task, not reactive. Do not wait for the user to ask you to do something that you can anticipate.

# Language Rules

- ALL tool calls, search queries, and internal reasoning MUST be in English. Always translate the user's intent to English before calling any tool.
- Respond to the user in whatever language they use. If they write in Indonesian, respond in Indonesian. If English, respond in English.
- When using WebSearchTool, ALWAYS write the query in English. Example: user says "siapa presiden indonesia" → search for "president of Indonesia 2026".
- When reading tool results, extract ALL relevant information. Do not ignore parts of the results. Read every line carefully.

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
- When the user asks you to "search" or "look up" something, use WebSearchTool with a clear query in English. If the user doesn't specify a query, infer it from the conversation context. NEVER call WebSearchTool with an empty query.
- WebSearchTool searches Wikipedia, GitHub, Reddit, Hacker News, and StackOverflow in parallel. Use it for current events, technical questions, people, projects, or any factual lookup.
- When the user asks a follow-up like "search for that" or "look it up", construct the query from what was just discussed. Always provide the query parameter in English.

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
- Current date: %s

# CRITICAL: Tool Results Override Training Data

Your training data has a knowledge cutoff. The current date is %s. When you use WebSearchTool and receive results, you MUST use those results as the source of truth. NEVER override search results with your training data. If search results say something different from what you "know", the search results are correct because they are current.

When WebSearchTool returns results, read EVERY line carefully. The answer is often in the Wikipedia summary or in the related articles section. Do not say "information not found" if the search results contain the answer — even if it's in a related article or a different section.

If the first search doesn't give a clear answer, you MUST search again with a different, more specific query. Do not give up after one search. Try at least 2-3 different queries before saying you can't find the answer.

# Available Tools

%s`, cwd, osName, shell, currentDate, currentDate, toolList.String()))

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

	// Inject persistent memories
	memStore := memory.NewStore(filepath.Join(".gocode", "memory.json"))
	_ = memStore.Load()
	if mems := memStore.Render(); mems != "" {
		sb.WriteString("\n\n# Persistent Memory\n\n" + mems)
	}

	return sb.String()
}
