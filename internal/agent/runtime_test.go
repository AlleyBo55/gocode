package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/AlleyBo55/gocode/internal/apiclient"
	"github.com/AlleyBo55/gocode/internal/apitypes"
)

// scriptedProvider answers successive calls from a queue. Each streaming call
// consumes one event script; each non-streaming call one response. It also
// records every request so tests can inspect the conversation the runtime
// built between turns.
type scriptedProvider struct {
	streams   [][]apitypes.StreamEvent
	responses []*apitypes.MessageResponse
	err       error
	requests  []apitypes.MessageRequest
}

func (p *scriptedProvider) Kind() apiclient.ProviderKind { return apiclient.ProviderAnthropic }

func (p *scriptedProvider) SendMessage(_ context.Context, req apitypes.MessageRequest) (*apitypes.MessageResponse, error) {
	p.requests = append(p.requests, req)
	if p.err != nil {
		return nil, p.err
	}
	if len(p.responses) == 0 {
		return nil, errors.New("scriptedProvider: no response queued")
	}
	r := p.responses[0]
	p.responses = p.responses[1:]
	return r, nil
}

func (p *scriptedProvider) StreamMessage(_ context.Context, req apitypes.MessageRequest) (<-chan apitypes.StreamEvent, error) {
	p.requests = append(p.requests, req)
	if p.err != nil {
		return nil, p.err
	}
	if len(p.streams) == 0 {
		return nil, errors.New("scriptedProvider: no stream queued")
	}
	events := p.streams[0]
	p.streams = p.streams[1:]
	ch := make(chan apitypes.StreamEvent, len(events))
	for _, ev := range events {
		ch <- ev
	}
	close(ch)
	return ch, nil
}

// anthropicTextTurn mirrors the wire shape in apiclient/testdata/anthropic_text.sse:
// input tokens in message_start, output tokens in message_delta.
func anthropicTextTurn(model, text string, in, out int) []apitypes.StreamEvent {
	return []apitypes.StreamEvent{
		{Kind: "message_start", Message: &apitypes.MessageResponse{ID: "msg_1", Model: model, Usage: apitypes.Usage{InputTokens: in, OutputTokens: 1, CacheReadInputTokens: 7}}},
		{Kind: "content_block_start", Index: 0, ContentBlock: &apitypes.OutputContentBlock{Kind: "text"}},
		{Kind: "content_block_delta", Index: 0, BlockDelta: &apitypes.ContentBlockDelta{Kind: "text_delta", Text: text}},
		{Kind: "content_block_stop", Index: 0},
		{Kind: "message_delta", Delta: &apitypes.DeltaPayload{StopReason: "end_turn"}, DeltaUsage: &apitypes.Usage{OutputTokens: out}},
		{Kind: "message_stop"},
	}
}

// anthropicToolTurn streams a tool_use block whose input arrives in the three
// partials Anthropic really sends: an empty one, then the JSON in pieces.
func anthropicToolTurn(model, toolID, tool string, partials []string) []apitypes.StreamEvent {
	events := []apitypes.StreamEvent{
		{Kind: "message_start", Message: &apitypes.MessageResponse{ID: "msg_t", Model: model, Usage: apitypes.Usage{InputTokens: 400, OutputTokens: 3}}},
		{Kind: "content_block_start", Index: 0, ContentBlock: &apitypes.OutputContentBlock{Kind: "text"}},
		{Kind: "content_block_delta", Index: 0, BlockDelta: &apitypes.ContentBlockDelta{Kind: "text_delta", Text: "Reading."}},
		{Kind: "content_block_stop", Index: 0},
		{Kind: "content_block_start", Index: 1, ContentBlock: &apitypes.OutputContentBlock{Kind: "tool_use", ID: toolID, Name: tool, Input: json.RawMessage("{}")}},
	}
	for _, p := range partials {
		events = append(events, apitypes.StreamEvent{Kind: "content_block_delta", Index: 1, BlockDelta: &apitypes.ContentBlockDelta{Kind: "input_json_delta", PartialJSON: p}})
	}
	return append(events,
		apitypes.StreamEvent{Kind: "content_block_stop", Index: 1},
		apitypes.StreamEvent{Kind: "message_delta", Delta: &apitypes.DeltaPayload{StopReason: "tool_use"}, DeltaUsage: &apitypes.Usage{OutputTokens: 60}},
		apitypes.StreamEvent{Kind: "message_stop"},
	)
}

