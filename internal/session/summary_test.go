package session

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestListSessionsSummaryKeepsMultibyteValid(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)
	content := strings.Repeat("中文输出", 40)
	if _, err := store.Save(StoredSession{
		SessionID:  "sess-cjk",
		WorkingDir: "/proj/cjk",
		Messages:   []Message{{Role: "user", Content: content}},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	metas, err := store.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(metas) != 1 {
		t.Fatalf("ListSessions returned %d sessions, want 1", len(metas))
	}
	summary := metas[0].Summary
	if !utf8.ValidString(summary) {
		t.Fatalf("summary is invalid UTF-8: %q", summary)
	}
	if strings.ContainsRune(summary, utf8.RuneError) {
		t.Fatalf("summary contains U+FFFD: %q", summary)
	}
	want := strings.Repeat("中文输出", 19) + "中" + "..."
	if summary != want {
		t.Fatalf("summary = %q, want %q", summary, want)
	}
}
