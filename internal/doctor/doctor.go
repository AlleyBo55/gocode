// Package doctor probes what a model can actually do through gocode's
// provider layer: stream, honour a system prompt, call a tool, and call
// several tools in one turn. The answers turn "it broke with model X" into
// "X does not support parallel tool calls", and they are cached per model
// so the probe spends tokens once, not on every run.
package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/AlleyBo55/gocode/internal/apiclient"
	"github.com/AlleyBo55/gocode/internal/apitypes"
)

// Check names, in the order they run.
const (
	CheckStreaming    = "streaming"
	CheckSystemPrompt = "system prompt"
	CheckToolCall     = "tool call"
	CheckParallel     = "parallel tool calls"
)

// Check is one probed capability.
type Check struct {
	Name    string        `json:"name"`
	OK      bool          `json:"ok"`
	Detail  string        `json:"detail,omitempty"`
	Latency time.Duration `json:"latency_ns"`
}

// Report is the result of probing one model.
type Report struct {
	Model    string         `json:"model"`
	ProbedAt time.Time      `json:"probed_at"`
	Checks   []Check        `json:"checks"`
	Usage    apitypes.Usage `json:"usage"` // tokens the probe itself spent
}

// Supports reports whether a named check passed. known is false when the
// report has no such check.
func (r Report) Supports(name string) (ok, known bool) {
	for _, c := range r.Checks {
		if c.Name == name {
			return c.OK, true
		}
	}
	return false, false
}

// AllOK is true when every check passed.
func (r Report) AllOK() bool {
	for _, c := range r.Checks {
		if !c.OK {
			return false
		}
	}
	return len(r.Checks) > 0
}

// Render formats the report in the same ✓/✗ style as the rest of doctor.
func (r Report) Render() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Model: %s   probed %s   probe cost: %d in / %d out tokens",
		r.Model, r.ProbedAt.Local().Format("2006-01-02 15:04"), r.Usage.InputTokens, r.Usage.OutputTokens)
	width := 0
	for _, c := range r.Checks {
		width = max(width, len(c.Name))
	}
	for _, c := range r.Checks {
		mark := "✓"
		if !c.OK {
			mark = "✗"
		}
		fmt.Fprintf(&b, "\n  %s %-*s  %s", mark, width, c.Name, c.Detail)
	}
	return b.String()
}

// Prober runs the capability checks against one provider and model.
type Prober struct {
	Provider apiclient.Provider
	Model    string
	// Timeout bounds each check. Zero means 60 seconds.
	Timeout time.Duration
	// MaxTokens caps each probe reply. Zero means 256, which is plenty for a
	// one-word answer or two tool calls and keeps a runaway model cheap.
	MaxTokens int
}

// Run executes every check in order and never returns an error: a failure
// to complete a check is that check's result.
func (p Prober) Run(ctx context.Context) Report {
	rep := Report{Model: p.Model, ProbedAt: time.Now()}
	add := func(c Check, u apitypes.Usage) {
		rep.Checks = append(rep.Checks, c)
		rep.Usage.InputTokens += u.InputTokens
		rep.Usage.OutputTokens += u.OutputTokens
		rep.Usage.CacheCreationInputTokens += u.CacheCreationInputTokens
		rep.Usage.CacheReadInputTokens += u.CacheReadInputTokens
	}

	add(p.checkStreaming(ctx))
	add(p.checkSystemPrompt(ctx))
	tool, u := p.checkToolCall(ctx)
	add(tool, u)
	if !tool.OK {
		add(Check{Name: CheckParallel, Detail: "skipped: single tool call did not work"}, apitypes.Usage{})
	} else {
		add(p.checkParallel(ctx))
	}
	return rep
}

func (p Prober) timeout() time.Duration {
	if p.Timeout > 0 {
		return p.Timeout
	}
	return 60 * time.Second
}

