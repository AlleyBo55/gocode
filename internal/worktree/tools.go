package worktree

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/AlleyBo55/gocode/internal/toolimpl"
)

// EnterWorktreeTool implements toolimpl.ToolExecutor.
// It creates or enters a worktree, calling os.Chdir() to scope file operations.
type EnterWorktreeTool struct {
	Mgr     *Manager
	origDir string // saved so ExitWorktreeTool can restore
}

// Execute creates or enters a worktree for the given branch.
// Params: "branch" (required), "path" (optional — defaults to <repoRoot>/.worktrees/<branch>).
func (t *EnterWorktreeTool) Execute(params map[string]interface{}) toolimpl.ToolResult {
	branch, _ := params["branch"].(string)
	if branch == "" {
		return toolimpl.ToolResult{Success: false, Error: "missing required parameter: branch"}
	}

	wtPath, _ := params["path"].(string)
	if wtPath == "" {
		wtPath = filepath.Join(t.Mgr.RepoRoot, ".worktrees", branch)
	}

	// Save original directory before changing.
	origDir, err := os.Getwd()
	if err != nil {
		return toolimpl.ToolResult{Success: false, Error: fmt.Sprintf("getting current directory: %v", err)}
	}
	t.origDir = origDir

	// Create the worktree if the path doesn't exist yet.
	if _, statErr := os.Stat(wtPath); os.IsNotExist(statErr) {
		if createErr := t.Mgr.Create(branch, wtPath); createErr != nil {
			return toolimpl.ToolResult{Success: false, Error: fmt.Sprintf("creating worktree: %v", createErr)}
		}
	}

	if err := os.Chdir(wtPath); err != nil {
		return toolimpl.ToolResult{Success: false, Error: fmt.Sprintf("entering worktree: %v", err)}
	}

	return toolimpl.ToolResult{
		Success: true,
		Output:  fmt.Sprintf("Entered worktree at %s (branch %s)", wtPath, branch),
	}
}

// OrigDir returns the saved original directory (used by ExitWorktreeTool).
func (t *EnterWorktreeTool) OrigDir() string {
	return t.origDir
}

// ExitWorktreeTool implements toolimpl.ToolExecutor.
// It restores the original working directory.
type ExitWorktreeTool struct {
	enterTool *EnterWorktreeTool
}

// Execute restores the original working directory saved by EnterWorktreeTool.
func (t *ExitWorktreeTool) Execute(params map[string]interface{}) toolimpl.ToolResult {
	origDir := t.enterTool.OrigDir()
	if origDir == "" {
		return toolimpl.ToolResult{Success: false, Error: "not currently in a worktree (no saved directory)"}
	}

	if err := os.Chdir(origDir); err != nil {
		return toolimpl.ToolResult{Success: false, Error: fmt.Sprintf("restoring directory: %v", err)}
	}

	return toolimpl.ToolResult{
		Success: true,
		Output:  fmt.Sprintf("Exited worktree, restored to %s", origDir),
	}
}

// RegisterWorktreeTools registers EnterWorktreeTool and ExitWorktreeTool in the registry.
func RegisterWorktreeTools(registry *toolimpl.Registry, mgr *Manager) {
	enter := &EnterWorktreeTool{Mgr: mgr}
	exit := &ExitWorktreeTool{enterTool: enter}
	registry.Set("enterworktree", enter)
	registry.Set("exitworktree", exit)
}
