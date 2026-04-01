# CLI Reference

[← Back to README](../README.md)

All 23 gocode commands.

---

## Agent Commands

| Command | Description |
|---------|-------------|
| `chat` | Start interactive agent chat session |
| `prompt [text]` | Run a single prompt through the agent and exit |

### `gocode chat`

```
Flags:
  --model string      Model name or alias (default "sonnet")
  --max-turns int     Max agent loop iterations (default 30)
  --max-tokens int    Max output tokens per request (default 8192)
  --api-key string    API key override
  --resume string     Resume a saved session by ID
```

### `gocode prompt`

```
Flags:
  --model string      Model name or alias (default "sonnet")
  --max-turns int     Max agent loop iterations (default 30)
  --max-tokens int    Max output tokens per request (default 8192)
  --api-key string    API key override
  --no-stream         Wait for full response before printing
```

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
| `parity-audit` | Run parity audit |

---

[← Back to README](../README.md)
