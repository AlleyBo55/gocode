package apiclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/AlleyBo55/gocode/internal/apitypes"
)

func newCompat(t *testing.T, srv *httptest.Server, name string) *OpenAiCompatProvider {
	t.Helper()
	p := NewOpenAiCompatProvider(OpenAiCompatConfig{ProviderName: name, BaseURLEnv: "GOCODE_TEST_UNSET_BASE_URL", DefaultBase: srv.URL}, apitypes.AuthApiKey("sk-test"))
	p.BaseURL = srv.URL
	p.Retry = fastRetry()
	return p
}

func sentPayload(t *testing.T, rec *recorder) map[string]any {
	t.Helper()
	var sent map[string]any
	if err := json.Unmarshal(rec.last(t).Body, &sent); err != nil {
		t.Fatalf("request body is not JSON: %v", err)
	}
	return sent
}

func TestOpenAiStreamTextFixtureEndToEnd(t *testing.T) {
	body := fixture(t, "openai_text.sse")
	rec := &recorder{}
	srv := newServer(t, rec, func(w http.ResponseWriter, r *http.Request) { writeStream(w, body, 7) })

	ch, err := newCompat(t, srv, "OpenAI").StreamMessage(context.Background(), userRequest("gpt-4o"))
	if err != nil {
		t.Fatal(err)
	}
	events := collect(t, ch)

	req := rec.last(t)
	if req.Path != "/chat/completions" {
		t.Errorf("path = %s", req.Path)
	}
	if req.Header.Get("Authorization") != "Bearer sk-test" {
		t.Errorf("auth header = %q", req.Header.Get("Authorization"))
	}
	sent := sentPayload(t, rec)
	if sent["stream"] != true {
		t.Error("stream flag missing")
	}
	// Without this the server omits usage from the stream and every streamed
	// turn is recorded as zero tokens.
	opts, _ := sent["stream_options"].(map[string]any)
	if opts["include_usage"] != true {
		t.Errorf("stream_options.include_usage not requested: %v", sent["stream_options"])
	}

	want := []string{"message_start", "content_block_start", "content_block_delta", "content_block_delta", "content_block_stop", "message_delta", "message_stop"}
	if got := kinds(events); !reflect.DeepEqual(got, want) {
		t.Fatalf("kinds = %v, want %v", got, want)
	}
	if start := events[0]; start.Message.ID != "chatcmpl-abc" || start.Message.Model != "gpt-4o-2024-08-06" {
		t.Errorf("message_start = %+v", start.Message)
	}
	if got := textOf(events, 0); got != "Hello, world!" {
		t.Errorf("text = %q", got)
	}
	delta := findKind(events, "message_delta")
	if delta.Delta.StopReason != "end_turn" {
		t.Errorf("stop_reason = %q, want end_turn (normalised from stop)", delta.Delta.StopReason)
	}
	// The usage-only trailing chunk is what stream_options buys us.
	if delta.DeltaUsage == nil || delta.DeltaUsage.InputTokens != 12 || delta.DeltaUsage.OutputTokens != 4 {
		t.Errorf("usage = %+v, want 12/4", delta.DeltaUsage)
	}
}

func TestOpenAiStreamToolCallsFixtureEndToEnd(t *testing.T) {
	body := fixture(t, "openai_tool_calls.sse")
	srv := newServer(t, &recorder{}, func(w http.ResponseWriter, r *http.Request) { writeStream(w, body, 11) })
	ch, err := newCompat(t, srv, "OpenAI").StreamMessage(context.Background(), userRequest("gpt-4o"))
	if err != nil {
		t.Fatal(err)
	}
	events := collect(t, ch)

	// OpenAI tool call index 0 becomes content block 1; block 0 is reserved
	// for text so a reply that mixes both keeps stable indices.
	want := []string{"message_start", "content_block_start", "content_block_delta", "content_block_delta", "content_block_stop", "message_delta", "message_stop"}
	if got := kinds(events); !reflect.DeepEqual(got, want) {
		t.Fatalf("kinds = %v, want %v", got, want)
	}
	tool := blockStart(events, 1)
	if tool == nil || tool.Kind != "tool_use" || tool.ID != "call_abc123" || tool.Name != "read_file" {
		t.Fatalf("tool block = %+v", tool)
	}
	if got := toolInputOf(events, 1); got != `{"path": "go.mod"}` {
		t.Errorf("arguments reassembled = %q", got)
	}
	if blockStart(events, 0) != nil {
		t.Error("no text block should be started when the model emitted none")
	}
	delta := findKind(events, "message_delta")
	if delta.Delta.StopReason != "tool_use" || delta.DeltaUsage.InputTokens != 40 || delta.DeltaUsage.OutputTokens != 12 {
		t.Errorf("message_delta = %+v / %+v", delta.Delta, delta.DeltaUsage)
	}
}

