<p align="center">
  <img src="assets/logo.png" alt="gocode — Claude Code rewritten in Go. The fastest AI coding agent runtime." width="500" />
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go" />
  <img src="https://img.shields.io/badge/Claude_Code-Go_Port-E34F26?style=for-the-badge" alt="Claude Code Go Port" />
  <img src="https://img.shields.io/badge/Multi--Model-Claude_GPT_Gemini_Grok-blueviolet?style=for-the-badge" alt="Multi-Model" />
  <img src="https://img.shields.io/badge/MCP-Protocol_Compliant-blueviolet?style=for-the-badge" alt="MCP Protocol Compliant" />
  <img src="https://img.shields.io/badge/Cursor-Ready-green?style=for-the-badge" alt="Cursor MCP Ready" />
  <img src="https://img.shields.io/badge/Kiro-Integrated-orange?style=for-the-badge" alt="Kiro Integrated" />
  <img src="https://img.shields.io/badge/VS_Code-Ready-007ACC?style=for-the-badge" alt="VS Code MCP Ready" />
  <img src="https://img.shields.io/badge/Antigravity-Ready-purple?style=for-the-badge" alt="Antigravity MCP Ready" />
  <img src="https://img.shields.io/badge/License-MIT-green?style=for-the-badge" alt="MIT License" />
</p>

<h1 align="center">gocode — Claude Code, Rewritten in Go. Now Multi-Model.</h1>

<h3 align="center">The Go version of Claude Code. One binary. Zero dependencies. 20× faster.<br/>Now with multi-agent orchestration, model fallback, and IDE-level tools — from your terminal.</h3>

<p align="center">
  <strong>An open-source Go reimplementation of the Claude Code AI coding agent.</strong><br/>
  We took the Claude Code architecture — the AI agent runtime that powers tool orchestration, session management, and prompt routing — and rebuilt every subsystem in Go. Then we went further: multi-model support, multi-agent orchestration, model fallback chains, IDE-level tooling, and a skills system. One binary, any LLM, instant startup.
</p>

<p align="center">
  <code>go install github.com/AlleyBo55/gocode/cmd/gocode@latest</code>
</p>

---

## Why gocode Exists

Every great product starts with a simple observation.

Claude Code is a remarkable piece of engineering. The way it decomposes tool orchestration. The way it manages sessions. The way it routes prompts. The architecture is elegant. The ideas are right.

But the implementation carries weight it doesn't need. And it only works with Claude.

We asked two questions: **what if the best AI coding agent was also the fastest?** And **what if it worked with any model?**

Not a wrapper. Not a thin binding. A complete, ground-up reimplementation in Go — every registry, every scoring algorithm, every subsystem — enhanced with multi-model support, a standalone terminal agent, a production-grade MCP server, and native IDE integrations that the original never had.

gocode starts in under 10 milliseconds. It ships as a single 12MB binary. It works with Claude, GPT-4o, Gemini, and Grok. You download it. You run it. That's it.

---

## Two Modes. One Binary.

gocode does something no other tool does: it's both a **standalone AI coding agent** and a **full MCP server** in a single 12MB binary.

| Mode | What It Does | How You Use It |
|------|-------------|----------------|
| **Agent Mode (TUI)** | Full terminal UI with split panels, diff viewer, streaming. Talk to any LLM. | `gocode chat` |
| **Agent Mode (REPL)** | Line-based chat for scripts and pipes. | `gocode chat --no-tui` |
| **API Server Mode** | Headless HTTP REST API for remote clients. | `gocode serve` |
| **MCP Server Mode** | Plug into Cursor, Kiro, VS Code, Antigravity, or Claude Desktop. | `gocode mcp-serve` |

You don't have to choose. You get both.

---

## What's New in v0.7.0

We took the best ideas from Claude Code, OpenCode, and Claw Code — and built them all in Go. One binary. Every feature.

### 🖥 Full Terminal UI

A bubbletea-powered TUI launches by default. Split panels, colored diff viewer, mode switching, Go-themed design.

- Chat panel on the left, git diff viewer on the right (Ctrl+D to toggle)
- Tab to switch between Build mode (full access) and Plan mode (read-only)
- 4 built-in themes: `golang`, `monokai`, `dracula`, `nord`
- Custom themes via `.gocode/theme.json`

