// Package lsp provides a client for communicating with language servers
// over the Language Server Protocol (LSP) using JSON-RPC 2.0 over stdio.
package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// ---------------------------------------------------------------------------
// LSP domain types
// ---------------------------------------------------------------------------

// Position is a zero-based line/character offset in a text document.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range is a span between two positions.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Location is a range inside a particular document.
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// TextEdit is a textual edit applicable to a document.
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

// TextDocumentEdit groups edits for a single versioned document.
type TextDocumentEdit struct {
	TextDocument struct {
		URI     string `json:"uri"`
		Version *int   `json:"version,omitempty"`
	} `json:"textDocument"`
	Edits []TextEdit `json:"edits"`
}

// WorkspaceEdit describes a set of changes across documents.
type WorkspaceEdit struct {
	DocumentChanges []TextDocumentEdit `json:"documentChanges,omitempty"`
	// Some servers use the simpler "changes" map instead.
	Changes map[string][]TextEdit `json:"changes,omitempty"`
}

// Diagnostic represents a compiler/linter diagnostic for a document.
type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity,omitempty"` // 1=Error,2=Warning,3=Info,4=Hint
	Code     string `json:"code,omitempty"`
	Source   string `json:"source,omitempty"`
	Message  string `json:"message"`
}

// ---------------------------------------------------------------------------
// JSON-RPC 2.0 wire types
// ---------------------------------------------------------------------------

type jsonrpcRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int64       `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      int64            `json:"id"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *jsonrpcError    `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *jsonrpcError) Error() string {
	return fmt.Sprintf("LSP error %d: %s", e.Code, e.Message)
}

// ---------------------------------------------------------------------------
// Client
// ---------------------------------------------------------------------------

// Client manages a connection to a language server over stdio.
type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	reader *bufio.Reader

	nextID   int64 // atomic
	mu       sync.Mutex
	pending  map[int64]chan jsonrpcResponse
	closed   bool
	closeMu  sync.Mutex
	readErr  chan error
}

// NewClient spawns the language server process, wires up stdio, and performs
// the LSP initialize / initialized handshake. Returns a descriptive error if
// the language server binary is not found or the handshake fails.
func NewClient(command string, args ...string) (*Client, error) {
	path, err := exec.LookPath(command)
	if err != nil {
		return nil, fmt.Errorf("language server unavailable: %q not found on PATH: %w", command, err)
	}

	cmd := exec.Command(path, args...)

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("language server unavailable: cannot create stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("language server unavailable: cannot create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("language server unavailable: failed to start %q: %w", command, err)
	}

	c := &Client{
		cmd:     cmd,
		stdin:   stdinPipe,
		stdout:  stdoutPipe,
		reader:  bufio.NewReader(stdoutPipe),
		pending: make(map[int64]chan jsonrpcResponse),
		readErr: make(chan error, 1),
	}

	// Start background reader that dispatches responses to pending callers.
	go c.readLoop()

	// Perform the initialize handshake.
	if err := c.initialize(); err != nil {
		c.Close()
		return nil, fmt.Errorf("language server unavailable: initialize handshake failed: %w", err)
	}

	return c, nil
}

// ---------------------------------------------------------------------------
// Low-level JSON-RPC transport (Content-Length framing)
// ---------------------------------------------------------------------------

// send writes a JSON-RPC request with Content-Length header to the server.
func (c *Client) send(req jsonrpcRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return fmt.Errorf("language server unavailable: connection closed")
	}
	if _, err := io.WriteString(c.stdin, header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	if _, err := c.stdin.Write(body); err != nil {
		return fmt.Errorf("write body: %w", err)
	}
	return nil
}

// call sends a request and waits for the matching response.
func (c *Client) call(method string, params interface{}) (json.RawMessage, error) {
	id := atomic.AddInt64(&c.nextID, 1)

	ch := make(chan jsonrpcResponse, 1)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, fmt.Errorf("language server unavailable: connection closed")
	}
	c.pending[id] = ch
	c.mu.Unlock()

	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
	if err := c.send(req); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}

	resp := <-ch
	if resp.Error != nil {
		return nil, resp.Error
	}
	return resp.Result, nil
}

// notify sends a JSON-RPC notification (no id, no response expected).
func (c *Client) notify(method string, params interface{}) error {
	body, err := json.Marshal(struct {
		JSONRPC string      `json:"jsonrpc"`
		Method  string      `json:"method"`
		Params  interface{} `json:"params,omitempty"`
	}{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return fmt.Errorf("language server unavailable: connection closed")
	}
	if _, err := io.WriteString(c.stdin, header); err != nil {
		return err
	}
	_, err = c.stdin.Write(body)
	return err
}

// readLoop continuously reads Content-Length framed JSON-RPC messages from
// stdout and dispatches them to the appropriate pending caller.
func (c *Client) readLoop() {
	for {
		// Read headers until blank line.
		var contentLength int
		for {
			line, err := c.reader.ReadString('\n')
			if err != nil {
				c.readErr <- fmt.Errorf("read header: %w", err)
				c.drainPending()
				return
			}
			line = strings.TrimSpace(line)
			if line == "" {
				break // end of headers
			}
			if strings.HasPrefix(line, "Content-Length:") {
				val := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
				n, err := strconv.Atoi(val)
				if err != nil {
					c.readErr <- fmt.Errorf("invalid Content-Length %q: %w", val, err)
					c.drainPending()
					return
				}
				contentLength = n
			}
		}

		if contentLength <= 0 {
			continue
		}

		body := make([]byte, contentLength)
		if _, err := io.ReadFull(c.reader, body); err != nil {
			c.readErr <- fmt.Errorf("read body: %w", err)
			c.drainPending()
			return
		}

		var resp jsonrpcResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			// Skip malformed messages (could be server notifications).
			continue
		}

		// Dispatch to the waiting caller if this is a response (has an id).
		if resp.ID != 0 {
			c.mu.Lock()
			ch, ok := c.pending[resp.ID]
			if ok {
				delete(c.pending, resp.ID)
			}
			c.mu.Unlock()
			if ok {
				ch <- resp
			}
		}
	}
}

// drainPending closes all pending response channels so callers don't hang.
func (c *Client) drainPending() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, ch := range c.pending {
		close(ch)
		delete(c.pending, id)
	}
}

// ---------------------------------------------------------------------------
// LSP lifecycle
// ---------------------------------------------------------------------------

// initialize performs the LSP initialize request and sends initialized notification.
func (c *Client) initialize() error {
	params := map[string]interface{}{
		"processId": nil,
		"capabilities": map[string]interface{}{
			"textDocument": map[string]interface{}{
				"rename": map[string]interface{}{
					"prepareSupport": true,
				},
				"definition":  map[string]interface{}{},
				"references":  map[string]interface{}{},
				"publishDiagnostics": map[string]interface{}{},
			},
			"workspace": map[string]interface{}{
				"workspaceEdit": map[string]interface{}{
					"documentChanges": true,
				},
			},
		},
		"rootUri": nil,
	}

	result, err := c.call("initialize", params)
	if err != nil {
		return fmt.Errorf("initialize request: %w", err)
	}

	// We don't strictly need to parse the result, but verify it's valid JSON.
	var initResult map[string]interface{}
	if err := json.Unmarshal(result, &initResult); err != nil {
		return fmt.Errorf("parse initialize result: %w", err)
	}

	// Send the initialized notification.
	if err := c.notify("initialized", map[string]interface{}{}); err != nil {
		return fmt.Errorf("initialized notification: %w", err)
	}

	return nil
}

// Close performs a graceful LSP shutdown (shutdown request + exit notification)
// and then terminates the server process.
func (c *Client) Close() error {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()

	c.mu.Lock()
	alreadyClosed := c.closed
	c.closed = true
	c.mu.Unlock()

	if alreadyClosed {
		return nil
	}

	// Best-effort shutdown handshake.
	_, _ = c.call("shutdown", nil)
	_ = c.notify("exit", nil)

	// Close pipes and wait for process.
	c.stdin.Close()
	c.stdout.Close()
	return c.cmd.Wait()
}

// ---------------------------------------------------------------------------
// Helper: file URI conversion
// ---------------------------------------------------------------------------

// fileURI converts a file path to a file:// URI.
func fileURI(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	// On Windows filepath.Abs returns "C:\..." — url.URL handles this.
	return "file://" + url.PathEscape(filepath.ToSlash(abs))
}

// uriToPath converts a file:// URI back to a local path.
func uriToPath(uri string) string {
	if strings.HasPrefix(uri, "file://") {
		p := strings.TrimPrefix(uri, "file://")
		decoded, err := url.PathUnescape(p)
		if err == nil {
			return filepath.FromSlash(decoded)
		}
	}
	return uri
}

// textDocumentPositionParams builds the common LSP TextDocumentPositionParams.
// line and col are 0-based as per LSP spec.
func textDocumentPositionParams(file string, line, col int) map[string]interface{} {
	return map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": fileURI(file),
		},
		"position": map[string]interface{}{
			"line":      line,
			"character": col,
		},
	}
}

// ---------------------------------------------------------------------------
// LSP methods
// ---------------------------------------------------------------------------

// Rename sends a textDocument/rename request and returns the resulting
// workspace edits. line and col are 0-based.
func (c *Client) Rename(file string, line, col int, newName string) ([]WorkspaceEdit, error) {
	params := textDocumentPositionParams(file, line, col)
	params["newName"] = newName

	raw, err := c.call("textDocument/rename", params)
	if err != nil {
		return nil, fmt.Errorf("rename: %w", err)
	}
	if raw == nil || string(raw) == "null" {
		return nil, nil
	}

	var edit WorkspaceEdit
	if err := json.Unmarshal(raw, &edit); err != nil {
		return nil, fmt.Errorf("rename: parse response: %w", err)
	}
	return []WorkspaceEdit{edit}, nil
}

// GotoDefinition sends a textDocument/definition request and returns the
// definition locations. line and col are 0-based.
func (c *Client) GotoDefinition(file string, line, col int) ([]Location, error) {
	params := textDocumentPositionParams(file, line, col)

	raw, err := c.call("textDocument/definition", params)
	if err != nil {
		return nil, fmt.Errorf("gotoDefinition: %w", err)
	}
	if raw == nil || string(raw) == "null" {
		return nil, nil
	}

	return parseLocations(raw)
}

// FindReferences sends a textDocument/references request and returns all
// reference locations. line and col are 0-based.
func (c *Client) FindReferences(file string, line, col int) ([]Location, error) {
	params := textDocumentPositionParams(file, line, col)
	params["context"] = map[string]interface{}{
		"includeDeclaration": true,
	}

	raw, err := c.call("textDocument/references", params)
	if err != nil {
		return nil, fmt.Errorf("findReferences: %w", err)
	}
	if raw == nil || string(raw) == "null" {
		return nil, nil
	}

	return parseLocations(raw)
}

// Diagnostics sends a textDocument/diagnostic request (LSP 3.17+) to retrieve
// diagnostics for the given file. For servers that only support push-based
// diagnostics (textDocument/publishDiagnostics), this may return an empty
// slice — callers should use a notification handler in that case.
func (c *Client) Diagnostics(file string) ([]Diagnostic, error) {
	params := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": fileURI(file),
		},
	}

	raw, err := c.call("textDocument/diagnostic", params)
	if err != nil {
		return nil, fmt.Errorf("diagnostics: %w", err)
	}
	if raw == nil || string(raw) == "null" {
		return nil, nil
	}

	// The response is a DocumentDiagnosticReport which has an "items" field.
	var report struct {
		Items []Diagnostic `json:"items"`
	}
	if err := json.Unmarshal(raw, &report); err != nil {
		return nil, fmt.Errorf("diagnostics: parse response: %w", err)
	}
	return report.Items, nil
}

// parseLocations handles the LSP definition/references response which can be
// a single Location, an array of Location, or an array of LocationLink.
func parseLocations(raw json.RawMessage) ([]Location, error) {
	// Try array of Location first.
	var locs []Location
	if err := json.Unmarshal(raw, &locs); err == nil {
		return locs, nil
	}

	// Try single Location.
	var loc Location
	if err := json.Unmarshal(raw, &loc); err == nil {
		return []Location{loc}, nil
	}

	// Try array of LocationLink (extract targetUri + targetRange).
	var links []struct {
		TargetURI   string `json:"targetUri"`
		TargetRange Range  `json:"targetRange"`
	}
	if err := json.Unmarshal(raw, &links); err == nil {
		out := make([]Location, len(links))
		for i, l := range links {
			out[i] = Location{URI: l.TargetURI, Range: l.TargetRange}
		}
		return out, nil
	}

	return nil, fmt.Errorf("unexpected location format: %s", string(raw))
}
