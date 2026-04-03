package pdf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadPDF_ValidMagicBytes(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "test.pdf")
	// Minimal valid PDF-like content (magic bytes + some data).
	content := []byte("%PDF-1.4 fake pdf content here for testing purposes")
	if err := os.WriteFile(p, content, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ReadPDF(p)
	if err != nil {
		t.Fatalf("ReadPDF: %v", err)
	}
	if result.OriginalSize != int64(len(content)) {
		t.Errorf("OriginalSize = %d, want %d", result.OriginalSize, len(content))
	}
	if result.Base64 == "" {
		t.Error("Base64 should not be empty")
	}
}

func TestReadPDF_InvalidMagicBytes(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "notpdf.pdf")
	if err := os.WriteFile(p, []byte("<html>not a pdf</html>"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ReadPDF(p)
	if err == nil {
		t.Fatal("expected error for invalid magic bytes")
	}
	if !strings.Contains(err.Error(), "%PDF-") {
		t.Errorf("error should mention %%PDF- header, got: %v", err)
	}
}

func TestReadPDF_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "empty.pdf")
	if err := os.WriteFile(p, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ReadPDF(p)
	if err == nil {
		t.Fatal("expected error for empty file")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error should mention empty, got: %v", err)
	}
}

func TestReadPDF_SizeLimit(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "huge.pdf")

	// Create a file that exceeds MaxPDFSize. We write just the header and
	// then truncate to the desired size to avoid allocating 20 MB+ in memory.
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(MaxPDFSize + 1); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	_, err = ReadPDF(p)
	if err == nil {
		t.Fatal("expected error for oversized file")
	}
	if !strings.Contains(err.Error(), "exceeds maximum") {
		t.Errorf("error should mention size limit, got: %v", err)
	}
}

func TestReadPDF_FileNotFound(t *testing.T) {
	_, err := ReadPDF("/nonexistent/path/to/file.pdf")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestReadPDF_TooShortForMagic(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tiny.pdf")
	if err := os.WriteFile(p, []byte("%PD"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ReadPDF(p)
	if err == nil {
		t.Fatal("expected error for file shorter than magic bytes")
	}
}
