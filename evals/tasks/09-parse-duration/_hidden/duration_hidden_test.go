package duration

import (
	"errors"
	"testing"
	"time"
)

func TestParseHumanValid(t *testing.T) {
	cases := map[string]time.Duration{
		"45s":         45 * time.Second,
		"90m":         90 * time.Minute,
		"2h":          2 * time.Hour,
		"1h30m":       90 * time.Minute,
		"1d":          24 * time.Hour,
		"1d 2h 3m 4s": 24*time.Hour + 2*time.Hour + 3*time.Minute + 4*time.Second,
		"  10s ":      10 * time.Second,
		"0s":          0,
	}
	for in, want := range cases {
		got, err := ParseHuman(in)
		if err != nil {
			t.Errorf("ParseHuman(%q) error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseHuman(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestParseHumanInvalid(t *testing.T) {
	for _, in := range []string{"", "   ", "h", "5", "5x", "-5s", "1.5h", "abc", "5s5", "1h -2m"} {
		if _, err := ParseHuman(in); !errors.Is(err, ErrInvalidDuration) {
			t.Errorf("ParseHuman(%q) = %v, want ErrInvalidDuration", in, err)
		}
	}
}
