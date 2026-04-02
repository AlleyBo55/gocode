package agent

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
}

// Authorize checks if a tool invocation is allowed.
// Returns true if allowed, false if denied, and an optional denial reason.
func (p *PermissionPolicy) Authorize(toolName string, input string) (bool, string) {
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
