package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/AlleyBo55/gocode/internal/agent"
)

// --- Config loading tests ---

func TestNewShellHookRunner_LoadsConfig(t *testing.T) {
	dir := t.TempDir()
	config := HookConfig{
		Hooks: []HookDef{
			{Event: "PreToolUse", Pattern: "bashtool", Command: "echo ok", Timeout: 10},
			{Event: "PostToolUse", Pattern: "*", Command: "echo done"},
		},
	}
	data, _ := json.Marshal(config)
	configPath := filepath.Join(dir, "hooks.json")
	os.WriteFile(configPath, data, 0o644)

	runner, err := NewShellHookRunner(configPath, nil)
	if err != nil {
		t.Fatalf("NewShellHookRunner: %v", err)
	}
	if len(runner.config.Hooks) != 2 {
		t.Errorf("expected 2 hooks, got %d", len(runner.config.Hooks))
	}
	if runner.config.Hooks[0].Event != "PreToolUse" {
		t.Errorf("hook[0].Event = %q, want %q", runner.config.Hooks[0].Event, "PreToolUse")
	}
	if runner.config.Hooks[0].Timeout != 10 {
		t.Errorf("hook[0].Timeout = %d, want 10", runner.config.Hooks[0].Timeout)
	}
}

func TestNewShellHookRunner_MissingFile(t *testing.T) {
	_, err := NewShellHookRunner("/nonexistent/hooks.json", nil)
	if err == nil {
		t.Fatal("expected error for missing config file")
	}
}

func TestNewShellHookRunner_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "hooks.json")
	os.WriteFile(configPath, []byte("{invalid json"), 0o644)

	_, err := NewShellHookRunner(configPath, nil)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

// --- PreToolUse tests ---

func skipIfWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell hook tests require unix sh")
	}
}

func writeHookConfig(t *testing.T, dir string, hooks []HookDef) string {
	t.Helper()
	config := HookConfig{Hooks: hooks}
	data, _ := json.Marshal(config)
	configPath := filepath.Join(dir, "hooks.json")
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return configPath
}

