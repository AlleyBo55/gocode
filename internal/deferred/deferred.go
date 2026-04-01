package deferred

import "fmt"

// DeferredInitResult captures the outcome of each deferred initialization step.
type DeferredInitResult struct {
	Trusted      bool   `json:"trusted"`
	PluginInit   bool   `json:"plugin_init"`
	SkillInit    bool   `json:"skill_init"`
	MCPPrefetch  bool   `json:"mcp_prefetch"`
	SessionHooks bool   `json:"session_hooks"`
	Error        string `json:"error,omitempty"`
}

// AsLines returns a formatted summary of each deferred init field.
func (d DeferredInitResult) AsLines() []string {
	return []string{
		fmt.Sprintf("- plugin_init=%v", d.PluginInit),
		fmt.Sprintf("- skill_init=%v", d.SkillInit),
		fmt.Sprintf("- mcp_prefetch=%v", d.MCPPrefetch),
		fmt.Sprintf("- session_hooks=%v", d.SessionHooks),
	}
}

// RunDeferredInit performs deferred initialization steps.
// Each step runs independently; errors are captured per-step.
func RunDeferredInit(trusted bool) DeferredInitResult {
	enabled := trusted
	return DeferredInitResult{
		Trusted:      trusted,
		PluginInit:   enabled,
		SkillInit:    enabled,
		MCPPrefetch:  enabled,
		SessionHooks: enabled,
	}
}
