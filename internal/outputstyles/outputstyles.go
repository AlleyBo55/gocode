package outputstyles

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FormatFunc transforms agent output text according to a style.
type FormatFunc func(text string) string

// Style defines an output formatting style.
type Style struct {
	Name        string
	Description string
	FormatFunc  FormatFunc
}

// Registry holds built-in and user-defined output styles.
type Registry struct {
	styles map[string]Style
}

// NewRegistry creates a Registry pre-loaded with built-in styles.
func NewRegistry() *Registry {
	r := &Registry{styles: make(map[string]Style)}
	r.registerBuiltins()
	return r
}

func (r *Registry) registerBuiltins() {
	r.styles["concise"] = Style{
		Name:        "concise",
		Description: "Short, to-the-point responses without extra explanation",
		FormatFunc:  formatConcise,
	}
	r.styles["verbose"] = Style{
		Name:        "verbose",
		Description: "Detailed responses with step-by-step reasoning",
		FormatFunc:  formatVerbose,
	}
	r.styles["markdown"] = Style{
		Name:        "markdown",
		Description: "Full markdown formatting with headers and code blocks",
		FormatFunc:  formatMarkdown,
	}
	r.styles["minimal"] = Style{
		Name:        "minimal",
		Description: "Bare text with no formatting",
		FormatFunc:  formatMinimal,
	}
}

// Get returns the named style, or nil if not found.
func (r *Registry) Get(name string) *Style {
	s, ok := r.styles[strings.ToLower(name)]
	if !ok {
		return nil
	}
	return &s
}

// List returns all registered style names.
func (r *Registry) List() []string {
	names := make([]string, 0, len(r.styles))
	for n := range r.styles {
		names = append(names, n)
	}
	return names
}

// Register adds or replaces a style in the registry.
func (r *Registry) Register(s Style) {
	r.styles[strings.ToLower(s.Name)] = s
}

// Apply formats text using the named style. Returns the original text if the
// style is not found.
func Apply(style string, text string) string {
	r := NewRegistry()
	s := r.Get(style)
	if s == nil {
		return text
	}
	return s.FormatFunc(text)
}

// LoadUserStyles loads user-defined styles from the given directory. Each file
// in the directory becomes a style whose name is the filename (without
// extension) and whose format function prepends the file content as a header.
func (r *Registry) LoadUserStyles(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no user styles directory — that's fine
		}
		return fmt.Errorf("read user styles dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		instruction := string(data)
		r.styles[strings.ToLower(name)] = Style{
			Name:        name,
			Description: "User-defined style: " + name,
			FormatFunc: func(text string) string {
				return instruction + "\n\n" + text
			},
		}
	}
	return nil
}

// --- built-in format functions ---

func formatConcise(text string) string {
	// Strip excessive blank lines, keep it tight.
	lines := strings.Split(text, "\n")
	var out []string
	prevBlank := false
	for _, l := range lines {
		blank := strings.TrimSpace(l) == ""
		if blank && prevBlank {
			continue
		}
		out = append(out, l)
		prevBlank = blank
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func formatVerbose(text string) string {
	// Pass through as-is; the verbose style relies on the system prompt
	// instruction rather than post-processing.
	return text
}

func formatMarkdown(text string) string {
	// Pass through — markdown is the default rendering mode.
	return text
}

func formatMinimal(text string) string {
	// Strip markdown formatting: headers, bold, italic, code fences.
	s := text
	// Remove code fences.
	s = strings.ReplaceAll(s, "```", "")
	// Remove header markers.
	lines := strings.Split(s, "\n")
	var out []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		// Strip leading # markers.
		for strings.HasPrefix(trimmed, "#") {
			trimmed = strings.TrimPrefix(trimmed, "#")
		}
		trimmed = strings.TrimSpace(trimmed)
		// Strip bold/italic markers.
		trimmed = strings.ReplaceAll(trimmed, "**", "")
		trimmed = strings.ReplaceAll(trimmed, "__", "")
		trimmed = strings.ReplaceAll(trimmed, "*", "")
		trimmed = strings.ReplaceAll(trimmed, "_", "")
		out = append(out, trimmed)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
