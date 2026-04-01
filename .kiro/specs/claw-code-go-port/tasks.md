# Implementation Plan: claw-code Go Port

## Overview

Port the Python "claw-code" agent harness runtime to idiomatic Go, organized by dependency order: foundational packages first, then packages that depend on them, then CLI/integration layers last. All code uses Go conventions (interfaces, error values, goroutines, `encoding/json`, `go:embed`). Property-based tests use `pgregory.net/rapid`.

## Tasks

- [x] 1. Initialize Go module, project structure, and embedded data
  - [x] 1.1 Create `go.mod` with module path, `go.sum`, and add dependencies (`cobra`, `pgregory.net/rapid`)
    - Run `go mod init` and `go get` for cobra and rapid
    - Create the `cmd/claw-code/`, `internal/` package directories, and `data/` directory
    - _Requirements: 20.1, 20.2_

  - [x] 1.2 Create `data/commands.json`, `data/tools.json` placeholder snapshots and `data.go` with `go:embed` directives
    - Populate placeholder JSON arrays with sample command/tool entries for testing
    - Define `var CommandsJSON []byte` and `var ToolsJSON []byte` with embed directives
    - _Requirements: 21.1_

  - [x] 1.3 Create `Makefile` with build, test, and clean targets
    - `build` target: `go build -o bin/claw-code ./cmd/claw-code`
    - `test` target: `go test ./...`
    - `clean` target: remove `bin/`
    - _Requirements: 20.6_

- [x] 2. Implement `internal/models` — core data structs
  - [x] 2.1 Create `internal/models/models.go` with Subsystem, PortingModule, PermissionDenial, UsageSummary, PortingBacklog structs
    - All fields exported with JSON struct tags
    - Implement `UsageSummary.AddTurn(inputTokens, outputTokens int)`
    - Implement `PortingBacklog.SummaryLines() []string`
    - _Requirements: 1.1, 1.2, 1.3, 1.5_

  - [ ]* 2.2 Write property test: JSON Serialization Round-Trip for model structs
    - **Property 1: JSON Serialization Round-Trip**
    - **Validates: Requirements 1.4, 22.1**

  - [ ]* 2.3 Write property test: UsageSummary AddTurn Accumulation
    - **Property 2: UsageSummary AddTurn Accumulation**
    - **Validates: Requirements 1.2**

  - [ ]* 2.4 Write property test: PortingBacklog SummaryLines Length and Content
    - **Property 3: PortingBacklog SummaryLines Length and Content**
    - **Validates: Requirements 1.3**

  - [ ]* 2.5 Write unit tests for models edge cases
    - Test zero-value struct initialization
    - Test AddTurn with zero tokens
    - Test SummaryLines with empty backlog
    - _Requirements: 1.5_

- [x] 3. Implement `internal/permissions` — tool permission context
  - [x] 3.1 Create `internal/permissions/permissions.go` with ToolPermissionContext struct and PermissionChecker interface
    - Constructor: `NewToolPermissionContext(denyNames []string, denyPrefixes []string)`
    - `IsBlocked(toolName string) bool` — exact match on deny-names, then prefix scan on deny-prefixes
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5_

  - [ ]* 3.2 Write property test: Permission Context Correctness
    - **Property 4: Permission Context Correctness**
    - **Validates: Requirements 2.3, 2.4, 2.5**

  - [ ]* 3.3 Write unit tests for permissions edge cases
    - Test empty deny lists, overlapping prefixes, exact-match vs prefix priority
    - _Requirements: 2.3, 2.4, 2.5_

- [x] 4. Implement `internal/context` — workspace scanning
  - [x] 4.1 Create `internal/context/context.go` with PortContext struct
    - `BuildPortContext(root string) (*PortContext, error)` — uses `filepath.Walk` to count files by extension
    - `Render() string` — outputs Markdown
    - Return zero counts for missing directories
    - _Requirements: 3.1, 3.2, 3.3, 3.4_

  - [ ]* 4.2 Write property test: Workspace File Count Accuracy
    - **Property 21: Workspace File Count Accuracy**
    - **Validates: Requirements 3.2**

  - [ ]* 4.3 Write unit tests for context edge cases
    - Test missing directory returns zero counts
    - Test Render output contains expected sections
    - _Requirements: 3.3, 3.4_

