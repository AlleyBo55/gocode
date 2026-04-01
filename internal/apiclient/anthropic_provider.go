package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/AlleyBo55/gocode/internal/apitypes"
)

const (
	defaultBaseURL     = "https://api.anthropic.com"
	anthropicVersion   = "2023-06-01"
)

// AnthropicProvider communicates with the Anthropic Messages API.
type AnthropicProvider struct {
	BaseURL string
	Auth    apitypes.AuthSource
	Client  *http.Client
	Retry   apitypes.RetryConfig
}

// NewAnthropicProvider creates a new Anthropic provider.
func NewAnthropicProvider(auth apitypes.AuthSource) *AnthropicProvider {
	baseURL := os.Getenv("ANTHROPIC_BASE_URL")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &AnthropicProvider{
		BaseURL: baseURL,
		Auth:    auth,
		Client:  &http.Client{Timeout: 5 * time.Minute},
		Retry:   apitypes.DefaultRetryConfig(),
	}
}

func (p *AnthropicProvider) Kind() ProviderKind { return ProviderAnthropic }

// SendMessage sends a non-streaming request to the Anthropic API.
func (p *AnthropicProvider) SendMessage(ctx context.Context, req apitypes.MessageRequest) (*apitypes.MessageResponse, error) {
	req.Stream = false
	resp, err := p.sendWithRetry(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	requestID := requestIDFromHeaders(resp.Header)
	var msgResp apitypes.MessageResponse
	if err := json.NewDecoder(resp.Body).Decode(&msgResp); err != nil {
		return nil, apitypes.WrapJson(err)
	}
	if msgResp.RequestID == "" {
		msgResp.RequestID = requestID
	}
	return &msgResp, nil
}

// StreamMessage sends a streaming request and returns a channel of StreamEvents.
func (p *AnthropicProvider) StreamMessage(ctx context.Context, req apitypes.MessageRequest) (<-chan apitypes.StreamEvent, error) {
	req.Stream = true
	resp, err := p.sendWithRetry(ctx, req)
	if err != nil {
		return nil, err
	}

	ch := make(chan apitypes.StreamEvent, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		parser := NewSseParser()
		buf := make([]byte, 4096)
		for {
			n, readErr := resp.Body.Read(buf)
			if n > 0 {
				events, parseErr := parser.Push(buf[:n])
				if parseErr != nil {
					return
				}
				for _, ev := range events {
					select {
					case ch <- ev:
					case <-ctx.Done():
						return
					}
				}
			}
			if readErr != nil {
				if readErr != io.EOF {
					// Stream interrupted
				}
				// Flush remaining
				events, _ := parser.Finish()
				for _, ev := range events {
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

func (p *AnthropicProvider) sendWithRetry(ctx context.Context, req apitypes.MessageRequest) (*http.Response, error) {
	var lastErr error
	for attempt := 1; attempt <= p.Retry.MaxRetries+1; attempt++ {
		resp, err := p.sendRaw(ctx, req)
		if err != nil {
			if attempt <= p.Retry.MaxRetries {
				lastErr = err
				backoff, bErr := p.Retry.BackoffForAttempt(attempt)
				if bErr != nil {
					return nil, bErr
				}
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
		backoff, bErr := p.Retry.BackoffForAttempt(attempt)
		if bErr != nil {
			return nil, bErr
		}
		time.Sleep(backoff)
	}
	return nil, apitypes.NewRetriesExhausted(p.Retry.MaxRetries+1, lastErr)
}

func (p *AnthropicProvider) sendRaw(ctx context.Context, req apitypes.MessageRequest) (*http.Response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, apitypes.WrapJson(err)
	}
	url := strings.TrimRight(p.BaseURL, "/") + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, apitypes.WrapHttp(err)
	}
	httpReq.Header.Set("anthropic-version", anthropicVersion)
	httpReq.Header.Set("content-type", "application/json")
	ApplyAuth(httpReq, p.Auth)

	resp, err := p.Client.Do(httpReq)
	if err != nil {
		return nil, apitypes.WrapHttp(err)
	}
	return resp, nil
}

func requestIDFromHeaders(h http.Header) string {
	if id := h.Get("request-id"); id != "" {
		return id
	}
	return h.Get("x-request-id")
}

func readApiError(resp *http.Response) *apitypes.ApiError {
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var envelope struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &envelope)
	return apitypes.NewApiError(resp.StatusCode, envelope.Error.Type, envelope.Error.Message, string(body))
}
