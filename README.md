<p align="center">
  <img src="assets/logo.png" alt="gocode — the fastest open-source AI coding agent. One binary. Any model." width="500" />
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go" />
  <img src="https://img.shields.io/badge/200+_Models-11_Providers-blueviolet?style=for-the-badge" alt="200+ Models" />
  <img src="https://img.shields.io/badge/Multi--Agent-Orchestration-E34F26?style=for-the-badge" alt="Multi-Agent" />
  <img src="https://img.shields.io/badge/MCP-Protocol_Compliant-blueviolet?style=for-the-badge" alt="MCP Protocol Compliant" />
  <img src="https://img.shields.io/badge/Cursor-Ready-green?style=for-the-badge" alt="Cursor MCP Ready" />
  <img src="https://img.shields.io/badge/Kiro-Integrated-orange?style=for-the-badge" alt="Kiro Integrated" />
  <img src="https://img.shields.io/badge/VS_Code-Ready-007ACC?style=for-the-badge" alt="VS Code MCP Ready" />
  <img src="https://img.shields.io/badge/Antigravity-Ready-purple?style=for-the-badge" alt="Antigravity MCP Ready" />
  <img src="https://img.shields.io/badge/License-MIT-green?style=for-the-badge" alt="MIT License" />
</p>

<h1 align="center">gocode — The Open-Source Claude Code Alternative. Built in Go. Works With Any Model.</h1>

<p align="center">
  <img src="assets/screen1.png" alt="gocode terminal screenshot" width="700" />
</p>

<h3 align="center">One binary. Zero dependencies. 200+ models. A team of agents.<br/>Inspired by the best terminal AI agents. Built from scratch in Go. Faster than everything else.</h3>

<p align="center">
  <code>go install github.com/AlleyBo55/gocode/cmd/gocode@latest</code>
</p>

---

## Why gocode

We loved what Claude Code did for terminal-based AI coding. But we wanted something faster, model-agnostic, and dependency-free.

So we built gocode from scratch in Go — inspired by the best ideas in terminal AI agents, but with a completely original architecture. Every subsystem was designed and written from the ground up: the provider layer, the agent runtime, the tool executor, the orchestrator, the memory system, the planning engine. All of it.

The result: a single 12MB binary that starts in under 10 milliseconds, works with 200+ models across 11 providers, and ships with capabilities that most agents don't even attempt.

---

## What Makes gocode Different

### Any Model. Your Choice.

Most terminal agents lock you into one provider. gocode works with all of them. Claude, GPT, Gemini, Grok, DeepSeek, Mistral, Llama, local models — set one env var and go. Or use OpenRouter and access every model with a single API key.

### A Team of Agents, Not Just One

gocode doesn't run a single agent loop. It runs an orchestrator with specialist sub-agents that plan, coordinate, and delegate to each other. Up to 5 concurrent background agents, each with their own model preference and tool permissions.

### Memory That Persists — and Dreams

Cross-session memory that remembers your preferences, project conventions, and architectural decisions. Plus a dream system that autonomously consolidates and prunes memory during idle periods, keeping what matters and forgetting what doesn't.

### IDE-Level Tooling in Your Terminal

Real LSP integration (actual renames, actual go-to-definition — not regex). AST-grep for structural code search. A WebSocket bridge that connects to VS Code, Cursor, Kiro, and Antigravity. This isn't a chatbot with file access. It's an agent with IDE capabilities.

### Instant. Always.

Under 10ms startup. 12MB binary. No runtime dependencies. No Python. No Node. No virtual environments. `go install` and you're done.

---

## What's New

### v0.9.0 — One More Thing.

Eighteen new capabilities. Eight new skills that change how the agent thinks. A dream system that consolidates memory while you sleep. A planning engine that delegates to the strongest model in the room. Vim keybindings. A cron scheduler. A swarm of agents that talk to each other. A WebSocket bridge for IDE integration. PDF reading. Output styles. And a buddy system — because even an AI agent deserves a companion.

### v0.8.0 — The Universal Model Layer

200+ models. 11 providers. Local inference. Persistent memory. Task management. Web search. The agent can delegate to specialists and remember what you told it last week.

### v0.7.0 — The Agent Operating System

Full terminal UI. Multi-agent orchestration. Model fallback. 21 slash commands. Skills system. Plugin architecture. LSP integration. AST-grep.

> **[Full Changelog →](CHANGELOG.md)**

---

## Modes