// openaiTextTurn mirrors the OpenAI-compat translation: no usage at start,
// everything in the final message_delta.
func openaiTextTurn(model, text string, in, out int) []apitypes.StreamEvent {
	return []apitypes.StreamEvent{
		{Kind: "message_start", Message: &apitypes.MessageResponse{ID: "chatcmpl", Model: model}},
		{Kind: "content_block_start", Index: 0, ContentBlock: &apitypes.OutputContentBlock{Kind: "text"}},
		{Kind: "content_block_delta", Index: 0, BlockDelta: &apitypes.ContentBlockDelta{Kind: "text_delta", Text: text}},
		{Kind: "content_block_stop", Index: 0},
		{Kind: "message_delta", Delta: &apitypes.DeltaPayload{StopReason: "end_turn"}, DeltaUsage: &apitypes.Usage{InputTokens: in, OutputTokens: out}},
		{Kind: "message_stop"},
	}
}

func drain(t *testing.T, ch <-chan apitypes.StreamEvent) []apitypes.StreamEvent {
	t.Helper()
	var out []apitypes.StreamEvent
	timeout := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, ev)
		case <-timeout:
			t.Fatal("runtime stream did not close")
		}
	}
}

func eventKinds(events []apitypes.StreamEvent) []string {
	out := make([]string, len(events))
	for i, ev := range events {
		out[i] = ev.Kind
	}
	return out
}

func TestStreamTurnRecordsInputTokensFromMessageStart(t *testing.T) {
	p := &scriptedProvider{streams: [][]apitypes.StreamEvent{anthropicTextTurn("claude-sonnet-4-6", "Hello", 25, 15)}}
	rt := NewConversationRuntime(RuntimeOptions{Provider: p, Executor: NewStaticExecutor(), Model: "sonnet"})

	ch, err := rt.StreamUserMessage(context.Background(), "hi")
	if err != nil {
		t.Fatal(err)
	}
	events := drain(t, ch)
	if len(events) != 6 {
		t.Fatalf("expected the 6 provider events forwarded verbatim, got %v", eventKinds(events))
	}

	u := rt.GetUsage()
	if u.InputTokens != 25 || u.OutputTokens != 15 || u.CacheReadInputTokens != 7 || u.Turns != 1 {
		t.Errorf("usage = %+v; input and cache counters come from message_start, output from message_delta", u)
	}
	// Attributed to the model the provider served, not the alias requested.
	models := u.Models()
	if len(models) != 1 || models[0].Model != "claude-sonnet-4-6" {
		t.Errorf("ledger = %v, want claude-sonnet-4-6", names(models))
	}

	// The reply was appended to the session as an assistant text turn.
	session := rt.GetSession()
	if len(session) != 2 || session[1].Role != "assistant" || session[1].Content[0].Text != "Hello" {
		t.Errorf("session = %+v", session)
	}
	// Streaming requests must ask for the tools, the system prompt and the
	// requested model; the served model is only used for the ledger.
	if p.requests[0].Model != "sonnet" {
		t.Errorf("request model = %q", p.requests[0].Model)
	}
}

func TestStreamTurnOpenAIShapeIsAttributedAndCounted(t *testing.T) {
	p := &scriptedProvider{streams: [][]apitypes.StreamEvent{openaiTextTurn("gpt-4o-2024-08-06", "Hi", 12, 4)}}
	rt := NewConversationRuntime(RuntimeOptions{Provider: p, Executor: NewStaticExecutor(), Model: "gpt-4o"})
	ch, _ := rt.StreamUserMessage(context.Background(), "hi")
	drain(t, ch)
	u := rt.GetUsage()
	if u.InputTokens != 12 || u.OutputTokens != 4 || u.Models()[0].Model != "gpt-4o-2024-08-06" {
		t.Errorf("usage = %+v ledger = %v", u, names(u.Models()))
	}
}