- [x] 5. Checkpoint — Ensure foundational packages compile and tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 6. Implement `internal/commands` — command registry
  - [x] 6.1 Create `internal/commands/commands.go` with Command, ArgDef, CommandExecution structs and CommandRegistry
    - Implement CommandLookup interface: `GetCommand`, `FindCommands`, `FilterPlugins`, `FilterSkills`, `RenderIndex`
    - Load from embedded JSON via `json.Unmarshal`
    - Case-insensitive substring search in `FindCommands`
    - Define `ErrCommandNotFound` sentinel error
    - _Requirements: 4.1, 4.2, 4.3, 4.4, 4.5, 4.6_

  - [ ]* 6.2 Write property test: Registry Lookup Correctness (commands)
    - **Property 5: Registry Lookup Correctness**
    - **Validates: Requirements 4.2**

  - [ ]* 6.3 Write property test: Registry Search Completeness (commands)
    - **Property 6: Registry Search Completeness**
    - **Validates: Requirements 4.3**

  - [ ]* 6.4 Write property test: Command Category Partitioning
    - **Property 7: Command Category Partitioning**
    - **Validates: Requirements 4.4, 15.4**

  - [ ]* 6.5 Write property test: Render Completeness (commands)
    - **Property 11: Render Completeness**
    - **Validates: Requirements 4.5**

  - [ ]* 6.6 Write property test: Execution Result Name Consistency (commands)
    - **Property 27: Execution Result Name Consistency**
    - **Validates: Requirements 4.6**

  - [ ]* 6.7 Write property test: Malformed JSON Parse Error (commands)
    - **Property 25: Malformed JSON Parse Error**
    - **Validates: Requirements 21.3**

  - [ ]* 6.8 Write unit tests for commands edge cases
    - Test empty registry, lookup miss returns error, filter on empty categories
    - _Requirements: 4.2, 4.3, 4.4_

- [x] 7. Implement `internal/tools` — tool registry
  - [x] 7.1 Create `internal/tools/tools.go` with Tool, ArgDef, ToolExecution structs and ToolRegistry
    - Implement ToolLookup interface: `GetTool`, `FindTools`, `FilterByPermissions`, `FilterByMode`, `RenderIndex`
    - Load from embedded JSON via `json.Unmarshal`
    - Define `ErrToolNotFound` sentinel error
    - _Requirements: 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 5.7_

  - [ ]* 7.2 Write property test: Registry Lookup Correctness (tools)
    - **Property 5: Registry Lookup Correctness**
    - **Validates: Requirements 5.2**

  - [ ]* 7.3 Write property test: Registry Search Completeness (tools)
    - **Property 6: Registry Search Completeness**
    - **Validates: Requirements 5.3**

  - [ ]* 7.4 Write property test: Render Completeness (tools)
    - **Property 11: Render Completeness**
    - **Validates: Requirements 5.6**

  - [ ]* 7.5 Write property test: Execution Result Name Consistency (tools)
    - **Property 27: Execution Result Name Consistency**
    - **Validates: Requirements 5.7**

  - [ ]* 7.6 Write property test: Malformed JSON Parse Error (tools)
    - **Property 25: Malformed JSON Parse Error**
    - **Validates: Requirements 21.3**

  - [ ]* 7.7 Write unit tests for tools edge cases
    - Test FilterByPermissions with all-blocked, none-blocked
    - Test FilterByMode combinations
    - _Requirements: 5.4, 5.5_

- [x] 8. Implement `internal/toolpool` — tool pool assembly
  - [x] 8.1 Create `internal/toolpool/toolpool.go` with ToolPool struct and AssembleToolPool function
    - Chain permission filter, mode filter, MCP filter
    - `Render() string` for Markdown output
    - _Requirements: 6.1, 6.2, 6.3_

  - [ ]* 8.2 Write property test: Tool Pool Filter Composition
    - **Property 8: Tool Pool Filter Composition**
    - **Validates: Requirements 5.4, 5.5, 6.2**

  - [ ]* 8.3 Write property test: Tool Pool Assembly Idempotence
    - **Property 9: Tool Pool Assembly Idempotence**
    - **Validates: Requirements 6.3**

  - [ ]* 8.4 Write unit tests for toolpool edge cases
    - Test empty tool set, all tools filtered out
    - _Requirements: 6.2_

- [x] 9. Implement `internal/execution` — execution registry
  - [x] 9.1 Create `internal/execution/execution.go` with Executable interface, MirroredCommand, MirroredTool, ExecutionRegistry
    - `Build(commands CommandLookup, tools ToolLookup) error`
    - `Lookup(name string) (Executable, error)`
    - _Requirements: 7.1, 7.2, 7.3_

  - [ ]* 9.2 Write property test: Execution Registry Completeness
    - **Property 10: Execution Registry Completeness**
    - **Validates: Requirements 7.2, 7.3**

  - [ ]* 9.3 Write unit tests for execution edge cases
    - Test lookup miss, duplicate name handling
    - _Requirements: 7.3_

