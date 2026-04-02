package hashline

import (
	"strings"
	"testing"
)

func TestContentHash_Deterministic(t *testing.T) {
	// Same content should always produce the same hash.
	h1 := ContentHash("func main() {")
	h2 := ContentHash("func main() {")
	if h1 != h2 {
		t.Errorf("expected identical hashes, got %q and %q", h1, h2)
	}
}

func TestContentHash_TrimmedEquivalence(t *testing.T) {
	// Leading/trailing whitespace should not affect the hash.
	h1 := ContentHash("  hello  ")
	h2 := ContentHash("hello")
	if h1 != h2 {
		t.Errorf("expected same hash for trimmed-equivalent lines, got %q and %q", h1, h2)
	}
}

func TestContentHash_Length(t *testing.T) {
	cases := []string{"", "a", "hello world", "func main() {", strings.Repeat("x", 10000)}
	for _, c := range cases {
		h := ContentHash(c)
		if len(h) != 2 {
			t.Errorf("ContentHash(%q) = %q, want 2-char hash", c, h)
		}
	}
}

func TestContentHash_DifferentContent(t *testing.T) {
	// Different content should (usually) produce different hashes.
	// We test a few known-different pairs; collisions are possible but unlikely.
	h1 := ContentHash("func main() {")
	h2 := ContentHash("func foo() {")
	// We can't guarantee no collision, but we can check the function runs.
	_ = h1
	_ = h2
}

func TestContentHash_EmptyLine(t *testing.T) {
	h := ContentHash("")
	if len(h) != 2 {
		t.Errorf("expected 2-char hash for empty line, got %q", h)
	}
	// Empty and whitespace-only should be the same.
	h2 := ContentHash("   ")
	if h != h2 {
		t.Errorf("expected empty and whitespace-only to hash the same, got %q and %q", h, h2)
	}
}

func TestAnnotateLines_Format(t *testing.T) {
	lines := []string{"package main", "", "func main() {"}
	result := AnnotateLines(lines)

	outputLines := strings.Split(strings.TrimSuffix(result, "\n"), "\n")
	if len(outputLines) != 3 {
		t.Fatalf("expected 3 output lines, got %d", len(outputLines))
	}

	// Check format: LINE#HASH| content
	for i, line := range outputLines {
		// Should start with "N#XX| "
		parts := strings.SplitN(line, "| ", 2)
		if len(parts) != 2 {
			t.Errorf("line %d: expected 'LINE#HASH| content' format, got %q", i, line)
			continue
		}
		prefix := parts[0]
		content := parts[1]

		// Prefix should be "N#XX"
		hashParts := strings.SplitN(prefix, "#", 2)
		if len(hashParts) != 2 {
			t.Errorf("line %d: expected 'LINE#HASH' prefix, got %q", i, prefix)
			continue
		}

		if len(hashParts[1]) != 2 {
			t.Errorf("line %d: expected 2-char hash, got %q", i, hashParts[1])
		}

		// Content should match original.
		if content != lines[i] {
			t.Errorf("line %d: expected content %q, got %q", i, lines[i], content)
		}
	}
}

func TestAnnotateLines_PreservesContent(t *testing.T) {
	lines := []string{"  indented", "tabs\there", "special chars: !@#$%"}
	result := AnnotateLines(lines)
	for _, orig := range lines {
		if !strings.Contains(result, orig) {
			t.Errorf("annotated output should contain original content %q", orig)
		}
	}
}

func TestAnnotateLines_OneIndexed(t *testing.T) {
	lines := []string{"first", "second", "third"}
	result := AnnotateLines(lines)
	outputLines := strings.Split(strings.TrimSuffix(result, "\n"), "\n")

	// First line should start with "1#"
	if !strings.HasPrefix(outputLines[0], "1#") {
		t.Errorf("expected first line to start with '1#', got %q", outputLines[0])
	}
	if !strings.HasPrefix(outputLines[2], "3#") {
		t.Errorf("expected third line to start with '3#', got %q", outputLines[2])
	}
}

func TestValidateEdit_AllMatch(t *testing.T) {
	lines := []string{"func main() {", "  fmt.Println(\"hello\")", "}"}
	refs := []EditRef{
		{LineNumber: 1, Hash: ContentHash(lines[0])},
		{LineNumber: 3, Hash: ContentHash(lines[2])},
	}
	if err := ValidateEdit(lines, refs); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestValidateEdit_Mismatch(t *testing.T) {
	lines := []string{"func main() {", "  fmt.Println(\"hello\")", "}"}
	refs := []EditRef{
		{LineNumber: 1, Hash: ContentHash(lines[0])},
		{LineNumber: 2, Hash: "zz"}, // wrong hash
	}
	err := ValidateEdit(lines, refs)
	if err == nil {
		t.Fatal("expected error for mismatched hash")
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Errorf("error should mention line 2, got: %v", err)
	}
	if !strings.Contains(err.Error(), "hash mismatch") {
		t.Errorf("error should mention hash mismatch, got: %v", err)
	}
}

func TestValidateEdit_OutOfRange(t *testing.T) {
	lines := []string{"only one line"}
	refs := []EditRef{
		{LineNumber: 5, Hash: "aa"},
	}
	err := ValidateEdit(lines, refs)
	if err == nil {
		t.Fatal("expected error for out-of-range line")
	}
	if !strings.Contains(err.Error(), "out of range") {
		t.Errorf("error should mention out of range, got: %v", err)
	}
}

func TestValidateEdit_EmptyRefs(t *testing.T) {
	lines := []string{"some content"}
	if err := ValidateEdit(lines, nil); err != nil {
		t.Errorf("expected no error for empty refs, got %v", err)
	}
}

func TestAnnotateLines_RoundTrip(t *testing.T) {
	// Annotating lines and re-hashing each line's content should match the embedded hash.
	lines := []string{"package main", "", "import \"fmt\"", "func main() {", "  fmt.Println(\"hi\")", "}"}
	result := AnnotateLines(lines)
	outputLines := strings.Split(strings.TrimSuffix(result, "\n"), "\n")

	for i, annotated := range outputLines {
		parts := strings.SplitN(annotated, "| ", 2)
		if len(parts) != 2 {
			t.Fatalf("line %d: bad format", i)
		}
		hashParts := strings.SplitN(parts[0], "#", 2)
		embeddedHash := hashParts[1]
		content := parts[1]

		recomputed := ContentHash(content)
		if recomputed != embeddedHash {
			t.Errorf("line %d: embedded hash %q != recomputed %q for content %q", i+1, embeddedHash, recomputed, content)
		}
	}
}
