package apiclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/AlleyBo55/gocode/internal/apitypes"
)

func newAnthropic(t *testing.T, srv *httptest.Server) *AnthropicProvider {
	t.Helper()
	p := NewAnthropicProvider(apitypes.AuthApiKey("sk-ant-test"))
	p.BaseURL = srv.URL
	p.Retry = fastRetry()
	return p
}

func TestAnthropicStreamTextFixtureEndToEnd(t *testing.T) {
	body := fixture(t, "anthropic_text.sse")
	rec := &recorder{}
	srv := newServer(t, rec, func(w http.ResponseWriter, r *http.Request) { writeStream(w, body, 5) })
	p := newAnthropic(t, srv)

	ch, err := p.StreamMessage(context.Background(), userRequest("claude-sonnet-4-6"))
	if err != nil {
		t.Fatal(err)
	}
	events := collect(t, ch)

	req := rec.last(t)
	if req.Path != "/v1/messages" {
		t.Errorf("path = %s", req.Path)
	}
	if req.Header.Get("anthropic-version") == "" || req.Header.Get("x-api-key") != "sk-ant-test" {
		t.Errorf("headers missing: %v", req.Header)
	}
	var sent map[string]any
	if err := json.Unmarshal(req.Body, &sent); err != nil {
		t.Fatal(err)
	}
	if sent["stream"] != true {
		t.Errorf("stream flag not set on wire: %v", sent)
	}

	want := []string{"message_start", "content_block_start", "content_block_delta", "content_block_delta", "content_block_stop", "message_delta", "message_stop"}
	if got := kinds(events); !reflect.DeepEqual(got, want) {
		t.Fatalf("kinds = %v", got)
	}
	if got := textOf(events, 0); got != "Hello, world!" {
		t.Errorf("text = %q", got)
	}
}

func TestAnthropicStreamToolUseFixtureEndToEnd(t *testing.T) {
	body := fixture(t, "anthropic_tool_use.sse")
	srv := newServer(t, &recorder{}, func(w http.ResponseWriter, r *http.Request) { writeStream(w, body, 13) })
	ch, err := newAnthropic(t, srv).StreamMessage(context.Background(), userRequest("claude-sonnet-4-6"))
	if err != nil {
		t.Fatal(err)
	}
	events := collect(t, ch)
	if tool := blockStart(events, 1); tool == nil || tool.Name != "read_file" {
		t.Fatalf("tool block missing: %v", kinds(events))
	}
	if got := toolInputOf(events, 1); got != `{"path": "go.mod"}` {
		t.Errorf("tool input = %q", got)
	}
	if d := findKind(events, "message_delta"); d == nil || d.Delta.StopReason != "tool_use" {
		t.Errorf("stop reason not tool_use")
	}
}

func TestAnthropicStreamSurfacesServerErrorFrame(t *testing.T) {
	body := fixture(t, "anthropic_error_midstream.sse")
	srv := newServer(t, &recorder{}, func(w http.ResponseWriter, r *http.Request) { writeStream(w, body, 4096) })
	ch, err := newAnthropic(t, srv).StreamMessage(context.Background(), userRequest("claude-sonnet-4-6"))
	if err != nil {
		t.Fatal(err)
	}
	events := collect(t, ch)
	ev := findKind(events, "error")
	if ev == nil || ev.BlockDelta == nil || !strings.Contains(ev.BlockDelta.Text, "overloaded_error") {
		t.Fatalf("overloaded_error was not surfaced: %v", kinds(events))
	}
	if hasKind(events, "message_stop") {
		t.Error("a failed stream must not also claim message_stop")
	}
}

func TestAnthropicStreamSurfacesMalformedFrameInsteadOfClosingSilently(t *testing.T) {
	// Before the fix the goroutine returned on parse error, the channel closed
	// cleanly, and the runtime recorded the partial reply as a complete turn.
	good := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"x\"}}\n\n" +
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\"}}\n\n"
	bad := "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{oops\n\n"
	srv := newServer(t, &recorder{}, func(w http.ResponseWriter, r *http.Request) { writeStream(w, []byte(good+bad), 4096) })

	ch, err := newAnthropic(t, srv).StreamMessage(context.Background(), userRequest("claude-sonnet-4-6"))
	if err != nil {
		t.Fatal(err)
	}
	events := collect(t, ch)
	want := []string{"message_start", "content_block_start", "error"}
	if got := kinds(events); !reflect.DeepEqual(got, want) {
		t.Fatalf("kinds = %v, want %v", got, want)
	}
	if !strings.Contains(events[2].BlockDelta.Text, "invalid sse frame") {
		t.Errorf("error text = %q", events[2].BlockDelta.Text)
	}
}

func TestAnthropicStreamSurfacesDroppedConnection(t *testing.T) {
	body := fixture(t, "anthropic_text.sse")
	// Cut after the second frame, mid-message, with no terminating chunk.
	cut := strings.Index(string(body), "event: ping")
	srv := newServer(t, &recorder{}, func(w http.ResponseWriter, r *http.Request) { writeStreamThenDrop(w, body[:cut]) })

	ch, err := newAnthropic(t, srv).StreamMessage(context.Background(), userRequest("claude-sonnet-4-6"))
	if err != nil {
		t.Fatal(err)
	}
	events := collect(t, ch)
	if got := kinds(events); len(got) != 3 || got[0] != "message_start" || got[1] != "content_block_start" || got[2] != "error" {
		t.Fatalf("kinds = %v, want the two delivered frames then an error", got)
	}
	if !strings.Contains(events[2].BlockDelta.Text, "stream interrupted") {
		t.Errorf("error text = %q", events[2].BlockDelta.Text)
	}
}

