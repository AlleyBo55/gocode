package main

import (
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func postTo(t *testing.T, base, key, path, body string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest("POST", base+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("x-api-key", key)
	req.Header.Set("content-type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, string(data)
}

func TestMeterSwapsRunKeyForRealKeyAndRefusesUnknownKeys(t *testing.T) {
	var gotKey, gotAuth, gotPath string
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		gotKey, gotAuth, gotPath = r.Header.Get("X-Api-Key"), r.Header.Get("Authorization"), r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"model":"claude-sonnet-5-5","usage":{"input_tokens":1000,"output_tokens":100}}`)
	}))
	defer upstream.Close()
	m, err := newMeter(upstream.URL, "sk-real-key", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	token := m.newRun()
	// A base URL configured with /v1 and an SDK that appends /v1 again
	// must still reach /v1/messages.
	resp, _ := postTo(t, m.URL, token, "/v1/v1/messages", `{}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if gotKey != "sk-real-key" || gotAuth != "" || gotPath != "/v1/messages" {
		t.Errorf("upstream saw key=%q auth=%q path=%q; want the real key, no Authorization, /v1/messages", gotKey, gotAuth, gotPath)
	}

	for _, key := range []string{"sk-ant-api03-guess", ""} {
		resp, _ := postTo(t, m.URL, key, "/v1/messages", `{}`)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("key %q: status = %d, want 401", key, resp.StatusCode)
		}
	}
	used := m.finishRun(token)
	if resp, _ := postTo(t, m.URL, token, "/v1/messages", `{}`); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("a finished run's key still works: status %d", resp.StatusCode)
	}
	if calls != 1 {
		t.Errorf("upstream called %d times; refused requests must never be forwarded", calls)
	}
	if want := (1000*2.0 + 100*10.0) / 1e6; used.Requests != 1 || math.Abs(used.CostUSD-want) > 1e-12 {
		t.Errorf("usage = %+v, want 1 request costing %v", used, want)
	}
}

func TestMeterPricesStreamedUsageWithCacheTiers(t *testing.T) {
	stream := "event: message_start\n" +
		`data: {"type":"message_start","message":{"model":"claude-sonnet-5-5","usage":{"input_tokens":10,"cache_creation_input_tokens":1000,"cache_read_input_tokens":5000,"cache_creation":{"ephemeral_5m_input_tokens":600,"ephemeral_1h_input_tokens":400},"output_tokens":1}}}` + "\n\n" +
		"event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}` + "\n\n" +
		"event: message_delta\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":200}}` + "\n\n" +
		"event: message_stop\n" +
		`data: {"type":"message_stop"}` + "\n\n"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, stream)
	}))
	defer upstream.Close()
	m, err := newMeter(upstream.URL, "sk-real-key", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	token := m.newRun()
	_, body := postTo(t, m.URL, token, "/v1/messages", `{"stream":true}`)
	if body != stream {
		t.Errorf("the agent must receive the stream byte for byte; got:\n%s", body)
	}
	u := m.finishRun(token)
	// $2 input, $2.50 5-minute write, $4 1-hour write, $0.10 read, $10 output.
	want := (10*2.0 + 600*2.5 + 400*4.0 + 5000*0.10 + 200*10.0) / 1e6
	if math.Abs(u.CostUSD-want) > 1e-12 {
		t.Errorf("cost = %v, want %v", u.CostUSD, want)
	}
	if u.Requests != 1 || u.InputTokens != 10 || u.CacheWrite != 1000 || u.CacheRead != 5000 || u.OutputTokens != 200 {
		t.Errorf("usage = %+v", u)
	}
}

func TestMeterStopsForwardingOnceTheBudgetIsSpent(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		// 1M output tokens on Sonnet 5.5 is $10.
		_, _ = io.WriteString(w, `{"model":"claude-sonnet-5-5","usage":{"input_tokens":0,"output_tokens":1000000}}`)
	}))
	defer upstream.Close()
	m, err := newMeter(upstream.URL, "sk-real-key", 5)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	token := m.newRun()
	if resp, _ := postTo(t, m.URL, token, "/v1/messages", `{}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("first request: status %d", resp.StatusCode)
	}
	if !m.Exhausted() {
		t.Fatalf("spent $%.2f of a $5 budget but not exhausted", m.Spent())
	}
	resp, body := postTo(t, m.URL, token, "/v1/messages", `{}`)
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "budget") {
		t.Errorf("over budget: status %d body %s; want a 400 naming the budget", resp.StatusCode, body)
	}
	if calls != 1 {
		t.Errorf("upstream called %d times after the budget was spent", calls)
	}
}
