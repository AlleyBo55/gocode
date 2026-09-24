package apiclient

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/AlleyBo55/gocode/internal/apitypes"
)

// providerEnv is every variable the resolution code reads. Tests clear all of
// them so the developer's own shell cannot change the outcome of a table case.
var providerEnv = []string{
	"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_BASE_URL",
	"OPENAI_API_KEY", "OPENAI_BASE_URL",
	"GEMINI_API_KEY", "GOOGLE_API_KEY", "GEMINI_BASE_URL",
	"XAI_API_KEY", "XAI_BASE_URL",
	"DEEPSEEK_API_KEY", "MISTRAL_API_KEY", "TOGETHER_API_KEY", "GROQ_API_KEY",
	"OPENROUTER_API_KEY", "AZURE_OPENAI_API_KEY", "AZURE_OPENAI_ENDPOINT",
	"CODEX_HOME", "CODEX_BASE_URL", "OLLAMA_HOST", promptCacheDisableEnv,
}

func clearProviderEnv(t *testing.T) {
	t.Helper()
	for _, k := range providerEnv {
		t.Setenv(k, "")
	}
}

// fixture loads a recorded wire capture from testdata. Fixtures are the real
// frame shapes each provider sends; edit them only against a fresh capture.
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return data
}

type capturedRequest struct {
	Path   string
	Header http.Header
	Body   []byte
}

// recorder keeps every request a test server saw so tests can assert on the
// wire request as well as the parsed response.
type recorder struct {
	mu   sync.Mutex
	reqs []capturedRequest
}

func (r *recorder) record(req *http.Request) {
	body, _ := io.ReadAll(req.Body)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reqs = append(r.reqs, capturedRequest{Path: req.URL.Path, Header: req.Header.Clone(), Body: body})
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.reqs)
}

func (r *recorder) last(t *testing.T) capturedRequest {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.reqs) == 0 {
		t.Fatal("no requests recorded")
	}
	return r.reqs[len(r.reqs)-1]
}

func newServer(t *testing.T, rec *recorder, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		rec.record(req)
		h(w, req)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// writeStream sends body as an event stream in flushed pieces of the given
// size, so the client's read loop sees frames cut at arbitrary offsets the way
// a real socket delivers them.
func writeStream(w http.ResponseWriter, body []byte, piece int) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	fl, _ := w.(http.Flusher)
	for i := 0; i < len(body); i += piece {
		end := min(i+piece, len(body))
		_, _ = w.Write(body[i:end])
		if fl != nil {
			fl.Flush()
		}
	}
}

// writeStreamThenDrop sends a prefix of the stream and then kills the
// connection without a terminating chunk, which is what a proxy timeout or a
// dropped socket looks like to the client.
func writeStreamThenDrop(w http.ResponseWriter, body []byte) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
	if fl, ok := w.(http.Flusher); ok {
		fl.Flush()
	}
	panic(http.ErrAbortHandler)
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

// collect drains a stream channel until it closes, failing the test if the
// producer never closes it.
func collect(t *testing.T, ch <-chan apitypes.StreamEvent) []apitypes.StreamEvent {
	t.Helper()
	var out []apitypes.StreamEvent
	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, ev)
		case <-deadline:
			t.Fatalf("stream did not close; %d events so far: %v", len(out), kinds(out))
		}
	}
}

func kinds(events []apitypes.StreamEvent) []string {
	out := make([]string, len(events))
	for i, ev := range events {
		out[i] = ev.Kind
	}
	return out
}

func findKind(events []apitypes.StreamEvent, kind string) *apitypes.StreamEvent {
	for i := range events {
		if events[i].Kind == kind {
			return &events[i]
		}
	}
	return nil
}

func hasKind(events []apitypes.StreamEvent, kind string) bool {
	return findKind(events, kind) != nil
}

// textOf concatenates text deltas for one content block, as the runtime does.
func textOf(events []apitypes.StreamEvent, index int) string {
	var s string
	for _, ev := range events {
		if ev.Kind == "content_block_delta" && ev.Index == index && ev.BlockDelta != nil && ev.BlockDelta.Kind == "text_delta" {
			s += ev.BlockDelta.Text
		}
	}
	return s
}

// toolInputOf reassembles input_json_delta partials for one content block,
// mirroring the runtime's appendJSON accumulation.
func toolInputOf(events []apitypes.StreamEvent, index int) string {
	var s string
	for _, ev := range events {
		if ev.Kind == "content_block_delta" && ev.Index == index && ev.BlockDelta != nil && ev.BlockDelta.Kind == "input_json_delta" {
			s += ev.BlockDelta.PartialJSON
		}
	}
	return s
}

// blockStart returns the content_block_start event for an index, if any.
func blockStart(events []apitypes.StreamEvent, index int) *apitypes.OutputContentBlock {
	for _, ev := range events {
		if ev.Kind == "content_block_start" && ev.Index == index && ev.ContentBlock != nil {
			return ev.ContentBlock
		}
	}
	return nil
}

// fastRetry keeps the retry tests under a few milliseconds while still
// exercising the real backoff arithmetic.
func fastRetry() apitypes.RetryConfig {
	return apitypes.RetryConfig{MaxRetries: 2, InitialBackoff: time.Millisecond, MaxBackoff: 2 * time.Millisecond}
}

func userRequest(model string) apitypes.MessageRequest {
	return apitypes.MessageRequest{
		Model:     model,
		MaxTokens: 256,
		Messages:  []apitypes.InputMessage{apitypes.UserText("hi")},
	}
}
