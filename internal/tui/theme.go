package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Theme defines customizable TUI colors.
type Theme struct {
	Name       string `json:"name"`
	Primary    string `json:"primary"`    // header, assistant, mode badge (256-color number)
	Secondary  string `json:"secondary"`  // user prompt, input
	Accent     string `json:"accent"`     // errors, highlights
	Background string `json:"background"` // status bar bg
	Text       string `json:"text"`       // main text
	Dim        string `json:"dim"`        // gray/muted text
}

// KeybindConfig defines customizable key bindings.
type KeybindConfig struct {
	Send   string `json:"send"`   // default "enter"
	Mode   string `json:"mode"`   // default "tab"
	Diff   string `json:"diff"`   // default "ctrl+d"
	Cancel string `json:"cancel"` // default "escape"
	Quit   string `json:"quit"`   // default "ctrl+c"
}

// BuiltinThemes maps theme names to their definitions.
var BuiltinThemes = map[string]Theme{
	"golang": {Name: "golang", Primary: "38", Secondary: "37", Accent: "168", Background: "234", Text: "255", Dim: "245"},
	"monokai": {Name: "monokai", Primary: "208", Secondary: "114", Accent: "203", Background: "235", Text: "255", Dim: "245"},
	"dracula": {Name: "dracula", Primary: "141", Secondary: "84", Accent: "212", Background: "236", Text: "255", Dim: "245"},
	"nord":    {Name: "nord", Primary: "110", Secondary: "109", Accent: "131", Background: "236", Text: "255", Dim: "245"},
}

// DefaultKeybinds returns the default key binding configuration.
func DefaultKeybinds() KeybindConfig {
	return KeybindConfig{Send: "enter", Mode: "tab", Diff: "ctrl+d", Cancel: "escape", Quit: "ctrl+c"}
}

// LoadTheme loads a theme from .gocode/theme.json or returns the named built-in.
func LoadTheme(name string) Theme {
	// Try loading from config file
	cfgPath := filepath.Join(".gocode", "theme.json")
	if data, err := os.ReadFile(cfgPath); err == nil {
		var t Theme
		if err := json.Unmarshal(data, &t); err == nil {
			// If only name is set, use built-in
			if t.Primary == "" {
				if builtin, ok := BuiltinThemes[t.Name]; ok {
					return builtin
				}
			}
			// Fill missing fields from golang default
			def := BuiltinThemes["golang"]
			if t.Primary == "" {
				t.Primary = def.Primary
			}
			if t.Secondary == "" {
				t.Secondary = def.Secondary
			}
			if t.Accent == "" {
				t.Accent = def.Accent
			}
			if t.Background == "" {
				t.Background = def.Background
			}
			if t.Text == "" {
				t.Text = def.Text
			}
			if t.Dim == "" {
				t.Dim = def.Dim
			}
			return t
		}
	}
	// Use flag or default
	if name != "" {
		if t, ok := BuiltinThemes[name]; ok {
			return t
		}
	}
	return BuiltinThemes["golang"]
}

// LoadKeybinds loads key bindings from .gocode/keybinds.json or returns defaults.
func LoadKeybinds() KeybindConfig {
	kb := DefaultKeybinds()
	cfgPath := filepath.Join(".gocode", "keybinds.json")
	if data, err := os.ReadFile(cfgPath); err == nil {
		json.Unmarshal(data, &kb)
	}
	return kb
}
