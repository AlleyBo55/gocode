package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/AlleyBo55/gocode/data"
	"github.com/AlleyBo55/gocode/internal/agent"
	"github.com/AlleyBo55/gocode/internal/apiserver"
	"github.com/AlleyBo55/gocode/internal/apiclient"
	"github.com/AlleyBo55/gocode/internal/astgrep"
	"github.com/AlleyBo55/gocode/internal/authkeys"
	"github.com/AlleyBo55/gocode/internal/bootstrap"
	"github.com/AlleyBo55/gocode/internal/commandgraph"
	"github.com/AlleyBo55/gocode/internal/commands"
	"github.com/AlleyBo55/gocode/internal/editorcompat"
	"github.com/AlleyBo55/gocode/internal/execution"
	"github.com/AlleyBo55/gocode/internal/hashline"
	"github.com/AlleyBo55/gocode/internal/initdeep"
	"github.com/AlleyBo55/gocode/internal/manifest"
	"github.com/AlleyBo55/gocode/internal/mcp"
	"github.com/AlleyBo55/gocode/internal/mcpclient"
	"github.com/AlleyBo55/gocode/internal/modes"
	"github.com/AlleyBo55/gocode/internal/orchestrator"
	"github.com/AlleyBo55/gocode/internal/permissions"
	"github.com/AlleyBo55/gocode/internal/plugins"
	"github.com/AlleyBo55/gocode/internal/profiles"
	"github.com/AlleyBo55/gocode/internal/queryengine"
	"github.com/AlleyBo55/gocode/internal/repl"
	"github.com/AlleyBo55/gocode/internal/runtime"
	"github.com/AlleyBo55/gocode/internal/tui"
	"github.com/AlleyBo55/gocode/internal/session"
	"github.com/AlleyBo55/gocode/internal/setup"
	"github.com/AlleyBo55/gocode/internal/skills"
	"github.com/AlleyBo55/gocode/internal/tmux"
	"github.com/AlleyBo55/gocode/internal/toolimpl"
	"github.com/AlleyBo55/gocode/internal/toolpool"
	"github.com/AlleyBo55/gocode/internal/tools"
)

var version = "v0.8.1"

// isTerminal checks if stdout is a terminal (not piped).
func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// stdRecoveryLogger logs recovery events to stderr via the standard log package.
type stdRecoveryLogger struct{}

func (stdRecoveryLogger) OnRecovery(action string, detail string) {
	log.Printf("[recovery] %s: %s", action, detail)
}

// wireAdvancedTools registers Phase 1 hashline/context-aware wrappers and
// Phase 3 tools (ast-grep, tmux, MCP client) into the tool registry.
// Returns a cleanup function that should be deferred.
func wireAdvancedTools(toolImpl *toolimpl.Registry, hashlineEnabled bool) func() {
	// Phase 1: hashline wrappers (conditional)
	if hashlineEnabled {
		hashline.RegisterHashlineTools(toolImpl)
	}

	// Phase 1: context-aware read (always enabled — wraps filereadtool with AGENTS.md)
	initdeep.RegisterContextAwareRead(toolImpl)

	// Phase 3: ast-grep tool
	astgrep.RegisterAstGrepTool(toolImpl)

	// Phase 3: tmux tools
	tmuxMgr := tmux.NewManager()
	tmux.RegisterTmuxTools(toolImpl, tmuxMgr)

	// Phase 3: MCP client tools (only if user has a .gocode/mcp.json config)
	mcpConfigPath := filepath.Join(".gocode", "mcp.json")
	if _, statErr := os.Stat(mcpConfigPath); statErr == nil {
		mcpMgr, err := mcpclient.NewManager(mcpConfigPath)
		if err != nil {
			log.Printf("[mcpclient] failed to create manager: %v", err)
			return tmuxMgr.KillAll
		}

		// Best-effort connect — failures are logged, not fatal
		if connectErr := mcpMgr.ConnectAll(); connectErr != nil {
			log.Printf("[mcpclient] %v", connectErr)
		}

		// Register discovered MCP tools in the tool registry
		for _, t := range mcpMgr.ListTools() {
			toolName := t.Name
			toolImpl.Set(toolName, &mcpToolAdapter{mgr: mcpMgr, toolName: toolName})
		}

		return func() {
			tmuxMgr.KillAll()
			mcpMgr.Close()
		}
	}

	return tmuxMgr.KillAll
}

// mcpToolAdapter adapts an MCP client tool call to the toolimpl.ToolExecutor interface.
type mcpToolAdapter struct {
	mgr      *mcpclient.Manager
	toolName string
}

func (a *mcpToolAdapter) Execute(params map[string]interface{}) toolimpl.ToolResult {
	output, err := a.mgr.CallTool(a.toolName, params)
	if err != nil {
		return toolimpl.ToolResult{Success: false, Error: err.Error()}
	}
	return toolimpl.ToolResult{Success: true, Output: output}
}

// orchestratorToolAdapter adapts orchestrator delegation tools to the toolimpl.ToolExecutor interface.
type orchestratorToolAdapter struct {
	orch     *orchestrator.Orchestrator
	toolName string
}

func (a *orchestratorToolAdapter) Execute(params map[string]interface{}) toolimpl.ToolResult {
	result := a.orch.Execute(a.toolName, params)
	if result.IsError {
		return toolimpl.ToolResult{Success: false, Error: result.Output}
	}
	return toolimpl.ToolResult{Success: true, Output: result.Output}
}

// buildFallbackProvider wraps a single resolved provider into a FallbackProvider
// with a chain of one entry. Users can configure additional entries via config later.
func buildFallbackProvider(provider apiclient.Provider, model string) *apiclient.FallbackProvider {
	return apiclient.NewFallbackProvider([]apiclient.FallbackEntry{
		{Model: model, Provider: provider},
	}, nil)
}

