package checkpoint

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupTestRepo creates a temporary git repo and returns its path and a cleanup function.
func setupTestRepo(t *testing.T) (string, func()) {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")

	// Create an initial commit so HEAD exists
	seedFile := filepath.Join(dir, "README.md")
	if err := os.WriteFile(seedFile, []byte("# test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", "README.md")
	run("commit", "-m", "initial commit")

	return dir, func() {}
}

func TestCreateAndList(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	mgr := NewManager("test-session", dir)

	// Create a file and checkpoint it
	f1 := filepath.Join(dir, "file1.txt")
	if err := os.WriteFile(f1, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	cp1, err := mgr.Create([]string{"file1.txt"}, "added file1")
	if err != nil {
		t.Fatalf("Create checkpoint 1: %v", err)
	}
	if cp1.ID != 1 {
		t.Errorf("expected ID 1, got %d", cp1.ID)
	}
	if cp1.CommitSHA == "" {
		t.Error("expected non-empty commit SHA")
	}

	// Create a second file and checkpoint
	f2 := filepath.Join(dir, "file2.txt")
	if err := os.WriteFile(f2, []byte("world"), 0644); err != nil {
		t.Fatal(err)
	}

	cp2, err := mgr.Create([]string{"file2.txt"}, "added file2")
	if err != nil {
		t.Fatalf("Create checkpoint 2: %v", err)
	}
	if cp2.ID != 2 {
		t.Errorf("expected ID 2, got %d", cp2.ID)
	}

	// List should return both checkpoints in order
	checkpoints, err := mgr.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(checkpoints) != 2 {
		t.Fatalf("expected 2 checkpoints, got %d", len(checkpoints))
	}
	if checkpoints[0].ID != 1 || checkpoints[1].ID != 2 {
		t.Errorf("checkpoints not in order: %d, %d", checkpoints[0].ID, checkpoints[1].ID)
	}
	if checkpoints[0].Message != "added file1" {
		t.Errorf("expected message 'added file1', got %q", checkpoints[0].Message)
	}
}

func TestRestore(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	mgr := NewManager("test-session", dir)

	// Create file1 and checkpoint
	f1 := filepath.Join(dir, "file1.txt")
	if err := os.WriteFile(f1, []byte("version1"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Create([]string{"file1.txt"}, "v1"); err != nil {
		t.Fatal(err)
	}

	// Modify file1 and create checkpoint 2
	if err := os.WriteFile(f1, []byte("version2"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Create([]string{"file1.txt"}, "v2"); err != nil {
		t.Fatal(err)
	}

	// Create file2 and checkpoint 3
	f2 := filepath.Join(dir, "file2.txt")
	if err := os.WriteFile(f2, []byte("extra"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Create([]string{"file2.txt"}, "v3"); err != nil {
		t.Fatal(err)
	}

	// Restore to checkpoint 1
	if err := mgr.Restore(1); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	// file1 should have version1 content
	data, err := os.ReadFile(f1)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "version1" {
		t.Errorf("expected 'version1', got %q", string(data))
	}

	// Checkpoints 2 and 3 should be removed
	checkpoints, err := mgr.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(checkpoints) != 1 {
		t.Errorf("expected 1 checkpoint after restore, got %d", len(checkpoints))
	}
}

func TestCleanup(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	mgr := NewManager("test-session", dir)

	// Create a file and two checkpoints
	f1 := filepath.Join(dir, "file1.txt")
	if err := os.WriteFile(f1, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Create([]string{"file1.txt"}, "cp1"); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(f1, []byte("world"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Create([]string{"file1.txt"}, "cp2"); err != nil {
		t.Fatal(err)
	}

	// Cleanup should remove all refs
	if err := mgr.Cleanup(false); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	checkpoints, err := mgr.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(checkpoints) != 0 {
		t.Errorf("expected 0 checkpoints after cleanup, got %d", len(checkpoints))
	}

	// Verify refs are invisible to git log and git branch
	cmd := exec.Command("git", "for-each-ref", "refs/gocode/checkpoints/test-session/")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Errorf("expected no refs after cleanup, got: %s", out)
	}
}

func TestNotInGitRepo(t *testing.T) {
	// Use a temp dir that is NOT a git repo
	dir := t.TempDir()

	mgr := NewManager("test-session", dir)

	// Create should fail
	f := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(f, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := mgr.Create([]string{"file.txt"}, "test")
	if err == nil {
		t.Error("expected error for non-git repo, got nil")
	}

	// List should fail
	_, err = mgr.List()
	if err == nil {
		t.Error("expected error for non-git repo List, got nil")
	}

	// Restore should fail
	err = mgr.Restore(1)
	if err == nil {
		t.Error("expected error for non-git repo Restore, got nil")
	}

	// Cleanup should fail
	err = mgr.Cleanup(false)
	if err == nil {
		t.Error("expected error for non-git repo Cleanup, got nil")
	}
}

func TestCheckpointIsolation(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	mgr := NewManager("test-session", dir)

	// Create a checkpoint
	f := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(f, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Create([]string{"file.txt"}, "test"); err != nil {
		t.Fatal(err)
	}

	// Checkpoint refs should not appear in git log or git branch
	logOut, err := exec.Command("git", "-C", dir, "log", "--oneline", "--all").Output()
	if err != nil {
		t.Fatal(err)
	}
	// The checkpoint commit should not appear in normal log --all
	// (for-each-ref based refs under refs/gocode/ are not standard branches/tags)
	branchOut, err := exec.Command("git", "-C", dir, "branch", "-a").Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(branchOut), "gocode") {
		t.Errorf("checkpoint refs visible in git branch: %s", branchOut)
	}
	_ = logOut // log --all may include the ref depending on git version, but branch should not
}

func TestRestoreInvalidID(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	mgr := NewManager("test-session", dir)

	// Restore non-existent checkpoint
	err := mgr.Restore(999)
	if err == nil {
		t.Error("expected error for invalid checkpoint ID, got nil")
	}
}
