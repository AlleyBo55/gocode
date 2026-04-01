package portcontext

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PortContext holds workspace scanning results for a porting project.
type PortContext struct {
	SourceRoot  string `json:"source_root"`
	TestsRoot   string `json:"tests_root"`
	AssetsRoot  string `json:"assets_root"`
	ArchiveRoot string `json:"archive_root"`

	GoFileCount    int  `json:"go_file_count"`
	TestFileCount  int  `json:"test_file_count"`
	AssetFileCount int  `json:"asset_file_count"`
	ArchiveAvailable bool `json:"archive_available"`
}

// BuildPortContext scans the workspace rooted at root and returns a PortContext
// with file counts and path information. Missing directories result in zero
// counts, not errors.
func BuildPortContext(root string) (*PortContext, error) {
	sourceRoot := resolveSourceRoot(root)
	testsRoot := filepath.Join(root, "tests")
	assetsRoot := filepath.Join(root, "assets")
	archiveRoot := filepath.Join(root, "archive")

	ctx := &PortContext{
		SourceRoot:  sourceRoot,
		TestsRoot:   testsRoot,
		AssetsRoot:  assetsRoot,
		ArchiveRoot: archiveRoot,
	}

	ctx.GoFileCount = countFiles(sourceRoot, func(name string) bool {
		return strings.HasSuffix(name, ".go")
	})

	ctx.TestFileCount = countFiles(testsRoot, func(name string) bool {
		return strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, ".test.go")
	})

	ctx.AssetFileCount = countFiles(assetsRoot, func(_ string) bool {
		return true
	})

	_, err := os.Stat(archiveRoot)
	ctx.ArchiveAvailable = err == nil

	return ctx, nil
}

// Render returns a Markdown-formatted string describing the workspace context.
func (c *PortContext) Render() string {
	var b strings.Builder
	b.WriteString("## Workspace Context\n\n")
	b.WriteString(fmt.Sprintf("- **Source Root**: `%s`\n", c.SourceRoot))
	b.WriteString(fmt.Sprintf("- **Tests Root**: `%s`\n", c.TestsRoot))
	b.WriteString(fmt.Sprintf("- **Assets Root**: `%s`\n", c.AssetsRoot))
	b.WriteString(fmt.Sprintf("- **Archive Root**: `%s`\n", c.ArchiveRoot))
	b.WriteString("\n### File Counts\n\n")
	b.WriteString(fmt.Sprintf("- Go files: %d\n", c.GoFileCount))
	b.WriteString(fmt.Sprintf("- Test files: %d\n", c.TestFileCount))
	b.WriteString(fmt.Sprintf("- Asset files: %d\n", c.AssetFileCount))
	b.WriteString(fmt.Sprintf("- Archive available: %t\n", c.ArchiveAvailable))
	return b.String()
}

// resolveSourceRoot returns root/src if it exists, otherwise root/internal.
func resolveSourceRoot(root string) string {
	srcPath := filepath.Join(root, "src")
	if info, err := os.Stat(srcPath); err == nil && info.IsDir() {
		return srcPath
	}
	return filepath.Join(root, "internal")
}

// countFiles walks dir and counts files matching the predicate.
// Returns 0 if the directory does not exist.
func countFiles(dir string, match func(string) bool) int {
	count := 0
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip inaccessible entries
		}
		if !info.IsDir() && match(info.Name()) {
			count++
		}
		return nil
	})
	return count
}
