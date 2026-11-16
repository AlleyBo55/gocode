package doctor

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCompactJSONKeepsMultibyteArgsValid(t *testing.T) {
	raw, err := json.Marshal(map[string]string{"query": strings.Repeat("中文", 120)})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := compactJSON(raw)
	if !utf8.ValidString(got) {
		t.Fatalf("compactJSON returned invalid UTF-8: %q", got)
	}
	if strings.ContainsRune(got, utf8.RuneError) {
		t.Fatalf("compactJSON contains U+FFFD: %q", got)
	}
	if utf8.RuneCountInString(got) > 80 {
		t.Fatalf("compactJSON returned %d runes, over the 80-rune budget: %q", utf8.RuneCountInString(got), got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("compactJSON did not end with an ellipsis: %q", got)
	}
}
