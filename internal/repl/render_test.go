package repl

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// RenderToolOutput is what the operator actually reads after a tool call. Its
// previews are capped at 80 runes per line and 200 runes for an error, so
// non-ASCII output must still come out as valid text at the full budget.
func TestRenderToolOutputKeepsMultibytePreviewValid(t *testing.T) {
	line := strings.Repeat("中文输出", 40) // 160 characters, 480 bytes
	got := RenderToolOutput("Bash", line, false)

	if !utf8.ValidString(got) {
		t.Fatalf("RenderToolOutput returned invalid UTF-8: %q", got)
	}
	if strings.ContainsRune(got, utf8.RuneError) {
		t.Fatalf("RenderToolOutput contains U+FFFD: %q", got)
	}

	// The 80-rune budget must survive: the old byte slice kept only 26 of them.
	want := strings.Repeat("中文输出", 19) + "中" + "..."
	if !strings.Contains(got, want) {
		t.Fatalf("RenderToolOutput did not keep the 80-rune preview\n got: %q\nwant to contain: %q", got, want)
	}
}

func TestRenderToolOutputKeepsErrorPreviewValid(t *testing.T) {
	msg := strings.Repeat("命令失败：找不到文件。", 30)
	got := RenderToolOutput("Read", msg, true)

	if !utf8.ValidString(got) {
		t.Fatalf("RenderToolOutput returned invalid UTF-8: %q", got)
	}
	if strings.ContainsRune(got, utf8.RuneError) {
		t.Fatalf("RenderToolOutput contains U+FFFD: %q", got)
	}
	if !strings.Contains(got, "...") {
		t.Fatalf("RenderToolOutput did not mark the cut: %q", got)
	}
}
