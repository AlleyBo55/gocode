package agent

import (
	"errors"
	"strings"
	"testing"
)

// scriptedPrompter answers with a fixed decision and counts how often it was
// consulted, so tests can assert that trusted calls skip the prompt entirely.
type scriptedPrompter struct {
	allow bool
	err   error
	calls int
	last  [2]string
}

func (p *scriptedPrompter) Prompt(tool, op string) (bool, error) {
	p.calls++
	p.last = [2]string{tool, op}
	return p.allow, p.err
}

func TestAuthorizeDecisionTable(t *testing.T) {
	deny := func() *scriptedPrompter { return &scriptedPrompter{allow: false} }
	allow := func() *scriptedPrompter { return &scriptedPrompter{allow: true} }
	failing := func() *scriptedPrompter { return &scriptedPrompter{err: errors.New("stdin closed")} }

	cases := []struct {
		name        string
		mode        PermissionMode
		prompter    func() *scriptedPrompter
		trusted     *TrustedToolStore
		tool, input string
		wantAllowed bool
		wantReason  string
		wantPrompts int
	}{
		{
			name: "full access never prompts, even when the prompter would deny",
			mode: DangerFullAccess, prompter: deny, tool: "BashTool", input: `{"command":"rm -rf /"}`,
			wantAllowed: true, wantPrompts: 0,
		},
		{
			name: "full access ignores the trusted store entirely",
			mode: DangerFullAccess, prompter: deny, trusted: storeWith(), tool: "BashTool", input: `{}`,
			wantAllowed: true, wantPrompts: 0,
		},
		{
			name: "workspace mode with a trusted match skips the prompt",
			mode: WorkspaceWrite, prompter: deny, trusted: storeWith("BashTool:git *"), tool: "BashTool", input: `{"command":"git status"}`,
			wantAllowed: true, wantPrompts: 0,
		},
		{
			name: "workspace mode with a trusted store but no match asks",
			mode: WorkspaceWrite, prompter: allow, trusted: storeWith("BashTool:git *"), tool: "BashTool", input: `{"command":"rm -rf /"}`,
			wantAllowed: true, wantPrompts: 1,
		},
		{
			name: "user approval allows once",
			mode: WorkspaceWrite, prompter: allow, tool: "BashTool", input: `{"command":"ls"}`,
			wantAllowed: true, wantPrompts: 1,
		},
		{
			name: "user denial is reported as such",
			mode: WorkspaceWrite, prompter: deny, tool: "BashTool", input: `{"command":"ls"}`,
			wantAllowed: false, wantReason: "permission denied by user", wantPrompts: 1,
		},
		{
			name: "a prompter failure denies and says why",
			mode: WorkspaceWrite, prompter: failing, tool: "BashTool", input: `{"command":"ls"}`,
			wantAllowed: false, wantReason: "permission prompt error: stdin closed", wantPrompts: 1,
		},
		{
			name: "a prompter failure denies even for a tool the store would otherwise not match",
			mode: WorkspaceWrite, prompter: failing, trusted: storeWith("OtherTool"), tool: "BashTool", input: `{}`,
			wantAllowed: false, wantReason: "permission prompt error: stdin closed", wantPrompts: 1,
		},
		{
			name: "the store is consulted before the prompter, so a denying prompter still loses to trust",
			mode: WorkspaceWrite, prompter: deny, trusted: storeWith("*"), tool: "Anything", input: `{}`,
			wantAllowed: true, wantPrompts: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.prompter()
			policy := PermissionPolicy{Mode: tc.mode, Prompter: p, Trusted: tc.trusted}
			allowed, reason := policy.Authorize(tc.tool, tc.input)
			if allowed != tc.wantAllowed {
				t.Errorf("allowed = %v, want %v (reason %q)", allowed, tc.wantAllowed, reason)
			}
			if reason != tc.wantReason {
				t.Errorf("reason = %q, want %q", reason, tc.wantReason)
			}
			if p.calls != tc.wantPrompts {
				t.Errorf("prompter consulted %d times, want %d", p.calls, tc.wantPrompts)
			}
			if tc.wantPrompts > 0 && (p.last[0] != tc.tool || p.last[1] != tc.input) {
				t.Errorf("prompter saw %v, want tool=%s input=%s", p.last, tc.tool, tc.input)
			}
		})
	}
}

func TestAuthorizeWithNoPrompterFailsOpen(t *testing.T) {
	// Documented, not endorsed: PermissionPolicy without a Prompter allows
	// everything in WorkspaceWrite mode. The only constructor in the codebase
	// (NewConversationRuntime) substitutes AllowAllPrompter for nil, so this
	// path is not reachable from the CLI. If a second constructor ever appears
	// it should fail closed, and this test is where that decision is recorded.
	policy := PermissionPolicy{Mode: WorkspaceWrite}
	if allowed, _ := policy.Authorize("BashTool", `{"command":"rm -rf /"}`); !allowed {
		t.Error("current behaviour is allow; if you changed it to deny, update this test and the comment above")
	}
}

func TestAllowAllPrompterAllows(t *testing.T) {
	ok, err := AllowAllPrompter{}.Prompt("x", "y")
	if !ok || err != nil {
		t.Errorf("AllowAllPrompter = %v, %v", ok, err)
	}
}

func TestRuntimeDefaultsToAllowAllPrompterWhenNoneGiven(t *testing.T) {
	// This is the reason the fail-open path above is unreachable: the runtime
	// never hands the policy a nil prompter.
	rt := NewConversationRuntime(RuntimeOptions{})
	if rt.permPolicy.Prompter == nil {
		t.Fatal("runtime must substitute a prompter")
	}
	if _, isAllowAll := rt.permPolicy.Prompter.(AllowAllPrompter); !isAllowAll {
		t.Errorf("default prompter is %T, want AllowAllPrompter", rt.permPolicy.Prompter)
	}
	if rt.permPolicy.Mode != WorkspaceWrite {
		t.Errorf("default mode = %v, want WorkspaceWrite (prompting), never full access by accident", rt.permPolicy.Mode)
	}
}

func TestAuthorizeDenialReasonIsUsableAsToolOutput(t *testing.T) {
	// The runtime returns the reason to the model as the tool result, so it
	// has to be a plain sentence the model can act on.
	policy := PermissionPolicy{Mode: WorkspaceWrite, Prompter: &scriptedPrompter{allow: false}}
	_, reason := policy.Authorize("BashTool", `{"command":"ls"}`)
	if strings.TrimSpace(reason) == "" || strings.ContainsAny(reason, "{}") {
		t.Errorf("reason %q should be prose, not JSON", reason)
	}
}
