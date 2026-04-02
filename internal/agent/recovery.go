package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AlleyBo55/gocode/internal/apitypes"
	"github.com/AlleyBo55/gocode/internal/session"
)

// RecoveryLogger receives recovery events for observability.
type RecoveryLogger interface {
	OnRecovery(action string, detail string)
}

// SessionRecoveryManager wraps ConversationRuntime with recovery logic.
// It intercepts errors from the inner runtime and applies recovery strategies:
//   - Context window exhaustion: compact session, then retry
//   - Transient API failure (429, 5xx): exponential backoff with max retries
//   - Corrupted session state: reload from SessionStore, then retry
type SessionRecoveryManager struct {
	inner        *ConversationRuntime
	sessionStore session.SessionPersistence
	logger       RecoveryLogger
	maxRetries   int
	lastSessionID string
}

// NewSessionRecoveryManager wraps an existing runtime with recovery capabilities.
func NewSessionRecoveryManager(rt *ConversationRuntime, store session.SessionPersistence, logger RecoveryLogger) *SessionRecoveryManager {
	return &SessionRecoveryManager{
		inner:        rt,
		sessionStore: store,
		logger:       logger,
		maxRetries:   3,
	}
}

// SetLastSessionID sets the session ID used for corrupted-session recovery.
func (m *SessionRecoveryManager) SetLastSessionID(id string) {
	m.lastSessionID = id
}

// SendUserMessage delegates to the inner runtime, catching context-window,
// transient, and corrupted-session errors and applying recovery strategies.
func (m *SessionRecoveryManager) SendUserMessage(ctx context.Context, text string) (*apitypes.MessageResponse, error) {
	resp, err := m.inner.SendUserMessage(ctx, text)
	if err == nil {
		return resp, nil
	}

	var apiErr *apitypes.ApiError
	if !errors.As(err, &apiErr) {
		return nil, err
	}

	// Strategy 1: Context window exhaustion
	if isContextWindowError(apiErr) {
		return m.recoverContextWindow(ctx, text)
	}

	// Strategy 2: Transient API failure
	if isTransientError(apiErr) {
		return m.recoverTransient(ctx, text)
	}

	// Strategy 3: Corrupted session — try reload from store
	if m.lastSessionID != "" && m.sessionStore != nil {
		return m.recoverCorruptedSession(ctx, text)
	}

	return nil, err
}

// recoverContextWindow compacts the session and retries.
func (m *SessionRecoveryManager) recoverContextWindow(ctx context.Context, text string) (*apitypes.MessageResponse, error) {
	sessionBefore := len(m.inner.GetSession())
	m.inner.CompactSession(10) // preserve last 5 pairs = 10 messages
	sessionAfter := len(m.inner.GetSession())
	removed := sessionBefore - sessionAfter

	m.logger.OnRecovery("context_window_compaction",
		fmt.Sprintf("compacted session from %d to %d messages (removed %d)", sessionBefore, sessionAfter, removed))

	resp, err := m.inner.SendUserMessage(ctx, text)
	if err != nil {
		return nil, fmt.Errorf("recovery failed after context window compaction: %w", err)
	}
	return resp, nil
}

// recoverTransient retries with exponential backoff up to maxRetries.
func (m *SessionRecoveryManager) recoverTransient(ctx context.Context, text string) (*apitypes.MessageResponse, error) {
	cfg := apitypes.RetryConfig{
		MaxRetries:     m.maxRetries,
		InitialBackoff: 200 * time.Millisecond,
		MaxBackoff:     2 * time.Second,
	}

	var lastErr error
	for attempt := 1; attempt <= cfg.MaxRetries; attempt++ {
		backoff, bErr := cfg.BackoffForAttempt(attempt)
		if bErr != nil {
			return nil, bErr
		}

		m.logger.OnRecovery("transient_retry",
			fmt.Sprintf("attempt %d/%d, backing off %v", attempt, cfg.MaxRetries, backoff))

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}

		resp, err := m.inner.SendUserMessage(ctx, text)
		if err == nil {
			m.logger.OnRecovery("transient_retry",
				fmt.Sprintf("succeeded on attempt %d/%d", attempt, cfg.MaxRetries))
			return resp, nil
		}
		lastErr = err

		// Only keep retrying if it's still a transient error
		var apiErr *apitypes.ApiError
		if errors.As(err, &apiErr) && !isTransientError(apiErr) {
			return nil, err
		}
	}

	m.logger.OnRecovery("transient_retry",
		fmt.Sprintf("exhausted all %d retry attempts", cfg.MaxRetries))
	return nil, fmt.Errorf("recovery failed after %d transient retries: %w", cfg.MaxRetries, lastErr)
}

// recoverCorruptedSession reloads the session from the store and retries.
func (m *SessionRecoveryManager) recoverCorruptedSession(ctx context.Context, text string) (*apitypes.MessageResponse, error) {
	stored, err := m.sessionStore.Load(m.lastSessionID)
	if err != nil {
		return nil, fmt.Errorf("recovery failed: could not reload session %s: %w", m.lastSessionID, err)
	}

	// Convert stored messages to InputMessage format
	var messages []apitypes.InputMessage
	for _, msg := range stored.Messages {
		messages = append(messages, apitypes.InputMessage{
			Role: msg.Role,
			Content: []apitypes.InputContentBlock{
				{Kind: "text", Text: msg.Content},
			},
		})
	}

	m.inner.RestoreSession(messages)
	m.logger.OnRecovery("session_reload",
		fmt.Sprintf("reloaded session %s with %d messages", m.lastSessionID, len(messages)))

	resp, retryErr := m.inner.SendUserMessage(ctx, text)
	if retryErr != nil {
		return nil, fmt.Errorf("recovery failed after session reload: %w", retryErr)
	}
	return resp, nil
}

// isContextWindowError checks if the error indicates context window exhaustion.
func isContextWindowError(apiErr *apitypes.ApiError) bool {
	return apiErr.Kind == apitypes.ErrApi && strings.Contains(apiErr.ErrorType, "context_window")
}

// isTransientError checks if the error is a transient API failure.
func isTransientError(apiErr *apitypes.ApiError) bool {
	if apiErr.Kind == apitypes.ErrHttp {
		return true
	}
	if apiErr.Kind == apitypes.ErrApi {
		switch apiErr.Status {
		case 429, 500, 502, 503, 504:
			return true
		}
	}
	return false
}
