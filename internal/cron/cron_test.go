package cron

import (
	"testing"
	"time"
)

func TestParseValid(t *testing.T) {
	tests := []struct{ expr string; wantMin int }{
		{"* * * * *", 60},
		{"*/5 * * * *", 12},
		{"0 * * * *", 1},
		{"0 0 * * *", 1},
		{"0 0 1 * *", 1},
		{"0 0 * * 0", 1},
		{"0,15,30,45 * * * *", 4},
		{"0-10 * * * *", 11},
		{"0-10/2 * * * *", 6},
	}
	for _, tt := range tests {
		f, err := Parse(tt.expr)
		if err != nil {
			t.Errorf("Parse(%q): %v", tt.expr, err)
			continue
		}
		if len(f.Minute) != tt.wantMin {
			t.Errorf("Parse(%q).Minute has %d values, want %d", tt.expr, len(f.Minute), tt.wantMin)
		}
	}
}

func TestParseInvalid(t *testing.T) {
	bad := []string{"", "* *", "* * * * * *", "abc * * * *", "61 * * * *", "* 25 * * *"}
	for _, expr := range bad {
		if _, err := Parse(expr); err == nil {
			t.Errorf("Parse(%q) should fail", expr)
		}
	}
}

func TestNextRun(t *testing.T) {
	// Every 5 minutes: */5 * * * *
	f, _ := Parse("*/5 * * * *")
	from := time.Date(2025, 1, 1, 10, 3, 0, 0, time.Local)
	next := NextRun(f, from)
	if next.Minute() != 5 {
		t.Errorf("expected minute 5, got %d", next.Minute())
	}

	// Daily at 9am: 0 9 * * *
	f2, _ := Parse("0 9 * * *")
	from2 := time.Date(2025, 1, 1, 10, 0, 0, 0, time.Local)
	next2 := NextRun(f2, from2)
	if next2.Day() != 2 || next2.Hour() != 9 {
		t.Errorf("expected Jan 2 9:00, got %v", next2)
	}

	// Sunday at noon: 0 12 * * 0
	f3, _ := Parse("0 12 * * 0")
	// Jan 1 2025 is Wednesday
	from3 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.Local)
	next3 := NextRun(f3, from3)
	if next3.Weekday() != time.Sunday {
		t.Errorf("expected Sunday, got %v", next3.Weekday())
	}
}

func TestNextRunStrictlyAfter(t *testing.T) {
	f, _ := Parse("30 * * * *")
	from := time.Date(2025, 1, 1, 10, 30, 0, 0, time.Local)
	next := NextRun(f, from)
	if !next.After(from) {
		t.Errorf("NextRun should be strictly after from: got %v", next)
	}
	if next.Hour() != 11 || next.Minute() != 30 {
		t.Errorf("expected 11:30, got %d:%d", next.Hour(), next.Minute())
	}
}

func TestDayOfWeek7IsSunday(t *testing.T) {
	f, err := Parse("0 0 * * 7")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.DayOfWeek) != 1 || f.DayOfWeek[0] != 0 {
		t.Errorf("day 7 should map to 0 (Sunday), got %v", f.DayOfWeek)
	}
}

func TestToHuman(t *testing.T) {
	tests := []struct{ expr, want string }{
		{"*/5 * * * *", "Every 5 minutes"},
		{"*/1 * * * *", "Every minute"},
		{"0 * * * *", "Every hour"},
		{"30 * * * *", "Every hour at :30"},
		{"0 */2 * * *", "Every 2 hours"},
		{"0 0 * * 1-5", "Weekdays at 12:00 AM"},
	}
	for _, tt := range tests {
		got := ToHuman(tt.expr)
		if got != tt.want {
			t.Errorf("ToHuman(%q) = %q, want %q", tt.expr, got, tt.want)
		}
	}
}
