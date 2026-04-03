package outputstyles

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestBuiltinStylesExist(t *testing.T) {
	r := NewRegistry()
	expected := []string{"concise", "verbose", "markdown", "minimal"}
	for _, name := range expected {
		if s := r.Get(name); s == nil {
			t.Errorf("expected built-in style %q to exist", name)
		}
	}
}

func TestApply_ConciseCollapsesBlankLines(t *testing.T) {
	input := "line1\n\n\n\nline2\n\n\nline3"
	out := Apply("concise", input)
	if strings.Contains(out, "\n\n\n") {
		t.Error("concise style should collapse consecutive blank lines")
	}
	if !strings.Contains(out, "line1") || !strings.Contains(out, "line2") || !strings.Contains(out, "line3") {
		t.Error("concise style should preserve content")
	}
}

func TestApply_MinimalStripsMarkdown(t *testing.T) {
	input := "# Header\n\n**bold text**\n\n```go\nfmt.Println()\n```"
	out := Apply("minimal", input)
	if strings.Contains(out, "#") {
		t.Error("minimal style should strip header markers")
	}
	if strings.Contains(out, "**") {
		t.Error("minimal style should strip bold markers")
	}
	if strings.Contains(out, "```") {
		t.Error("minimal style should strip code fences")
	}
	if !strings.Contains(out, "Header") {
		t.Error("minimal style should preserve header text")
	}
}

func TestApply_VerbosePassthrough(t *testing.T) {
	input := "Some detailed text\nwith multiple lines"
	out := Apply("verbose", input)
	if out != input {
		t.Errorf("verbose should pass through text unchanged, got %q", out)
	}
}

func TestApply_UnknownStyleReturnsOriginal(t *testing.T) {
	input := "hello world"
	out := Apply("nonexistent", input)
	if out != input {
		t.Errorf("unknown style should return original text, got %q", out)
	}
}

func TestRegistryList(t *testing.T) {
	r := NewRegistry()
	names := r.List()
	sort.Strings(names)
	if len(names) != 4 {
		t.Errorf("expected 4 built-in styles, got %d", len(names))
	}
}

func TestLoadUserStyles(t *testing.T) {
	dir := t.TempDir()
	stylesDir := filepath.Join(dir, ".gocode", "output-styles")
	if err := os.MkdirAll(stylesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stylesDir, "pirate.txt"), []byte("Respond like a pirate."), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewRegistry()
	if err := r.LoadUserStyles(stylesDir); err != nil {
		t.Fatalf("LoadUserStyles: %v", err)
	}

	s := r.Get("pirate")
	if s == nil {
		t.Fatal("expected user style 'pirate' to be loaded")
	}
	out := s.FormatFunc("Hello")
	if !strings.Contains(out, "Respond like a pirate") {
		t.Error("user style should prepend instruction")
	}
	if !strings.Contains(out, "Hello") {
		t.Error("user style should include original text")
	}
}

func TestLoadUserStyles_MissingDir(t *testing.T) {
	r := NewRegistry()
	err := r.LoadUserStyles("/nonexistent/path/to/styles")
	if err != nil {
		t.Errorf("missing dir should not error, got: %v", err)
	}
}

func TestRegisterCustomStyle(t *testing.T) {
	r := NewRegistry()
	r.Register(Style{
		Name:        "shout",
		Description: "UPPERCASE EVERYTHING",
		FormatFunc:  func(text string) string { return strings.ToUpper(text) },
	})
	s := r.Get("shout")
	if s == nil {
		t.Fatal("expected custom style to be registered")
	}
	if s.FormatFunc("hello") != "HELLO" {
		t.Error("custom style should uppercase text")
	}
}
