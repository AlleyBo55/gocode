package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
)

// Compare mode bills every agent through this proxy. Each agent run gets a
// throwaway API key; the proxy swaps it for the real key, forwards the
// request to Anthropic, and reads the usage the API reports in the response.
// The cost column is the API's own token counts at list price: one referee
// for every agent, whatever each CLI estimates for itself.

// anthropicPrice is a model's list price in USD per million tokens, split
// the way the Messages API reports usage.
type anthropicPrice struct {
	Input        float64 `json:"input"`
	Output       float64 `json:"output"`
	CacheWrite5m float64 `json:"cache_write_5m"`
	CacheWrite1h float64 `json:"cache_write_1h"`
	CacheRead    float64 `json:"cache_read"`
}

// anthropicPrices are Anthropic's list prices from
// platform.claude.com/docs/en/about-claude/pricing, checked 2026-11-02.
// Keys match model ids by longest prefix, so dated ids price correctly.
var anthropicPrices = map[string]anthropicPrice{
	"claude-fable-5-1":  {10, 50, 12.50, 20, 0.25},
	"claude-opus-5-5":   {4, 20, 5, 8, 0.20},
	"claude-sonnet-5-5": {2, 10, 2.50, 4, 0.10},
	"claude-haiku-5-5":  {0.10, 0.50, 0.125, 0.20, 0.01},
	"claude-fable-5":    {10, 50, 12.50, 20, 1},
	"claude-opus-5":     {5, 25, 6.25, 10, 0.50},
	"claude-sonnet-5":   {2, 10, 2.50, 4, 0.20},
	"claude-opus-4-8":   {5, 25, 6.25, 10, 0.50},
	"claude-opus-4-7":   {5, 25, 6.25, 10, 0.50},
	"claude-opus-4-6":   {5, 25, 6.25, 10, 0.50},
	"claude-opus-4-5":   {5, 25, 6.25, 10, 0.50},
	"claude-sonnet-4-6": {3, 15, 3.75, 6, 0.30},
	"claude-sonnet-4-5": {3, 15, 3.75, 6, 0.30},
	"claude-haiku-4-5":  {1, 5, 1.25, 2, 0.10},
}

// Haiku 5.5 is priced by prompt length: a request whose prompt (input plus
// cache reads and writes) is over 100,000 tokens pays these rates instead.
var haiku55LongPrompt = anthropicPrice{0.50, 2.50, 0.625, 1, 0.05}

func priceFor(model string, promptTokens int) (anthropicPrice, bool) {
	id := strings.ToLower(strings.TrimSpace(model))
	best := ""
	for k := range anthropicPrices {
		if strings.HasPrefix(id, k) && len(k) > len(best) {
			best = k
		}
	}
	if best == "" {
		return anthropicPrice{}, false
	}
	if best == "claude-haiku-5-5" && promptTokens > 100_000 {
		return haiku55LongPrompt, true
	}
	return anthropicPrices[best], true
}

