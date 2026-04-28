---
inclusion: auto
---

# Agent Architecture Guide — gocode

## System Design Philosophy

This is not a chatbot with file access. This is an agent operating system. Every design decision optimizes for:

1. **Composability** — Components compose through interfaces, not inheritance.
2. **Isolation** — Sub-agents run in independent goroutines with their own contexts.
3. **Resilience** — Failures are contained. One agent crashing doesn't take down the system.
4. **Observability** — Every tool call, every permission check, every fallback is traceable.

## Core Runtime Loop

```
User Message → ConversationRuntime.SendUserMessage()
  → buildRequest() (system prompt + session + tools)
  → provider.SendMessage() or provider.StreamMessage()
  → parse response content blocks
  → if tool_use blocks exist:
      → for each tool: authorize → pre-hook → execute → post-hook
      → append tool results to session
      → loop back to provider call
  → if no tool_use blocks: return final response
```

The loop is bounded by `maxIter` (default 30). Context cancellation propagates cleanly.

## Provider Abstraction

```go
type Provider interface {
    SendMessage(ctx context.Context, req MessageRequest) (*MessageResponse, error)
    StreamMessage(ctx context.Context, req MessageRequest) (<-chan StreamEvent, error)
}
```

Every LLM backend implements this. The `FallbackProvider` wraps multiple providers and fails over on 429/5xx. The `ModelRouter` maps task categories to different `FallbackProvider` chains.

## Tool Execution Pipeline

```
Tool Call → PermissionPolicy.Authorize()
         → HookRunner.PreToolUse()
         → ToolExecutor.Execute()
         → HookRunner.PostToolUse()
         → Result (with merged hook feedback)
```

Tools are registered in `internal/toolimpl/Registry`. Each tool implements:

```go
type ToolExecutor interface {
    Execute(params map[string]interface{}) ToolResult
}
```

## Multi-Agent Orchestration

The orchestrator maintains named sub-agent profiles. Each profile specifies:
- System prompt (role-specific)
- Model category (deep/quick/visual/ultrabrain)
- Tool permissions (subset of available tools)

Background agents run in goroutines with buffered result channels. The orchestrator implements `ToolExecutor` itself — delegation appears as tool calls to the parent runtime.

## Session Management

- Sessions are arrays of `InputMessage` (role + content blocks).
- Persistence uses atomic writes: write to temp file, then rename.
- Compaction keeps the last N message pairs when context fills up.
- Recovery: on context exhaustion, compact and retry. On API failure, exponential backoff.

## Memory System

Three scopes: session, project, global. The dream system runs a 4-phase consolidation cycle during idle:
1. Orient — read current memory state
2. Gather — scan conversation for signals worth remembering
3. Consolidate — merge into durable memory files
4. Prune — remove entries below relevance threshold

## When Adding New Features

1. Define the interface first. What does the component need to do?
2. Implement behind the interface. Keep the implementation private.
3. Register in the appropriate place (tool registry, command registry, etc.).
4. Wire in `cmd/gocode/main.go` if it needs CLI exposure.
5. Add tests — at minimum, unit tests for the happy path and error cases.
6. Document in the appropriate `docs/` file.

## Anti-Patterns to Avoid

- Don't reach across package boundaries for internal state.
- Don't add global variables. Pass dependencies explicitly.
- Don't use `init()` functions for anything non-trivial.
- Don't block the main goroutine with synchronous network calls.
- Don't add external dependencies without justification.
- Don't put business logic in `cmd/`. That's wiring only.
