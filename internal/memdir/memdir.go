package memdir

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Scope defines memory visibility.
type Scope string

const (
	ScopeProject Scope = "project"
	ScopeUser    Scope = "user"
	ScopeTeam    Scope = "team"
)

// MemoryEntry is a single memory with metadata.
type MemoryEntry struct {
	ID         string    `json:"id"`
	Content    string    `json:"content"`
	Scope      Scope     `json:"scope"`
	CreatedAt  time.Time `json:"created_at"`
	LastAccess time.Time `json:"last_access"`
	Relevance  float64   `json:"relevance"`
	Tags       []string  `json:"tags,omitempty"`
}

// Store manages the memory directory with aging and relevance.
type Store struct {
	projectDir string // .gocode/memory/
	userDir    string // ~/.gocode/memory/
	teamDir    string // .gocode/team-memory/
}

// NewStore creates a memory directory store with default directories.
func NewStore() *Store {
	home, _ := os.UserHomeDir()
	return &Store{
		projectDir: filepath.Join(".gocode", "memory"),
		userDir:    filepath.Join(home, ".gocode", "memory"),
		teamDir:    filepath.Join(".gocode", "team-memory"),
	}
}

// NewStoreWithDirs creates a Store with explicit directories (useful for testing).
func NewStoreWithDirs(projectDir, userDir, teamDir string) *Store {
	return &Store{
		projectDir: projectDir,
		userDir:    userDir,
		teamDir:    teamDir,
	}
}

// dirForScope returns the filesystem directory for a given scope.
func (s *Store) dirForScope(scope Scope) string {
	switch scope {
	case ScopeProject:
		return s.projectDir
	case ScopeUser:
		return s.userDir
	case ScopeTeam:
		return s.teamDir
	default:
		return s.projectDir
	}
}

// Save persists a memory entry as a JSON file named by ID.
func (s *Store) Save(entry MemoryEntry) error {
	dir := s.dirForScope(entry.Scope)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating memory dir: %w", err)
	}
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling memory entry: %w", err)
	}
	path := filepath.Join(dir, entry.ID+".json")
	return os.WriteFile(path, data, 0o644)
}

// Load reads all memories from a scope directory.
func (s *Store) Load(scope Scope) ([]MemoryEntry, error) {
	dir := s.dirForScope(scope)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading memory dir: %w", err)
	}

	var memories []MemoryEntry
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue // skip unreadable files
		}
		var mem MemoryEntry
		if err := json.Unmarshal(data, &mem); err != nil {
			continue // skip corrupt files
		}
		memories = append(memories, mem)
	}
	return memories, nil
}

// keyword patterns for memory extraction
var memoryKeywords = []string{
	"always", "never", "important", "remember", "note that",
}

// filePathPattern matches common file paths.
var filePathPattern = regexp.MustCompile(`(?:\./|/)?[\w\-]+(?:/[\w\-\.]+)+`)

// projectNamePattern matches Go module-style project names.
var projectNamePattern = regexp.MustCompile(`(?:github\.com|gitlab\.com|bitbucket\.org)/[\w\-]+/[\w\-]+`)

// ExtractMemories analyzes conversation text and extracts key facts as MemoryEntry values.
// It uses basic keyword extraction: sentences containing "always", "never", "important",
// "remember", "note that", project names, or file paths.
func (s *Store) ExtractMemories(conversationText string) ([]MemoryEntry, error) {
	sentences := splitSentences(conversationText)
	now := time.Now()
	var memories []MemoryEntry
	seen := make(map[string]bool)

	for _, sentence := range sentences {
		sentence = strings.TrimSpace(sentence)
		if sentence == "" {
			continue
		}
		lower := strings.ToLower(sentence)

		matched := false
		for _, kw := range memoryKeywords {
			if strings.Contains(lower, kw) {
				matched = true
				break
			}
		}
		if !matched {
			matched = filePathPattern.MatchString(sentence) || projectNamePattern.MatchString(sentence)
		}

		if matched && !seen[sentence] {
			seen[sentence] = true
			memories = append(memories, MemoryEntry{
				ID:         fmt.Sprintf("mem-%d", len(memories)+1),
				Content:    sentence,
				Scope:      ScopeProject,
				CreatedAt:  now,
				LastAccess: now,
				Relevance:  1.0,
			})
		}
	}
	return memories, nil
}

// splitSentences splits text into sentences on period, newline, or exclamation/question marks.
func splitSentences(text string) []string {
	var sentences []string
	for _, line := range strings.Split(text, "\n") {
		// Split on sentence-ending punctuation
		parts := regexp.MustCompile(`[.!?]+`).Split(line, -1)
		sentences = append(sentences, parts...)
	}
	return sentences
}

// FindRelevant returns memories sorted by relevance score descending, limited to limit results.
// It loads memories from all scopes and optionally filters by context keywords.
func (s *Store) FindRelevant(context string, limit int) ([]MemoryEntry, error) {
	var all []MemoryEntry
	for _, scope := range []Scope{ScopeProject, ScopeUser, ScopeTeam} {
		entries, err := s.Load(scope)
		if err != nil {
			continue
		}
		all = append(all, entries...)
	}

	// Score boost for context keyword matches
	contextWords := strings.Fields(strings.ToLower(context))
	for i := range all {
		bonus := 0.0
		lower := strings.ToLower(all[i].Content)
		for _, w := range contextWords {
			if len(w) > 2 && strings.Contains(lower, w) {
				bonus += 0.1
			}
		}
		all[i].Relevance += bonus
		// Cap at 1.0
		if all[i].Relevance > 1.0 {
			all[i].Relevance = 1.0
		}
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].Relevance > all[j].Relevance
	})

	if limit > 0 && limit < len(all) {
		all = all[:limit]
	}
	return all, nil
}

// Lambda is the decay constant for memory aging.
const Lambda = 0.01

// Age applies time-based exponential decay to all memory relevance scores.
// Formula: relevance' = relevance * e^(-lambda * days_since_last_access)
func (s *Store) Age() error {
	now := time.Now()
	for _, scope := range []Scope{ScopeProject, ScopeUser, ScopeTeam} {
		entries, err := s.Load(scope)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			days := now.Sub(entry.LastAccess).Hours() / 24.0
			if days <= 0 {
				continue
			}
			entry.Relevance = entry.Relevance * math.Exp(-Lambda*days)
			if entry.Relevance < 0 {
				entry.Relevance = 0
			}
			if err := s.Save(entry); err != nil {
				return fmt.Errorf("saving aged memory %s: %w", entry.ID, err)
			}
		}
	}
	return nil
}

// Sync is a stub that returns nil. Team sync via git/API is future work.
func (s *Store) Sync() error {
	return nil
}
