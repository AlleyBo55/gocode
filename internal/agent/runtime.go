package agent

import (
	"context"
	"encoding/json"
	"fmt"

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
	Hooks         HookRunner
	ToolCb        ToolCallback
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
		permPolicy:   PermissionPolicy{Mode: opts.PermMode, Prompter: prompter},
		hooks:        hooks,
		toolCb:       toolCb,
	}
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
	for iteration := 0; iteration < r.maxIter; iteration++ {
		req := r.buildRequest()
		resp, err := r.provider.SendMessage(ctx, req)
		if err != nil {
			return nil, err
		}
		r.usage.Add(resp.Usage)

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

		// Execute each tool
		for _, tu := range pendingTools {
			result := r.executeTool(tu)
			r.session = append(r.session, apitypes.UserToolResult(tu.id, result.Output, result.IsError))
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
	go func() {
		defer close(outCh)
		for iteration := 0; iteration < r.maxIter; iteration++ {
			req := r.buildRequest()
			eventCh, err := r.provider.StreamMessage(ctx, req)
			if err != nil {
				return
			}

			// Collect the full response while forwarding events
			var contentBlocks []apitypes.OutputContentBlock
			var pendingTools []toolUseInfo
			var currentUsage apitypes.Usage

			for ev := range eventCh {
				select {
				case outCh <- ev:
				case <-ctx.Done():
					return
				}
				// Track content blocks from stream events
				switch ev.Kind {
				case "content_block_start":
					if ev.ContentBlock != nil {
						contentBlocks = append(contentBlocks, *ev.ContentBlock)
					}
				case "content_block_delta":
					if ev.BlockDelta != nil && len(contentBlocks) > 0 {
						last := &contentBlocks[len(contentBlocks)-1]
						switch ev.BlockDelta.Kind {
						case "text_delta":
							last.Text += ev.BlockDelta.Text
						case "input_json_delta":
							last.Input = appendJSON(last.Input, ev.BlockDelta.PartialJSON)
						}
					}
				case "message_delta":
					if ev.DeltaUsage != nil {
						currentUsage = *ev.DeltaUsage
					}
				}
			}

			r.usage.Add(currentUsage)

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

			for _, tu := range pendingTools {
				result := r.executeTool(tu)
				r.session = append(r.session, apitypes.UserToolResult(tu.id, result.Output, result.IsError))
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

func (r *ConversationRuntime) executeTool(tu toolUseInfo) apitypes.ToolResult {
	// Parse input
	var inputMap map[string]interface{}
	if len(tu.input) > 0 {
		_ = json.Unmarshal(tu.input, &inputMap)
	}
	if inputMap == nil {
		inputMap = make(map[string]interface{})
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

// GetUsage returns the cumulative usage tracker.
func (r *ConversationRuntime) GetUsage() UsageTracker { return r.usage }

// GetToolCb returns the tool callback for external wiring (e.g., spinner integration).
func (r *ConversationRuntime) GetToolCb() ToolCallback { return r.toolCb }

// GetSession returns the current conversation session.
func (r *ConversationRuntime) GetSession() []apitypes.InputMessage { return r.session }

// GetModel returns the model name configured for this runtime.
func (r *ConversationRuntime) GetModel() string { return r.model }

// RestoreSession replaces the current session with a saved one.
func (r *ConversationRuntime) RestoreSession(messages []apitypes.InputMessage) {
	r.session = messages
}

func appendJSON(existing json.RawMessage, partial string) json.RawMessage {
	if len(existing) == 0 {
		return json.RawMessage(partial)
	}
	return json.RawMessage(string(existing) + partial)
}
