package mcpclient

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/AlleyBo55/gocode/internal/apitypes"
)

// --- JSON-RPC types (local to avoid circular imports with internal/mcp) ---

// jsonRPCRequest represents a JSON-RPC 2.0 request.
type jsonRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int64       `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// jsonRPCResponse represents a JSON-RPC 2.0 response.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

// jsonRPCError represents a JSON-RPC 2.0 error object.
type jsonRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *jsonRPCError) Error() string {
	return fmt.Sprintf("JSON-RPC error %d: %s", e.Code, e.Message)
}

// --- MCP protocol types ---

// initializeParams is sent during the initialize handshake.
type initializeParams struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Capabilities    struct{}   `json:"capabilities"`
	ClientInfo      clientInfo `json:"clientInfo"`
}

type clientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// initializeResult is the server's response to initialize.
type initializeResult struct {
	ProtocolVersion string `json:"protocolVersion"`
}

// mcpToolDef is a tool definition returned by tools/list.
type mcpToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// toolsListResult is the result of a tools/list call.
type toolsListResult struct {
	Tools []mcpToolDef `json:"tools"`
}

// toolCallParams is sent for tools/call.
type toolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// toolCallResult is the result of a tools/call.
type toolCallResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError,omitempty"`
}

// --- Configuration ---

// ClientConfig is loaded from .gocode/mcp.json.
type ClientConfig struct {
	Servers []ServerDef `json:"servers"`
}

// ServerDef defines an external MCP server to connect to.
type ServerDef struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// --- Server connection ---

// ServerConn represents a live connection to an external MCP server.
type ServerConn struct {
	def    ServerDef
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	nextID int64
	tools  []apitypes.ToolDef
	mu     sync.Mutex
	alive  bool
}

// nextRequestID returns the next unique request ID for this connection.
func (sc *ServerConn) nextRequestID() int64 {
	return atomic.AddInt64(&sc.nextID, 1)
}

// send writes a JSON-RPC request using Content-Length framing and reads the response.
func (sc *ServerConn) send(method string, params interface{}) (*jsonRPCResponse, error) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if !sc.alive {
		return nil, fmt.Errorf("server %q is not running", sc.def.Name)
	}

	id := sc.nextRequestID()
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	// Write with Content-Length header (LSP/MCP framing)
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	if _, err := io.WriteString(sc.stdin, header); err != nil {
		sc.alive = false
		return nil, fmt.Errorf("writing header to server %q: %w", sc.def.Name, err)
	}
	if _, err := sc.stdin.Write(body); err != nil {
		sc.alive = false
		return nil, fmt.Errorf("writing body to server %q: %w", sc.def.Name, err)
	}

	// Read response with Content-Length framing
	contentLen := 0
	for {
		line, err := sc.stdout.ReadString('\n')
		if err != nil {
			sc.alive = false
			return nil, fmt.Errorf("reading response header from server %q: %w", sc.def.Name, err)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			// Empty line separates headers from body
			break
		}
		if strings.HasPrefix(line, "Content-Length:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			if _, err := fmt.Sscanf(val, "%d", &contentLen); err != nil {
				return nil, fmt.Errorf("parsing Content-Length from server %q: %w", sc.def.Name, err)
			}
		}
	}

	if contentLen <= 0 {
		return nil, fmt.Errorf("invalid Content-Length %d from server %q", contentLen, sc.def.Name)
	}

	respBody := make([]byte, contentLen)
	if _, err := io.ReadFull(sc.stdout, respBody); err != nil {
		sc.alive = false
		return nil, fmt.Errorf("reading response body from server %q: %w", sc.def.Name, err)
	}

	var resp jsonRPCResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("unmarshaling response from server %q: %w", sc.def.Name, err)
	}

	return &resp, nil
}

// close terminates the server process.
func (sc *ServerConn) close() {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.alive = false
	if sc.stdin != nil {
		sc.stdin.Close()
	}
	if sc.cmd != nil && sc.cmd.Process != nil {
		sc.cmd.Process.Kill()
		sc.cmd.Wait()
	}
}

// --- Manager ---

// Manager connects to external MCP servers as a client.
type Manager struct {
	mu      sync.RWMutex
	servers map[string]*ServerConn
	config  ClientConfig
}

