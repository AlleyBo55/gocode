package apiclient

import (
	"context"
	"errors"
	"strings"

	"github.com/AlleyBo55/gocode/internal/apitypes"
)

// FallbackLogger receives fallback events for observability.
type FallbackLogger interface {
	OnFallback(originalModel string, err error, fallbackModel string)
}

// FallbackEntry pairs a model name with its provider.
type FallbackEntry struct {
	Model    string
	Provider Provider
}

// FallbackProvider implements Provider by trying models in order.
// On retryable errors it falls through to the next entry in the chain.
type FallbackProvider struct {
	chain  []FallbackEntry
	logger FallbackLogger
}

// NewFallbackProvider creates a FallbackProvider from an ordered list of entries.
// The logger may be nil, in which case fallback events are silently ignored.
func NewFallbackProvider(chain []FallbackEntry, logger FallbackLogger) *FallbackProvider {
	return &FallbackProvider{
		chain:  chain,
		logger: logger,
	}
}

// Kind returns the provider kind of the first entry in the chain.
func (f *FallbackProvider) Kind() ProviderKind {
	if len(f.chain) == 0 {
		return ProviderAnthropic // sensible default
	}
	return f.chain[0].Provider.Kind()
}

// SendMessage tries each provider in order. On retryable errors (429, 500-504,
// context-window-exceeded), it falls through to the next entry. Non-retryable
// errors are returned immediately.
func (f *FallbackProvider) SendMessage(ctx context.Context, req apitypes.MessageRequest) (*apitypes.MessageResponse, error) {
	var lastErr error
	for i, entry := range f.chain {
		reqCopy := req
		reqCopy.Model = entry.Model

		resp, err := entry.Provider.SendMessage(ctx, reqCopy)
		if err == nil {
			return resp, nil
		}

		if !isFallbackRetryable(err) {
			return nil, err
		}

		lastErr = err

		// Log the fallback if there's a next entry to try.
		if f.logger != nil && i+1 < len(f.chain) {
			f.logger.OnFallback(entry.Model, err, f.chain[i+1].Model)
		}
	}
	return nil, lastErr
}

// StreamMessage tries each provider in order with the same fallback logic as SendMessage.
func (f *FallbackProvider) StreamMessage(ctx context.Context, req apitypes.MessageRequest) (<-chan apitypes.StreamEvent, error) {
	var lastErr error
	for i, entry := range f.chain {
		reqCopy := req
		reqCopy.Model = entry.Model

		ch, err := entry.Provider.StreamMessage(ctx, reqCopy)
		if err == nil {
			return ch, nil
		}

		if !isFallbackRetryable(err) {
			return nil, err
		}

		lastErr = err

		if f.logger != nil && i+1 < len(f.chain) {
			f.logger.OnFallback(entry.Model, err, f.chain[i+1].Model)
		}
	}
	return nil, lastErr
}

// isFallbackRetryable checks whether an error should trigger a fallback attempt.
// It triggers on HTTP 429, 500, 502, 503, 504 and context-window-exceeded errors.
func isFallbackRetryable(err error) bool {
	var apiErr *apitypes.ApiError
	if !errors.As(err, &apiErr) {
		return false
	}

	// Check retryable HTTP status codes.
	switch apiErr.Status {
	case 429, 500, 502, 503, 504:
		return true
	}

	// Check for context-window-exceeded error type.
	if isContextWindowExceeded(apiErr.ErrorType) {
		return true
	}

	return false
}

// isContextWindowExceeded checks if the error type string indicates a context
// window exceeded condition.
func isContextWindowExceeded(errorType string) bool {
	lower := strings.ToLower(errorType)
	return lower == "context_window_exceeded" ||
		strings.Contains(lower, "context_length_exceeded") ||
		strings.Contains(lower, "context_window")
}
