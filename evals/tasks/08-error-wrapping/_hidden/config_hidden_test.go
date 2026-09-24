package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigMissingFileIsNotExist(t *testing.T) {
	_, err := LoadConfig(filepath.Join(t.TempDir(), "nope.conf"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("errors.Is(err, os.ErrNotExist) = false; err = %v", err)
	}
	if !strings.HasPrefix(err.Error(), "load config:") {
		t.Errorf("message lost its prefix: %q", err)
	}
}

func TestLoadConfigEmptyFileIsErrEmpty(t *testing.T) {
	p := filepath.Join(t.TempDir(), "empty.conf")
	if err := os.WriteFile(p, []byte("# only a comment\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadConfig(p)
	if !errors.Is(err, ErrEmpty) {
		t.Fatalf("errors.Is(err, ErrEmpty) = false; err = %v", err)
	}
	if !strings.Contains(err.Error(), "load config:") {
		t.Errorf("message lost its prefix: %q", err)
	}
}

func TestLoadConfigStillParses(t *testing.T) {
	p := filepath.Join(t.TempDir(), "ok.conf")
	if err := os.WriteFile(p, []byte("a = 1\n# c\nb=two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if got["a"] != "1" || got["b"] != "two" || len(got) != 2 {
		t.Errorf("parsed = %v", got)
	}
}
