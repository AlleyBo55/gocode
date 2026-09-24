package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/AlleyBo55/gocode/internal/agent"
	"github.com/AlleyBo55/gocode/internal/apiclient"
	"github.com/AlleyBo55/gocode/internal/apitypes"
	"github.com/AlleyBo55/gocode/internal/swarm"
)

// SubAgentDef defines a specialist agent profile.
type SubAgentDef struct {
	Name         string
	SystemPrompt string
	Model        string
	ToolPerms    []string // allowed tool names; empty = all
	Category     apiclient.TaskCategory
}

// AgentResult is the outcome of a background agent execution.
type AgentResult struct {
	AgentName string
	Output    string
	Err       error
}

// Orchestrator coordinates specialist sub-agents.
type Orchestrator struct {
	registry   map[string]SubAgentDef
	router     *apiclient.ModelRouter
	executor   agent.ToolExecutor
	maxBgAgent int

	// Swarm, when set, makes every delegated agent a member: it is registered
	// under a unique instance name for the duration of its run, gets the
	// send_agent_message and list_agents tools bound to that name, and reads
	// its inbox before each turn. Background agents post their result to the
	// main agent's inbox when they finish.
	Swarm *swarm.SwarmManager

	mu  sync.Mutex
	seq map[string]int // per-profile instance counter for unique names
}

// WithSwarm attaches a swarm manager and returns the orchestrator for chaining.
func (o *Orchestrator) WithSwarm(s *swarm.SwarmManager) *Orchestrator {
	o.Swarm = s
	return o
}

// NewOrchestrator creates an orchestrator with built-in sub-agent profiles
// and wires the given ModelRouter for category-based provider selection.
func NewOrchestrator(router *apiclient.ModelRouter, executor agent.ToolExecutor) *Orchestrator {
	o := &Orchestrator{
		registry:   make(map[string]SubAgentDef),
		router:     router,
		executor:   executor,
		maxBgAgent: 5,
	}
	o.registerBuiltins()
	return o
}

// registerBuiltins adds the four built-in sub-agent profiles.
func (o *Orchestrator) registerBuiltins() {
	o.registry["coordinator"] = SubAgentDef{
		Name:         "coordinator",
		SystemPrompt: "You are the main coordinator agent. Break complex tasks into sub-tasks and delegate to specialist agents. Synthesize results into a coherent response.",
		Model:        "",
		ToolPerms:    nil,
		Category:     apiclient.CategoryQuick,
	}
	o.registry["deep-worker"] = SubAgentDef{
		Name:         "deep-worker",
		SystemPrompt: "You are a research agent specializing in deep analysis. Thoroughly investigate the codebase, read files, search for patterns, and provide detailed findings.",
		Model:        "",
		ToolPerms:    []string{"filereadtool", "greptool", "globtool", "bashtool"},
		Category:     apiclient.CategoryDeep,
	}
	o.registry["planner"] = SubAgentDef{
		Name:         "planner",
		SystemPrompt: "You are a planning agent. Analyze the task scope, identify ambiguities, and produce a structured plan with clear steps and rationale.",
		Model:        "",
		ToolPerms:    nil,
		Category:     apiclient.CategoryQuick,
	}
	o.registry["debugger"] = SubAgentDef{
		Name:         "debugger",
		SystemPrompt: "You are a debugging agent. Diagnose errors by reading stack traces, inspecting code, running targeted tests, and identifying root causes.",
		Model:        "",
		ToolPerms:    []string{"filereadtool", "greptool", "bashtool", "fileedittool"},
		Category:     apiclient.CategoryDeep,
	}
}

// Register adds or replaces a sub-agent definition in the registry.
func (o *Orchestrator) Register(def SubAgentDef) {
	o.registry[def.Name] = def
}

// Delegate spawns a new ConversationRuntime for the named sub-agent,
// sends the task, and returns the agent's text output.
func (o *Orchestrator) Delegate(ctx context.Context, agentName string, task string) (string, error) {
	output, _, err := o.delegate(ctx, agentName, task)
	return output, err
}

