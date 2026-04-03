// Package cron provides minimal cron expression parsing and scheduling.
//
// Ported from Claude Code's src/utils/cron.ts — a clean 5-field cron parser
// with next-run calculation. Supports: wildcard, N, */N step, N-M range,
// comma lists. All times in local timezone. No L, W, ?, or name aliases.
//
// Improvement: Go's time.Timer gives us precise scheduling without polling.
// Claude Code polls with setInterval; we use time.AfterFunc for zero-drift.
package cron

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Fields holds the expanded values for each cron field.
type Fields struct {
	Minute     []int
	Hour       []int
	DayOfMonth []int
	Month      []int
	DayOfWeek  []int
}

type fieldRange struct {
	min, max int
}

var fieldRanges = []fieldRange{
	{0, 59},  // minute
	{0, 23},  // hour
	{1, 31},  // day of month
	{1, 12},  // month
	{0, 6},   // day of week (0=Sunday; 7 accepted as Sunday alias)
}

// Parse parses a 5-field cron expression into expanded number arrays.
// Returns an error if the expression is invalid.
func Parse(expr string) (*Fields, error) {
	parts := strings.Fields(strings.TrimSpace(expr))
	if len(parts) != 5 {
		return nil, fmt.Errorf("cron: expected 5 fields, got %d", len(parts))
	}
	expanded := make([][]int, 5)
	for i := 0; i < 5; i++ {
		vals, err := expandField(parts[i], fieldRanges[i])
		if err != nil {
			return nil, fmt.Errorf("cron field %d (%q): %w", i, parts[i], err)
		}
		expanded[i] = vals
	}
	return &Fields{
		Minute:     expanded[0],
		Hour:       expanded[1],
		DayOfMonth: expanded[2],
		Month:      expanded[3],
		DayOfWeek:  expanded[4],
	}, nil
}

// expandField parses a single cron field into a sorted array of matching values.
func expandField(field string, r fieldRange) ([]int, error) {
	out := make(map[int]bool)
	isDow := r.min == 0 && r.max == 6

	for _, part := range strings.Split(field, ",") {
		// */N or *
		if strings.HasPrefix(part, "*") {
			step := 1
			if strings.Contains(part, "/") {
				s, err := strconv.Atoi(strings.SplitN(part, "/", 2)[1])
				if err != nil || s < 1 {
					return nil, fmt.Errorf("invalid step in %q", part)
				}
				step = s
			}
			for i := r.min; i <= r.max; i += step {
				out[i] = true
			}
			continue
		}

		// N-M or N-M/S
		if strings.Contains(part, "-") {
			rangeParts := strings.SplitN(part, "/", 2)
			bounds := strings.SplitN(rangeParts[0], "-", 2)
			lo, err1 := strconv.Atoi(bounds[0])
			hi, err2 := strconv.Atoi(bounds[1])
			if err1 != nil || err2 != nil {
				return nil, fmt.Errorf("invalid range %q", part)
			}
			step := 1
			if len(rangeParts) > 1 {
				s, err := strconv.Atoi(rangeParts[1])
				if err != nil || s < 1 {
					return nil, fmt.Errorf("invalid step in range %q", part)
				}
				step = s
			}
			effMax := r.max
			if isDow {
				effMax = 7 // accept 7 as Sunday alias in ranges
			}
			if lo > hi || lo < r.min || hi > effMax {
				return nil, fmt.Errorf("range out of bounds: %q", part)
			}
			for i := lo; i <= hi; i += step {
				v := i
				if isDow && v == 7 {
					v = 0
				}
				out[v] = true
			}
			continue
		}

		// Plain N
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid value %q", part)
		}
		if isDow && n == 7 {
			n = 0
		}
		if n < r.min || n > r.max {
			return nil, fmt.Errorf("value %d out of range [%d, %d]", n, r.min, r.max)
		}
		out[n] = true
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("empty field")
	}
	result := make([]int, 0, len(out))
	for v := range out {
		result = append(result, v)
	}
	sort.Ints(result)
	return result, nil
}