func TestOpenAiStreamParallelToolCallsGetDistinctBlocks(t *testing.T) {
	body := fixture(t, "openai_parallel_tool_calls.sse")
	srv := newServer(t, &recorder{}, func(w http.ResponseWriter, r *http.Request) { writeStream(w, body, 4096) })
	ch, err := newCompat(t, srv, "OpenAI").StreamMessage(context.Background(), userRequest("gpt-4o"))
	if err != nil {
		t.Fatal(err)
	}
	events := collect(t, ch)

	a, b := blockStart(events, 1), blockStart(events, 2)
	if a == nil || a.ID != "call_a" || b == nil || b.ID != "call_b" {
		t.Fatalf("blocks: %+v / %+v", a, b)
	}
	if toolInputOf(events, 1) != `{"path":"a.go"}` || toolInputOf(events, 2) != `{"path":"b.go"}` {
		t.Errorf("arguments crossed streams: %q / %q", toolInputOf(events, 1), toolInputOf(events, 2))
	}
	// Both blocks are closed exactly once. Order between them is not
	// specified (map iteration), so compare as a set.
	var stops []int
	for _, ev := range events {
		if ev.Kind == "content_block_stop" {
			stops = append(stops, ev.Index)
		}
	}
	sort.Ints(stops)
	if !reflect.DeepEqual(stops, []int{1, 2}) {
		t.Errorf("content_block_stop indices = %v, want [1 2]", stops)
	}
}

func TestOpenAiStreamSurfacesMalformedChunkAndDoesNotClaimEndTurn(t *testing.T) {
	body := fixture(t, "openai_text.sse")
	cut := strings.Index(string(body), `data: {"id":"chatcmpl-abc","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o-2024-08-06","system_fingerprint":"fp_test","choices":[{"index":0,"delta":{"content":", world!"}`)
	broken := string(body[:cut]) + "data: {\"id\":\"chatcmpl-abc\",\"choices\":[{\"delta\":{\"content\":\n\n"
	srv := newServer(t, &recorder{}, func(w http.ResponseWriter, r *http.Request) { writeStream(w, []byte(broken), 4096) })

	ch, err := newCompat(t, srv, "OpenAI").StreamMessage(context.Background(), userRequest("gpt-4o"))
	if err != nil {
		t.Fatal(err)
	}
	events := collect(t, ch)
	if got := textOf(events, 0); got != "Hello" {
		t.Errorf("text delivered before the bad chunk = %q", got)
	}
	last := events[len(events)-1]
	if last.Kind != "error" || !strings.Contains(last.BlockDelta.Text, "stream interrupted") {
		t.Fatalf("last event = %+v, want a surfaced error", last)
	}
	// A truncated reply must not be finalised as a normal end_turn.
	if hasKind(events, "message_delta") || hasKind(events, "message_stop") {
		t.Errorf("truncated stream was finalised: %v", kinds(events))
	}
}

func TestOpenAiStreamSurfacesDroppedConnection(t *testing.T) {
	body := fixture(t, "openai_text.sse")
	cut := strings.Index(string(body), `data: {"id":"chatcmpl-abc","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o-2024-08-06","system_fingerprint":"fp_test","choices":[{"index":0,"delta":{"content":", world!"}`)
	srv := newServer(t, &recorder{}, func(w http.ResponseWriter, r *http.Request) { writeStreamThenDrop(w, body[:cut]) })

	ch, err := newCompat(t, srv, "OpenAI").StreamMessage(context.Background(), userRequest("gpt-4o"))
	if err != nil {
		t.Fatal(err)
	}
	events := collect(t, ch)
	last := events[len(events)-1]
	if last.Kind != "error" || !strings.Contains(last.BlockDelta.Text, "stream interrupted") {
		t.Fatalf("dropped connection not surfaced; kinds = %v", kinds(events))
	}
	if hasKind(events, "message_stop") {
		t.Error("dropped stream must not be finalised")
	}
}