// delegate runs one sub-agent and also returns the swarm instance name it ran
// under, so a background completion can be attributed.
func (o *Orchestrator) delegate(ctx context.Context, agentName string, task string) (string, string, error) {
	def, ok := o.registry[agentName]
	if !ok {
		return "", "", fmt.Errorf("unknown sub-agent: %q", agentName)
	}

	provider, err := o.router.Route(def.Category)
	if err != nil {
		return "", "", fmt.Errorf("routing for agent %q: %w", agentName, err)
	}

	// Build a tool executor that respects the sub-agent's tool permissions.
	exec := o.filteredExecutor(def)
	opts := agent.RuntimeOptions{
		Provider:      provider,
		Executor:      exec,
		Model:         resolveModel(def, provider),
		MaxTokens:     8192,
		MaxIterations: 30,
		SystemPrompt:  def.SystemPrompt,
	}

	instance := agentName
	if o.Swarm != nil {
		instance = o.nextInstanceName(agentName)
		if err := o.Swarm.Register(instance, def.ToolPerms, string(def.Category)); err != nil {
			return "", "", fmt.Errorf("joining swarm as %q: %w", instance, err)
		}
		defer o.Swarm.Unregister(instance)
		opts.Executor = swarm.WrapExecutor(exec, o.Swarm, instance)
		opts.Inbox = o.Swarm.InboxFor(instance)
		opts.SystemPrompt = def.SystemPrompt + "\n\nYou are agent \"" + instance + "\" in a swarm. " +
			"Other agents may message you; their messages appear at the start of a turn. " +
			"Report to \"" + swarm.MainAgent + "\" with send_agent_message when you have something it should know before you finish."
	}

	rt := agent.NewConversationRuntime(opts)
	resp, err := rt.SendUserMessage(ctx, task)
	if err != nil {
		return "", instance, fmt.Errorf("sub-agent %q failed: %w", instance, err)
	}
	return extractTextOutput(resp), instance, nil
}

// nextInstanceName gives each spawn of a profile its own swarm identity:
// deep-worker-1, deep-worker-2, ... Two parallel deep-workers must not share
// a mailbox.
func (o *Orchestrator) nextInstanceName(profile string) string {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.seq == nil {
		o.seq = make(map[string]int)
	}
	o.seq[profile]++
	return fmt.Sprintf("%s-%d", profile, o.seq[profile])
}

// DelegateBackground spawns a background agent in a goroutine.
// Returns a channel that will receive exactly one AgentResult. With a swarm
// attached, the result is also posted to the main agent's inbox, so the
// interactive session hears about it on its next turn without polling.
func (o *Orchestrator) DelegateBackground(ctx context.Context, agentName string, task string) <-chan AgentResult {
	ch := make(chan AgentResult, 1)
	go func() {
		defer close(ch)
		output, instance, err := o.delegate(ctx, agentName, task)
		if o.Swarm != nil {
			o.reportToMain(instance, task, output, err)
		}
		ch <- AgentResult{
			AgentName: agentName,
			Output:    output,
			Err:       err,
		}
	}()
	return ch
}

// reportToMain posts a background agent's outcome to the main agent. The
// report is bounded so a verbose agent cannot flood the main context; the
// full output is still available to whoever holds the result channel.
func (o *Orchestrator) reportToMain(instance, task, output string, err error) {
	const maxReport = 4000
	var b strings.Builder
	if err != nil {
		fmt.Fprintf(&b, "Background agent %s failed on task %q: %v", instance, truncate(task, 200), err)
	} else {
		fmt.Fprintf(&b, "Background agent %s finished task %q.\n\n%s", instance, truncate(task, 200), truncate(output, maxReport))
	}
	// Delivery can fail only if main is not registered or its mailbox is full;
	// neither is worth failing the agent over.
	_ = o.Swarm.SendMessage(instance, swarm.MainAgent, b.String())
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + fmt.Sprintf("… [%d more bytes]", len(s)-n)
}

// --- ToolExecutor interface implementation ---
// The Orchestrator implements agent.ToolExecutor so that delegation
// appears as tool calls (orchestrator_delegate, orchestrator_delegate_bg)
// to the parent ConversationRuntime.

// Execute handles orchestrator tool calls.
func (o *Orchestrator) Execute(name string, input map[string]interface{}) apitypes.ToolResult {
	switch name {
	case "orchestrator_delegate":
		return o.executeDelegate(input)
	case "orchestrator_delegate_bg":
		return o.executeDelegateBg(input)
	default:
		return apitypes.ToolResult{Output: fmt.Sprintf("unknown orchestrator tool: %s", name), IsError: true}
	}
}

