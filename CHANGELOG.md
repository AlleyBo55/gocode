# Changelog

All notable changes to gocode are documented here.

---

## v0.10.0 — The Any-Model Release Has To Actually Work With Any Model

*No new parity rows. The provider layer, the permission boundary, and the agent loop get tests, and the things those tests found get fixed.*

### Token efficiency
- **Prompt caching on Anthropic.** The system prompt (which covers the tool schemas) and the newest message carry `cache_control` breakpoints, so every turn after the first reads the fixed prefix and the prior conversation at a tenth of the input price. Markers are set on request copies, never on the stored session. `/cost` already prices cache reads and writes at Anthropic's rates. Set `GOCODE_DISABLE_PROMPT_CACHE=1` for an Anthropic-compatible endpoint that rejects the field.
- **Tool output is capped at 40 KB.** Every tool result, including plugin tools, keeps its head and tail (failures live at the end) and replaces the middle with a marker that says how much was dropped and how to ask for the part you need. Previously one `cat` of a build log could put 100k tokens into context and every later turn paid for it again.
- **File reads are paged.** `FileReadTool` without an `end_line` returns 2000 lines and names the `start_line` to continue from; explicit ranges are honoured in full. A file ending in a newline no longer gets a phantom empty last line.
- **Context estimate counts tool results.** The 85% auto-compaction threshold was computed over message text only; tool results, the bulk of any coding session, counted as zero, so compaction fired long after the real window was exceeded.

### Swarm, made real
- Until now `send_agent_message` was registered for one agent named `main` and never offered to the model, and nothing read any mailbox. Now every sub-agent the orchestrator spawns joins the swarm under its own name (`deep-worker-1`, `planner-2`, ...) for the duration of its run, is offered `send_agent_message` and `list_agents`, and reads its inbox at the start of every turn. The interactive session is `main` and has the same two tools. Background agents post a bounded report of their result, or their failure, to `main`'s inbox, so the next turn hears about it without polling `/tasks`.

### Fixed
- **Streams that broke mid-reply were recorded as complete.** Both provider stream readers returned silently on a malformed frame or a dropped connection; the runtime saw a clean close and treated the partial reply as the model's answer. They now emit the error event the REPL already renders. Anthropic's own mid-stream `error` frames (`overloaded_error`) are surfaced the same way instead of being forwarded as empty events.
- **Model fallback never fired for rate limits.** Providers retry internally and wrap the final 429 in a `RetriesExhausted` error whose status is 0; the fallback chain judged the wrapper, not the cause. OpenAI's `context_length_exceeded` lives in `error.code`, which was never read.
- **Token counts for streamed turns were wrong.** OpenAI-compatible streams never requested usage (`stream_options.include_usage`) and recorded zero. Anthropic streams recorded zero *input* tokens because input lives in `message_start` and only `message_delta` was read.
- **Trusted-tool patterns matched substrings of the raw JSON.** `BashTool:ls *` auto-approved `rm -rf ./tools`; `FileWriteTool:src *` approved `src/../../etc/passwd`. Prefixes now match the command on a word boundary, the search pattern at its start, and the cleaned path at a directory boundary.
- **Compound commands slipped past a trusted prefix.** `BashTool:git *` approved `git status; rm -rf /`. Every command on the line must now be covered by a trusted prefix; command substitution and file-writing redirection are never auto-trusted (`2>&1` and `>/dev/null` are fine).
- **Malformed tool arguments executed with empty input.** A tool call whose JSON did not parse ran with no arguments and produced a misleading "path is required" error. The model now gets a precise error naming the bad JSON and is asked to resend.
- The Codex provider path was unreachable (`codex` matched the generic OpenAI branch first) and read the wrong keys from `~/.codex/auth.json`. It now reads the file the Codex CLI actually writes, honours `CODEX_HOME`, and fails with a clear missing-credentials error instead of a 401.

