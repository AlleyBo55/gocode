# Requirements Document

## Introduction

This document specifies the requirements for porting the Python "claw-code" project (an agent harness runtime) to idiomatic Go. The Go port must replicate all functionality of the original Python codebase, follow Go conventions (modules, packages, interfaces, error handling), and add integration capabilities for the Kiro CLI ecosystem. The resulting binary should be a standalone CLI tool that can also serve as an MCP server for use within Kiro.

## Glossary

- **Runtime**: The top-level Go application that orchestrates session bootstrap, prompt routing, and turn execution
- **Query_Engine**: The core component that processes prompts, tracks token budgets, manages turn limits, and produces turn results
- **Tool_Registry**: The subsystem that loads, indexes, filters, and executes tool definitions from snapshot data
- **Command_Registry**: The subsystem that loads, indexes, filters, and executes command definitions from snapshot data
- **Tool_Pool**: The assembled collection of tools available for a session, filtered by mode and permissions
- **Permission_Context**: The subsystem that determines which tools are blocked based on deny-lists and deny-prefixes
- **Session_Store**: The subsystem responsible for persisting and loading session state as JSON files
- **Transcript_Store**: The subsystem that appends, compacts, replays, and flushes conversation transcripts
- **Execution_Registry**: The unified lookup registry that wraps commands and tools for dispatch
- **Port_Context**: The workspace scanning subsystem that discovers source files, tests, assets, and archives
- **Port_Manifest**: The subsystem that scans the source directory and generates a workspace manifest
- **Bootstrap_Graph**: The subsystem that defines and renders the bootstrap stage dependency graph
- **Command_Graph**: The subsystem that segments commands into builtins, plugin-like, and skill-like categories
- **Setup_Report**: The subsystem that gathers workspace environment info (Go version, platform, test command) and runs prefetches
- **Prefetch_Engine**: The subsystem that runs side-effect prefetches (MDM reads, keychain, project scans) concurrently
- **Deferred_Init**: The subsystem that performs deferred initialization of plugins, skills, MCP prefetch, and session hooks
- **System_Init**: The subsystem that builds the system initialization message for a session
- **History_Log**: The subsystem that records and renders session history events
- **CLI_Entrypoint**: The main CLI binary providing subcommands for all runtime operations
- **Kiro_Integration**: The set of capabilities enabling the Go port to work within the Kiro CLI ecosystem
- **MCP_Server**: A Model Context Protocol server endpoint exposed by the Go port for tool/command access
- **Steering_File_Generator**: The component that produces Kiro-compatible steering files from runtime state
- **Data_Model**: The set of Go structs corresponding to the Python dataclasses (Subsystem, PortingModule, PermissionDenial, UsageSummary, PortingBacklog)

## Requirements

### Requirement 1: Data Model Port

**User Story:** As a developer, I want all Python data models ported to Go structs with proper JSON serialization, so that the Go port maintains data compatibility with the original project.

#### Acceptance Criteria

1. THE Data_Model SHALL define Go structs for Subsystem, PortingModule, PermissionDenial, UsageSummary, and PortingBacklog with exported fields and JSON struct tags
2. THE Data_Model SHALL implement a method on UsageSummary that adds token counts from a turn, equivalent to the Python add_turn method
3. THE Data_Model SHALL implement a method on PortingBacklog that returns summary lines as a string slice
4. WHEN a Data_Model struct is serialized to JSON and deserialized back, THE Data_Model SHALL produce an equivalent struct (round-trip property)
5. IF a Data_Model struct receives zero-value fields, THEN THE Data_Model SHALL initialize those fields to their Go zero values without error

### Requirement 2: Permission Context Port

**User Story:** As a developer, I want the tool permission system ported to Go, so that tool access control works identically to the Python version.

#### Acceptance Criteria

1. THE Permission_Context SHALL define a struct with immutable deny-name and deny-prefix collections
2. THE Permission_Context SHALL provide a constructor that builds the context from string slices of deny names and deny prefixes
3. WHEN a tool name matches an entry in the deny-names set, THE Permission_Context SHALL report the tool as blocked
4. WHEN a tool name starts with any entry in the deny-prefixes set, THE Permission_Context SHALL report the tool as blocked
5. WHEN a tool name does not match any deny-name or deny-prefix, THE Permission_Context SHALL report the tool as allowed

### Requirement 3: Workspace Context Port

**User Story:** As a developer, I want workspace scanning ported to Go, so that the runtime can discover project files and render context.

#### Acceptance Criteria