func (p Prober) request(prompt string) apitypes.MessageRequest {
	maxTokens := p.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 256
	}
	return apitypes.MessageRequest{
		Model:     p.Model,
		MaxTokens: maxTokens,
		Messages:  []apitypes.InputMessage{apitypes.UserText(prompt)},
	}
}

func (p Prober) checkStreaming(parent context.Context) (Check, apitypes.Usage) {
	ctx, cancel := context.WithTimeout(parent, p.timeout())
	defer cancel()
	c := Check{Name: CheckStreaming}
	start := time.Now()

	ch, err := p.Provider.StreamMessage(ctx, p.request("Reply with the single word OK and nothing else."))
	if err != nil {
		c.Detail = "request failed: " + err.Error()
		c.Latency = time.Since(start)
		return c, apitypes.Usage{}
	}

	var (
		usage      apitypes.Usage
		text       strings.Builder
		firstToken time.Duration
		sawStop    bool
		errText    string
	)
	for ev := range ch {
		switch ev.Kind {
		case "message_start":
			if ev.Message != nil {
				usage = ev.Message.Usage
			}
		case "content_block_delta":
			if ev.BlockDelta != nil && ev.BlockDelta.Kind == "text_delta" && ev.BlockDelta.Text != "" {
				if firstToken == 0 {
					firstToken = time.Since(start)
				}
				text.WriteString(ev.BlockDelta.Text)
			}
		case "message_delta":
			if ev.DeltaUsage != nil {
				usage = mergeUsage(usage, *ev.DeltaUsage)
			}
		case "message_stop":
			sawStop = true
		case "error":
			if ev.BlockDelta != nil {
				errText = ev.BlockDelta.Text
			} else {
				errText = "stream reported an error"
			}
		}
	}
	c.Latency = time.Since(start)

	switch {
	case errText != "":
		c.Detail = errText
	case text.Len() == 0:
		c.Detail = "stream closed without any text"
	case !sawStop:
		c.Detail = "stream ended without message_stop; the reply may have been cut off"
	case ctx.Err() != nil:
		c.Detail = "timed out after " + p.timeout().String()
	default:
		c.OK = true
		c.Detail = fmt.Sprintf("first token in %s", firstToken.Round(time.Millisecond))
	}
	return c, usage
}

func (p Prober) checkSystemPrompt(parent context.Context) (Check, apitypes.Usage) {
	ctx, cancel := context.WithTimeout(parent, p.timeout())
	defer cancel()
	c := Check{Name: CheckSystemPrompt}
	start := time.Now()

	req := p.request("hello")
	req.System = "You are a test harness. Whatever the user says, reply with exactly the word PINEAPPLE and nothing else."
	resp, err := p.Provider.SendMessage(ctx, req)
	c.Latency = time.Since(start)
	if err != nil {
		c.Detail = "request failed: " + err.Error()
		return c, apitypes.Usage{}
	}
	reply := textOf(resp)
	if strings.Contains(strings.ToUpper(reply), "PINEAPPLE") {
		c.OK = true
		c.Detail = "honoured"
	} else {
		c.Detail = "ignored; replied " + quoteShort(reply)
	}
	return c, resp.Usage
}

func (p Prober) checkToolCall(parent context.Context) (Check, apitypes.Usage) {
	ctx, cancel := context.WithTimeout(parent, p.timeout())
	defer cancel()
	c := Check{Name: CheckToolCall}
	start := time.Now()

	req := p.request("What is the weather in Paris right now? You must call the get_weather tool to find out; do not answer from memory.")
	req.Tools = []apitypes.ToolDef{weatherTool()}
	resp, err := p.Provider.SendMessage(ctx, req)
	c.Latency = time.Since(start)
	if err != nil {
		c.Detail = "request failed: " + err.Error()
		return c, apitypes.Usage{}
	}

	calls := weatherCalls(resp)
	if len(calls) == 0 {
		c.Detail = fmt.Sprintf("no tool_use block (stop_reason=%s); replied %s", resp.StopReason, quoteShort(textOf(resp)))
		return c, resp.Usage
	}
	args := calls[0].Input
	var parsed map[string]any
	if err := json.Unmarshal(args, &parsed); err != nil {
		c.Detail = "arguments were not a JSON object: " + quoteShort(string(args))
		return c, resp.Usage
	}
	if raw, ok := parsed["raw"]; ok && len(parsed) == 1 {
		// The provider layer wraps unparsable arguments as {"raw": "..."}.
		c.Detail = "arguments were not valid JSON: " + quoteShort(fmt.Sprint(raw))
		return c, resp.Usage
	}
	if _, ok := parsed["city"]; !ok {
		c.Detail = "arguments missed the required \"city\" field: " + compactJSON(args)
		return c, resp.Usage
	}
	c.OK = true
	c.Detail = "arguments " + compactJSON(args)
	return c, resp.Usage
}

