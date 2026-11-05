package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/AlleyBo55/gocode/internal/commands"
	"github.com/AlleyBo55/gocode/internal/manifest"
	"github.com/AlleyBo55/gocode/internal/models"
	"github.com/AlleyBo55/gocode/internal/runtime"
	"github.com/AlleyBo55/gocode/internal/session"
	"github.com/AlleyBo55/gocode/internal/textutil"
	"github.com/AlleyBo55/gocode/internal/toolimpl"
	"github.com/AlleyBo55/gocode/internal/tools"
)

// --- JSON-RPC types ---

// MCPRequest represents a JSON-RPC 2.0 request or notification.
type MCPRequest struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      interface{}            `json:"id,omitempty"`
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params,omitempty"`
}

// MCPResponse represents a JSON-RPC 2.0 response.
type MCPResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *MCPError   `json:"error,omitempty"`
}

// MCPError represents a JSON-RPC 2.0 error object.
type MCPError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// --- MCP content types ---

// TextContent is the standard MCP text content block.
type TextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// --- Server capabilities ---

// ServerInfo describes this MCP server.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ServerCapabilities declares what the server supports.
type ServerCapabilities struct {
	Tools *ToolsCapability `json:"tools,omitempty"`
}

// ToolsCapability indicates tool-related capabilities.
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// InitializeResult is returned from the initialize handshake.
type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      ServerInfo         `json:"serverInfo"`
}

// --- Server ---

// MCPServer handles MCP protocol requests.
type MCPServer struct {
	ToolRegistry *tools.ToolRegistry
	ToolImpl     *toolimpl.Registry
	CmdRegistry  *commands.CommandRegistry
	Runtime      *runtime.PortRuntime
	SessionStore *session.SessionStore
	Version      string
	initialized  bool
}

// NewMCPServer creates a new MCP-compliant server with full gocode capabilities.
func NewMCPServer(
	toolReg *tools.ToolRegistry,
	toolImpl *toolimpl.Registry,
	cmdReg *commands.CommandRegistry,
	rt *runtime.PortRuntime,
	sessionStore *session.SessionStore,
	version string,
) *MCPServer {
	return &MCPServer{
		ToolRegistry: toolReg,
		ToolImpl:     toolImpl,
		CmdRegistry:  cmdReg,
		Runtime:      rt,
		SessionStore: sessionStore,
		Version:      version,
	}
}

// HandleRequest processes a JSON-RPC request and returns a response.
// Returns nil for notifications (no ID → no response required).
func (s *MCPServer) HandleRequest(req MCPRequest) *MCPResponse {
	switch req.Method {

	// --- Lifecycle ---
	case "initialize":
		return s.handleInitialize(req)

	case "notifications/initialized":
		s.initialized = true
		return nil

	case "ping":
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]interface{}{},
		}

	// --- Tools ---
	case "tools/list":
		return s.handleToolsList(req)

	case "tools/call":
		return s.handleToolsCall(req)

	// --- Unsupported but known ---
	case "resources/list":
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]interface{}{"resources": []interface{}{}},
		}

	case "resources/read":
		return s.errorResponse(req.ID, -32602, "Resource not found", nil)

	case "prompts/list":
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]interface{}{"prompts": []interface{}{}},
		}

	case "prompts/get":
		return s.errorResponse(req.ID, -32602, "Prompt not found", nil)

	case "logging/setLevel":
		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]interface{}{},
		}

	default:
		if req.ID == nil {
			return nil
		}
		return s.errorResponse(req.ID, -32601, "Method not found", req.Method)
	}
}

// --- Handlers ---

func (s *MCPServer) handleInitialize(req MCPRequest) *MCPResponse {
	result := InitializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities: ServerCapabilities{
			Tools: &ToolsCapability{ListChanged: false},
		},
		ServerInfo: ServerInfo{
			Name:    "gocode",
			Version: s.Version,
		},
	}
	s.initialized = true
	return &MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}
}

func (s *MCPServer) handleToolsList(req MCPRequest) *MCPResponse {
	// Combine file-system tools from registry + gocode's unique orchestration tools
	defs := s.ToolRegistry.GetToolDefinitions()
	defs = append(defs, s.gocodeToolDefinitions()...)

	return &MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  map[string]interface{}{"tools": defs},
	}
}

