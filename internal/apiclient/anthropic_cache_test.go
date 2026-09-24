package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/AlleyBo55/gocode/internal/apitypes"
)

func conversation() []apitypes.InputMessage {
	return []apitypes.InputMessage{
		apitypes.UserText("read go.mod"),
		{Role: "assistant", Content: []apitypes.InputContentBlock{
			{Kind: "tool_use", ID: "t1", Name: "read_file", Input: json.RawMessage(`{"path":"go.mod"}`)},
		}},
		apitypes.UserToolResult("t1", "module example.com", false),
	}
}

func TestAnthropicWireRequestPlacesExactlyTwoBreakpoints(t *testing.T) {
	req := apitypes.MessageRequest{
		Model: "claude-sonnet-4-6", MaxTokens: 100, System: "You are terse.",
		Tools:    []apitypes.ToolDef{{Name: "read_file", InputSchema: json.RawMessage(`{"type":"object"}`)}},
		Messages: conversation(),
	}
	wire := anthropicWireRequest(req, true)

	// System becomes a block list so the marker has somewhere to live; the
	// marker there also covers the tools, which precede system in the cache.
	if len(wire.System) != 1 || wire.System[0].Text != "You are terse." || wire.System[0].CacheControl == nil {
		t.Fatalf("system = %+v", wire.System)
	}
	if len(wire.Tools) != 1 || wire.Tools[0].Name != "read_file" {
		t.Errorf("tools altered: %+v", wire.Tools)
	}

	markers := 0
	for i, m := range wire.Messages {
		for j, b := range m.Content {
			if b.CacheControl != nil {
				markers++
				if i != len(wire.Messages)-1 || j != len(m.Content)-1 {
					t.Errorf("marker on message %d block %d; only the final block may carry one", i, j)
				}
			}
		}
	}
	if markers != 1 {
		t.Errorf("message markers = %d, want 1", markers)
	}
	if got := wire.Messages[2].Content[0]; got.Kind != "tool_result" || got.Content != "module example.com" {
		t.Errorf("last block content altered: %+v", got)
	}
}

func TestAnthropicWireRequestNeverMutatesTheSession(t *testing.T) {
	// The runtime passes its live session slice. A marker written through to
	// it would persist and add a breakpoint every turn until Anthropic
	// rejects the request.
	session := conversation()
	req := apitypes.MessageRequest{Model: "m", System: "s", Messages: session}
	for turn := 0; turn < 6; turn++ {
		wire := anthropicWireRequest(req, true)
		total := 0
		for _, m := range wire.Messages {
			for _, b := range m.Content {
				if b.CacheControl != nil {
					total++
				}
			}
		}
		if total != 1 {
			t.Fatalf("turn %d: %d message breakpoints on the wire, want 1", turn, total)
		}
		for _, m := range session {
			for _, b := range m.Content {
				if b.CacheControl != nil {
					t.Fatalf("turn %d: session block was mutated: %+v", turn, b)
				}
			}
		}
		// The conversation grows between turns, as it does in the agent loop.
		session = append(session, apitypes.UserText("next"))
		req.Messages = session
	}
}

func TestAnthropicWireRequestStripsStrayMarkers(t *testing.T) {
	msgs := conversation()
	msgs[0].Content[0].CacheControl = apitypes.EphemeralCache() // a marker that leaked into storage somehow
	wire := anthropicWireRequest(apitypes.MessageRequest{Messages: msgs}, true)
	if wire.Messages[0].Content[0].CacheControl != nil {
		t.Error("stale marker on an early message must be removed, or the breakpoint budget is spent on it")
	}
	if wire.Messages[2].Content[0].CacheControl == nil {
		t.Error("final block still gets the marker")
	}
}