// apiUsage is the usage object of a Messages API response.
type apiUsage struct {
	InputTokens              int `json:"input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreation            struct {
		Ephemeral5m int `json:"ephemeral_5m_input_tokens"`
		Ephemeral1h int `json:"ephemeral_1h_input_tokens"`
	} `json:"cache_creation"`
}

// merge folds a later usage report into u. Streamed counts are cumulative
// and a message_delta may omit fields message_start carried, so each counter
// keeps the larger value.
func (u *apiUsage) merge(o apiUsage) {
	u.InputTokens = max(u.InputTokens, o.InputTokens)
	u.CacheCreationInputTokens = max(u.CacheCreationInputTokens, o.CacheCreationInputTokens)
	u.CacheReadInputTokens = max(u.CacheReadInputTokens, o.CacheReadInputTokens)
	u.OutputTokens = max(u.OutputTokens, o.OutputTokens)
	u.CacheCreation.Ephemeral5m = max(u.CacheCreation.Ephemeral5m, o.CacheCreation.Ephemeral5m)
	u.CacheCreation.Ephemeral1h = max(u.CacheCreation.Ephemeral1h, o.CacheCreation.Ephemeral1h)
}

// cacheWrites splits cache-write tokens by TTL. Without the breakdown every
// write is priced at the default 5-minute TTL.
func (u apiUsage) cacheWrites() (fiveMin, oneHour int) {
	if b := u.CacheCreation; b.Ephemeral5m+b.Ephemeral1h > 0 {
		return b.Ephemeral5m, b.Ephemeral1h
	}
	return u.CacheCreationInputTokens, 0
}

func costOf(model string, u apiUsage) (float64, bool) {
	w5, w1 := u.cacheWrites()
	p, ok := priceFor(model, u.InputTokens+w5+w1+u.CacheReadInputTokens)
	if !ok {
		return 0, false
	}
	return (float64(u.InputTokens)*p.Input +
		float64(w5)*p.CacheWrite5m +
		float64(w1)*p.CacheWrite1h +
		float64(u.CacheReadInputTokens)*p.CacheRead +
		float64(u.OutputTokens)*p.Output) / 1_000_000, true
}

// MeteredUsage is what the proxy saw for one agent run.
type MeteredUsage struct {
	Requests     int
	InputTokens  int // uncached input
	CacheWrite   int
	CacheRead    int
	OutputTokens int
	CostUSD      float64
	Unpriced     []string // models the price table does not know
	HTTPErrors   int      // non-2xx responses from the API
}

type meter struct {
	upstream *url.URL
	realKey  string
	budget   float64
	proxy    *httputil.ReverseProxy
	srv      *http.Server
	URL      string // http://127.0.0.1:port, what agents use as their base URL

	mu    sync.Mutex
	runs  map[string]*MeteredUsage
	spent float64
}

type runCtxKey struct{}

// newMeter starts the proxy on a loopback port. A budget of 0 is unlimited.
func newMeter(upstream, realKey string, budget float64) (*meter, error) {
	target, err := url.Parse(upstream)
	if err != nil {
		return nil, err
	}
	m := &meter{upstream: target, realKey: realKey, budget: budget, runs: map[string]*MeteredUsage{}}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = 32
	m.proxy = &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(m.upstream)
			pr.Out.Header.Del("Authorization")
			pr.Out.Header.Set("X-Api-Key", m.realKey)
			// With no Accept-Encoding of its own the transport asks for
			// gzip and decompresses, so the usage below stays readable.
			pr.Out.Header.Del("Accept-Encoding")
		},
		ModifyResponse: m.meterResponse,
		Transport:      transport,
		FlushInterval:  -1,
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	m.URL = "http://" + ln.Addr().String()
	m.srv = &http.Server{Handler: m, ReadHeaderTimeout: 30 * time.Second}
	go func() { _ = m.srv.Serve(ln) }()
	return m, nil
}

func (m *meter) Close() error { return m.srv.Close() }

// newRun registers a run and returns the API key its agent should use.
func (m *meter) newRun() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	token := "sk-ant-api03-eval-" + hex.EncodeToString(b)
	m.mu.Lock()
	m.runs[token] = &MeteredUsage{}
	m.mu.Unlock()
	return token
}

// finishRun retires a run's key and returns what the run spent. Requests
// that arrive later with that key are refused.
func (m *meter) finishRun(token string) MeteredUsage {
	m.mu.Lock()
	defer m.mu.Unlock()
	u := m.runs[token]
	delete(m.runs, token)
	if u == nil {
		return MeteredUsage{}
	}
	out := *u
	out.Unpriced = slices.Clone(u.Unpriced)
	slices.Sort(out.Unpriced)
	return out
}

func (m *meter) Spent() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.spent
}

func (m *meter) Exhausted() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.budget > 0 && m.spent >= m.budget
}

func (m *meter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("X-Api-Key")
	if token == "" {
		token = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	m.mu.Lock()
	run := m.runs[token]
	exhausted := m.budget > 0 && m.spent >= m.budget
	m.mu.Unlock()
	switch {
	case run == nil:
		writeAPIError(w, http.StatusUnauthorized, "authentication_error", "unknown or finished eval run key")
		return
	case exhausted:
		writeAPIError(w, http.StatusBadRequest, "invalid_request_error",
			fmt.Sprintf("the eval budget of $%.2f is spent; the comparison stopped sending requests", m.budget))
		return
	}
	// Some SDKs take a base URL ending in /v1 and others append it, so an
	// agent configured either way reaches the same endpoint.
	for strings.HasPrefix(r.URL.Path, "/v1/v1/") {
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/v1")
	}
	r.URL.RawPath = ""
	m.proxy.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), runCtxKey{}, run)))
}

func writeAPIError(w http.ResponseWriter, status int, kind, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":  "error",
		"error": map[string]string{"type": kind, "message": msg},
	})
}

func (m *meter) meterResponse(resp *http.Response) error {
	run, _ := resp.Request.Context().Value(runCtxKey{}).(*MeteredUsage)
	if run == nil {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		m.mu.Lock()
		run.HTTPErrors++
		m.mu.Unlock()
		return nil
	}
	resp.Body = &meterBody{
		ReadCloser: resp.Body,
		sse:        strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream"),
		done:       func(model string, u apiUsage) { m.record(run, model, u) },
	}
	return nil
}

func (m *meter) record(run *MeteredUsage, model string, u apiUsage) {
	cost, priced := costOf(model, u)
	w5, w1 := u.cacheWrites()
	m.mu.Lock()
	defer m.mu.Unlock()
	run.Requests++
	run.InputTokens += u.InputTokens
	run.CacheWrite += w5 + w1
	run.CacheRead += u.CacheReadInputTokens
	run.OutputTokens += u.OutputTokens
	run.CostUSD += cost
	m.spent += cost
	if !priced && !slices.Contains(run.Unpriced, model) {
		run.Unpriced = append(run.Unpriced, model)
	}
}

// meterBody passes a response through unchanged while reading its usage:
// from the message_start and message_delta events when streamed, from the
// JSON body otherwise. Responses without usage (count_tokens, model lists)
// record nothing.
type meterBody struct {
	io.ReadCloser
	sse   bool
	buf   []byte
	model string
	usage apiUsage
	seen  bool
	done  func(model string, u apiUsage)
	once  sync.Once
}

const maxMeteredJSON = 16 << 20

func (b *meterBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.feed(p[:n])
	}
	if err != nil {
		b.finish()
	}
	return n, err
}

func (b *meterBody) Close() error {
	b.finish()
	return b.ReadCloser.Close()
}

func (b *meterBody) feed(p []byte) {
	if !b.sse {
		if len(b.buf)+len(p) <= maxMeteredJSON {
			b.buf = append(b.buf, p...)
		}
		return
	}
	b.buf = append(b.buf, p...)
	for {
		i := bytes.IndexByte(b.buf, '\n')
		if i < 0 {
			break
		}
		b.event(b.buf[:i])
		b.buf = b.buf[i+1:]
	}
}

func (b *meterBody) event(line []byte) {
	data, ok := bytes.CutPrefix(bytes.TrimRight(line, "\r"), []byte("data:"))
	if !ok {
		return
	}
	var ev struct {
		Type    string `json:"type"`
		Message struct {
			Model string   `json:"model"`
			Usage apiUsage `json:"usage"`
		} `json:"message"`
		Usage apiUsage `json:"usage"`
	}
	if json.Unmarshal(data, &ev) != nil {
		return
	}
	switch ev.Type {
	case "message_start":
		b.model = ev.Message.Model
		b.usage.merge(ev.Message.Usage)
		b.seen = true
	case "message_delta":
		b.usage.merge(ev.Usage)
		b.seen = true
	}
}

func (b *meterBody) finish() {
	b.once.Do(func() {
		if b.sse {
			if len(b.buf) > 0 {
				b.event(b.buf)
			}
		} else {
			var body struct {
				Model string    `json:"model"`
				Usage *apiUsage `json:"usage"`
			}
			if json.Unmarshal(b.buf, &body) == nil && body.Usage != nil {
				b.model, b.usage, b.seen = body.Model, *body.Usage, true
			}
		}
		b.buf = nil
		if b.seen {
			b.done(b.model, b.usage)
		}
	})
}
