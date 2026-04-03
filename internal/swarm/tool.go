package swarm

import (
	"encoding/json"
	"fmt"

	"github.com/AlleyBo55/gocode/internal/models"
	"github.com/AlleyBo55/gocode/internal/toolimpl"
)

// SendMessageTool implements toolimpl.ToolExecutor for inter-agent messaging.
type SendMessageTool struct {
	Swarm    *SwarmManager
	FromName string // name of the agent that owns this tool instance
}

// ToolDef returns the tool definition for send_agent_message.
func (t *SendMessageTool) ToolDef() models.ToolDefinition {
	return models.ToolDefinition{
		Name:        "send_agent_message",
		Description: "Send a message to another agent in the swarm by name.",
		InputSchema: models.InputSchema{
			Type: "object",
			Properties: map[string]models.SchemaProperty{
				"to": {
					Type:        "string",
					Description: "Name of the target agent to send the message to.",
				},
				"message": {
					Type:        "string",
					Description: "The message content to deliver.",
				},
			},
			Required: []string{"to", "message"},
		},
	}
}

// Execute sends a message to the target agent via the SwarmManager.
func (t *SendMessageTool) Execute(params map[string]interface{}) toolimpl.ToolResult {
	to, _ := params["to"].(string)
	if to == "" {
		return toolimpl.ToolResult{Success: false, Error: "missing required param: to"}
	}
	message, _ := params["message"].(string)
	if message == "" {
		return toolimpl.ToolResult{Success: false, Error: "missing required param: message"}
	}

	if err := t.Swarm.SendMessage(t.FromName, to, message); err != nil {
		return toolimpl.ToolResult{Success: false, Error: fmt.Sprintf("failed to send message: %v", err)}
	}

	data, _ := json.Marshal(map[string]string{
		"status": "delivered",
		"from":   t.FromName,
		"to":     to,
	})
	return toolimpl.ToolResult{Success: true, Output: string(data)}
}

// RegisterSwarmTool registers the send_agent_message tool in the given registry.
func RegisterSwarmTool(registry *toolimpl.Registry, swarm *SwarmManager, agentName string) {
	registry.Set("send_agent_message", &SendMessageTool{Swarm: swarm, FromName: agentName})
}