### Added
- **Zero-config local models.** A model id with an Ollama-style tag (`qwen2.5-coder:7b`, or the `llama` alias) is routed to a running Ollama or LM Studio automatically, no key, no `OPENAI_BASE_URL`. `OLLAMA_HOST` is honoured for non-default addresses. If nothing is listening, the error names the local servers it looked for instead of blaming a missing Anthropic key.
- **Works with nothing configured.** With no `--model`, no provider keys, and a local server that has models installed, `gocode` picks a coding model from it, prints which one on stderr, and starts. With nothing at all, it prints three ways to get started (local and free, one OpenRouter key, your own provider key), naming the local models it found, instead of a bare credentials error.
- **Bare `gocode` starts chat** when run in a terminal, like every other agent CLI. Piped or scripted, it prints help. `gocode --model X` works as shorthand for `gocode chat --model X`.
- **Help is organised.** `gocode --help` groups commands (Agent, Servers and integrations, Sessions and settings, Diagnostics) and hides the eighteen porting-harness commands (`manifest`, `parity-audit`, `teleport-mode`, ...). They still run; they just no longer greet new users.
- **Loop guard.** When the model issues the same tool calls and gets the same results three iterations in a row, it is told so in the tool result; after five, the loop stops with reason `stuck`. A command re-run after an edit, or a poll whose output changes, never trips it. Weak models used to burn every one of `--max-turns` this way.
- **`--max-cost <usd>`** on `chat` and `prompt`. Stops the session cleanly when its estimated spend reaches the limit, before running any pending tools, and leaves the transcript valid. Warns up front when the model has no price data and the limit cannot be enforced. Intended for unattended runs such as the GitHub Action.
- **Per-model cost ledger.** `/cost` now shows tokens, turns, estimated cost, and cost share for each model that served the session, from a real list-price table (`internal/apiclient/pricing.go`) instead of a blended $3/$15 guess. Under a fallback chain this shows which model actually answered.
- **`gocode doctor --model <alias>`.** Four small requests probe whether the model streams, honours a system prompt, calls a tool with valid arguments, and calls several tools in one turn. Results are cached in `.gocode/capabilities.json` (`--refresh` to redo). `chat` and `prompt` warn at startup when the cached probe says the model failed tool calls or ignored the system prompt.
- **Eval harness** under `evals/`: 15 small Go tasks with hidden tests and reference solutions, a runner that builds gocode and scores each task by the module's own tests, and a self-test that proves every task is solvable and discriminating without spending a token. `go run ./evals/runner -model sonnet`. Opt-in; it spends tokens and is not part of CI.
- Tests for `apiclient` (recorded SSE fixtures for both wire formats, retry and error paths, provider resolution), `agent` (permission decisions, trust store, the streaming and non-streaming loops, the new guards), and `permissions`. These packages had none.

### Changed
- Bare `gocode` in a terminal now starts chat instead of printing help. Scripts are unaffected: without a TTY it still prints help and exits 0. Positional text (`gocode "fix it"`) is rejected as before; use `gocode prompt "fix it"`.
- A local-looking model id with no server running now fails with a local-specific message rather than "missing Anthropic credentials".
- `/cost` output is multi-line and priced per model; anything parsing the old single line will see extra lines. `--output-format json` `total_cost` uses the same real pricing.
- `--model codex` with no credentials now errors at startup instead of returning a provider that fails on first use.
- `doctor` exits non-zero when `--model` cannot be resolved. The no-flag path is unchanged.
- A trusted `BashTool:git *` no longer covers `git log | head` on its own; trust `head *` too, or answer the prompt.

### Known gaps
- Proxy providers (OpenRouter, Groq, ...) prefer the environment key over `--api-key`, the reverse of native providers. Left as is; noted in `provider_test.go`.
- The price table is hand-maintained and will drift. There is no override file yet.
- The alias `deepseek` points at `deepseek-chat`, which DeepSeek retired in July 2026.

---

## v0.9.0 — One More Thing.

*18 new features. 8 new skills. The agent learns to dream, plan, coordinate, and remember.*