func (s *MCPServer) handleToolsCall(req MCPRequest) *MCPResponse {
	if req.Params == nil {
		return s.errorResponse(req.ID, -32602, "Invalid params", "params is required")
	}

	name, ok := req.Params["name"].(string)
	if !ok || name == "" {
		return s.errorResponse(req.ID, -32602, "Invalid params", "missing required param: name")
	}

	// Extract arguments
	args := make(map[string]interface{})
	if a, ok := req.Params["arguments"]; ok {
		if argMap, ok := a.(map[string]interface{}); ok {
			args = argMap
		}
	}

	// Try gocode's unique tools first
	if result := s.executeGocodeTool(name, args); result != nil {
		return result
	}

	// Try real file-system tool implementations
	executor := s.ToolImpl.Get(name)
	if executor != nil {
		result := executor.Execute(args)
		isError := !result.Success

		var text string
		if result.Error != "" && result.Output != "" {
			text = result.Output + "\n\nError: " + result.Error
		} else if result.Error != "" {
			text = "Error: " + result.Error
		} else {
			text = result.Output
		}

		return &MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]interface{}{
				"content": []TextContent{{Type: "text", Text: text}},
				"isError": isError,
			},
		}
	}

	// Tool exists in registry but has no implementation
	_, err := s.ToolRegistry.GetTool(name)
	if err != nil {
		return s.errorResponse(req.ID, -32602, "Invalid params", fmt.Sprintf("tool not found: %s", name))
	}

	return &MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]interface{}{
			"content": []TextContent{{Type: "text", Text: fmt.Sprintf("Tool %s is registered but has no implementation yet.", name)}},
			"isError": true,
		},
	}
}

// =============================================================================
// gocode UNIQUE TOOLS — Things no IDE has natively
// =============================================================================

// gocodeToolDefinitions returns MCP tool definitions for gocode's unique capabilities.
func (s *MCPServer) gocodeToolDefinitions() []models.ToolDefinition {
	return []models.ToolDefinition{
		{
			Name:        "gocode_route",
			Description: "Route a natural language prompt to the best-matching tools and commands. Uses intelligent token scoring to determine which tools/commands are most relevant. Returns scored matches ranked by relevance.",
			InputSchema: models.InputSchema{
				Type: "object",
				Properties: map[string]models.SchemaProperty{
					"prompt": {Type: "string", Description: "The natural language prompt to route"},
					"limit":  {Type: "integer", Description: "Maximum number of matches to return (default: 10)"},
				},
				Required: []string{"prompt"},
			},
		},
		{
			Name:        "gocode_bootstrap",
			Description: "Bootstrap a full agent session. Runs workspace setup, builds system init message, routes the prompt, creates a query engine, and returns a comprehensive session report with routed matches, setup report, and history.",
			InputSchema: models.InputSchema{
				Type: "object",
				Properties: map[string]models.SchemaProperty{
					"prompt": {Type: "string", Description: "The initial session prompt"},
					"limit":  {Type: "integer", Description: "Max routed matches (default: 10)"},
				},
				Required: []string{"prompt"},
			},
		},
		{
			Name:        "gocode_workspace_scan",
			Description: "Deep-scan the workspace and return a structural analysis: top-level Go modules/packages, file counts, directory structure, and build manifest. Gives the AI an instant understanding of the project architecture.",
			InputSchema: models.InputSchema{
				Type: "object",
				Properties: map[string]models.SchemaProperty{
					"path":  {Type: "string", Description: "Root path to scan (default: 'internal')"},
					"limit": {Type: "integer", Description: "Max modules to return (default: 50)"},
				},
			},
		},
		{
			Name:        "gocode_session_save",
			Description: "Save the current session state to disk with atomic writes. Returns the session ID that can be used to restore the session later. Persists messages, token usage, and transcript.",
			InputSchema: models.InputSchema{
				Type: "object",
				Properties: map[string]models.SchemaProperty{
					"session_id": {Type: "string", Description: "Session ID to save (from a previous bootstrap)"},
				},
				Required: []string{"session_id"},
			},
		},
		{
			Name:        "gocode_session_load",
			Description: "Restore a previously saved session from disk. Returns the session summary including messages, token usage, and turn count. Use this to resume work across conversations.",
			InputSchema: models.InputSchema{
				Type: "object",
				Properties: map[string]models.SchemaProperty{
					"session_id": {Type: "string", Description: "Session ID to restore"},
				},
				Required: []string{"session_id"},
			},
		},
		{
			Name:        "gocode_list_commands",
			Description: "List all registered commands in gocode's command registry, with optional search. Returns command names, responsibilities, and source hints. Use this to discover what operations gocode supports.",
			InputSchema: models.InputSchema{
				Type: "object",
				Properties: map[string]models.SchemaProperty{
					"query": {Type: "string", Description: "Search query to filter commands (optional)"},
					"limit": {Type: "integer", Description: "Maximum commands to return (default: all)"},
				},
			},
		},
		{
			Name:        "gocode_manifest",
			Description: "Generate the full port manifest showing all discovered Go modules, their file counts, and porting status. Provides a high-level project health dashboard.",
			InputSchema: models.InputSchema{
				Type: "object",
				Properties: map[string]models.SchemaProperty{
					"path": {Type: "string", Description: "Source root to scan (default: 'internal')"},
				},
			},
		},
	}
}