// builtinServers returns the default built-in server configurations.
func builtinServers() []ServerDef {
	return []ServerDef{
		{
			Name:    "web-search",
			Command: "npx",
			Args:    []string{"-y", "@anthropic/brave-search-mcp"},
			Env:     map[string]string{"BRAVE_API_KEY": "${BRAVE_API_KEY}"},
		},
		{
			Name:    "docs-lookup",
			Command: "npx",
			Args:    []string{"-y", "@anthropic/docs-lookup-mcp"},
			Env:     map[string]string{},
		},
		{
			Name:    "code-search",
			Command: "npx",
			Args:    []string{"-y", "@anthropic/code-search-mcp"},
			Env:     map[string]string{},
		},
	}
}

// NewManager creates a manager and loads config from the given path.
// If the config file does not exist, built-in server configs are used.
func NewManager(configPath string) (*Manager, error) {
	m := &Manager{
		servers: make(map[string]*ServerConn),
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Use built-in defaults when no config file exists
			m.config = ClientConfig{Servers: builtinServers()}
			return m, nil
		}
		return nil, fmt.Errorf("reading MCP client config %s: %w", configPath, err)
	}

	if err := json.Unmarshal(data, &m.config); err != nil {
		return nil, fmt.Errorf("parsing MCP client config %s: %w", configPath, err)
	}

	return m, nil
}

// ConnectAll spawns all configured servers, performs the initialize handshake,
// and discovers tools via tools/list.
func (m *Manager) ConnectAll() error {
	var errs []string
	for _, def := range m.config.Servers {
		if err := m.connectServer(def); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", def.Name, err))
			log.Printf("[mcpclient] failed to connect to server %q: %v", def.Name, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed to connect to some MCP servers: %s", strings.Join(errs, "; "))
	}
	return nil
}

// connectServer spawns a single MCP server, performs the handshake, and discovers tools.
func (m *Manager) connectServer(def ServerDef) error {
	// Build environment
	env := os.Environ()
	for k, v := range def.Env {
		// Expand environment variable references like ${VAR}
		expanded := os.ExpandEnv(v)
		env = append(env, fmt.Sprintf("%s=%s", k, expanded))
	}

	cmd := exec.Command(def.Command, def.Args...)
	cmd.Env = env

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("creating stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		stdinPipe.Close()
		return fmt.Errorf("creating stdout pipe: %w", err)
	}

	// Discard stderr to avoid blocking
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting server process %q: %w", def.Command, err)
	}

	sc := &ServerConn{
		def:    def,
		cmd:    cmd,
		stdin:  stdinPipe,
		stdout: bufio.NewReaderSize(stdoutPipe, 1024*1024),
		alive:  true,
	}

	// Monitor for unexpected exit
	go m.monitorServer(def.Name, sc)

	// Perform initialize handshake
	if err := m.initializeHandshake(sc); err != nil {
		sc.close()
		return fmt.Errorf("initialize handshake: %w", err)
	}

	// Send initialized notification (no response expected — but we don't read for notifications)
	// The notification is fire-and-forget per MCP spec
	m.sendNotification(sc, "notifications/initialized", nil)

	// Discover tools
	if err := m.discoverTools(sc, def.Name); err != nil {
		sc.close()
		return fmt.Errorf("discovering tools: %w", err)
	}

	m.mu.Lock()
	m.servers[def.Name] = sc
	m.mu.Unlock()

	log.Printf("[mcpclient] connected to server %q, discovered %d tools", def.Name, len(sc.tools))
	return nil
}

// sendNotification sends a JSON-RPC notification (no id, no response expected).
func (m *Manager) sendNotification(sc *ServerConn, method string, params interface{}) {
	type notification struct {
		JSONRPC string      `json:"jsonrpc"`
		Method  string      `json:"method"`
		Params  interface{} `json:"params,omitempty"`
	}
	msg := notification{JSONRPC: "2.0", Method: method, Params: params}
	body, err := json.Marshal(msg)
	if err != nil {
		return
	}
	sc.mu.Lock()
	defer sc.mu.Unlock()
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	io.WriteString(sc.stdin, header)
	sc.stdin.Write(body)
}

