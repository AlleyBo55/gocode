package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWorkingDirRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)

	original := StoredSession{
		SessionID:    "test-wd-roundtrip",
		WorkingDir:   "/home/user/project",
		Messages:     []Message{{Role: "user", Content: "hello"}},
		InputTokens:  100,
		OutputTokens: 50,
	}

	_, err := store.Save(original)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := store.Load("test-wd-roundtrip")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.WorkingDir != original.WorkingDir {
		t.Errorf("WorkingDir = %q, want %q", loaded.WorkingDir, original.WorkingDir)
	}
	if loaded.SessionID != original.SessionID {
		t.Errorf("SessionID = %q, want %q", loaded.SessionID, original.SessionID)
	}
	if len(loaded.Messages) != 1 || loaded.Messages[0].Content != "hello" {
		t.Errorf("Messages mismatch: got %v", loaded.Messages)
	}
	if loaded.InputTokens != 100 || loaded.OutputTokens != 50 {
		t.Errorf("Token counts mismatch: input=%d output=%d", loaded.InputTokens, loaded.OutputTokens)
	}
}

func TestListSessionsMultiple(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)

	sessions := []StoredSession{
		{SessionID: "sess-a", WorkingDir: "/proj/a", Messages: []Message{{Role: "user", Content: "first session"}}},
		{SessionID: "sess-b", WorkingDir: "/proj/b", Messages: []Message{{Role: "user", Content: "second session"}}},
		{SessionID: "sess-c", WorkingDir: "/proj/a", Messages: []Message{{Role: "user", Content: "third session"}}},
	}

	for i, s := range sessions {
		if _, err := store.Save(s); err != nil {
			t.Fatalf("Save %d: %v", i, err)
		}
		// Stagger mod times so ordering is deterministic
		path := filepath.Join(dir, s.SessionID+".json")
		modTime := time.Now().Add(time.Duration(i) * time.Second)
		if err := os.Chtimes(path, modTime, modTime); err != nil {
			t.Fatalf("Chtimes: %v", err)
		}
	}

	metas, err := store.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}

	if len(metas) != 3 {
		t.Fatalf("ListSessions returned %d sessions, want 3", len(metas))
	}

	// Should be sorted by mod time descending (sess-c newest)
	if metas[0].SessionID != "sess-c" {
		t.Errorf("first session = %q, want sess-c (most recent)", metas[0].SessionID)
	}
	if metas[1].SessionID != "sess-b" {
		t.Errorf("second session = %q, want sess-b", metas[1].SessionID)
	}
	if metas[2].SessionID != "sess-a" {
		t.Errorf("third session = %q, want sess-a (oldest)", metas[2].SessionID)
	}

	// Verify WorkingDir is populated
	for _, m := range metas {
		if m.WorkingDir == "" {
			t.Errorf("session %s has empty WorkingDir", m.SessionID)
		}
	}
}

func TestListSessionsEmptyDir(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)

	metas, err := store.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(metas) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(metas))
	}
}

func TestListSessionsNonExistentDir(t *testing.T) {
	store := NewSessionStore(filepath.Join(t.TempDir(), "nonexistent"))

	metas, err := store.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if metas != nil {
		t.Errorf("expected nil, got %v", metas)
	}
}

func TestFindMostRecentMatching(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)

	// Save two sessions for /proj/a with different mod times
	s1 := StoredSession{SessionID: "old-a", WorkingDir: "/proj/a", Messages: []Message{{Role: "user", Content: "old"}}}
	s2 := StoredSession{SessionID: "new-a", WorkingDir: "/proj/a", Messages: []Message{{Role: "user", Content: "new"}}}
	s3 := StoredSession{SessionID: "other-b", WorkingDir: "/proj/b", Messages: []Message{{Role: "user", Content: "other"}}}

	for _, s := range []StoredSession{s1, s2, s3} {
		if _, err := store.Save(s); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	// Make "new-a" the most recent for /proj/a
	oldTime := time.Now().Add(-1 * time.Hour)
	newTime := time.Now()
	os.Chtimes(filepath.Join(dir, "old-a.json"), oldTime, oldTime)
	os.Chtimes(filepath.Join(dir, "new-a.json"), newTime, newTime)
	os.Chtimes(filepath.Join(dir, "other-b.json"), newTime, newTime)

	found, err := store.FindMostRecent("/proj/a")
	if err != nil {
		t.Fatalf("FindMostRecent: %v", err)
	}
	if found.SessionID != "new-a" {
		t.Errorf("FindMostRecent returned %q, want new-a", found.SessionID)
	}
	if found.WorkingDir != "/proj/a" {
		t.Errorf("WorkingDir = %q, want /proj/a", found.WorkingDir)
	}
}

func TestFindMostRecentNoMatch(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)

	s := StoredSession{SessionID: "only-b", WorkingDir: "/proj/b", Messages: []Message{{Role: "user", Content: "b"}}}
	if _, err := store.Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	_, err := store.FindMostRecent("/proj/a")
	if err == nil {
		t.Fatal("expected error for non-matching directory, got nil")
	}
}
