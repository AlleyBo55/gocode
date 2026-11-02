package agent

import "testing"

// denyPrompter refuses everything, so an "allowed" result can only come from
// DangerFullAccess, never from a prompt.
type denyPrompter struct{}

func (denyPrompter) Prompt(string, string) (bool, error) { return false, nil }

func TestPlanModeBlocksWritesEvenWithFullAccess(t *testing.T) {
	p := &PermissionPolicy{Mode: DangerFullAccess}
	p.SetPlanMode(true)

	for _, tool := range []string{"BashTool", "filewritetool", "FileEditTool", "mcp_rea_open_binary", "task", "ast_grep"} {
		if ok, reason := p.Authorize(tool, "{}"); ok || reason == "" {
			t.Errorf("%s: allowed in plan mode (reason %q)", tool, reason)
		}
	}
	for _, tool := range []string{"FileReadTool", "greptool", "GlobTool", "lsp_find_references"} {
		if ok, reason := p.Authorize(tool, "{}"); !ok {
			t.Errorf("%s: blocked in plan mode: %s", tool, reason)
		}
	}
}

func TestPlanModeOffRestoresNormalRules(t *testing.T) {
	p := &PermissionPolicy{Mode: WorkspaceWrite, Prompter: denyPrompter{}}
	p.SetPlanMode(true)
	p.SetPlanMode(false)
	if p.PlanMode() {
		t.Fatal("PlanMode() = true after SetPlanMode(false)")
	}
	// Back to normal: the prompter decides, and it denies.
	if ok, reason := p.Authorize("BashTool", "{}"); ok || reason != "permission denied by user" {
		t.Fatalf("Authorize = %v, %q; want prompter denial", ok, reason)
	}
	full := &PermissionPolicy{Mode: DangerFullAccess}
	if ok, _ := full.Authorize("BashTool", "{}"); !ok {
		t.Fatal("DangerFullAccess without plan mode should allow BashTool")
	}
}
