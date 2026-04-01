package repl

import (
	"fmt"
	"io"

	"github.com/AlleyBo55/gocode/internal/apitypes"
)

// Display handles rendering of streaming events and tool status to the terminal.
type Display struct {
	w io.Writer
}

// NewDisplay creates a new Display writing to w.
func NewDisplay(w io.Writer) *Display {
	return &Display{w: w}
}

// StreamEvent renders a single stream event incrementally.
func (d *Display) StreamEvent(ev apitypes.StreamEvent) {
	switch ev.Kind {
	case "content_block_delta":
		if ev.BlockDelta != nil {
			switch ev.BlockDelta.Kind {
			case "text_delta":
				fmt.Fprint(d.w, ev.BlockDelta.Text)
			case "thinking_delta":
				// Could render thinking indicator
			}
		}
	case "message_stop":
		fmt.Fprintln(d.w)
	case "content_block_start":
		if ev.ContentBlock != nil && ev.ContentBlock.Kind == "tool_use" {
			fmt.Fprintf(d.w, "\n⚡ Tool: %s\n", ev.ContentBlock.Name)
		}
	}
}

// ToolStart displays tool invocation info.
func (d *Display) ToolStart(name string, inputSummary string) {
	fmt.Fprintf(d.w, "  → Running %s", name)
	if inputSummary != "" {
		fmt.Fprintf(d.w, " (%s)", inputSummary)
	}
	fmt.Fprintln(d.w)
}

// ToolDone displays tool completion status.
func (d *Display) ToolDone(name string, isError bool) {
	if isError {
		fmt.Fprintf(d.w, "  ✗ %s failed\n", name)
	} else {
		fmt.Fprintf(d.w, "  ✓ %s done\n", name)
	}
}

// Error displays an error message.
func (d *Display) Error(err error) {
	fmt.Fprintf(d.w, "Error: %v\n", err)
}

// Usage displays token usage.
func (d *Display) Usage(usage string) {
	fmt.Fprintln(d.w, usage)
}

// PermissionPrompt displays a permission request and reads the response.
func (d *Display) PermissionPrompt(toolName, operation string) {
	fmt.Fprintf(d.w, "🔒 Tool %s wants to: %s\n", toolName, operation)
	fmt.Fprint(d.w, "   Allow? [y/N]: ")
}
