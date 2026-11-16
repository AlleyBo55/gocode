package agent

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSnippetKeepsMultibyteInputValid(t *testing.T) {
	in := strings.Repeat("中文输出", 60)
	got := snippet(in)
	if !utf8.ValidString(got) {
		t.Fatalf("snippet returned invalid UTF-8: %q", got)
	}
	if strings.ContainsRune(got, utf8.RuneError) {
		t.Fatalf("snippet contains U+FFFD: %q", got)
	}
	want := strings.Repeat("中文输出", 49) + "中" + "..."
	if !strings.Contains(got, want) {
		t.Fatalf("snippet did not keep the 200-rune preview\n got: %q\nwant to contain: %q", got, want)
	}
}

func TestSnippetLeavesShortInputAlone(t *testing.T) {
	if got := snippet("hello"); got != `"hello"` {
		t.Errorf("snippet(%q) = %s, want %q", "hello", got, `"hello"`)
	}
}
