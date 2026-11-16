package repl

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSummarizeToolInputKeepsMultibyteValid(t *testing.T) {
	in := map[string]interface{}{"command": strings.Repeat("中文输出", 30)}
	got := summarizeToolInput("Bash", in)
	if !utf8.ValidString(got) {
		t.Fatalf("summarizeToolInput returned invalid UTF-8: %q", got)
	}
	if strings.ContainsRune(got, utf8.RuneError) {
		t.Fatalf("summarizeToolInput contains U+FFFD: %q", got)
	}
	want := strings.Repeat("中文输出", 19) + "中" + "..."
	if !strings.Contains(got, want) {
		t.Fatalf("summarizeToolInput did not keep the 80-rune preview\n got: %q\nwant to contain: %q", got, want)
	}
}

func TestSummarizeJSONKeepsMultibyteValid(t *testing.T) {
	in := `{"command": "` + strings.Repeat("中文输出", 30) + `"}`
	got := summarizeJSON(in)
	if !utf8.ValidString(got) {
		t.Fatalf("summarizeJSON returned invalid UTF-8: %q", got)
	}
	if strings.ContainsRune(got, utf8.RuneError) {
		t.Fatalf("summarizeJSON contains U+FFFD: %q", got)
	}
	if utf8.RuneCountInString(got) > 120 {
		t.Fatalf("summarizeJSON returned %d runes, over the 120-rune budget: %q", utf8.RuneCountInString(got), got)
	}
}