func TestPreToolUse_DenyViaExitCode2(t *testing.T) {
	skipIfWindows(t)
	dir := t.TempDir()

	// Create a script that exits with code 2
	scriptPath := filepath.Join(dir, "deny.sh")
	os.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 2\n"), 0o755)

	configPath := writeHookConfig(t, dir, []HookDef{
		{Event: "PreToolUse", Pattern: "bashtool", Command: scriptPath, Timeout: 5},
	})

	runner, err := NewShellHookRunner(configPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	result := runner.PreToolUse("bashtool", map[string]interface{}{"command": "ls"})
	if !result.Denied {
		t.Error("expected Denied=true for exit code 2")
	}
	if len(result.Messages) == 0 {
		t.Error("expected at least one message")
	}
}

func TestPreToolUse_DenyViaJSON(t *testing.T) {
	skipIfWindows(t)
	dir := t.TempDir()

	scriptPath := filepath.Join(dir, "deny-json.sh")
	os.WriteFile(scriptPath, []byte(`#!/bin/sh
echo '{"permissionDecision":"deny","message":"blocked by policy"}'
`), 0o755)

	configPath := writeHookConfig(t, dir, []HookDef{
		{Event: "PreToolUse", Pattern: "bashtool", Command: scriptPath, Timeout: 5},
	})

	runner, err := NewShellHookRunner(configPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	result := runner.PreToolUse("bashtool", map[string]interface{}{"command": "rm -rf /"})
	if !result.Denied {
		t.Error("expected Denied=true for JSON deny")
	}
	found := false
	for _, m := range result.Messages {
		if m == "blocked by policy" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected message 'blocked by policy', got %v", result.Messages)
	}
}

func TestPreToolUse_AskEscalation(t *testing.T) {
	skipIfWindows(t)
	dir := t.TempDir()

	scriptPath := filepath.Join(dir, "ask.sh")
	os.WriteFile(scriptPath, []byte(`#!/bin/sh
echo '{"permissionDecision":"ask","message":"please confirm"}'
`), 0o755)

	configPath := writeHookConfig(t, dir, []HookDef{
		{Event: "PreToolUse", Pattern: "*", Command: scriptPath, Timeout: 5},
	})

	runner, err := NewShellHookRunner(configPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	result := runner.PreToolUse("anytool", map[string]interface{}{})
	if result.Denied {
		t.Error("expected Denied=false for ask")
	}
	if !result.Escalate {
		t.Error("expected Escalate=true for ask")
	}
}

func TestPreToolUse_UpdatedInput(t *testing.T) {
	skipIfWindows(t)
	dir := t.TempDir()

	scriptPath := filepath.Join(dir, "update.sh")
	os.WriteFile(scriptPath, []byte(`#!/bin/sh
echo '{"permissionDecision":"allow","updatedInput":{"command":"ls -la"}}'
`), 0o755)

	configPath := writeHookConfig(t, dir, []HookDef{
		{Event: "PreToolUse", Pattern: "bashtool", Command: scriptPath, Timeout: 5},
	})

	runner, err := NewShellHookRunner(configPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	result := runner.PreToolUse("bashtool", map[string]interface{}{"command": "ls"})
	if result.Denied {
		t.Error("expected Denied=false for allow with updatedInput")
	}
	if result.UpdatedInput == nil {
		t.Fatal("expected UpdatedInput to be set")
	}
	if result.UpdatedInput["command"] != "ls -la" {
		t.Errorf("UpdatedInput[command] = %v, want 'ls -la'", result.UpdatedInput["command"])
	}
}

func TestPreToolUse_NonZeroExitWarns(t *testing.T) {
	skipIfWindows(t)
	dir := t.TempDir()

	scriptPath := filepath.Join(dir, "warn.sh")
	os.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 1\n"), 0o755)

	configPath := writeHookConfig(t, dir, []HookDef{
		{Event: "PreToolUse", Pattern: "bashtool", Command: scriptPath, Timeout: 5},
	})

	runner, err := NewShellHookRunner(configPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	result := runner.PreToolUse("bashtool", map[string]interface{}{})
	if result.Denied {
		t.Error("expected Denied=false for non-2 exit code")
	}
	if len(result.Messages) == 0 {
		t.Error("expected warning message for non-zero exit")
	}
}

func TestPreToolUse_NoMatchingPattern(t *testing.T) {
	skipIfWindows(t)
	dir := t.TempDir()

	scriptPath := filepath.Join(dir, "deny.sh")
	os.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 2\n"), 0o755)

	configPath := writeHookConfig(t, dir, []HookDef{
		{Event: "PreToolUse", Pattern: "bashtool", Command: scriptPath, Timeout: 5},
	})

	runner, err := NewShellHookRunner(configPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	// "fileedittool" doesn't match "bashtool" pattern
	result := runner.PreToolUse("fileedittool", map[string]interface{}{})
	if result.Denied {
		t.Error("expected Denied=false when pattern doesn't match")
	}
}

func TestPreToolUse_DelegatesInner(t *testing.T) {
	dir := t.TempDir()
	configPath := writeHookConfig(t, dir, nil) // no shell hooks

	inner := &mockHookRunner{
		preResult: agent.HookResult{Denied: true, Messages: []string{"inner denied"}},
	}

	runner, err := NewShellHookRunner(configPath, inner)
	if err != nil {
		t.Fatal(err)
	}

	result := runner.PreToolUse("anytool", map[string]interface{}{})
	if !result.Denied {
		t.Error("expected inner denial to propagate")
	}
}

// --- PostToolUse tests ---

func TestPostToolUse_LogsMessage(t *testing.T) {
	skipIfWindows(t)
	dir := t.TempDir()

	scriptPath := filepath.Join(dir, "log.sh")
	os.WriteFile(scriptPath, []byte(`#!/bin/sh
echo '{"message":"post-hook logged"}'
`), 0o755)

	configPath := writeHookConfig(t, dir, []HookDef{
		{Event: "PostToolUse", Pattern: "fileedittool", Command: scriptPath, Timeout: 5},
	})

	runner, err := NewShellHookRunner(configPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	result := runner.PostToolUse("fileedittool", map[string]interface{}{"path": "main.go"}, "ok", false)
	found := false
	for _, m := range result.Messages {
		if m == "post-hook logged" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected message 'post-hook logged', got %v", result.Messages)
	}
}

func TestPostToolUse_DelegatesInner(t *testing.T) {
	dir := t.TempDir()
	configPath := writeHookConfig(t, dir, nil)

	inner := &mockHookRunner{
		postResult: agent.HookResult{Messages: []string{"inner post msg"}},
	}

	runner, err := NewShellHookRunner(configPath, inner)
	if err != nil {
		t.Fatal(err)
	}

	result := runner.PostToolUse("anytool", map[string]interface{}{}, "output", false)
	found := false
	for _, m := range result.Messages {
		if m == "inner post msg" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected inner message, got %v", result.Messages)
	}
}

// --- Timeout test ---

func TestPreToolUse_Timeout(t *testing.T) {
	skipIfWindows(t)
	dir := t.TempDir()

	scriptPath := filepath.Join(dir, "slow.sh")
	os.WriteFile(scriptPath, []byte("#!/bin/sh\nsleep 30\n"), 0o755)

	configPath := writeHookConfig(t, dir, []HookDef{
		{Event: "PreToolUse", Pattern: "*", Command: scriptPath, Timeout: 1},
	})

	runner, err := NewShellHookRunner(configPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	result := runner.PreToolUse("anytool", map[string]interface{}{})
	// Should not deny, but should have an error message about timeout
	if result.Denied {
		t.Error("timed-out hook should not deny")
	}
	if len(result.Messages) == 0 {
		t.Error("expected timeout error message")
	}
}

// --- Missing script test ---

func TestPreToolUse_MissingScript(t *testing.T) {
	skipIfWindows(t)
	dir := t.TempDir()

	configPath := writeHookConfig(t, dir, []HookDef{
		{Event: "PreToolUse", Pattern: "*", Command: "/nonexistent/script.sh", Timeout: 5},
	})

	runner, err := NewShellHookRunner(configPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	result := runner.PreToolUse("anytool", map[string]interface{}{})
	if result.Denied {
		t.Error("missing script should not deny")
	}
	if len(result.Messages) == 0 {
		t.Error("expected error message for missing script")
	}
}

// --- matchGlob tests ---

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern, name string
		want          bool
	}{
		{"*", "anything", true},
		{"bashtool", "bashtool", true},
		{"bashtool", "fileedittool", false},
		{"bash*", "bashtool", true},
		{"bash*", "fileedittool", false},
		{"file*tool", "fileedittool", true},
	}
	for _, tt := range tests {
		got := matchGlob(tt.pattern, tt.name)
		if got != tt.want {
			t.Errorf("matchGlob(%q, %q) = %v, want %v", tt.pattern, tt.name, got, tt.want)
		}
	}
}

// --- mock ---

type mockHookRunner struct {
	preResult  agent.HookResult
	postResult agent.HookResult
}

func (m *mockHookRunner) PreToolUse(string, map[string]interface{}) agent.HookResult {
	return m.preResult
}

func (m *mockHookRunner) PostToolUse(string, map[string]interface{}, string, bool) agent.HookResult {
	return m.postResult
}