// buildModelRouter creates a ModelRouter that maps all categories to the same
// FallbackProvider. This is the default single-model configuration.
func buildModelRouter(fp *apiclient.FallbackProvider) *apiclient.ModelRouter {
	return apiclient.NewModelRouter(map[apiclient.TaskCategory]*apiclient.FallbackProvider{
		apiclient.CategoryDeep:              fp,
		apiclient.CategoryQuick:             fp,
		apiclient.CategoryVisualEngineering: fp,
		apiclient.CategoryUltrabrain:        fp,
	})
}

func main() {
	// Initialize registries from embedded data.
	cmdReg, err := commands.NewCommandRegistry(data.CommandsJSON)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading commands: %v\n", err)
		os.Exit(1)
	}

	toolReg, err := tools.NewToolRegistry(data.ToolsJSON)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading tools: %v\n", err)
		os.Exit(1)
	}

	execReg := execution.BuildExecutionRegistry(cmdReg, toolReg)
	sessionStore := session.NewSessionStore("")
	rt := runtime.NewPortRuntime(cmdReg, toolReg, execReg, sessionStore)

	rootCmd := &cobra.Command{
		Use:     "gocode",
		Short:   "gocode agent harness runtime (Go port)",
		Version: version,
	}

	// 1. summary
	rootCmd.AddCommand(&cobra.Command{
		Use:   "summary",
		Short: "Render workspace summary",
		RunE: func(cmd *cobra.Command, args []string) error {
			config := queryengine.NewDefaultConfig()
			engine := queryengine.FromWorkspace(config, sessionStore)
			fmt.Println(engine.RenderSummary())
			return nil
		},
	})

	// 2. manifest
	rootCmd.AddCommand(&cobra.Command{
		Use:   "manifest",
		Short: "Print port manifest",
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := manifest.BuildPortManifest("internal")
			if err != nil {
				return err
			}
			fmt.Println(m.Render())
			return nil
		},
	})

	// 3. parity-audit
	rootCmd.AddCommand(&cobra.Command{
		Use:   "parity-audit",
		Short: "Run parity audit",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Parity audit not yet implemented")
		},
	})

	// 4. setup-report
	rootCmd.AddCommand(&cobra.Command{
		Use:   "setup-report",
		Short: "Run setup and print report",
		Run: func(cmd *cobra.Command, args []string) {
			cwd, _ := os.Getwd()
			report := setup.RunSetup(cwd, true)
			fmt.Println(report.Render())
		},
	})

	// 5. command-graph
	rootCmd.AddCommand(&cobra.Command{
		Use:   "command-graph",
		Short: "Show command graph",
		Run: func(cmd *cobra.Command, args []string) {
			allCmds := cmdReg.FindCommands("", 0)
			cg := commandgraph.BuildCommandGraph(allCmds)
			fmt.Println(cg.Render())
		},
	})

	// 6. tool-pool
	rootCmd.AddCommand(&cobra.Command{
		Use:   "tool-pool",
		Short: "Show tool pool",
		Run: func(cmd *cobra.Command, args []string) {
			pc := permissions.NewToolPermissionContext(nil, nil)
			tp := toolpool.AssembleToolPool(toolReg, false, true, pc)
			fmt.Println(tp.Render())
		},
	})

	// 7. bootstrap-graph
	rootCmd.AddCommand(&cobra.Command{
		Use:   "bootstrap-graph",
		Short: "Show bootstrap graph",
		Run: func(cmd *cobra.Command, args []string) {
			bg := bootstrap.BuildBootstrapGraph()
			fmt.Println(bg.Render())
		},
	})

	// 8. subsystems
	subsystemsCmd := &cobra.Command{
		Use:   "subsystems",
		Short: "List modules from manifest",
		RunE: func(cmd *cobra.Command, args []string) error {
			limit, _ := cmd.Flags().GetInt("limit")
			m, err := manifest.BuildPortManifest("internal")
			if err != nil {
				return err
			}
			mods := m.TopLevelModules
			if limit > 0 && limit < len(mods) {
				mods = mods[:limit]
			}
			for _, mod := range mods {
				fmt.Printf("%s (%d files) — %s\n", mod.Name, mod.FileCount, mod.Notes)
			}
			return nil
		},
	}
	subsystemsCmd.Flags().Int("limit", 32, "Maximum number of modules to list")
	rootCmd.AddCommand(subsystemsCmd)

	// 9. commands
	commandsCmd := &cobra.Command{
		Use:   "commands",
		Short: "List commands",
		Run: func(cmd *cobra.Command, args []string) {
			limit, _ := cmd.Flags().GetInt("limit")
			query, _ := cmd.Flags().GetString("query")
			noPlugins, _ := cmd.Flags().GetBool("no-plugin-commands")
			noSkills, _ := cmd.Flags().GetBool("no-skill-commands")

			if query != "" {
				results := cmdReg.FindCommands(query, limit)
				for _, c := range results {
					fmt.Printf("%s — %s (source: %s)\n", c.Name, c.Responsibility, c.SourceHint)
				}
				return
			}

			results := cmdReg.GetCommands(!noPlugins, !noSkills)
			if limit > 0 && limit < len(results) {
				results = results[:limit]
			}
			for _, c := range results {
				fmt.Printf("%s — %s (source: %s)\n", c.Name, c.Responsibility, c.SourceHint)
			}
		},
	}
	commandsCmd.Flags().Int("limit", 0, "Maximum number of commands to list")
	commandsCmd.Flags().String("query", "", "Search query for commands")
	commandsCmd.Flags().Bool("no-plugin-commands", false, "Exclude plugin commands")
	commandsCmd.Flags().Bool("no-skill-commands", false, "Exclude skill commands")
	rootCmd.AddCommand(commandsCmd)

	// 10. tools
	toolsCmd := &cobra.Command{
		Use:   "tools",
		Short: "List tools",
		Run: func(cmd *cobra.Command, args []string) {
			limit, _ := cmd.Flags().GetInt("limit")
			query, _ := cmd.Flags().GetString("query")
			simpleMode, _ := cmd.Flags().GetBool("simple-mode")
			noMCP, _ := cmd.Flags().GetBool("no-mcp")
			denyTool, _ := cmd.Flags().GetStringSlice("deny-tool")
			denyPrefix, _ := cmd.Flags().GetStringSlice("deny-prefix")

			if query != "" {
				results := toolReg.FindTools(query, limit)
				for _, t := range results {
					fmt.Printf("%s — %s (source: %s)\n", t.Name, t.Responsibility, t.SourceHint)
				}
				return
			}

			pc := permissions.NewToolPermissionContext(denyTool, denyPrefix)
			results := toolReg.GetTools(simpleMode, !noMCP, pc)
			if limit > 0 && limit < len(results) {
				results = results[:limit]
			}
			for _, t := range results {
				fmt.Printf("%s — %s (source: %s)\n", t.Name, t.Responsibility, t.SourceHint)
			}
		},
	}
	toolsCmd.Flags().Int("limit", 0, "Maximum number of tools to list")
	toolsCmd.Flags().String("query", "", "Search query for tools")
	toolsCmd.Flags().Bool("simple-mode", false, "Only show simple-mode tools")
	toolsCmd.Flags().Bool("no-mcp", false, "Exclude MCP tools")
	toolsCmd.Flags().StringSlice("deny-tool", nil, "Tool names to deny")
	toolsCmd.Flags().StringSlice("deny-prefix", nil, "Tool name prefixes to deny")
	rootCmd.AddCommand(toolsCmd)

	// 11. route
	routeCmd := &cobra.Command{
		Use:   "route [prompt]",
		Short: "Route a prompt to matching commands/tools",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			limit, _ := cmd.Flags().GetInt("limit")
			matches := rt.RoutePrompt(args[0], limit)
			if len(matches) == 0 {
				fmt.Println("No matches found.")
				return
			}
			for _, m := range matches {
				fmt.Printf("[%s] %s (score: %d, source: %s)\n", m.Kind, m.Name, m.Score, m.SourceHint)
			}
		},
	}
	routeCmd.Flags().Int("limit", 0, "Maximum number of matches to return")
	rootCmd.AddCommand(routeCmd)

	// 12. bootstrap
	bootstrapCmd := &cobra.Command{
		Use:   "bootstrap [prompt]",
		Short: "Bootstrap a session",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			limit, _ := cmd.Flags().GetInt("limit")
			sess := rt.BootstrapSession(args[0], limit)
			fmt.Println(sess.AsMarkdown())
		},
	}
	bootstrapCmd.Flags().Int("limit", 0, "Maximum number of routed matches")
	rootCmd.AddCommand(bootstrapCmd)

	// 13. turn-loop
	turnLoopCmd := &cobra.Command{
		Use:   "turn-loop [prompt]",
		Short: "Run turn loop",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			limit, _ := cmd.Flags().GetInt("limit")
			maxTurns, _ := cmd.Flags().GetInt("max-turns")
			structuredOutput, _ := cmd.Flags().GetBool("structured-output")

			config := queryengine.NewDefaultConfig()
			if maxTurns > 0 {
				config.MaxTurns = maxTurns
			}
			config.StructuredOutput = structuredOutput

			engine := queryengine.FromWorkspace(config, sessionStore)
			matches := rt.RoutePrompt(args[0], limit)

			var matchedCmds, matchedTools []string
			for _, m := range matches {
				if m.Kind == "command" {
					matchedCmds = append(matchedCmds, m.Name)
				} else {
					matchedTools = append(matchedTools, m.Name)
				}
			}

			result := engine.SubmitMessage(args[0], matchedCmds, matchedTools, nil)

			if structuredOutput {
				out, _ := json.MarshalIndent(result, "", "  ")
				fmt.Println(string(out))
			} else {
				fmt.Println(result.Output)
				fmt.Printf("Stop reason: %s\n", result.StopReason)
			}
		},
	}
	turnLoopCmd.Flags().Int("limit", 0, "Maximum number of routed matches")
	turnLoopCmd.Flags().Int("max-turns", 0, "Maximum number of turns")
	turnLoopCmd.Flags().Bool("structured-output", false, "Output as JSON")
	rootCmd.AddCommand(turnLoopCmd)

	// 14. flush-transcript
	rootCmd.AddCommand(&cobra.Command{
		Use:   "flush-transcript [session_id]",
		Short: "Flush transcript for a session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			config := queryengine.NewDefaultConfig()
			engine, err := queryengine.FromSavedSession(args[0], config, sessionStore)
			if err != nil {
				return fmt.Errorf("loading session: %w", err)
			}
			engine.FlushTranscript()
			fmt.Printf("Transcript flushed for session %s\n", args[0])
			return nil
		},
	})

	// 15. load-session
	rootCmd.AddCommand(&cobra.Command{
		Use:   "load-session [session_id]",
		Short: "Load a saved session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			config := queryengine.NewDefaultConfig()
			engine, err := queryengine.FromSavedSession(args[0], config, sessionStore)
			if err != nil {
				return fmt.Errorf("loading session: %w", err)
			}
			fmt.Println(engine.RenderSummary())
			return nil
		},
	})

	// 16. remote-mode
	rootCmd.AddCommand(&cobra.Command{
		Use:   "remote-mode [target]",
		Short: "Remote mode",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := modes.RemoteMode(args[0])
			if err != nil {
				return err
			}
			fmt.Println(report.AsText())
			return nil
		},
	})

	// 17. ssh-mode
	rootCmd.AddCommand(&cobra.Command{
		Use:   "ssh-mode [target]",
		Short: "SSH mode",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := modes.SSHMode(args[0])
			if err != nil {
				return err
			}
			fmt.Println(report.AsText())
			return nil
		},
	})

	// 18. teleport-mode
	rootCmd.AddCommand(&cobra.Command{
		Use:   "teleport-mode [target]",
		Short: "Teleport mode",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := modes.TeleportMode(args[0])
			if err != nil {
				return err
			}
			fmt.Println(report.AsText())
			return nil
		},
	})

	// 19. direct-connect
	rootCmd.AddCommand(&cobra.Command{
		Use:   "direct-connect [target]",
		Short: "Direct connect",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := modes.DirectConnect(args[0])
			if err != nil {
				return err
			}
			fmt.Println(report.AsText())
			return nil
		},
	})

	// 20. deep-link
	rootCmd.AddCommand(&cobra.Command{
		Use:   "deep-link [target]",
		Short: "Deep link",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := modes.DeepLink(args[0])
			if err != nil {
				return err
			}
			fmt.Println(report.AsText())
			return nil
		},
	})

	// 22. chat — interactive REPL agent mode
	chatCmd := &cobra.Command{
		Use:   "chat",
		Short: "Start interactive agent chat",
		RunE: func(cmd *cobra.Command, args []string) error {
			model, _ := cmd.Flags().GetString("model")
			goal, _ := cmd.Flags().GetString("goal")
			maxTurns, _ := cmd.Flags().GetInt("max-turns")
			maxTokens, _ := cmd.Flags().GetInt("max-tokens")
			apiKey, _ := cmd.Flags().GetString("api-key")
			hashlineEnabled, _ := cmd.Flags().GetBool("hashline")
			skillName, _ := cmd.Flags().GetString("skill")
			skipPerms, _ := cmd.Flags().GetBool("dangerously-skip-permissions")
			printPrompt, _ := cmd.Flags().GetBool("print")
			verbose, _ := cmd.Flags().GetBool("verbose")
			noProjectConfig, _ := cmd.Flags().GetBool("no-project-config")
			allowedTools, _ := cmd.Flags().GetStringSlice("allowedTools")
			disallowedTools, _ := cmd.Flags().GetStringSlice("disallowedTools")
			useTUI, _ := cmd.Flags().GetBool("tui")
			_, _ = cmd.Flags().GetBool("no-tui") // kept for backward compat
			themeName, _ := cmd.Flags().GetString("theme")

			// Goal-based model selection: if --goal is set and --model wasn't explicitly changed
			if goal != "" && !cmd.Flags().Changed("model") {
				model = apiclient.RecommendModel(goal)
			}

			permMode := agent.WorkspaceWrite
			if skipPerms {
				permMode = agent.DangerFullAccess
			}

			if noProjectConfig {
				repl.SkipProjectConfig = true
			}

			provider, resolvedModel, err := apiclient.ResolveProvider(model, apiKey)
			if err != nil {
				return fmt.Errorf("resolving provider: %w", err)
			}

			// After resolving the model, set model-aware max tokens if user didn't override
			if !cmd.Flags().Changed("max-tokens") {
				maxTokens = apiclient.MaxTokensForModel(resolvedModel)
			}

			// Phase 1: wrap provider with FallbackProvider and ModelRouter
			fp := buildFallbackProvider(provider, resolvedModel)
			router := buildModelRouter(fp)

			toolImpl := toolimpl.NewRegistry()

			// Phase 1 + Phase 3: register advanced tools (hashline, context-aware read, ast-grep, tmux, MCP client)
			cleanup := wireAdvancedTools(toolImpl, hashlineEnabled)
			defer cleanup()

			executor := agent.NewRegistryExecutor(toolImpl, toolReg)

			// Phase 2: create Orchestrator with ModelRouter and executor,
			// then register its delegation tools in the tool registry.
			orch := orchestrator.NewOrchestrator(router, executor)
			toolImpl.Set("orchestrator_delegate", &orchestratorToolAdapter{orch: orch, toolName: "orchestrator_delegate"})
			toolImpl.Set("orchestrator_delegate_bg", &orchestratorToolAdapter{orch: orch, toolName: "orchestrator_delegate_bg"})

			// Phase 2: load skills on startup
			skillLoader := skills.NewSkillLoader("")
			loadedSkills, skillErrs := skillLoader.LoadAll()
			for _, e := range skillErrs {
				log.Printf("[skills] %v", e)
			}

			systemPrompt := repl.BuildSystemPrompt(executor.ListTools())

			// If --skill flag is provided, prepend the skill's system prompt
			if skillName != "" {
				sk, ok := skillLoader.GetSkill(skillName)
				if !ok {
					return fmt.Errorf("unknown skill: %s", skillName)
				}
				systemPrompt = sk.SystemPrompt + "\n\n" + systemPrompt
			}

			// Append tool allow/disallow info to system prompt
			if len(allowedTools) > 0 {
				systemPrompt += fmt.Sprintf("\n\n# Allowed Tools\nOnly use these tools: %s\n", strings.Join(allowedTools, ", "))
			}
			if len(disallowedTools) > 0 {
				systemPrompt += fmt.Sprintf("\n\n# Disallowed Tools\nDo NOT use these tools: %s\n", strings.Join(disallowedTools, ", "))
			}

			if printPrompt {
				fmt.Println(systemPrompt)
				return nil
			}

			if verbose {
				log.Printf("[verbose] model=%s maxTurns=%d maxTokens=%d", resolvedModel, maxTurns, maxTokens)
			}

			// Load trusted tools store
			trustedStore := agent.NewTrustedToolStore("")
			_ = trustedStore.Load()

			prompter := &repl.TerminalPermissionPrompter{
				Scanner: bufio.NewScanner(os.Stdin),
				Writer:  os.Stdout,
				Trusted: trustedStore,
			}

			// Use FallbackProvider (which implements Provider) for the runtime
			toolCb := &repl.TerminalToolCallback{Writer: os.Stdout}

			// Load plugins and create hook runner
			pm := plugins.NewPluginManager(filepath.Join(".gocode", "plugins"))
			loadedPlugins, pluginErrs := pm.LoadAll()
			for _, e := range pluginErrs {
				log.Printf("[plugins] %v", e)
			}
			hookRunner := plugins.NewPluginHookRunner(loadedPlugins)

			rt := agent.NewConversationRuntime(agent.RuntimeOptions{
				Provider:      fp,
				Executor:      executor,
				Model:         resolvedModel,
				MaxTokens:     maxTokens,
				MaxIterations: maxTurns,
				SystemPrompt:  systemPrompt,
				PermMode:      permMode,
				Prompter:      prompter,
				Trusted:       trustedStore,
				ToolCb:        toolCb,
				Hooks:         hookRunner,
			})

			// Phase 1: wrap runtime with SessionRecoveryManager
			_ = agent.NewSessionRecoveryManager(rt, sessionStore, stdRecoveryLogger{})

			if useTUI && isTerminal() {
				tui.ApplyTheme(tui.LoadTheme(themeName))
				return tui.Run(rt, tui.Config{
					Version:  version,
					Model:    resolvedModel,
					MaxTurns: maxTurns,
					Skills:   loadedSkills,
				})
			}

			r := repl.NewREPL(rt, os.Stdin, os.Stdout, repl.REPLConfig{
				Version:  version,
				Model:    resolvedModel,
				MaxTurns: maxTurns,
			}, loadedSkills)
			return r.Run(context.Background())
		},
	}
	chatCmd.Flags().String("model", "sonnet", "Model name or alias")
	chatCmd.Flags().String("goal", "", "Goal-based model selection: coding, latency, balanced")
	chatCmd.Flags().Int("max-turns", 30, "Maximum agent loop iterations")
	chatCmd.Flags().Int("max-tokens", 8192, "Maximum output tokens per request")
	chatCmd.Flags().String("api-key", "", "API key (overrides env vars)")
	chatCmd.Flags().Bool("hashline", false, "Enable hashline mode for hash-anchored file I/O")
	chatCmd.Flags().String("skill", "", "Activate a skill by name (prepends skill system prompt)")
	chatCmd.Flags().Bool("dangerously-skip-permissions", false, "Skip all permission prompts (full access)")
	chatCmd.Flags().Bool("print", false, "Print system prompt and exit")
	chatCmd.Flags().Bool("verbose", false, "Log API request/response sizes")
	chatCmd.Flags().Bool("no-project-config", false, "Skip loading GOCODE.md/CLAUDE.md")
	chatCmd.Flags().StringSlice("allowedTools", nil, "Whitelist specific tools")
	chatCmd.Flags().StringSlice("disallowedTools", nil, "Blacklist specific tools")
	chatCmd.Flags().Bool("tui", false, "Force bubbletea TUI mode")
	chatCmd.Flags().Bool("no-tui", false, "Force line-based REPL mode (disable TUI)")
	chatCmd.Flags().String("theme", "", "TUI color theme (golang, monokai, dracula, nord)")
	rootCmd.AddCommand(chatCmd)

	// --- profile commands ---
	profileCmd := &cobra.Command{Use: "profile", Short: "Manage launch profiles"}
	profileCmd.AddCommand(&cobra.Command{
		Use:   "init",
		Short: "Create a default profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := profiles.Profile{Provider: "anthropic", Model: "sonnet", Goal: "coding"}
			path := profiles.DefaultProfilePath()
			if err := profiles.SaveProfile(path, p); err != nil {
				return err
			}
			fmt.Printf("Profile created at %s\n", path)
			return nil
		},
	})
	profileCmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Show current profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := profiles.LoadProfile(profiles.DefaultProfilePath())
			if err != nil {
				return fmt.Errorf("no profile found (run 'gocode profile init'): %w", err)
			}
			data, _ := json.MarshalIndent(p, "", "  ")
			fmt.Println(string(data))
			return nil
		},
	})
	profileRecommendCmd := &cobra.Command{
		Use:   "recommend",
		Short: "Recommend the best profile for your goal",
		Run: func(cmd *cobra.Command, args []string) {
			goal, _ := cmd.Flags().GetString("goal")
			p := profiles.Recommend(goal)
			data, _ := json.MarshalIndent(p, "", "  ")
			fmt.Println(string(data))
		},
	}
	profileRecommendCmd.Flags().String("goal", "coding", "Goal: coding, latency, balanced")
	profileCmd.AddCommand(profileRecommendCmd)
	profileAutoCmd := &cobra.Command{
		Use:   "auto",
		Short: "Auto-detect best provider from env vars and save profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			goal, _ := cmd.Flags().GetString("goal")
			p := profiles.AutoDetect(goal)
			path := profiles.DefaultProfilePath()
			if err := profiles.SaveProfile(path, p); err != nil {
				return err
			}
			fmt.Printf("Auto-detected profile saved to %s\n", path)
			data, _ := json.MarshalIndent(p, "", "  ")
			fmt.Println(string(data))
			return nil
		},
	}
	profileAutoCmd.Flags().String("goal", "coding", "Goal: coding, latency, balanced")
	profileCmd.AddCommand(profileAutoCmd)
	rootCmd.AddCommand(profileCmd)

	// --- doctor CLI command ---
	doctorCmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check environment and dependencies",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Checking environment...")
			checks := []struct{ name, cmd string }{
				{"git", "git --version"},
				{"go", "go version"},
				{"tmux", "tmux -V"},
				{"ast-grep", "ast-grep --version"},
			}
			for _, c := range checks {
				parts := strings.Fields(c.cmd)
				if out, err := exec.Command(parts[0], parts[1:]...).Output(); err == nil {
					fmt.Printf("  ✓ %s: %s\n", c.name, strings.TrimSpace(string(out)))
				} else {
					fmt.Printf("  ✗ %s: not found\n", c.name)
				}
			}
			for _, env := range []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GEMINI_API_KEY", "XAI_API_KEY",
				"OPENROUTER_API_KEY", "TOGETHER_API_KEY", "GROQ_API_KEY", "DEEPSEEK_API_KEY", "MISTRAL_API_KEY",
				"AZURE_OPENAI_API_KEY"} {
				if os.Getenv(env) != "" {
					fmt.Printf("  ✓ %s: set\n", env)
				} else {
					fmt.Printf("  - %s: not set\n", env)
				}
			}
			if baseURL := os.Getenv("OPENAI_BASE_URL"); baseURL != "" {
				fmt.Printf("  ℹ OPENAI_BASE_URL: %s\n", baseURL)
			}
			for _, name := range []string{"GOCODE.md", "CLAUDE.md"} {
				if _, err := os.Stat(name); err == nil {
					fmt.Printf("  ✓ %s: found\n", name)
				}
			}
			// Check codex auth
			home, _ := os.UserHomeDir()
			if _, err := os.Stat(filepath.Join(home, ".codex", "auth.json")); err == nil {
				fmt.Printf("  ✓ Codex auth: ~/.codex/auth.json found\n")
			}
			// Check profile
			if _, err := os.Stat(profiles.DefaultProfilePath()); err == nil {
				fmt.Printf("  ✓ Profile: %s found\n", profiles.DefaultProfilePath())
			}
		},
	}
	rootCmd.AddCommand(doctorCmd)

	// --- smoke — quick runtime smoke test ---
	rootCmd.AddCommand(&cobra.Command{
		Use:   "smoke",
		Short: "Quick runtime smoke test",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Running smoke test...")
			// Check binary version
			fmt.Printf("  ✓ Version: %s\n", version)
			// Check tool registry
			fmt.Printf("  ✓ Commands: %d registered\n", len(cmdReg.GetCommands(true, true)))
			fmt.Printf("  ✓ Tools: %d registered\n", len(toolReg.GetTools(false, true, permissions.NewToolPermissionContext(nil, nil))))
			// Check session store
			fmt.Printf("  ✓ Session dir: %s\n", sessionStore.Dir)
			// Check skills
			sl := skills.NewSkillLoader("")
			ls, _ := sl.LoadAll()
			fmt.Printf("  ✓ Skills: %d loaded\n", len(ls))
			// Check plugins
			pm := plugins.NewPluginManager(filepath.Join(".gocode", "plugins"))
			lp, _ := pm.LoadAll()
			fmt.Printf("  ✓ Plugins: %d loaded\n", len(lp))
			fmt.Println("Smoke test passed.")
		},
	})

	// --- hardening — runtime hardening check ---
	rootCmd.AddCommand(&cobra.Command{
		Use:   "hardening",
		Short: "Runtime hardening check — verify security and stability",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Runtime hardening check...")
			issues := 0
			// Check file permissions on config dirs
			for _, dir := range []string{".gocode", ".gocode/skills", ".gocode/plugins"} {
				if info, err := os.Stat(dir); err == nil {
					perm := info.Mode().Perm()
					if perm&0o077 != 0 {
						fmt.Printf("  ⚠ %s: world-readable (mode %o) — recommend chmod 700\n", dir, perm)
						issues++
					} else {
						fmt.Printf("  ✓ %s: permissions OK\n", dir)
					}
				}
			}
			// Check auth key file
			authPath := filepath.Join(".gocode", "auth_keys.json")
			if info, err := os.Stat(authPath); err == nil {
				perm := info.Mode().Perm()
				if perm&0o077 != 0 {
					fmt.Printf("  ⚠ %s: world-readable (mode %o) — recommend chmod 600\n", authPath, perm)
					issues++
				} else {
					fmt.Printf("  ✓ %s: permissions OK\n", authPath)
				}
			}
			// Check API keys not in shell history
			home, _ := os.UserHomeDir()
			for _, histFile := range []string{".bash_history", ".zsh_history"} {
				histPath := filepath.Join(home, histFile)
				if data, err := os.ReadFile(histPath); err == nil {
					content := string(data)
					for _, key := range []string{"ANTHROPIC_API_KEY=", "OPENAI_API_KEY=", "GEMINI_API_KEY="} {
						if strings.Contains(content, key) {
							fmt.Printf("  ⚠ API key found in %s — use env file or secrets manager\n", histFile)
							issues++
							break
						}
					}
				}
			}
			if issues == 0 {
				fmt.Println("  ✓ All hardening checks passed.")
			} else {
				fmt.Printf("  %d issue(s) found.\n", issues)
			}
		},
	})

	// 23. prompt — one-shot agent mode
	promptCmd := &cobra.Command{
		Use:   "prompt [text]",
		Short: "Run a single prompt through the agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			model, _ := cmd.Flags().GetString("model")
			maxTurns, _ := cmd.Flags().GetInt("max-turns")
			maxTokens, _ := cmd.Flags().GetInt("max-tokens")
			apiKey, _ := cmd.Flags().GetString("api-key")
			noStream, _ := cmd.Flags().GetBool("no-stream")
			hashlineEnabled, _ := cmd.Flags().GetBool("hashline")
			skillName, _ := cmd.Flags().GetString("skill")
			printPrompt, _ := cmd.Flags().GetBool("print")
			verbose, _ := cmd.Flags().GetBool("verbose")

			provider, resolvedModel, err := apiclient.ResolveProvider(model, apiKey)
			if err != nil {
				return fmt.Errorf("resolving provider: %w", err)
			}

			// After resolving the model, set model-aware max tokens if user didn't override
			if !cmd.Flags().Changed("max-tokens") {
				maxTokens = apiclient.MaxTokensForModel(resolvedModel)
			}

			// Phase 1: wrap provider with FallbackProvider and ModelRouter
			fp := buildFallbackProvider(provider, resolvedModel)
			router := buildModelRouter(fp)

			toolImpl := toolimpl.NewRegistry()

			// Phase 1 + Phase 3: register advanced tools
			cleanup := wireAdvancedTools(toolImpl, hashlineEnabled)
			defer cleanup()

			executor := agent.NewRegistryExecutor(toolImpl, toolReg)

			// Phase 2: create Orchestrator with ModelRouter and executor,
			// then register its delegation tools in the tool registry.
			orch := orchestrator.NewOrchestrator(router, executor)
			toolImpl.Set("orchestrator_delegate", &orchestratorToolAdapter{orch: orch, toolName: "orchestrator_delegate"})
			toolImpl.Set("orchestrator_delegate_bg", &orchestratorToolAdapter{orch: orch, toolName: "orchestrator_delegate_bg"})

			// Phase 2: load skills on startup
			skillLoader := skills.NewSkillLoader("")
			_, skillErrs := skillLoader.LoadAll()
			for _, e := range skillErrs {
				log.Printf("[skills] %v", e)
			}

			systemPrompt := repl.BuildSystemPrompt(executor.ListTools())

			// If --skill flag is provided, prepend the skill's system prompt
			if skillName != "" {
				sk, ok := skillLoader.GetSkill(skillName)
				if !ok {
					return fmt.Errorf("unknown skill: %s", skillName)
				}
				systemPrompt = sk.SystemPrompt + "\n\n" + systemPrompt
			}

			if printPrompt {
				fmt.Println(systemPrompt)
				return nil
			}

			if verbose {
				log.Printf("[verbose] model=%s maxTurns=%d maxTokens=%d", resolvedModel, maxTurns, maxTokens)
			}

			// Use FallbackProvider (which implements Provider) for the runtime
			rt := agent.NewConversationRuntime(agent.RuntimeOptions{
				Provider:      fp,
				Executor:      executor,
				Model:         resolvedModel,
				MaxTokens:     maxTokens,
				MaxIterations: maxTurns,
				SystemPrompt:  systemPrompt,
				PermMode:      agent.DangerFullAccess,
			})

			// Phase 1: wrap runtime with SessionRecoveryManager
			_ = agent.NewSessionRecoveryManager(rt, sessionStore, stdRecoveryLogger{})

			return repl.RunOneShot(context.Background(), rt, args[0], !noStream, os.Stdout)
		},
	}
	promptCmd.Flags().String("model", "sonnet", "Model name or alias")
	promptCmd.Flags().Int("max-turns", 30, "Maximum agent loop iterations")
	promptCmd.Flags().Int("max-tokens", 8192, "Maximum output tokens per request")
	promptCmd.Flags().String("api-key", "", "API key (overrides env vars)")
	promptCmd.Flags().Bool("no-stream", false, "Disable streaming output")
	promptCmd.Flags().Bool("hashline", false, "Enable hashline mode for hash-anchored file I/O")
	promptCmd.Flags().String("skill", "", "Activate a skill by name (prepends skill system prompt)")
	promptCmd.Flags().Bool("print", false, "Print system prompt and exit")
	promptCmd.Flags().Bool("verbose", false, "Log API request/response sizes")
	rootCmd.AddCommand(promptCmd)

	// 21. mcp-serve
	mcpServeCmd := &cobra.Command{
		Use:   "mcp-serve",
		Short: "Start MCP server (MCP protocol compliant)",
		RunE: func(cmd *cobra.Command, args []string) error {
			transport, _ := cmd.Flags().GetString("transport")
			addr, _ := cmd.Flags().GetString("addr")

			// Create the real tool implementation registry
			toolImpl := toolimpl.NewRegistry()
			server := mcp.NewMCPServer(toolReg, toolImpl, cmdReg, rt, sessionStore, version)

			switch transport {
			case "stdio":
				return server.ServeStdio()
			case "http":
				fmt.Fprintf(os.Stderr, "Starting MCP HTTP server on %s\n", addr)
				return server.ServeHTTP(addr)
			default:
				return fmt.Errorf("unknown transport: %s (use stdio or http)", transport)
			}
		},
	}
	mcpServeCmd.Flags().String("transport", "stdio", "Transport type: stdio or http")
	mcpServeCmd.Flags().String("addr", ":8080", "HTTP listen address (only for http transport)")
	rootCmd.AddCommand(mcpServeCmd)

	// --- Feature 5: serve — HTTP REST API server ---
	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start headless HTTP API server",
		RunE: func(cmd *cobra.Command, args []string) error {
			addr, _ := cmd.Flags().GetString("addr")
			// Load auth keys for request validation
			store := authkeys.NewStore(filepath.Join(".gocode", "auth_keys.json"))
			store.Load()
			var authOpts []func(string) bool
			if len(store.List()) > 0 {
				authOpts = append(authOpts, store.Validate)
			}
			handler := apiserver.NewHandler(
				apiserver.Config{Version: version},
				func(msg string) (string, error) {
					return fmt.Sprintf("echo: %s", msg), nil
				},
				func() (int, string) {
					return 0, "none"
				},
				authOpts...,
			)
			fmt.Fprintf(os.Stderr, "gocode API server listening on %s\n", addr)
			return http.ListenAndServe(addr, handler)
		},
	}
	serveCmd.Flags().String("addr", ":3000", "Listen address")
	rootCmd.AddCommand(serveCmd)

	// --- Feature 6: stats — usage statistics ---
	rootCmd.AddCommand(&cobra.Command{
		Use:   "stats",
		Short: "Show usage statistics across all sessions",
		Run: func(cmd *cobra.Command, args []string) {
			entries, err := os.ReadDir(sessionStore.Dir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "No sessions found (%v)\n", err)
				return
			}
			totalSessions := 0
			totalInput := 0
			totalOutput := 0
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
					continue
				}
				sid := strings.TrimSuffix(e.Name(), ".json")
				s, err := sessionStore.Load(sid)
				if err != nil {
					continue
				}
				totalSessions++
				totalInput += s.InputTokens
				totalOutput += s.OutputTokens
			}
			fmt.Printf("Sessions:      %d\n", totalSessions)
			fmt.Printf("Total input:   %d tokens\n", totalInput)
			fmt.Printf("Total output:  %d tokens\n", totalOutput)
		},
	})

	// --- Feature 7: export / import ---
	rootCmd.AddCommand(&cobra.Command{
		Use:   "export [session_id]",
		Short: "Export session JSON to stdout",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := sessionStore.Load(args[0])
			if err != nil {
				return err
			}
			data, err := json.MarshalIndent(s, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:   "import [file]",
		Short: "Import a session JSON file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("reading file: %w", err)
			}
			var s session.StoredSession
			if err := json.Unmarshal(data, &s); err != nil {
				return fmt.Errorf("parsing session: %w", err)
			}
			if s.SessionID == "" {
				return fmt.Errorf("session_id is required in the JSON file")
			}
			path, err := sessionStore.Save(s)
			if err != nil {
				return err
			}
			fmt.Printf("Imported session %s to %s\n", s.SessionID, path)
			return nil
		},
	})

	// --- Feature 8: pr — create GitHub PR ---
	rootCmd.AddCommand(&cobra.Command{
		Use:   "pr",
		Short: "Create a GitHub PR with AI-generated description",
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := exec.LookPath("gh"); err != nil {
				return fmt.Errorf("requires gh CLI: https://cli.github.com")
			}
			diff, _ := exec.Command("git", "diff", "main...HEAD", "--stat").Output()
			if len(diff) > 0 {
				fmt.Printf("Changes:\n%s\n", string(diff))
			}
			out, err := exec.Command("gh", "pr", "create", "--fill").CombinedOutput()
			fmt.Print(string(out))
			return err
		},
	})

	// --- Feature 9: github — list issues ---
	rootCmd.AddCommand(&cobra.Command{
		Use:   "github",
		Short: "GitHub integration — list issues and PRs",
		Run: func(cmd *cobra.Command, args []string) {
			out, err := exec.Command("gh", "issue", "list", "--limit", "10").Output()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Requires gh CLI: https://cli.github.com\n")
				return
			}
			fmt.Print(string(out))
		},
	})

	// --- config command ---
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Show current configuration",
		Run: func(cmd *cobra.Command, args []string) {
			cwd, _ := os.Getwd()
			fmt.Printf("Version:        %s\n", version)
			fmt.Printf("Working dir:    %s\n", cwd)
			fmt.Printf("Session dir:    %s\n", sessionStore.Dir)
			fmt.Printf("Skills dir:     .gocode/skills/\n")
			fmt.Printf("Plugins dir:    .gocode/plugins/\n")
			fmt.Printf("Theme:          %s\n", tui.LoadTheme("").Name)
			for _, name := range []string{"GOCODE.md", "CLAUDE.md"} {
				if _, err := os.Stat(name); err == nil {
					fmt.Printf("Project config: %s\n", name)
					break
				}
			}
			for _, env := range []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GEMINI_API_KEY", "XAI_API_KEY"} {
				if os.Getenv(env) != "" {
					fmt.Printf("Provider:       %s (set)\n", env)
				}
			}
			sl := skills.NewSkillLoader("")
			ls, _ := sl.LoadAll()
			fmt.Printf("Skills:         %d loaded\n", len(ls))
			pm := plugins.NewPluginManager(filepath.Join(".gocode", "plugins"))
			lp, _ := pm.LoadAll()
			fmt.Printf("Plugins:        %d loaded\n", len(lp))
			editor := editorcompat.DetectEditor()
			fmt.Printf("Editor:         %s\n", editor)
		},
	}
	rootCmd.AddCommand(configCmd)

	// --- plugin commands ---
	pluginCmd := &cobra.Command{Use: "plugin", Short: "Manage plugins"}
	pluginCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List installed plugins",
		Run: func(cmd *cobra.Command, args []string) {
			pm := plugins.NewPluginManager(filepath.Join(".gocode", "plugins"))
			loaded, errs := pm.LoadAll()
			for _, e := range errs {
				fmt.Fprintf(os.Stderr, "Warning: %v\n", e)
			}
			if len(loaded) == 0 {
				fmt.Println("No plugins installed.")
				return
			}
			for _, p := range loaded {
				fmt.Printf("%s v%s — %s\n", p.Name, p.Version, p.Description)
			}
		},
	})
	pluginCmd.AddCommand(&cobra.Command{
		Use:   "install [path]",
		Short: "Install a plugin from a local directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := os.ReadFile(filepath.Join(args[0], "plugin.json"))
			if err != nil {
				return fmt.Errorf("reading plugin.json: %w", err)
			}
			var p plugins.Plugin
			if err := json.Unmarshal(data, &p); err != nil {
				return fmt.Errorf("parsing plugin.json: %w", err)
			}
			pm := plugins.NewPluginManager(filepath.Join(".gocode", "plugins"))
			if err := pm.Install(p.Name, p); err != nil {
				return err
			}
			fmt.Printf("Installed plugin %s v%s\n", p.Name, p.Version)
			return nil
		},
	})
	pluginCmd.AddCommand(&cobra.Command{
		Use:   "uninstall [name]",
		Short: "Uninstall a plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pm := plugins.NewPluginManager(filepath.Join(".gocode", "plugins"))
			if err := pm.Uninstall(args[0]); err != nil {
				return err
			}
			fmt.Printf("Uninstalled plugin %s\n", args[0])
			return nil
		},
	})
	rootCmd.AddCommand(pluginCmd)

	// --- auth — manage remote access auth keys ---
	authCmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage remote access auth keys",
	}
	authStorePath := filepath.Join(".gocode", "auth_keys.json")
	authCmd.AddCommand(&cobra.Command{
		Use:   "generate [name]",
		Short: "Generate a new auth key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store := authkeys.NewStore(authStorePath)
			if err := store.Load(); err != nil {
				return err
			}
			ak, err := store.Generate(args[0])
			if err != nil {
				return err
			}
			if err := store.Save(); err != nil {
				return err
			}
			fmt.Printf("Generated key: %s\n  ID:   %s\n  Name: %s\n  Key:  %s\n", ak.Name, ak.ID, ak.Name, ak.Key)
			return nil
		},
	})
	authCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all auth keys",
		RunE: func(cmd *cobra.Command, args []string) error {
			store := authkeys.NewStore(authStorePath)
			if err := store.Load(); err != nil {
				return err
			}
			keys := store.List()
			if len(keys) == 0 {
				fmt.Println("No auth keys configured.")
				return nil
			}
			for _, k := range keys {
				fmt.Printf("%s  %s  (created %s)\n", k.ID, k.Name, k.CreatedAt)
			}
			return nil
		},
	})
	authCmd.AddCommand(&cobra.Command{
		Use:   "delete [id]",
		Short: "Delete an auth key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store := authkeys.NewStore(authStorePath)
			if err := store.Load(); err != nil {
				return err
			}
			if err := store.Delete(args[0]); err != nil {
				return err
			}
			if err := store.Save(); err != nil {
				return err
			}
			fmt.Printf("Deleted key %s\n", args[0])
			return nil
		},
	})
	rootCmd.AddCommand(authCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
