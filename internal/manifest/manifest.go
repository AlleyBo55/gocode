package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gocode/gocode/internal/models"
)

// PortManifest holds the result of scanning a source directory.
type PortManifest struct {
	SrcRoot         string             `json:"src_root"`
	TotalGoFiles    int                `json:"total_go_files"`
	TopLevelModules []models.Subsystem `json:"top_level_modules"`
}

// BuildPortManifest scans srcDir for Go files and builds a manifest.
// Returns an error if the directory does not exist.
func BuildPortManifest(srcDir string) (*PortManifest, error) {
	info, err := os.Stat(srcDir)
	if err != nil {
		return nil, fmt.Errorf("scanning source directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", srcDir)
	}

	counts := make(map[string]int)
	total := 0

	err = filepath.Walk(srcDir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if fi.IsDir() || !strings.HasSuffix(fi.Name(), ".go") {
			return nil
		}
		total++
		rel, _ := filepath.Rel(srcDir, path)
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) > 1 {
			counts[parts[0]]++
		} else {
			counts[fi.Name()]++
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking source directory: %w", err)
	}

	type kv struct {
		key   string
		count int
	}
	sorted := make([]kv, 0, len(counts))
	for k, v := range counts {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].count > sorted[j].count })

	modules := make([]models.Subsystem, len(sorted))
	for i, s := range sorted {
		modules[i] = models.Subsystem{
			Name:      s.key,
			Path:      filepath.Join(srcDir, s.key),
			FileCount: s.count,
			Notes:     "Go port module",
		}
	}

	return &PortManifest{
		SrcRoot:         srcDir,
		TotalGoFiles:    total,
		TopLevelModules: modules,
	}, nil
}

// Render returns a Markdown-formatted representation of the manifest.
func (m *PortManifest) Render() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Port root: `%s`\n", m.SrcRoot))
	b.WriteString(fmt.Sprintf("Total Go files: **%d**\n\n", m.TotalGoFiles))
	b.WriteString("Top-level Go modules:\n")
	for _, mod := range m.TopLevelModules {
		b.WriteString(fmt.Sprintf("- `%s` (%d files) — %s\n", mod.Name, mod.FileCount, mod.Notes))
	}
	return b.String()
}
