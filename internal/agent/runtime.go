package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AlleyBo55/gocode/internal/apiclient"
	"github.com/AlleyBo55/gocode/internal/apitypes"
)

// RuntimeOptions configures a ConversationRuntime.
type RuntimeOptions struct {
	Provider      apiclient.Provider
	Executor      ToolExecutor
	Model         string
	MaxTokens     int
	MaxIterations int
	SystemPrompt  string
	PermMode      PermissionMode
	Prompter      PermissionPrompter
	Trusted       *TrustedToolStore
	Hooks         HookRunner
	ToolCb        ToolCallback
	// MaxCostUSD stops the agent loop once the session's estimated spend,
	// from the per-model price table, reaches this amount. Zero is unlimited.
	MaxCostUSD float64
	// Inbox, when set, is drained at the start of every model turn. Each
	// string becomes a user message the model reads before it answers, which
	// is how messages from other agents in a swarm reach this one. It must
	// not block.
	Inbox func() []string
}

// ToolCallback is called before and after tool execution for UI updates.
type ToolCallback interface {
	OnToolStart(name string, input map[string]interface{})
	OnToolEnd(name string, isError bool)
}

// NoOpToolCallback does nothing.
type NoOpToolCallback struct{}

func (NoOpToolCallback) OnToolStart(string, map[string]interface{}) {}
func (NoOpToolCallback) OnToolEnd(string, bool)                     {}

// ConversationRuntime orchestrates the agentic tool-use loop.
//  ConversationRuntime<C: ApiClient, T: ToolExecutor>.
type ConversationRuntime struct {
	provider     apiclient.Provider
	executor     ToolExecutor
	session      []apitypes.InputMessage
	model        string
	maxTokens    int
	maxIter      int
	systemPrompt string
	permPolicy   PermissionPolicy
	hooks        HookRunner
	usage        UsageTracker
	toolCb       ToolCallback
	maxCost      float64
	loop         loopGuard // reset for every user message
	inbox        func() []string
}

// NewConversationRuntime creates a new runtime from options.
func NewConversationRuntime(opts RuntimeOptions) *ConversationRuntime {
	hooks := opts.Hooks
	if hooks == nil {
		hooks = NoOpHookRunner{}
	}
	prompter := opts.Prompter
	if prompter == nil {
		prompter = AllowAllPrompter{}
	}
	maxIter := opts.MaxIterations
	if maxIter <= 0 {
		maxIter = 30
	}
	maxTokens := opts.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 8192
	}
	toolCb := opts.ToolCb
	if toolCb == nil {
		toolCb = NoOpToolCallback{}
	}
	return &ConversationRuntime{
		provider:     opts.Provider,
		executor:     opts.Executor,
		model:        opts.Model,
		maxTokens:    maxTokens,
		maxIter:      maxIter,
		systemPrompt: opts.SystemPrompt,
		permPolicy:   PermissionPolicy{Mode: opts.PermMode, Prompter: prompter, Trusted: opts.Trusted},
		hooks:        hooks,
		toolCb:       toolCb,
		maxCost:      opts.MaxCostUSD,
		inbox:        opts.Inbox,
	}
}

// deliverInbox appends any pending inter-agent messages to the session as a
// user message. It runs before each request, so a message that arrives while
// tools are executing is read on the very next turn. Placing it after tool
// results keeps every tool_use answered by its tool_result; providers accept
// consecutive user messages and merge them.
func (r *ConversationRuntime) deliverInbox() {
	if r.inbox == nil {
		return
	}
	msgs := r.inbox()
	if len(msgs) == 0 {
		return
	}
	r.session = append(r.session, apitypes.UserText(strings.Join(msgs, "\n\n")))
}

// SendUserMessage runs the full agent loop: send prompt, execute tools, loop until done.
func (r *ConversationRuntime) SendUserMessage(ctx context.Context, text string) (*apitypes.MessageResponse, error) {
	r.session = append(r.session, apitypes.UserText(text))
	return r.sendLoop(ctx)
}