1. THE Port_Context SHALL define a struct containing source root, tests root, assets root, archive root, file counts, and archive availability
2. WHEN build_port_context is called, THE Port_Context SHALL scan the filesystem and count source files with the appropriate extensions
3. THE Port_Context SHALL provide a render function that outputs the context as a Markdown-formatted string
4. IF a scanned directory does not exist, THEN THE Port_Context SHALL set the file count to zero and continue without error

### Requirement 4: Command Registry Port

**User Story:** As a developer, I want the command registry ported to Go, so that commands can be loaded, searched, filtered, and executed.

#### Acceptance Criteria

1. THE Command_Registry SHALL load command definitions from a JSON snapshot file at initialization
2. WHEN get_command is called with a command name, THE Command_Registry SHALL return the matching command or an error if not found
3. WHEN find_commands is called with a search query, THE Command_Registry SHALL return all commands whose names contain the query (case-insensitive)
4. THE Command_Registry SHALL provide filtering methods that separate commands into plugin and skill categories
5. THE Command_Registry SHALL provide a render function that outputs a Markdown-formatted command index
6. WHEN execute_command is called, THE Command_Registry SHALL return a CommandExecution result struct

### Requirement 5: Tool Registry Port

**User Story:** As a developer, I want the tool registry ported to Go, so that tools can be loaded, searched, filtered by permissions, and executed.

#### Acceptance Criteria

1. THE Tool_Registry SHALL load tool definitions from a JSON snapshot file at initialization
2. WHEN get_tool is called with a tool name, THE Tool_Registry SHALL return the matching tool or an error if not found
3. WHEN find_tools is called with a search query, THE Tool_Registry SHALL return all tools whose names contain the query (case-insensitive)
4. WHEN filtering tools, THE Tool_Registry SHALL exclude tools blocked by the provided Permission_Context
5. THE Tool_Registry SHALL support filtering by simple_mode and MCP inclusion flags
6. THE Tool_Registry SHALL provide a render function that outputs a Markdown-formatted tool index
7. WHEN execute_tool is called, THE Tool_Registry SHALL return a ToolExecution result struct

### Requirement 6: Tool Pool Assembly Port

**User Story:** As a developer, I want the tool pool assembly ported to Go, so that sessions get the correct set of available tools.

#### Acceptance Criteria

1. THE Tool_Pool SHALL define a struct containing the assembled tools and a Markdown render method
2. WHEN assemble_tool_pool is called with simple_mode, include_mcp, and a Permission_Context, THE Tool_Pool SHALL return a pool containing only the tools that pass all filters
3. WHEN the same filters are applied twice, THE Tool_Pool SHALL produce identical results (idempotence)

### Requirement 7: Execution Registry Port

**User Story:** As a developer, I want the execution registry ported to Go, so that commands and tools can be looked up through a unified interface.

#### Acceptance Criteria

1. THE Execution_Registry SHALL wrap commands as MirroredCommand and tools as MirroredTool with a common interface
2. WHEN build_execution_registry is called, THE Execution_Registry SHALL populate itself from the Command_Registry and Tool_Registry
3. WHEN a lookup is performed, THE Execution_Registry SHALL return the matching command or tool wrapper, or an error if not found

### Requirement 8: Query Engine Port

**User Story:** As a developer, I want the query engine ported to Go, so that prompts can be processed with turn limits, budget tracking, and streaming support.

#### Acceptance Criteria

1. THE Query_Engine SHALL define a config struct with max_turns, max_budget_tokens, compact_after_turns, and structured_output fields
2. THE Query_Engine SHALL provide constructors from_workspace and from_saved_session that initialize engine state
3. WHEN submit_message is called, THE Query_Engine SHALL process the prompt and return a TurnResult containing the output, matched commands, matched tools, permission denials, usage summary, and stop reason
4. WHEN stream_submit_message is called, THE Query_Engine SHALL yield streaming events via a Go channel or callback
5. WHEN the turn count exceeds compact_after_turns, THE Query_Engine SHALL trigger message compaction
6. WHEN the cumulative token usage exceeds max_budget_tokens, THE Query_Engine SHALL stop processing and set the stop reason to budget_exceeded
7. THE Query_Engine SHALL provide persist_session and flush_transcript methods that delegate to Session_Store and Transcript_Store
8. THE Query_Engine SHALL provide a render_summary method that returns a Markdown-formatted summary string

### Requirement 9: Session Store Port

**User Story:** As a developer, I want session persistence ported to Go, so that sessions can be saved and restored from disk.

