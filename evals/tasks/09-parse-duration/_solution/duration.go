// Package duration parses human-written durations.
package duration

import (
	"errors"
	"strings"
	"time"
)

// ErrInvalidDuration is returned when the input cannot be parsed.
var ErrInvalidDuration = errors.New("duration: invalid")

var units = map[byte]time.Duration{
	'd': 24 * time.Hour,
	'h': time.Hour,
	'm': time.Minute,
	's': time.Second,
}

// ParseHuman parses strings such as "1h30m", "45s", "2d" or "1d 2h" into a
// Duration. Supported units are d, h, m and s.
func ParseHuman(s string) (time.Duration, error) {
	s = strings.Join(strings.Fields(s), "")
	if s == "" {
		return 0, ErrInvalidDuration
	}
	var total time.Duration
	i := 0
	for i < len(s) {
		start := i
		var n int64
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			n = n*10 + int64(s[i]-'0')
			i++
		}
		if i == start || i == len(s) {
			return 0, ErrInvalidDuration // no digits, or digits with no unit
		}
		unit, ok := units[s[i]]
		if !ok {
			return 0, ErrInvalidDuration
		}
		total += time.Duration(n) * unit
		i++
	}
	return total, nil
}
