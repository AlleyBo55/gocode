package tmux

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/AlleyBo55/gocode/internal/toolimpl"
)

// Manager tracks tmux sessions created during a conversation.
// All sessions are cleaned up when KillAll is called (typically at session end).
type Manager struct {
	mu       sync.Mutex
	sessions map[string]bool // session name -> alive

	once   sync.Once
	binPath string
	binErr  error
}

// NewManager creates a tmux session manager.
func NewManager() *Manager {
	return &Manager{
		sessions: make(map[string]bool),
	}
}

// ensureBinary resolves the tmux binary once.
func (m *Manager) ensureBinary() error {
	m.once.Do(func() {
		m.binPath, m.binErr = exec.LookPath("tmux")
		if m.binErr != nil {
			m.binErr = fmt.Errorf("tmux is not installed or not found on PATH — please install tmux to use terminal session tools (e.g. 'apt install tmux' or 'brew install tmux')")
		}
	})
	return m.binErr
}

// Create creates a named tmux session with an optional initial command.
func (m *Manager) Create(name string, initialCmd string) error {
	if err := m.ensureBinary(); err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("session name must not be empty")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.sessions[name] {
		return fmt.Errorf("tmux session %q already exists", name)
	}

	args := []string{"new-session", "-d", "-s", name}
	if initialCmd != "" {
		args = append(args, initialCmd)
	}

	cmd := exec.Command(m.binPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("creating tmux session %q: %s", name, strings.TrimSpace(stderr.String()))
	}

	m.sessions[name] = true
	return nil
}

// Send sends a command to a named session and captures output.
// It sends the command via send-keys, waits briefly for execution,
// then captures the pane content.
func (m *Manager) Send(name string, command string) (string, error) {
	if err := m.ensureBinary(); err != nil {
		return "", err
	}

	m.mu.Lock()
	alive := m.sessions[name]
	m.mu.Unlock()

	if !alive {
		return "", fmt.Errorf("tmux session %q does not exist or was not created by this manager", name)
	}

	// Send the command
	sendCmd := exec.Command(m.binPath, "send-keys", "-t", name, command, "Enter")
	var stderr bytes.Buffer
	sendCmd.Stderr = &stderr
	if err := sendCmd.Run(); err != nil {
		return "", fmt.Errorf("sending command to tmux session %q: %s", name, strings.TrimSpace(stderr.String()))
	}

	// Brief pause to let the command produce output
	time.Sleep(500 * time.Millisecond)

	// Capture pane content
	return m.Read(name)
}

// Read captures the current visible pane content of a named session.
func (m *Manager) Read(name string) (string, error) {
	if err := m.ensureBinary(); err != nil {
		return "", err
	}

	m.mu.Lock()
	alive := m.sessions[name]
	m.mu.Unlock()

	if !alive {
		return "", fmt.Errorf("tmux session %q does not exist or was not created by this manager", name)
	}

	cmd := exec.Command(m.binPath, "capture-pane", "-t", name, "-p")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("reading tmux session %q: %s", name, strings.TrimSpace(stderr.String()))
	}

	return stdout.String(), nil
}

// Kill terminates a named session.
func (m *Manager) Kill(name string) error {
	if err := m.ensureBinary(); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.sessions[name] {
		return fmt.Errorf("tmux session %q does not exist or was not created by this manager", name)
	}

	cmd := exec.Command(m.binPath, "kill-session", "-t", name)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Mark as dead regardless — the session may have already exited
		delete(m.sessions, name)
		return fmt.Errorf("killing tmux session %q: %s", name, strings.TrimSpace(stderr.String()))
	}

	delete(m.sessions, name)
	return nil
}

// KillAll terminates all sessions created by this manager.
// Errors are silently ignored — this is intended for cleanup.
func (m *Manager) KillAll() {
	if m.ensureBinary() != nil {
		return
	}

	m.mu.Lock()
	names := make([]string, 0, len(m.sessions))
	for name := range m.sessions {
		names = append(names, name)
	}
	m.mu.Unlock()

	for _, name := range names {
		_ = m.Kill(name)
	}
}

