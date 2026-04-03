package customcmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAll_BasicCommand(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "project")
	userDir := filepath.Join(dir, "user")
	os.MkdirAll(projectDir, 0o755)

	content := "---\ndescription: \"Test command\"\n---\nHello world\n"
	os.WriteFile(filepath.Join(projectDir, "greet.md"), []byte(content), 0o644)

	loader := NewLoaderWithDirs(projectDir, userDir)
	cmds, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	if cmds[0].Name != "greet" {
		t.Errorf("name = %q, want %q", cmds[0].Name, "greet")
	}
	if cmds[0].Description != "Test command" {
		t.Errorf("description = %q, want %q", cmds[0].Description, "Test command")
	}
	if cmds[0].Body != "Hello world\n" {
		t.Errorf("body = %q, want %q", cmds[0].Body, "Hello world\n")
	}
	if cmds[0].Source != "project" {
		t.Errorf("source = %q, want %q", cmds[0].Source, "project")
	}
}

func TestLoadAll_ProjectOverridesUser(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "project")
	userDir := filepath.Join(dir, "user")
	os.MkdirAll(projectDir, 0o755)
	os.MkdirAll(userDir, 0o755)

	os.WriteFile(filepath.Join(userDir, "deploy.md"), []byte("user deploy"), 0o644)
	os.WriteFile(filepath.Join(projectDir, "deploy.md"), []byte("project deploy"), 0o644)

	loader := NewLoaderWithDirs(projectDir, userDir)
	cmds, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	if cmds[0].Source != "project" {
		t.Errorf("source = %q, want %q (project should override user)", cmds[0].Source, "project")
	}
	if cmds[0].Body != "project deploy" {
		t.Errorf("body = %q, want %q", cmds[0].Body, "project deploy")
	}
}

func TestLoadAll_NoFrontmatter(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "project")
	os.MkdirAll(projectDir, 0o755)

	content := "Just a plain prompt with no frontmatter."
	os.WriteFile(filepath.Join(projectDir, "plain.md"), []byte(content), 0o644)

	loader := NewLoaderWithDirs(projectDir, filepath.Join(dir, "user"))
	cmds, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	if cmds[0].Body != content {
		t.Errorf("body = %q, want %q", cmds[0].Body, content)
	}
	if cmds[0].Description != "" {
		t.Errorf("description should be empty, got %q", cmds[0].Description)
	}
}

func TestLoadAll_EmptyDirectories(t *testing.T) {
	dir := t.TempDir()
	loader := NewLoaderWithDirs(filepath.Join(dir, "project"), filepath.Join(dir, "user"))
	cmds, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(cmds) != 0 {
		t.Errorf("expected 0 commands, got %d", len(cmds))
	}
}

func TestLoadAll_WithArgs(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "project")
	os.MkdirAll(projectDir, 0o755)

	content := "---\ndescription: \"Deploy tool\"\nargs:\n  - name: target\n    required: true\n  - name: mode\n    required: false\n---\nDeploy $1 in $2 mode.\n"
	os.WriteFile(filepath.Join(projectDir, "deploy.md"), []byte(content), 0o644)

	loader := NewLoaderWithDirs(projectDir, filepath.Join(dir, "user"))
	cmds, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	if len(cmds[0].Args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(cmds[0].Args))
	}
	if cmds[0].Args[0].Name != "target" || !cmds[0].Args[0].Required {
		t.Errorf("arg[0] = %+v, want {Name:target Required:true}", cmds[0].Args[0])
	}
	if cmds[0].Args[1].Name != "mode" || cmds[0].Args[1].Required {
		t.Errorf("arg[1] = %+v, want {Name:mode Required:false}", cmds[0].Args[1])
	}
}

func TestResolve_Substitution(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "project")
	os.MkdirAll(projectDir, 0o755)

	content := "---\ndescription: \"Test\"\nargs:\n  - name: target\n    required: true\n---\nAnalyze $1 now.\n"
	os.WriteFile(filepath.Join(projectDir, "analyze.md"), []byte(content), 0o644)

	loader := NewLoaderWithDirs(projectDir, filepath.Join(dir, "user"))
	result, err := loader.Resolve("analyze", []string{"myfile.go"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	expected := "Analyze myfile.go now.\n"
	if result != expected {
		t.Errorf("result = %q, want %q", result, expected)
	}
}

func TestResolve_MissingRequiredArg(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "project")
	os.MkdirAll(projectDir, 0o755)

	content := "---\ndescription: \"Test\"\nargs:\n  - name: target\n    required: true\n---\nDo $1.\n"
	os.WriteFile(filepath.Join(projectDir, "cmd.md"), []byte(content), 0o644)

	loader := NewLoaderWithDirs(projectDir, filepath.Join(dir, "user"))
	_, err := loader.Resolve("cmd", nil)
	if err == nil {
		t.Fatal("expected error for missing required arg")
	}
}

func TestResolve_UnknownCommand(t *testing.T) {
	dir := t.TempDir()
	loader := NewLoaderWithDirs(filepath.Join(dir, "project"), filepath.Join(dir, "user"))
	_, err := loader.Resolve("nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
}

func TestLoadAll_SkipsNonMdFiles(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "project")
	os.MkdirAll(projectDir, 0o755)

	os.WriteFile(filepath.Join(projectDir, "valid.md"), []byte("hello"), 0o644)
	os.WriteFile(filepath.Join(projectDir, "readme.txt"), []byte("not a command"), 0o644)
	os.MkdirAll(filepath.Join(projectDir, "subdir"), 0o755)

	loader := NewLoaderWithDirs(projectDir, filepath.Join(dir, "user"))
	cmds, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(cmds) != 1 {
		t.Errorf("expected 1 command (only .md), got %d", len(cmds))
	}
}

func TestLoadAll_BothDirs(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "project")
	userDir := filepath.Join(dir, "user")
	os.MkdirAll(projectDir, 0o755)
	os.MkdirAll(userDir, 0o755)

	os.WriteFile(filepath.Join(userDir, "global.md"), []byte("global cmd"), 0o644)
	os.WriteFile(filepath.Join(projectDir, "local.md"), []byte("local cmd"), 0o644)

	loader := NewLoaderWithDirs(projectDir, userDir)
	cmds, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(cmds))
	}

	names := map[string]bool{}
	for _, c := range cmds {
		names[c.Name] = true
	}
	if !names["global"] || !names["local"] {
		t.Errorf("expected commands 'global' and 'local', got %v", names)
	}
}