| Mode | What It Does | How You Use It |
|------|-------------|----------------|
| **Agent Mode (REPL)** | Line-based chat. Default mode. | `gocode chat` |
| **Agent Mode (TUI)** | Full terminal UI with split panels, diff viewer, themes. | `gocode chat --tui` |
| **API Server Mode** | Headless HTTP REST API for remote clients. | `gocode serve` |
| **MCP Server Mode** | Plug into Cursor, Kiro, VS Code, Antigravity, or Claude Desktop. | `gocode mcp-serve` |

---

## Supported Models

Every model. Every provider. One binary. No lock-in.

**4 native providers. 7 proxy services. Local inference. 200+ models.** Set one env var and go.

| Provider | Highlights | Env Var |
|----------|-----------|---------|
| **Anthropic** | Claude Opus 4.6, Sonnet 4.6, Haiku 4.5 | `ANTHROPIC_API_KEY` |
| **OpenAI** | GPT-5.4, o3, o4-mini, Codex | `OPENAI_API_KEY` |
| **Google** | Gemini 3.1 Pro, Gemini 3 Flash | `GEMINI_API_KEY` |
| **xAI** | Grok 4.20 Beta, Grok 3 | `XAI_API_KEY` |
| **DeepSeek** | DeepSeek Chat, R1 Reasoner, Coder | `DEEPSEEK_API_KEY` |
| **Mistral** | Mistral Large, Codestral, Pixtral | `MISTRAL_API_KEY` |
| **Groq** | Llama 3.3 70B at 800 tok/s | `GROQ_API_KEY` |
| **Together AI** | Llama 405B, Qwen 72B Turbo | `TOGETHER_API_KEY` |
| **OpenRouter** | 200+ models, one API key | `OPENROUTER_API_KEY` |
| **Azure OpenAI** | Enterprise GPT deployments | `AZURE_OPENAI_API_KEY` |
| **Local (Ollama/LM Studio)** | Run any model on your machine | `OPENAI_BASE_URL` |

```bash
gocode chat --model sonnet          # Claude
gocode chat --model gpt5            # GPT-5.4
gocode chat --model deepseek        # DeepSeek
gocode chat --model groq-llama      # Llama on Groq (800 tok/s)
gocode chat --model llama           # Ollama local
gocode chat --goal coding           # auto-pick the best coding model
```

### One Key. Every Model.

Don't want to manage 11 API keys? Set one OpenRouter key and access every model from every provider.

```bash
export OPENROUTER_API_KEY=sk-or-your-key

gocode chat --model openai/gpt-4o                         # GPT-4o
gocode chat --model anthropic/claude-sonnet-4-20250514    # Claude Sonnet
gocode chat --model google/gemini-2.5-pro-preview         # Gemini
gocode chat --model x-ai/grok-3                           # Grok
gocode chat --model deepseek/deepseek-chat                # DeepSeek
gocode chat --model meta-llama/llama-3.3-70b-instruct     # Llama
gocode chat --model mistralai/mistral-large-latest        # Mistral
```

