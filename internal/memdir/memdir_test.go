package memdir

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func tempStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	return NewStoreWithDirs(
		filepath.Join(dir, "project"),
		filepath.Join(dir, "user"),
		filepath.Join(dir, "team"),
	)
}

func TestSaveLoadRoundTrip(t *testing.T) {
	s := tempStore(t)
	now := time.Now().Truncate(time.Second)

	entry := MemoryEntry{
		ID:         "test-1",
		Content:    "always use gofmt",
		Scope:      ScopeProject,
		CreatedAt:  now,
		LastAccess: now,
		Relevance:  0.9,
		Tags:       []string{"go", "style"},
	}

	if err := s.Save(entry); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := s.Load(ScopeProject)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(loaded))
	}
	got := loaded[0]
	if got.ID != entry.ID {
		t.Errorf("ID: got %q, want %q", got.ID, entry.ID)
	}
	if got.Content != entry.Content {
		t.Errorf("Content: got %q, want %q", got.Content, entry.Content)
	}
	if got.Scope != entry.Scope {
		t.Errorf("Scope: got %q, want %q", got.Scope, entry.Scope)
	}
	if math.Abs(got.Relevance-entry.Relevance) > 1e-9 {
		t.Errorf("Relevance: got %f, want %f", got.Relevance, entry.Relevance)
	}
}

func TestSaveLoadMultipleScopes(t *testing.T) {
	s := tempStore(t)
	now := time.Now().Truncate(time.Second)

	for _, scope := range []Scope{ScopeProject, ScopeUser, ScopeTeam} {
		entry := MemoryEntry{
			ID:         "entry-" + string(scope),
			Content:    "content for " + string(scope),
			Scope:      scope,
			CreatedAt:  now,
			LastAccess: now,
			Relevance:  0.5,
		}
		if err := s.Save(entry); err != nil {
			t.Fatalf("Save(%s): %v", scope, err)
		}
	}

	for _, scope := range []Scope{ScopeProject, ScopeUser, ScopeTeam} {
		loaded, err := s.Load(scope)
		if err != nil {
			t.Fatalf("Load(%s): %v", scope, err)
		}
		if len(loaded) != 1 {
			t.Errorf("Load(%s): expected 1, got %d", scope, len(loaded))
		}
	}
}

func TestLoadEmptyScope(t *testing.T) {
	s := tempStore(t)
	loaded, err := s.Load(ScopeProject)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded) != 0 {
		t.Errorf("expected 0 entries, got %d", len(loaded))
	}
}

func TestFindRelevantOrdering(t *testing.T) {
	s := tempStore(t)
	now := time.Now().Truncate(time.Second)

	entries := []MemoryEntry{
		{ID: "low", Content: "low relevance", Scope: ScopeProject, CreatedAt: now, LastAccess: now, Relevance: 0.2},
		{ID: "high", Content: "high relevance", Scope: ScopeProject, CreatedAt: now, LastAccess: now, Relevance: 0.9},
		{ID: "mid", Content: "mid relevance", Scope: ScopeProject, CreatedAt: now, LastAccess: now, Relevance: 0.5},
	}
	for _, e := range entries {
		if err := s.Save(e); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	results, err := s.FindRelevant("", 10)
	if err != nil {
		t.Fatalf("FindRelevant: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Should be sorted descending by relevance
	for i := 1; i < len(results); i++ {
		if results[i].Relevance > results[i-1].Relevance {
			t.Errorf("results not sorted descending: [%d].Relevance=%f > [%d].Relevance=%f",
				i, results[i].Relevance, i-1, results[i-1].Relevance)
		}
	}
}

func TestFindRelevantLimit(t *testing.T) {
	s := tempStore(t)
	now := time.Now().Truncate(time.Second)

	for i := 0; i < 5; i++ {
		entry := MemoryEntry{
			ID:         fmt.Sprintf("mem-%d", i),
			Content:    fmt.Sprintf("memory %d", i),
			Scope:      ScopeProject,
			CreatedAt:  now,
			LastAccess: now,
			Relevance:  float64(i) / 10.0,
		}
		if err := s.Save(entry); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	results, err := s.FindRelevant("", 2)
	if err != nil {
		t.Fatalf("FindRelevant: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestAgeDecay(t *testing.T) {
	s := tempStore(t)
	// Create an entry with LastAccess 10 days ago
	pastAccess := time.Now().Add(-10 * 24 * time.Hour)
	entry := MemoryEntry{
		ID:         "aging-test",
		Content:    "this should decay",
		Scope:      ScopeProject,
		CreatedAt:  pastAccess,
		LastAccess: pastAccess,
		Relevance:  1.0,
	}
	if err := s.Save(entry); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := s.Age(); err != nil {
		t.Fatalf("Age: %v", err)
	}

	loaded, err := s.Load(ScopeProject)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(loaded))
	}

	aged := loaded[0]
	// After 10 days with lambda=0.01: relevance' = 1.0 * e^(-0.01*10) ≈ 0.9048
	expected := math.Exp(-Lambda * 10.0)
	if math.Abs(aged.Relevance-expected) > 0.01 {
		t.Errorf("aged relevance: got %f, want ~%f", aged.Relevance, expected)
	}
	if aged.Relevance >= 1.0 {
		t.Errorf("aged relevance should be less than original: got %f", aged.Relevance)
	}
	if aged.Relevance <= 0 {
		t.Errorf("aged relevance should be positive: got %f", aged.Relevance)
	}
}

func TestExtractMemories(t *testing.T) {
	s := tempStore(t)
	text := `You should always use gofmt.
This is a normal sentence.
Remember that the config is in /etc/gocode/config.yaml.
The project is at github.com/AlleyBo55/gocode.
Never commit secrets to git.`

	memories, err := s.ExtractMemories(text)
	if err != nil {
		t.Fatalf("ExtractMemories: %v", err)
	}

	if len(memories) < 3 {
		t.Errorf("expected at least 3 extracted memories, got %d", len(memories))
	}

	// Check that keywords triggered extraction
	found := map[string]bool{}
	for _, m := range memories {
		found[m.Content] = true
	}
	if !found["You should always use gofmt"] {
		t.Error("expected 'always' keyword to trigger extraction")
	}
	if !found["Never commit secrets to git"] {
		t.Error("expected 'never' keyword to trigger extraction")
	}
}

func TestLoadCorruptFile(t *testing.T) {
	s := tempStore(t)
	dir := s.dirForScope(ScopeProject)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Write a corrupt JSON file
	if err := os.WriteFile(filepath.Join(dir, "corrupt.json"), []byte("{invalid json"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Write a valid file too
	now := time.Now()
	entry := MemoryEntry{ID: "valid", Content: "valid entry", Scope: ScopeProject, CreatedAt: now, LastAccess: now, Relevance: 0.5}
	if err := s.Save(entry); err != nil {
		t.Fatal(err)
	}

	loaded, err := s.Load(ScopeProject)
	if err != nil {
		t.Fatalf("Load should not error on corrupt files: %v", err)
	}
	// Should skip corrupt file and load the valid one
	if len(loaded) != 1 {
		t.Errorf("expected 1 valid entry, got %d", len(loaded))
	}
}