// SendWithMessage runs the full agent loop with a pre-built message (for multimodal input).
func (r *ConversationRuntime) SendWithMessage(ctx context.Context, msg apitypes.InputMessage) (*apitypes.MessageResponse, error) {
	r.session = append(r.session, msg)
	return r.sendLoop(ctx)
}

func (r *ConversationRuntime) sendLoop(ctx context.Context) (*apitypes.MessageResponse, error) {
	r.loop = loopGuard{}
	for iteration := 0; iteration < r.maxIter; iteration++ {
		if h := r.overBudget(); h != nil {
			return h.response(), nil
		}
		r.deliverInbox()
		req := r.buildRequest()
		resp, err := r.provider.SendMessage(ctx, req)
		if err != nil {
			return nil, err
		}
		r.usage.AddForModel(servedModel(resp.Model, r.model), resp.Usage)

		// Build assistant message from response content
		assistantMsg := apitypes.InputMessage{Role: "assistant"}
		var pendingTools []toolUseInfo
		for _, block := range resp.Content {
			switch block.Kind {
			case "text":
				assistantMsg.Content = append(assistantMsg.Content, apitypes.InputContentBlock{Kind: "text", Text: block.Text})
			case "tool_use":
				assistantMsg.Content = append(assistantMsg.Content, apitypes.InputContentBlock{
					Kind: "tool_use", ID: block.ID, Name: block.Name, Input: block.Input,
				})
				pendingTools = append(pendingTools, toolUseInfo{id: block.ID, name: block.Name, input: block.Input})
			}
		}
		r.session = append(r.session, assistantMsg)

		// No tool calls — we're done
		if len(pendingTools) == 0 {
			return resp, nil
		}

		if h := r.runTools(pendingTools); h != nil {
			return h.response(), nil
		}
	}

	// Max iterations exceeded
	return &apitypes.MessageResponse{
		Type:       "message",
		Role:       "assistant",
		StopReason: "max_iterations",
		Content:    []apitypes.OutputContentBlock{{Kind: "text", Text: fmt.Sprintf("Agent loop exceeded maximum of %d iterations", r.maxIter)}},
		Usage:      apitypes.Usage{},
	}, nil
}

// StreamUserMessage runs the agent loop with streaming responses.
// Returns a channel that emits StreamEvents for the current turn.
// For multi-turn tool loops, it internally handles tool execution and re-streams.
func (r *ConversationRuntime) StreamUserMessage(ctx context.Context, text string) (<-chan apitypes.StreamEvent, error) {
	r.session = append(r.session, apitypes.UserText(text))
	return r.streamLoop(ctx)
}

// StreamWithMessage runs the agent loop with streaming for a pre-built message (for multimodal input).
func (r *ConversationRuntime) StreamWithMessage(ctx context.Context, msg apitypes.InputMessage) (<-chan apitypes.StreamEvent, error) {
	r.session = append(r.session, msg)
	return r.streamLoop(ctx)
}

