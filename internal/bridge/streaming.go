package bridge

import (
	"encoding/json"
	"fmt"
)

// chatPayload is the JSON payload for a chat streaming message.
type chatPayload struct {
	Text string `json:"text"`
}

// notificationPayload is the JSON payload for a notification message.
type notificationPayload struct {
	Message string `json:"message"`
}

// StreamToBridge sends agent response text to the IDE session in real-time.
// It wraps the text as a MsgChat message with the text as payload.
func StreamToBridge(b *Bridge, sessionID string, text string) error {
	payload, err := json.Marshal(chatPayload{Text: text})
	if err != nil {
		return fmt.Errorf("bridge stream: marshal payload: %w", err)
	}
	return b.Send(sessionID, Message{
		Type:    MsgChat,
		ID:      generateID(),
		Payload: payload,
	})
}

// NotifyToBridge sends a notification message to the IDE session.
func NotifyToBridge(b *Bridge, sessionID string, notification string) error {
	payload, err := json.Marshal(notificationPayload{Message: notification})
	if err != nil {
		return fmt.Errorf("bridge notify: marshal payload: %w", err)
	}
	return b.Send(sessionID, Message{
		Type:    MsgNotification,
		ID:      generateID(),
		Payload: payload,
	})
}
