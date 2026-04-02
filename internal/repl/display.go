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
				fmt.Fprint(d.w, ev.BlockDelta.Thinking)
			}
		}
	case "message_stop":
		fmt.Fprintln(d.w)
	case "content_block_start":
		if ev.ContentBlock != nil {
			switch ev.ContentBlock.Kind {
			case "tool_use":
				fmt.Fprintf(d.w, "\n%s⚡ %s%s\n", cBlue, ev.ContentBlock.Name, ansiReset)
			case "thinking":
				fmt.Fprintf(d.w, "%s💭 ", cGray+ansiDim)
			}
		}
	case "content_block_stop":
		fmt.Fprint(d.w, ansiReset)
	}
}

// RenderResponse renders a full message response with markdown formatting.
func (d *Display) RenderResponse(resp *apitypes.MessageResponse) {
	for _, block := range resp.Content {
		switch block.Kind {
		case "text":
			rendered := RenderMarkdown(block.Text)
			fmt.Fprint(d.w, rendered)
		case "tool_use":
			inputStr := "{}"
			if len(block.Input) > 0 {
				inputStr = string(block.Input)
			}
			fmt.Fprintf(d.w, "\n%s⚡ %s%s(%s)\n", cBlue, block.Name, ansiReset, cGray+summarizeJSON(inputStr)+ansiReset)
		case "thinking":
			fmt.Fprintf(d.w, "\n%s💭 %s%s\n", cGray+ansiDim, block.Thinking, ansiReset)
		}
	}
}

// Error displays an error message.
func (d *Display) Error(err error) {
	fmt.Fprintf(d.w, "%sError: %v%s\n", cRed, err, ansiReset)
}

// Usage displays token usage.
func (d *Display) Usage(usage string) {
	fmt.Fprintf(d.w, "%s%s%s\n", cCyan, usage, ansiReset)
}

// PermissionPrompt displays a permission request.
func (d *Display) PermissionPrompt(toolName, operation string) {
	fmt.Fprintf(d.w, "%s🔒 %s%s wants to run: %s%s%s\n", cYellow, cWhite+ansiBold, toolName, ansiReset+cGray, operation, ansiReset)
	fmt.Fprintf(d.w, "   %sAllow? [y/N]:%s ", cYellow, ansiReset)
}

// PermissionPromptExtended displays a permission request with trust options.
func (d *Display) PermissionPromptExtended(toolName, operation string) {
	fmt.Fprintf(d.w, "%s🔒 %s%s%s wants to run: %s%s%s\n", cYellow, cWhite+ansiBold, toolName, ansiReset, cGray, operation, ansiReset)
	fmt.Fprintf(d.w, "   %s[y]%s yes  %s[n]%s no  %s[a]%s always trust %s  %s[t]%s trust command%s: ", cGreen, ansiReset, cRed, ansiReset, cCyan, ansiReset, toolName, cCyan, ansiReset, ansiReset)
}
