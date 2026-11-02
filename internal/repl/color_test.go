package repl

import (
	"bytes"
	"strings"
	"testing"
)

func TestStripANSIRemovesSequences(t *testing.T) {
	in := cBlue + "hello" + ansiReset + " " + cGray + ansiBold + "world" + ansiReset
	got := StripANSI(in)
	if got != "hello world" {
		t.Fatalf("StripANSI = %q, want %q", got, "hello world")
	}
	if strings.ContainsRune(got, 0x1b) {
		t.Fatal("escape byte survived StripANSI")
	}
}

func TestColorWriterStripsForNonTerminal(t *testing.T) {
	// A bytes.Buffer is not a terminal, so the wrapper must strip color.
	buf := &bytes.Buffer{}
	w := colorWriter(buf)
	if _, err := w.Write([]byte(cBlue + "x" + ansiReset)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := buf.String(); got != "x" {
		t.Fatalf("piped output = %q, want %q (no escapes)", got, "x")
	}
}

func TestColorWriterReportsFullWriteLength(t *testing.T) {
	// Callers rely on Write returning len(p) even when escapes are dropped,
	// so a short-write error is never reported for stripped bytes.
	buf := &bytes.Buffer{}
	w := colorWriter(buf)
	p := []byte(cBlue + "hi" + ansiReset)
	n, err := w.Write(p)
	if err != nil || n != len(p) {
		t.Fatalf("Write = (%d, %v), want (%d, nil)", n, err, len(p))
	}
}
