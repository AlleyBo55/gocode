package toolimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ToolResult is the outcome of executing a tool.
type ToolResult struct {
	Success bool   `json:"success"`
	Output  string `json:"output"`
	Error   string `json:"error,omitempty"`
}

// ToolExecutor is implemented by all concrete tool implementations.
type ToolExecutor interface {
	Execute(params map[string]interface{}) ToolResult
}

// Registry maps tool names to their implementations.
type Registry struct {
	executors map[string]ToolExecutor
}

// NewRegistry creates a Registry with all built-in tool implementations.
func NewRegistry() *Registry {
	r := &Registry{executors: make(map[string]ToolExecutor)}
	r.executors["bashtool"] = &BashTool{}
	r.executors["filereadtool"] = &FileReadTool{}
	r.executors["fileedittool"] = &FileEditTool{}
	r.executors["filewritetool"] = &FileWriteTool{}
	r.executors["globtool"] = &GlobTool{}
	r.executors["greptool"] = &GrepTool{}
	r.executors["listdirectorytool"] = &ListDirectoryTool{}
	r.executors["webfetchtool"] = &WebFetchTool{}
	r.executors["websearchtool"] = &WebSearchTool{}
	r.executors["notebookedittool"] = &FileEditTool{} // .ipynb files are JSON
	return r
}

// Set registers (or replaces) the executor for the given tool name.
func (r *Registry) Set(name string, executor ToolExecutor) {
	r.executors[strings.ToLower(name)] = executor
}

// Get returns the executor for the given tool name (case-insensitive), or nil.
func (r *Registry) Get(name string) ToolExecutor {
	return r.executors[strings.ToLower(name)]
}

// ExecuteTool parses a JSON payload string and dispatches to the named tool.
func (r *Registry) ExecuteTool(name, payload string) ToolResult {
	executor := r.Get(name)
	if executor == nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("tool not implemented: %s", name)}
	}
	var params map[string]interface{}
	if payload != "" {
		if err := json.Unmarshal([]byte(payload), &params); err != nil {
			return ToolResult{Success: false, Error: fmt.Sprintf("invalid JSON params: %v", err)}
		}
	}
	if params == nil {
		params = make(map[string]interface{})
	}
	return executor.Execute(params)
}

// --- BashTool ---

// BashTool executes shell commands.
type BashTool struct{}

func (t *BashTool) Execute(params map[string]interface{}) ToolResult {
	command, _ := params["command"].(string)
	if command == "" {
		return ToolResult{Success: false, Error: "missing required param: command"}
	}

	timeoutMs := 30000
	if v, ok := params["timeout_ms"]; ok {
		switch tv := v.(type) {
		case float64:
			timeoutMs = int(tv)
		case json.Number:
			if n, err := tv.Int64(); err == nil {
				timeoutMs = int(n)
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	var sb strings.Builder
	if stdout.Len() > 0 {
		sb.WriteString(stdout.String())
	}
	if stderr.Len() > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("STDERR:\n")
		sb.WriteString(stderr.String())
	}

	if err != nil {
		exitCode := -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		if ctx.Err() == context.DeadlineExceeded {
			return ToolResult{
				Success: false,
				Output:  sb.String(),
				Error:   fmt.Sprintf("command timed out after %dms", timeoutMs),
			}
		}
		return ToolResult{
			Success: false,
			Output:  sb.String(),
			Error:   fmt.Sprintf("exit code %d: %v", exitCode, err),
		}
	}

	return ToolResult{Success: true, Output: sb.String()}
}

// --- FileReadTool ---

// FileReadTool reads file contents with optional line range.
type FileReadTool struct{}

func (t *FileReadTool) Execute(params map[string]interface{}) ToolResult {
	path, _ := params["path"].(string)
	if path == "" {
		return ToolResult{Success: false, Error: "missing required param: path"}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("reading file: %v", err)}
	}

	lines := strings.Split(string(data), "\n")

	startLine := 0
	endLine := len(lines)

	if v, ok := params["start_line"]; ok {
		if n, err := toInt(v); err == nil && n > 0 {
			startLine = n - 1 // convert 1-indexed to 0-indexed
		}
	}
	if v, ok := params["end_line"]; ok {
		if n, err := toInt(v); err == nil && n > 0 {
			endLine = n // 1-indexed inclusive → 0-indexed exclusive
		}
	}

	if startLine < 0 {
		startLine = 0
	}
	if endLine > len(lines) {
		endLine = len(lines)
	}
	if startLine >= endLine {
		return ToolResult{Success: true, Output: ""}
	}

	// Add line numbers
	var sb strings.Builder
	for i := startLine; i < endLine; i++ {
		sb.WriteString(fmt.Sprintf("%d: %s\n", i+1, lines[i]))
	}
	return ToolResult{Success: true, Output: sb.String()}
}

// --- FileEditTool ---

// FileEditTool edits files via exact string replacement.
type FileEditTool struct{}

func (t *FileEditTool) Execute(params map[string]interface{}) ToolResult {
	path, _ := params["path"].(string)
	oldText, _ := params["old_text"].(string)
	newText, _ := params["new_text"].(string)

	if path == "" || oldText == "" {
		return ToolResult{Success: false, Error: "missing required params: path, old_text"}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("reading file: %v", err)}
	}

	content := string(data)
	count := strings.Count(content, oldText)
	if count == 0 {
		return ToolResult{Success: false, Error: "old_text not found in file"}
	}
	if count > 1 {
		return ToolResult{Success: false, Error: fmt.Sprintf("old_text found %d times; must be unique", count)}
	}

	newContent := strings.Replace(content, oldText, newText, 1)
	if err := os.WriteFile(path, []byte(newContent), 0o644); err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("writing file: %v", err)}
	}

	FormatFile(path)

	return ToolResult{Success: true, Output: fmt.Sprintf("Edited %s: replaced 1 occurrence", path)}
}

