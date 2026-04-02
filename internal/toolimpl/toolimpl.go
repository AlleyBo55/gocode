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
	"regexp"
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

// WebSearchTool searches Wikipedia, GitHub, and Reddit in parallel. No API key required.
type WebSearchTool struct{}

func (t *WebSearchTool) Execute(params map[string]interface{}) ToolResult {
	query, _ := params["query"].(string)
	if query == "" {
		return ToolResult{Success: false, Error: "missing required param: query. You MUST provide a search query string. Example: {\"query\": \"president of Indonesia 2024\"}"}
	}

	encodedQuery := strings.ReplaceAll(query, " ", "+")
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	type sourceResult struct {
		name    string
		content string
		err     error
	}

	ch := make(chan sourceResult, 5)

	// Wikipedia search + summary
	go func() {
		content, err := searchWikipedia(ctx, encodedQuery)
		ch <- sourceResult{"Wikipedia", content, err}
	}()

	// GitHub search
	go func() {
		content, err := searchGitHub(ctx, encodedQuery)
		ch <- sourceResult{"GitHub", content, err}
	}()

	// Reddit search
	go func() {
		content, err := searchReddit(ctx, encodedQuery)
		ch <- sourceResult{"Reddit", content, err}
	}()

	// Hacker News search
	go func() {
		content, err := searchHackerNews(ctx, encodedQuery)
		ch <- sourceResult{"Hacker News", content, err}
	}()

	// StackExchange search
	go func() {
		content, err := searchStackExchange(ctx, encodedQuery)
		ch <- sourceResult{"StackOverflow", content, err}
	}()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Search results for: %s\n\n", query))

	for i := 0; i < 5; i++ {
		r := <-ch
		if r.err == nil && r.content != "" {
			sb.WriteString(fmt.Sprintf("--- %s ---\n%s\n\n", r.name, r.content))
		}
	}

	result := sb.String()
	if len(result) < 50 {
		return ToolResult{Success: true, Output: fmt.Sprintf("No results found for %q across Wikipedia, GitHub, Reddit, Hacker News, and StackOverflow.", query)}
	}
	return ToolResult{Success: true, Output: result}
}

func searchWikipedia(ctx context.Context, query string) (string, error) {
	searchURL := fmt.Sprintf("https://en.wikipedia.org/w/api.php?action=query&list=search&srsearch=%s&format=json&srlimit=5", query)
	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "gocode/1.0 (https://github.com/AlleyBo55/gocode)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body := make([]byte, 32768)
	n, _ := resp.Body.Read(body)

	var sr struct {
		Query struct {
			Search []struct {
				Title   string `json:"title"`
				Snippet string `json:"snippet"`
			} `json:"search"`
		} `json:"query"`
	}
	if err := json.Unmarshal(body[:n], &sr); err != nil || len(sr.Query.Search) == 0 {
		return "", fmt.Errorf("no results")
	}

	var result strings.Builder

	// Get summaries + infobox data for top 2 results
	limit := 2
	if len(sr.Query.Search) < limit {
		limit = len(sr.Query.Search)
	}
	// Also try the base article title (e.g. "List of presidents of X" → also try "President of X")
	titlesToFetch := make([]string, 0, limit+2)
	for i := 0; i < limit; i++ {
		titlesToFetch = append(titlesToFetch, sr.Query.Search[i].Title)
	}
	// Derive related titles: strip "List of" prefix, strip year suffixes
	for _, t := range sr.Query.Search {
		if strings.HasPrefix(t.Title, "List of ") {
			base := strings.TrimPrefix(t.Title, "List of ")
			// "presidents of Indonesia" → "President of Indonesia"
			if strings.HasPrefix(base, "presidents") {
				base = "President" + strings.TrimPrefix(base, "presidents")
			}
			if strings.HasPrefix(base, "vice presidents") {
				base = "Vice President" + strings.TrimPrefix(base, "vice presidents")
			}
			titlesToFetch = append(titlesToFetch, base)
		}
	}

	for _, rawTitle := range titlesToFetch {
		title := strings.ReplaceAll(rawTitle, " ", "_")

		// Get page summary
		sumURL := fmt.Sprintf("https://en.wikipedia.org/api/rest_v1/page/summary/%s", title)
		sumReq, _ := http.NewRequestWithContext(ctx, "GET", sumURL, nil)
		sumReq.Header.Set("User-Agent", "gocode/1.0 (https://github.com/AlleyBo55/gocode)")
		if sumResp, err := http.DefaultClient.Do(sumReq); err == nil {
			sumBody := make([]byte, 32768)
			sn, _ := sumResp.Body.Read(sumBody)
			sumResp.Body.Close()
			var summary struct {
				Title   string `json:"title"`
				Extract string `json:"extract"`
			}
			if json.Unmarshal(sumBody[:sn], &summary) == nil && summary.Extract != "" {
				result.WriteString(fmt.Sprintf("%s\n%s\nhttps://en.wikipedia.org/wiki/%s\n", summary.Title, summary.Extract, title))
			}
		}

		// Also get infobox data from raw wikitext (has current incumbent, dates, etc.)
		infobox := fetchWikiInfobox(ctx, title)
		if infobox != "" {
			result.WriteString(fmt.Sprintf("Infobox data: %s\n", infobox))
		}
	}

	// Include search snippets
	for _, r := range sr.Query.Search {
		snippet := stripHTML(r.Snippet)
		if snippet != "" {
			result.WriteString(fmt.Sprintf("- %s: %s\n", r.Title, snippet))
		}
	}

	if result.Len() == 0 {
		return "", fmt.Errorf("no results")
	}
	return result.String(), nil
}

