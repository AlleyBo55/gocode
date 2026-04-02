package plugins

import "github.com/AlleyBo55/gocode/internal/agent"

// PluginHookRunner wraps plugin hooks into the agent.HookRunner interface.
type PluginHookRunner struct {
	plugins []Plugin
}

// NewPluginHookRunner creates a hook runner from loaded plugins.
func NewPluginHookRunner(plugins []Plugin) *PluginHookRunner {
	return &PluginHookRunner{plugins: plugins}
}

// PreToolUse checks all plugin hooks for pre_tool events matching the tool name.
func (r *PluginHookRunner) PreToolUse(toolName string, input map[string]interface{}) agent.HookResult {
	var messages []string
	denied := false
	for _, p := range r.plugins {
		for _, h := range p.Hooks {
			if h.Event != "pre_tool" {
				continue
			}
			if !matchGlob(h.Pattern, toolName) {
				continue
			}
			switch h.Action {
			case "deny":
				denied = true
				msg := h.Message
				if msg == "" {
					msg = "Plugin " + p.Name + " denied tool " + toolName
				}
				messages = append(messages, msg)
			case "log":
				if h.Message != "" {
					messages = append(messages, "["+p.Name+"] "+h.Message)
				}
			}
		}
	}
	return agent.HookResult{Denied: denied, Messages: messages}
}

// PostToolUse checks all plugin hooks for post_tool events matching the tool name.
func (r *PluginHookRunner) PostToolUse(toolName string, input map[string]interface{}, output string, isError bool) agent.HookResult {
	var messages []string
	for _, p := range r.plugins {
		for _, h := range p.Hooks {
			if h.Event != "post_tool" {
				continue
			}
			if !matchGlob(h.Pattern, toolName) {
				continue
			}
			switch h.Action {
			case "log":
				if h.Message != "" {
					messages = append(messages, "["+p.Name+"] "+h.Message)
				}
			case "deny":
				// post_tool deny is a no-op (tool already ran)
			}
		}
	}
	return agent.HookResult{Messages: messages}
}