func TestStreamToolLoopExecutesToolAndFeedsResultBack(t *testing.T) {
	var got map[string]interface{}
	exec := NewStaticExecutor().Register("read_file", func(in map[string]interface{}) apitypes.ToolResult {
		got = in
		return apitypes.ToolResult{Output: "module example.com"}
	})
	p := &scriptedProvider{streams: [][]apitypes.StreamEvent{
		anthropicToolTurn("claude-sonnet-4-6", "toolu_1", "read_file", []string{"", `{"path": "go.`, `mod"}`}),
		// A fallback chain could serve the second turn from another model;
		// the ledger must show both.
		openaiTextTurn("gpt-4o-mini", "It is example.com.", 500, 8),
	}}
	rt := NewConversationRuntime(RuntimeOptions{Provider: p, Executor: exec, Model: "sonnet", PermMode: DangerFullAccess})

	ch, err := rt.StreamUserMessage(context.Background(), "what module is this?")
	if err != nil {
		t.Fatal(err)
	}
	events := drain(t, ch)

	// Both turns' events reach the consumer in order.
	kinds := eventKinds(events)
	if len(kinds) != 11+6 || kinds[0] != "message_start" || kinds[11] != "message_start" {
		t.Fatalf("events = %v", kinds)
	}

	// The tool saw the reassembled JSON, not the raw partials.
	if !reflect.DeepEqual(got, map[string]interface{}{"path": "go.mod"}) {
		t.Errorf("tool input = %v", got)
	}

	// Session: user, assistant(text+tool_use), user(tool_result), assistant(text).
	s := rt.GetSession()
	if len(s) != 4 {
		t.Fatalf("session has %d messages: %+v", len(s), s)
	}
	if s[1].Role != "assistant" || len(s[1].Content) != 2 || s[1].Content[1].Kind != "tool_use" || s[1].Content[1].ID != "toolu_1" || string(s[1].Content[1].Input) != `{"path": "go.mod"}` {
		t.Errorf("assistant tool turn = %+v", s[1])
	}
	if s[2].Role != "user" || s[2].Content[0].Kind != "tool_result" || s[2].Content[0].ToolUseID != "toolu_1" || s[2].Content[0].Content != "module example.com" || s[2].Content[0].IsError {
		t.Errorf("tool result turn = %+v", s[2])
	}
	if s[3].Content[0].Text != "It is example.com." {
		t.Errorf("final turn = %+v", s[3])
	}
	// The second request carried the whole conversation so far.
	if len(p.requests) != 2 || len(p.requests[1].Messages) != 3 {
		t.Errorf("second request had %d messages, want 3", len(p.requests[1].Messages))
	}

	u := rt.GetUsage()
	if u.Turns != 2 || u.InputTokens != 900 || u.OutputTokens != 68 {
		t.Errorf("usage = %+v", u)
	}
	if got := names(u.Models()); !reflect.DeepEqual(got, []string{"claude-sonnet-4-6", "gpt-4o-mini"}) {
		t.Errorf("ledger = %v", got)
	}
}

func TestStreamToolLoopDeniedToolIsNotExecuted(t *testing.T) {
	executed := false
	exec := NewStaticExecutor().Register("bashtool", func(map[string]interface{}) apitypes.ToolResult {
		executed = true
		return apitypes.ToolResult{Output: "ran"}
	})
	p := &scriptedProvider{streams: [][]apitypes.StreamEvent{
		anthropicToolTurn("claude-sonnet-4-6", "toolu_2", "BashTool", []string{`{"command":"rm -rf /"}`}),
		anthropicTextTurn("claude-sonnet-4-6", "Understood.", 10, 2),
	}}
	rt := NewConversationRuntime(RuntimeOptions{
		Provider: p, Executor: exec, Model: "sonnet",
		PermMode: WorkspaceWrite, Prompter: &scriptedPrompter{allow: false},
	})
	ch, _ := rt.StreamUserMessage(context.Background(), "clean up")
	drain(t, ch)

	if executed {
		t.Fatal("denied tool must not run")
	}
	s := rt.GetSession()
	if len(s) != 4 || s[2].Content[0].Kind != "tool_result" || !s[2].Content[0].IsError || s[2].Content[0].Content != "permission denied by user" {
		t.Errorf("model should receive the denial as an error tool_result: %+v", s[2])
	}
}