// fetchWikiInfobox extracts key-value pairs from a Wikipedia article's infobox.
// This gets dynamic data like current incumbent, population, dates, etc.
func fetchWikiInfobox(ctx context.Context, title string) string {
	url := fmt.Sprintf("https://en.wikipedia.org/w/api.php?action=query&titles=%s&prop=revisions&rvprop=content&rvsection=0&rvslots=main&format=json", title)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "gocode/1.0 (https://github.com/AlleyBo55/gocode)")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	body := make([]byte, 65536)
	n, _ := resp.Body.Read(body)

	var wr struct {
		Query struct {
			Pages map[string]struct {
				Revisions []struct {
					Slots struct {
						Main struct {
							Content string `json:"*"`
						} `json:"main"`
					} `json:"slots"`
				} `json:"revisions"`
			} `json:"pages"`
		} `json:"query"`
	}
	if json.Unmarshal(body[:n], &wr) != nil {
		return ""
	}

	// Extract useful infobox fields
	var parts []string
	interestingFields := []string{
		"incumbent", "officeholder", "leader_name", "leader_title",
		"incumbentsince", "president", "vice_president", "vicepresident",
		"capital", "population_estimate", "area_km2", "gdp_nominal",
		"established", "founded", "winner", "champion", "gold",
	}

	for _, page := range wr.Query.Pages {
		if len(page.Revisions) == 0 {
			continue
		}
		content := page.Revisions[0].Slots.Main.Content
		for _, field := range interestingFields {
			// Match patterns like: | incumbent = [[Prabowo Subianto]]
			patterns := []string{
				fmt.Sprintf(`(?i)\|\s*%s\s*=\s*\[\[(.*?)\]\]`, field),
				fmt.Sprintf(`(?i)\|\s*%s\s*=\s*([^\n|]+)`, field),
			}
			for _, pat := range patterns {
				re := regexp.MustCompile(pat)
				if m := re.FindStringSubmatch(content); len(m) > 1 {
					val := strings.TrimSpace(m[1])
					// Clean up wiki markup
					val = strings.ReplaceAll(val, "[[", "")
					val = strings.ReplaceAll(val, "]]", "")
					val = strings.ReplaceAll(val, "{{", "")
					val = strings.ReplaceAll(val, "}}", "")
					if val != "" && len(val) < 200 {
						parts = append(parts, fmt.Sprintf("%s: %s", field, val))
					}
					break
				}
			}
		}
	}

	return strings.Join(parts, "; ")
}

