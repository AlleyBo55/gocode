package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AlleyBo55/gocode/internal/apiclient"
	"github.com/AlleyBo55/gocode/internal/apitypes"
)

// fakeModel answers each probe from closures so a test can describe a model's
// behaviour ("ignores system prompts", "one tool call at a time") directly.
type fakeModel struct {
	stream func(req apitypes.MessageRequest) ([]apitypes.StreamEvent, error)
	send   func(req apitypes.MessageRequest) (*apitypes.MessageResponse, error)
	// delay, when set, makes SendMessage wait that long or until ctx is done,
	// the way a real HTTP call would.
	delay time.Duration
	sent  []apitypes.MessageRequest
}

func (f *fakeModel) Kind() apiclient.ProviderKind { return apiclient.ProviderOpenAi }

func (f *fakeModel) SendMessage(ctx context.Context, req apitypes.MessageRequest) (*apitypes.MessageResponse, error) {
	f.sent = append(f.sent, req)
	if f.delay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(f.delay):
		}
	}
	return f.send(req)
}

func (f *fakeModel) StreamMessage(_ context.Context, req apitypes.MessageRequest) (<-chan apitypes.StreamEvent, error) {
	f.sent = append(f.sent, req)
	events, err := f.stream(req)
	if err != nil {
		return nil, err
	}
	ch := make(chan apitypes.StreamEvent, len(events))
	for _, ev := range events {
		ch <- ev
	}
	close(ch)
	return ch, nil
}

func textResp(text string) *apitypes.MessageResponse {
	return &apitypes.MessageResponse{
		Role: "assistant", StopReason: "end_turn",
		Content: []apitypes.OutputContentBlock{{Kind: "text", Text: text}},
		Usage:   apitypes.Usage{InputTokens: 10, OutputTokens: 2},
	}
}

func toolResp(args ...string) *apitypes.MessageResponse {
	r := &apitypes.MessageResponse{Role: "assistant", StopReason: "tool_use", Usage: apitypes.Usage{InputTokens: 40, OutputTokens: 12}}
	for i, a := range args {
		r.Content = append(r.Content, apitypes.OutputContentBlock{Kind: "tool_use", ID: "call_" + string(rune('a'+i)), Name: "get_weather", Input: json.RawMessage(a)})
	}
	return r
}

func goodStream(apitypes.MessageRequest) ([]apitypes.StreamEvent, error) {
	return []apitypes.StreamEvent{
		{Kind: "message_start", Message: &apitypes.MessageResponse{Usage: apitypes.Usage{InputTokens: 8, OutputTokens: 1}}},
		{Kind: "content_block_start", Index: 0, ContentBlock: &apitypes.OutputContentBlock{Kind: "text"}},
		{Kind: "content_block_delta", Index: 0, BlockDelta: &apitypes.ContentBlockDelta{Kind: "text_delta", Text: "OK"}},
		{Kind: "content_block_stop", Index: 0},
		{Kind: "message_delta", DeltaUsage: &apitypes.Usage{OutputTokens: 1}},
		{Kind: "message_stop"},
	}, nil
}

// capableModel behaves like a current frontier model on every probe.
func capableModel() *fakeModel {
	return &fakeModel{
		stream: goodStream,
		send: func(req apitypes.MessageRequest) (*apitypes.MessageResponse, error) {
			prompt := req.Messages[0].Content[0].Text
			switch {
			case req.System != "":
				return textResp("PINEAPPLE"), nil
			case strings.Contains(prompt, "Tokyo"):
				return toolResp(`{"city":"Paris"}`, `{"city":"Tokyo"}`), nil
			case len(req.Tools) > 0:
				return toolResp(`{"city":"Paris"}`), nil
			}
			return textResp("unexpected probe"), nil
		},
	}
}

func check(t *testing.T, rep Report, name string) Check {
	t.Helper()
	for _, c := range rep.Checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no check %q in %+v", name, rep.Checks)
	return Check{}
}

func TestProbeCapableModelPassesEverything(t *testing.T) {
	m := capableModel()
	rep := Prober{Provider: m, Model: "gpt-4o"}.Run(context.Background())

	if !rep.AllOK() {
		t.Fatalf("expected all checks to pass:\n%s", rep.Render())
	}
	if len(rep.Checks) != 4 || rep.Checks[0].Name != CheckStreaming || rep.Checks[3].Name != CheckParallel {
		t.Errorf("checks = %+v", rep.Checks)
	}
	if !strings.HasPrefix(check(t, rep, CheckStreaming).Detail, "first token in") {
		t.Errorf("streaming detail = %q", check(t, rep, CheckStreaming).Detail)
	}
	if d := check(t, rep, CheckToolCall).Detail; d != `arguments {"city":"Paris"}` {
		t.Errorf("tool call detail should echo the arguments as JSON, got %q", d)
	}
	if d := check(t, rep, CheckParallel).Detail; d != "2 calls in one turn" {
		t.Errorf("parallel detail = %q", d)
	}
	// One stream + three sends, and the probe's own spend is summed.
	if len(m.sent) != 4 {
		t.Errorf("requests made = %d, want 4", len(m.sent))
	}
	if rep.Usage.InputTokens != 8+10+40+40 || rep.Usage.OutputTokens != 1+2+12+12 {
		t.Errorf("probe usage = %+v", rep.Usage)
	}
	if rep.Model != "gpt-4o" || rep.ProbedAt.IsZero() {
		t.Errorf("report header = %+v", rep)
	}
	// Probes are cheap by construction.
	for _, req := range m.sent {
		if req.MaxTokens != 256 {
			t.Errorf("probe request max_tokens = %d, want 256", req.MaxTokens)
		}
	}
}