```bash
gocode chat --model sonnet              # TUI launches by default
gocode chat --model gpt5 --theme nord   # with Nord theme
gocode chat --no-tui                    # fall back to line-based REPL
```

### 🧠 It Never Stops

Model hits a rate limit? Falls through to the next in the chain. Context window full? Compacts and keeps going. API down? Exponential backoff, three retries, automatic recovery.

### 🤝 It Delegates

The orchestrator breaks complex tasks into pieces and hands them to specialists — a deep researcher, a planner, a debugger — each with its own model, context, and tool permissions. Up to five agents running in parallel.

### 🔧 IDE-Level Tools + Auto-Format

LSP integration, AST-grep, tmux sessions, MCP client. Plus auto-format after every edit — gofmt, prettier, black, rustfmt.

### 🌐 HTTP API Server + Remote Access

```bash
gocode serve --addr :3000           # headless API server
gocode auth generate my-phone       # secure remote access
```

### 🖼 Multimodal Input

Include an image path in your message — it gets base64-encoded and sent as vision input. Works in both REPL and TUI.

### 📋 18 Slash Commands

`/help` `/exit` `/clear` `/compact` `/cost` `/model` `/skill` `/plan` `/init-deep` `/diff` `/undo` `/redo` `/status` `/review` `/permissions` `/doctor` `/connect` `/share`

### 🔧 CLI Commands

| Command | What It Does |
|---------|-------------|
| `gocode serve` | Headless HTTP API server |
| `gocode stats` | Usage statistics across all sessions |
| `gocode export/import` | Session import/export |
| `gocode pr` | Create GitHub PR via `gh` CLI |
| `gocode github` | List GitHub issues via `gh` CLI |
| `gocode auth` | Manage remote access keys |

> **[Advanced Features →](docs/advanced-features.md)** · **[UX Features →](docs/ux-features.md)**

---

## Skills — Expertise on Demand

Here's the thing about coding agents: they're generalists. They know a little about everything and a lot about nothing. That's fine for "add a comment to line 42." It's not fine for "build me a pixel-perfect clone of this website" or "refactor this Go codebase to follow idiomatic error handling."

Skills change that. One flag, and your agent becomes a specialist.

```bash
gocode chat --skill golang-best-practices    # now it writes Go like a senior engineer
gocode chat --skill nothing-design           # now it designs like Teenage Engineering
gocode chat --skill clone-website            # now it reverse-engineers websites
```

Or switch mid-session:

```
you> /skill react-best-practices
Skill react-best-practices activated.
you> review this component for performance issues
```

### 8 Built-in Skills

| Skill | What It Does | Activate With |
|-------|-------------|---------------|
| `git-master` | Atomic commits, interactive rebase, clean history | `--skill git-master` |
| `frontend-ui-ux` | Design-first UI development, accessibility, semantic HTML | `--skill frontend-ui-ux` |
| `nothing-design` | Nothing-inspired monochrome design system. Swiss typography, OLED blacks, mechanical precision. | `--skill nothing-design` |
| `golang-best-practices` | Idiomatic Go — code style, error handling, testing patterns, naming conventions | `--skill golang-best-practices` |
| `clone-website` | Pixel-perfect website cloning. Extract CSS, download assets, rebuild in Next.js. | `--skill clone-website` |
| `nextjs-best-practices` | Next.js 15+ patterns — RSC boundaries, async APIs, data fetching, route handlers | `--skill nextjs-best-practices` |
| `react-best-practices` | React performance optimization — eliminate waterfalls, bundle size, re-render patterns | `--skill react-best-practices` |
| `web-design-guidelines` | Accessibility audit, responsive design, interaction states, WCAG compliance | `--skill web-design-guidelines` |

### Create Your Own

Drop a JSON file in `.gocode/skills/`:

```json
{
  "name": "my-skill",
  "system_prompt": "You are an expert in...",
  "tool_permissions": ["bashtool", "filereadtool", "fileedittool"]
}
```

It loads automatically on next startup.

### Community Skills — Standing on the Shoulders of Giants

The built-in skills are distilled from these open-source projects. We're grateful to the authors who shared their expertise with the community:

