package swarm

import (
	"fmt"
	"sync"
	"time"
)

// AgentStatus represents the current state of a swarm agent.
type AgentStatus string

const (
	StatusRunning   AgentStatus = "running"
	StatusIdle      AgentStatus = "idle"
	StatusCompleted AgentStatus = "completed"
)

// AgentInfo describes a registered agent in the swarm.
type AgentInfo struct {
	Name         string
	Status       AgentStatus
	Capabilities []string
	Category     string
}

// SwarmMessage is a message sent between agents.
type SwarmMessage struct {
	From    string
	To      string
	Content string
	Time    time.Time
}

// mailboxSize is the default buffer size for each agent's mailbox.
const mailboxSize = 64

// Mailbox is a buffered message queue for an agent.
type Mailbox struct {
	messages chan SwarmMessage
}

// NewMailbox creates a mailbox with a buffered channel.
func NewMailbox() *Mailbox {
	return &Mailbox{messages: make(chan SwarmMessage, mailboxSize)}
}

// Send enqueues a message. Returns false if the mailbox is full.
func (m *Mailbox) Send(msg SwarmMessage) bool {
	select {
	case m.messages <- msg:
		return true
	default:
		return false
	}
}

// Receive returns the next message without blocking.
// The second return value is false when no message is available.
func (m *Mailbox) Receive() (SwarmMessage, bool) {
	select {
	case msg := <-m.messages:
		return msg, true
	default:
		return SwarmMessage{}, false
	}
}

// SwarmManager coordinates agent-to-agent messaging and discovery.
type SwarmManager struct {
	mu        sync.RWMutex
	agents    map[string]*AgentInfo
	mailboxes map[string]*Mailbox
	maxAgents int
}

// NewSwarmManager creates a SwarmManager with the given concurrency limit.
// If maxAgents <= 0 it defaults to 10.
func NewSwarmManager(maxAgents int) *SwarmManager {
	if maxAgents <= 0 {
		maxAgents = 10
	}
	return &SwarmManager{
		agents:    make(map[string]*AgentInfo),
		mailboxes: make(map[string]*Mailbox),
		maxAgents: maxAgents,
	}
}

// Register adds an agent to the swarm. Returns an error if the swarm is
// full or the name is already taken.
func (s *SwarmManager) Register(name string, capabilities []string, category string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.agents[name]; exists {
		return fmt.Errorf("agent %q is already registered", name)
	}
	if len(s.agents) >= s.maxAgents {
		return fmt.Errorf("swarm is full (%d/%d agents)", len(s.agents), s.maxAgents)
	}

	s.agents[name] = &AgentInfo{
		Name:         name,
		Status:       StatusRunning,
		Capabilities: capabilities,
		Category:     category,
	}
	s.mailboxes[name] = NewMailbox()
	return nil
}

// Unregister removes an agent and its mailbox from the swarm.
func (s *SwarmManager) Unregister(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.agents, name)
	delete(s.mailboxes, name)
}

// UpdateStatus changes the status of a registered agent.
// No-op if the agent is not found.
func (s *SwarmManager) UpdateStatus(name string, status AgentStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if info, ok := s.agents[name]; ok {
		info.Status = status
	}
}

// SendMessage delivers a message from one agent to another.
// Returns an error if the target agent does not exist or the mailbox is full.
func (s *SwarmManager) SendMessage(from, to, content string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	mb, ok := s.mailboxes[to]
	if !ok {
		return fmt.Errorf("target agent %q not found", to)
	}

	msg := SwarmMessage{
		From:    from,
		To:      to,
		Content: content,
		Time:    time.Now(),
	}
	if !mb.Send(msg) {
		return fmt.Errorf("mailbox for agent %q is full", to)
	}
	return nil
}

// ReceiveMessage returns the next message for the named agent without blocking.
// Returns false when no message is available or the agent is not registered.
func (s *SwarmManager) ReceiveMessage(name string) (SwarmMessage, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	mb, ok := s.mailboxes[name]
	if !ok {
		return SwarmMessage{}, false
	}
	return mb.Receive()
}

// ListAgents returns a snapshot of all registered agents.
func (s *SwarmManager) ListAgents() []AgentInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	agents := make([]AgentInfo, 0, len(s.agents))
	for _, info := range s.agents {
		agents = append(agents, *info)
	}
	return agents
}
