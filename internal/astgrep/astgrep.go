package astgrep

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"sync"

	"github.com/AlleyBo55/gocode/internal/toolimpl"
)

// Match represents a single ast-grep match result.
type Match struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Snippet string `json:"snippet"`
}

// rawMatch mirrors the JSON structure emitted by `ast-grep run --json`.
type rawMatch struct {
	File  string `json:"file"`
	Range struct {
		Start struct {
			Line   int `json:"line"`
			Column int `json:"column"`
		} `json:"start"`
	} `json:"range"`
	Text string `json:"text"`
}

// Engine wraps the ast-grep CLI tool. It is stateless; the binary path is
// resolved lazily on first use and cached for the process lifetime.
type Engine struct {
	once    sync.Once
	binPath string
	binErr  error
}

// ensureBinary resolves the ast-grep binary once.
func (e *Engine) ensureBinary() error {
	e.once.Do(func() {
		e.binPath, e.binErr = exec.LookPath("ast-grep")
		if e.binErr != nil {
			e.binErr = fmt.Errorf("ast-grep is not installed or not found on PATH: %w", e.binErr)
		}
	})
	return e.binErr
}

// Search runs `ast-grep run --pattern <pattern> --json <dir>` and parses the
// JSON output into a slice of Match. If lang is non-empty it is passed via
// --lang. An empty result set is returned without error when no matches exist.
func (e *Engine) Search(pattern string, dir string, lang string) ([]Match, error) {
	if err := e.ensureBinary(); err != nil {
		return nil, err
	}

	args := []string{"run", "--pattern", pattern, "--json", dir}
	if lang != "" {
		args = append(args, "--lang", lang)
	}

	cmd := exec.Command(e.binPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// ast-grep exits 0 on success (matches or no matches) and non-zero on
		// real errors. An empty stdout with a non-zero exit is a real error.
		if stdout.Len() == 0 {
			return nil, fmt.Errorf("ast-grep search failed: %s", stderr.String())
		}
	}

	return parseJSON(stdout.Bytes())
}

// Replace runs `ast-grep run --pattern <pattern> --rewrite <replacement> <dir>`
// and returns the matches that were rewritten. If lang is non-empty it is
// passed via --lang.
func (e *Engine) Replace(pattern string, replacement string, dir string, lang string) ([]Match, error) {
	if err := e.ensureBinary(); err != nil {
		return nil, err
	}

	args := []string{"run", "--pattern", pattern, "--rewrite", replacement, dir}
	if lang != "" {
		args = append(args, "--lang", lang)
	}

	cmd := exec.Command(e.binPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stdout.Len() == 0 {
			return nil, fmt.Errorf("ast-grep replace failed: %s", stderr.String())
		}
	}

	return parseJSON(stdout.Bytes())
}

// parseJSON decodes the ast-grep JSON array output into []Match.
func parseJSON(data []byte) ([]Match, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, nil
	}

	var raw []rawMatch
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing ast-grep JSON output: %w", err)
	}

	matches := make([]Match, len(raw))
	for i, r := range raw {
		matches[i] = Match{
			File:    r.File,
			Line:    r.Range.Start.Line,
			Snippet: r.Text,
		}
	}
	return matches, nil
}

// ---------------------------------------------------------------------------
// Tool wrapper — implements toolimpl.ToolExecutor
// ---------------------------------------------------------------------------

// AstGrepTool exposes the Engine as a ToolExecutor.
// Params:
//   - pattern (string, required): the structural search pattern
//   - dir (string, optional, default "."): target directory
//   - lang (string, optional): language filter (go, javascript, typescript, python, …)
//   - replacement (string, optional): if provided, a rewrite is performed instead of a search
type AstGrepTool struct {
	Engine *Engine
}

func (t *AstGrepTool) Execute(params map[string]interface{}) toolimpl.ToolResult {
	pattern, _ := params["pattern"].(string)
	if pattern == "" {
		return toolimpl.ToolResult{Success: false, Error: "missing required param: pattern"}
	}

	dir, _ := params["dir"].(string)
	if dir == "" {
		dir = "."
	}

	lang, _ := params["lang"].(string)
	replacement, hasReplace := params["replacement"].(string)

	var (
		matches []Match
		err     error
	)

	if hasReplace && replacement != "" {
		matches, err = t.Engine.Replace(pattern, replacement, dir, lang)
	} else {
		matches, err = t.Engine.Search(pattern, dir, lang)
	}

	if err != nil {
		return toolimpl.ToolResult{Success: false, Error: err.Error()}
	}

	if len(matches) == 0 {
		return toolimpl.ToolResult{Success: true, Output: "No matches found."}
	}

	out, err := json.Marshal(matches)
	if err != nil {
		return toolimpl.ToolResult{Success: false, Error: fmt.Sprintf("marshalling results: %v", err)}
	}
	return toolimpl.ToolResult{Success: true, Output: string(out)}
}

// ---------------------------------------------------------------------------
// Registration
// ---------------------------------------------------------------------------

// RegisterAstGrepTool registers the ast_grep tool in the given registry.
func RegisterAstGrepTool(r *toolimpl.Registry) {
	r.Set("ast_grep", &AstGrepTool{Engine: &Engine{}})
}