func (r *ConversationRuntime) streamLoop(ctx context.Context) (<-chan apitypes.StreamEvent, error) {
	outCh := make(chan apitypes.StreamEvent, 64)
	r.loop = loopGuard{}
	go func() {
		defer close(outCh)
		for iteration := 0; iteration < r.maxIter; iteration++ {
			if h := r.overBudget(); h != nil {
				outCh <- h.event()
				return
			}
			r.deliverInbox()
			req := r.buildRequest()
			eventCh, err := r.provider.StreamMessage(ctx, req)
			if err != nil {
				// Send an error event so the REPL can display it
				outCh <- apitypes.StreamEvent{
					Kind: "error",
					BlockDelta: &apitypes.ContentBlockDelta{
						Kind: "text_delta",
						Text: "Error: " + err.Error(),
					},
				}
				return
			}

			// Collect the full response while forwarding events
			var contentBlocks []apitypes.OutputContentBlock
			var pendingTools []toolUseInfo
			var currentUsage apitypes.Usage
			turnModel := r.model

			for ev := range eventCh {
				select {
				case outCh <- ev:
				case <-ctx.Done():
					return
				}
				// Track content blocks from stream events
				switch ev.Kind {
				case "message_start":
					if ev.Message != nil {
						// Anthropic reports input tokens here and only output
						// tokens in message_delta; reading message_delta alone
						// recorded every streamed turn as zero input.
						currentUsage = ev.Message.Usage
						turnModel = servedModel(ev.Message.Model, r.model)
					}
				case "content_block_start":
					if ev.ContentBlock != nil {
						// Ensure contentBlocks is large enough for the index
						for len(contentBlocks) <= ev.Index {
							contentBlocks = append(contentBlocks, apitypes.OutputContentBlock{})
						}
						contentBlocks[ev.Index] = *ev.ContentBlock
					}
				case "content_block_delta":
					if ev.BlockDelta != nil {
						// Grow contentBlocks if needed (OpenAI uses offset indices)
						for len(contentBlocks) <= ev.Index {
							contentBlocks = append(contentBlocks, apitypes.OutputContentBlock{})
						}
						block := &contentBlocks[ev.Index]
						switch ev.BlockDelta.Kind {
						case "text_delta":
							block.Text += ev.BlockDelta.Text
						case "input_json_delta":
							block.Input = appendJSON(block.Input, ev.BlockDelta.PartialJSON)
						}
					}
				case "message_delta":
					if ev.DeltaUsage != nil {
						currentUsage = mergeUsage(currentUsage, *ev.DeltaUsage)
					}
				}
			}

			r.usage.AddForModel(turnModel, currentUsage)

			// Build assistant message
			assistantMsg := apitypes.InputMessage{Role: "assistant"}
			for _, block := range contentBlocks {
				switch block.Kind {
				case "text":
					assistantMsg.Content = append(assistantMsg.Content, apitypes.InputContentBlock{Kind: "text", Text: block.Text})
				case "tool_use":
					assistantMsg.Content = append(assistantMsg.Content, apitypes.InputContentBlock{
						Kind: "tool_use", ID: block.ID, Name: block.Name, Input: block.Input,
					})
					pendingTools = append(pendingTools, toolUseInfo{id: block.ID, name: block.Name, input: block.Input})
				}
			}
			r.session = append(r.session, assistantMsg)

			if len(pendingTools) == 0 {
				return
			}

			if h := r.runTools(pendingTools); h != nil {
				select {
				case outCh <- h.event():
				case <-ctx.Done():
				}
				return
			}
		}
	}()
	return outCh, nil
}

type toolUseInfo struct {
	id    string
	name  string
	input json.RawMessage
}

// halt is why the agent loop stopped before the model finished on its own.
type halt struct {
	reason  string // becomes the stop_reason: "budget_exceeded" or "stuck"
	message string
}

func (h *halt) response() *apitypes.MessageResponse {
	return &apitypes.MessageResponse{
		Type:       "message",
		Role:       "assistant",
		StopReason: h.reason,
		Content:    []apitypes.OutputContentBlock{{Kind: "text", Text: h.message}},
	}
}

// event is the shape the REPL renders in red: Kind "error" with the text in
// BlockDelta, the same one provider failures use.
func (h *halt) event() apitypes.StreamEvent {
	return apitypes.StreamEvent{
		Kind:       "error",
		BlockDelta: &apitypes.ContentBlockDelta{Kind: "text_delta", Text: h.message},
	}
}