func TestOpenAiRetriesThenSucceedsAndBubblesContextLengthCode(t *testing.T) {
	body := fixture(t, "openai_text.sse")
	var n int32
	rec := &recorder{}
	srv := newServer(t, rec, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&n, 1) == 1 {
			writeJSON(w, http.StatusServiceUnavailable, `{"error":{"message":"overloaded","type":"server_error","code":null}}`)
			return
		}
		writeStream(w, body, 4096)
	})
	ch, err := newCompat(t, srv, "OpenAI").StreamMessage(context.Background(), userRequest("gpt-4o"))
	if err != nil {
		t.Fatalf("expected success on retry: %v", err)
	}
	collect(t, ch)
	if rec.count() != 2 {
		t.Errorf("attempts = %d, want 2", rec.count())
	}

	rec2 := &recorder{}
	srv2 := newServer(t, rec2, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusBadRequest, `{"error":{"message":"This model's maximum context length is 128000 tokens.","type":"invalid_request_error","param":"messages","code":"context_length_exceeded"}}`)
	})
	_, err = newCompat(t, srv2, "OpenAI").SendMessage(context.Background(), userRequest("gpt-4o"))
	var apiErr *apitypes.ApiError
	if !errors.As(err, &apiErr) || apiErr.Code != "context_length_exceeded" {
		t.Fatalf("code not preserved through the provider: %v", err)
	}
	if rec2.count() != 1 {
		t.Errorf("400 retried %d times", rec2.count())
	}
	if !isFallbackRetryable(err) {
		t.Error("context_length_exceeded from OpenAI must trigger fallback")
	}
}

func TestOpenAiSendMessageNormalisesChatCompletion(t *testing.T) {
	rec := &recorder{}
	srv := newServer(t, rec, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-request-id", "req_xyz")
		writeJSON(w, http.StatusOK, `{"id":"chatcmpl-x","object":"chat.completion","created":1,"model":"gpt-4o-2024-08-06","choices":[{"index":0,"message":{"role":"assistant","content":"Reading it now.","tool_calls":[{"id":"call_1","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"go.mod\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":30,"completion_tokens":9,"total_tokens":39}}`)
	})
	resp, err := newCompat(t, srv, "OpenAI").SendMessage(context.Background(), userRequest("gpt-4o"))
	if err != nil {
		t.Fatal(err)
	}
	sent := sentPayload(t, rec)
	if sent["stream"] != false {
		t.Errorf("stream = %v", sent["stream"])
	}
	if _, ok := sent["stream_options"]; ok {
		t.Error("stream_options must only be sent for streaming requests")
	}

	if resp.RequestID != "req_xyz" || resp.Model != "gpt-4o-2024-08-06" || resp.StopReason != "tool_use" {
		t.Errorf("header fields = %+v", resp)
	}
	if len(resp.Content) != 2 || resp.Content[0].Text != "Reading it now." {
		t.Fatalf("content = %+v", resp.Content)
	}
	tool := resp.Content[1]
	if tool.Kind != "tool_use" || tool.ID != "call_1" || tool.Name != "read_file" || string(tool.Input) != `{"path":"go.mod"}` {
		t.Errorf("tool_use = %+v", tool)
	}
	if resp.Usage.InputTokens != 30 || resp.Usage.OutputTokens != 9 {
		t.Errorf("usage = %+v", resp.Usage)
	}
}

func TestOpenAiSendMessageRejectsResponseWithoutChoices(t *testing.T) {
	srv := newServer(t, &recorder{}, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, `{"id":"chatcmpl-x","choices":[]}`)
	})
	_, err := newCompat(t, srv, "OpenAI").SendMessage(context.Background(), userRequest("gpt-4o"))
	if err == nil || !strings.Contains(err.Error(), "missing choices") {
		t.Fatalf("got %v", err)
	}
}