func TestAnthropicWireRequestDisabledSendsPlainShapes(t *testing.T) {
	req := apitypes.MessageRequest{Model: "m", System: "s", Messages: conversation()}
	wire := anthropicWireRequest(req, false)
	if len(wire.System) != 1 || wire.System[0].CacheControl != nil {
		t.Errorf("system = %+v", wire.System)
	}
	for _, m := range wire.Messages {
		for _, b := range m.Content {
			if b.CacheControl != nil {
				t.Errorf("marker present with caching disabled: %+v", b)
			}
		}
	}
	empty := anthropicWireRequest(apitypes.MessageRequest{Model: "m"}, true)
	if empty.System != nil || empty.Messages != nil {
		t.Errorf("empty request should stay empty: %+v", empty)
	}
	if data, _ := json.Marshal(empty); string(data) != `{"model":"m","max_tokens":0,"messages":null}` {
		t.Errorf("empty wire JSON = %s", data)
	}
}

func TestAnthropicProviderSendsCacheControlOnTheWire(t *testing.T) {
	rec := &recorder{}
	srv := newServer(t, rec, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, `{"id":"m","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"model":"claude-sonnet-4-6","usage":{"input_tokens":5,"output_tokens":1,"cache_read_input_tokens":1200}}`)
	})
	p := newAnthropic(t, srv)
	req := apitypes.MessageRequest{Model: "claude-sonnet-4-6", MaxTokens: 10, System: "sys", Messages: conversation()}

	t.Setenv(promptCacheDisableEnv, "")
	resp, err := p.SendMessage(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	// The cache read count comes back through the usual usage path, so /cost
	// prices it at the discounted rate without any further plumbing.
	if resp.Usage.CacheReadInputTokens != 1200 {
		t.Errorf("cache_read_input_tokens = %d", resp.Usage.CacheReadInputTokens)
	}

	var sent struct {
		System   []map[string]any `json:"system"`
		Messages []map[string]any `json:"messages"`
	}
	if err := json.Unmarshal(rec.last(t).Body, &sent); err != nil {
		t.Fatal(err)
	}
	if len(sent.System) != 1 || sent.System[0]["cache_control"] == nil {
		t.Errorf("system on the wire = %v", sent.System)
	}
	last := sent.Messages[len(sent.Messages)-1]["content"].([]any)
	if last[len(last)-1].(map[string]any)["cache_control"] == nil {
		t.Errorf("final message block lacks cache_control: %v", last)
	}
	if sent.Messages[0]["content"].([]any)[0].(map[string]any)["cache_control"] != nil {
		t.Error("first message must not carry a marker")
	}

	// Opt-out for endpoints that reject the field.
	t.Setenv(promptCacheDisableEnv, "1")
	if _, err := p.SendMessage(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	var plain struct {
		System   []map[string]any `json:"system"`
		Messages []map[string]any `json:"messages"`
	}
	if err := json.Unmarshal(rec.last(t).Body, &plain); err != nil {
		t.Fatal(err)
	}
	if plain.System[0]["cache_control"] != nil {
		t.Error("GOCODE_DISABLE_PROMPT_CACHE did not disable the system marker")
	}
	for _, m := range plain.Messages {
		for _, b := range m["content"].([]any) {
			if b.(map[string]any)["cache_control"] != nil {
				t.Errorf("marker sent with caching disabled: %v", b)
			}
		}
	}
	// System is still sent as a block list either way; Anthropic accepts both.
	if plain.System[0]["text"] != "sys" {
		t.Errorf("system text = %v", plain.System[0])
	}
}

func TestOpenAICompatIgnoresCacheControl(t *testing.T) {
	// The field is Anthropic-only; the chat-completions translation must not
	// leak it into a message shape other providers will reject.
	msgs := conversation()
	msgs[2].Content[0].CacheControl = apitypes.EphemeralCache()
	payload := buildChatCompletionRequest(apitypes.MessageRequest{Model: "gpt-4o", Messages: msgs})
	data, _ := json.Marshal(payload)
	if json.Valid(data) && containsKey(payload, "cache_control") {
		t.Errorf("cache_control leaked into the OpenAI payload: %s", data)
	}
}

func containsKey(v any, key string) bool {
	switch x := v.(type) {
	case map[string]any:
		for k, vv := range x {
			if k == key || containsKey(vv, key) {
				return true
			}
		}
	case []map[string]any:
		for _, m := range x {
			if containsKey(m, key) {
				return true
			}
		}
	case []any:
		for _, e := range x {
			if containsKey(e, key) {
				return true
			}
		}
	}
	return false
}