// ---------------------------------------------------------------------------
// Tool wrappers — each implements toolimpl.ToolExecutor
// ---------------------------------------------------------------------------

// TmuxCreateTool creates a new named tmux session.
// Params: name (string, required), command (string, optional initial command).
type TmuxCreateTool struct {
	Mgr *Manager
}

func (t *TmuxCreateTool) Execute(params map[string]interface{}) toolimpl.ToolResult {
	name, _ := params["name"].(string)
	if name == "" {
		return toolimpl.ToolResult{Success: false, Error: "missing required param: name"}
	}
	initialCmd, _ := params["command"].(string)

	if err := t.Mgr.Create(name, initialCmd); err != nil {
		return toolimpl.ToolResult{Success: false, Error: err.Error()}
	}
	msg := fmt.Sprintf("Created tmux session %q", name)
	if initialCmd != "" {
		msg += fmt.Sprintf(" with initial command: %s", initialCmd)
	}
	return toolimpl.ToolResult{Success: true, Output: msg}
}

// TmuxSendTool sends a command to a named tmux session and returns captured output.
// Params: name (string, required), command (string, required).
type TmuxSendTool struct {
	Mgr *Manager
}

func (t *TmuxSendTool) Execute(params map[string]interface{}) toolimpl.ToolResult {
	name, _ := params["name"].(string)
	if name == "" {
		return toolimpl.ToolResult{Success: false, Error: "missing required param: name"}
	}
	command, _ := params["command"].(string)
	if command == "" {
		return toolimpl.ToolResult{Success: false, Error: "missing required param: command"}
	}

	output, err := t.Mgr.Send(name, command)
	if err != nil {
		return toolimpl.ToolResult{Success: false, Error: err.Error()}
	}
	return toolimpl.ToolResult{Success: true, Output: output}
}

// TmuxReadTool captures the current visible pane content of a named session.
// Params: name (string, required).
type TmuxReadTool struct {
	Mgr *Manager
}

func (t *TmuxReadTool) Execute(params map[string]interface{}) toolimpl.ToolResult {
	name, _ := params["name"].(string)
	if name == "" {
		return toolimpl.ToolResult{Success: false, Error: "missing required param: name"}
	}

	output, err := t.Mgr.Read(name)
	if err != nil {
		return toolimpl.ToolResult{Success: false, Error: err.Error()}
	}
	return toolimpl.ToolResult{Success: true, Output: output}
}

// TmuxKillTool terminates a named tmux session.
// Params: name (string, required).
type TmuxKillTool struct {
	Mgr *Manager
}

func (t *TmuxKillTool) Execute(params map[string]interface{}) toolimpl.ToolResult {
	name, _ := params["name"].(string)
	if name == "" {
		return toolimpl.ToolResult{Success: false, Error: "missing required param: name"}
	}

	if err := t.Mgr.Kill(name); err != nil {
		return toolimpl.ToolResult{Success: false, Error: err.Error()}
	}
	return toolimpl.ToolResult{Success: true, Output: fmt.Sprintf("Killed tmux session %q", name)}
}

// ---------------------------------------------------------------------------
// Registration
// ---------------------------------------------------------------------------

// RegisterTmuxTools registers all four tmux tools in the given registry,
// each backed by the provided Manager instance.
func RegisterTmuxTools(r *toolimpl.Registry, mgr *Manager) {
	r.Set("tmux_create", &TmuxCreateTool{Mgr: mgr})
	r.Set("tmux_send", &TmuxSendTool{Mgr: mgr})
	r.Set("tmux_read", &TmuxReadTool{Mgr: mgr})
	r.Set("tmux_kill", &TmuxKillTool{Mgr: mgr})
}
