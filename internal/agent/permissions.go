package agent

import (
	"strings"
	"sync/atomic"
)

// PermissionMode governs what operations are allowed.
type PermissionMode int

const (
	// WorkspaceWrite allows read and write within the workspace.
	WorkspaceWrite PermissionMode = iota
	// DangerFullAccess allows all operations without prompting.
	DangerFullAccess
)

// PermissionPrompter asks the user for permission decisions.
type PermissionPrompter interface {
	Prompt(toolName string, operation string) (bool, error)
}

// PermissionPolicy determines whether a tool invocation is allowed.
type PermissionPolicy struct {
	Mode     PermissionMode
	Prompter PermissionPrompter
	Trusted  *TrustedToolStore
	// planMode blocks every tool outside planModeTools, whatever Mode says.
	planMode atomic.Bool
}

// planModeTools are the built-in tools that only read. Everything else is
// blocked in plan mode: bash, file edits, MCP tools, sub-agents, tmux, and
// ast_grep (it rewrites when given a replacement). An allowlist means a tool
// added later stays blocked until someone decides it is read-only.
var planModeTools = map[string]bool{
	"filereadtool":        true,
	"globtool":            true,
	"greptool":            true,
	"listdirectorytool":   true,
	"webfetchtool":        true,
	"websearchtool":       true,
	"lsp_diagnostics":     true,
	"lsp_find_references": true,
	"lsp_goto_definition": true,
}

// SetPlanMode turns read-only plan mode on or off. Safe to call while a turn runs.
func (p *PermissionPolicy) SetPlanMode(on bool) { p.planMode.Store(on) }

// PlanMode reports whether read-only plan mode is on.
func (p *PermissionPolicy) PlanMode() bool { return p.planMode.Load() }

// Authorize checks if a tool invocation is allowed.
// Returns true if allowed, false if denied, and an optional denial reason.
func (p *PermissionPolicy) Authorize(toolName string, input string) (bool, string) {
	if p.planMode.Load() && !planModeTools[strings.ToLower(toolName)] {
		return false, "plan mode is read-only: " + toolName + " is blocked. Finish the plan; the user runs /plan to leave plan mode."
	}
	if p.Mode == DangerFullAccess {
		return true, ""
	}
	// Check trusted tools store before prompting
	if p.Trusted != nil && p.Trusted.IsTrusted(toolName, input) {
		return true, ""
	}
	// In WorkspaceWrite mode, prompt the user if a prompter is available
	if p.Prompter != nil {
		allowed, err := p.Prompter.Prompt(toolName, input)
		if err != nil {
			return false, "permission prompt error: " + err.Error()
		}
		if !allowed {
			return false, "permission denied by user"
		}
	}
	return true, ""
}

// AllowAllPrompter always allows tool execution.
type AllowAllPrompter struct{}

func (AllowAllPrompter) Prompt(string, string) (bool, error) { return true, nil }
