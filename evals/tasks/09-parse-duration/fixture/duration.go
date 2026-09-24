// Package duration parses human-written durations.
package duration

import (
	"errors"
	"time"
)

// ErrInvalidDuration is returned when the input cannot be parsed.
var ErrInvalidDuration = errors.New("duration: invalid")

// ParseHuman parses strings such as "1h30m", "45s", "2d" or "1d 2h" into a
// Duration. Supported units are d, h, m and s.
func ParseHuman(s string) (time.Duration, error) {
	return 0, errors.New("not implemented")
}
