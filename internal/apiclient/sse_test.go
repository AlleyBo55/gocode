package apiclient

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/AlleyBo55/gocode/internal/apitypes"
)

// pushAll feeds body to a fresh parser in pieces of the given size and
// returns every event, including any flushed by Finish.
func pushAll(t *testing.T, body []byte, piece int) []apitypes.StreamEvent {
	t.Helper()
	p := NewSseParser()
	var out []apitypes.StreamEvent
	for i := 0; i < len(body); i += piece {
		end := min(i+piece, len(body))
		evs, err := p.Push(body[i:end])
		if err != nil {
			t.Fatalf("push (piece %d) at %d: %v", piece, i, err)
		}
		out = append(out, evs...)
	}
	evs, err := p.Finish()
	if err != nil {
		t.Fatalf("finish (piece %d): %v", piece, err)
	}
	return append(out, evs...)
}

func TestSseParserIsIndependentOfReadBoundaries(t *testing.T) {
	// A socket hands the parser arbitrary byte ranges. Every split must yield
	// the same events as the whole stream at once, or a token boundary that
	// happens to land inside a frame corrupts the reply.
	for _, name := range []string{"anthropic_text.sse", "anthropic_tool_use.sse", "anthropic_error_midstream.sse"} {
		body := fixture(t, name)
		whole := pushAll(t, body, len(body))
		if len(whole) == 0 {
			t.Fatalf("%s: fixture produced no events", name)
		}
		for _, piece := range []int{1, 2, 3, 7, 64, 1000} {
			got := pushAll(t, body, piece)
			if !reflect.DeepEqual(got, whole) {
				t.Errorf("%s: piece size %d produced %v, whole produced %v", name, piece, kinds(got), kinds(whole))
			}
		}
	}
}

func TestSseParserAnthropicTextFixture(t *testing.T) {
	events := pushAll(t, fixture(t, "anthropic_text.sse"), 4096)

	// ping is discarded; everything else arrives in order.
	want := []string{"message_start", "content_block_start", "content_block_delta", "content_block_delta", "content_block_stop", "message_delta", "message_stop"}
	if got := kinds(events); !reflect.DeepEqual(got, want) {
		t.Fatalf("kinds = %v, want %v", got, want)
	}

	start := findKind(events, "message_start")
	if start.Message == nil || start.Message.ID != "msg_01XFDUDYJgAACzvnptvVoYEL" || start.Message.Model != "claude-sonnet-4-6" {
		t.Errorf("message_start not decoded: %+v", start.Message)
	}
	// Anthropic reports input tokens here, not in message_delta. Anything that
	// only reads message_delta under-counts every streamed turn.
	if start.Message.Usage.InputTokens != 25 {
		t.Errorf("message_start input_tokens = %d, want 25", start.Message.Usage.InputTokens)
	}

	if got := textOf(events, 0); got != "Hello, world!" {
		t.Errorf("text = %q", got)
	}

	delta := findKind(events, "message_delta")
	if delta.Delta == nil || delta.Delta.StopReason != "end_turn" {
		t.Errorf("message_delta stop_reason not decoded: %+v", delta.Delta)
	}
	if delta.DeltaUsage == nil || delta.DeltaUsage.OutputTokens != 15 {
		t.Errorf("message_delta usage not decoded: %+v", delta.DeltaUsage)
	}
}

func TestSseParserAnthropicToolUseFixture(t *testing.T) {
	events := pushAll(t, fixture(t, "anthropic_tool_use.sse"), 4096)

	tool := blockStart(events, 1)
	if tool == nil || tool.Kind != "tool_use" || tool.ID != "toolu_01A09q90qw90lq917835lq9" || tool.Name != "read_file" {
		t.Fatalf("tool_use block start not decoded: %+v", tool)
	}
	// The first input_json_delta Anthropic sends is empty; the rest must
	// concatenate into valid JSON exactly as sent.
	if got := toolInputOf(events, 1); got != `{"path": "go.mod"}` {
		t.Errorf("tool input = %q", got)
	}
	if got := textOf(events, 0); got != "I'll read the file." {
		t.Errorf("text = %q", got)
	}
	if d := findKind(events, "message_delta"); d.Delta.StopReason != "tool_use" || d.DeltaUsage.OutputTokens != 61 {
		t.Errorf("message_delta = %+v / %+v", d.Delta, d.DeltaUsage)
	}
}