// --- FileWriteTool ---

// FileWriteTool creates or overwrites files.
type FileWriteTool struct{}

func (t *FileWriteTool) Execute(params map[string]interface{}) ToolResult {
	path, _ := params["path"].(string)
	content, _ := params["content"].(string)

	if path == "" {
		return ToolResult{Success: false, Error: "missing required param: path"}
	}

	// Create parent directories
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("creating directories: %v", err)}
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("writing file: %v", err)}
	}

	FormatFile(path)

	return ToolResult{Success: true, Output: fmt.Sprintf("Wrote %d bytes to %s", len(content), path)}
}

// --- GlobTool ---

// GlobTool finds files matching a glob pattern.
type GlobTool struct{}

func (t *GlobTool) Execute(params map[string]interface{}) ToolResult {
	pattern, _ := params["pattern"].(string)
	if pattern == "" {
		return ToolResult{Success: false, Error: "missing required param: pattern"}
	}

	basePath, _ := params["path"].(string)
	if basePath == "" {
		basePath = "."
	}

	var matches []string
	err := filepath.WalkDir(basePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if d.IsDir() {
			// Skip hidden directories
			if strings.HasPrefix(d.Name(), ".") && path != basePath {
				return filepath.SkipDir
			}
			return nil
		}
		matched, err := filepath.Match(pattern, d.Name())
		if err != nil {
			return nil
		}
		if matched {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("walking directory: %v", err)}
	}

	if len(matches) == 0 {
		return ToolResult{Success: true, Output: "No files matched the pattern."}
	}

	// Cap output
	output := strings.Join(matches, "\n")
	if len(matches) > 200 {
		output = strings.Join(matches[:200], "\n")
		output += fmt.Sprintf("\n... and %d more files", len(matches)-200)
	}
	return ToolResult{Success: true, Output: output}
}

// --- GrepTool ---

// GrepTool searches file contents for a pattern.
type GrepTool struct{}

