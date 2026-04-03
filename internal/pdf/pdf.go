package pdf

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/google/uuid"
)

// MaxPDFSize is the maximum raw PDF size (20 MB). After base64 encoding (~33%
// larger) this leaves room for conversation context within a 32 MB API limit.
const MaxPDFSize = 20 * 1024 * 1024

// PDFResult holds the base64-encoded content and original size of a PDF.
type PDFResult struct {
	Base64       string
	OriginalSize int64
}

// ExtractResult holds the output directory and page count from page extraction.
type ExtractResult struct {
	OutputDir string
	PageCount int
}

// ReadPDF reads a PDF file, validates the %PDF- magic bytes, enforces the 20 MB
// size limit, and returns the base64-encoded content.
func ReadPDF(path string) (PDFResult, error) {
	info, err := os.Stat(path)
	if err != nil {
		return PDFResult{}, fmt.Errorf("stat %s: %w", path, err)
	}
	if info.Size() == 0 {
		return PDFResult{}, fmt.Errorf("PDF file is empty: %s", path)
	}
	if info.Size() > MaxPDFSize {
		return PDFResult{}, fmt.Errorf("PDF file exceeds maximum allowed size of %d bytes: %s", MaxPDFSize, path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return PDFResult{}, fmt.Errorf("read %s: %w", path, err)
	}

	// Validate magic bytes.
	if len(data) < 5 || string(data[:5]) != "%PDF-" {
		return PDFResult{}, fmt.Errorf("file is not a valid PDF (missing %%PDF- header): %s", path)
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	return PDFResult{
		Base64:       encoded,
		OriginalSize: info.Size(),
	}, nil
}

// GetPageCount returns the number of pages in a PDF using the `pdfinfo`
// command-line tool (from poppler-utils). Returns an error if pdfinfo is not
// available or the page count cannot be determined.
func GetPageCount(path string) (int, error) {
	out, err := exec.Command("pdfinfo", path).Output()
	if err != nil {
		return 0, fmt.Errorf("pdfinfo failed: %w", err)
	}
	re := regexp.MustCompile(`(?m)^Pages:\s+(\d+)`)
	m := re.FindSubmatch(out)
	if m == nil {
		return 0, fmt.Errorf("could not parse page count from pdfinfo output")
	}
	count, err := strconv.Atoi(string(m[1]))
	if err != nil {
		return 0, fmt.Errorf("invalid page count: %w", err)
	}
	return count, nil
}

// ExtractPages converts PDF pages to JPEG images using `pdftoppm` (from
// poppler-utils). firstPage and lastPage are 1-indexed and inclusive. Pass 0
// for either to use pdftoppm defaults (all pages).
func ExtractPages(path string, firstPage, lastPage int) (ExtractResult, error) {
	info, err := os.Stat(path)
	if err != nil {
		return ExtractResult{}, fmt.Errorf("stat %s: %w", path, err)
	}
	if info.Size() == 0 {
		return ExtractResult{}, fmt.Errorf("PDF file is empty: %s", path)
	}

	// Check pdftoppm availability.
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		return ExtractResult{}, fmt.Errorf("pdftoppm is not installed: install poppler-utils")
	}

	outputDir := filepath.Join(os.TempDir(), "gocode-pdf-"+uuid.New().String())
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return ExtractResult{}, fmt.Errorf("create output dir: %w", err)
	}

	prefix := filepath.Join(outputDir, "page")
	args := []string{"-jpeg", "-r", "100"}
	if firstPage > 0 {
		args = append(args, "-f", strconv.Itoa(firstPage))
	}
	if lastPage > 0 {
		args = append(args, "-l", strconv.Itoa(lastPage))
	}
	args = append(args, path, prefix)

	cmd := exec.Command("pdftoppm", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return ExtractResult{}, fmt.Errorf("pdftoppm failed: %s: %w", string(out), err)
	}

	// Count generated JPEG files.
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return ExtractResult{}, fmt.Errorf("read output dir: %w", err)
	}
	count := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".jpg" {
			count++
		}
	}
	if count == 0 {
		return ExtractResult{}, fmt.Errorf("pdftoppm produced no output pages; the PDF may be invalid")
	}

	return ExtractResult{
		OutputDir: outputDir,
		PageCount: count,
	}, nil
}
