package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/AlleyBo55/gocode/internal/apitypes"
)

// OpenAiCompatConfig configures an OpenAI-compatible provider.
type OpenAiCompatConfig struct {
	ProviderName string
	BaseURLEnv   string
	DefaultBase  string
}

// OpenAiCompatProvider communicates with OpenAI-compatible APIs (xAI, OpenAI).
type OpenAiCompatProvider struct {
	Config  OpenAiCompatConfig
	BaseURL string
	Auth    apitypes.AuthSource
	Client  *http.Client
	Retry   apitypes.RetryConfig
}

// NewOpenAiCompatProvider creates a new OpenAI-compatible provider.
func NewOpenAiCompatProvider(config OpenAiCompatConfig, auth apitypes.AuthSource) *OpenAiCompatProvider {
	baseURL := os.Getenv(config.BaseURLEnv)
	if baseURL == "" {
		baseURL = config.DefaultBase
	}
	return &OpenAiCompatProvider{
		Config:  config,
		BaseURL: baseURL,
		Auth:    auth,
		Client:  &http.Client{Timeout: 5 * time.Minute},
		Retry:   apitypes.DefaultRetryConfig(),
	}
}

func (p *OpenAiCompatProvider) Kind() ProviderKind {
	if p.Config.ProviderName == "xAI" {
		return ProviderXai
	}
	return ProviderOpenAi
}

// SendMessage sends a non-streaming request, translating to/from OpenAI format.
func (p *OpenAiCompatProvider) SendMessage(ctx context.Context, req apitypes.MessageRequest) (*apitypes.MessageResponse, error) {
	req.Stream = false
	resp, err := p.sendWithRetry(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	requestID := requestIDFromHeaders(resp.Header)
	var chatResp chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, apitypes.WrapJson(err)
	}
	msgResp, err := normalizeResponse(req.Model, chatResp)
	if err != nil {
		return nil, err
	}
	if msgResp.RequestID == "" {
		msgResp.RequestID = requestID
	}
	return msgResp, nil
}

// StreamMessage sends a streaming request, translating OpenAI chunks to unified StreamEvents.
func (p *OpenAiCompatProvider) StreamMessage(ctx context.Context, req apitypes.MessageRequest) (<-chan apitypes.StreamEvent, error) {
	req.Stream = true
	resp, err := p.sendWithRetry(ctx, req)
	if err != nil {
		return nil, err
	}
	ch := make(chan apitypes.StreamEvent, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		state := newStreamState(req.Model)
		parser := newOpenAiSseParser()
		buf := make([]byte, 4096)
		for {
			n, readErr := resp.Body.Read(buf)
			if n > 0 {
				chunks, parseErr := parser.push(buf[:n])
				if parseErr != nil {
					return
				}
				for _, chunk := range chunks {
					events := state.ingestChunk(chunk)
					for _, ev := range events {
						select {
						case ch <- ev:
						case <-ctx.Done():
							return
						}
					}
				}
			}
			if readErr != nil {
				for _, ev := range state.finish() {
					select {
					case ch <- ev:
					case <-ctx.Done():
						return
					}
				}
				return
			}
		}
	}()
	return ch, nil
}

func (p *OpenAiCompatProvider) sendWithRetry(ctx context.Context, req apitypes.MessageRequest) (*http.Response, error) {
	var lastErr error
	for attempt := 1; attempt <= p.Retry.MaxRetries+1; attempt++ {
		resp, err := p.sendRaw(ctx, req)
		if err != nil {
			if attempt <= p.Retry.MaxRetries {
				lastErr = err
				backoff, _ := p.Retry.BackoffForAttempt(attempt)
				time.Sleep(backoff)
				continue
			}
			return nil, apitypes.NewRetriesExhausted(attempt, err)
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return resp, nil
		}
		apiErr := readApiError(resp)
		if !apiErr.Retryable || attempt > p.Retry.MaxRetries {
			if attempt > p.Retry.MaxRetries && apiErr.Retryable {
				return nil, apitypes.NewRetriesExhausted(attempt, apiErr)
			}
			return nil, apiErr
		}
		lastErr = apiErr
		backoff, _ := p.Retry.BackoffForAttempt(attempt)
		time.Sleep(backoff)
	}
	return nil, apitypes.NewRetriesExhausted(p.Retry.MaxRetries+1, lastErr)
}