// NextRun computes the next time strictly after `from` that matches the cron
// fields, using local timezone. Walks forward minute-by-minute, bounded at
// 366 days. Returns zero time if no match (shouldn't happen for valid cron).
//
// Standard cron semantics: when both dayOfMonth and dayOfWeek are constrained
// (neither is full range), a date matches if EITHER matches.
func NextRun(f *Fields, from time.Time) time.Time {
	minuteSet := toSet(f.Minute)
	hourSet := toSet(f.Hour)
	domSet := toSet(f.DayOfMonth)
	monthSet := toSet(f.Month)
	dowSet := toSet(f.DayOfWeek)

	domWild := len(f.DayOfMonth) == 31
	dowWild := len(f.DayOfWeek) == 7

	// Round up to next whole minute.
	t := from.Truncate(time.Minute).Add(time.Minute)

	maxIter := 366 * 24 * 60
	for i := 0; i < maxIter; i++ {
		month := int(t.Month())
		if !monthSet[month] {
			// Jump to start of next month.
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
			continue
		}

		dom := t.Day()
		dow := int(t.Weekday())
		var dayMatches bool
		switch {
		case domWild && dowWild:
			dayMatches = true
		case domWild:
			dayMatches = dowSet[dow]
		case dowWild:
			dayMatches = domSet[dom]
		default:
			dayMatches = domSet[dom] || dowSet[dow] // OR semantics
		}
		if !dayMatches {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location())
			continue
		}

		if !hourSet[t.Hour()] {
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, t.Location())
			continue
		}

		if !minuteSet[t.Minute()] {
			t = t.Add(time.Minute)
			continue
		}

		return t
	}
	return time.Time{} // should never happen for valid cron
}

// ToHuman converts a cron expression to a human-readable string.
// Covers common patterns; falls through to raw expression for complex ones.
func ToHuman(expr string) string {
	parts := strings.Fields(strings.TrimSpace(expr))
	if len(parts) != 5 {
		return expr
	}
	minute, hour, dom, month, dow := parts[0], parts[1], parts[2], parts[3], parts[4]

	// Every N minutes: */N * * * *
	if strings.HasPrefix(minute, "*/") && hour == "*" && dom == "*" && month == "*" && dow == "*" {
		n, _ := strconv.Atoi(strings.TrimPrefix(minute, "*/"))
		if n == 1 {
			return "Every minute"
		}
		return fmt.Sprintf("Every %d minutes", n)
	}

	// Every hour: N * * * *
	if isNum(minute) && hour == "*" && dom == "*" && month == "*" && dow == "*" {
		m, _ := strconv.Atoi(minute)
		if m == 0 {
			return "Every hour"
		}
		return fmt.Sprintf("Every hour at :%02d", m)
	}

	// Every N hours: M */N * * *
	if isNum(minute) && strings.HasPrefix(hour, "*/") && dom == "*" && month == "*" && dow == "*" {
		n, _ := strconv.Atoi(strings.TrimPrefix(hour, "*/"))
		m, _ := strconv.Atoi(minute)
		suffix := ""
		if m != 0 {
			suffix = fmt.Sprintf(" at :%02d", m)
		}
		if n == 1 {
			return "Every hour" + suffix
		}
		return fmt.Sprintf("Every %d hours%s", n, suffix)
	}

	// Daily: M H * * *
	if isNum(minute) && isNum(hour) && dom == "*" && month == "*" && dow == "*" {
		return fmt.Sprintf("Every day at %s", formatTime(minute, hour))
	}

	// Weekly: M H * * D
	dayNames := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
	if isNum(minute) && isNum(hour) && dom == "*" && month == "*" && isNum(dow) {
		d, _ := strconv.Atoi(dow)
		d = d % 7
		return fmt.Sprintf("Every %s at %s", dayNames[d], formatTime(minute, hour))
	}

	// Weekdays: M H * * 1-5
	if isNum(minute) && isNum(hour) && dom == "*" && month == "*" && dow == "1-5" {
		return fmt.Sprintf("Weekdays at %s", formatTime(minute, hour))
	}

	return expr
}

func isNum(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

func formatTime(minute, hour string) string {
	h, _ := strconv.Atoi(hour)
	m, _ := strconv.Atoi(minute)
	t := time.Date(2000, 1, 1, h, m, 0, 0, time.Local)
	return t.Format("3:04 PM")
}

func toSet(vals []int) map[int]bool {
	s := make(map[int]bool, len(vals))
	for _, v := range vals {
		s[v] = true
	}
	return s
}