func searchGitHub(ctx context.Context, query string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/search/repositories?q=%s&per_page=5&sort=stars", query)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "gocode/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body := make([]byte, 32768)
	n, _ := resp.Body.Read(body)

	var gr struct {
		TotalCount int `json:"total_count"`
		Items      []struct {
			FullName    string `json:"full_name"`
			Description string `json:"description"`
			Stars       int    `json:"stargazers_count"`
			HTMLURL     string `json:"html_url"`
			Language    string `json:"language"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body[:n], &gr); err != nil || len(gr.Items) == 0 {
		return "", fmt.Errorf("no results")
	}

	var sb strings.Builder
	for _, item := range gr.Items {
		desc := item.Description
		if len(desc) > 120 {
			desc = desc[:120] + "..."
		}
		lang := ""
		if item.Language != "" {
			lang = fmt.Sprintf(" [%s]", item.Language)
		}
		sb.WriteString(fmt.Sprintf("⭐ %d  %s%s\n  %s\n  %s\n", item.Stars, item.FullName, lang, desc, item.HTMLURL))
	}
	return sb.String(), nil
}

func searchReddit(ctx context.Context, query string) (string, error) {
	url := fmt.Sprintf("https://www.reddit.com/search.json?q=%s&limit=5&sort=relevance", query)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "gocode/1.0 (https://github.com/AlleyBo55/gocode)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body := make([]byte, 32768)
	n, _ := resp.Body.Read(body)

	var rr struct {
		Data struct {
			Children []struct {
				Data struct {
					Title     string  `json:"title"`
					Subreddit string  `json:"subreddit_name_prefixed"`
					Score     int     `json:"score"`
					URL       string  `json:"url"`
					Selftext  string  `json:"selftext"`
					NumComments int   `json:"num_comments"`
				} `json:"data"`
			} `json:"children"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body[:n], &rr); err != nil || len(rr.Data.Children) == 0 {
		return "", fmt.Errorf("no results")
	}

	var sb strings.Builder
	for _, child := range rr.Data.Children {
		d := child.Data
		selftext := d.Selftext
		if len(selftext) > 150 {
			selftext = selftext[:150] + "..."
		}
		sb.WriteString(fmt.Sprintf("↑%d 💬%d  %s  %s\n", d.Score, d.NumComments, d.Subreddit, d.Title))
		if selftext != "" {
			sb.WriteString(fmt.Sprintf("  %s\n", selftext))
		}
		sb.WriteString(fmt.Sprintf("  %s\n", d.URL))
	}
	return sb.String(), nil
}

func searchHackerNews(ctx context.Context, query string) (string, error) {
	url := fmt.Sprintf("https://hn.algolia.com/api/v1/search?query=%s&tags=story&hitsPerPage=5", query)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "gocode/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body := make([]byte, 32768)
	n, _ := resp.Body.Read(body)

	var hn struct {
		Hits []struct {
			Title    string `json:"title"`
			URL      string `json:"url"`
			Points   int    `json:"points"`
			NumComments int `json:"num_comments"`
			ObjectID string `json:"objectID"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(body[:n], &hn); err != nil || len(hn.Hits) == 0 {
		return "", fmt.Errorf("no results")
	}

	var sb strings.Builder
	for _, h := range hn.Hits {
		link := h.URL
		if link == "" {
			link = fmt.Sprintf("https://news.ycombinator.com/item?id=%s", h.ObjectID)
		}
		sb.WriteString(fmt.Sprintf("▲%d 💬%d  %s\n  %s\n", h.Points, h.NumComments, h.Title, link))
	}
	return sb.String(), nil
}

func searchStackExchange(ctx context.Context, query string) (string, error) {
	url := fmt.Sprintf("https://api.stackexchange.com/2.3/search/advanced?order=desc&sort=relevance&q=%s&site=stackoverflow&pagesize=5&filter=default", query)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "gocode/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body := make([]byte, 32768)
	n, _ := resp.Body.Read(body)

	var se struct {
		Items []struct {
			Title string `json:"title"`
			Link  string `json:"link"`
			Score int    `json:"score"`
			AnswerCount int `json:"answer_count"`
			IsAnswered bool `json:"is_answered"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body[:n], &se); err != nil || len(se.Items) == 0 {
		return "", fmt.Errorf("no results")
	}

	var sb strings.Builder
	for _, item := range se.Items {
		answered := "○"
		if item.IsAnswered {
			answered = "✓"
		}
		title := stripHTML(item.Title)
		sb.WriteString(fmt.Sprintf("%s ↑%d  %s\n  %s\n", answered, item.Score, title, item.Link))
	}
	return sb.String(), nil
}

// stripHTML removes HTML tags from a string.
func stripHTML(s string) string {
	for strings.Contains(s, "<") {
		start := strings.Index(s, "<")
		end := strings.Index(s, ">")
		if end > start {
			s = s[:start] + s[end+1:]
		} else {
			break
		}
	}
	return s
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
