// Package initdeep scans a project directory tree and generates hierarchical
// AGENTS.md context files. Each directory containing source code files gets an
// AGENTS.md summarising its purpose, key files, and child directories. Existing
// AGENTS.md files are preserved (never overwritten).
package initdeep

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// sourceExtensions lists file extensions considered "source code".
var sourceExtensions = map[string]bool{
	".go":    true,
	".js":    true,
	".ts":    true,
	".py":    true,
	".java":  true,
	".rs":    true,
	".c":     true,
	".cpp":   true,
	".h":     true,
	".rb":    true,
	".swift": true,
	".kt":    true,
}

// Generator scans a project and creates hierarchical AGENTS.md files.
type Generator struct {
	skipDirs map[string]bool
}

// NewGenerator creates a generator with default skip directories.
func NewGenerator() *Generator {
	return &Generator{
		skipDirs: map[string]bool{
			"node_modules": true,
			"vendor":       true,
			".git":         true,
			"dist":         true,
		},
	}
}

// GenerateReport summarises what was created and what was skipped.
type GenerateReport struct {
	Created []string // paths of created AGENTS.md files
	Skipped []string // paths where AGENTS.md already existed
}

// Generate walks the directory tree from root, creating AGENTS.md in each
// directory that contains source files. It skips directories in the skipDirs
// set and respects .gitignore patterns at the root level. Existing AGENTS.md
// files are preserved and recorded in the Skipped list.
func (g *Generator) Generate(root string) (GenerateReport, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return GenerateReport{}, fmt.Errorf("resolving root path: %w", err)
	}

	// Load .gitignore patterns from root (best-effort).
	ignorePatterns := loadGitignore(filepath.Join(root, ".gitignore"))

	var report GenerateReport

	err = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil // skip unreadable entries
		}

		if !info.IsDir() {
			return nil
		}

		// Get the directory name for skip-dir checks.
		dirName := filepath.Base(path)
		if g.skipDirs[dirName] && path != root {
			return filepath.SkipDir
		}

		// Check .gitignore patterns against the relative path.
		rel, err := filepath.Rel(root, path)
		if err == nil && rel != "." && isGitignored(rel, ignorePatterns) {
			return filepath.SkipDir
		}

		// Check if AGENTS.md already exists.
		agentsPath := filepath.Join(path, "AGENTS.md")
		if _, err := os.Stat(agentsPath); err == nil {
			report.Skipped = append(report.Skipped, agentsPath)
			return nil
		}

		// Collect source files and subdirectories in this directory.
		sourceFiles, subDirs, err := listDirContents(path)
		if err != nil {
			return nil // skip unreadable dirs
		}

		// Only create AGENTS.md if there are source files.
		if len(sourceFiles) == 0 {
			return nil
		}

		content := buildAgentsMD(dirName, sourceFiles, subDirs)
		if err := os.WriteFile(agentsPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("writing %s: %w", agentsPath, err)
		}
		report.Created = append(report.Created, agentsPath)

		return nil
	})

	if err != nil {
		return report, err
	}
	return report, nil
}

// FindNearestAgentsMD walks up from dir to find the nearest AGENTS.md file.
// Returns the path to the file, or an error if none is found up to the
// filesystem root.
func FindNearestAgentsMD(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolving directory: %w", err)
	}

	for {
		candidate := filepath.Join(dir, "AGENTS.md")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root.
			return "", fmt.Errorf("no AGENTS.md found in %s or any ancestor directory", dir)
		}
		dir = parent
	}
}

// listDirContents returns the source files and immediate subdirectory names
// in the given directory (non-recursive).
func listDirContents(dir string) (sourceFiles []string, subDirs []string, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			subDirs = append(subDirs, entry.Name())
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if sourceExtensions[ext] {
			sourceFiles = append(sourceFiles, entry.Name())
		}
	}

	sort.Strings(sourceFiles)
	sort.Strings(subDirs)
	return sourceFiles, subDirs, nil
}

// buildAgentsMD produces the Markdown content for an AGENTS.md file.
func buildAgentsMD(dirName string, sourceFiles []string, subDirs []string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# %s\n\n", dirName)
	fmt.Fprintf(&b, "This directory contains source code for the `%s` package.\n\n", dirName)

	b.WriteString("## Source Files\n\n")
	for _, f := range sourceFiles {
		fmt.Fprintf(&b, "- `%s`\n", f)
	}
	b.WriteString("\n")

	if len(subDirs) > 0 {
		b.WriteString("## Subdirectories\n\n")
		for _, d := range subDirs {
			fmt.Fprintf(&b, "- `%s/`\n", d)
		}
		b.WriteString("\n")
	}

	return b.String()
}

// loadGitignore reads a .gitignore file and returns a list of non-empty,
// non-comment patterns. Returns nil if the file doesn't exist.
func loadGitignore(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var patterns []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns
}

// isGitignored checks whether a relative path matches any of the gitignore
// patterns. This is a simplified matcher that handles common cases:
// directory patterns (ending with /), wildcard patterns, and exact prefixes.
func isGitignored(relPath string, patterns []string) bool {
	// Normalise to forward slashes for matching.
	relPath = filepath.ToSlash(relPath)

	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}

		// Strip trailing slash for directory-only patterns — we only
		// call this function for directories anyway.
		cleanPattern := strings.TrimSuffix(pattern, "/")

		// Check if any path component matches the pattern exactly.
		parts := strings.Split(relPath, "/")
		for _, part := range parts {
			if matched, _ := filepath.Match(cleanPattern, part); matched {
				return true
			}
		}

		// Also check the full relative path against the pattern.
		if matched, _ := filepath.Match(cleanPattern, relPath); matched {
			return true
		}
	}
	return false
}
