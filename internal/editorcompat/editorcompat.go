// Package editorcompat detects the current editor environment and adapts behavior.
package editorcompat

import (
	"os"
	"path/filepath"
)

// EditorType represents a detected editor.
type EditorType string

const (
	EditorNone        EditorType = "none"
	EditorVSCode      EditorType = "vscode"
	EditorCursor      EditorType = "cursor"
	EditorKiro        EditorType = "kiro"
	EditorNeovim      EditorType = "neovim"
	EditorAntigravity EditorType = "antigravity"
)

// DetectEditor checks environment variables to detect the current editor.
func DetectEditor() EditorType {
	checks := []struct {
		env    string
		editor EditorType
	}{
		{"VSCODE_PID", EditorVSCode},
		{"CURSOR_PID", EditorCursor},
		{"KIRO_PID", EditorKiro},
		{"NVIM", EditorNeovim},
		{"ANTIGRAVITY_PID", EditorAntigravity},
	}
	for _, c := range checks {
		if os.Getenv(c.env) != "" {
			return c.editor
		}
	}
	return EditorNone
}

// EditorConfig holds editor-specific configuration.
type EditorConfig struct {
	Editor       EditorType
	WorkspaceDir string
	ConfigDir    string
}

// LoadEditorConfig detects the editor and loads its configuration.
func LoadEditorConfig() EditorConfig {
	editor := DetectEditor()
	cwd, _ := os.Getwd()
	configDir := filepath.Join(cwd, ".gocode")
	return EditorConfig{
		Editor:       editor,
		WorkspaceDir: cwd,
		ConfigDir:    configDir,
	}
}