func (t *GrepTool) Execute(params map[string]interface{}) ToolResult {
	pattern, _ := params["pattern"].(string)
	if pattern == "" {
		return ToolResult{Success: false, Error: "missing required param: pattern"}
	}

	searchPath, _ := params["path"].(string)
	if searchPath == "" {
		searchPath = "."
	}

	include, _ := params["include"].(string)
	patternLower := strings.ToLower(pattern)

	var results []string
	maxResults := 100

	err := filepath.WalkDir(searchPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && d.IsDir() && strings.HasPrefix(d.Name(), ".") && path != searchPath {
				return filepath.SkipDir
			}
			return nil
		}
		if len(results) >= maxResults {
			return filepath.SkipAll
		}

		// Apply include filter
		if include != "" {
			matched, _ := filepath.Match(include, d.Name())
			if !matched {
				return nil
			}
		}

		// Skip binary-ish files
		info, _ := d.Info()
		if info != nil && info.Size() > 1024*1024 { // skip files > 1MB
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if len(results) >= maxResults {
				break
			}
			if strings.Contains(strings.ToLower(line), patternLower) {
				results = append(results, fmt.Sprintf("%s:%d: %s", path, i+1, strings.TrimSpace(line)))
			}
		}
		return nil
	})
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("searching: %v", err)}
	}

	if len(results) == 0 {
		return ToolResult{Success: true, Output: "No matches found."}
	}

	output := strings.Join(results, "\n")
	if len(results) >= maxResults {
		output += fmt.Sprintf("\n... (showing first %d results)", maxResults)
	}
	return ToolResult{Success: true, Output: output}
}

// --- ListDirectoryTool ---

// ListDirectoryTool lists directory contents.
type ListDirectoryTool struct{}

func (t *ListDirectoryTool) Execute(params map[string]interface{}) ToolResult {
	path, _ := params["path"].(string)
	if path == "" {
		path = "."
	}

	recursive := false
	if v, ok := params["recursive"]; ok {
		switch rv := v.(type) {
		case bool:
			recursive = rv
		case string:
			recursive = rv == "true"
		}
	}

	if recursive {
		return t.listRecursive(path)
	}
	return t.listFlat(path)
}

func (t *ListDirectoryTool) listFlat(path string) ToolResult {
	entries, err := os.ReadDir(path)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("reading directory: %v", err)}
	}

	var sb strings.Builder
	for _, entry := range entries {
		info, _ := entry.Info()
		if entry.IsDir() {
			sb.WriteString(fmt.Sprintf("[DIR]  %s/\n", entry.Name()))
		} else if info != nil {
			sb.WriteString(fmt.Sprintf("[FILE] %s (%d bytes)\n", entry.Name(), info.Size()))
		} else {
			sb.WriteString(fmt.Sprintf("[FILE] %s\n", entry.Name()))
		}
	}
	if sb.Len() == 0 {
		return ToolResult{Success: true, Output: "Directory is empty."}
	}
	return ToolResult{Success: true, Output: sb.String()}
}

func (t *ListDirectoryTool) listRecursive(basePath string) ToolResult {
	var sb strings.Builder
	count := 0
	maxEntries := 500

	err := filepath.WalkDir(basePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if count >= maxEntries {
			return filepath.SkipAll
		}
		// Skip hidden directories (except root)
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") && path != basePath {
			return filepath.SkipDir
		}

		rel, _ := filepath.Rel(basePath, path)
		if rel == "." {
			return nil
		}

		if d.IsDir() {
			sb.WriteString(fmt.Sprintf("[DIR]  %s/\n", rel))
		} else {
			info, _ := d.Info()
			if info != nil {
				sb.WriteString(fmt.Sprintf("[FILE] %s (%d bytes)\n", rel, info.Size()))
			} else {
				sb.WriteString(fmt.Sprintf("[FILE] %s\n", rel))
			}
		}
		count++
		return nil
	})
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("walking directory: %v", err)}
	}
	if count >= maxEntries {
		sb.WriteString(fmt.Sprintf("\n... truncated (showing first %d entries)\n", maxEntries))
	}
	if sb.Len() == 0 {
		return ToolResult{Success: true, Output: "Directory is empty."}
	}
	return ToolResult{Success: true, Output: sb.String()}
}

// --- WebFetchTool ---

// WebFetchTool fetches a URL and returns the body text.
type WebFetchTool struct{}

func (t *WebFetchTool) Execute(params map[string]interface{}) ToolResult {
	url, _ := params["url"].(string)
	if url == "" {
		return ToolResult{Success: false, Error: "missing required param: url"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("creating request: %v", err)}
	}
	req.Header.Set("User-Agent", "gocode/1.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("fetching URL: %v", err)}
	}
	defer resp.Body.Close()
	body := make([]byte, 10240) // 10KB max
	n, _ := resp.Body.Read(body)
	return ToolResult{Success: true, Output: string(body[:n])}
}

