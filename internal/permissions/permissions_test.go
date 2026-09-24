package permissions

import "testing"

func TestIsBlockedDecisionTable(t *testing.T) {
	cases := []struct {
		name         string
		denyNames    []string
		denyPrefixes []string
		tool         string
		want         bool
	}{
		{"empty context blocks nothing", nil, nil, "BashTool", false},
		{"exact name blocks", []string{"BashTool"}, nil, "BashTool", true},
		{"exact name is case-insensitive both ways", []string{"bashtool"}, nil, "BASHTOOL", true},
		{"exact name does not block a prefix of itself", []string{"BashTool"}, nil, "Bash", false},
		{"exact name does not block a longer name", []string{"Bash"}, nil, "BashTool", false},
		{"prefix blocks", nil, []string{"mcp__"}, "mcp__github__create_issue", true},
		{"prefix is case-insensitive", nil, []string{"MCP__"}, "mcp__x", true},
		{"prefix is anchored at the start", nil, []string{"github"}, "mcp__github__x", false},
		{"either list is enough", []string{"Other"}, []string{"mcp__"}, "mcp__x", true},
		{"unrelated tool passes", []string{"BashTool"}, []string{"mcp__"}, "FileReadTool", false},
		// A deny-list is conservative: an empty prefix matches every name.
		// A config typo that produces "" therefore blocks all tools, which is
		// loud and safe rather than silent and permissive.
		{"empty prefix blocks everything", nil, []string{""}, "FileReadTool", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := NewToolPermissionContext(tc.denyNames, tc.denyPrefixes)
			if got := ctx.IsBlocked(tc.tool); got != tc.want {
				t.Errorf("IsBlocked(%q) = %v, want %v", tc.tool, got, tc.want)
			}
		})
	}
}

func TestToolPermissionContextDoesNotAliasItsInputs(t *testing.T) {
	names := []string{"BashTool"}
	prefixes := []string{"mcp__"}
	ctx := NewToolPermissionContext(names, prefixes)
	names[0] = "Other"
	prefixes[0] = "zzz"
	if !ctx.IsBlocked("BashTool") || !ctx.IsBlocked("mcp__x") {
		t.Error("context must copy its inputs; it is documented as immutable after construction")
	}
}

func TestToolPermissionContextSatisfiesChecker(t *testing.T) {
	var _ PermissionChecker = NewToolPermissionContext(nil, nil)
}
