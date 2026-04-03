package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ErrSessionNotFound is returned when a session file does not exist.
var ErrSessionNotFound = errors.New("session not found")

// Message represents a conversation message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// StoredSession represents a persisted session.
type StoredSession struct {
	SessionID    string    `json:"session_id"`
	WorkingDir   string    `json:"working_dir,omitempty"`
	Messages     []Message `json:"messages"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
}

// SessionMeta holds metadata for session listing.
type SessionMeta struct {
	SessionID  string    `json:"session_id"`
	WorkingDir string    `json:"working_dir"`
	ModTime    time.Time `json:"mod_time"`
	Summary    string    `json:"summary,omitempty"`
}

// SessionPersistence defines the interface for session storage operations.
type SessionPersistence interface {
	Save(session StoredSession) (string, error)
	Load(sessionID string) (StoredSession, error)
}

// SessionStore manages session persistence to disk.
type SessionStore struct {
	Dir string
}

// NewSessionStore creates a new SessionStore. If dir is empty, defaults to ".port_sessions".
func NewSessionStore(dir string) *SessionStore {
	if dir == "" {
		dir = ".port_sessions"
	}
	return &SessionStore{Dir: dir}
}

// Save writes the session as a JSON file in the session directory.
// It creates the directory if it doesn't exist and uses atomic write (temp + rename).
// Returns the file path of the saved session.
func (s *SessionStore) Save(session StoredSession) (string, error) {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return "", fmt.Errorf("creating session directory: %w", err)
	}

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshaling session %s: %w", session.SessionID, err)
	}

	target := filepath.Join(s.Dir, session.SessionID+".json")

	tmp, err := os.CreateTemp(s.Dir, "session-*.tmp")
	if err != nil {
		return "", fmt.Errorf("creating temp file for session %s: %w", session.SessionID, err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", fmt.Errorf("writing session %s: %w", session.SessionID, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return "", fmt.Errorf("closing temp file for session %s: %w", session.SessionID, err)
	}

	if err := os.Rename(tmpName, target); err != nil {
		os.Remove(tmpName)
		return "", fmt.Errorf("renaming temp file for session %s: %w", session.SessionID, err)
	}

	return target, nil
}

// Load reads and deserializes a session from disk.
// Returns ErrSessionNotFound if the session file does not exist.
func (s *SessionStore) Load(sessionID string) (StoredSession, error) {
	path := filepath.Join(s.Dir, sessionID+".json")

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return StoredSession{}, fmt.Errorf("loading session %s: %w", sessionID, ErrSessionNotFound)
		}
		return StoredSession{}, fmt.Errorf("loading session %s: %w", sessionID, err)
	}

	var session StoredSession
	if err := json.Unmarshal(data, &session); err != nil {
		return StoredSession{}, fmt.Errorf("parsing session %s: %w", sessionID, err)
	}

	return session, nil
}

// ListSessions returns metadata for all saved sessions, sorted by mod time descending.
func (s *SessionStore) ListSessions() ([]SessionMeta, error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading session directory: %w", err)
	}

	var metas []SessionMeta
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}

		info, err := e.Info()
		if err != nil {
			continue
		}

		sessionID := strings.TrimSuffix(e.Name(), ".json")
		data, err := os.ReadFile(filepath.Join(s.Dir, e.Name()))
		if err != nil {
			continue
		}

		var stored StoredSession
		if err := json.Unmarshal(data, &stored); err != nil {
			continue
		}

		summary := ""
		if len(stored.Messages) > 0 {
			first := stored.Messages[0].Content
			if len(first) > 80 {
				first = first[:80] + "..."
			}
			summary = first
		}

		metas = append(metas, SessionMeta{
			SessionID:  sessionID,
			WorkingDir: stored.WorkingDir,
			ModTime:    info.ModTime(),
			Summary:    summary,
		})
	}

	sort.Slice(metas, func(i, j int) bool {
		return metas[i].ModTime.After(metas[j].ModTime)
	})

	return metas, nil
}

// FindMostRecent returns the most recently modified session for the given working directory.
func (s *SessionStore) FindMostRecent(cwd string) (StoredSession, error) {
	metas, err := s.ListSessions()
	if err != nil {
		return StoredSession{}, err
	}

	for _, m := range metas {
		if m.WorkingDir == cwd {
			return s.Load(m.SessionID)
		}
	}

	return StoredSession{}, fmt.Errorf("no session found for directory %s: %w", cwd, ErrSessionNotFound)
}