// ListTools returns the tool definitions for orchestrator delegation.
func (o *Orchestrator) ListTools() []apitypes.ToolDef {
	return []apitypes.ToolDef{
		{
			Name:        "orchestrator_delegate",
			Description: "Delegate a task to a named specialist sub-agent and wait for the result.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"agent_name": {"type": "string", "description": "Name of the sub-agent to delegate to (coordinator, deep-worker, planner, debugger)"},
					"task": {"type": "string", "description": "The task description to send to the sub-agent"}
				},
				"required": ["agent_name", "task"]
			}`),
		},
		{
			Name:        "orchestrator_delegate_bg",
			Description: "Delegate a task to a named specialist sub-agent in the background. Returns immediately with a confirmation.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"agent_name": {"type": "string", "description": "Name of the sub-agent to delegate to (coordinator, deep-worker, planner, debugger)"},
					"task": {"type": "string", "description": "The task description to send to the sub-agent"}
				},
				"required": ["agent_name", "task"]
			}`),
		},
	}
}

func (o *Orchestrator) executeDelegate(input map[string]interface{}) apitypes.ToolResult {
	agentName, _ := input["agent_name"].(string)
	task, _ := input["task"].(string)

	if agentName == "" {
		return apitypes.ToolResult{Output: "agent_name is required", IsError: true}
	}
	if task == "" {
		return apitypes.ToolResult{Output: "task is required", IsError: true}
	}

	output, err := o.Delegate(context.Background(), agentName, task)
	if err != nil {
		return apitypes.ToolResult{Output: err.Error(), IsError: true}
	}
	return apitypes.ToolResult{Output: output}
}

func (o *Orchestrator) executeDelegateBg(input map[string]interface{}) apitypes.ToolResult {
	agentName, _ := input["agent_name"].(string)
	task, _ := input["task"].(string)

	if agentName == "" {
		return apitypes.ToolResult{Output: "agent_name is required", IsError: true}
	}
	if task == "" {
		return apitypes.ToolResult{Output: "task is required", IsError: true}
	}

	// Fire and forget — the background channel is not tracked here.
	// The BackgroundManager (in background.go) provides full lifecycle management.
	o.DelegateBackground(context.Background(), agentName, task)

	return apitypes.ToolResult{
		Output: fmt.Sprintf("Background agent %q started for task: %s", agentName, task),
	}
}

// --- Helpers ---

// filteredExecutor returns a ToolExecutor that only exposes the tools
// permitted by the sub-agent definition. If ToolPerms is empty, all tools
// are allowed.
func (o *Orchestrator) filteredExecutor(def SubAgentDef) agent.ToolExecutor {
	if len(def.ToolPerms) == 0 {
		return o.executor
	}
	return &permFilteredExecutor{
		inner:   o.executor,
		allowed: toSet(def.ToolPerms),
	}
}

// resolveModel picks the model string for a sub-agent. If the definition
// specifies a model, use it; otherwise fall back to the first model in the
// provider's chain (empty string lets the provider decide).
func resolveModel(def SubAgentDef, _ apiclient.Provider) string {
	if def.Model != "" {
		return def.Model
	}
	return ""
}

// extractTextOutput collects all text blocks from a MessageResponse.
func extractTextOutput(resp *apitypes.MessageResponse) string {
	if resp == nil {
		return ""
	}
	var text string
	for _, block := range resp.Content {
		if block.Kind == "text" {
			if text != "" {
				text += "\n"
			}
			text += block.Text
		}
	}
	return text
}

// toSet converts a string slice to a map for O(1) lookups.
func toSet(items []string) map[string]bool {
	s := make(map[string]bool, len(items))
	for _, item := range items {
		s[item] = true
	}
	return s
}

// permFilteredExecutor wraps a ToolExecutor and restricts which tools are visible/executable.
type permFilteredExecutor struct {
	inner   agent.ToolExecutor
	allowed map[string]bool
}

func (f *permFilteredExecutor) Execute(name string, input map[string]interface{}) apitypes.ToolResult {
	if !f.allowed[name] {
		return apitypes.ToolResult{
			Output:  fmt.Sprintf("tool %q is not permitted for this sub-agent", name),
			IsError: true,
		}
	}
	return f.inner.Execute(name, input)
}

func (f *permFilteredExecutor) ListTools() []apitypes.ToolDef {
	all := f.inner.ListTools()
	filtered := make([]apitypes.ToolDef, 0, len(f.allowed))
	for _, t := range all {
		if f.allowed[t.Name] {
			filtered = append(filtered, t)
		}
	}
	return filtered
}
