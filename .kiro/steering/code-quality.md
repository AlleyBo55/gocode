---
inclusion: auto
---

# Code Quality & Review Standards — gocode

## The Bar

Every change to this codebase should pass the "would a senior Go engineer at a top-tier company approve this in code review?" test. We optimize for:

- **Readability** over cleverness. Code is read 10x more than it's written.
- **Correctness** over speed-to-ship. Bugs in an agent that writes code are amplified.
- **Simplicity** over abstraction. Don't add a layer unless it pays for itself 3x.

## Error Handling

```go
// GOOD: wrap with context, use %w for the chain
if err != nil {
    return fmt.Errorf("connecting to provider %s: %w", name, err)
}

// BAD: naked error return
if err != nil {
    return err
}

// BAD: string formatting without %w
if err != nil {
    return fmt.Errorf("failed: %v", err)
}
```

- Every error should tell you WHERE it happened and WHY.
- Use `errors.Is` for sentinel errors, `errors.As` for typed errors.
- Never swallow errors silently. At minimum, log them.

## Concurrency

```go
// GOOD: bounded goroutines with context
ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
defer cancel()

ch := make(chan result, len(sources))
for _, src := range sources {
    go func(s source) {
        ch <- s.fetch(ctx)
    }(src)
}

// GOOD: select with context cancellation
select {
case r := <-ch:
    // handle
case <-ctx.Done():
    return ctx.Err()
}
```

- Always pass `context.Context` as the first parameter.
- Always respect context cancellation in loops and goroutines.
- Buffered channels prevent goroutine leaks. Size them to the number of producers.
- Use `sync.WaitGroup` when you need to wait for all goroutines, channels when you need results.

## Interface Design

```go
// GOOD: small, focused interface
type Provider interface {
    SendMessage(ctx context.Context, req MessageRequest) (*MessageResponse, error)
    StreamMessage(ctx context.Context, req MessageRequest) (<-chan StreamEvent, error)
}

// BAD: kitchen-sink interface
type Provider interface {
    SendMessage(...)
    StreamMessage(...)
    ListModels(...)
    GetUsage(...)
    SetConfig(...)
    Validate(...)
}
```

- Interfaces with 1-3 methods are ideal. More than 5 is a smell.
- Define interfaces where they're consumed, not where they're implemented.
- Use composition: embed small interfaces into larger ones when needed.

## Struct Design

```go
// GOOD: options pattern for complex construction
type RuntimeOptions struct {
    Provider      Provider
    Executor      ToolExecutor
    Model         string
    MaxTokens     int
    MaxIterations int
}

func NewConversationRuntime(opts RuntimeOptions) *ConversationRuntime { ... }
```

- Use options structs when constructors have more than 3 parameters.
- Zero values should be safe defaults.
- Unexported fields for invariants that must be maintained.

## Testing Philosophy

- Test behavior, not implementation. If you refactor internals, tests shouldn't break.
- Property-based tests for invariants: "for all valid inputs X, property P holds."
- Table-driven tests for enumerable cases.
- No mocks unless absolutely necessary. Prefer real implementations with test fixtures.
- Test error paths. The happy path is easy; the edge cases are where bugs live.

```go
func TestWebSearchTool_EmptyQuery(t *testing.T) {
    tool := &WebSearchTool{}
    result := tool.Execute(map[string]interface{}{"query": ""})
    if result.Success {
        t.Fatal("expected failure for empty query")
    }
}
```

## Documentation

- Every exported symbol gets a doc comment. No exceptions.
- Package-level doc comments explain the package's purpose and key types.
- Internal design decisions go in code comments, not external docs.
- User-facing behavior goes in `docs/`.
- Architecture decisions go in `docs/architecture.md`.

## Performance Awareness

- Profile before optimizing. `go test -cpuprofile` and `go tool pprof`.
- Avoid allocations in hot paths (streaming, tool dispatch).
- Pre-allocate slices when the size is known: `make([]T, 0, n)`.
- Use `strings.Builder` for string concatenation in loops.
- Buffer sizes: 32KB for HTTP response bodies, 64 for channel buffers.