func TestProbeWeakModelReportsEachGapPlainly(t *testing.T) {
	m := &fakeModel{
		stream: func(apitypes.MessageRequest) ([]apitypes.StreamEvent, error) {
			return []apitypes.StreamEvent{
				{Kind: "message_start", Message: &apitypes.MessageResponse{}},
				{Kind: "error", BlockDelta: &apitypes.ContentBlockDelta{Kind: "text_delta", Text: "Error: stream interrupted: unexpected EOF"}},
			}, nil
		},
		send: func(req apitypes.MessageRequest) (*apitypes.MessageResponse, error) {
			if req.System != "" {
				return textResp("Hello! How can I help you today?"), nil
			}
			return textResp("The weather in Paris is sunny."), nil // never calls the tool
		},
	}
	rep := Prober{Provider: m, Model: "weak"}.Run(context.Background())

	if rep.AllOK() {
		t.Fatal("nothing should pass")
	}
	if c := check(t, rep, CheckStreaming); c.OK || !strings.Contains(c.Detail, "stream interrupted") {
		t.Errorf("streaming = %+v", c)
	}
	if c := check(t, rep, CheckSystemPrompt); c.OK || !strings.Contains(c.Detail, "ignored") || !strings.Contains(c.Detail, "Hello!") {
		t.Errorf("system prompt = %+v", c)
	}
	if c := check(t, rep, CheckToolCall); c.OK || !strings.Contains(c.Detail, "no tool_use block") || !strings.Contains(c.Detail, "end_turn") {
		t.Errorf("tool call = %+v", c)
	}
	// Parallel is not probed when a single call already failed: no point
	// spending tokens to learn the same thing twice.
	if c := check(t, rep, CheckParallel); c.OK || !strings.HasPrefix(c.Detail, "skipped") {
		t.Errorf("parallel = %+v", c)
	}
	if len(m.sent) != 3 {
		t.Errorf("requests made = %d, want 3 (parallel skipped)", len(m.sent))
	}
}

func TestProbeSerialToolUserIsNamedNotFailed(t *testing.T) {
	m := capableModel()
	m.send = func(req apitypes.MessageRequest) (*apitypes.MessageResponse, error) {
		if req.System != "" {
			return textResp("PINEAPPLE"), nil
		}
		return toolResp(`{"city":"Paris"}`), nil // always exactly one call
	}
	rep := Prober{Provider: m, Model: "serial"}.Run(context.Background())
	if c := check(t, rep, CheckToolCall); !c.OK {
		t.Errorf("single tool call should pass: %+v", c)
	}
	if c := check(t, rep, CheckParallel); c.OK || !strings.Contains(c.Detail, "serialises") {
		t.Errorf("parallel = %+v", c)
	}
	if ok, known := rep.Supports(CheckParallel); ok || !known {
		t.Errorf("Supports(parallel) = %v,%v", ok, known)
	}
	if _, known := rep.Supports("nonsense"); known {
		t.Error("unknown check must not be reported as known")
	}
}

func TestProbeToolArgumentQuality(t *testing.T) {
	cases := []struct {
		name, args, wantDetail string
	}{
		{"invalid json wrapped by provider layer", `{"raw":"{\"city\": Paris}"}`, "not valid JSON"},
		{"not an object", `["Paris"]`, "not a JSON object"},
		{"missing required field", `{"location":"Paris"}`, "city"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := capableModel()
			m.send = func(req apitypes.MessageRequest) (*apitypes.MessageResponse, error) {
				if req.System != "" {
					return textResp("PINEAPPLE"), nil
				}
				return toolResp(tc.args), nil
			}
			rep := Prober{Provider: m, Model: "m"}.Run(context.Background())
			if c := check(t, rep, CheckToolCall); c.OK || !strings.Contains(c.Detail, tc.wantDetail) {
				t.Errorf("tool call = %+v, want detail mentioning %q", c, tc.wantDetail)
			}
		})
	}
}

func TestProbeRequestErrorsAreCheckResultsNotPanics(t *testing.T) {
	boom := apitypes.NewApiError(401, "authentication_error", "invalid x-api-key", "")
	m := &fakeModel{
		stream: func(apitypes.MessageRequest) ([]apitypes.StreamEvent, error) { return nil, boom },
		send:   func(apitypes.MessageRequest) (*apitypes.MessageResponse, error) { return nil, boom },
	}
	rep := Prober{Provider: m, Model: "m"}.Run(context.Background())
	if len(rep.Checks) != 4 {
		t.Fatalf("every check should still be reported: %+v", rep.Checks)
	}
	for _, c := range rep.Checks[:3] {
		if c.OK || !strings.Contains(c.Detail, "invalid x-api-key") {
			t.Errorf("%s = %+v, want the API error surfaced", c.Name, c)
		}
	}
	if rep.Usage != (apitypes.Usage{}) {
		t.Errorf("failed requests must not count usage: %+v", rep.Usage)
	}
}