### New Skills (8)
- **loop** — Autonomous "keep going until done" mode. The agent continues executing without waiting for input until the task is complete or it hits a blocker.
- **stuck** — Structured recovery mode. When the agent gets confused or loops, it re-reads the original request, reviews completed actions, identifies the blocker, and tries an alternative approach.
- **debug** — Structured debugging methodology. Reproduce → examine errors → add logging → isolate root cause → fix → verify.
- **verify** — Work verification. Re-reads modified files, runs tests, checks for regressions, validates against original requirements.
- **simplify** — Code complexity reduction. Identifies dead code, flattens nested logic, extracts reusable functions, prefers stdlib solutions.
- **remember** — Active memory management. Saves important facts, project conventions, user preferences, and architectural decisions to the memory system.
- **skillify** — Meta-skill creation. Analyzes conversation patterns and generates reusable skill JSON profiles from recurring workflows.
- **batch** — Parallel batch processing. Enumerates targets, processes each individually, reports per-target results with a summary.

### ULTRAPLAN — Deep Planning (`internal/ultraplan/`)
- Background planning agent with 30-minute timeout
- Routes to strongest available model via `CategoryUltrabrain` (Opus-class)
- `/ultraplan <task>` slash command for on-demand deep planning
- Non-blocking: user continues chatting while planner works
- Keyword detection triggers automatic planning on "ultraplan" in messages

### Dream System — Autonomous Memory Consolidation (`internal/dream/`)
- Four-phase consolidation cycle: orient → gather → consolidate → prune
- Triggers automatically after 5 minutes of session idle
- Runs final consolidation on session end
- File-lock protection against concurrent writes
- Configurable idle duration and relevance threshold (default 0.05)

