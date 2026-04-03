// Package hooks provides shell-script lifecycle hooks for tool execution.
// Hooks are configured via .gocode/hooks.json and execute shell commands
// at PreToolUse and PostToolUse events.
package hooks

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/AlleyBo55/gocode/internal/agent"
)

// HookConfig is the top-level hooks configuration from .gocode/hooks.json.
type HookConfig struct {
	Hooks []HookDef `json:"hooks"`
}

// HookDef defines a single lifecycle hook.
type HookDef struct {
	Event   string `json:"event"`   // "PreToolUse", "PostToolUse", "Notification"
	Pattern string `json:"pattern"` // glob pattern for tool names
	Command string `json:"command"` // shell command to execute
	Timeout int    `json:"timeout"` // seconds, default 30
}

// HookOutput is the JSON structure a hook script can emit on stdout.
type HookOutput struct {
	PermissionDecision string                 `json:"permissionDecision"` // "allow", "deny", "ask"
	UpdatedInput       map[string]interface{} `json:"updatedInput,omitempty"`
	Message            string                 `json:"message,omitempty"`
}

// defaultTimeout is the default hook execution timeout in seconds.
const defaultTimeout = 30

// ShellHookRunner implements agent.HookRunner by executing shell scripts.
type ShellHookRunner struct {
	config HookConfig
	inner  agent.HookRunner
}

// NewShellHookRunner loads hooks.json from configPath and wraps an existing HookRunner.
func NewShellHookRunner(configPath string, inner agent.HookRunner) (*ShellHookRunner, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("reading hooks config: %w", err)
	}
	var config HookConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing hooks config: %w", err)
	}
	if inner == nil {
		inner = agent.NoOpHookRunner{}
	}
	return &ShellHookRunner{config: config, inner: inner}, nil
}

// PreToolUse executes matching PreToolUse hooks, then delegates to inner.
func (r *ShellHookRunner) PreToolUse(toolName string, input map[string]interface{}) agent.HookResult {
	var messages []string
	denied := false
	escalate := false
	var updatedInput map[string]interface{}

	for _, h := range r.config.Hooks {
		if !strings.EqualFold(h.Event, "PreToolUse") {
			continue
		}
		if !matchGlob(h.Pattern, toolName) {
			continue
		}

		output, exitCode, err := r.executeHook(h, toolName, input, "", false)
		if err != nil {
			messages = append(messages, fmt.Sprintf("[hook] error executing %s: %v", h.Command, err))
			continue
		}

		// Exit code 2 = deny regardless of stdout
		if exitCode == 2 {
			denied = true
			msg := "hook denied tool call (exit code 2)"
			if output != nil && output.Message != "" {
				msg = output.Message
			}
			messages = append(messages, msg)
			continue
		}

		// Non-zero exit (not 2) = warn but allow
		if exitCode != 0 {
			msg := fmt.Sprintf("[hook] warning: %s exited with code %d", h.Command, exitCode)
			if output != nil && output.Message != "" {
				msg = output.Message
			}
			messages = append(messages, msg)
			continue
		}

		// Parse JSON output for permission decisions
		if output != nil {
			switch output.PermissionDecision {
			case "deny":
				denied = true
				if output.Message != "" {
					messages = append(messages, output.Message)
				} else {
					messages = append(messages, "hook denied tool call")
				}
			case "ask":
				escalate = true
				if output.Message != "" {
					messages = append(messages, output.Message)
				}
			case "allow", "":
				if output.Message != "" {
					messages = append(messages, output.Message)
				}
			}
			if output.UpdatedInput != nil {
				updatedInput = output.UpdatedInput
			}
		}
	}

	// Delegate to inner hook runner
	innerResult := r.inner.PreToolUse(toolName, input)
	if innerResult.Denied {
		denied = true
	}
	if innerResult.Escalate {
		escalate = true
	}
	messages = append(messages, innerResult.Messages...)
	if innerResult.UpdatedInput != nil && updatedInput == nil {
		updatedInput = innerResult.UpdatedInput
	}

	return agent.HookResult{
		Denied:       denied,
		Messages:     messages,
		UpdatedInput: updatedInput,
		Escalate:     escalate,
	}
}

// PostToolUse executes matching PostToolUse hooks, then delegates to inner.
func (r *ShellHookRunner) PostToolUse(toolName string, input map[string]interface{}, output string, isError bool) agent.HookResult {
	var messages []string

	for _, h := range r.config.Hooks {
		if !strings.EqualFold(h.Event, "PostToolUse") {
			continue
		}
		if !matchGlob(h.Pattern, toolName) {
			continue
		}

		hookOut, _, err := r.executeHook(h, toolName, input, output, isError)
		if err != nil {
			messages = append(messages, fmt.Sprintf("[hook] error executing %s: %v", h.Command, err))
			continue
		}
		if hookOut != nil && hookOut.Message != "" {
			messages = append(messages, hookOut.Message)
		}
	}

	// Delegate to inner hook runner
	innerResult := r.inner.PostToolUse(toolName, input, output, isError)
	messages = append(messages, innerResult.Messages...)

	return agent.HookResult{
		Denied:   innerResult.Denied,
		Messages: messages,
	}
}

// executeHook runs a single hook command with the given context.
func (r *ShellHookRunner) executeHook(h HookDef, toolName string, input map[string]interface{}, output string, isError bool) (*HookOutput, int, error) {
	timeout := h.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", h.Command)
	// Ensure the process group is killed on timeout
	cmd.WaitDelay = time.Second

	// Set environment variables
	inputJSON, _ := json.Marshal(input)
	cmd.Env = append(os.Environ(),
		"GOCODE_TOOL_NAME="+toolName,
		"GOCODE_TOOL_INPUT="+string(inputJSON),
	)
	if output != "" {
		cmd.Env = append(cmd.Env, "GOCODE_TOOL_OUTPUT="+output)
	}
	if isError {
		cmd.Env = append(cmd.Env, "GOCODE_TOOL_ERROR=true")
	}

	stdout, err := cmd.Output()
	exitCode := 0
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, -1, fmt.Errorf("hook timed out after %ds", timeout)
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, -1, err
		}
	}

	// Try to parse stdout as JSON
	var hookOutput *HookOutput
	trimmed := strings.TrimSpace(string(stdout))
	if trimmed != "" {
		var ho HookOutput
		if json.Unmarshal([]byte(trimmed), &ho) == nil {
			hookOutput = &ho
		}
	}

	return hookOutput, exitCode, nil
}

// matchGlob checks if a tool name matches a glob pattern.
func matchGlob(pattern, name string) bool {
	if pattern == "*" {
		return true
	}
	if strings.Contains(pattern, "*") {
		matched, _ := filepath.Match(pattern, name)
		return matched
	}
	return pattern == name
}
