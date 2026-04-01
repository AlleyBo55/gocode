package apitypes

import (
	"math"
	"time"
)

// RetryConfig controls exponential backoff retry behavior.
type RetryConfig struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
}

// DefaultRetryConfig returns the default retry configuration with default.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:     2,
		InitialBackoff: 200 * time.Millisecond,
		MaxBackoff:     2 * time.Second,
	}
}

// BackoffForAttempt computes the backoff duration for a given attempt number (1-indexed).
// Returns an error if the computation overflows.
func (c RetryConfig) BackoffForAttempt(attempt int) (time.Duration, error) {
	if attempt < 1 {
		return c.InitialBackoff, nil
	}
	exponent := attempt - 1
	if exponent > 62 {
		return 0, &ApiError{Kind: ErrBackoffOverflow, Attempts: attempt}
	}
	multiplier := math.Pow(2, float64(exponent))
	backoff := time.Duration(float64(c.InitialBackoff) * multiplier)
	if backoff > c.MaxBackoff {
		backoff = c.MaxBackoff
	}
	return backoff, nil
}
