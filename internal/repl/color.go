package repl

import (
	"io"
	"os"
	"regexp"

	"github.com/mattn/go-isatty"
)

// ansiPattern matches CSI escape sequences (colors, cursor moves, line clears).
var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

// StripANSI removes ANSI escape sequences from s.
func StripANSI(s string) string {
	if !hasESC(s) {
		return s
	}
	return ansiPattern.ReplaceAllString(s, "")
}

func hasESC(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			return true
		}
	}
	return false
}

// ansiStripWriter removes ANSI escapes from everything written through it, so
// the colored output the REPL emits degrades to plain text on a pipe, a file,
// or a dumb terminal. Each Write carries complete escape sequences (the REPL
// never splits one across calls), so stripping per Write is sufficient.
type ansiStripWriter struct{ w io.Writer }

func (a ansiStripWriter) Write(p []byte) (int, error) {
	if !hasESC(string(p)) {
		return a.w.Write(p)
	}
	stripped := ansiPattern.ReplaceAll(p, nil)
	if _, err := a.w.Write(stripped); err != nil {
		return 0, err
	}
	// Report the original length: callers wrote p in full; the bytes we
	// dropped were escapes they did not need to see land.
	return len(p), nil
}

// colorWriter returns w unchanged when color should be shown, or a writer that
// strips ANSI when it should not. Color is on only for a real terminal with
// NO_COLOR unset and TERM not "dumb"; a pipe, a file, or a test buffer gets
// plain text.
func colorWriter(w io.Writer) io.Writer {
	if colorEnabled(w) {
		return w
	}
	return ansiStripWriter{w: w}
}

func colorEnabled(w io.Writer) bool {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}