// overBudget reports a halt once the session's estimated spend reaches
// MaxCostUSD. Unpriced models never trip it; the CLI warns about that up
// front rather than silently enforcing nothing.
func (r *ConversationRuntime) overBudget() *halt {
	if r.maxCost <= 0 {
		return nil
	}
	usd, _ := r.usage.Cost()
	if usd < r.maxCost {
		return nil
	}
	return &halt{
		reason: "budget_exceeded",
		message: fmt.Sprintf("Stopping: estimated session cost $%.4f has reached the --max-cost limit of $%.2f. "+
			"Raise the limit or start a new session to continue.", usd, r.maxCost),
	}
}

// Consecutive identical tool-call iterations before the model is told, and
// before the loop gives up. Weak models re-issue the same read or the same
// failing command until max-turns; both thresholds count iterations whose
// calls and results are byte-identical, so a command re-run after an edit,
// or a poll whose output changes, is never mistaken for a loop.
const (
	loopWarnAfter = 3
	loopStopAfter = 5
)

// loopGuard tracks how many iterations in a row have been identical.
type loopGuard struct {
	last    string
	repeats int
}

func (g *loopGuard) observe(fingerprint string) (warn, stop bool) {
	if fingerprint == g.last {
		g.repeats++
	} else {
		g.last = fingerprint
		g.repeats = 1
	}
	return g.repeats >= loopWarnAfter, g.repeats >= loopStopAfter
}

// runTools executes one turn's tool calls, appends their results to the
// session, and reports whether the loop should stop. When it does stop with
// calls outstanding, each gets an error result so the session stays valid:
// a tool_use with no tool_result is rejected by the API on the next request.
func (r *ConversationRuntime) runTools(pending []toolUseInfo) *halt {
	if h := r.overBudget(); h != nil {
		for _, tu := range pending {
			r.session = append(r.session, apitypes.UserToolResult(tu.id, "not executed: "+h.message, true))
		}
		return h
	}

	results := make([]apitypes.ToolResult, len(pending))
	var fp strings.Builder
	for i, tu := range pending {
		results[i] = r.executeTool(tu)
		fmt.Fprintf(&fp, "%s\x00%s\x00%s\x00", tu.name, tu.input, results[i].Output)
	}
	warn, stop := r.loop.observe(fp.String())

	for i, tu := range pending {
		out := results[i].Output
		if warn {
			out += fmt.Sprintf("\n\n[gocode] This exact tool call has now returned this exact result %d times in a row. "+
				"Repeating it will not change anything. Change approach, or stop and explain what is blocking you.", r.loop.repeats)
		}
		r.session = append(r.session, apitypes.UserToolResult(tu.id, out, results[i].IsError))
	}
	if stop {
		calls := "call"
		if len(pending) > 1 {
			calls = "set of tool calls"
		}
		return &halt{
			reason: "stuck",
			message: fmt.Sprintf("Stopping: the model repeated the same tool %s with the same result %d times in a row "+
				"and did not change approach after being told.", calls, r.loop.repeats),
		}
	}
	return nil
}

