<p align="center">
  <img src="assets/logo.png" alt="gocode — The fastest multi-model AI coding agent." width="500" />
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go" />
  <img src="https://img.shields.io/badge/Claude-Supported-E34F26?style=for-the-badge" alt="Claude" />
  <img src="https://img.shields.io/badge/GPT--4o-Supported-74aa9c?style=for-the-badge" alt="GPT-4o" />
  <img src="https://img.shields.io/badge/Gemini-Supported-4285F4?style=for-the-badge" alt="Gemini" />
  <img src="https://img.shields.io/badge/Grok-Supported-000000?style=for-the-badge" alt="Grok" />
  <img src="https://img.shields.io/badge/MCP-Protocol-blueviolet?style=for-the-badge" alt="MCP" />
  <img src="https://img.shields.io/badge/License-MIT-green?style=for-the-badge" alt="MIT License" />
</p>

<h1 align="center">gocode — One Agent. Every Model. Zero Dependencies.</h1>

<h3 align="center">A standalone AI coding agent + MCP server in a single Go binary.<br/>Use it from your terminal. Use it from your IDE. Use any LLM you want.</h3>

<p align="center">
  <code>go install github.com/AlleyBo55/gocode/cmd/gocode@latest</code>
</p>

---

## Two Modes. One Binary.

gocode does something no other tool does: it's both a **standalone AI coding agent** and a **full MCP server** in a single 12MB binary.

| Mode | What It Does | How You Use It |
|------|-------------|----------------|
| **Agent Mode** | Talk to any LLM directly from your terminal. It reads files, runs commands, edits code — autonomously. | `gocode chat` or `gocode prompt "..."` |
| **MCP Server Mode** | Plug into Cursor, Kiro, VS Code, Antigravity, or Claude Desktop as a tool server. | `gocode mcp-serve` |

You don't have to choose. You get both.

---

## Supported Models

Use any of these out of the box. Just set the right API key and go.

| Provider | Models | Alias | Env Var |
|----------|--------|-------|---------|
| **Anthropic** | Claude Opus 4, Sonnet 4, Haiku 4.5 | `opus`, `sonnet`, `haiku` | `ANTHROPIC_API_KEY` |
| **OpenAI** | GPT-4o, GPT-4o-mini, o1, o3, o4-mini, Codex | `gpt4o`, `gpt4-mini`, `o1`, `o3`, `o4-mini`, `codex` | `OPENAI_API_KEY` |
| **Google** | Gemini 2.5 Pro, Gemini 2.5 Flash | `gemini`, `gemini-pro`, `gemini-flash` | `GEMINI_API_KEY` |
| **xAI** | Grok 3, Grok 3 Mini, Grok 2 | `grok`, `grok-mini`, `grok-2` | `XAI_API_KEY` |

Or pass any full model ID: `--model gpt-4o-2024-08-06`, `--model claude-sonnet-4-6`, etc.

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
| MCP tools | **14** |
| Internal packages | **26** |

---

## Documentation

| Guide | Description |
|-------|-------------|
| 📖 **[Agent Mode Guide](docs/agent-mode.md)** | How to use `gocode chat` and `gocode prompt` — models, API keys, flags, slash commands, examples |
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

## License

MIT — use it, fork it, ship it.

---

<p align="center">
  <strong>gocode — one agent, every model, zero dependencies.</strong><br/>
  The AI coding agent that works the way you do.
</p>

<p align="center">
  ⭐ Star this repo if you believe developer tools should be fast, simple, and open.
</p>
