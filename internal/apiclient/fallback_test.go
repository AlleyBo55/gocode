package apiclient

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/AlleyBo55/gocode/internal/apitypes"
)

// fakeProvider answers every call with err, or with a response naming the
// model it was asked for. seen is shared across a chain so a test can read
// the order in which models were tried.
type fakeProvider struct {
	kind ProviderKind
	err  error
	seen *[]string
}

func (f *fakeProvider) SendMessage(_ context.Context, req apitypes.MessageRequest) (*apitypes.MessageResponse, error) {
	*f.seen = append(*f.seen, req.Model)
	if f.err != nil {
		return nil, f.err
	}
	return &apitypes.MessageResponse{Model: req.Model, Content: []apitypes.OutputContentBlock{{Kind: "text", Text: "ok"}}}, nil
}

func (f *fakeProvider) StreamMessage(_ context.Context, req apitypes.MessageRequest) (<-chan apitypes.StreamEvent, error) {
	*f.seen = append(*f.seen, req.Model)
	if f.err != nil {
		return nil, f.err
	}
	ch := make(chan apitypes.StreamEvent, 1)
	ch <- apitypes.StreamEvent{Kind: "message_start", Message: &apitypes.MessageResponse{Model: req.Model}}
	close(ch)
	return ch, nil
}

func (f *fakeProvider) Kind() ProviderKind { return f.kind }

type fallbackLog struct{ hops []string }

func (l *fallbackLog) OnFallback(from string, _ error, to string) {
	l.hops = append(l.hops, from+"->"+to)
}

func chain(seen *[]string, errs ...error) []FallbackEntry {
	names := []string{"primary", "secondary", "tertiary"}
	out := make([]FallbackEntry, len(errs))
	for i, err := range errs {
		out[i] = FallbackEntry{Model: names[i], Provider: &fakeProvider{kind: ProviderKind(i), err: err, seen: seen}}
	}
	return out
}

func status(code int) error {
	return apitypes.NewApiError(code, "err", "msg", "")
}