func (r *ConversationRuntime) executeTool(tu toolUseInfo) apitypes.ToolResult {
	inputMap, malformed := parseToolInput(tu.input)
	if malformed != "" {
		// Running a tool with empty arguments because the model's JSON did not
		// parse produces a misleading "path is required" style error. Tell the
		// model exactly what went wrong instead, so it can resend.
		r.toolCb.OnToolStart(tu.name, inputMap)
		r.toolCb.OnToolEnd(tu.name, false)
		return apitypes.ToolResult{ToolUseID: tu.id, IsError: true, Output: fmt.Sprintf(
			"Tool %s was not run: its arguments were not valid JSON (%s). "+
				"Resend the call with a single JSON object that matches the tool's input schema.", tu.name, malformed)}
	}
	inputStr, _ := json.Marshal(inputMap)

	// Notify UI that a tool is about to run (stops spinner, shows tool name)
	r.toolCb.OnToolStart(tu.name, inputMap)

	// Check permissions
	allowed, reason := r.permPolicy.Authorize(tu.name, string(inputStr))
	if !allowed {
		r.toolCb.OnToolEnd(tu.name, false)
		return apitypes.ToolResult{ToolUseID: tu.id, Output: reason, IsError: true}
	}

	// Pre-tool hook
	preResult := r.hooks.PreToolUse(tu.name, inputMap)
	if preResult.IsDenied() {
		r.toolCb.OnToolEnd(tu.name, false)
		return ToolResultFromHookDenial(tu.id, tu.name, preResult)
	}

	// Hook requested user confirmation
	if preResult.Escalate {
		escalateInput, _ := json.Marshal(inputMap)
		allowed, reason := r.permPolicy.Authorize(tu.name, string(escalateInput))
		if !allowed {
			r.toolCb.OnToolEnd(tu.name, false)
			return apitypes.ToolResult{ToolUseID: tu.id, Output: reason, IsError: true}
		}
	}

	// Apply updated input from hook
	if preResult.UpdatedInput != nil {
		inputMap = preResult.UpdatedInput
	}

	// Execute
	result := r.executor.Execute(tu.name, inputMap)
	r.toolCb.OnToolEnd(tu.name, !result.IsError)
	result.ToolUseID = tu.id
	result.Output = MergeHookFeedback(preResult.Messages, result.Output, false)

	// Post-tool hook
	postResult := r.hooks.PostToolUse(tu.name, inputMap, result.Output, result.IsError)
	if postResult.IsDenied() {
		result.IsError = true
	}
	result.Output = MergeHookFeedback(postResult.Messages, result.Output, postResult.IsDenied())

	return result
}

func (r *ConversationRuntime) buildRequest() apitypes.MessageRequest {
	return apitypes.MessageRequest{
		Model:     r.model,
		MaxTokens: r.maxTokens,
		Messages:  r.session,
		System:    r.systemPrompt,
		Tools:     r.executor.ListTools(),
		Stream:    false,
	}
}

// CompactSession keeps only the last N messages.
func (r *ConversationRuntime) CompactSession(preserveRecent int) {
	if preserveRecent >= len(r.session) {
		return
	}
	r.session = r.session[len(r.session)-preserveRecent:]
}

// EstimateSessionTokens returns a rough token count for the current session.
// Uses the heuristic: 1 token ≈ 4 characters. Tool results (block.Content)
// are usually the bulk of a coding session and must be counted, or the
// 85% compaction threshold in the REPL is reached long after the real
// context window is. Images are not counted: their token cost depends on
// pixel dimensions, not on the base64 length, which would overestimate by
// an order of magnitude.
func (r *ConversationRuntime) EstimateSessionTokens() int {
	totalChars := len(r.systemPrompt)
	for _, msg := range r.session {
		for _, block := range msg.Content {
			totalChars += len(block.Text)
			totalChars += len(block.Content)
			totalChars += len(block.Input)
			totalChars += len(block.Name) + len(block.ID) + len(block.ToolUseID)
		}
	}
	return totalChars / 4
}

// SummarizeAndReset compacts the session by asking the LLM to summarize,
// then starts fresh with just the summary as context.
func (r *ConversationRuntime) SummarizeAndReset(ctx context.Context) (string, error) {
	if len(r.session) < 4 {
		return "", nil
	}

	summaryPrompt := "Summarize the entire conversation so far in a concise paragraph. Include: what the user asked, what tools were used, what was accomplished, and any pending tasks. This summary will be used as context for a new session."

	r.session = append(r.session, apitypes.InputMessage{
		Role:    "user",
		Content: []apitypes.InputContentBlock{{Kind: "text", Text: summaryPrompt}},
	})

	resp, err := r.provider.SendMessage(ctx, r.buildRequest())
	if err != nil {
		// If even the summary request fails, do a hard reset
		r.session = nil
		return "Previous session was too large to summarize. Starting fresh.", nil
	}

	var summary string
	for _, block := range resp.Content {
		if block.Kind == "text" {
			summary += block.Text
		}
	}

	// Reset session with just the summary
	r.session = []apitypes.InputMessage{
		{
			Role: "user",
			Content: []apitypes.InputContentBlock{{Kind: "text",
				Text: "Here is a summary of our previous conversation:\n\n" + summary + "\n\nPlease continue from where we left off."}},
		},
		{
			Role: "assistant",
			Content: []apitypes.InputContentBlock{{Kind: "text",
				Text: "Got it, I have the context from our previous conversation. How can I help you next?"}},
		},
	}

	return summary, nil
}