func TestStreamProviderErrorIsSurfacedAsErrorEvent(t *testing.T) {
	p := &scriptedProvider{err: apitypes.NewMissingCredentials("Anthropic", "ANTHROPIC_API_KEY")}
	rt := NewConversationRuntime(RuntimeOptions{Provider: p, Executor: NewStaticExecutor(), Model: "sonnet"})
	ch, err := rt.StreamUserMessage(context.Background(), "hi")
	if err != nil {
		t.Fatalf("stream setup errors are delivered in-band, got %v", err)
	}
	events := drain(t, ch)
	if len(events) != 1 || events[0].Kind != "error" || events[0].BlockDelta == nil || events[0].BlockDelta.Text == "" {
		t.Fatalf("events = %+v", events)
	}
	if rt.GetUsage().Turns != 0 {
		t.Error("a failed turn must not be counted")
	}
}

func TestSendLoopAttributesToRequestedModelWhenProviderReportsNone(t *testing.T) {
	p := &scriptedProvider{responses: []*apitypes.MessageResponse{{
		Type: "message", Role: "assistant",
		Content:    []apitypes.OutputContentBlock{{Kind: "text", Text: "done"}},
		StopReason: "end_turn",
		Usage:      apitypes.Usage{InputTokens: 30, OutputTokens: 5},
	}}}
	rt := NewConversationRuntime(RuntimeOptions{Provider: p, Executor: NewStaticExecutor(), Model: "my-local-model"})
	resp, err := rt.SendUserMessage(context.Background(), "hi")
	if err != nil || resp.Content[0].Text != "done" {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
	u := rt.GetUsage()
	if u.Turns != 1 || u.InputTokens != 30 || u.Models()[0].Model != "my-local-model" {
		t.Errorf("usage = %+v ledger = %v", u, names(u.Models()))
	}
}

func TestSendLoopStopsAtMaxIterations(t *testing.T) {
	toolResp := &apitypes.MessageResponse{Role: "assistant", StopReason: "tool_use", Content: []apitypes.OutputContentBlock{
		{Kind: "tool_use", ID: "t", Name: "noop", Input: json.RawMessage(`{}`)},
	}}
	p := &scriptedProvider{responses: []*apitypes.MessageResponse{toolResp, toolResp, toolResp}}
	exec := NewStaticExecutor().Register("noop", func(map[string]interface{}) apitypes.ToolResult { return apitypes.ToolResult{Output: "ok"} })
	rt := NewConversationRuntime(RuntimeOptions{Provider: p, Executor: exec, Model: "m", MaxIterations: 2, PermMode: DangerFullAccess})

	resp, err := rt.SendUserMessage(context.Background(), "loop")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StopReason != "max_iterations" || len(p.requests) != 2 {
		t.Errorf("stop=%q requests=%d, want max_iterations after 2", resp.StopReason, len(p.requests))
	}
}

func TestMergeUsageKeepsBaseWhereDeltaIsSilent(t *testing.T) {
	base := apitypes.Usage{InputTokens: 25, OutputTokens: 1, CacheReadInputTokens: 7}
	got := mergeUsage(base, apitypes.Usage{OutputTokens: 15})
	want := apitypes.Usage{InputTokens: 25, OutputTokens: 15, CacheReadInputTokens: 7}
	if got != want {
		t.Errorf("anthropic-style merge = %+v, want %+v", got, want)
	}
	got = mergeUsage(apitypes.Usage{}, apitypes.Usage{InputTokens: 12, OutputTokens: 4})
	if got != (apitypes.Usage{InputTokens: 12, OutputTokens: 4}) {
		t.Errorf("openai-style merge = %+v", got)
	}
	if servedModel("", "req") != "req" || servedModel("served", "req") != "served" {
		t.Error("servedModel precedence")
	}
}

// repeatTurns returns n copies of the same tool turn: a model that never
// changes its mind.
func repeatTurns(n int, turn []apitypes.StreamEvent) [][]apitypes.StreamEvent {
	out := make([][]apitypes.StreamEvent, n)
	for i := range out {
		out[i] = turn
	}
	return out
}

func toolResults(session []apitypes.InputMessage) []apitypes.InputContentBlock {
	var out []apitypes.InputContentBlock
	for _, m := range session {
		for _, b := range m.Content {
			if b.Kind == "tool_result" {
				out = append(out, b)
			}
		}
	}
	return out
}

func TestLoopGuardWarnsThenStopsAnIdenticalToolLoop(t *testing.T) {
	calls := 0
	exec := NewStaticExecutor().Register("read_file", func(map[string]interface{}) apitypes.ToolResult {
		calls++
		return apitypes.ToolResult{Output: "same contents"}
	})
	turn := anthropicToolTurn("claude-sonnet-4-6", "toolu_x", "read_file", []string{`{"path":"a.go"}`})
	p := &scriptedProvider{streams: repeatTurns(30, turn)}
	rt := NewConversationRuntime(RuntimeOptions{Provider: p, Executor: exec, Model: "sonnet", PermMode: DangerFullAccess, MaxIterations: 30})

	ch, _ := rt.StreamUserMessage(context.Background(), "read a.go")
	events := drain(t, ch)

	if calls != loopStopAfter {
		t.Fatalf("tool ran %d times, want exactly %d (then halt), not the 30 max-turns", calls, loopStopAfter)
	}
	if len(p.requests) != loopStopAfter {
		t.Errorf("model was called %d times, want %d", len(p.requests), loopStopAfter)
	}
	last := events[len(events)-1]
	if last.Kind != "error" || last.BlockDelta == nil || !strings.Contains(last.BlockDelta.Text, "same result 5 times in a row") {
		t.Errorf("halt not surfaced to the user: %+v", last)
	}

	results := toolResults(rt.GetSession())
	if len(results) != loopStopAfter {
		t.Fatalf("tool results in session = %d", len(results))
	}
	// The first two identical results are delivered untouched; from the third
	// the model is told, in the result it reads, that it is looping.
	for i, r := range results {
		warned := strings.Contains(r.Content, "[gocode] This exact tool call")
		if want := i+1 >= loopWarnAfter; warned != want {
			t.Errorf("result %d warned=%v, want %v: %q", i+1, warned, want, r.Content)
		}
		if !strings.HasPrefix(r.Content, "same contents") {
			t.Errorf("result %d lost the real output: %q", i+1, r.Content)
		}
	}
}

func TestLoopGuardResetsWhenOutputChangesOrCallsDiffer(t *testing.T) {
	n := 0
	exec := NewStaticExecutor().Register("bashtool", func(map[string]interface{}) apitypes.ToolResult {
		n++
		return apitypes.ToolResult{Output: fmt.Sprintf("poll %d", n)} // changes every time: not a loop
	})
	turn := anthropicToolTurn("claude-sonnet-4-6", "toolu_p", "BashTool", []string{`{"command":"curl status"}`})
	streams := repeatTurns(8, turn)
	streams = append(streams, anthropicTextTurn("claude-sonnet-4-6", "Done.", 5, 2))
	p := &scriptedProvider{streams: streams}
	rt := NewConversationRuntime(RuntimeOptions{Provider: p, Executor: exec, Model: "sonnet", PermMode: DangerFullAccess, MaxIterations: 30})

	drain(t, mustStream(t, rt, "poll until ready"))
	if n != 8 {
		t.Errorf("a poll with changing output was cut short after %d calls", n)
	}
	for _, r := range toolResults(rt.GetSession()) {
		if strings.Contains(r.Content, "[gocode]") {
			t.Fatalf("warned on non-identical results: %q", r.Content)
		}
	}

	// Alternating edit/test iterations are the normal fix loop and must never
	// trip the guard even though each individual call recurs.
	execAlt := NewStaticExecutor().
		Register("edit", func(map[string]interface{}) apitypes.ToolResult { return apitypes.ToolResult{Output: "edited"} }).
		Register("test", func(map[string]interface{}) apitypes.ToolResult { return apitypes.ToolResult{Output: "FAIL"} })
	var alt [][]apitypes.StreamEvent
	for i := 0; i < 5; i++ {
		alt = append(alt,
			anthropicToolTurn("m", "e", "edit", []string{`{"path":"x"}`}),
			anthropicToolTurn("m", "t", "test", []string{`{"command":"go test"}`}))
	}
	alt = append(alt, anthropicTextTurn("m", "Fixed.", 5, 2))
	rt2 := NewConversationRuntime(RuntimeOptions{Provider: &scriptedProvider{streams: alt}, Executor: execAlt, Model: "m", PermMode: DangerFullAccess, MaxIterations: 30})
	events := drain(t, mustStream(t, rt2, "fix the test"))
	if hasErrorEvent(events) {
		t.Error("edit/test alternation was treated as a loop")
	}
	if len(rt2.GetSession()) != 1+2*10+1 {
		t.Errorf("session length = %d, want all 10 tool turns plus the final text", len(rt2.GetSession()))
	}
}

func TestLoopGuardAppliesToTheNonStreamingLoopToo(t *testing.T) {
	toolResp := &apitypes.MessageResponse{Role: "assistant", StopReason: "tool_use", Content: []apitypes.OutputContentBlock{
		{Kind: "tool_use", ID: "t", Name: "noop", Input: json.RawMessage(`{}`)},
	}}
	var resps []*apitypes.MessageResponse
	for i := 0; i < 30; i++ {
		resps = append(resps, toolResp)
	}
	exec := NewStaticExecutor().Register("noop", func(map[string]interface{}) apitypes.ToolResult { return apitypes.ToolResult{Output: "ok"} })
	p := &scriptedProvider{responses: resps}
	rt := NewConversationRuntime(RuntimeOptions{Provider: p, Executor: exec, Model: "m", MaxIterations: 30, PermMode: DangerFullAccess})

	resp, err := rt.SendUserMessage(context.Background(), "loop")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StopReason != "stuck" || len(p.requests) != loopStopAfter {
		t.Errorf("stop=%q requests=%d, want stuck after %d", resp.StopReason, len(p.requests), loopStopAfter)
	}
	if !strings.Contains(resp.Content[0].Text, "Stopping") {
		t.Errorf("message = %q", resp.Content[0].Text)
	}
}

func TestBudgetStopsBeforeRunningToolsAndKeepsSessionValid(t *testing.T) {
	executed := false
	exec := NewStaticExecutor().Register("read_file", func(map[string]interface{}) apitypes.ToolResult {
		executed = true
		return apitypes.ToolResult{Output: "x"}
	})
	// One turn on Sonnet with 1M input tokens costs $3; the budget is $1.
	turn := anthropicToolTurn("claude-sonnet-4-6", "toolu_b", "read_file", []string{`{"path":"a"}`})
	turn[0].Message.Usage = apitypes.Usage{InputTokens: 1_000_000}
	p := &scriptedProvider{streams: repeatTurns(3, turn)}
	rt := NewConversationRuntime(RuntimeOptions{Provider: p, Executor: exec, Model: "sonnet", PermMode: DangerFullAccess, MaxCostUSD: 1})

	events := drain(t, mustStream(t, rt, "go"))
	if executed {
		t.Fatal("tools must not run once the budget is exhausted")
	}
	if len(p.requests) != 1 {
		t.Errorf("model called %d times, want 1", len(p.requests))
	}
	last := events[len(events)-1]
	if last.Kind != "error" || !strings.Contains(last.BlockDelta.Text, "--max-cost limit of $1.00") || !strings.Contains(last.BlockDelta.Text, "cost $3.00") {
		t.Errorf("budget halt not surfaced: %+v", last)
	}
	// The dangling tool_use got an error result, so the transcript is still
	// acceptable to the API if the user continues.
	results := toolResults(rt.GetSession())
	if len(results) != 1 || !results[0].IsError || !strings.HasPrefix(results[0].Content, "not executed:") {
		t.Errorf("pending tool call was left unanswered: %+v", results)
	}

	// A follow-up message in the same session halts before spending anything.
	events = drain(t, mustStream(t, rt, "try again"))
	if len(p.requests) != 1 {
		t.Errorf("an exhausted session made another request (%d total)", len(p.requests))
	}
	if len(events) != 1 || events[0].Kind != "error" {
		t.Errorf("follow-up events = %v", eventKinds(events))
	}

	// Non-streaming path reports the same stop reason.
	rt2 := NewConversationRuntime(RuntimeOptions{Provider: &scriptedProvider{responses: []*apitypes.MessageResponse{{
		Role: "assistant", Model: "claude-sonnet-4-6", StopReason: "end_turn",
		Content: []apitypes.OutputContentBlock{{Kind: "text", Text: "hi"}},
		Usage:   apitypes.Usage{InputTokens: 1_000_000},
	}}}, Executor: exec, Model: "sonnet", MaxCostUSD: 1})
	if resp, _ := rt2.SendUserMessage(context.Background(), "a"); resp.StopReason != "end_turn" {
		t.Errorf("a turn with no tool calls completes even if it crossed the budget: %q", resp.StopReason)
	}
	if resp, _ := rt2.SendUserMessage(context.Background(), "b"); resp.StopReason != "budget_exceeded" {
		t.Errorf("next turn should halt: %q", resp.StopReason)
	}
}

func TestBudgetIsUnlimitedByDefaultAndForUnpricedModels(t *testing.T) {
	turn := anthropicTextTurn("my-finetune", "ok", 5_000_000, 10)
	rt := NewConversationRuntime(RuntimeOptions{Provider: &scriptedProvider{streams: repeatTurns(2, turn)}, Executor: NewStaticExecutor(), Model: "my-finetune", MaxCostUSD: 0.01})
	drain(t, mustStream(t, rt, "a"))
	if events := drain(t, mustStream(t, rt, "b")); hasErrorEvent(events) {
		t.Error("an unpriced model cannot be budgeted and must not be halted")
	}
	rt2 := NewConversationRuntime(RuntimeOptions{Provider: &scriptedProvider{streams: repeatTurns(2, anthropicTextTurn("claude-sonnet-4-6", "ok", 5_000_000, 10))}, Executor: NewStaticExecutor(), Model: "sonnet"})
	drain(t, mustStream(t, rt2, "a"))
	if events := drain(t, mustStream(t, rt2, "b")); hasErrorEvent(events) {
		t.Error("MaxCostUSD zero must mean unlimited")
	}
}

func TestMalformedToolArgumentsAreReportedNotExecuted(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"truncated json from a stream", `{"path": "a.g`, "unexpected end of JSON input"},
		{"provider raw wrapper", `{"raw":"{\"path\": a.go}"}`, `{\"path\": a.go}`},
		{"array instead of object", `["a.go"]`, "cannot unmarshal array"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			executed := false
			exec := NewStaticExecutor().Register("read_file", func(map[string]interface{}) apitypes.ToolResult {
				executed = true
				return apitypes.ToolResult{Output: "x"}
			})
			p := &scriptedProvider{streams: [][]apitypes.StreamEvent{
				anthropicToolTurn("m", "toolu_m", "read_file", []string{tc.input}),
				anthropicTextTurn("m", "Sorry, resending.", 5, 2),
			}}
			rt := NewConversationRuntime(RuntimeOptions{Provider: p, Executor: exec, Model: "m", PermMode: DangerFullAccess})
			drain(t, mustStream(t, rt, "read"))
			if executed {
				t.Fatal("tool ran with garbage arguments")
			}
			results := toolResults(rt.GetSession())
			if len(results) != 1 || !results[0].IsError {
				t.Fatalf("results = %+v", results)
			}
			for _, want := range []string{"was not run", "not valid JSON", tc.want, "Resend"} {
				if !strings.Contains(results[0].Content, want) {
					t.Errorf("error %q should mention %q", results[0].Content, want)
				}
			}
		})
	}

	// A genuine "raw" parameter carrying valid JSON is not the wrapper.
	got, malformed := parseToolInput(json.RawMessage(`{"raw":"{\"ok\":true}"}`))
	if malformed != "" || got["raw"] != `{"ok":true}` {
		t.Errorf("valid-JSON raw param misclassified: %v %q", got, malformed)
	}
	if got, malformed := parseToolInput(nil); malformed != "" || len(got) != 0 {
		t.Errorf("empty input: %v %q", got, malformed)
	}
	if got, malformed := parseToolInput(json.RawMessage(`null`)); malformed != "" || got == nil {
		t.Errorf("null input must yield an empty map: %v %q", got, malformed)
	}
}

