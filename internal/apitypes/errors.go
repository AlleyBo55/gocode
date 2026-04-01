package apitypes

import "fmt"

// ApiError kind constants  ApiError enum.
const (
	ErrMissingCredentials = "missing_credentials"
	ErrExpiredOAuthToken  = "expired_oauth_token"
	ErrAuth               = "auth"
	ErrInvalidApiKeyEnv   = "invalid_api_key_env"
	ErrHttp               = "http"
	ErrIo                 = "io"
	ErrJson               = "json"
	ErrApi                = "api"
	ErrRetriesExhausted   = "retries_exhausted"
	ErrInvalidSseFrame    = "invalid_sse_frame"
	ErrBackoffOverflow    = "backoff_overflow"
)

// ApiError is a structured error type  ApiError enum.
type ApiError struct {
	Kind      string
	Status    int
	ErrorType string
	Message   string
	Body      string
	Retryable bool
	Wrapped   error
	Provider  string
	EnvVars   []string
	Attempts  int
}

// Error implements the error interface with descriptive messages per Kind.
func (e *ApiError) Error() string {
	switch e.Kind {
	case ErrMissingCredentials:
		return fmt.Sprintf("missing %s credentials; export %s before calling the API", e.Provider, joinOr(e.EnvVars))
	case ErrExpiredOAuthToken:
		return "saved OAuth token is expired and no refresh token is available"
	case ErrAuth:
		return fmt.Sprintf("auth error: %s", e.Message)
	case ErrInvalidApiKeyEnv:
		if e.Wrapped != nil {
			return fmt.Sprintf("failed to read credential environment variable: %v", e.Wrapped)
		}
		return "failed to read credential environment variable"
	case ErrHttp:
		if e.Wrapped != nil {
			return fmt.Sprintf("http error: %v", e.Wrapped)
		}
		return "http error"
	case ErrIo:
		if e.Wrapped != nil {
			return fmt.Sprintf("io error: %v", e.Wrapped)
		}
		return "io error"
	case ErrJson:
		if e.Wrapped != nil {
			return fmt.Sprintf("json error: %v", e.Wrapped)
		}
		return "json error"
	case ErrApi:
		if e.ErrorType != "" && e.Message != "" {
			return fmt.Sprintf("api returned %d (%s): %s", e.Status, e.ErrorType, e.Message)
		}
		return fmt.Sprintf("api returned %d: %s", e.Status, e.Body)
	case ErrRetriesExhausted:
		if e.Wrapped != nil {
			return fmt.Sprintf("api failed after %d attempts: %v", e.Attempts, e.Wrapped)
		}
		return fmt.Sprintf("api failed after %d attempts", e.Attempts)
	case ErrInvalidSseFrame:
		return fmt.Sprintf("invalid sse frame: %s", e.Message)
	case ErrBackoffOverflow:
		return fmt.Sprintf("retry backoff overflowed on attempt %d", e.Attempts)
	default:
		return fmt.Sprintf("api error (%s): %s", e.Kind, e.Message)
	}
}

// Unwrap returns the wrapped error.
func (e *ApiError) Unwrap() error { return e.Wrapped }

// IsRetryable returns true if this error should trigger a retry.
func (e *ApiError) IsRetryable() bool {
	switch e.Kind {
	case ErrHttp:
		return true
	case ErrApi:
		return e.Retryable
	case ErrRetriesExhausted:
		if inner, ok := e.Wrapped.(*ApiError); ok {
			return inner.IsRetryable()
		}
		return false
	default:
		return false
	}
}

// NewMissingCredentials creates a MissingCredentials error.
func NewMissingCredentials(provider string, envVars ...string) *ApiError {
	return &ApiError{Kind: ErrMissingCredentials, Provider: provider, EnvVars: envVars}
}

// NewApiError creates an API response error.
func NewApiError(status int, errorType, message, body string) *ApiError {
	return &ApiError{
		Kind:      ErrApi,
		Status:    status,
		ErrorType: errorType,
		Message:   message,
		Body:      body,
		Retryable: isRetryableStatus(status),
	}
}

// NewRetriesExhausted creates a RetriesExhausted error.
func NewRetriesExhausted(attempts int, lastErr error) *ApiError {
	return &ApiError{Kind: ErrRetriesExhausted, Attempts: attempts, Wrapped: lastErr}
}

// WrapHttp wraps an HTTP transport error.
func WrapHttp(err error) *ApiError { return &ApiError{Kind: ErrHttp, Wrapped: err} }

// WrapIo wraps an I/O error.
func WrapIo(err error) *ApiError { return &ApiError{Kind: ErrIo, Wrapped: err} }

// WrapJson wraps a JSON error.
func WrapJson(err error) *ApiError { return &ApiError{Kind: ErrJson, Wrapped: err} }

// isRetryableStatus returns true for HTTP status codes that should trigger retry.
func isRetryableStatus(status int) bool {
	switch status {
	case 408, 409, 429, 500, 502, 503, 504:
		return true
	default:
		return false
	}
}

// IsRetryableStatus is the exported version for use by providers.
func IsRetryableStatus(status int) bool { return isRetryableStatus(status) }

func joinOr(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	if len(ss) == 1 {
		return ss[0]
	}
	return fmt.Sprintf("%s or %s", ss[0], ss[len(ss)-1])
}