- [x] 10. Checkpoint — Ensure registry and pool packages compile and tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 11. Implement `internal/history` — history log
  - [x] 11.1 Create `internal/history/history.go` with HistoryEvent, HistoryLog structs
    - `Append(event HistoryEvent)`
    - `Render() string` — Markdown timeline
    - _Requirements: 10.1, 10.2, 10.3_

  - [ ]* 11.2 Write property test: History Log Ordering
    - **Property 15: History Log Ordering**
    - **Validates: Requirements 10.2**

  - [ ]* 11.3 Write property test: Render Completeness (history)
    - **Property 11: Render Completeness**
    - **Validates: Requirements 10.3**

  - [ ]* 11.4 Write unit tests for history edge cases
    - Test empty log render, single event
    - _Requirements: 10.2, 10.3_

- [x] 12. Implement `internal/transcript` — transcript store
  - [x] 12.1 Create `internal/transcript/transcript.go` with TranscriptEntry, TranscriptStore, TranscriptManager interface
    - `Append(entry TranscriptEntry) error`
    - `Compact() error` — keep last N entries, summarize rest
    - `Replay() ([]TranscriptEntry, error)`
    - `Flush() error` — write to disk, clear buffer
    - _Requirements: 11.1, 11.2, 11.3, 11.4, 11.5_

  - [ ]* 12.2 Write property test: Transcript Append-Replay Round-Trip
    - **Property 12: Transcript Append-Replay Round-Trip**
    - **Validates: Requirements 11.2, 11.4**

  - [ ]* 12.3 Write property test: Transcript Flush Clears Buffer
    - **Property 13: Transcript Flush Clears Buffer**
    - **Validates: Requirements 11.5**

  - [ ]* 12.4 Write property test: Transcript Compaction Reduces Size
    - **Property 14: Transcript Compaction Reduces Size**
    - **Validates: Requirements 11.3**

  - [ ]* 12.5 Write unit tests for transcript edge cases
    - Test flush on empty store, compact below threshold, replay ordering
    - _Requirements: 11.2, 11.3, 11.5_

- [x] 13. Implement `internal/session` — session persistence
  - [x] 13.1 Create `internal/session/session.go` with StoredSession, Message structs and SessionStore
    - `Save(session StoredSession) error` — atomic write (temp + rename), `os.MkdirAll`
    - `Load(sessionID string) (StoredSession, error)`
    - Define `ErrSessionNotFound` sentinel error
    - _Requirements: 9.1, 9.2, 9.3, 9.4, 9.5_

  - [ ]* 13.2 Write property test: Session Store File Round-Trip
    - **Property 17: Session Store File Round-Trip**
    - **Validates: Requirements 9.4**

  - [ ]* 13.3 Write property test: JSON Serialization Round-Trip (session)
    - **Property 1: JSON Serialization Round-Trip**
    - **Validates: Requirements 1.4, 22.1**

  - [ ]* 13.4 Write unit tests for session edge cases
    - Test load missing session, save creates directory, invalid session ID
    - _Requirements: 9.3, 9.5_

- [x] 14. Checkpoint — Ensure storage packages compile and tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 15. Implement `internal/setup` — workspace setup and prefetch
  - [x] 15.1 Create `internal/setup/setup.go` with SetupReport, PrefetchResult, PrefetchEngine, WorkspaceSetup
    - `WorkspaceSetup` detects Go version (`exec.Command`), platform (`runtime.GOOS`), test command
    - `PrefetchEngine` runs prefetches concurrently via `sync.WaitGroup` + goroutines
    - `RunSetup() (SetupReport, error)` combines both
    - _Requirements: 13.1, 13.2, 13.3, 13.4_

  - [ ]* 15.2 Write property test: Concurrent Error Isolation
    - **Property 20: Concurrent Error Isolation**
    - **Validates: Requirements 13.4, 14.3**

  - [ ]* 15.3 Write unit tests for setup edge cases
    - Test missing Go binary, failed prefetch captured in result
    - _Requirements: 13.1, 13.4_