func (p *OpenAiCompatProvider) sendRaw(ctx context.Context, req apitypes.MessageRequest) (*http.Response, error) {
	payload := buildChatCompletionRequest(req)
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, apitypes.WrapJson(err)
	}
	url := chatCompletionsEndpoint(p.BaseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, apitypes.WrapHttp(err)
	}
	httpReq.Header.Set("content-type", "application/json")
	// OpenAI-compat uses Bearer auth
	if p.Auth.ApiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.Auth.ApiKey)
	}
	if p.Auth.BearerToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.Auth.BearerToken)
	}
	resp, err := p.Client.Do(httpReq)
	if err != nil {
		return nil, apitypes.WrapHttp(err)
	}
	return resp, nil
}

// --- Request Translation ---

func buildChatCompletionRequest(req apitypes.MessageRequest) map[string]interface{} {
	messages := []map[string]interface{}{}
	if req.System != "" {
		messages = append(messages, map[string]interface{}{"role": "system", "content": req.System})
	}
	for _, msg := range req.Messages {
		messages = append(messages, translateMessage(msg)...)
	}
	payload := map[string]interface{}{
		"model":    req.Model,
		"messages": messages,
		"stream":   req.Stream,
	}
	// Newer OpenAI models (o1, o3, o4, gpt-5.x) require max_completion_tokens
	// instead of max_tokens. Other providers and older models use max_tokens.
	if usesMaxCompletionTokens(req.Model) {
		payload["max_completion_tokens"] = req.MaxTokens
	} else {
		payload["max_tokens"] = req.MaxTokens
	}
	if len(req.Tools) > 0 {
		tools := make([]map[string]interface{}, len(req.Tools))
		for i, t := range req.Tools {
			tools[i] = openaiToolDef(t)
		}
		payload["tools"] = tools
	}
	if req.ToolChoice != nil {
		payload["tool_choice"] = openaiToolChoice(req.ToolChoice)
	}
	return payload
}

// usesMaxCompletionTokens returns true for models that require max_completion_tokens
// instead of max_tokens (newer OpenAI reasoning/frontier models).
func usesMaxCompletionTokens(model string) bool {
	m := strings.ToLower(model)
	return strings.HasPrefix(m, "o1") ||
		strings.HasPrefix(m, "o3") ||
		strings.HasPrefix(m, "o4") ||
		strings.Contains(m, "gpt-5") ||
		strings.Contains(m, "gpt-4.1") ||
		strings.Contains(m, "gpt-4o-2024-12") ||
		strings.Contains(m, "gpt-4o-2025")
}

func translateMessage(msg apitypes.InputMessage) []map[string]interface{} {
	if msg.Role == "assistant" {
		var text string
		var toolCalls []map[string]interface{}
		for _, block := range msg.Content {
			switch block.Kind {
			case "text":
				text += block.Text
			case "tool_use":
				toolCalls = append(toolCalls, map[string]interface{}{
					"id":   block.ID,
					"type": "function",
					"function": map[string]interface{}{
						"name":      block.Name,
						"arguments": string(block.Input),
					},
				})
			}
		}
		m := map[string]interface{}{"role": "assistant"}
		if text != "" {
			m["content"] = text
		}
		if len(toolCalls) > 0 {
			m["tool_calls"] = toolCalls
		}
		return []map[string]interface{}{m}
	}
	var result []map[string]interface{}
	for _, block := range msg.Content {
		switch block.Kind {
		case "text":
			result = append(result, map[string]interface{}{"role": "user", "content": block.Text})
		case "tool_result":
			result = append(result, map[string]interface{}{
				"role":         "tool",
				"tool_call_id": block.ToolUseID,
				"content":      block.Content,
			})
		}
	}
	return result
}

func openaiToolDef(t apitypes.ToolDef) map[string]interface{} {
	var schema interface{}
	_ = json.Unmarshal(t.InputSchema, &schema)
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        t.Name,
			"description": t.Description,
			"parameters":  schema,
		},
	}
}

func openaiToolChoice(tc *apitypes.ToolChoice) interface{} {
	switch tc.Kind {
	case "auto":
		return "auto"
	case "any":
		return "required"
	case "tool":
		return map[string]interface{}{"type": "function", "function": map[string]interface{}{"name": tc.Name}}
	default:
		return "auto"
	}
}

func chatCompletionsEndpoint(baseURL string) string {
	trimmed := strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(trimmed, "/chat/completions") {
		return trimmed
	}
	return trimmed + "/chat/completions"
}

// NormalizeFinishReason converts OpenAI finish reasons to Anthropic-style.
func NormalizeFinishReason(reason string) string {
	switch reason {
	case "stop":
		return "end_turn"
	case "tool_calls":
		return "tool_use"
	default:
		return reason
	}
}

