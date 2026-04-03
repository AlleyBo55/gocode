package worktree

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Context holds the detected worktree context for the current instance.
type Context struct {
	WorktreeRoot string // root of the current worktree (or repo root)
	RepoRoot     string // root of the main repository
	IsWorktree   bool   // true if running inside a non-main worktree
	Branch       string // current branch name
}

// WorktreeInfo describes an active worktree.
type WorktreeInfo struct {
	Path   string
	Branch string
	HEAD   string
}

// Manager handles worktree creation, listing, and removal.
type Manager struct {
	RepoRoot string
}

// NewManager creates a Manager rooted at the given repository path.
func NewManager(repoRoot string) *Manager {
	return &Manager{RepoRoot: repoRoot}
}

// Detect inspects the current directory and returns worktree context.
// It runs git rev-parse to discover the toplevel and git-common-dir.
func Detect() (*Context, error) {
	toplevel, err := gitOutput("rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("detecting worktree: not a git repository: %w", err)
	}

	commonDir, err := gitOutput("rev-parse", "--git-common-dir")
	if err != nil {
		return nil, fmt.Errorf("detecting worktree common dir: %w", err)
	}

	// Resolve to absolute paths for reliable comparison.
	toplevel, _ = filepath.Abs(toplevel)
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(toplevel, commonDir)
	}
	commonDir, _ = filepath.Abs(commonDir)

	// The main repo root is the parent of the common .git directory.
	repoRoot := filepath.Dir(commonDir)
	// If commonDir ends with ".git", repoRoot is its parent.
	if filepath.Base(commonDir) == ".git" {
		repoRoot = filepath.Dir(commonDir)
	}

	branch, _ := gitOutput("rev-parse", "--abbrev-ref", "HEAD")

	isWorktree := toplevel != repoRoot

	return &Context{
		WorktreeRoot: toplevel,
		RepoRoot:     repoRoot,
		IsWorktree:   isWorktree,
		Branch:       branch,
	}, nil
}

// Create creates a new git worktree at the given path for the specified branch.
func (m *Manager) Create(branch, path string) error {
	if !filepath.IsAbs(path) {
		path = filepath.Join(m.RepoRoot, path)
	}
	cmd := exec.Command("git", "worktree", "add", path, branch)
	cmd.Dir = m.RepoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("creating worktree: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// List returns all active worktrees by parsing `git worktree list --porcelain`.
func (m *Manager) List() ([]WorktreeInfo, error) {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = m.RepoRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("listing worktrees: %w", err)
	}
	return ParseWorktreeList(string(out)), nil
}

// Remove removes a worktree by path.
func (m *Manager) Remove(path string) error {
	cmd := exec.Command("git", "worktree", "remove", path)
	cmd.Dir = m.RepoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("removing worktree: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// ParseWorktreeList parses the porcelain output of `git worktree list --porcelain`.
func ParseWorktreeList(output string) []WorktreeInfo {
	var worktrees []WorktreeInfo
	var current WorktreeInfo
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			if current.Path != "" {
				worktrees = append(worktrees, current)
				current = WorktreeInfo{}
			}
			continue
		}
		if strings.HasPrefix(line, "worktree ") {
			current.Path = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "HEAD ") {
			current.HEAD = strings.TrimPrefix(line, "HEAD ")
		} else if strings.HasPrefix(line, "branch ") {
			current.Branch = strings.TrimPrefix(line, "branch ")
		}
	}
	// Flush last entry if output doesn't end with blank line.
	if current.Path != "" {
		worktrees = append(worktrees, current)
	}
	return worktrees
}

// Lock acquires an advisory file lock for the given worktree path.
// It creates a .gocode.lock file with O_CREATE|O_EXCL to ensure exclusivity.
// Returns a release function that removes the lock file.
func Lock(worktreePath string) (release func(), err error) {
	lockPath := filepath.Join(worktreePath, ".gocode.lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("acquiring lock on %s: %w", worktreePath, err)
	}
	f.Close()
	return func() {
		os.Remove(lockPath)
	}, nil
}

// gitOutput runs a git command and returns trimmed stdout.
func gitOutput(args ...string) (string, error) {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// SessionDir returns the session storage directory scoped to the current
// worktree. When running inside a worktree, sessions are stored under
// <WorktreeRoot>/.port_sessions to isolate them from other worktrees.
// When not in a worktree (or detection fails), it returns the default
// empty string so the caller falls back to the normal ".port_sessions".
func SessionDir() string {
	ctx, err := Detect()
	if err != nil {
		return ""
	}
	if ctx.IsWorktree {
		return filepath.Join(ctx.WorktreeRoot, ".port_sessions")
	}
	return ""
}