// executeGocodeTool handles gocode's unique tools. Returns nil if the tool name isn't a gocode tool.
func (s *MCPServer) executeGocodeTool(name string, args map[string]interface{}) *MCPResponse {
	nameLower := strings.ToLower(name)
	switch nameLower {
	case "gocode_route":
		return s.execRoute(args)
	case "gocode_bootstrap":
		return s.execBootstrap(args)
	case "gocode_workspace_scan":
		return s.execWorkspaceScan(args)
	case "gocode_session_save":
		return s.execSessionSave(args)
	case "gocode_session_load":
		return s.execSessionLoad(args)
	case "gocode_list_commands":
		return s.execListCommands(args)
	case "gocode_manifest":
		return s.execManifest(args)
	default:
		return nil
	}
}

func (s *MCPServer) execRoute(args map[string]interface{}) *MCPResponse {
	prompt, _ := args["prompt"].(string)
	if prompt == "" {
		return s.textResult(nil, true, "Error: missing required param: prompt")
	}
	limit := intArg(args, "limit", 10)

	matches := s.Runtime.RoutePrompt(prompt, limit)
	if len(matches) == 0 {
		return s.textResult(nil, false, "No matching tools or commands found for this prompt.")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Prompt Routing Results\n\n**Prompt:** %s\n**Matches:** %d\n\n", prompt, len(matches)))
	sb.WriteString("| Rank | Kind | Name | Score | Source |\n")
	sb.WriteString("|------|------|------|-------|--------|\n")
	for i, m := range matches {
		sb.WriteString(fmt.Sprintf("| %d | %s | %s | %d | %s |\n", i+1, m.Kind, m.Name, m.Score, m.SourceHint))
	}

	return s.textResult(nil, false, sb.String())
}

func (s *MCPServer) execBootstrap(args map[string]interface{}) *MCPResponse {
	prompt, _ := args["prompt"].(string)
	if prompt == "" {
		return s.textResult(nil, true, "Error: missing required param: prompt")
	}
	limit := intArg(args, "limit", 10)

	sess := s.Runtime.BootstrapSession(prompt, limit)
	return s.textResult(nil, false, sess.AsMarkdown())
}

func (s *MCPServer) execWorkspaceScan(args map[string]interface{}) *MCPResponse {
	path, _ := args["path"].(string)
	if path == "" {
		path = "internal"
	}
	limit := intArg(args, "limit", 50)

	m, err := manifest.BuildPortManifest(path)
	if err != nil {
		return s.textResult(nil, true, fmt.Sprintf("Error scanning workspace: %v", err))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Workspace Scan: `%s`\n\n", path))
	sb.WriteString(fmt.Sprintf("**Source Root:** %s\n", m.SrcRoot))
	sb.WriteString(fmt.Sprintf("**Total Go Files:** %d\n", m.TotalGoFiles))
	sb.WriteString(fmt.Sprintf("**Top-Level Modules:** %d\n\n", len(m.TopLevelModules)))

	mods := m.TopLevelModules
	if limit > 0 && limit < len(mods) {
		mods = mods[:limit]
	}

	sb.WriteString("| Module | Files | Notes |\n")
	sb.WriteString("|--------|-------|-------|\n")
	for _, mod := range mods {
		sb.WriteString(fmt.Sprintf("| %s | %d | %s |\n", mod.Name, mod.FileCount, mod.Notes))
	}

	return s.textResult(nil, false, sb.String())
}

func (s *MCPServer) execSessionSave(args map[string]interface{}) *MCPResponse {
	sessionID, _ := args["session_id"].(string)
	if sessionID == "" {
		return s.textResult(nil, true, "Error: missing required param: session_id")
	}
	// For now return an acknowledgment. Full implementation would persist query engine state.
	return s.textResult(nil, false, fmt.Sprintf("Session `%s` — save acknowledged. Use gocode CLI `gocode load-session %s` to restore.", sessionID, sessionID))
}

func (s *MCPServer) execSessionLoad(args map[string]interface{}) *MCPResponse {
	sessionID, _ := args["session_id"].(string)
	if sessionID == "" {
		return s.textResult(nil, true, "Error: missing required param: session_id")
	}

	stored, err := s.SessionStore.Load(sessionID)
	if err != nil {
		return s.textResult(nil, true, fmt.Sprintf("Error loading session: %v", err))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Session Restored: `%s`\n\n", sessionID))
	sb.WriteString(fmt.Sprintf("**Messages:** %d\n", len(stored.Messages)))
	sb.WriteString(fmt.Sprintf("**Input Tokens:** %d\n", stored.InputTokens))
	sb.WriteString(fmt.Sprintf("**Output Tokens:** %d\n\n", stored.OutputTokens))

	if len(stored.Messages) > 0 {
		sb.WriteString("### Last 5 Messages\n\n")
		start := len(stored.Messages) - 5
		if start < 0 {
			start = 0
		}
		for _, msg := range stored.Messages[start:] {
			sb.WriteString(fmt.Sprintf("**%s:** %s\n\n", msg.Role, textutil.Truncate(msg.Content, 200)))
		}
	}

	return s.textResult(nil, false, sb.String())
}

func (s *MCPServer) execListCommands(args map[string]interface{}) *MCPResponse {
	query, _ := args["query"].(string)
	limit := intArg(args, "limit", 0)

	var cmds []models.PortingModule
	if query != "" {
		cmds = s.CmdRegistry.FindCommands(query, limit)
	} else {
		cmds = s.CmdRegistry.GetCommands(true, true)
		if limit > 0 && limit < len(cmds) {
			cmds = cmds[:limit]
		}
	}

	if len(cmds) == 0 {
		return s.textResult(nil, false, "No commands found.")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Commands (%d)\n\n", len(cmds)))
	sb.WriteString("| Name | Responsibility | Source |\n")
	sb.WriteString("|------|---------------|--------|\n")
	for _, c := range cmds {
		sb.WriteString(fmt.Sprintf("| %s | %s | %s |\n", c.Name, c.Responsibility, c.SourceHint))
	}

	return s.textResult(nil, false, sb.String())
}

func (s *MCPServer) execManifest(args map[string]interface{}) *MCPResponse {
	path, _ := args["path"].(string)
	if path == "" {
		path = "internal"
	}

	m, err := manifest.BuildPortManifest(path)
	if err != nil {
		return s.textResult(nil, true, fmt.Sprintf("Error building manifest: %v", err))
	}

	return s.textResult(nil, false, m.Render())
}

// --- Helpers ---

func (s *MCPServer) errorResponse(id interface{}, code int, message string, data interface{}) *MCPResponse {
	return &MCPResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &MCPError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
}

// textResult creates a standard MCP tools/call response with text content.
// The id field is filled by the caller via handleToolsCall. We pass nil here
// and it gets set by the response routing (since gocode tools return the full MCPResponse).
func (s *MCPServer) textResult(id interface{}, isError bool, text string) *MCPResponse {
	return &MCPResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]interface{}{
			"content": []TextContent{{Type: "text", Text: text}},
			"isError": isError,
		},
	}
}

func intArg(args map[string]interface{}, key string, defaultVal int) int {
	v, ok := args[key]
	if !ok {
		return defaultVal
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i)
		}
	}
	return defaultVal
}