func TestSseParserAcceptsCRLFFrames(t *testing.T) {
	lf := fixture(t, "anthropic_text.sse")
	crlf := []byte(strings.ReplaceAll(string(lf), "\n", "\r\n"))
	if !reflect.DeepEqual(kinds(pushAll(t, crlf, 4096)), kinds(pushAll(t, lf, 4096))) {
		t.Error("CRLF framing produced different events from LF framing")
	}
	// And with CRLF cut at every byte, which exercises the 4-byte separator
	// straddling a read boundary.
	if !reflect.DeepEqual(kinds(pushAll(t, crlf, 1)), kinds(pushAll(t, lf, 4096))) {
		t.Error("CRLF framing at 1-byte reads produced different events")
	}
}

func TestSseParserSkipsCommentsDoneAndJoinsMultiLineData(t *testing.T) {
	raw := ": keep-alive comment\n" +
		"data: {\"type\":\"message_stop\"}\n\n" +
		"data: [DONE]\n\n" +
		// The SSE spec allows a payload split over several data: lines,
		// joined by newlines. One leading space after the colon is stripped.
		"data: {\"type\":\"content_block_delta\",\"index\":0,\n" +
		"data:  \"delta\":{\"type\":\"text_delta\",\"text\":\"two lines\"}}\n\n"
	events := pushAll(t, []byte(raw), 4096)
	want := []string{"message_stop", "content_block_delta"}
	if got := kinds(events); !reflect.DeepEqual(got, want) {
		t.Fatalf("kinds = %v, want %v", got, want)
	}
	if events[1].BlockDelta == nil || events[1].BlockDelta.Text != "two lines" {
		t.Errorf("multi-line data not joined: %+v", events[1].BlockDelta)
	}
}

func TestSseParserFinishFlushesUnterminatedFrame(t *testing.T) {
	p := NewSseParser()
	evs, err := p.Push([]byte("data: {\"type\":\"message_stop\"}"))
	if err != nil || len(evs) != 0 {
		t.Fatalf("unterminated frame should wait: %v %v", evs, err)
	}
	evs, err = p.Finish()
	if err != nil || len(evs) != 1 || evs[0].Kind != "message_stop" {
		t.Fatalf("finish = %v, %v", evs, err)
	}
	// Finish is idempotent on an empty buffer.
	if evs, err = p.Finish(); err != nil || evs != nil {
		t.Errorf("second finish = %v, %v", evs, err)
	}
}

func TestSseParserReportsBadFrameWithoutLosingEarlierEvents(t *testing.T) {
	p := NewSseParser()
	evs, err := p.Push([]byte("data: {\"type\":\"message_start\"}\n\ndata: {not json\n\n"))
	var apiErr *apitypes.ApiError
	if !errors.As(err, &apiErr) || apiErr.Kind != apitypes.ErrInvalidSseFrame {
		t.Fatalf("want ErrInvalidSseFrame, got %v", err)
	}
	if len(evs) != 1 || evs[0].Kind != "message_start" {
		t.Errorf("events before the bad frame were dropped: %v", kinds(evs))
	}
}

func TestSseParserReshapesAnthropicErrorFrameForDisplay(t *testing.T) {
	// Anthropic's mid-stream failure is an "error" frame with no content
	// block. The REPL only shows Kind "error" when BlockDelta carries text
	// (repl.go), so the parser must put the message there or it is dropped.
	events := pushAll(t, fixture(t, "anthropic_error_midstream.sse"), 4096)
	ev := findKind(events, "error")
	if ev == nil {
		t.Fatalf("no error event in %v", kinds(events))
	}
	if ev.BlockDelta == nil || ev.BlockDelta.Kind != "text_delta" {
		t.Fatalf("error event has no displayable text: %+v", ev)
	}
	for _, want := range []string{"overloaded_error", "Overloaded"} {
		if !strings.Contains(ev.BlockDelta.Text, want) {
			t.Errorf("error text %q should mention %q", ev.BlockDelta.Text, want)
		}
	}
	// Content that arrived before the failure is still delivered.
	if got := textOf(events, 0); got != "Partial" {
		t.Errorf("text before error = %q", got)
	}
}

func TestErrorEventMatchesTheShapeTheReplChecks(t *testing.T) {
	// repl.go: `if ev.Kind == "error" && ev.BlockDelta != nil`. Every producer
	// goes through errorEvent so this is the one place the shape is pinned.
	ev := errorEvent("Error: boom")
	if ev.Kind != "error" || ev.BlockDelta == nil || ev.BlockDelta.Text != "Error: boom" {
		t.Errorf("errorEvent shape drifted: %+v", ev)
	}
	if got := streamErrorEvent(errors.New("eof")).BlockDelta.Text; !strings.Contains(got, "stream interrupted") || !strings.Contains(got, "eof") {
		t.Errorf("streamErrorEvent text = %q", got)
	}
}
