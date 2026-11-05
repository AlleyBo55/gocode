package textutil

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateKeepsShortStrings(t *testing.T) {
	for _, s := range []string{"", "abc", "查询结果", "🚀 构建完成"} {
		if got := Truncate(s, 80); got != s {
			t.Fatalf("Truncate(%q, 80) = %q, want it unchanged", s, got)
		}
	}
}

func TestTruncateCountsRunesNotBytes(t *testing.T) {
	cases := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"ascii", strings.Repeat("a", 20), 10, "aaaaaaa..."},
		{"chinese", strings.Repeat("中文", 20), 10, "中文中文中文中..."}, {"japanese", strings.Repeat("あいう", 10), 8, "あいうあい..."},
		{"emoji", strings.Repeat("🚀", 10), 5, "🚀🚀..."},
		{"exact limit", "中文内容", 4, "中文内容"},
		{"ellipsis is the whole limit", "中文内容", 3, "..."},
		{"limit under ellipsis", "中文内容", 2, ".."},
		{"zero limit", "中文内容", 0, ""},
		{"negative limit", "中文内容", -3, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Truncate(tc.in, tc.max)
			if got != tc.want {
				t.Fatalf("Truncate(%q, %d) = %q, want %q", tc.in, tc.max, got, tc.want)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("Truncate(%q, %d) = %q, which is not valid UTF-8", tc.in, tc.max, got)
			}
			if max := tc.max; max > 0 && utf8.RuneCountInString(got) > max {
				t.Fatalf("Truncate(%q, %d) = %q, %d runes, over the limit", tc.in, tc.max, got, utf8.RuneCountInString(got))
			}
		})
	}
}

// A byte slice at a multibyte boundary is what this helper replaced: it left a
// partial sequence behind, so check the exact case that used to break.
func TestTruncateNeverSplitsACharacter(t *testing.T) {
	// The byte cut this replaced landed inside a 3-byte character at every
	// limit that was not a multiple of three.
	in := strings.Repeat("测", 200)
	got := Truncate(in, 80)
	if !utf8.ValidString(got) {
		t.Fatalf("Truncate left invalid UTF-8: %q", got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("Truncate did not mark the cut: %q", got)
	}
	if runes := utf8.RuneCountInString(got); runes != 80 {
		t.Fatalf("Truncate kept %d runes, want the full 80-rune budget: %q", runes, got)
	}
}