// initializeHandshake performs the MCP initialize handshake and verifies protocol version.
func (m *Manager) initializeHandshake(sc *ServerConn) error {
	params := initializeParams{
		ProtocolVersion: "2024-11-05",
		Capabilities:    struct{}{},
		ClientInfo: clientInfo{
			Name:    "gocode",
			Version: "1.0.0",
		},
	}

	resp, err := sc.send("initialize", params)
	if err != nil {
		return fmt.Errorf("sending initialize: %w", err)
	}
	if resp.Error != nil {
		return resp.Error
	}

	var result initializeResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return fmt.Errorf("parsing initialize result: %w", err)
	}

	if result.ProtocolVersion != "2024-11-05" {
		return fmt.Errorf("unsupported protocol version %q (expected 2024-11-05)", result.ProtocolVersion)
	}

	return nil
}

// discoverTools calls tools/list and registers discovered tools with namespace prefixes.
func (m *Manager) discoverTools(sc *ServerConn, serverName string) error {
	resp, err := sc.send("tools/list", nil)
	if err != nil {
		return fmt.Errorf("calling tools/list: %w", err)
	}
	if resp.Error != nil {
		return resp.Error
	}

	var result toolsListResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return fmt.Errorf("parsing tools/list result: %w", err)
	}

	sc.tools = make([]apitypes.ToolDef, 0, len(result.Tools))
	for _, t := range result.Tools {
		namespacedName := fmt.Sprintf("mcp_%s_%s", serverName, t.Name)
		sc.tools = append(sc.tools, apitypes.ToolDef{
			Name:        namespacedName,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}

	return nil
}

// monitorServer watches for unexpected server process exit.
func (m *Manager) monitorServer(name string, sc *ServerConn) {
	if sc.cmd == nil {
		return
	}
	err := sc.cmd.Wait()

	sc.mu.Lock()
	wasAlive := sc.alive
	sc.alive = false
	sc.mu.Unlock()

	if wasAlive {
		log.Printf("[mcpclient] server %q exited unexpectedly: %v", name, err)
	}
}

// CallTool forwards a tools/call request to the appropriate server.
// The toolName must be in "mcp_servername_toolname" format.
func (m *Manager) CallTool(toolName string, args map[string]interface{}) (string, error) {
	// Parse namespace: mcp_servername_toolname
	if !strings.HasPrefix(toolName, "mcp_") {
		return "", fmt.Errorf("tool %q does not have mcp_ namespace prefix", toolName)
	}

	rest := toolName[4:] // strip "mcp_"
	serverName, originalToolName, err := splitServerTool(rest)
	if err != nil {
		return "", fmt.Errorf("parsing tool name %q: %w", toolName, err)
	}

	m.mu.RLock()
	sc, ok := m.servers[serverName]
	m.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("no MCP server %q connected", serverName)
	}

	if !sc.alive {
		return "", fmt.Errorf("MCP server %q is not running (exited unexpectedly)", serverName)
	}

	params := toolCallParams{
		Name:      originalToolName,
		Arguments: args,
	}

	resp, err := sc.send("tools/call", params)
	if err != nil {
		return "", fmt.Errorf("calling tool %q on server %q: %w", originalToolName, serverName, err)
	}
	if resp.Error != nil {
		return "", resp.Error
	}

	var result toolCallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", fmt.Errorf("parsing tools/call result: %w", err)
	}

	// Concatenate text content blocks
	var sb strings.Builder
	for i, c := range result.Content {
		if c.Type == "text" {
			if i > 0 && sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(c.Text)
		}
	}

	if result.IsError {
		return "", fmt.Errorf("tool error: %s", sb.String())
	}

	return sb.String(), nil
}

// splitServerTool splits "servername_toolname" into server name and tool name.
// The server name is the first segment before the first underscore.
func splitServerTool(s string) (serverName, toolName string, err error) {
	idx := strings.Index(s, "_")
	if idx < 0 {
		return "", "", fmt.Errorf("expected format servername_toolname, got %q", s)
	}
	return s[:idx], s[idx+1:], nil
}

// ListTools returns all discovered tools from all connected servers
// with namespace prefixes in "mcp_servername_toolname" format.
func (m *Manager) ListTools() []apitypes.ToolDef {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var all []apitypes.ToolDef
	for _, sc := range m.servers {
		sc.mu.Lock()
		if sc.alive {
			all = append(all, sc.tools...)
		}
		sc.mu.Unlock()
	}
	return all
}

// Close shuts down all server connections.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, sc := range m.servers {
		log.Printf("[mcpclient] closing server %q", name)
		sc.close()
	}
	m.servers = make(map[string]*ServerConn)
}