func TestFallbackReturnsFirstSuccessWithoutTouchingTheRest(t *testing.T) {
	var seen []string
	log := &fallbackLog{}
	fp := NewFallbackProvider(chain(&seen, nil, nil), log)

	resp, err := fp.SendMessage(context.Background(), userRequest("ignored"))
	if err != nil || resp.Model != "primary" {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
	if !reflect.DeepEqual(seen, []string{"primary"}) {
		t.Errorf("tried %v, want only primary", seen)
	}
	if len(log.hops) != 0 {
		t.Errorf("no fallback should be logged: %v", log.hops)
	}
}

func TestFallbackRequestModelIsOverriddenPerEntry(t *testing.T) {
	// The caller's req.Model is whatever the user typed; each entry must be
	// asked for its own model or the chain is a no-op.
	var seen []string
	fp := NewFallbackProvider(chain(&seen, status(503), nil), nil)
	resp, err := fp.SendMessage(context.Background(), userRequest("user-typed"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(seen, []string{"primary", "secondary"}) || resp.Model != "secondary" {
		t.Errorf("seen=%v resp.Model=%s", seen, resp.Model)
	}
}

func TestFallbackFallsThroughOnRetryableAndLogsEachHop(t *testing.T) {
	var seen []string
	log := &fallbackLog{}
	fp := NewFallbackProvider(chain(&seen, status(429), status(502), nil), log)

	if _, err := fp.SendMessage(context.Background(), userRequest("x")); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(seen, []string{"primary", "secondary", "tertiary"}) {
		t.Errorf("order = %v", seen)
	}
	if !reflect.DeepEqual(log.hops, []string{"primary->secondary", "secondary->tertiary"}) {
		t.Errorf("hops = %v", log.hops)
	}
}

func TestFallbackStopsOnNonRetryableError(t *testing.T) {
	var seen []string
	log := &fallbackLog{}
	bad := status(400)
	fp := NewFallbackProvider(chain(&seen, bad, nil), log)

	_, err := fp.SendMessage(context.Background(), userRequest("x"))
	if !errors.Is(err, bad) {
		t.Fatalf("want the 400 back, got %v", err)
	}
	if len(seen) != 1 || len(log.hops) != 0 {
		t.Errorf("a 400 must not fall through: seen=%v hops=%v", seen, log.hops)
	}
}

func TestFallbackReturnsLastErrorWhenEveryEntryFailsAndDoesNotLogPastTheEnd(t *testing.T) {
	var seen []string
	log := &fallbackLog{}
	last := status(504)
	fp := NewFallbackProvider(chain(&seen, status(429), last), log)

	_, err := fp.SendMessage(context.Background(), userRequest("x"))
	if !errors.Is(err, last) {
		t.Fatalf("want last error, got %v", err)
	}
	// One hop was possible; there is no entry after secondary to hop to.
	if !reflect.DeepEqual(log.hops, []string{"primary->secondary"}) {
		t.Errorf("hops = %v", log.hops)
	}
}

func TestFallbackStreamMessageUsesTheSameChain(t *testing.T) {
	var seen []string
	log := &fallbackLog{}
	fp := NewFallbackProvider(chain(&seen, status(500), nil), log)

	ch, err := fp.StreamMessage(context.Background(), userRequest("x"))
	if err != nil {
		t.Fatal(err)
	}
	events := collect(t, ch)
	if len(events) != 1 || events[0].Message.Model != "secondary" {
		t.Errorf("stream came from %v", events)
	}
	if !reflect.DeepEqual(log.hops, []string{"primary->secondary"}) {
		t.Errorf("hops = %v", log.hops)
	}

	var seen2 []string
	bad := status(401)
	_, err = NewFallbackProvider(chain(&seen2, bad, nil), nil).StreamMessage(context.Background(), userRequest("x"))
	if !errors.Is(err, bad) || len(seen2) != 1 {
		t.Errorf("stream must stop on non-retryable: err=%v seen=%v", err, seen2)
	}
}

func TestFallbackNilLoggerIsSafe(t *testing.T) {
	var seen []string
	fp := NewFallbackProvider(chain(&seen, status(429), nil), nil)
	if _, err := fp.SendMessage(context.Background(), userRequest("x")); err != nil {
		t.Fatal(err)
	}
}

func TestFallbackKindComesFromTheHeadOfTheChain(t *testing.T) {
	var seen []string
	entries := chain(&seen, nil, nil)
	entries[0].Provider.(*fakeProvider).kind = ProviderGemini
	if k := NewFallbackProvider(entries, nil).Kind(); k != ProviderGemini {
		t.Errorf("kind = %v", k)
	}
	if k := NewFallbackProvider(nil, nil).Kind(); k != ProviderAnthropic {
		t.Errorf("empty chain kind = %v, want Anthropic default", k)
	}
}

func TestIsFallbackRetryableDecisionTable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"plain error is not an ApiError", errors.New("dial tcp: refused"), false},
		{"429", status(429), true},
		{"500", status(500), true},
		{"502", status(502), true},
		{"503", status(503), true},
		{"504", status(504), true},
		{"400", status(400), false},
		{"401", status(401), false},
		{"403", status(403), false},
		{"404", status(404), false},
		// 408/409 are retried inside the provider but are not a reason to
		// switch models.
		{"408 is provider-retry only", status(408), false},
		{"409 is provider-retry only", status(409), false},
		{"anthropic context window in type", apitypes.NewApiError(400, "context_window_exceeded", "too long", ""), true},
		{"openai context length in code", &apitypes.ApiError{Kind: apitypes.ErrApi, Status: 400, ErrorType: "invalid_request_error", Code: "context_length_exceeded"}, true},
		{"unrelated 400 code", &apitypes.ApiError{Kind: apitypes.ErrApi, Status: 400, ErrorType: "invalid_request_error", Code: "invalid_api_key"}, false},
		{"retries exhausted wrapping 429", apitypes.NewRetriesExhausted(3, status(429)), true},
		{"retries exhausted wrapping 503", apitypes.NewRetriesExhausted(3, status(503)), true},
		{"retries exhausted wrapping transport error", apitypes.NewRetriesExhausted(3, apitypes.WrapHttp(errors.New("reset"))), false},
		{"retries exhausted with nil cause", apitypes.NewRetriesExhausted(3, nil), false},
		{"http transport error alone", apitypes.WrapHttp(errors.New("reset")), false},
		{"missing credentials", apitypes.NewMissingCredentials("X", "X_KEY"), false},
	}
	for _, tc := range cases {
		if got := isFallbackRetryable(tc.err); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestModelRouterRoutesKnownCategoriesAndRejectsUnknown(t *testing.T) {
	var seen []string
	deep := NewFallbackProvider(chain(&seen, nil), nil)
	r := NewModelRouter(map[TaskCategory]*FallbackProvider{CategoryDeep: deep})

	got, err := r.Route(CategoryDeep)
	if err != nil || got != deep {
		t.Errorf("Route(deep) = %v, %v", got, err)
	}
	if _, err := r.Route(CategoryUltrabrain); err == nil {
		t.Error("unconfigured category must error, not return nil")
	}
}
