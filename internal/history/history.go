package history

import "fmt"

// HistoryEvent represents a single session history event.
type HistoryEvent struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

// HistoryLog is an ordered collection of history events.
type HistoryLog struct {
	Events []HistoryEvent `json:"events"`
}

// Append adds a new event to the history log.
func (h *HistoryLog) Append(title, detail string) {
	h.Events = append(h.Events, HistoryEvent{Title: title, Detail: detail})
}

// Render returns a Markdown-formatted timeline of all events.
func (h *HistoryLog) Render() string {
	out := "# Session History\n\n"
	for _, e := range h.Events {
		out += fmt.Sprintf("- %s: %s\n", e.Title, e.Detail)
	}
	return out
}
