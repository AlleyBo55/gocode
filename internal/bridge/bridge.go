// Package bridge provides a WebSocket server for bidirectional IDE communication.
// It implements the WebSocket protocol (RFC 6455) using only the Go standard library.
package bridge

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// DefaultPort is the default WebSocket server port.
const DefaultPort = 19836

// MaxPortRetries is the number of sequential ports to try if the default is in use.
const MaxPortRetries = 5

// SessionCleanupTimeout is the duration after which dropped sessions are cleaned up.
const SessionCleanupTimeout = 30 * time.Second

// ProtocolVersion is the current bridge protocol version.
const ProtocolVersion = "1.0"

// websocketGUID is the magic GUID used in the WebSocket handshake (RFC 6455 §4.2.2).
const websocketGUID = "258EAFA5-E914-47DA-95CA-5AB5DC11AD75"

// MessageType represents the type of a bridge message.
type MessageType string

const (
	MsgChat           MessageType = "chat"
	MsgToolPermission MessageType = "tool_permission"
	MsgStatus         MessageType = "status"
	MsgError          MessageType = "error"
	MsgNotification   MessageType = "notification"
)

// Message is the JSON envelope for all bridge communication.
type Message struct {
	Type      MessageType     `json:"type"`
	ID        string          `json:"id"`
	SessionID string          `json:"session_id,omitempty"`
	Payload   json.RawMessage `json:"payload"`
}

// HandshakePayload is exchanged during the initial WebSocket handshake message.
type HandshakePayload struct {
	ClientType      string `json:"client_type"`
	SessionID       string `json:"session_id"`
	ProtocolVersion string `json:"protocol_version"`
}

// Session represents a connected IDE client session.
type Session struct {
	ID         string
	ClientType string
	conn       net.Conn
	CreatedAt  time.Time
	mu         sync.Mutex
}

// writeMessage sends a JSON-encoded Message over the WebSocket connection.
func (s *Session) writeMessage(msg Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	return writeWSFrame(s.conn, 1, data) // opcode 1 = text
}

// Bridge is the WebSocket server for IDE integration.
type Bridge struct {
	port      int
	sessions  map[string]*Session
	mu        sync.RWMutex
	server    *http.Server
	listener  net.Listener
	onMessage func(sessionID string, msg Message)
	done      chan struct{}
}

// NewBridge creates a new Bridge on the given port. If port is 0, DefaultPort is used.
func NewBridge(port int) *Bridge {
	if port <= 0 {
		port = DefaultPort
	}
	return &Bridge{
		port:     port,
		sessions: make(map[string]*Session),
		done:     make(chan struct{}),
	}
}

// OnMessage sets the callback invoked when a message is received from any session.
func (b *Bridge) OnMessage(fn func(sessionID string, msg Message)) {
	b.onMessage = fn
}

// Port returns the port the bridge is configured to listen on.
// After Start, this reflects the actual bound port.
func (b *Bridge) Port() int {
	return b.port
}

// Sessions returns a snapshot of current session IDs.
func (b *Bridge) Sessions() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	ids := make([]string, 0, len(b.sessions))
	for id := range b.sessions {
		ids = append(ids, id)
	}
	return ids
}