#### Acceptance Criteria

1. THE Session_Store SHALL define a StoredSession struct with JSON serialization tags
2. WHEN save_session is called, THE Session_Store SHALL write the session as a JSON file in the .port_sessions directory
3. WHEN load_session is called with a session ID, THE Session_Store SHALL read and deserialize the JSON file into a StoredSession
4. WHEN a session is saved and then loaded, THE Session_Store SHALL produce an equivalent StoredSession (round-trip property)
5. IF the .port_sessions directory does not exist, THEN THE Session_Store SHALL create the directory before writing

### Requirement 10: History Log Port

**User Story:** As a developer, I want session history ported to Go, so that session events can be recorded and rendered.

#### Acceptance Criteria

1. THE History_Log SHALL define HistoryEvent and HistoryLog structs
2. WHEN an event is appended, THE History_Log SHALL add the event to the log in order
3. THE History_Log SHALL provide a Markdown render method that formats all events

### Requirement 11: Transcript Store Port

**User Story:** As a developer, I want transcript management ported to Go, so that conversation transcripts can be appended, compacted, replayed, and flushed.

#### Acceptance Criteria

1. THE Transcript_Store SHALL support append, compact, replay, and flush operations
2. WHEN append is called, THE Transcript_Store SHALL add the entry to the end of the transcript
3. WHEN compact is called, THE Transcript_Store SHALL reduce the transcript size while preserving essential content
4. WHEN replay is called, THE Transcript_Store SHALL return all transcript entries in order
5. WHEN flush is called, THE Transcript_Store SHALL write the transcript to disk and clear the in-memory buffer

### Requirement 12: Runtime Port

**User Story:** As a developer, I want the main runtime ported to Go, so that prompt routing and session management work end-to-end.

#### Acceptance Criteria

1. THE Runtime SHALL define a RoutedMatch struct with kind, name, source_hint, and score fields
2. THE Runtime SHALL define a RuntimeSession struct that holds full session state and provides a Markdown render method
3. WHEN route_prompt is called, THE Runtime SHALL match the prompt against registered commands and tools and return a list of RoutedMatch results sorted by score
4. WHEN bootstrap_session is called, THE Runtime SHALL initialize a new RuntimeSession with all subsystems ready

### Requirement 13: Setup and Prefetch Port

**User Story:** As a developer, I want workspace setup and prefetch ported to Go, so that environment detection and concurrent prefetches work correctly.

#### Acceptance Criteria

1. THE Setup_Report SHALL detect Go version, platform name, and test command for the workspace
2. THE Prefetch_Engine SHALL run MDM raw read, keychain prefetch, and project scan concurrently using goroutines
3. WHEN run_setup is called, THE Setup_Report SHALL return a report containing prefetch results and deferred init state
4. IF a prefetch operation fails, THEN THE Prefetch_Engine SHALL capture the error in the PrefetchResult without stopping other prefetches

### Requirement 14: Deferred Initialization Port

**User Story:** As a developer, I want deferred initialization ported to Go, so that plugins, skills, MCP prefetch, and session hooks initialize after bootstrap.

#### Acceptance Criteria

1. THE Deferred_Init SHALL define a result struct with trusted flag, plugin init, skill init, MCP prefetch, and session hooks fields
2. WHEN run_deferred_init is called, THE Deferred_Init SHALL perform all initialization steps and return the result
3. IF any initialization step fails, THEN THE Deferred_Init SHALL capture the error in the result without stopping other steps

### Requirement 15: System Init and Bootstrap Graph Port

**User Story:** As a developer, I want system initialization and bootstrap graph ported to Go, so that session startup messages and stage dependencies are correctly built.

#### Acceptance Criteria

1. THE System_Init SHALL build a system initialization message string for a new session
2. THE Bootstrap_Graph SHALL define the bootstrap stage dependency graph as a data structure
3. THE Bootstrap_Graph SHALL provide a render method that outputs the graph as Markdown
4. THE Command_Graph SHALL segment commands into builtin, plugin-like, and skill-like categories

### Requirement 16: Port Manifest Port

**User Story:** As a developer, I want the workspace manifest ported to Go, so that the source directory can be scanned and a manifest generated.

#### Acceptance Criteria

1. THE Port_Manifest SHALL define a manifest struct with a Markdown render method
2. WHEN build_port_manifest is called, THE Port_Manifest SHALL scan the source directory and populate the manifest with discovered modules
3. IF the source directory does not exist, THEN THE Port_Manifest SHALL return an error