func mustStream(t *testing.T, rt *ConversationRuntime, text string) <-chan apitypes.StreamEvent {
	t.Helper()
	ch, err := rt.StreamUserMessage(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	return ch
}

func hasErrorEvent(events []apitypes.StreamEvent) bool {
	for _, ev := range events {
		if ev.Kind == "error" {
			return true
		}
	}
	return false
}

func TestEstimateSessionTokensCountsToolResults(t *testing.T) {
	rt := NewConversationRuntime(RuntimeOptions{Provider: &scriptedProvider{}, Executor: NewStaticExecutor(), SystemPrompt: strings.Repeat("s", 400)})
	base := rt.EstimateSessionTokens()
	if base != 100 {
		t.Fatalf("system prompt alone = %d tokens, want 100", base)
	}
	// A 40KB tool result is ~10k tokens and was previously counted as zero.
	rt.RestoreSession([]apitypes.InputMessage{
		apitypes.UserText("read it"),
		{Role: "assistant", Content: []apitypes.InputContentBlock{{Kind: "tool_use", ID: "t", Name: "read_file", Input: json.RawMessage(`{"path":"a"}`)}}},
		apitypes.UserToolResult("t", strings.Repeat("x", 40_000), false),
	})
	got := rt.EstimateSessionTokens()
	if got < base+10_000 {
		t.Errorf("estimate = %d; tool result content is not being counted", got)
	}
	// Images are deliberately excluded: base64 length is not a token count.
	rt.RestoreSession([]apitypes.InputMessage{{Role: "user", Content: []apitypes.InputContentBlock{
		{Kind: "image", Source: &apitypes.ImageSource{Type: "base64", MediaType: "image/png", Data: strings.Repeat("A", 200_000)}},
	}}})
	if got := rt.EstimateSessionTokens(); got != base {
		t.Errorf("image counted as %d tokens; base64 length must not be used", got-base)
	}
}

func TestInboxIsDeliveredBeforeEachTurnWithoutBreakingToolPairing(t *testing.T) {
	// One message waits before the first turn; another arrives while the tool
	// runs. Each must reach the model on the next request, and the tool_result
	// must still directly follow its tool_use.
	exec := NewStaticExecutor().Register("read_file", func(map[string]interface{}) apitypes.ToolResult {
		return apitypes.ToolResult{Output: "contents"}
	})
	queue := []string{"[message from planner-1]\nplan is ready"}
	afterTool := false
	inbox := func() []string {
		out := queue
		queue = nil
		if afterTool {
			afterTool = false
			return append(out, "[message from main]\nhurry up")
		}
		return out
	}
	p := &scriptedProvider{streams: [][]apitypes.StreamEvent{
		anthropicToolTurn("m", "toolu_1", "read_file", []string{`{"path":"a"}`}),
		anthropicTextTurn("m", "Done.", 5, 2),
	}}
	rt := NewConversationRuntime(RuntimeOptions{Provider: p, Executor: exec, Model: "m", PermMode: DangerFullAccess, Inbox: inbox})
	// The second message "arrives" once the tool has produced its result.
	exec.Register("read_file", func(map[string]interface{}) apitypes.ToolResult {
		afterTool = true
		return apitypes.ToolResult{Output: "contents"}
	})

	drain(t, mustStream(t, rt, "go"))

	first := p.requests[0].Messages
	if len(first) != 2 || first[0].Content[0].Text != "go" || first[1].Role != "user" || !strings.Contains(first[1].Content[0].Text, "plan is ready") {
		t.Errorf("first request should carry the prompt then the waiting message: %+v", first)
	}
	second := p.requests[1].Messages
	// prompt, inbox, assistant(tool_use), tool_result, inbox
	if len(second) != 5 {
		t.Fatalf("second request has %d messages: %+v", len(second), second)
	}
	if second[2].Role != "assistant" || second[3].Content[0].Kind != "tool_result" || second[3].Content[0].ToolUseID != "toolu_1" {
		t.Errorf("tool_result must directly follow tool_use: %+v", second[2:4])
	}
	if second[4].Role != "user" || !strings.Contains(second[4].Content[0].Text, "hurry up") {
		t.Errorf("message that arrived during the tool call not delivered: %+v", second[4])
	}
}

func TestNoInboxMeansNoExtraMessages(t *testing.T) {
	p := &scriptedProvider{streams: [][]apitypes.StreamEvent{anthropicTextTurn("m", "hi", 1, 1)}}
	rt := NewConversationRuntime(RuntimeOptions{Provider: p, Executor: NewStaticExecutor(), Model: "m"})
	drain(t, mustStream(t, rt, "hello"))
	if len(p.requests[0].Messages) != 1 {
		t.Errorf("without an inbox the request is just the prompt: %+v", p.requests[0].Messages)
	}
	empty := func() []string { return nil }
	rt2 := NewConversationRuntime(RuntimeOptions{Provider: &scriptedProvider{streams: [][]apitypes.StreamEvent{anthropicTextTurn("m", "hi", 1, 1)}}, Executor: NewStaticExecutor(), Model: "m", Inbox: empty})
	drain(t, mustStream(t, rt2, "hello"))
	if len(rt2.GetSession()) != 2 {
		t.Errorf("an empty inbox must add nothing: %+v", rt2.GetSession())
	}
}
