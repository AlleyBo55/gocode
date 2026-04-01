package permissions

import "strings"

// PermissionChecker determines whether a tool is blocked.
type PermissionChecker interface {
	IsBlocked(toolName string) bool
}

// ToolPermissionContext holds deny-name and deny-prefix collections for tool access control.
// It is immutable after construction.
type ToolPermissionContext struct {
	denyNames    map[string]struct{}
	denyPrefixes []string
}

// NewToolPermissionContext builds a ToolPermissionContext from string slices of deny names
// and deny prefixes. All names and prefixes are lowercased for case-insensitive matching.
func NewToolPermissionContext(denyNames []string, denyPrefixes []string) *ToolPermissionContext {
	nameSet := make(map[string]struct{}, len(denyNames))
	for _, n := range denyNames {
		nameSet[strings.ToLower(n)] = struct{}{}
	}

	prefixes := make([]string, len(denyPrefixes))
	for i, p := range denyPrefixes {
		prefixes[i] = strings.ToLower(p)
	}

	return &ToolPermissionContext{
		denyNames:    nameSet,
		denyPrefixes: prefixes,
	}
}

// IsBlocked returns true if the tool name matches an entry in the deny-names set
// or starts with any entry in the deny-prefixes list.
func (ctx *ToolPermissionContext) IsBlocked(toolName string) bool {
	lower := strings.ToLower(toolName)

	if _, ok := ctx.denyNames[lower]; ok {
		return true
	}

	for _, prefix := range ctx.denyPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}

	return false
}
