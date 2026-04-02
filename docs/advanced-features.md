# Advanced Agent Features

[← Back to README](../README.md)

gocode v0.3.0 introduces three layers of advanced capabilities that transform it from a coding agent into a full agent operating system.

---

## Phase 1 — Core Quality

These features make every interaction more reliable. They work silently, behind the scenes, so you never have to think about them.

### Hash-Anchored File I/O

Every line gocode reads gets a content hash. Every edit is validated against those hashes before it touches your files. If the file changed since the last read, the edit is rejected. No more silent overwrites. No more stale patches.

```
42#a7| func main() {
43#3f|     fmt.Println("hello")
44#00| }
```

Enable with `--hashline`:

```bash
gocode chat --hashline
```

The hash is a 2-character CRC32 fingerprint. Deterministic, fast, collision-resistant. The agent sees annotated output; your files stay untouched.

### Model Fallback Chain

When your primary model hits a rate limit, a server error, or a context window ceiling, gocode doesn't stop. It falls through to the next model in the chain. Automatically. Silently.

- HTTP 429 (rate limit) → try next model
- HTTP 500/502/503/504 → try next model
- Context window exceeded → try next model
- Auth error → stop immediately (that's a real problem)

The `FallbackProvider` implements the same `Provider` interface. The `ConversationRuntime` doesn't know the difference. That's the point.

### Category-Based Model Routing

Not every task needs the same model. A quick single-file edit doesn't need the same horsepower as a deep architectural analysis.

| Category | Use Case | Example |
|----------|----------|---------|
| `deep` | Autonomous research, multi-file analysis | "Trace the auth flow across all packages" |
| `quick` | Single-file changes, simple questions | "Add a comment to line 42" |
| `visual-engineering` | UI and visual tasks | "Build a responsive navbar" |
| `ultrabrain` | Complex architecture decisions | "Design the migration strategy" |

The `ModelRouter` maps each category to its own `FallbackProvider`. Routing and fallback compose naturally.

### Session Recovery

Long sessions crash. Context windows fill up. APIs go down. gocode handles all of it:

1. **Context window exhaustion** — compacts the conversation (keeps system prompt + last 5 message pairs), retries
2. **Transient API failure** — exponential backoff, up to 3 attempts
3. **Corrupted session state** — reloads from the last saved checkpoint

You keep working. The agent keeps working. Nobody loses context.

---

## Phase 2 — Multi-Agent

One agent is good. A team of specialists is better.

### Orchestrator

The orchestrator maintains a registry of named sub-agents, each with its own system prompt, model preference, and tool permissions. When a task arrives, it delegates to the right specialist.

Four built-in profiles ship out of the box:

| Agent | Role | Category |
|-------|------|----------|
| `coordinator` | Main task decomposition and synthesis | quick |
| `deep-worker` | Research, multi-file analysis | deep |
| `planner` | Scope analysis and planning | quick |
| `debugger` | Error diagnosis and root cause analysis | deep |

Delegation appears as tool calls to the parent runtime. The orchestrator implements `ToolExecutor` — it's agents all the way down.

### Background Agents

Sub-agents can run in parallel. Each gets its own goroutine, its own `context.Context`, its own conversation history. Results come back through typed Go channels.

- Up to 5 concurrent background agents
- Independent context cancellation via `context.Context`
- Error isolation — one agent failing doesn't affect the others
- Buffered result channels prevent goroutine leaks

### Planning Mode

Before the agent touches any code, you can run an interview-style planning session:

```
you> /plan
```

The planner asks clarifying questions about scope, constraints, and expected outcomes. When you're satisfied, it produces a structured plan:

```json
{
  "summary": "Refactor authentication to use JWT",
  "ambiguities": ["Cookie vs localStorage for refresh tokens?"],
  "steps": [
    {"description": "Extract token validation into middleware", "rationale": "Centralizes auth checks"}
  ],
  "scope": "medium — 3-5 files affected"
}
```

Approve it, and the plan becomes the task specification for the orchestrator.

### Skills System

Skills are domain-tuned agent profiles. Each skill is a JSON file with a custom system prompt, tool permissions, and optional MCP server configurations.

Two built-in skills ship with gocode:

- **git-master** — atomic commits, interactive rebase, clean history. Tools: `bashtool`, `filereadtool`, `fileedittool`.
- **frontend-ui-ux** — design-first UI development, accessibility, responsive patterns. Tools: all file tools + glob/grep.

Create your own by dropping a JSON file in `.gocode/skills/`:

```json
{
  "name": "api-designer",
  "system_prompt": "You are an API design expert...",
  "tool_permissions": ["filereadtool", "fileedittool", "filewritetool"],
  "mcp_servers": []
}
```

Skills with MCP server configs will start those servers as child processes and register their tools automatically.

---

## Phase 3 — IDE-Level Tools

These tools give the agent capabilities that previously required a full IDE.

### LSP Integration

The agent can talk to language servers. Real renames. Real go-to-definition. Real find-references. Not regex. Not string matching. The actual language server protocol.

| Tool | What It Does |
|------|-------------|
| `lsp_rename` | Rename a symbol across the entire workspace |
| `lsp_goto_definition` | Jump to where a symbol is defined |
| `lsp_find_references` | Find every usage of a symbol |
| `lsp_diagnostics` | Get compiler errors and warnings for a file |

### AST-Grep

Pattern-aware code search and rewriting using Abstract Syntax Trees. Structural matching, not string matching.

```bash
# The agent can search for structural patterns
ast-grep run --pattern 'fmt.Println($$$)' --json .

# And rewrite them
ast-grep run --pattern 'fmt.Println($$$)' --rewrite 'log.Printf($$$)' .
```

Supports Go, JavaScript, TypeScript, and Python. Requires `ast-grep` on your PATH.

### Tmux Sessions

Agents can now run persistent terminal sessions. REPLs, debuggers, TUI applications — anything that needs an interactive terminal.

| Tool | What It Does |
|------|-------------|
| `tmux_create` | Create a named persistent session |
| `tmux_send` | Send a command and capture output |
| `tmux_read` | Read the current visible pane |
| `tmux_kill` | Terminate a session |

All sessions are cleaned up when the conversation ends. Requires `tmux` installed.

### MCP Client

gocode can now connect to external MCP servers as a client. Web search, documentation lookup, code search — any MCP-compliant server becomes a tool the agent can use.

Configure in `.gocode/mcp.json`:

```json
{
  "servers": [
    {
      "name": "web-search",
      "command": "npx",
      "args": ["-y", "@anthropic/brave-search-mcp"],
      "env": {"BRAVE_API_KEY": "${BRAVE_API_KEY}"}
    }
  ]
}
```

Tools are auto-discovered via `tools/list` and namespaced as `mcp_servername_toolname`. Protocol version `2024-11-05` is verified during the handshake.

### Context Generation (`/init-deep`)

Auto-generate hierarchical `AGENTS.md` context files throughout your project:

```
you> /init-deep
Generating AGENTS.md context files...
Created 12 AGENTS.md files, skipped 3 existing.
```

Each `AGENTS.md` summarizes its directory's purpose, source files, and subdirectories. When the agent reads any file, it automatically finds and reads the nearest `AGENTS.md` — giving it project context without you lifting a finger.

Skips `node_modules`, `vendor`, `.git`, `dist`, and `.gitignore`'d paths. Never overwrites existing `AGENTS.md` files.

---

## New CLI Flags

| Flag | Command | Default | Description |
|------|---------|---------|-------------|
| `--hashline` | `chat`, `prompt` | `false` | Enable hash-anchored file I/O |

## New Slash Commands

| Command | What It Does |
|---------|-------------|
| `/plan` | Start an interview-style planning session |
| `/init-deep` | Generate hierarchical AGENTS.md context files |

---

[← Back to README](../README.md)