### Requirement 17: Direct and Remote Modes Port

**User Story:** As a developer, I want direct connect, deep link, remote, SSH, and teleport modes ported to Go, so that all runtime connection modes are available.

#### Acceptance Criteria

1. THE Runtime SHALL support direct-connect mode that establishes a local connection
2. THE Runtime SHALL support deep-link mode that generates a connection URL
3. THE Runtime SHALL support remote-mode for remote runtime connections
4. THE Runtime SHALL support ssh-mode for SSH-tunneled connections
5. THE Runtime SHALL support teleport-mode for teleport-based connections
6. WHEN any connection mode fails, THE Runtime SHALL return a descriptive error

### Requirement 18: CLI Entrypoint Port

**User Story:** As a developer, I want the CLI entrypoint ported to Go with all subcommands, so that the Go binary provides the same CLI interface as the Python version.

#### Acceptance Criteria

1. THE CLI_Entrypoint SHALL provide subcommands: summary, manifest, parity-audit, setup-report, command-graph, tool-pool, bootstrap-graph, subsystems, commands, tools, route, bootstrap, turn-loop, flush-transcript, load-session, remote-mode, ssh-mode, teleport-mode, direct-connect, deep-link
2. WHEN a subcommand is invoked with valid arguments, THE CLI_Entrypoint SHALL execute the corresponding operation and print the result to stdout
3. IF a subcommand receives invalid arguments, THEN THE CLI_Entrypoint SHALL print a usage message to stderr and exit with a non-zero code
4. THE CLI_Entrypoint SHALL use a Go CLI framework (cobra or equivalent) for argument parsing and help generation


### Requirement 19: Kiro CLI Integration

**User Story:** As a developer, I want the Go port to integrate with the Kiro CLI, so that I can use the runtime within the Kiro ecosystem.

#### Acceptance Criteria

1. THE Kiro_Integration SHALL expose the Go port as a Kiro-compatible CLI tool that can be invoked from Kiro workflows
2. THE Kiro_Integration SHALL support Kiro hooks integration by reading and executing hook definitions from .kiro/hooks directories
3. THE Steering_File_Generator SHALL generate Kiro-compatible steering files from runtime configuration and session state
4. THE MCP_Server SHALL expose tool and command registries over the Model Context Protocol via stdio or HTTP transport
5. WHEN the MCP_Server receives a valid tool invocation request, THE MCP_Server SHALL execute the tool and return the result in MCP response format
6. WHEN the MCP_Server receives an invalid request, THE MCP_Server SHALL return an MCP-compliant error response
7. THE Kiro_Integration SHALL support the Kiro spec-driven development workflow by reading spec files from .kiro/specs directories

### Requirement 20: Go Project Structure

**User Story:** As a developer, I want the Go port to follow idiomatic Go project structure, so that the codebase is maintainable and follows community conventions.

#### Acceptance Criteria

1. THE Runtime SHALL be organized as a Go module with a go.mod file at the project root
2. THE Runtime SHALL organize code into packages that map logically to the Python module boundaries (models, permissions, context, commands, tools, query engine, session, runtime, setup, CLI)
3. THE Runtime SHALL use Go interfaces to define contracts between subsystems
4. THE Runtime SHALL use Go error handling conventions (returning error values) instead of exceptions
5. THE Runtime SHALL use goroutines and channels for concurrent operations (prefetch, streaming)
6. THE Runtime SHALL provide a Makefile or Go build tags for building the CLI binary

### Requirement 21: Reference Data Loading

**User Story:** As a developer, I want reference data (command and tool snapshots) embedded or loadable in the Go binary, so that the registry can initialize without external file dependencies.

#### Acceptance Criteria

1. THE Runtime SHALL embed reference data JSON files using Go embed directives
2. WHEN the embedded data is loaded, THE Command_Registry and Tool_Registry SHALL parse the JSON and populate their indexes
3. IF the embedded JSON is malformed, THEN THE Runtime SHALL return a descriptive parse error at initialization

### Requirement 22: JSON Serialization Round-Trip

**User Story:** As a developer, I want all serializable structs to support JSON round-trip, so that data integrity is maintained across save/load cycles.

#### Acceptance Criteria

1. FOR ALL serializable structs, serializing to JSON and deserializing back SHALL produce an equivalent struct
2. THE Runtime SHALL use Go encoding/json with struct tags for all serialization
3. WHEN a JSON field is missing during deserialization, THE Runtime SHALL use the Go zero value for that field
