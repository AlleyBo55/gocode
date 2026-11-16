package orchestrator

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateKeepsMultibyteTextValid(t *testing.T) {
	in := strings.Repeat("中", 300)
	got := truncate(in, 200)
	if !utf8.ValidString(got) {
		t.Fatalf("truncate returned invalid UTF-8: %q", got)
	}
	if strings.ContainsRune(got, utf8.RuneError) {
		t.Fatalf("truncate contains U+FFFD: %q", got)
	}
	want := strings.Repeat("中", 200) + "… [300 more bytes]"
	if got != want {
		t.Fatalf("truncate = %q, want %q", got, want)
	}
}

func TestTruncateLeavesShortTextAlone(t *testing.T) {
	if got := truncate("abc", 200); got != "abc" {
		t.Errorf("truncate(%q, 200) = %q, want %q", "abc", got, "abc")
	}
}