- [x] 16. Implement `internal/deferred` — deferred initialization
  - [x] 16.1 Create `internal/deferred/deferred.go` with DeferredInitResult struct and RunDeferredInit function
    - Fields: Trusted, PluginInit, SkillInit, MCPPrefetch, SessionHooks (each with optional error)
    - Each step runs independently; errors captured per-step
    - _Requirements: 14.1, 14.2, 14.3_

  - [ ]* 16.2 Write unit tests for deferred init
    - Test partial failure scenario, all-success scenario
    - _Requirements: 14.2, 14.3_

- [x] 17. Implement `internal/systeminit` — system init message
  - [x] 17.1 Create `internal/systeminit/systeminit.go` with BuildSystemInitMessage function
    - Assembles string from workspace context, manifest, tool pool, and bootstrap graph renders
    - _Requirements: 15.1_

  - [ ]* 17.2 Write property test: System Init Message Non-Empty
    - **Property 28: System Init Message Non-Empty**
    - **Validates: Requirements 15.1**

- [x] 18. Implement `internal/bootstrap` — bootstrap graph
  - [x] 18.1 Create `internal/bootstrap/bootstrap.go` with BootstrapGraph struct
    - Define stages as DAG with name, dependencies, status
    - `Render() string` — Markdown output
    - _Requirements: 15.2, 15.3_

  - [ ]* 18.2 Write property test: Render Completeness (bootstrap)
    - **Property 11: Render Completeness**
    - **Validates: Requirements 15.3**

  - [ ]* 18.3 Write unit tests for bootstrap edge cases
    - Test empty graph, single stage, cyclic dependency handling
    - _Requirements: 15.2, 15.3_

- [x] 19. Implement `internal/commandgraph` — command graph segmentation
  - [x] 19.1 Create `internal/commandgraph/commandgraph.go` with CommandGraph struct
    - Segment commands into builtin, plugin-like, skill-like categories
    - _Requirements: 15.4_

  - [ ]* 19.2 Write unit tests for commandgraph
    - Test partitioning with mixed categories, empty input
    - _Requirements: 15.4_

- [x] 20. Implement `internal/manifest` — port manifest
  - [x] 20.1 Create `internal/manifest/manifest.go` with PortManifest struct
    - `BuildPortManifest(srcDir string) (*PortManifest, error)` — scan directory, discover Go files
    - `Render() string` — Markdown output
    - Return error if directory doesn't exist
    - _Requirements: 16.1, 16.2, 16.3_

  - [ ]* 20.2 Write unit tests for manifest
    - Test missing directory returns error, empty directory
    - _Requirements: 16.2, 16.3_

- [x] 21. Implement `internal/modes` — direct and remote modes
  - [x] 21.1 Create `internal/modes/direct.go` with DirectModeReport and mode functions
    - `DirectConnect() (DirectModeReport, error)`
    - `DeepLink() (DirectModeReport, error)`
    - _Requirements: 17.1, 17.2, 17.6_

  - [x] 21.2 Create `internal/modes/remote.go` with RuntimeModeReport and mode functions
    - `RemoteMode() (RuntimeModeReport, error)`
    - `SSHMode() (RuntimeModeReport, error)`
    - `TeleportMode() (RuntimeModeReport, error)`
    - _Requirements: 17.3, 17.4, 17.5, 17.6_

  - [ ]* 21.3 Write unit tests for modes
    - Test error returns for each mode
    - _Requirements: 17.6_

- [x] 22. Checkpoint — Ensure all internal packages compile and tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 23. Implement `internal/queryengine` — query engine
  - [x] 23.1 Create `internal/queryengine/queryengine.go` with QueryEngineConfig, TurnResult, StreamEvent, QueryEnginePort
    - Constructors: `FromWorkspace(config QueryEngineConfig, ...) *QueryEnginePort` and `FromSavedSession(session StoredSession, ...) *QueryEnginePort`
    - `SubmitMessage(prompt string) (TurnResult, error)`
    - `StreamSubmitMessage(prompt string) (<-chan StreamEvent, error)` — goroutine + channel
    - `PersistSession() error`, `FlushTranscript() error`, `RenderSummary() string`
    - Budget enforcement: stop when cumulative tokens exceed max_budget_tokens
    - Compaction trigger: compact when turn count exceeds compact_after_turns
    - Define `ErrBudgetExceeded` sentinel error
    - _Requirements: 8.1, 8.2, 8.3, 8.4, 8.5, 8.6, 8.7, 8.8_

  - [ ]* 23.2 Write property test: Query Engine Budget Enforcement
    - **Property 18: Query Engine Budget Enforcement**
    - **Validates: Requirements 8.6**

  - [ ]* 23.3 Write property test: Query Engine Compaction Trigger
    - **Property 19: Query Engine Compaction Trigger**
    - **Validates: Requirements 8.5**

  - [ ]* 23.4 Write unit tests for queryengine edge cases
    - Test zero budget, zero max turns, streaming channel closure
    - _Requirements: 8.5, 8.6_

