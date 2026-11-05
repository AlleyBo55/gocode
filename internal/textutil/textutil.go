// Package textutil holds small, dependency-free string helpers shared by the
// CLI, the interactive REPL and the MCP server.
package textutil

import "unicode/utf8"

// Ellipsis ends a string that Truncate cut.
const Ellipsis = "..."

// Truncate shortens s so it is at most max runes long, ending a cut string with
// an ellipsis. The limit counts RUNES, not bytes.
//
// Byte-based slicing (`s[:max]`) cuts multibyte text — Chinese, Japanese,
// Korean, emoji — in the middle of a character. The fragment then holds a
// partial UTF-8 sequence, which a terminal renders as U+FFFD, and because those
// characters occupy three or four bytes the preview shows roughly a third of
// the text its limit promised. Counting runes keeps the output valid UTF-8 and
// makes the limit mean the same thing for every language.
//
// max <= 0 yields an empty string. max below the ellipsis length yields the
// ellipsis cut to max runes, so the result never exceeds the limit.
func Truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	ellipsis := utf8.RuneCountInString(Ellipsis)
	if max <= ellipsis {
		return string([]rune(Ellipsis)[:max])
	}
	return string([]rune(s)[:max-ellipsis]) + Ellipsis
}
