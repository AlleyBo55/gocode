---
inclusion: auto
---

# Go Engineering Standards — gocode

You are working on **gocode**, a high-performance AI coding agent written in Go. Single binary, zero runtime dependencies, 26 internal packages, 200+ model support across 11 providers.

## Architecture Principles

- Every package owns its interface. No god objects. No circular imports.
- Concurrency via goroutines + channels. Never share memory without a channel or sync primitive.
- `(T, error)` everywhere. No panics in library code. Panics are bugs.
- Interfaces are small (1-3 methods). Accept interfaces, return structs.
- `go:embed` for compiled-in data. No runtime file reads for registries.
- Atomic file writes (temp + rename) for all persistence. Zero corruption guarantee.

## Code Style

- Follow `gofmt` and `go vet` unconditionally. No exceptions.
- Naming: `camelCase` for unexported, `PascalCase` for exported. No underscores in Go identifiers.
- Package names are short, lowercase, singular nouns: `agent`, `session`, `repl`, not `agents`, `sessions`.
- Error messages start lowercase, no trailing punctuation: `return fmt.Errorf("loading config: %w", err)`
- Wrap errors with `%w` for the chain. Use `errors.Is` / `errors.As` for checking.
- Comments on exported symbols are mandatory. Start with the symbol name: `// NewRegistry creates...`
- Table-driven tests. Subtests with `t.Run`. No test helpers that hide assertions.
- Keep functions under 50 lines. If it's longer, extract a helper.

## Project-Specific Patterns

- Tool implementations go in `internal/toolimpl/` and implement the `ToolExecutor` interface.
- Provider implementations go in `internal/apiclient/` and implement the `Provider` interface.
- New CLI subcommands are added in `cmd/gocode/main.go` using Cobra.
- Skills are JSON files in `.gocode/skills/` with `name`, `system_prompt`, `tool_permissions`.
- The `ConversationRuntime` in `internal/agent/runtime.go` is the core agent loop — understand it before touching agent behavior.
- The `FallbackProvider` and `ModelRouter` in `internal/apiclient/` handle model routing and failover.
- Session persistence uses `internal/session/` with atomic writes.
- The orchestrator in `internal/orchestrator/` manages sub-agents with independent goroutines.

## Testing

- Use `pgregory.net/rapid` for property-based testing where invariants exist.
- Test files live next to their source: `foo.go` → `foo_test.go`.
- Integration tests that need external services are gated with build tags: `//go:build integration`.
- Benchmark critical paths: streaming, tool dispatch, session serialization.
- Use `testdata/` directories for fixture files.

## Performance Expectations

- Startup must remain under 10ms. Profile before adding init-time work.
- Binary size target: ~12MB. Avoid heavy dependencies.
- Streaming responses must forward tokens with sub-millisecond overhead.
- Tool execution timeout: 15s default for network, 30s for shell commands.
- Session serialization must be non-blocking (goroutine + channel).

## Dependencies

- Minimize external deps. The go.mod should stay lean.
- Charmbracelet stack for TUI (bubbletea, lipgloss, bubbles).
- Cobra for CLI. No alternatives.
- `encoding/json` from stdlib. No third-party JSON libraries.
- `net/http` from stdlib for all HTTP. No third-party HTTP clients.
- `pgregory.net/rapid` for property-based testing only.

## Security

- Never log API keys. Mask them in error messages.
- Validate all file paths against directory traversal before tool execution.
- Permission system gates dangerous tools (BashTool, FileWriteTool) behind user confirmation.
- Sanitize shell command inputs. No raw string interpolation into commands.
- MCP server validates JSON-RPC requests before dispatch.