func TestProbeStreamWithoutStopIsSuspect(t *testing.T) {
	m := capableModel()
	m.stream = func(apitypes.MessageRequest) ([]apitypes.StreamEvent, error) {
		return []apitypes.StreamEvent{
			{Kind: "content_block_delta", Index: 0, BlockDelta: &apitypes.ContentBlockDelta{Kind: "text_delta", Text: "OK"}},
		}, nil
	}
	rep := Prober{Provider: m, Model: "m"}.Run(context.Background())
	if c := check(t, rep, CheckStreaming); c.OK || !strings.Contains(c.Detail, "message_stop") {
		t.Errorf("streaming = %+v", c)
	}
}

func TestProbeHonoursPerCheckTimeout(t *testing.T) {
	// Every SendMessage hangs for longer than the per-check budget; the
	// context the prober passes must be what cuts it short.
	m := capableModel()
	m.delay = 2 * time.Second
	start := time.Now()
	rep := Prober{Provider: m, Model: "m", Timeout: 20 * time.Millisecond}.Run(context.Background())
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("probe took %s; the timeout was not applied", elapsed)
	}
	for _, name := range []string{CheckSystemPrompt, CheckToolCall} {
		if c := check(t, rep, name); c.OK || !errors.Is(context.DeadlineExceeded, context.DeadlineExceeded) || !strings.Contains(c.Detail, context.DeadlineExceeded.Error()) {
			t.Errorf("%s = %+v, want the deadline surfaced", name, c)
		}
	}
	// Streaming has no delay in the fake and is unaffected by the others.
	if c := check(t, rep, CheckStreaming); !c.OK {
		t.Errorf("streaming = %+v", c)
	}
}

func TestRenderShape(t *testing.T) {
	rep := Prober{Provider: capableModel(), Model: "gpt-4o"}.Run(context.Background())
	out := rep.Render()
	lines := strings.Split(out, "\n")
	if len(lines) != 5 {
		t.Fatalf("want header + 4 checks:\n%s", out)
	}
	if !strings.HasPrefix(lines[0], "Model: gpt-4o") || !strings.Contains(lines[0], "probe cost: 98 in / 27 out") {
		t.Errorf("header = %q", lines[0])
	}
	for _, l := range lines[1:] {
		if !strings.HasPrefix(l, "  ✓ ") {
			t.Errorf("check line = %q", l)
		}
	}
	// Names are padded to one column so details line up.
	col := strings.Index(lines[1], "first token")
	if col < 0 || strings.Index(lines[2], "honoured") != col {
		t.Errorf("details not aligned:\n%s", out)
	}
	rep.Checks[3].OK = false
	if !strings.Contains(rep.Render(), "  ✗ parallel tool calls") {
		t.Error("failed checks render with ✗")
	}
}

func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "capabilities.json")
	s := NewStore(path)
	if _, ok := s.Get("gpt-4o"); ok {
		t.Fatal("empty store must not report a cached probe")
	}
	if reports, err := s.Load(); err != nil || len(reports) != 0 {
		t.Fatalf("missing file should load as empty: %v %v", reports, err)
	}

	rep := Prober{Provider: capableModel(), Model: "gpt-4o"}.Run(context.Background())
	if err := s.Put(rep); err != nil {
		t.Fatal(err)
	}
	other := rep
	other.Model = "claude-sonnet-4-6"
	if err := s.Put(other); err != nil {
		t.Fatal(err)
	}

	got, ok := NewStore(path).Get("gpt-4o")
	if !ok || got.Model != "gpt-4o" || len(got.Checks) != 4 || !got.AllOK() {
		t.Errorf("round trip lost data: %+v", got)
	}
	if got.Usage != rep.Usage || !got.ProbedAt.Equal(rep.ProbedAt) {
		t.Errorf("usage/time changed across round trip: %+v vs %+v", got, rep)
	}
	if reports, _ := s.Load(); len(reports) != 2 {
		t.Errorf("second Put must not overwrite the first: %d entries", len(reports))
	}

	if err := os.WriteFile(path, []byte("{corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(); err == nil {
		t.Error("corrupt cache must be an error, not an empty cache")
	}
	if _, ok := s.Get("gpt-4o"); ok {
		t.Error("Get on a corrupt cache must miss")
	}
	// Put recovers by starting over rather than failing forever.
	if err := s.Put(rep); err != nil {
		t.Errorf("Put over corrupt file: %v", err)
	}
	if _, ok := s.Get("gpt-4o"); !ok {
		t.Error("Put should have replaced the corrupt file")
	}

	if NewStore("").Path() != filepath.Join(".gocode", "capabilities.json") {
		t.Errorf("default path = %s", NewStore("").Path())
	}
}