One binary. One key. Every model on the planet. Get your key at [openrouter.ai/keys](https://openrouter.ai/keys).

> **[Full Model List — 200+ Models →](docs/supported-models.md)**

---

## The Full Feature Set

### Multi-Agent Orchestration
- 4 built-in sub-agent profiles: coordinator, deep-worker, planner, debugger
- Up to 5 concurrent background agents with independent contexts
- Agent-to-agent messaging via swarm coordination
- Category-based model routing (deep/quick/visual/ultrabrain)
- Automatic model fallback on rate limits and server errors

### Planning & Strategy
- `/plan` — interview-style planning sessions
- `/ultraplan` — deep planning with strongest available model (background, 30min timeout)
- Structured plan output with scope analysis and step-by-step blueprints

### Memory & Persistence
- Cross-session memory with 3 scopes and team sync
- Dream system — autonomous memory consolidation (orient → gather → consolidate → prune)
- Session persistence and resume (`-c` / `-r` flags)
- Memory aging and relevance-based pruning

### IDE-Level Tools
- LSP integration — real renames, go-to-definition, find-references, diagnostics
- AST-grep — structural code search and rewrite (Go, JS, TS, Python)
- Hash-anchored file I/O — CRC32 line hashes prevent stale edits
- Auto-format on save (gofmt, prettier, black, rustfmt)

### IDE Bridge
- WebSocket server for bidirectional IDE communication
- Works with VS Code, Cursor, Kiro, Antigravity
- Permission forwarding and real-time response streaming
- Multiple concurrent IDE connections

### Terminal UI
- Full bubbletea TUI with split panels (chat + git diff viewer)
- 4 built-in themes: golang, monokai, dracula, nord
- Custom themes via `.gocode/theme.json`
- Custom keybinds via `.gocode/keybinds.json`
- Vim keybindings — full normal/insert/visual modes with motions, operators, text objects

### MCP Server & Client
- Full MCP protocol compliance (server mode for IDEs)
- MCP client — connect to external MCP servers for extended capabilities
- 14+ built-in tools
- Dual transport: stdio and HTTP

### Skills System
- 16 built-in skills (see below)
- Custom skills via `.gocode/skills/` JSON files
- Mid-session skill switching with `/skill`
- Skills with MCP server configs auto-start child processes

### Scheduling & Automation
- Cron scheduler with 5-field expressions
- Background agent execution on schedule
- Persistent schedules in `.gocode/cron.json`
- GitHub Actions integration (`gocode-action` for PR review and issue implementation)

### Session & Git
- Git checkpoints with `/undo N` and per-session refs
- Git worktree tools for parallel branch work
- `/diff` — see changes made this session
- `/review` — agent self-reviews its own changes
- `/commit` — auto-generated commit messages

### More
- PDF reading (text extraction, 50MB limit, pure Go)
- Web search (DuckDuckGo, no API key needed)
- Web fetch (URL content extraction)
- Structured JSON output with `--output-format json` and `--output-schema`
- Output styles (concise, verbose, markdown, minimal) + custom styles
- Notebook editing (Jupyter .ipynb cell-level operations)
- Tmux persistent terminal sessions
- Plugin system with hook pipeline
- Migrations system for automatic config upgrades
- 25+ slash commands
- 23 CLI subcommands

---

## Skills — Expertise on Demand

One flag, and your agent becomes a specialist.

```bash
gocode chat --skill golang-best-practices    # writes Go like a senior engineer
gocode chat --skill nothing-design           # designs like Teenage Engineering
gocode chat --skill loop                     # autonomous keep-going mode
```

### 16 Built-in Skills

| Skill | What It Does |
|-------|-------------|
| `git-master` | Atomic commits, interactive rebase, clean history |
| `frontend-ui-ux` | Design-first UI development, accessibility, semantic HTML |
| `nothing-design` | Nothing-inspired monochrome design. Swiss typography, OLED blacks. |
| `golang-best-practices` | Idiomatic Go — code style, error handling, testing, naming |
| `clone-website` | Pixel-perfect website cloning. Extract CSS, rebuild in Next.js. |
| `nextjs-best-practices` | Next.js 15+ patterns — RSC, async APIs, data fetching |
| `react-best-practices` | React performance — eliminate waterfalls, bundle size, re-renders |
| `web-design-guidelines` | Accessibility audit, responsive design, WCAG compliance |
| `loop` | Autonomous keep-going mode — works until the task is done |
| `stuck` | Recovery mode for confused or frozen agent sessions |
| `debug` | Structured troubleshooting — reproduce, isolate, fix, verify |
| `verify` | Double-check work by re-reading files, running tests, validating |
| `simplify` | Code review for complexity reduction and dead code removal |
| `remember` | Active memory management — save facts and preferences to memdir |
| `skillify` | Meta-skill — capture conversation patterns as reusable skill JSON |
| `batch` | Parallel batch processing across multiple files or worktree agents |

Create your own — drop a JSON file in `.gocode/skills/`.

### Community Skills — Standing on the Shoulders of Giants

| Skill | Inspired By | Author |
|-------|------------|--------|
| `nothing-design` | [nothing-design-skill](https://github.com/dominikmartn/nothing-design-skill) | [@dominikmartn](https://github.com/dominikmartn) |
| `golang-best-practices` | [cc-skills-golang](https://github.com/samber/cc-skills-golang) | [@samber](https://github.com/samber) |
| `clone-website` | [ai-website-cloner-template](https://github.com/JCodesMore/ai-website-cloner-template) | [@JCodesMore](https://github.com/JCodesMore) |
| `nextjs-best-practices` | [claude-code-nextjs-skills](https://github.com/laguagu/claude-code-nextjs-skills) | [@laguagu](https://github.com/laguagu) |
| `react-best-practices` | [claude-code-nextjs-skills](https://github.com/laguagu/claude-code-nextjs-skills) | [@laguagu](https://github.com/laguagu) |
| `web-design-guidelines` | [claude-code-nextjs-skills](https://github.com/laguagu/claude-code-nextjs-skills) | [@laguagu](https://github.com/laguagu) |

---

## Installation

### One-Line Install (macOS / Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/AlleyBo55/gocode/main/install.sh | bash
```

### One-Line Install (Windows PowerShell)

```powershell
irm https://raw.githubusercontent.com/AlleyBo55/gocode/main/install.ps1 | iex
```

### Go Install (all platforms, requires Go 1.21+)

```bash
go install github.com/AlleyBo55/gocode/cmd/gocode@latest
```

### Download Binary Manually

Grab the binary for your platform from [GitHub Releases](https://github.com/AlleyBo55/gocode/releases):

| Platform | File |
|----------|------|
| macOS (Apple Silicon) | `gocode_*_darwin_arm64.tar.gz` |
| macOS (Intel) | `gocode_*_darwin_amd64.tar.gz` |
| Linux (x86_64) | `gocode_*_linux_amd64.tar.gz` |
| Linux (ARM64) | `gocode_*_linux_arm64.tar.gz` |
| Windows (x86_64) | `gocode_*_windows_amd64.zip` |
| Windows (ARM64) | `gocode_*_windows_arm64.zip` |

### Linux Packages (deb/rpm)

```bash
# Debian/Ubuntu
curl -fsSL https://github.com/AlleyBo55/gocode/releases/latest/download/gocode_amd64.deb -o gocode.deb
sudo dpkg -i gocode.deb

# Fedora/RHEL
curl -fsSL https://github.com/AlleyBo55/gocode/releases/latest/download/gocode_amd64.rpm -o gocode.rpm
sudo rpm -i gocode.rpm
```

### Build from Source

```bash
git clone https://github.com/AlleyBo55/gocode.git
cd gocode
go build -o gocode ./cmd/gocode/
sudo mv gocode /usr/local/bin/
```

### Verify

```bash
gocode --version
```

---

## Quickstart

```bash
# 1. Install
go install github.com/AlleyBo55/gocode/cmd/gocode@latest

# 2. Set your API key (pick any provider)
export ANTHROPIC_API_KEY=sk-ant-...

# 3. Chat
gocode chat

# Or one-shot
gocode prompt "find all TODO comments in this project"
```

No Python. No Node. No virtual environments. One binary, one env var, go.

---

## Architecture

26 internal packages. Clean interfaces. Zero external runtime dependencies. Goroutines and channels for native concurrency. `go:embed` for compiled-in registries. Atomic file writes for zero-corruption session persistence.

```
gocode/
├── cmd/gocode/          # CLI entrypoint — 23 subcommands
├── data/                # Embedded command/tool registries
├── internal/
│   ├── agent/           # ConversationRuntime, ToolExecutor, permissions, hooks
│   ├── apiclient/       # Provider interface, Anthropic/OpenAI/Gemini/xAI/proxy providers
│   ├── orchestrator/    # Multi-agent orchestration, background agents
│   ├── swarm/           # Agent-to-agent messaging, discovery registry
│   ├── ultraplan/       # Deep planning with strongest model
│   ├── dream/           # Autonomous memory consolidation
│   ├── lsp/             # Language Server Protocol integration
│   ├── astgrep/         # Structural code search
│   ├── bridge/          # WebSocket IDE bridge
│   ├── mcp/             # MCP server (full protocol)
│   ├── mcpclient/       # MCP client (external servers)
│   ├── tui/             # Bubbletea terminal UI
│   ├── repl/            # Interactive REPL
│   ├── vim/             # Vim keybinding engine
│   ├── skills/          # Skills system
│   ├── cron/            # Cron scheduler
│   ├── session/         # Session persistence
│   ├── memory/          # Persistent memory
│   ├── buddy/           # Terminal companion system
│   └── ...              # 6 more packages
```

> **[Full Architecture Guide →](docs/architecture.md)**

---

## Documentation

| Guide | Description |
|-------|-------------|
| 📖 **[Agent Mode Guide](docs/agent-mode.md)** | Models, API keys, flags, slash commands, examples |
| 🌍 **[Supported Models](docs/supported-models.md)** | 200+ models across 11 providers |
| 🚀 **[Advanced Features](docs/advanced-features.md)** | Multi-agent, fallback, planning, skills, LSP, AST-grep |
| 🎨 **[UX Features](docs/ux-features.md)** | Streaming, thinking blocks, slash commands, cost estimation |
| 🔌 **[MCP & IDE Guide](docs/mcp-ide-guide.md)** | Cursor, Kiro, VS Code, Antigravity, Claude Desktop |
| 🏗 **[Architecture](docs/architecture.md)** | Internal packages, system diagrams, design decisions |
| 📚 **[CLI Reference](docs/cli-reference.md)** | All CLI commands with flags and examples |
| 📋 **[Changelog](CHANGELOG.md)** | Full version history |

---

## gocode vs Claude Code

gocode is inspired by Claude Code but built from scratch with a different architecture, different language, and a broader vision. Here's how they compare:

| Metric | Claude Code (Node.js) | gocode (Go) |
|--------|----------------------|-------------|
| Startup time | ~200ms | **<10ms** |
| Binary size | ~180MB (node_modules) | **~12MB** (single file) |
| Runtime dependencies | Node.js 18+, npm | **None** |
| LLM providers | Claude only | **200+ models, 11 providers** |
| Deployment | `npm install -g` | **Copy one file** |
| Concurrency | Node.js async/await | **Goroutines + channels** |
| MCP support | Yes (client + server) | **Yes (client + server)** |
| IDE integrations | VS Code, JetBrains, Web, Desktop | **5 IDEs via MCP** |
| Multi-agent / subagents | Yes | **Yes (4 profiles, 5 concurrent)** |
| Model fallback | No | **Yes (automatic failover)** |
| Skills system | Yes | **Yes (16 built-in + custom)** |
| Custom slash commands | Yes | **Yes** |
| Hooks (lifecycle) | Yes | **Yes** |
| Web search | Yes | **Yes (built-in, no API key)** |
| Persistent memory | Yes | **Yes (3 scopes, aging, team sync)** |
| Git checkpoints / rewind | Yes | **Yes (/undo N, per-session refs)** |
| Git worktree tools | Yes | **Yes** |
| Task management tools | Yes | **Yes (+ background agents)** |
| Notebook editing | Yes | **Yes** |
| GitHub Actions | Yes | **Yes (gocode-action)** |
| Structured output | Yes | **Yes (--output-format json, --output-schema)** |
| Session continue | Yes | **Yes (-c / -r flags)** |
| Vim keybindings | Yes | **Yes (full normal/insert/visual modes)** |
| Deep planning | Yes | **Yes (/ultraplan, background, 30min timeout)** |
| Dream system | Yes | **Yes (orient→gather→consolidate→prune)** |
| Cron/scheduled tasks | Yes | **Yes (5-field cron, background agents)** |
| IDE bridge | Yes | **Yes (WebSocket, bidirectional)** |
| Swarm coordination | Yes | **Yes (agent-to-agent messaging)** |
| PDF handling | Yes | **Yes (50MB limit, pure Go)** |
| Output styles | Yes | **Yes (4 built-in + custom)** |
| Buddy system | No | **Yes (18 species, deterministic gacha)** |
| Multi-model support | No (Claude only) | **Yes (200+ models, 11 providers)** |
| Category-based routing | No | **Yes (deep/quick/visual/ultrabrain)** |
| Hash-anchored file I/O | No | **Yes (CRC32 line hashes)** |
| AST-grep integration | No | **Yes (structural code search)** |
| Tmux sessions | No | **Yes (persistent terminal sessions)** |
| TUI mode | No | **Yes (bubbletea split panels, themes)** |

---

## Contributing

```bash
git clone https://github.com/AlleyBo55/gocode.git
cd gocode
make test && make build
```

---

## The Buddy System

One more thing. gocode comes with a terminal companion. 18 species across 5 rarity tiers. Deterministic gacha seeded from your user ID. Tracks DEBUGGING, CHAOS, and SNARK stats. Displays ASCII sprites in your REPL banner.

Because even an AI agent deserves a friend.

```
you> /buddy
```

---

## License

MIT — use it, fork it, ship it.

---

## Search Keywords

`claude code alternative` · `claude code replacement` · `claude code open source` · `open source claude code` · `ai coding agent go` · `go ai agent` · `mcp server go` · `cursor mcp server go` · `kiro mcp server` · `vscode mcp server golang` · `fast ai agent go` · `single binary ai agent` · `multi model ai agent` · `multi agent orchestration go` · `deepseek coding agent` · `groq fast inference agent` · `ollama coding agent` · `local llm coding agent` · `200 models ai agent` · `openai compatible agent` · `ai pair programmer terminal` · `terminal coding agent golang` · `claude code but faster` · `claude code go alternative`

---

<p align="center">
  <em>Built from scratch. Built in Go. Built to be fast, open, and yours.</em>
</p>

<p align="center">
  <strong>gocode — the open-source Claude Code alternative. 200+ models. Multi-agent. Instant startup.</strong><br/>
  One binary. Zero dependencies. Any LLM. A team of agents.
</p>

<p align="center">
  ⭐ Star this repo if you believe developer tools should be fast, simple, and open.
</p>