func TestBuildChatCompletionRequestTranslation(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`)
	req := apitypes.MessageRequest{
		Model:     "gpt-4o",
		MaxTokens: 512,
		System:    "You are terse.",
		Messages: []apitypes.InputMessage{
			apitypes.UserText("read go.mod"),
			{Role: "assistant", Content: []apitypes.InputContentBlock{
				{Kind: "text", Text: "Sure."},
				{Kind: "tool_use", ID: "call_1", Name: "read_file", Input: json.RawMessage(`{"path":"go.mod"}`)},
			}},
			apitypes.UserToolResult("call_1", "module example.com", false),
		},
		Tools:      []apitypes.ToolDef{{Name: "read_file", Description: "Read a file", InputSchema: schema}},
		ToolChoice: &apitypes.ToolChoice{Kind: "any"},
		Stream:     true,
	}
	p := buildChatCompletionRequest(req)

	msgs := p["messages"].([]map[string]any)
	if len(msgs) != 4 {
		t.Fatalf("messages = %d, want system+user+assistant+tool", len(msgs))
	}
	if msgs[0]["role"] != "system" || msgs[0]["content"] != "You are terse." {
		t.Errorf("system message = %v", msgs[0])
	}
	if msgs[1]["role"] != "user" || msgs[1]["content"] != "read go.mod" {
		t.Errorf("user message = %v", msgs[1])
	}
	calls := msgs[2]["tool_calls"].([]map[string]any)
	fn := calls[0]["function"].(map[string]any)
	if msgs[2]["content"] != "Sure." || calls[0]["id"] != "call_1" || fn["name"] != "read_file" || fn["arguments"] != `{"path":"go.mod"}` {
		t.Errorf("assistant message = %v", msgs[2])
	}
	if msgs[3]["role"] != "tool" || msgs[3]["tool_call_id"] != "call_1" || msgs[3]["content"] != "module example.com" {
		t.Errorf("tool result = %v", msgs[3])
	}

	tools := p["tools"].([]map[string]any)
	tf := tools[0]["function"].(map[string]any)
	if tools[0]["type"] != "function" || tf["name"] != "read_file" {
		t.Errorf("tool def = %v", tools[0])
	}
	if params, ok := tf["parameters"].(map[string]any); !ok || params["type"] != "object" {
		t.Errorf("schema must be embedded as an object, not a string: %v", tf["parameters"])
	}
	if p["tool_choice"] != "required" {
		t.Errorf("tool_choice any should map to required, got %v", p["tool_choice"])
	}
	if p["max_tokens"] != 512 {
		t.Errorf("gpt-4o (bare) uses max_tokens, got %v / %v", p["max_tokens"], p["max_completion_tokens"])
	}
	if _, ok := p["stream_options"]; !ok {
		t.Error("streaming request must ask for usage")
	}
}

func TestBuildChatCompletionRequestTokenParamAndToolChoice(t *testing.T) {
	for model, wantKey := range map[string]string{
		"o3":            "max_completion_tokens",
		"o4-mini":       "max_completion_tokens",
		"gpt-5.4":       "max_completion_tokens",
		"gpt-4.1":       "max_completion_tokens",
		"gpt-4o":        "max_tokens",
		"llama3.3:70b":  "max_tokens",
		"deepseek-chat": "max_tokens",
	} {
		p := buildChatCompletionRequest(apitypes.MessageRequest{Model: model, MaxTokens: 7})
		if p[wantKey] != 7 {
			t.Errorf("%s: want %s=7, got max_tokens=%v max_completion_tokens=%v", model, wantKey, p["max_tokens"], p["max_completion_tokens"])
		}
	}

	p := buildChatCompletionRequest(apitypes.MessageRequest{Model: "gpt-4o", ToolChoice: &apitypes.ToolChoice{Kind: "tool", Name: "read_file"}})
	tc := p["tool_choice"].(map[string]any)
	if tc["type"] != "function" || tc["function"].(map[string]any)["name"] != "read_file" {
		t.Errorf("forced tool choice = %v", tc)
	}
	if _, ok := p["stream_options"]; ok {
		t.Error("non-streaming request must not send stream_options")
	}
}

func TestChatCompletionsEndpointIsIdempotent(t *testing.T) {
	for in, want := range map[string]string{
		"https://api.openai.com/v1":                  "https://api.openai.com/v1/chat/completions",
		"https://api.openai.com/v1/":                 "https://api.openai.com/v1/chat/completions",
		"http://localhost:11434/v1/chat/completions": "http://localhost:11434/v1/chat/completions",
	} {
		if got := chatCompletionsEndpoint(in); got != want {
			t.Errorf("%s -> %s, want %s", in, got, want)
		}
	}
}

func TestNormalizeFinishReasonAndToolArguments(t *testing.T) {
	for in, want := range map[string]string{"stop": "end_turn", "tool_calls": "tool_use", "length": "length", "": ""} {
		if got := NormalizeFinishReason(in); got != want {
			t.Errorf("%q -> %q, want %q", in, got, want)
		}
	}
	if got := string(parseToolArguments(`{"a":1}`)); got != `{"a":1}` {
		t.Errorf("valid arguments altered: %s", got)
	}
	// Models sometimes emit unparsable arguments; the runtime still needs a
	// JSON object so the tool sees what was sent.
	if got := string(parseToolArguments(`{"a":`)); got != `{"raw":"{\"a\":"}` {
		t.Errorf("invalid arguments should be wrapped, got %s", got)
	}
}

func TestOpenAiCompatKindOnlyDistinguishesXai(t *testing.T) {
	if k := (&OpenAiCompatProvider{Config: OpenAiCompatConfig{ProviderName: "xAI"}}).Kind(); k != ProviderXai {
		t.Errorf("xAI kind = %v", k)
	}
	if k := (&OpenAiCompatProvider{Config: OpenAiCompatConfig{ProviderName: "Groq"}}).Kind(); k != ProviderOpenAi {
		t.Errorf("Groq kind = %v", k)
	}
}
