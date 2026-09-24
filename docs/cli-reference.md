# CLI Reference

[← Back to README](../README.md)

---

## `gocode` with no arguments

In a terminal, `gocode` starts the interactive chat, exactly like `gocode chat`.
`gocode --model X` is shorthand for `gocode chat --model X`. When stdin or
stdout is not a terminal (a pipe, a script, CI) it prints help and exits 0, so
nothing ever blocks waiting for input.

What it does with no configuration:

| You have | What happens |
|---|---|
| A provider key exported (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `OPENROUTER_API_KEY`, ...) | Uses it, default model `sonnet` |
| No key, but Ollama or LM Studio running with models installed | Picks a coding model from it, prints which one on stderr, starts |
| No key, no local server | Prints three ways to get started (local and free, one OpenRouter key, your own key) and exits 1 |

Local models need no configuration: any id with an Ollama-style tag
(`qwen2.5-coder:7b`, `llama3.3:70b`, or the `llama` alias) is routed to Ollama
at `localhost:11434`, or LM Studio at `localhost:1234`, whichever answers.
Set `OLLAMA_HOST` for a non-default address; `OPENAI_BASE_URL` still overrides
everything when set.

`gocode --help` groups commands into Agent, Servers and integrations, Sessions
and settings, and Diagnostics. The porting-harness commands (`manifest`,
`parity-audit`, `bootstrap-graph`, `teleport-mode`, ...) are hidden from help
but still run.

---

## Agent Commands

| Command | Description |
|---------|-------------|
| `chat` | Start interactive agent chat session |
| `prompt [text]` | Run a single prompt through the agent and exit |

### `gocode chat`

```
Flags:
  --model string          Model name or alias (default "sonnet")
  --max-turns int         Max agent loop iterations (default 30)
  --max-tokens int        Max output tokens per request (default 8192)
  --max-cost float        Stop when the session's estimated cost reaches this many USD (0 = unlimited)
  --api-key string        API key override
  --resume string         Resume a saved session by ID
  --output-style string   Output style: concise, verbose, markdown, minimal (default "markdown")
  --vim                   Enable vim keybindings in REPL input
  --bridge                Start WebSocket bridge server alongside REPL
```

`--max-cost` is computed from the per-model price table, the same one `/cost`
uses. If the model has no price entry the flag cannot be enforced and gocode
says so at startup. When the limit is hit the loop stops before running any
pending tool calls and the session stays valid; raise the limit or start a new
session to continue.

The agent loop also stops on its own when the model repeats the same tool
calls with the same results five iterations in a row (it is warned in the
tool result after three). This catches weak models that would otherwise spend
every turn re-reading the same file.

### `gocode prompt`

```
Flags:
  --model string      Model name or alias (default "sonnet")
  --max-turns int     Max agent loop iterations (default 30)
  --max-tokens int    Max output tokens per request (default 8192)
  --max-cost float    Stop when the run's estimated cost reaches this many USD (0 = unlimited)
  --api-key string    API key override
  --no-stream         Wait for full response before printing
```

`--max-cost` is the flag to set in CI and in the GitHub Action: a model that
loops or a prompt that balloons is capped in dollars rather than turns.

---

## MCP Server

| Command | Description |
|---------|-------------|
| `mcp-serve` | Start MCP server (stdio or HTTP) |

```
Flags:
  --transport string   Transport type: stdio or http (default "stdio")
  --addr string        HTTP listen address (default ":8080")
```

---

## Bridge Server

| Command | Description |
|---------|-------------|
| `bridge` | Start WebSocket bridge server for IDE integration |

```
Flags:
  --port int   WebSocket server port (default 19836)
```

Establishes a bidirectional WebSocket connection between gocode and IDEs (VS Code, JetBrains). Supports session management, permission forwarding, and real-time response streaming.

---

## Runtime Commands

| Command | Description |
|---------|-------------|
| `route [prompt]` | Route a prompt to matching commands/tools |
| `bootstrap [prompt]` | Bootstrap a full agent session |
| `turn-loop [prompt]` | Run a stateful multi-turn agent loop |
| `summary` | Render workspace summary |
| `manifest` | Print port manifest |
| `setup-report` | Show environment and prefetch report |

---

## Registry Commands

| Command | Description |
|---------|-------------|
| `commands` | List and search commands |
| `tools` | List, search, and filter tools |
| `subsystems` | List discovered modules |
| `tool-pool` | Show assembled tool pool |
| `command-graph` | Show command segmentation |
| `bootstrap-graph` | Show bootstrap stage graph |

---

## Session Commands

| Command | Description |
|---------|-------------|
| `flush-transcript [id]` | Flush transcript for a session |
| `load-session [id]` | Restore a saved session |

---

## Connection Commands

| Command | Description |
|---------|-------------|
| `remote-mode [target]` | Remote runtime connection |
| `ssh-mode [target]` | SSH-tunneled connection |
| `teleport-mode [target]` | Teleport-based connection |
| `direct-connect [target]` | Direct local connection |
| `deep-link [target]` | Deep link connection |

---

## Utility

| Command | Description |
|---------|-------------|
| `doctor` | Check environment and dependencies; with `--model`, probe what a model can do |
| `parity-audit` | Run parity audit |

### `gocode doctor`

```
Flags:
  --model string     Probe this model's capabilities (streaming, system prompt, tool calls, parallel tool calls)
  --api-key string   API key for the probe (overrides env vars)
  --refresh          Ignore the cached probe result and probe again
```

Without flags, `doctor` reports on git, go, tmux, ast-grep, which provider
keys are set, and whether a profile and Codex credentials exist.

With `--model`, it also makes four small requests through the same provider
layer `chat` uses and reports:

```
Model: gpt-4o-2024-08-06   probed 2026-09-10 12:41   probe cost: 812 in / 61 out tokens
  ✓ streaming            first token in 380ms
  ✓ system prompt        honoured
  ✓ tool call            arguments {"city":"Paris"}
  ✗ parallel tool calls  1 call in one turn: this model serialises tool use, expect more round trips
```

The result is cached per model in `.gocode/capabilities.json`, so the probe
spends tokens once. `chat` and `prompt` read that cache at startup and warn
when the model failed the tool-call probe or ignored the system prompt, which
turns "it didn't edit anything" into a known cause before the first request.

---

## Slash Commands (Wave 2)

| Command | Description |
|---------|-------------|
| `/ultraplan <task>` | Deep planning with strongest model (background Opus agent, 30min timeout) |
| `/vim` | Toggle vim keybindings on/off |
| `/output-style [style]` | Switch output style (concise, verbose, markdown, minimal) or show current |
| `/cron list` | List active scheduled tasks with next execution time |
| `/cron remove <id>` | Remove a scheduled task |
| `/buddy` | Display terminal companion sprite and stats |

---

[← Back to README](../README.md)