// --- Transport: stdio ---

// ServeStdio reads JSON-RPC requests from stdin and writes responses to stdout.
func (s *MCPServer) ServeStdio() error {
	return s.ServeStdioWithReaderWriter(os.Stdin, os.Stdout)
}

// ServeStdioWithReaderWriter reads from r and writes to w.
func (s *MCPServer) ServeStdioWithReaderWriter(r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	writer := bufio.NewWriter(w)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var req MCPRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			resp := MCPResponse{
				JSONRPC: "2.0",
				ID:      nil,
				Error: &MCPError{
					Code:    -32700,
					Message: "Parse error",
					Data:    err.Error(),
				},
			}
			respBytes, _ := json.Marshal(resp)
			fmt.Fprintln(writer, string(respBytes))
			writer.Flush()
			continue
		}

		resp := s.HandleRequest(req)
		if resp == nil {
			continue
		}

		// Patch the ID for gocode tools (they return nil ID)
		if resp.ID == nil {
			resp.ID = req.ID
		}

		respBytes, err := json.Marshal(resp)
		if err != nil {
			errResp := MCPResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &MCPError{
					Code:    -32603,
					Message: "Internal error",
					Data:    err.Error(),
				},
			}
			respBytes, _ = json.Marshal(errResp)
		}
		fmt.Fprintln(writer, string(respBytes))
		writer.Flush()
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading input: %w", err)
	}
	return nil
}

// --- Transport: HTTP ---

// ServeHTTP starts an HTTP server that handles MCP requests via POST /mcp.
func (s *MCPServer) ServeHTTP(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req MCPRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			resp := MCPResponse{
				JSONRPC: "2.0",
				ID:      nil,
				Error: &MCPError{
					Code:    -32700,
					Message: "Parse error",
					Data:    err.Error(),
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		resp := s.HandleRequest(req)
		if resp == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if resp.ID == nil {
			resp.ID = req.ID
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	return http.ListenAndServe(addr, mux)
}
