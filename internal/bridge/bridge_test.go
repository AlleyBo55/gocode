package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestNewBridge(t *testing.T) {
	b := NewBridge(9999)
	if b == nil {
		t.Fatal("NewBridge returned nil")
	}
	if b.Port() != 9999 {
		t.Errorf("Port() = %d, want 9999", b.Port())
	}
}

func TestNewBridge_DefaultPort(t *testing.T) {
	b := NewBridge(0)
	if b == nil {
		t.Fatal("NewBridge returned nil")
	}
	if b.Port() != DefaultPort {
		t.Errorf("Port() = %d, want %d", b.Port(), DefaultPort)
	}
}

func TestBridgeStartStop(t *testing.T) {
	port := 19900
	b := NewBridge(port)

	ctx := context.Background()
	if err := b.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Give the server a moment to be ready.
	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/health", b.Port()))
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /health status = %d, want 200", resp.StatusCode)
	}

	if err := b.Stop(); err != nil {
		t.Errorf("Stop: %v", err)
	}
}

func TestBridgeSessions_Empty(t *testing.T) {
	b := NewBridge(0)
	sessions := b.Sessions()
	if len(sessions) != 0 {
		t.Errorf("Sessions() = %v, want empty slice", sessions)
	}
}

func TestMessageMarshal(t *testing.T) {
	types := []MessageType{MsgChat, MsgToolPermission, MsgStatus, MsgError, MsgNotification}

	for _, mt := range types {
		t.Run(string(mt), func(t *testing.T) {
			original := Message{
				Type:      mt,
				ID:        "test-id-123",
				SessionID: "sess-456",
				Payload:   json.RawMessage(`{"key":"value"}`),
			}

			data, err := json.Marshal(original)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}

			var decoded Message
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}

			if decoded.Type != original.Type {
				t.Errorf("Type = %q, want %q", decoded.Type, original.Type)
			}
			if decoded.ID != original.ID {
				t.Errorf("ID = %q, want %q", decoded.ID, original.ID)
			}
			if decoded.SessionID != original.SessionID {
				t.Errorf("SessionID = %q, want %q", decoded.SessionID, original.SessionID)
			}
			if string(decoded.Payload) != string(original.Payload) {
				t.Errorf("Payload = %s, want %s", decoded.Payload, original.Payload)
			}
		})
	}
}

func TestHandshakePayload(t *testing.T) {
	original := HandshakePayload{
		ClientType:      "vscode",
		SessionID:       "sess-abc",
		ProtocolVersion: "1.0",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded HandshakePayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.ClientType != original.ClientType {
		t.Errorf("ClientType = %q, want %q", decoded.ClientType, original.ClientType)
	}
	if decoded.SessionID != original.SessionID {
		t.Errorf("SessionID = %q, want %q", decoded.SessionID, original.SessionID)
	}
	if decoded.ProtocolVersion != original.ProtocolVersion {
		t.Errorf("ProtocolVersion = %q, want %q", decoded.ProtocolVersion, original.ProtocolVersion)
	}
}

func TestBridgePermissionPrompter_Timeout(t *testing.T) {
	b := NewBridge(0)
	prompter := NewBridgePermissionPrompter(b, "nonexistent-session")
	prompter.timeout = 10 * time.Millisecond

	approved, err := prompter.Prompt("bash", "run command")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if approved {
		t.Error("expected approved=false on timeout")
	}
}

func TestStreamToBridge_NoSession(t *testing.T) {
	b := NewBridge(0)
	err := StreamToBridge(b, "missing-session", "hello")
	if err == nil {
		t.Fatal("expected error for missing session, got nil")
	}
}

func TestNotifyToBridge_NoSession(t *testing.T) {
	b := NewBridge(0)
	err := NotifyToBridge(b, "missing-session", "notification text")
	if err == nil {
		t.Fatal("expected error for missing session, got nil")
	}
}