// --- Response Types (OpenAI format) ---

type chatCompletionResponse struct {
	ID      string       `json:"id"`
	Model   string       `json:"model"`
	Choices []chatChoice `json:"choices"`
	Usage   *openaiUsage `json:"usage,omitempty"`
}

type chatChoice struct {
	Message      chatMessage `json:"message"`
	FinishReason string      `json:"finish_reason,omitempty"`
}

type chatMessage struct {
	Role      string             `json:"role"`
	Content   string             `json:"content,omitempty"`
	ToolCalls []responseToolCall `json:"tool_calls,omitempty"`
}

type responseToolCall struct {
	ID       string               `json:"id"`
	Function responseToolFunction `json:"function"`
}

type responseToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

func normalizeResponse(model string, resp chatCompletionResponse) (*apitypes.MessageResponse, error) {
	if len(resp.Choices) == 0 {
		return nil, &apitypes.ApiError{Kind: apitypes.ErrInvalidSseFrame, Message: "chat completion response missing choices"}
	}
	choice := resp.Choices[0]
	var content []apitypes.OutputContentBlock
	if choice.Message.Content != "" {
		content = append(content, apitypes.OutputContentBlock{Kind: "text", Text: choice.Message.Content})
	}
	for _, tc := range choice.Message.ToolCalls {
		content = append(content, apitypes.OutputContentBlock{
			Kind:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: parseToolArguments(tc.Function.Arguments),
		})
	}
	m := resp.Model
	if m == "" {
		m = model
	}
	usage := apitypes.Usage{}
	if resp.Usage != nil {
		usage.InputTokens = resp.Usage.PromptTokens
		usage.OutputTokens = resp.Usage.CompletionTokens
	}
	return &apitypes.MessageResponse{
		ID:         resp.ID,
		Type:       "message",
		Role:       choice.Message.Role,
		Content:    content,
		Model:      m,
		StopReason: NormalizeFinishReason(choice.FinishReason),
		Usage:      usage,
	}, nil
}

func parseToolArguments(args string) json.RawMessage {
	var v json.RawMessage
	if err := json.Unmarshal([]byte(args), &v); err != nil {
		return json.RawMessage(fmt.Sprintf(`{"raw":%q}`, args))
	}
	return v
}

// --- Streaming State Machine ---

type openAiSseParser struct {
	buffer []byte
}

func newOpenAiSseParser() *openAiSseParser { return &openAiSseParser{} }

func (p *openAiSseParser) push(chunk []byte) ([]chatCompletionChunk, error) {
	p.buffer = append(p.buffer, chunk...)
	var chunks []chatCompletionChunk
	for {
		frame := p.nextFrame()
		if frame == nil {
			break
		}
		trimmed := strings.TrimSpace(*frame)
		if trimmed == "" {
			continue
		}
		var dataLines []string
		for _, line := range strings.Split(trimmed, "\n") {
			line = strings.TrimRight(line, "\r")
			if strings.HasPrefix(line, ":") {
				continue
			}
			if after, ok := strings.CutPrefix(line, "data:"); ok {
				dataLines = append(dataLines, strings.TrimSpace(after))
			}
		}
		if len(dataLines) == 0 {
			continue
		}
		payload := strings.Join(dataLines, "\n")
		if payload == "[DONE]" {
			continue
		}
		var c chatCompletionChunk
		if err := json.Unmarshal([]byte(payload), &c); err != nil {
			return chunks, apitypes.WrapJson(err)
		}
		chunks = append(chunks, c)
	}
	return chunks, nil
}

func (p *openAiSseParser) nextFrame() *string {
	if idx := bytes.Index(p.buffer, []byte("\n\n")); idx >= 0 {
		frame := string(p.buffer[:idx])
		p.buffer = p.buffer[idx+2:]
		return &frame
	}
	if idx := bytes.Index(p.buffer, []byte("\r\n\r\n")); idx >= 0 {
		frame := string(p.buffer[:idx])
		p.buffer = p.buffer[idx+4:]
		return &frame
	}
	return nil
}

type chatCompletionChunk struct {
	ID      string        `json:"id"`
	Model   string        `json:"model,omitempty"`
	Choices []chunkChoice `json:"choices,omitempty"`
	Usage   *openaiUsage  `json:"usage,omitempty"`
}

type chunkChoice struct {
	Delta        chunkDelta `json:"delta"`
	FinishReason string     `json:"finish_reason,omitempty"`
}

