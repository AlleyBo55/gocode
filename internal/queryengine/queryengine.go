package queryengine

import (
	"fmt"
	"strings"

	"github.com/gocode/gocode/internal/manifest"
	"github.com/gocode/gocode/internal/models"
	"github.com/gocode/gocode/internal/session"
	"github.com/gocode/gocode/internal/transcript"
	"github.com/google/uuid"
)

// ErrBudgetExceeded is returned when the cumulative token usage exceeds the budget.
var ErrBudgetExceeded = fmt.Errorf("budget exceeded")

// QueryEngineConfig holds engine configuration.
type QueryEngineConfig struct {
	MaxTurns          int  `json:"max_turns"`
	MaxBudgetTokens   int  `json:"max_budget_tokens"`
	CompactAfterTurns int  `json:"compact_after_turns"`
	StructuredOutput  bool `json:"structured_output"`
}

// NewDefaultConfig returns a QueryEngineConfig with default values.
func NewDefaultConfig() QueryEngineConfig {
	return QueryEngineConfig{
		MaxTurns:          8,
		MaxBudgetTokens:   2000,
		CompactAfterTurns: 12,
		StructuredOutput:  false,
	}
}

// TurnResult holds the output of a single turn.
type TurnResult struct {
	Prompt            string                  `json:"prompt"`
	Output            string                  `json:"output"`
	MatchedCommands   []string                `json:"matched_commands"`
	MatchedTools      []string                `json:"matched_tools"`
	PermissionDenials []models.PermissionDenial `json:"permission_denials"`
	Usage             models.UsageSummary     `json:"usage"`
	StopReason        string                  `json:"stop_reason"`
}

// StreamEvent represents a single streaming event.
type StreamEvent struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data"`
}

// QueryEnginePort is the core query processing component.
type QueryEnginePort struct {
	Config            QueryEngineConfig
	SessionID         string
	Messages          []string
	PermissionDenials []models.PermissionDenial
	TotalUsage        models.UsageSummary
	Transcript        *transcript.TranscriptStore
	SessionStore      *session.SessionStore
	Manifest          *manifest.PortManifest
	turnCount         int
	compacted         bool
}

// FromWorkspace creates a new QueryEnginePort for a fresh workspace session.
func FromWorkspace(config QueryEngineConfig, sessionStore *session.SessionStore) *QueryEnginePort {
	return &QueryEnginePort{
		Config:            config,
		SessionID:         uuid.New().String(),
		Messages:          []string{},
		PermissionDenials: []models.PermissionDenial{},
		TotalUsage:        models.UsageSummary{},
		Transcript:        transcript.NewTranscriptStore(),
		SessionStore:      sessionStore,
	}
}

// FromSavedSession restores a QueryEnginePort from a previously saved session.
func FromSavedSession(sessionID string, config QueryEngineConfig, store *session.SessionStore) (*QueryEnginePort, error) {
	stored, err := store.Load(sessionID)
	if err != nil {
		return nil, fmt.Errorf("restoring session %s: %w", sessionID, err)
	}

	messages := make([]string, len(stored.Messages))
	for i, m := range stored.Messages {
		messages[i] = m.Content
	}

	return &QueryEnginePort{
		Config:            config,
		SessionID:         sessionID,
		Messages:          messages,
		PermissionDenials: []models.PermissionDenial{},
		TotalUsage: models.UsageSummary{
			InputTokens:  stored.InputTokens,
			OutputTokens: stored.OutputTokens,
		},
		Transcript:   transcript.NewTranscriptStore(),
		SessionStore: store,
		turnCount:    len(stored.Messages) / 2, // approximate turns from message pairs
	}, nil
}

// SubmitMessage processes a prompt and returns a TurnResult.
// It checks max turns, tracks usage, checks budget, and compacts if needed.
// StopReason is one of: "completed", "max_turns_reached", "max_budget_reached".
func (qe *QueryEnginePort) SubmitMessage(
	prompt string,
	matchedCommands, matchedTools []string,
	deniedTools []models.PermissionDenial,
) TurnResult {
	// Check max turns
	if qe.Config.MaxTurns > 0 && qe.turnCount >= qe.Config.MaxTurns {
		return TurnResult{
			Prompt:            prompt,
			Output:            "Maximum turns reached.",
			MatchedCommands:   matchedCommands,
			MatchedTools:      matchedTools,
			PermissionDenials: deniedTools,
			Usage:             qe.TotalUsage,
			StopReason:        "max_turns_reached",
		}
	}

	// Format output from prompt and matched items
	output := formatOutput(prompt, matchedCommands, matchedTools)

	// Track usage
	qe.TotalUsage.AddTurn(prompt, output)
	qe.turnCount++

	// Record message and transcript
	qe.Messages = append(qe.Messages, prompt, output)
	qe.Transcript.Append(fmt.Sprintf("user: %s", prompt))
	qe.Transcript.Append(fmt.Sprintf("assistant: %s", output))

	// Track permission denials
	qe.PermissionDenials = append(qe.PermissionDenials, deniedTools...)

	// Check budget
	totalTokens := qe.TotalUsage.InputTokens + qe.TotalUsage.OutputTokens
	if qe.Config.MaxBudgetTokens > 0 && totalTokens >= qe.Config.MaxBudgetTokens {
		return TurnResult{
			Prompt:            prompt,
			Output:            output,
			MatchedCommands:   matchedCommands,
			MatchedTools:      matchedTools,
			PermissionDenials: deniedTools,
			Usage:             qe.TotalUsage,
			StopReason:        "max_budget_reached",
		}
	}

	// Compact if needed
	qe.CompactMessagesIfNeeded()

	return TurnResult{
		Prompt:            prompt,
		Output:            output,
		MatchedCommands:   matchedCommands,
		MatchedTools:      matchedTools,
		PermissionDenials: deniedTools,
		Usage:             qe.TotalUsage,
		StopReason:        "completed",
	}
}

