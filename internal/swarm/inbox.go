package swarm

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/AlleyBo55/gocode/internal/agent"
	"github.com/AlleyBo55/gocode/internal/apitypes"
)

// MainAgent is the name the interactive session registers under. Sub-agents
// report to it and can message it by this name.
const MainAgent = "main"

// Drain returns and removes every pending message for an agent, oldest first.
func (s *SwarmManager) Drain(name string) []SwarmMessage {
	var out []SwarmMessage
	for {
		msg, ok := s.ReceiveMessage(name)
		if !ok {
			return out
		}
		out = append(out, msg)
	}
}

// FormatInbox renders messages the way they are shown to a model: one entry
// per message, sender named, so the model can reply to the right agent.
func FormatInbox(msgs []SwarmMessage) []string {
	out := make([]string, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, fmt.Sprintf("[message from %s]\n%s", m.From, m.Content))
	}
	return out
}

// Tool names offered to every agent in the swarm.
const (
	ToolSendMessage = "send_agent_message"
	ToolListAgents  = "list_agents"
)

// ToolDefs are the swarm tools as the model sees them. The description names
// the main agent so a sub-agent knows where to report.
func ToolDefs() []apitypes.ToolDef {
	return []apitypes.ToolDef{
		{
			Name: ToolSendMessage,
			Description: "Send a message to another agent in the swarm. Use list_agents to see who is running. " +
				"The interactive session is \"" + MainAgent + "\". The recipient sees the message at the start of its next turn.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"to":{"type":"string","description":"Name of the target agent"},"message":{"type":"string","description":"The message"}},"required":["to","message"]}`),
		},
		{
			Name:        ToolListAgents,
			Description: "List the agents currently in the swarm with their status and category.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		},
	}
}

// IsSwarmTool reports whether name is one of the swarm tools.
func IsSwarmTool(name string) bool {
	switch strings.ToLower(name) {
	case ToolSendMessage, ToolListAgents:
		return true
	}
	return false
}

// ExecuteTool runs a swarm tool on behalf of the named agent. ok is false
// when name is not a swarm tool.
func (s *SwarmManager) ExecuteTool(from, name string, input map[string]interface{}) (result apitypes.ToolResult, ok bool) {
	switch strings.ToLower(name) {
	case ToolSendMessage:
		to, _ := input["to"].(string)
		message, _ := input["message"].(string)
		if to == "" || message == "" {
			return apitypes.ToolResult{Output: "both \"to\" and \"message\" are required", IsError: true}, true
		}
		if err := s.SendMessage(from, to, message); err != nil {
			return apitypes.ToolResult{Output: err.Error() + "; use list_agents to see who is running", IsError: true}, true
		}
		return apitypes.ToolResult{Output: fmt.Sprintf("delivered to %s; it will read the message at the start of its next turn", to)}, true
	case ToolListAgents:
		agents := s.ListAgents()
		sort.Slice(agents, func(i, j int) bool { return agents[i].Name < agents[j].Name })
		if len(agents) == 0 {
			return apitypes.ToolResult{Output: "no agents registered"}, true
		}
		var b strings.Builder
		for _, a := range agents {
			fmt.Fprintf(&b, "%s\t%s", a.Name, a.Status)
			if a.Category != "" {
				fmt.Fprintf(&b, "\t%s", a.Category)
			}
			if a.Name == from {
				b.WriteString("\t(you)")
			}
			b.WriteString("\n")
		}
		return apitypes.ToolResult{Output: b.String()}, true
	}
	return apitypes.ToolResult{}, false
}

// WrapExecutor adds the swarm tools to an agent's tool set, bound to the
// agent's own name so messages carry the right sender. Swarm tool calls are
// answered here; everything else passes through.
func WrapExecutor(inner agent.ToolExecutor, s *SwarmManager, self string) agent.ToolExecutor {
	return &swarmExecutor{inner: inner, swarm: s, self: self}
}

// InboxFor returns the Inbox function for a runtime: it drains and formats
// the named agent's messages.
func (s *SwarmManager) InboxFor(name string) func() []string {
	return func() []string { return FormatInbox(s.Drain(name)) }
}

type swarmExecutor struct {
	inner agent.ToolExecutor
	swarm *SwarmManager
	self  string
}

func (e *swarmExecutor) Execute(name string, input map[string]interface{}) apitypes.ToolResult {
	if res, ok := e.swarm.ExecuteTool(e.self, name, input); ok {
		return res
	}
	return e.inner.Execute(name, input)
}

func (e *swarmExecutor) ListTools() []apitypes.ToolDef {
	return append(e.inner.ListTools(), ToolDefs()...)
}