| Skill | Inspired By | Author |
|-------|------------|--------|
| `nothing-design` | [nothing-design-skill](https://github.com/dominikmartn/nothing-design-skill) | [@dominikmartn](https://github.com/dominikmartn) |
| `golang-best-practices` | [cc-skills-golang](https://github.com/samber/cc-skills-golang) | [@samber](https://github.com/samber) |
| `clone-website` | [ai-website-cloner-template](https://github.com/JCodesMore/ai-website-cloner-template) | [@JCodesMore](https://github.com/JCodesMore) |
| `nextjs-best-practices` | [claude-code-nextjs-skills](https://github.com/laguagu/claude-code-nextjs-skills) | [@laguagu](https://github.com/laguagu) |
| `react-best-practices` | [claude-code-nextjs-skills](https://github.com/laguagu/claude-code-nextjs-skills) | [@laguagu](https://github.com/laguagu) |
| `web-design-guidelines` | [claude-code-nextjs-skills](https://github.com/laguagu/claude-code-nextjs-skills) | [@laguagu](https://github.com/laguagu) |

---

## Supported Models

Use any of these out of the box. Just set the right API key and go.

| Provider | Models | Alias | Env Var |
|----------|--------|-------|---------|
| **Anthropic** | Claude Opus 4.6, Sonnet 4.6, Haiku 4.5 | `opus`, `sonnet`, `haiku` | `ANTHROPIC_API_KEY` |
| **OpenAI** | GPT-5.4, GPT-5.4 Mini, GPT-5.4 Nano, GPT-4o, o3, o4-mini, Codex | `gpt5`, `gpt54-mini`, `gpt54-nano`, `gpt4o`, `o3`, `o4-mini`, `codex` | `OPENAI_API_KEY` |
| **Google** | Gemini 3.1 Pro, Gemini 3 Flash, Gemini 2.5 Pro | `gemini`, `gemini-flash`, `gemini-2.5` | `GEMINI_API_KEY` |
| **xAI** | Grok 4.20 Beta, Grok 3, Grok 3 Mini | `grok`, `grok-3`, `grok-mini` | `XAI_API_KEY` |

Or pass any full model ID: `--model gpt-5.4`, `--model claude-sonnet-4-6`, `--model gemini-3.1-pro-preview`, `--model grok-4.20-beta`, etc.

---

## Installation

### One-Line Install (macOS / Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/AlleyBo55/gocode/main/install.sh | bash
```

Downloads the latest release binary for your platform and installs to `/usr/local/bin`. No Go required.

### One-Line Install (Windows PowerShell)

```powershell
irm https://raw.githubusercontent.com/AlleyBo55/gocode/main/install.ps1 | iex
```

Or download manually — see below.

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

**macOS / Linux:**

```bash
# Example: macOS Apple Silicon
curl -fsSL https://github.com/AlleyBo55/gocode/releases/latest/download/gocode_darwin_arm64.tar.gz | tar xz
sudo mv gocode /usr/local/bin/
```

**Windows (PowerShell):**

```powershell
# Download and extract
Invoke-WebRequest -Uri "https://github.com/AlleyBo55/gocode/releases/latest/download/gocode_windows_amd64.zip" -OutFile gocode.zip
Expand-Archive gocode.zip -DestinationPath .
# Move to a directory in your PATH, e.g.:
Move-Item gocode.exe "$env:LOCALAPPDATA\Microsoft\WindowsApps\gocode.exe"
```

### Linux Packages (deb/rpm)

```bash
# Debian/Ubuntu
curl -fsSL https://github.com/AlleyBo55/gocode/releases/latest/download/gocode_amd64.deb -o gocode.deb
sudo dpkg -i gocode.deb

# Fedora/RHEL
curl -fsSL https://github.com/AlleyBo55/gocode/releases/latest/download/gocode_amd64.rpm -o gocode.rpm
sudo rpm -i gocode.rpm
```

### Build from Source (all platforms)

```bash
git clone https://github.com/AlleyBo55/gocode.git
cd gocode
go build -o gocode ./cmd/gocode/    # on Windows: go build -o gocode.exe ./cmd/gocode/
sudo mv gocode /usr/local/bin/       # on Windows: move to a PATH directory
```

### Verify Installation

```bash
gocode --version
# gocode version v0.3.5
```

---

## Quickstart — 60 Seconds to Your First Agent Session

```bash
# 1. Install
go install github.com/AlleyBo55/gocode/cmd/gocode@latest

# 2. Set your API key (pick your provider)
export ANTHROPIC_API_KEY=sk-ant-...    # Claude
export OPENAI_API_KEY=sk-...           # GPT-4o
export GEMINI_API_KEY=AI...            # Gemini
export XAI_API_KEY=xai-...             # Grok

# 3. Chat
gocode chat

# Or run a one-shot prompt
gocode prompt "find all TODO comments in this project and list them"
```

That's it. No Python. No Node. No virtual environments. No config files. One binary, one env var, go.

---

## What Can gocode Do?

### As a Standalone Agent (`gocode chat` / `gocode prompt`)

- 🤖 **Autonomous coding** — reads your codebase, writes code, runs tests, fixes bugs
- 🔄 **Multi-turn tool loops** — Claude/GPT/Gemini calls tools, gets results, keeps going until done
- 🌊 **Real-time streaming** — see tokens as they arrive, not after the model finishes thinking
- 🔐 **Permission system** — asks before running dangerous commands
- 💰 **Token tracking** — `/cost` shows exactly how many tokens you've used
- 📝 **Session persistence** — resume conversations with `--resume`
- 🔀 **Model switching** — `--model gpt4o` today, `--model sonnet` tomorrow, same agent
- 🛡 **Hash-anchored edits** — content hashes prevent stale overwrites (`--hashline`)
- 🔁 **Model fallback** — automatic failover across providers on rate limits and errors
- 🧠 **Multi-agent orchestration** — delegate to specialist sub-agents running in parallel
- 📋 **Planning mode** — `/plan` runs an interview before any code is touched
- 🎯 **Skills system** — domain-tuned agent profiles from `.gocode/skills/`
- 🔬 **LSP integration** — real renames, go-to-definition, find-references via language servers
- 🌳 **AST-grep** — structural code search and rewrite, not regex
- 💻 **Tmux sessions** — persistent terminal sessions for REPLs and debuggers
- 🔌 **MCP client** — connect to external MCP servers for web search, docs, code search
- 📚 **Auto-context** — `/init-deep` generates project-wide AGENTS.md context files
- 🖥 **Full TUI** — bubbletea terminal UI with split panels, diff viewer, themes
- 🖼 **Multimodal** — send images inline with your messages
- 🌐 **HTTP API** — `gocode serve` for remote access from any client
- 🔑 **Auth keys** — secure remote access with `gocode auth generate`
- 🎨 **Themes** — golang, monokai, dracula, nord (or custom via JSON)
- ✨ **Auto-format** — gofmt, prettier, black, rustfmt after every edit
- 📊 **GitHub integration** — `gocode pr` and `gocode github` via gh CLI

### As an MCP Server (`gocode mcp-serve`)

- 🔌 **14 production tools** — shell, file I/O, grep, glob, workspace analysis
- 💻 **5 IDEs supported** — Cursor, Kiro, VS Code, Antigravity, Claude Desktop
- 🌐 **Dual transport** — stdio for IDEs, HTTP for anything else
- 📋 **Full MCP compliance** — initialize, tools/list, tools/call, ping, resources/list

### The Numbers

| Metric | Value |
|--------|-------|
| Startup time | **<10ms** |
| Binary size | **~12MB** |
| Runtime dependencies | **None** |
| Supported LLM providers | **4** (Anthropic, OpenAI, Google, xAI) |
| Supported IDEs | **5** (Cursor, Kiro, VS Code, Antigravity, Claude Desktop) |
| MCP tools | **14 built-in + external via MCP client** |
| Internal packages | **38** |
| Built-in agent profiles | **4** (coordinator, deep-worker, planner, debugger) |
| Built-in skills | **8** (git-master, frontend-ui-ux, nothing-design, golang, clone-website, nextjs, react, web-design) |
| Max concurrent background agents | **5** |

---

## Documentation

| Guide | Description |
|-------|-------------|
| 📖 **[Agent Mode Guide](docs/agent-mode.md)** | How to use `gocode chat` and `gocode prompt` — models, API keys, flags, slash commands, examples |
| 🚀 **[Advanced Features](docs/advanced-features.md)** | Multi-agent orchestration, model fallback, planning mode, skills, LSP, AST-grep, tmux, MCP client, context generation |
| 🎨 **[UX Features](docs/ux-features.md)** | Streaming responses, thinking blocks, slash commands, GOCODE.md project config, git context, cost estimation |
| 🔌 **[MCP & IDE Integration Guide](docs/mcp-ide-guide.md)** | How to connect gocode to Cursor, Kiro, VS Code, Antigravity, Claude Desktop |
| 🏗 **[Architecture](docs/architecture.md)** | Internal package structure, system diagrams, design decisions |
| 📚 **[CLI Reference](docs/cli-reference.md)** | Full list of all 23 CLI commands with flags and examples |

---

## Contributing

```bash
git clone https://github.com/AlleyBo55/gocode.git
cd gocode
make test    # Run all tests
make build   # Build the binary
```

Open a PR. Start a discussion. File an issue. Every contribution makes gocode better.

---

## The Numbers

| Metric | Claude Code (Python) | gocode (Go) |
|--------|---------------------|-------------|
| Startup time | ~200ms | **<10ms** (20× faster) |
| Binary size | N/A (interpreted) | **~12MB** (single file) |
| Runtime dependencies | Python 3.10+, pip, venv | **None** |
| LLM providers | Claude only | **Claude, GPT-5.4, Gemini 3.1, Grok 4.20** |
| Deployment | `pip install` + virtualenv | **Copy one file** |
| Concurrency model | asyncio / threading | **Goroutines + channels** |
| MCP compliance | N/A | **Full specification** |
| IDE integrations | N/A | **5 IDEs supported** |
| Standalone agent | Yes (Claude only) | **Yes (any model)** |
| Multi-agent orchestration | No | **Yes (4 built-in profiles)** |
| Model fallback | No | **Yes (automatic failover)** |
| Planning mode | No | **Yes (`/plan` command)** |
| Skills system | No | **Yes (8 built-in + custom JSON profiles)** |
| LSP integration | No | **Yes (rename, definition, references)** |
| MCP client | No | **Yes (connect to external servers)** |

---

## Search Keywords

gocode is the **Go version of Claude Code** — if you searched for any of these terms, you found the right project:

`claude code go` · `claude code golang` · `claude code alternative` · `claude code open source` · `claude code rewrite` · `claude code port` · `go claude code` · `golang claude code` · `claude code cli golang` · `ai coding agent go` · `ai coding agent golang` · `go ai agent` · `golang ai agent` · `mcp server go` · `mcp server golang` · `mcp golang` · `model context protocol go` · `cursor mcp server go` · `kiro mcp server` · `vscode mcp server golang` · `claude desktop mcp go` · `go ai coding assistant` · `golang ai coding tool` · `claude code go port` · `claude code go version` · `claude code reimplementation` · `open source claude code` · `claude code alternative golang` · `fast ai agent go` · `lightweight ai agent` · `single binary ai agent` · `multi model ai agent` · `gpt4o coding agent` · `gemini coding agent` · `grok coding agent` · `multi agent orchestration go` · `agent fallback chain` · `lsp integration go agent` · `ast grep go` · `mcp client golang` · `ai planning agent` · `agent skills system` · `background agents golang` · `claude code skills` · `ai agent skills` · `coding agent skills golang` · `nothing design system ai` · `website cloner ai agent` · `nextjs best practices agent` · `react performance agent` · `golang best practices agent` · `web design review ai` · `pixel perfect clone agent` · `ai coding assistant terminal` · `claude code go alternative` · `openai agent go` · `gemini agent golang` · `grok agent terminal` · `autonomous coding agent` · `ai pair programmer terminal` · `claude code replacement` · `claude code open source go` · `ai agent with skills` · `domain expert ai agent` · `go ai coding tool cli`

---

## License

MIT — use it, fork it, ship it.

---

<p align="center">
  <em>"The people who are crazy enough to think they can change the world are the ones who do."</em>
</p>

<p align="center">
  <strong>gocode — the Go version of Claude Code. Now a multi-agent operating system.</strong><br/>
  One binary. Zero dependencies. Instant startup. Any LLM. A team of agents.<br/>
  This is what an AI coding agent should feel like.
</p>

<p align="center">
  ⭐ Star this repo if you believe developer tools should be fast, simple, and open.
</p>