### Vim Keybindings (`internal/vim/`)
- Full vim-mode editing: Normal, Insert, Visual, Operator-Pending, Search modes
- Motions: h/j/k/l, w/b/e, 0/$, gg/G, f/F with count prefixes
- Operators: d/c/y with motion and text-object composition (dw, ciw, ya", etc.)
- Text objects: iw/aw, i"/a", i(/a(
- Line operations: dd, yy, p, x, r
- Search: / forward, ? backward with wrap-around
- Ex commands: :w (submit), :q (quit), :wq (both)
- `/vim` toggle command in REPL

### Cron/Scheduled Tasks (`internal/cron/`)
- 5-field cron expression parser (minute/hour/dom/month/dow)
- `ScheduleCronTool` registered in tool registry — agents can create/delete/list cron jobs
- Persistent schedules in `.gocode/cron.json`
- Background goroutine timer-based execution
- Human-readable cron descriptions via `cronToHuman()`
- Scheduler wired into startup (loads persisted schedules) and shutdown (cleanup)

### Bridge/IDE Integration (`internal/bridge/`)
- WebSocket server on configurable port (default 19836)
- Pure stdlib WebSocket implementation (RFC 6455, no external deps)
- JSON message protocol: chat, tool_permission, status, error, notification
- Session management with auto-cleanup on disconnect (30s timeout)
- Permission bridge: forwards tool permission prompts to IDE for approval
- Message bridge: streams agent responses to IDE in real-time
- Port retry: tries 5 sequential ports if default is in use
- `gocode bridge --port <port>` CLI command

### Swarm Coordination (`internal/swarm/`)
- SwarmManager with agent discovery registry
- Mailbox-based inter-agent messaging (buffered channels, non-blocking)
- `SendMessageTool` (`send_agent_message`) for agent-to-agent communication
- Maximum swarm size enforcement (default 10 agents)
- Agent status tracking: running, idle, completed
- `RegisterSwarmTool` convenience function for tool registry wiring

### PDF Handling
- PDF detection in FileReadTool (`.pdf` extension, case-insensitive)
- Delegates to `pdf.ReadPDF` for base64-encoded content extraction
- Magic bytes validation, size limits (50MB)

### Output Styles (`internal/outputstyles/`)
- 4 built-in styles: concise, verbose, markdown, minimal
- User-defined styles from `.gocode/output-styles/` directory
- `--output-style` CLI flag on `gocode chat`
- `/output-style [style]` slash command for mid-session switching
- Style validation against registry

### Migrations System (`internal/migrations/`)
- Automatic config/data format upgrades on startup
- Migration runner with version tracking in `.gocode/version.json`
- Sequential execution of pending migrations
- Backup creation before migration execution

### Buddy System (`internal/buddy/`)
- 18 companion species across 5 rarity tiers (Common/Uncommon/Rare/Epic/Legendary)
- Deterministic gacha via Mulberry32 PRNG seeded from user ID
- ASCII sprite rendering with animation frames
- Buddy display in REPL banner (name, species, rarity)
- Stats tracking: DEBUGGING, CHAOS, SNARK (0-100)

### New CLI Commands & Flags
- `gocode bridge --port <port>` — start WebSocket bridge server
- `--output-style <style>` — set output style (concise, verbose, markdown, minimal)
- `--vim` — enable vim keybindings

### New Slash Commands
- `/ultraplan <task>` — deep planning with strongest model
- `/vim` — toggle vim keybindings
- `/output-style [style]` — switch or show output style

### Infrastructure
- All 16 skills verified loading correctly (8 original + 8 new)
- Full test suite: `go build ./...` and `go test ./...` — zero failures
- Documentation updated: README comparison table, advanced-features.md, cli-reference.md

---

## v0.8.0 — The Universal Model Layer

### 200+ Model Support
- Universal model layer: 4 native providers + 7 proxy services via OpenAI-compatible shim
- Native: Anthropic (Claude), OpenAI (GPT), Google (Gemini), xAI (Grok)
- Proxy: DeepSeek, Mistral, Groq, Together AI, OpenRouter, Azure OpenAI
- Local: Ollama, LM Studio, vLLM via `OPENAI_BASE_URL`
- Codex backend integration with `~/.codex/auth.json` auth
- Full model list with aliases in [docs/supported-models.md](docs/supported-models.md)

### Provider Launch Profiles
- `gocode profile init` — create default profile
- `gocode profile auto --goal coding` — auto-detect best provider from env vars
- `gocode profile recommend --goal latency` — preview without saving
- `gocode profile show` — show current profile
- Goal-based model selection: `--goal coding`, `--goal latency`, `--goal balanced`

### Persistent Memory
- Cross-session memory system persisted to `.gocode/memory.json`
- `/memory set key value` — store a memory
- `/memory get key` — retrieve a memory
- `/memory list` — show all memories
- `/memory delete key` — remove a memory
- Memories injected into system prompt automatically

### Task Management
- Persistent task tracking in `.gocode/tasks.json`
- `/tasks add Fix the auth bug` — create a task
- `/tasks list` — show all tasks
- `/tasks done 1` — mark task complete

### Runtime Hardening
- `gocode smoke` — quick runtime smoke test (version, registries, skills, plugins)
- `gocode hardening` — security audit (file permissions, API key exposure in shell history)
- `gocode doctor` expanded to check all 10 provider env vars + Codex auth + profile

### New Tools
- WebFetchTool — fetch URL content (10KB max, no API key)
- WebSearchTool — DuckDuckGo instant answer API (no API key, no MCP required)
- NotebookEditTool — Jupyter notebook (.ipynb) editing
- `orchestrator_delegate` — delegate tasks to specialist sub-agents
- `orchestrator_delegate_bg` — background sub-agent delegation

### New Slash Commands
- `/commit` — auto-generate commit message with `Co-Authored-By: gocoder6969`
- `/memory` — manage persistent cross-session memories
- `/tasks` — manage persistent task list

### Multi-Agent Orchestration (Wired)
- Orchestrator delegation tools now registered in tool registry (previously dead code)
- LLM can call `orchestrator_delegate` and `orchestrator_delegate_bg`
- 4 built-in sub-agent profiles: coordinator, deep-worker, planner, debugger
- Permission-filtered tool access per sub-agent
- BackgroundManager with 5-slot concurrency limit

---

## v0.7.0 — The Agent Operating System

### Full Terminal UI (Bubbletea TUI)
- Split panels: chat on left, git diff viewer on right (Ctrl+D toggle)
- Tab to switch between Build mode (full access) and Plan mode (read-only)
- 4 built-in themes: `golang`, `monokai`, `dracula`, `nord`
- Custom themes via `.gocode/theme.json`
- Custom keybinds via `.gocode/keybinds.json`
- REPL is default; `--tui` flag for TUI mode

### Multi-Agent Orchestration
- Orchestrator with 4 built-in sub-agent profiles
- ModelRouter for category-based provider selection (deep, quick, visual, ultrabrain)
- BackgroundManager for concurrent agent execution (max 5 slots)
- FallbackProvider with automatic failover on 429/500/502/503/504

### IDE-Level Tools
- LSP integration — rename, go-to-definition, find-references
- AST-grep — structural code search and rewrite
- Tmux sessions — persistent terminal sessions
- MCP client — connect to external MCP servers

### UX Parity with Claude Code
- Real-time streaming with token-by-token display
- Thinking block display (Claude extended thinking)
- Model-aware max tokens
- Git context in system prompt (branch, changed files)
- GOCODE.md / CLAUDE.md project config loading
- Cost estimation with `/cost`
- Ctrl+C interrupt support

### 21 Slash Commands
`/help` `/exit` `/clear` `/compact` `/cost` `/model` `/skill` `/plan` `/init-deep` `/diff` `/undo` `/redo` `/status` `/review` `/permissions` `/doctor` `/connect` `/share` `/commit` `/memory` `/tasks`

### CLI Commands
- `gocode serve` — headless HTTP REST API server
- `gocode stats` — usage statistics across sessions
- `gocode export/import` — session import/export
- `gocode pr` — GitHub PR creation via `gh` CLI
- `gocode github` — GitHub issue listing via `gh` CLI
- `gocode auth generate/list/delete` — remote access key management
- `gocode config` — show runtime configuration
- `gocode plugin list/install/uninstall` — plugin management

### Plugin System
- Hook pipeline with pre/post tool-use interception
- 2 bundled plugins: safety-guard, git-auto-commit
- Plugin install/uninstall via CLI

### Editor Compatibility
- Editor detection (VS Code, Cursor, Kiro, Neovim, etc.)
- Editor-specific configuration hints

### Skills System
- 8 built-in skills with community attributions
- `--skill` flag and `/skill` command for mid-session switching
- Custom skills via `.gocode/skills/` JSON files

### Auto-Format
- gofmt/goimports for Go
- prettier for JS/TS
- black for Python
- rustfmt for Rust

---

## v0.3.0 — The Foundation

### Core Architecture
- Complete Go reimplementation of Claude Code agent runtime
- 38 internal packages: agent, apiclient, apitypes, bootstrap, commands, context, execution, hashline, history, manifest, mcp, models, modes, permissions, queryengine, repl, runtime, session, setup, tools, toolimpl, toolpool, transcript
- Single binary (~12MB), zero runtime dependencies
- <10ms startup time

### Multi-Model Support
- 4 native providers: Anthropic (Claude), OpenAI (GPT), Google (Gemini), xAI (Grok)
- Model aliases: `sonnet`, `opus`, `haiku`, `gpt5`, `gpt4o`, `gemini`, `grok`
- `--model` flag for provider selection

### Agent Mode
- Interactive REPL (`gocode chat`)
- One-shot mode (`gocode prompt`)
- Multi-turn tool-use loops
- Permission system (workspace-write, full-access)

### MCP Server
- Full MCP protocol compliance (initialize, tools/list, tools/call, ping, resources/list)
- 14 built-in tools: BashTool, FileReadTool, FileEditTool, FileWriteTool, GlobTool, GrepTool, ListDirectoryTool
- Dual transport: stdio (for IDEs) and HTTP
- Works with Cursor, Kiro, VS Code, Antigravity, Claude Desktop

### Session Management
- Session persistence and resume
- Transcript flushing
- Session store with JSON serialization

### CLI Commands
- `gocode chat` — interactive agent
- `gocode prompt` — one-shot agent
- `gocode mcp-serve` — MCP server
- `gocode summary` — workspace summary
- `gocode manifest` — port manifest
- `gocode subsystems` — module listing
- `gocode commands` — command registry
- `gocode tools` — tool registry
- `gocode route` — prompt routing
- `gocode bootstrap` — session bootstrapping
- `gocode turn-loop` — turn loop execution
- `gocode setup-report` — environment setup report

### Installation
- `go install` for all platforms
- One-line install scripts (macOS/Linux bash, Windows PowerShell)
- Binary downloads for all platforms (darwin/linux/windows, amd64/arm64)
- deb/rpm packages for Linux
- Build from source
