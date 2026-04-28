---
inclusion: auto
---

# Development Workflow — gocode

## Build & Test

```bash
# Build
go build -o gocode ./cmd/gocode/

# Test all packages
go test ./...

# Test specific package
go test ./internal/agent/...

# Test with race detector
go test -race ./...

# Benchmark
go test -bench=. ./internal/toolimpl/

# Vet
go vet ./...
```

## Project Layout

```
cmd/gocode/main.go       — CLI entrypoint, all Cobra commands, wiring
internal/                — All business logic (26 packages)
data/                    — Embedded JSON registries (go:embed)
docs/                    — User-facing documentation
.gocode/                 — Runtime config (skills, hooks, cron, themes)
```

## Adding a New Tool

1. Create executor in `internal/toolimpl/`:
```go
type MyNewTool struct{}

func (t *MyNewTool) Execute(params map[string]interface{}) ToolResult {
    // implementation
    return ToolResult{Success: true, Output: "done"}
}
```

2. Register in `NewRegistry()`:
```go
r.executors["mynewtool"] = &MyNewTool{}
```

3. Add tool definition to `data/tools.json` with name, description, inputSchema.

4. Write tests in `internal/toolimpl/toolimpl_test.go`.

## Adding a New Provider

1. Implement `Provider` interface in `internal/apiclient/`.
2. Add resolution logic in `ResolveProvider()`.
3. Add model aliases in `internal/apiclient/aliases.go`.
4. Document in `docs/supported-models.md`.

## Adding a New Slash Command

1. Add handler in `internal/repl/repl.go` in the slash command switch.
2. Add to help text.
3. Document in `docs/ux-features.md`.

## Git Conventions

- Atomic commits. One logical change per commit.
- Commit messages: imperative mood, 50 char subject, blank line, body if needed.
- Branch naming: `feat/short-description`, `fix/short-description`, `refactor/short-description`.
- Always run `go test ./...` before committing.
- Use `/review` to self-review before pushing.

## Configuration Files

| File | Purpose |
|------|---------|
| `GOCODE.md` / `CLAUDE.md` | Project-specific agent instructions |
| `.gocode/skills/*.json` | Custom skill definitions |
| `.gocode/hooks.json` | Lifecycle hooks |
| `.gocode/cron.json` | Scheduled tasks |
| `.gocode/mcp.json` | MCP client server configs |
| `.gocode/theme.json` | Custom TUI theme |
| `.gocode/keybinds.json` | Custom keybindings |
| `.gocode/output-styles/` | Custom output styles |

## Debugging

- Use `--verbose` flag for request/response logging.
- Use `--print` to inspect the system prompt.
- Use `/doctor` to check environment health.
- Use `/status` for session diagnostics.
- Set `GOCODE_DEBUG=1` for internal debug logging.