// --- WebSearchTool ---

// WebSearchTool performs a web search using DuckDuckGo's instant answer API.
type WebSearchTool struct{}

func (t *WebSearchTool) Execute(params map[string]interface{}) ToolResult {
	query, _ := params["query"].(string)
	if query == "" {
		return ToolResult{Success: false, Error: "missing required param: query"}
	}

	// Use DuckDuckGo instant answer API (no API key required)
	searchURL := fmt.Sprintf("https://api.duckduckgo.com/?q=%s&format=json&no_html=1&skip_disambig=1",
		strings.ReplaceAll(query, " ", "+"))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("creating request: %v", err)}
	}
	req.Header.Set("User-Agent", "gocode/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("search request failed: %v", err)}
	}
	defer resp.Body.Close()

	body := make([]byte, 32768) // 32KB max
	n, _ := resp.Body.Read(body)
	rawJSON := body[:n]

	// Parse DuckDuckGo response
	var ddg struct {
		Abstract       string `json:"Abstract"`
		AbstractSource string `json:"AbstractSource"`
		AbstractURL    string `json:"AbstractURL"`
		Answer         string `json:"Answer"`
		AnswerType     string `json:"AnswerType"`
		Heading        string `json:"Heading"`
		RelatedTopics  []struct {
			Text     string `json:"Text"`
			FirstURL string `json:"FirstURL"`
		} `json:"RelatedTopics"`
	}

	var sb strings.Builder
	if json.Unmarshal(rawJSON, &ddg) == nil {
		if ddg.Heading != "" {
			sb.WriteString(fmt.Sprintf("# %s\n\n", ddg.Heading))
		}
		if ddg.Abstract != "" {
			sb.WriteString(fmt.Sprintf("%s\nSource: %s (%s)\n\n", ddg.Abstract, ddg.AbstractSource, ddg.AbstractURL))
		}
		if ddg.Answer != "" {
			sb.WriteString(fmt.Sprintf("Answer: %s\n\n", ddg.Answer))
		}
		if len(ddg.RelatedTopics) > 0 {
			sb.WriteString("Related:\n")
			limit := 8
			if len(ddg.RelatedTopics) < limit {
				limit = len(ddg.RelatedTopics)
			}
			for i := 0; i < limit; i++ {
				rt := ddg.RelatedTopics[i]
				if rt.Text != "" {
					sb.WriteString(fmt.Sprintf("- %s\n  %s\n", rt.Text, rt.FirstURL))
				}
			}
		}
	}

	result := sb.String()
	if result == "" {
		// Fallback: return raw response snippet
		result = fmt.Sprintf("Search results for %q (raw):\n%s", query, string(rawJSON[:min(n, 2048)]))
	}

	return ToolResult{Success: true, Output: result}
}

// --- helpers ---

func toInt(v interface{}) (int, error) {
	switch n := v.(type) {
	case float64:
		return int(n), nil
	case int:
		return n, nil
	case json.Number:
		i, err := n.Int64()
		return int(i), err
	case string:
		return strconv.Atoi(n)
	}
	return 0, fmt.Errorf("cannot convert %T to int", v)
}

// FormatFile runs the appropriate code formatter for the given file path.
// It is best-effort: if the formatter binary is not found, it silently skips.
func FormatFile(path string) {
	ext := strings.ToLower(filepath.Ext(path))
	var cmd *exec.Cmd
	switch ext {
	case ".go":
		if p, err := exec.LookPath("goimports"); err == nil {
			cmd = exec.Command(p, "-w", path)
		} else if p, err := exec.LookPath("gofmt"); err == nil {
			cmd = exec.Command(p, "-w", path)
		}
	case ".js", ".ts", ".jsx", ".tsx":
		if p, err := exec.LookPath("prettier"); err == nil {
			cmd = exec.Command(p, "--write", path)
		}
	case ".py":
		if p, err := exec.LookPath("black"); err == nil {
			cmd = exec.Command(p, path)
		}
	case ".rs":
		if p, err := exec.LookPath("rustfmt"); err == nil {
			cmd = exec.Command(p, path)
		}
	}
	if cmd != nil {
		_ = cmd.Run()
	}
}