type chunkDelta struct {
	Content   string          `json:"content,omitempty"`
	ToolCalls []deltaToolCall `json:"tool_calls,omitempty"`
}

type deltaToolCall struct {
	Index    int           `json:"index"`
	ID       string        `json:"id,omitempty"`
	Function deltaFunction `json:"function"`
}

type deltaFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type streamState struct {
	model          string
	messageStarted bool
	textStarted    bool
	finished       bool
	stopReason     string
	usage          *apitypes.Usage
	toolCalls      map[int]*toolCallState
}

type toolCallState struct {
	index     int
	id        string
	name      string
	arguments string
	started   bool
	stopped   bool
}

func newStreamState(model string) *streamState {
	return &streamState{model: model, toolCalls: make(map[int]*toolCallState)}
}

func (s *streamState) ingestChunk(chunk chatCompletionChunk) []apitypes.StreamEvent {
	var events []apitypes.StreamEvent
	if !s.messageStarted {
		s.messageStarted = true
		m := chunk.Model
		if m == "" {
			m = s.model
		}
		events = append(events, apitypes.StreamEvent{
			Kind: "message_start",
			Message: &apitypes.MessageResponse{
				ID: chunk.ID, Type: "message", Role: "assistant", Model: m,
			},
		})
	}
	if chunk.Usage != nil {
		s.usage = &apitypes.Usage{InputTokens: chunk.Usage.PromptTokens, OutputTokens: chunk.Usage.CompletionTokens}
	}
	for _, choice := range chunk.Choices {
		if choice.Delta.Content != "" {
			if !s.textStarted {
				s.textStarted = true
				events = append(events, apitypes.StreamEvent{
					Kind: "content_block_start", Index: 0,
					ContentBlock: &apitypes.OutputContentBlock{Kind: "text"},
				})
			}
			events = append(events, apitypes.StreamEvent{
				Kind: "content_block_delta", Index: 0,
				BlockDelta: &apitypes.ContentBlockDelta{Kind: "text_delta", Text: choice.Delta.Content},
			})
		}
		for _, tc := range choice.Delta.ToolCalls {
			st, ok := s.toolCalls[tc.Index]
			if !ok {
				st = &toolCallState{index: tc.Index}
				s.toolCalls[tc.Index] = st
			}
			if tc.ID != "" {
				st.id = tc.ID
			}
			if tc.Function.Name != "" {
				st.name = tc.Function.Name
			}
			st.arguments += tc.Function.Arguments
			blockIdx := tc.Index + 1
			if !st.started && st.name != "" {
				st.started = true
				events = append(events, apitypes.StreamEvent{
					Kind: "content_block_start", Index: blockIdx,
					ContentBlock: &apitypes.OutputContentBlock{Kind: "tool_use", ID: st.id, Name: st.name, Input: json.RawMessage("{}")},
				})
			}
			if tc.Function.Arguments != "" && st.started {
				events = append(events, apitypes.StreamEvent{
					Kind: "content_block_delta", Index: blockIdx,
					BlockDelta: &apitypes.ContentBlockDelta{Kind: "input_json_delta", PartialJSON: tc.Function.Arguments},
				})
			}
		}
		if choice.FinishReason != "" {
			s.stopReason = NormalizeFinishReason(choice.FinishReason)
			for _, st := range s.toolCalls {
				if st.started && !st.stopped {
					st.stopped = true
					events = append(events, apitypes.StreamEvent{Kind: "content_block_stop", Index: st.index + 1})
				}
			}
		}
	}
	return events
}

func (s *streamState) finish() []apitypes.StreamEvent {
	if s.finished {
		return nil
	}
	s.finished = true
	var events []apitypes.StreamEvent
	if s.textStarted {
		events = append(events, apitypes.StreamEvent{Kind: "content_block_stop", Index: 0})
	}
	for _, st := range s.toolCalls {
		if st.started && !st.stopped {
			st.stopped = true
			events = append(events, apitypes.StreamEvent{Kind: "content_block_stop", Index: st.index + 1})
		}
	}
	if s.messageStarted {
		stopReason := s.stopReason
		if stopReason == "" {
			stopReason = "end_turn"
		}
		usage := apitypes.Usage{}
		if s.usage != nil {
			usage = *s.usage
		}
		events = append(events, apitypes.StreamEvent{
			Kind:       "message_delta",
			Delta:      &apitypes.DeltaPayload{StopReason: stopReason},
			DeltaUsage: &usage,
		})
		events = append(events, apitypes.StreamEvent{Kind: "message_stop"})
	}
	return events
}
