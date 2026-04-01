package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/gocode/gocode/internal/commands"
	"github.com/gocode/gocode/internal/execution"
	"github.com/gocode/gocode/internal/tools"
)

// MCPRequest represents a JSON-RPC request in the MCP protocol.
type MCPRequest struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      interface{}            `json:"id"`
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params"`
}

// MCPResponse represents a JSON-RPC response in the MCP protocol.
type MCPResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *MCPError   `json:"error,omitempty"`
}

// MCPError represents a JSON-RPC error object.
type MCPError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// MCPServer handles MCP protocol requests using the execution, command, and tool registries.
type MCPServer struct {
	ExecRegistry *execution.ExecutionRegistry
	CmdRegistry  *commands.CommandRegistry
	ToolRegistry *tools.ToolRegistry
}

// NewMCPServer creates a new MCPServer with the given registries.
func NewMCPServer(execReg *execution.ExecutionRegistry, cmdReg *commands.CommandRegistry, toolReg *tools.ToolRegistry) *MCPServer {
	return &MCPServer{
		ExecRegistry: execReg,
		CmdRegistry:  cmdReg,
		ToolRegistry: toolReg,
	}
}

// HandleRequest processes an MCPRequest and returns an MCPResponse.
// Supported methods:
//   - "tools/list": returns a list of tool names
//   - "tools/call": looks up a tool in the execution registry, executes it, returns the result
//   - "commands/list": returns a list of command names
//   - Unknown methods return error code -32601 "Method not found"
//   - Invalid params return error code -32602 "Invalid params"
func (s *MCPServer) HandleRequest(req MCPRequest) MCPResponse {
	switch req.Method {
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(req)
	case "commands/list":
		return s.handleCommandsList(req)
	default:
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    -32601,
				Message: "Method not found",
			},
		}
	}
}

func (s *MCPServer) handleToolsList(req MCPRequest) MCPResponse {
	allTools := s.ToolRegistry.FindTools("", 0)
	names := make([]string, len(allTools))
	for i, t := range allTools {
		names[i] = t.Name
	}
	return MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  names,
	}
}

func (s *MCPServer) handleToolsCall(req MCPRequest) MCPResponse {
	if req.Params == nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    -32602,
				Message: "Invalid params",
				Data:    "params is required",
			},
		}
	}

	nameVal, ok := req.Params["name"]
	if !ok {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    -32602,
				Message: "Invalid params",
				Data:    "missing required param: name",
			},
		}
	}

	name, ok := nameVal.(string)
	if !ok {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    -32602,
				Message: "Invalid params",
				Data:    "param 'name' must be a string",
			},
		}
	}

	exec, err := s.ExecRegistry.Lookup(name)
	if err != nil {
		return MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &MCPError{
				Code:    -32602,
				Message: "Invalid params",
				Data:    fmt.Sprintf("tool not found: %s", name),
			},
		}
	}

	payload := ""
	if p, ok := req.Params["payload"]; ok {
		if ps, ok := p.(string); ok {
			payload = ps
		}
	}

	result := exec.Execute(payload)
	return MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}
}

func (s *MCPServer) handleCommandsList(req MCPRequest) MCPResponse {
	allCmds := s.CmdRegistry.FindCommands("", 0)
	names := make([]string, len(allCmds))
	for i, c := range allCmds {
		names[i] = c.Name
	}
	return MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  names,
	}
}

// ServeStdio reads JSON-RPC requests from stdin (one per line) and writes
// JSON-RPC responses to stdout (line-delimited). It runs until stdin is closed.
func (s *MCPServer) ServeStdio() error {
	scanner := bufio.NewScanner(os.Stdin)
	writer := bufio.NewWriter(os.Stdout)

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
		return fmt.Errorf("reading stdin: %w", err)
	}
	return nil
}

// ServeStdioWithReaderWriter is like ServeStdio but reads from r and writes to w.
// This is useful for testing.
func (s *MCPServer) ServeStdioWithReaderWriter(r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
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

// ServeHTTP starts an HTTP server on the given address that handles POST /mcp requests.
// Each request body should be a JSON-RPC MCPRequest; the response body is a JSON-RPC MCPResponse.
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
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	return http.ListenAndServe(addr, mux)
}
