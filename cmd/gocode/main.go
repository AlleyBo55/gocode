package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/AlleyBo55/gocode/data"
	"github.com/AlleyBo55/gocode/internal/agent"
	"github.com/AlleyBo55/gocode/internal/apiclient"
	"github.com/AlleyBo55/gocode/internal/astgrep"
	"github.com/AlleyBo55/gocode/internal/bootstrap"
	"github.com/AlleyBo55/gocode/internal/commandgraph"
	"github.com/AlleyBo55/gocode/internal/commands"
	"github.com/AlleyBo55/gocode/internal/execution"
	"github.com/AlleyBo55/gocode/internal/hashline"
	"github.com/AlleyBo55/gocode/internal/initdeep"
	"github.com/AlleyBo55/gocode/internal/manifest"
	"github.com/AlleyBo55/gocode/internal/mcp"
	"github.com/AlleyBo55/gocode/internal/mcpclient"
	"github.com/AlleyBo55/gocode/internal/modes"
	"github.com/AlleyBo55/gocode/internal/orchestrator"
	"github.com/AlleyBo55/gocode/internal/permissions"
	"github.com/AlleyBo55/gocode/internal/queryengine"
	"github.com/AlleyBo55/gocode/internal/repl"
	"github.com/AlleyBo55/gocode/internal/runtime"
	"github.com/AlleyBo55/gocode/internal/session"
	"github.com/AlleyBo55/gocode/internal/setup"
	"github.com/AlleyBo55/gocode/internal/skills"
	"github.com/AlleyBo55/gocode/internal/tmux"
	"github.com/AlleyBo55/gocode/internal/toolimpl"
	"github.com/AlleyBo55/gocode/internal/toolpool"
	"github.com/AlleyBo55/gocode/internal/tools"
)

var version = "v0.3.0"

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
			maxTurns, _ := cmd.Flags().GetInt("max-turns")
			maxTokens, _ := cmd.Flags().GetInt("max-tokens")
			apiKey, _ := cmd.Flags().GetString("api-key")
			hashlineEnabled, _ := cmd.Flags().GetBool("hashline")

			provider, resolvedModel, err := apiclient.ResolveProvider(model, apiKey)
			if err != nil {
				return fmt.Errorf("resolving provider: %w", err)
			}

			// Phase 1: wrap provider with FallbackProvider and ModelRouter
			fp := buildFallbackProvider(provider, resolvedModel)
			router := buildModelRouter(fp)

			toolImpl := toolimpl.NewRegistry()

			// Phase 1 + Phase 3: register advanced tools (hashline, context-aware read, ast-grep, tmux, MCP client)
			cleanup := wireAdvancedTools(toolImpl, hashlineEnabled)
			defer cleanup()

			executor := agent.NewRegistryExecutor(toolImpl, toolReg)

			// Phase 2: create Orchestrator with ModelRouter and executor
			_ = orchestrator.NewOrchestrator(router, executor)

			// Phase 2: load skills on startup
			skillLoader := skills.NewSkillLoader("")
			loadedSkills, skillErrs := skillLoader.LoadAll()
			for _, e := range skillErrs {
				log.Printf("[skills] %v", e)
			}
			_ = loadedSkills

			systemPrompt := repl.BuildSystemPrompt(executor.ListTools())

			prompter := &repl.TerminalPermissionPrompter{
				Scanner: bufio.NewScanner(os.Stdin),
				Writer:  os.Stdout,
			}

			// Use FallbackProvider (which implements Provider) for the runtime
			rt := agent.NewConversationRuntime(agent.RuntimeOptions{
				Provider:      fp,
				Executor:      executor,
				Model:         resolvedModel,
				MaxTokens:     maxTokens,
				MaxIterations: maxTurns,
				SystemPrompt:  systemPrompt,
				PermMode:      agent.WorkspaceWrite,
				Prompter:      prompter,
				ToolCb:        &repl.TerminalToolCallback{Writer: os.Stdout},
			})

			// Phase 1: wrap runtime with SessionRecoveryManager
			_ = agent.NewSessionRecoveryManager(rt, sessionStore, stdRecoveryLogger{})

			r := repl.NewREPL(rt, os.Stdin, os.Stdout, repl.REPLConfig{
				Version:  version,
				Model:    resolvedModel,
				MaxTurns: maxTurns,
			})
			return r.Run(context.Background())
		},
	}
	chatCmd.Flags().String("model", "sonnet", "Model name or alias")
	chatCmd.Flags().Int("max-turns", 30, "Maximum agent loop iterations")
	chatCmd.Flags().Int("max-tokens", 8192, "Maximum output tokens per request")
	chatCmd.Flags().String("api-key", "", "API key (overrides env vars)")
	chatCmd.Flags().Bool("hashline", false, "Enable hashline mode for hash-anchored file I/O")
	rootCmd.AddCommand(chatCmd)

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

			provider, resolvedModel, err := apiclient.ResolveProvider(model, apiKey)
			if err != nil {
				return fmt.Errorf("resolving provider: %w", err)
			}

			// Phase 1: wrap provider with FallbackProvider and ModelRouter
			fp := buildFallbackProvider(provider, resolvedModel)
			router := buildModelRouter(fp)

			toolImpl := toolimpl.NewRegistry()

			// Phase 1 + Phase 3: register advanced tools
			cleanup := wireAdvancedTools(toolImpl, hashlineEnabled)
			defer cleanup()

			executor := agent.NewRegistryExecutor(toolImpl, toolReg)

			// Phase 2: create Orchestrator with ModelRouter and executor
			_ = orchestrator.NewOrchestrator(router, executor)

			// Phase 2: load skills on startup
			skillLoader := skills.NewSkillLoader("")
			loadedSkills, skillErrs := skillLoader.LoadAll()
			for _, e := range skillErrs {
				log.Printf("[skills] %v", e)
			}
			_ = loadedSkills

			systemPrompt := repl.BuildSystemPrompt(executor.ListTools())

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

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
