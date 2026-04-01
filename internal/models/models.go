package models

import (
"fmt"
"strings"
)

// Subsystem represents a logical subsystem in the porting project.
type Subsystem struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	FileCount int    `json:"file_count"`
	Notes     string `json:"notes"`
}

// PortingModule represents a single module to be ported.
type PortingModule struct {
	Name           string `json:"name"`
	Responsibility string `json:"responsibility"`
	SourceHint     string `json:"source_hint"`
	Status         string `json:"status"`
}

// PermissionDenial records a tool access denial.
type PermissionDenial struct {
	ToolName string `json:"tool_name"`
	Reason   string `json:"reason"`
}

// UsageSummary tracks cumulative token usage across turns.
type UsageSummary struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// AddTurn adds word counts from prompt and output strings.
// Words are counted by splitting on whitespace, matching Python's str.split() behavior.
func (u *UsageSummary) AddTurn(prompt, output string) {
u.InputTokens += len(strings.Fields(prompt))
u.OutputTokens += len(strings.Fields(output))
}

// PortingBacklog represents the backlog of modules to port.
type PortingBacklog struct {
Title   string          `json:"title"`
Modules []PortingModule `json:"modules"`
}

// SummaryLines returns a formatted string slice summarizing each module.
func (b *PortingBacklog) SummaryLines() []string {
lines := make([]string, 0, len(b.Modules))
for _, m := range b.Modules {
lines = append(lines, fmt.Sprintf("- %s [%s] — %s (from %s)", m.Name, m.Status, m.Responsibility, m.SourceHint))
}
return lines
}
