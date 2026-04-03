package bridge

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// DefaultPermissionTimeout is how long to wait for an IDE permission response.
const DefaultPermissionTimeout = 2 * time.Minute

// permissionPayload is the JSON payload sent to the IDE for a permission request.
type permissionPayload struct {
	ToolName  string `json:"tool_name"`
	Operation string `json:"operation"`
}

// permissionResponse is the JSON payload received from the IDE for a permission decision.
type permissionResponse struct {
	Approved bool `json:"approved"`
}

// BridgePermissionPrompter implements agent.PermissionPrompter by forwarding
// permission requests to the IDE over the WebSocket bridge.
type BridgePermissionPrompter struct {
	bridge    *Bridge
	sessionID string
	timeout   time.Duration

	mu       sync.Mutex
	pending  map[string]chan permissionResponse // msgID -> response channel
}

// NewBridgePermissionPrompter creates a prompter that forwards permission
// requests to the given bridge session.
func NewBridgePermissionPrompter(b *Bridge, sessionID string) *BridgePermissionPrompter {
	p := &BridgePermissionPrompter{
		bridge:    b,
		sessionID: sessionID,
		timeout:   DefaultPermissionTimeout,
		pending:   make(map[string]chan permissionResponse),
	}
	return p
}

// HandleResponse should be called when a permission response message arrives
// from the IDE. It matches the message ID to a pending request and delivers
// the result.
func (p *BridgePermissionPrompter) HandleResponse(msgID string, payload json.RawMessage) {
	var resp permissionResponse
	if err := json.Unmarshal(payload, &resp); err != nil {
		return
	}
	p.mu.Lock()
	ch, ok := p.pending[msgID]
	if ok {
		delete(p.pending, msgID)
	}
	p.mu.Unlock()
	if ok {
		ch <- resp
	}
}

// Prompt sends a tool_permission message to the IDE and waits for the response.
// Returns (true, nil) if approved, (false, nil) if denied, or error on timeout.
func (p *BridgePermissionPrompter) Prompt(toolName string, operation string) (bool, error) {
	msgID := generateID()

	payload, err := json.Marshal(permissionPayload{
		ToolName:  toolName,
		Operation: operation,
	})
	if err != nil {
		return false, fmt.Errorf("bridge permission: marshal payload: %w", err)
	}

	// Register a channel for the response before sending.
	ch := make(chan permissionResponse, 1)
	p.mu.Lock()
	p.pending[msgID] = ch
	p.mu.Unlock()

	// Send the permission request to the IDE.
	msg := Message{
		Type:      MsgToolPermission,
		ID:        msgID,
		SessionID: p.sessionID,
		Payload:   payload,
	}
	if err := p.bridge.Send(p.sessionID, msg); err != nil {
		p.mu.Lock()
		delete(p.pending, msgID)
		p.mu.Unlock()
		return false, fmt.Errorf("bridge permission: send: %w", err)
	}

	// Wait for the IDE response or timeout.
	select {
	case resp := <-ch:
		return resp.Approved, nil
	case <-time.After(p.timeout):
		p.mu.Lock()
		delete(p.pending, msgID)
		p.mu.Unlock()
		return false, fmt.Errorf("bridge permission: timeout waiting for IDE response")
	}
}
