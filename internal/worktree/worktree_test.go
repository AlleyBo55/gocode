package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initGitRepo creates a temporary git repo and returns its path.
func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.email", "test@test.com")
	run(t, dir, "git", "config", "user.name", "Test")
	// Create an initial commit so branches work.
	f := filepath.Join(dir, "README.md")
	os.WriteFile(f, []byte("# test"), 0644)
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-m", "init")
	return dir
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, string(out))
	}
}

func TestDetectInGitRepo(t *testing.T) {
	dir := initGitRepo(t)

	// Change to the repo directory for Detect().
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	ctx, err := Detect()
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}
	if ctx.WorktreeRoot == "" {
		t.Error("expected non-empty WorktreeRoot")
	}
	if ctx.RepoRoot == "" {
		t.Error("expected non-empty RepoRoot")
	}
	// In a normal repo (not a worktree), IsWorktree should be false.
	if ctx.IsWorktree {
		t.Error("expected IsWorktree=false in main repo")
	}
}

func TestDetectInNonGitDirectory(t *testing.T) {
	dir := t.TempDir()

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	_, err := Detect()
	if err == nil {
		t.Error("expected error when not in a git repo")
	}
}

func TestLockUnlock(t *testing.T) {
	dir := t.TempDir()

	release, err := Lock(dir)
	if err != nil {
		t.Fatalf("Lock() error: %v", err)
	}

	// Lock file should exist.
	lockPath := filepath.Join(dir, ".gocode.lock")
	if _, statErr := os.Stat(lockPath); os.IsNotExist(statErr) {
		t.Error("expected lock file to exist")
	}

	// Second lock should fail.
	_, err2 := Lock(dir)
	if err2 == nil {
		t.Error("expected second Lock() to fail")
	}

	// Release and re-acquire.
	release()
	if _, statErr := os.Stat(lockPath); !os.IsNotExist(statErr) {
		t.Error("expected lock file to be removed after release")
	}

	release2, err3 := Lock(dir)
	if err3 != nil {
		t.Fatalf("Lock() after release error: %v", err3)
	}
	release2()
}

func TestParseWorktreeList(t *testing.T) {
	// Simulate porcelain output from `git worktree list --porcelain`.
	output := `worktree /home/user/project
HEAD abc1234567890abcdef1234567890abcdef123456
branch refs/heads/main

worktree /home/user/project-feature
HEAD def4567890abcdef1234567890abcdef12345678
branch refs/heads/feature

`
	wts := ParseWorktreeList(output)
	if len(wts) != 2 {
		t.Fatalf("expected 2 worktrees, got %d", len(wts))
	}

	if wts[0].Path != "/home/user/project" {
		t.Errorf("wt[0].Path = %q, want /home/user/project", wts[0].Path)
	}
	if wts[0].Branch != "refs/heads/main" {
		t.Errorf("wt[0].Branch = %q, want refs/heads/main", wts[0].Branch)
	}
	if wts[0].HEAD != "abc1234567890abcdef1234567890abcdef123456" {
		t.Errorf("wt[0].HEAD = %q", wts[0].HEAD)
	}

	if wts[1].Path != "/home/user/project-feature" {
		t.Errorf("wt[1].Path = %q", wts[1].Path)
	}
	if wts[1].Branch != "refs/heads/feature" {
		t.Errorf("wt[1].Branch = %q", wts[1].Branch)
	}
}

func TestParseWorktreeListNoTrailingNewline(t *testing.T) {
	output := `worktree /tmp/repo
HEAD aaa1111111111111111111111111111111111111
branch refs/heads/main`

	wts := ParseWorktreeList(output)
	if len(wts) != 1 {
		t.Fatalf("expected 1 worktree, got %d", len(wts))
	}
	if wts[0].Path != "/tmp/repo" {
		t.Errorf("Path = %q", wts[0].Path)
	}
}

func TestParseWorktreeListEmpty(t *testing.T) {
	wts := ParseWorktreeList("")
	if len(wts) != 0 {
		t.Errorf("expected 0 worktrees, got %d", len(wts))
	}
}

func TestManagerListInRepo(t *testing.T) {
	dir := initGitRepo(t)
	mgr := NewManager(dir)

	wts, err := mgr.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	// A fresh repo should have at least the main worktree.
	if len(wts) < 1 {
		t.Error("expected at least 1 worktree (main)")
	}
	// The main worktree path should match the repo dir.
	found := false
	absDir, _ := filepath.EvalSymlinks(dir)
	for _, wt := range wts {
		absWt, _ := filepath.EvalSymlinks(wt.Path)
		if absWt == absDir {
			found = true
		}
	}
	if !found {
		t.Errorf("main worktree not found in list; got %v", wts)
	}
}

func TestSessionDirDefault(t *testing.T) {
	// In a non-git directory, SessionDir should return "".
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	result := SessionDir()
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestDetectBranch(t *testing.T) {
	dir := initGitRepo(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	ctx, err := Detect()
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}
	// The default branch should be "main" or "master".
	if ctx.Branch == "" {
		t.Error("expected non-empty Branch")
	}
	if !strings.Contains(ctx.Branch, "main") && !strings.Contains(ctx.Branch, "master") {
		t.Logf("Branch = %q (not main/master, but that's ok for some git configs)", ctx.Branch)
	}
}
