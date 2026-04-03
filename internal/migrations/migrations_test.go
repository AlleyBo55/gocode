package migrations

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunPendingMigrations(t *testing.T) {
	dir := t.TempDir()
	ran := make([]int, 0)

	runner := NewRunnerWithMigrations(dir, []Migration{
		{Version: 1, Name: "first", Migrate: func(_ string) error { ran = append(ran, 1); return nil }},
		{Version: 2, Name: "second", Migrate: func(_ string) error { ran = append(ran, 2); return nil }},
	})

	if err := runner.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(ran) != 2 || ran[0] != 1 || ran[1] != 2 {
		t.Errorf("expected [1, 2], got %v", ran)
	}
}

func TestSkipCompletedMigrations(t *testing.T) {
	dir := t.TempDir()
	ran := make([]int, 0)

	runner := NewRunnerWithMigrations(dir, []Migration{
		{Version: 1, Name: "first", Migrate: func(_ string) error { ran = append(ran, 1); return nil }},
		{Version: 2, Name: "second", Migrate: func(_ string) error { ran = append(ran, 2); return nil }},
	})

	// Run once.
	if err := runner.Run(); err != nil {
		t.Fatalf("first Run: %v", err)
	}

	// Run again — should skip both.
	ran = nil
	runner2 := NewRunnerWithMigrations(dir, []Migration{
		{Version: 1, Name: "first", Migrate: func(_ string) error { ran = append(ran, 1); return nil }},
		{Version: 2, Name: "second", Migrate: func(_ string) error { ran = append(ran, 2); return nil }},
	})
	if err := runner2.Run(); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if len(ran) != 0 {
		t.Errorf("expected no migrations to run, got %v", ran)
	}
}

func TestIdempotentRun(t *testing.T) {
	dir := t.TempDir()
	count := 0

	migrations := []Migration{
		{Version: 1, Name: "counter", Migrate: func(_ string) error { count++; return nil }},
	}

	// Run three times.
	for i := 0; i < 3; i++ {
		runner := NewRunnerWithMigrations(dir, migrations)
		if err := runner.Run(); err != nil {
			t.Fatalf("Run %d: %v", i, err)
		}
	}
	if count != 1 {
		t.Errorf("migration should run exactly once, ran %d times", count)
	}
}

func TestPending(t *testing.T) {
	dir := t.TempDir()
	runner := NewRunnerWithMigrations(dir, []Migration{
		{Version: 1, Name: "a", Migrate: func(_ string) error { return nil }},
		{Version: 2, Name: "b", Migrate: func(_ string) error { return nil }},
		{Version: 3, Name: "c", Migrate: func(_ string) error { return nil }},
	})

	pending, err := runner.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 3 {
		t.Errorf("expected 3 pending, got %d", len(pending))
	}

	// Run first two.
	runner2 := NewRunnerWithMigrations(dir, []Migration{
		{Version: 1, Name: "a", Migrate: func(_ string) error { return nil }},
		{Version: 2, Name: "b", Migrate: func(_ string) error { return nil }},
	})
	if err := runner2.Run(); err != nil {
		t.Fatal(err)
	}

	// Check pending with all three registered.
	runner3 := NewRunnerWithMigrations(dir, []Migration{
		{Version: 1, Name: "a", Migrate: func(_ string) error { return nil }},
		{Version: 2, Name: "b", Migrate: func(_ string) error { return nil }},
		{Version: 3, Name: "c", Migrate: func(_ string) error { return nil }},
	})
	pending, err = runner3.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Version != 3 {
		t.Errorf("expected only version 3 pending, got %v", pending)
	}
}

func TestVersionOrder(t *testing.T) {
	dir := t.TempDir()
	ran := make([]int, 0)

	// Register out of order.
	runner := NewRunnerWithMigrations(dir, []Migration{
		{Version: 3, Name: "third", Migrate: func(_ string) error { ran = append(ran, 3); return nil }},
		{Version: 1, Name: "first", Migrate: func(_ string) error { ran = append(ran, 1); return nil }},
		{Version: 2, Name: "second", Migrate: func(_ string) error { ran = append(ran, 2); return nil }},
	})

	if err := runner.Run(); err != nil {
		t.Fatal(err)
	}
	if len(ran) != 3 || ran[0] != 1 || ran[1] != 2 || ran[2] != 3 {
		t.Errorf("expected [1, 2, 3], got %v", ran)
	}
}

func TestDefaultMigration_RenameConfigPaths(t *testing.T) {
	dir := t.TempDir()
	// Create legacy config file.
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"key":"val"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := migrateRenameConfigPaths(dir); err != nil {
		t.Fatalf("migrateRenameConfigPaths: %v", err)
	}

	// Old file should be gone, new file should exist.
	if _, err := os.Stat(filepath.Join(dir, "config.json")); !os.IsNotExist(err) {
		t.Error("old config.json should be removed")
	}
	data, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatalf("settings.json should exist: %v", err)
	}
	if string(data) != `{"key":"val"}` {
		t.Errorf("unexpected content: %s", data)
	}
}

func TestDefaultMigration_UpdateModelAliases(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"model":"gpt4"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := migrateUpdateModelAliases(dir); err != nil {
		t.Fatalf("migrateUpdateModelAliases: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == `{"model":"gpt4"}` {
		t.Error("model alias should have been updated")
	}
}