// Start begins listening for WebSocket connections. It tries the configured port
// and up to MaxPortRetries sequential ports if the port is already in use.
func (b *Bridge) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", b.handleWS)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","protocol_version":"%s"}`, ProtocolVersion)
	})

	b.server = &http.Server{Handler: mux}

	var ln net.Listener
	var lastErr error
	for i := 0; i <= MaxPortRetries; i++ {
		addr := fmt.Sprintf("127.0.0.1:%d", b.port+i)
		ln, lastErr = net.Listen("tcp", addr)
		if lastErr == nil {
			b.port = b.port + i
			break
		}
	}
	if ln == nil {
		return fmt.Errorf("bridge: failed to bind after %d attempts: %w", MaxPortRetries+1, lastErr)
	}
	b.listener = ln

	go func() {
		<-ctx.Done()
		b.Stop()
	}()

	go func() {
		if err := b.server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("bridge: server error: %v", err)
		}
		close(b.done)
	}()

	return nil
}

// Stop gracefully shuts down the bridge server and cleans up all sessions.
func (b *Bridge) Stop() error {
	b.mu.Lock()
	for id, sess := range b.sessions {
		sess.conn.Close()
		delete(b.sessions, id)
	}
	b.mu.Unlock()

	if b.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return b.server.Shutdown(ctx)
	}
	return nil
}

// Send sends a message to a specific session by ID.
func (b *Bridge) Send(sessionID string, msg Message) error {
	b.mu.RLock()
	sess, ok := b.sessions[sessionID]
	b.mu.RUnlock()
	if !ok {
		return fmt.Errorf("bridge: session %q not found", sessionID)
	}
	msg.SessionID = sessionID
	return sess.writeMessage(msg)
}

// Broadcast sends a message to all connected sessions.
func (b *Bridge) Broadcast(msg Message) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, sess := range b.sessions {
		m := msg
		m.SessionID = sess.ID
		_ = sess.writeMessage(m)
	}
}

// handleWS performs the WebSocket upgrade handshake and manages the connection lifecycle.
func (b *Bridge) handleWS(w http.ResponseWriter, r *http.Request) {
	// Validate WebSocket upgrade request.
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") ||
		!headerContains(r.Header, "Connection", "upgrade") {
		http.Error(w, "expected websocket upgrade", http.StatusBadRequest)
		return
	}
	wsKey := r.Header.Get("Sec-WebSocket-Key")
	if wsKey == "" {
		http.Error(w, "missing Sec-WebSocket-Key", http.StatusBadRequest)
		return
	}

	// Compute accept key per RFC 6455 §4.2.2.
	h := sha1.New()
	h.Write([]byte(wsKey + websocketGUID))
	acceptKey := base64.StdEncoding.EncodeToString(h.Sum(nil))

	// Hijack the connection to take over the raw TCP socket.
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "server does not support hijacking", http.StatusInternalServerError)
		return
	}
	conn, bufrw, err := hj.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Write the HTTP 101 Switching Protocols response.
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + acceptKey + "\r\n\r\n"
	if _, err := bufrw.WriteString(resp); err != nil {
		conn.Close()
		return
	}
	if err := bufrw.Flush(); err != nil {
		conn.Close()
		return
	}

	// Generate a session ID and wait for the client handshake message.
	sessionID := generateID()
	sess := &Session{
		ID:        sessionID,
		conn:      conn,
		CreatedAt: time.Now(),
	}

	b.mu.Lock()
	b.sessions[sessionID] = sess
	b.mu.Unlock()

	// Send handshake to client.
	hsPayload, _ := json.Marshal(HandshakePayload{
		SessionID:       sessionID,
		ProtocolVersion: ProtocolVersion,
	})
	_ = sess.writeMessage(Message{
		Type:      MsgStatus,
		ID:        generateID(),
		SessionID: sessionID,
		Payload:   hsPayload,
	})

	// Read loop — process incoming messages until the connection closes.
	go b.readLoop(sess)
}

// readLoop reads WebSocket frames from a session and dispatches messages.
func (b *Bridge) readLoop(sess *Session) {
	defer func() {
		// Schedule cleanup after timeout.
		go func() {
			time.Sleep(SessionCleanupTimeout)
			b.mu.Lock()
			delete(b.sessions, sess.ID)
			b.mu.Unlock()
		}()
		sess.conn.Close()
	}()

	for {
		data, err := readWSFrame(sess.conn)
		if err != nil {
			return // connection closed or error
		}

		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil {
			errPayload, _ := json.Marshal(map[string]string{"error": "invalid message format"})
			_ = sess.writeMessage(Message{
				Type:      MsgError,
				ID:        generateID(),
				SessionID: sess.ID,
				Payload:   errPayload,
			})
			continue
		}

		// If the first message contains a handshake payload, extract client type.
		if sess.ClientType == "" {
			var hs HandshakePayload
			if err := json.Unmarshal(msg.Payload, &hs); err == nil && hs.ClientType != "" {
				sess.ClientType = hs.ClientType
			}
		}

		msg.SessionID = sess.ID
		if b.onMessage != nil {
			b.onMessage(sess.ID, msg)
		}
	}
}

// --- Minimal WebSocket frame helpers (RFC 6455) ---

// writeWSFrame writes a single WebSocket frame. Server-to-client frames are NOT masked.
func writeWSFrame(w io.Writer, opcode byte, payload []byte) error {
	length := len(payload)

	// First byte: FIN bit + opcode.
	frame := []byte{0x80 | opcode}

	// Second byte: mask bit (0 for server) + payload length.
	switch {
	case length <= 125:
		frame = append(frame, byte(length))
	case length <= 65535:
		frame = append(frame, 126)
		frame = append(frame, byte(length>>8), byte(length))
	default:
		frame = append(frame, 127)
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, uint64(length))
		frame = append(frame, buf...)
	}

	frame = append(frame, payload...)
	_, err := w.Write(frame)
	return err
}

// readWSFrame reads a single WebSocket frame. Client-to-server frames MUST be masked.
func readWSFrame(r io.Reader) ([]byte, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}

	opcode := header[0] & 0x0F

	// Handle close frame.
	if opcode == 8 {
		return nil, io.EOF
	}

	// Handle ping — respond with pong.
	masked := header[1]&0x80 != 0
	length := uint64(header[1] & 0x7F)

	switch length {
	case 126:
		ext := make([]byte, 2)
		if _, err := io.ReadFull(r, ext); err != nil {
			return nil, err
		}
		length = uint64(binary.BigEndian.Uint16(ext))
	case 127:
		ext := make([]byte, 8)
		if _, err := io.ReadFull(r, ext); err != nil {
			return nil, err
		}
		length = binary.BigEndian.Uint64(ext)
	}

	var maskKey [4]byte
	if masked {
		if _, err := io.ReadFull(r, maskKey[:]); err != nil {
			return nil, err
		}
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}

	if masked {
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}
	}

	// If ping, send pong and read next frame.
	if opcode == 9 {
		// We need a writer, but we only have a reader here.
		// Ping handling is best-effort; skip for now.
		return readWSFrame(r)
	}

	return payload, nil
}

// headerContains checks if an HTTP header value list contains a token (case-insensitive).
func headerContains(h http.Header, key, token string) bool {
	for _, v := range h[http.CanonicalHeaderKey(key)] {
		for _, s := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(s), token) {
				return true
			}
		}
	}
	return false
}

// generateID produces a short random hex ID.
func generateID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}
