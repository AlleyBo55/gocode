package apiclient

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/AlleyBo55/gocode/internal/apitypes"
)

// SseFrame represents a parsed SSE frame with event type and data payload.
type SseFrame struct {
	Event string
	Data  string
}

// SseParser is a stateful SSE frame parser  SseParser.
type SseParser struct {
	buffer []byte
}

// NewSseParser creates a new SSE parser.
func NewSseParser() *SseParser {
	return &SseParser{}
}

// Push appends raw bytes to the buffer and returns any complete StreamEvents.
func (p *SseParser) Push(data []byte) ([]apitypes.StreamEvent, error) {
	p.buffer = append(p.buffer, data...)
	var events []apitypes.StreamEvent
	for {
		frame := p.nextFrame()
		if frame == nil {
			break
		}
		event, err := parseFrame(frame)
		if err != nil {
			return events, err
		}
		if event != nil {
			events = append(events, *event)
		}
	}
	return events, nil
}

// Finish processes any remaining data in the buffer.
func (p *SseParser) Finish() ([]apitypes.StreamEvent, error) {
	if len(p.buffer) == 0 {
		return nil, nil
	}
	trailing := string(p.buffer)
	p.buffer = nil
	event, err := parseFrame(&trailing)
	if err != nil {
		return nil, err
	}
	if event != nil {
		return []apitypes.StreamEvent{*event}, nil
	}
	return nil, nil
}

// nextFrame extracts the next complete SSE frame from the buffer.
func (p *SseParser) nextFrame() *string {
	// Look for \n\n separator
	if idx := bytes.Index(p.buffer, []byte("\n\n")); idx >= 0 {
		frame := string(p.buffer[:idx])
		p.buffer = p.buffer[idx+2:]
		return &frame
	}
	// Look for \r\n\r\n separator
	if idx := bytes.Index(p.buffer, []byte("\r\n\r\n")); idx >= 0 {
		frame := string(p.buffer[:idx])
		p.buffer = p.buffer[idx+4:]
		return &frame
	}
	return nil
}

// parseFrame parses a single SSE frame string into a StreamEvent.
// Returns nil for ping events, [DONE] markers, and comment-only frames.
func parseFrame(frame *string) (*apitypes.StreamEvent, error) {
	trimmed := strings.TrimSpace(*frame)
	if trimmed == "" {
		return nil, nil
	}

	var dataLines []string
	var eventName string

	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimRight(line, "\r")
		// Comment lines
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data:"))
		}
	}

	// Discard ping events
	if eventName == "ping" {
		return nil, nil
	}

	if len(dataLines) == 0 {
		return nil, nil
	}

	// Trim leading space from each data line (SSE spec: single space after "data:")
	for i, dl := range dataLines {
		if len(dl) > 0 && dl[0] == ' ' {
			dataLines[i] = dl[1:]
		}
	}

	payload := strings.Join(dataLines, "\n")
	if payload == "[DONE]" {
		return nil, nil
	}

	var event apitypes.StreamEvent
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return nil, &apitypes.ApiError{Kind: apitypes.ErrInvalidSseFrame, Message: err.Error()}
	}
	return &event, nil
}