// compactJSON renders a JSON value on one line without re-quoting it, so a
// tool's arguments read as JSON in the report rather than as an escaped string.
func compactJSON(raw json.RawMessage) string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return quoteShort(string(raw))
	}
	out, err := json.Marshal(v)
	if err != nil {
		return quoteShort(string(raw))
	}
	s := string(out)
	if len(s) > 80 {
		s = s[:79] + "…"
	}
	return s
}

func (p Prober) checkParallel(parent context.Context) (Check, apitypes.Usage) {
	ctx, cancel := context.WithTimeout(parent, p.timeout())
	defer cancel()
	c := Check{Name: CheckParallel}
	start := time.Now()

	req := p.request("Get the current weather for Paris and for Tokyo. Call get_weather once for each city, both in this same response.")
	req.Tools = []apitypes.ToolDef{weatherTool()}
	resp, err := p.Provider.SendMessage(ctx, req)
	c.Latency = time.Since(start)
	if err != nil {
		c.Detail = "request failed: " + err.Error()
		return c, apitypes.Usage{}
	}

	switch n := len(weatherCalls(resp)); {
	case n >= 2:
		c.OK = true
		c.Detail = fmt.Sprintf("%d calls in one turn", n)
	case n == 1:
		c.Detail = "1 call in one turn: this model serialises tool use, expect more round trips"
	default:
		c.Detail = "no tool call when asked for two"
	}
	return c, resp.Usage
}

func weatherTool() apitypes.ToolDef {
	return apitypes.ToolDef{
		Name:        "get_weather",
		Description: "Get the current weather for a city.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string","description":"City name"}},"required":["city"]}`),
	}
}

func weatherCalls(resp *apitypes.MessageResponse) []apitypes.OutputContentBlock {
	var out []apitypes.OutputContentBlock
	for _, b := range resp.Content {
		if b.Kind == "tool_use" && strings.EqualFold(b.Name, "get_weather") {
			out = append(out, b)
		}
	}
	return out
}

func textOf(resp *apitypes.MessageResponse) string {
	var b strings.Builder
	for _, block := range resp.Content {
		if block.Kind == "text" {
			b.WriteString(block.Text)
		}
	}
	return strings.TrimSpace(b.String())
}

func quoteShort(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return `""`
	}
	const limit = 80
	if len(s) > limit {
		s = s[:limit-1] + "…"
	}
	return fmt.Sprintf("%q", s)
}

// mergeUsage layers message_delta usage over message_start usage; non-zero
// delta counters are cumulative totals and replace the base.
func mergeUsage(base, delta apitypes.Usage) apitypes.Usage {
	pick := func(b, d int) int {
		if d != 0 {
			return d
		}
		return b
	}
	return apitypes.Usage{
		InputTokens:              pick(base.InputTokens, delta.InputTokens),
		CacheCreationInputTokens: pick(base.CacheCreationInputTokens, delta.CacheCreationInputTokens),
		CacheReadInputTokens:     pick(base.CacheReadInputTokens, delta.CacheReadInputTokens),
		OutputTokens:             pick(base.OutputTokens, delta.OutputTokens),
	}
}