- [x] 24. Implement `internal/runtime` — runtime orchestrator
  - [x] 24.1 Create `internal/runtime/runtime.go` with RoutedMatch, RuntimeSession, PortRuntime
    - Implement RuntimeOrchestrator interface
    - `RoutePrompt(prompt string) ([]RoutedMatch, error)` — substring scoring, sort descending
    - `BootstrapSession() (RuntimeSession, error)` — initialize all subsystems
    - _Requirements: 12.1, 12.2, 12.3, 12.4_

  - [ ]* 24.2 Write property test: Route Prompt Score Ordering
    - **Property 16: Route Prompt Score Ordering**
    - **Validates: Requirements 12.3**

  - [ ]* 24.3 Write unit tests for runtime edge cases
    - Test route with no matches, bootstrap with missing dependencies
    - _Requirements: 12.3, 12.4_

- [x] 25. Checkpoint — Ensure engine and runtime compile and tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 26. Implement `internal/mcp` — MCP server
  - [x] 26.1 Create `internal/mcp/server.go` with MCP server supporting stdio and HTTP transports
    - Wrap ExecutionRegistry for tool/command dispatch
    - Handle request validation, execution, MCP-compliant responses and errors
    - _Requirements: 19.4, 19.5, 19.6_

  - [ ]* 26.2 Write property test: MCP Valid Request Execution
    - **Property 23: MCP Valid Request Execution**
    - **Validates: Requirements 19.5**

  - [ ]* 26.3 Write property test: MCP Invalid Request Error Response
    - **Property 24: MCP Invalid Request Error Response**
    - **Validates: Requirements 19.6**

  - [ ]* 26.4 Write unit tests for MCP edge cases
    - Test malformed request, unknown tool, valid tool dispatch
    - _Requirements: 19.5, 19.6_

- [x] 27. Implement `internal/kiro` — Kiro integration
  - [x] 27.1 Create `internal/kiro/hooks.go` — read and execute hook definitions from `.kiro/hooks/`
    - _Requirements: 19.2_

  - [x] 27.2 Create `internal/kiro/steering.go` — generate steering files from runtime config and session state
    - _Requirements: 19.3_

  - [x] 27.3 Create `internal/kiro/specs.go` — read spec files from `.kiro/specs/`
    - _Requirements: 19.7_

  - [ ]* 27.4 Write property test: Steering File Generation Completeness
    - **Property 26: Steering File Generation Completeness**
    - **Validates: Requirements 19.3**

  - [ ]* 27.5 Write unit tests for kiro integration
    - Test missing hooks directory, empty specs directory, steering output format
    - _Requirements: 19.2, 19.3, 19.7_

- [x] 28. Implement CLI entrypoint with all subcommands
  - [x] 28.1 Create `cmd/claw-code/main.go` with cobra root command and all subcommands
    - Subcommands: summary, manifest, parity-audit, setup-report, command-graph, tool-pool, bootstrap-graph, subsystems, commands, tools, route, bootstrap, turn-loop, flush-transcript, load-session, remote-mode, ssh-mode, teleport-mode, direct-connect, deep-link
    - Each subcommand wires to the corresponding internal package function
    - Invalid arguments print usage to stderr and exit non-zero
    - _Requirements: 18.1, 18.2, 18.3, 18.4, 19.1_

  - [ ]* 28.2 Write property test: CLI Invalid Argument Rejection
    - **Property 22: CLI Invalid Argument Rejection**
    - **Validates: Requirements 18.3**

  - [ ]* 28.3 Write unit tests for CLI
    - Test help output, unknown subcommand, missing required flags
    - _Requirements: 18.2, 18.3_

- [x] 29. Final checkpoint — Ensure full build and all tests pass
  - Run `go build ./...` and `go test ./...`
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation
- Property tests validate universal correctness properties from the design document (28 properties total)
- Unit tests validate specific examples and edge cases
- Dependency order: models → permissions → context → commands/tools → toolpool → execution → history/transcript/session → setup/deferred/systeminit/bootstrap/commandgraph/manifest/modes → queryengine → runtime → mcp/kiro → CLI