func TestAnthropicStreamCancellationIsNotReportedAsAnError(t *testing.T) {
	body := fixture(t, "anthropic_text.sse")
	first := body[:strings.Index(string(body), "event: content_block_start")]
	srv := newServer(t, &recorder{}, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(first)
		w.(http.Flusher).Flush()
		<-r.Context().Done() // hold the stream open until the client goes away
	})

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := newAnthropic(t, srv).StreamMessage(ctx, userRequest("claude-sonnet-4-6"))
	if err != nil {
		t.Fatal(err)
	}
	if ev := <-ch; ev.Kind != "message_start" {
		t.Fatalf("first event = %s", ev.Kind)
	}
	cancel()
	rest := collect(t, ch)
	if hasKind(rest, "error") {
		t.Errorf("Ctrl-C must not render as a stream error, got %v", kinds(rest))
	}
}

func TestAnthropicRetriesRateLimitThenSucceeds(t *testing.T) {
	body := fixture(t, "anthropic_text.sse")
	var n int32
	rec := &recorder{}
	srv := newServer(t, rec, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&n, 1) <= 2 {
			writeJSON(w, http.StatusTooManyRequests, `{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`)
			return
		}
		writeStream(w, body, 4096)
	})
	ch, err := newAnthropic(t, srv).StreamMessage(context.Background(), userRequest("claude-sonnet-4-6"))
	if err != nil {
		t.Fatalf("expected success on third attempt, got %v", err)
	}
	collect(t, ch)
	if rec.count() != 3 {
		t.Errorf("attempts = %d, want 3", rec.count())
	}
}

func TestAnthropicRetriesExhaustedIsRecognisedByFallback(t *testing.T) {
	// This is the seam that was broken: the provider wraps the 429 in
	// RetriesExhausted, and the fallback chain judged the wrapper (Status 0)
	// rather than the cause, so failover never fired for rate limits.
	rec := &recorder{}
	srv := newServer(t, rec, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusTooManyRequests, `{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`)
	})
	_, err := newAnthropic(t, srv).SendMessage(context.Background(), userRequest("claude-sonnet-4-6"))
	var apiErr *apitypes.ApiError
	if !errors.As(err, &apiErr) || apiErr.Kind != apitypes.ErrRetriesExhausted || apiErr.Attempts != 3 {
		t.Fatalf("want RetriesExhausted after 3 attempts, got %v", err)
	}
	var inner *apitypes.ApiError
	if !errors.As(apiErr.Wrapped, &inner) || inner.Status != 429 || inner.ErrorType != "rate_limit_error" {
		t.Fatalf("cause not preserved: %v", apiErr.Wrapped)
	}
	if !isFallbackRetryable(err) {
		t.Error("fallback must treat an exhausted 429 as a reason to try the next model")
	}
	if rec.count() != 3 {
		t.Errorf("attempts = %d, want 3", rec.count())
	}
}

func TestAnthropicNonRetryableErrorReturnsImmediately(t *testing.T) {
	rec := &recorder{}
	srv := newServer(t, rec, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusBadRequest, `{"type":"error","error":{"type":"invalid_request_error","message":"max_tokens: must be positive"}}`)
	})
	_, err := newAnthropic(t, srv).SendMessage(context.Background(), userRequest("claude-sonnet-4-6"))
	var apiErr *apitypes.ApiError
	if !errors.As(err, &apiErr) || apiErr.Status != 400 || apiErr.ErrorType != "invalid_request_error" {
		t.Fatalf("got %v", err)
	}
	if !strings.Contains(err.Error(), "max_tokens: must be positive") {
		t.Errorf("message lost from Error(): %s", err)
	}
	if rec.count() != 1 {
		t.Errorf("400 must not be retried; attempts = %d", rec.count())
	}
	if isFallbackRetryable(err) {
		t.Error("a 400 must not trigger fallback either")
	}
}

func TestAnthropicSendMessageDecodesResponseAndRequestID(t *testing.T) {
	rec := &recorder{}
	srv := newServer(t, rec, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("request-id", "req_abc")
		writeJSON(w, http.StatusOK, `{"id":"msg_01","type":"message","role":"assistant","content":[{"type":"text","text":"Hi there"}],"model":"claude-sonnet-4-6","stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":3}}`)
	})
	resp, err := newAnthropic(t, srv).SendMessage(context.Background(), userRequest("claude-sonnet-4-6"))
	if err != nil {
		t.Fatal(err)
	}
	if resp.RequestID != "req_abc" || resp.Content[0].Text != "Hi there" || resp.Usage.InputTokens != 10 {
		t.Errorf("decoded = %+v", resp)
	}
	var sent map[string]any
	_ = json.Unmarshal(rec.last(t).Body, &sent)
	if sent["stream"] == true {
		t.Error("SendMessage must not request a stream")
	}
}

func TestReadApiErrorKeepsProviderCodeSeparateFromType(t *testing.T) {
	// OpenAI-compatible APIs put the actionable code in "code" and leave
	// "type" generic. Fallback keys off the code, so it must survive parsing.
	body := `{"error":{"message":"This model's maximum context length is 128000 tokens.","type":"invalid_request_error","param":"messages","code":"context_length_exceeded"}}`
	resp := &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader(body))}
	apiErr := readApiError(resp)
	if apiErr.ErrorType != "invalid_request_error" || apiErr.Code != "context_length_exceeded" {
		t.Errorf("type=%q code=%q", apiErr.ErrorType, apiErr.Code)
	}
	if apiErr.Retryable {
		t.Error("400 is not retryable at the provider level")
	}
	if !isFallbackRetryable(apiErr) {
		t.Error("context_length_exceeded must fall through to the next model")
	}
}