// GetUsage returns the cumulative usage tracker.
func (r *ConversationRuntime) GetUsage() UsageTracker { return r.usage }

// GetToolCb returns the tool callback for external wiring (e.g., spinner integration).
func (r *ConversationRuntime) GetToolCb() ToolCallback { return r.toolCb }

// SetToolCb replaces the tool callback (e.g., for structured output collection).
func (r *ConversationRuntime) SetToolCb(cb ToolCallback) { r.toolCb = cb }

// GetSession returns the current conversation session.
func (r *ConversationRuntime) GetSession() []apitypes.InputMessage { return r.session }

// GetModel returns the model name configured for this runtime.
func (r *ConversationRuntime) GetModel() string { return r.model }

// RestoreSession replaces the current session with a saved one.
func (r *ConversationRuntime) RestoreSession(messages []apitypes.InputMessage) {
	r.session = messages
}

// parseToolInput decodes a tool call's arguments. malformed is non-empty
// when they are not a JSON object: either the raw bytes do not parse, or the
// OpenAI-compatible provider already caught that and wrapped the original
// text as {"raw": "..."}. No gocode tool takes a lone string parameter named
// raw, so that shape is unambiguous.
func parseToolInput(input json.RawMessage) (map[string]interface{}, string) {
	inputMap := make(map[string]interface{})
	if len(input) == 0 {
		return inputMap, ""
	}
	if err := json.Unmarshal(input, &inputMap); err != nil {
		return make(map[string]interface{}), fmt.Sprintf("%v; received %s", err, snippet(string(input)))
	}
	if inputMap == nil {
		return make(map[string]interface{}), ""
	}
	if raw, ok := inputMap["raw"].(string); ok && len(inputMap) == 1 && !json.Valid([]byte(raw)) {
		return make(map[string]interface{}), "received " + snippet(raw)
	}
	return inputMap, ""
}

func snippet(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		s = s[:199] + "…"
	}
	return fmt.Sprintf("%q", s)
}

func appendJSON(existing json.RawMessage, partial string) json.RawMessage {
	if len(existing) == 0 || string(existing) == "{}" {
		return json.RawMessage(partial)
	}
	return json.RawMessage(string(existing) + partial)
}

// servedModel is the model to attribute a turn to: the id the provider
// reports, which under a fallback chain may differ from the one requested,
// falling back to the requested id when the provider reports none.
func servedModel(reported, requested string) string {
	if reported != "" {
		return reported
	}
	return requested
}

// mergeUsage layers a message_delta usage over the message_start usage.
// Every counter the delta reports is a cumulative total for the turn, so a
// non-zero delta value replaces the base; a zero one means "not reported
// here" and the base is kept. Anthropic sends input tokens only in
// message_start and output tokens in both; OpenAI-compatible streams send
// everything in the final delta.
func mergeUsage(base, delta apitypes.Usage) apitypes.Usage {
	pick := func(b, d int) int {
		if d != 0 {
			return d
		}
		return b
	}
	return apitypes.Usage{
		InputTokens:              pick(base.InputTokens, delta.InputTokens),
		CacheCreationInputTokens: pick(base.CacheCreationInputTokens, delta.CacheCreationInputTokens),
		CacheReadInputTokens:     pick(base.CacheReadInputTokens, delta.CacheReadInputTokens),
		OutputTokens:             pick(base.OutputTokens, delta.OutputTokens),
	}
}