// formatOutput builds a response string from the prompt and matched items.
func formatOutput(prompt string, matchedCommands, matchedTools []string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Processed: %s", prompt))
	if len(matchedCommands) > 0 {
		sb.WriteString(fmt.Sprintf(" | Commands: %s", strings.Join(matchedCommands, ", ")))
	}
	if len(matchedTools) > 0 {
		sb.WriteString(fmt.Sprintf(" | Tools: %s", strings.Join(matchedTools, ", ")))
	}
	return sb.String()
}

// StreamSubmitMessage processes a prompt and yields streaming events via a channel.
// Events: message_start, command_match, tool_match, permission_denial, message_delta, message_stop.
func (qe *QueryEnginePort) StreamSubmitMessage(
	prompt string,
	matchedCommands, matchedTools []string,
	deniedTools []models.PermissionDenial,
) <-chan StreamEvent {
	ch := make(chan StreamEvent)

	go func() {
		defer close(ch)

		// message_start
		ch <- StreamEvent{
			Type: "message_start",
			Data: map[string]interface{}{
				"prompt":     prompt,
				"session_id": qe.SessionID,
			},
		}

		// command_match events
		for _, cmd := range matchedCommands {
			ch <- StreamEvent{
				Type: "command_match",
				Data: map[string]interface{}{"command": cmd},
			}
		}

		// tool_match events
		for _, tool := range matchedTools {
			ch <- StreamEvent{
				Type: "tool_match",
				Data: map[string]interface{}{"tool": tool},
			}
		}

		// permission_denial events
		for _, denial := range deniedTools {
			ch <- StreamEvent{
				Type: "permission_denial",
				Data: map[string]interface{}{
					"tool_name": denial.ToolName,
					"reason":    denial.Reason,
				},
			}
		}

		// Process the turn via SubmitMessage
		result := qe.SubmitMessage(prompt, matchedCommands, matchedTools, deniedTools)

		// message_delta with the output
		ch <- StreamEvent{
			Type: "message_delta",
			Data: map[string]interface{}{"content": result.Output},
		}

		// message_stop
		ch <- StreamEvent{
			Type: "message_stop",
			Data: map[string]interface{}{
				"stop_reason": result.StopReason,
				"usage": map[string]interface{}{
					"input_tokens":  result.Usage.InputTokens,
					"output_tokens": result.Usage.OutputTokens,
				},
			},
		}
	}()

	return ch
}

// CompactMessagesIfNeeded compacts messages and transcript if the turn count
// exceeds the CompactAfterTurns threshold.
func (qe *QueryEnginePort) CompactMessagesIfNeeded() {
	if qe.Config.CompactAfterTurns > 0 && qe.turnCount > qe.Config.CompactAfterTurns {
		// Keep only the last CompactAfterTurns*2 messages (2 messages per turn: prompt+output)
		keep := qe.Config.CompactAfterTurns * 2
		if len(qe.Messages) > keep {
			qe.Messages = qe.Messages[len(qe.Messages)-keep:]
		}
		qe.Transcript.Compact()
		qe.compacted = true
	}
}

// PersistSession saves the current session state to the session store.
func (qe *QueryEnginePort) PersistSession() (string, error) {
	if qe.SessionStore == nil {
		return "", fmt.Errorf("no session store configured")
	}

	messages := make([]session.Message, 0, len(qe.Messages))
	for i, msg := range qe.Messages {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		messages = append(messages, session.Message{
			Role:    role,
			Content: msg,
		})
	}

	stored := session.StoredSession{
		SessionID:    qe.SessionID,
		Messages:     messages,
		InputTokens:  qe.TotalUsage.InputTokens,
		OutputTokens: qe.TotalUsage.OutputTokens,
	}

	return qe.SessionStore.Save(stored)
}

// FlushTranscript flushes the transcript store.
func (qe *QueryEnginePort) FlushTranscript() {
	if qe.Transcript != nil {
		qe.Transcript.Flush()
	}
}

// RenderSummary returns a Markdown-formatted summary of the engine state.
func (qe *QueryEnginePort) RenderSummary() string {
	var sb strings.Builder
	sb.WriteString("# Query Engine Summary\n\n")
	sb.WriteString(fmt.Sprintf("Session: `%s`\n", qe.SessionID))
	sb.WriteString(fmt.Sprintf("Turns: %d / %d\n", qe.turnCount, qe.Config.MaxTurns))
	sb.WriteString(fmt.Sprintf("Input tokens: %d\n", qe.TotalUsage.InputTokens))
	sb.WriteString(fmt.Sprintf("Output tokens: %d\n", qe.TotalUsage.OutputTokens))

	totalTokens := qe.TotalUsage.InputTokens + qe.TotalUsage.OutputTokens
	sb.WriteString(fmt.Sprintf("Total tokens: %d / %d\n", totalTokens, qe.Config.MaxBudgetTokens))

	if len(qe.PermissionDenials) > 0 {
		sb.WriteString(fmt.Sprintf("Permission denials: %d\n", len(qe.PermissionDenials)))
	}
	if qe.compacted {
		sb.WriteString("Messages compacted: yes\n")
	}
	if qe.Manifest != nil {
		sb.WriteString(fmt.Sprintf("Manifest: %s (%d Go files)\n", qe.Manifest.SrcRoot, qe.Manifest.TotalGoFiles))
	}
	return sb.String()
}
