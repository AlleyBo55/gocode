package agent

import "github.com/AlleyBo55/gocode/internal/apitypes"

// HookRunner executes pre/post tool-use hooks.
type HookRunner interface {
	PreToolUse(toolName string, input map[string]interface{}) HookResult
	PostToolUse(toolName string, input map[string]interface{}, output string, isError bool) HookResult
}

// HookResult captures the outcome of a hook execution.
type HookResult struct {
	Denied   bool
	Messages []string
}

// IsDenied returns true if the hook denied the operation.
func (r HookResult) IsDenied() bool { return r.Denied }

// NoOpHookRunner is a HookRunner that does nothing.
type NoOpHookRunner struct{}

func (NoOpHookRunner) PreToolUse(string, map[string]interface{}) HookResult {
	return HookResult{}
}

func (NoOpHookRunner) PostToolUse(string, map[string]interface{}, string, bool) HookResult {
	return HookResult{}
}

// MergeHookFeedback appends hook messages to tool output.
func MergeHookFeedback(messages []string, output string, denied bool) string {
	if len(messages) == 0 {
		return output
	}
	label := "Hook feedback"
	if denied {
		label = "Hook feedback (denied)"
	}
	result := output
	if result != "" {
		result += "\n\n"
	}
	result += label + ":\n"
	for _, m := range messages {
		result += m + "\n"
	}
	return result
}

// ToolResultFromHookDenial creates a ToolResult for a denied hook.
func ToolResultFromHookDenial(toolUseID, toolName string, hookResult HookResult) apitypes.ToolResult {
	msg := "PreToolUse hook denied tool `" + toolName + "`"
	if len(hookResult.Messages) > 0 {
		msg = ""
		for _, m := range hookResult.Messages {
			msg += m + "\n"
		}
	}
	return apitypes.ToolResult{ToolUseID: toolUseID, Output: msg, IsError: true}
}
