package transcript

// TranscriptManager defines the interface for transcript operations.
type TranscriptManager interface {
	Append(entry string)
	Compact()
	Replay() []string
	Flush()
}

// TranscriptStore holds conversation transcript entries in memory.
type TranscriptStore struct {
	Entries  []string
	Flushed  bool
	KeepLast int
}

// NewTranscriptStore creates a new TranscriptStore with default KeepLast=10.
func NewTranscriptStore() *TranscriptStore {
	return &TranscriptStore{
		Entries:  []string{},
		Flushed:  false,
		KeepLast: 10,
	}
}

// Append adds an entry to the transcript and marks it as unflushed.
func (t *TranscriptStore) Append(entry string) {
	t.Entries = append(t.Entries, entry)
	t.Flushed = false
}

// Compact keeps only the last KeepLast entries if the transcript exceeds that threshold.
func (t *TranscriptStore) Compact() {
	if len(t.Entries) > t.KeepLast {
		t.Entries = t.Entries[len(t.Entries)-t.KeepLast:]
	}
}

// Replay returns a copy of all transcript entries.
func (t *TranscriptStore) Replay() []string {
	cp := make([]string, len(t.Entries))
	copy(cp, t.Entries)
	return cp
}

// Flush marks the transcript as flushed (in-memory only, matching Python behavior).
func (t *TranscriptStore) Flush() {
	t.Flushed = true
}
