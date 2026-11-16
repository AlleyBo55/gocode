package toolimpl

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateOutputKeepsMultibyteValid(t *testing.T) {
	big := strings.Repeat("中文输出", 20000)
	got := TruncateOutput("bashtool", big)
	if !utf8.ValidString(got) {
		t.Fatalf("TruncateOutput returned invalid UTF-8")
	}
	if strings.ContainsRune(got, utf8.RuneError) {
		t.Fatalf("TruncateOutput contains U+FFFD")
	}
	if len(got) > MaxToolOutputBytes+400 {
		t.Errorf("truncated output is %d bytes, cap is %d plus a marker", len(got), MaxToolOutputBytes)
	}
	if !strings.Contains(got, "output truncated") {
		t.Errorf("truncated output lost its marker")
	}
	if !strings.HasPrefix(got, "中文输出") {
		t.Errorf("head lost: %q", got[:30])
	}
	if !strings.HasSuffix(got, "中文输出") {
		t.Errorf("tail lost: %q", got[len(got)-30:])
	}
}
