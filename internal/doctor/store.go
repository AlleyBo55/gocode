package doctor

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Store caches one Report per model in a JSON file, by default
// .gocode/capabilities.json in the working directory.
type Store struct {
	path string
}

// NewStore returns a store at path, or the default location when path is "".
func NewStore(path string) *Store {
	if path == "" {
		path = filepath.Join(".gocode", "capabilities.json")
	}
	return &Store{path: path}
}

// Path is where the store reads and writes.
func (s *Store) Path() string { return s.path }

// Load reads every cached report. A missing file is an empty cache, not an
// error; a corrupt file is an error, so a bad write cannot masquerade as
// "never probed".
func (s *Store) Load() (map[string]Report, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]Report{}, nil
		}
		return nil, err
	}
	var reports map[string]Report
	if err := json.Unmarshal(data, &reports); err != nil {
		return nil, err
	}
	if reports == nil {
		reports = map[string]Report{}
	}
	return reports, nil
}

// Get returns the cached report for a model.
func (s *Store) Get(model string) (Report, bool) {
	reports, err := s.Load()
	if err != nil {
		return Report{}, false
	}
	r, ok := reports[model]
	return r, ok
}

// Put stores a report under its model id, creating the parent directory.
func (s *Store) Put(r Report) error {
	reports, err := s.Load()
	if err != nil {
		// Do not let one corrupt file block every future probe; start over.
		reports = map[string]Report{}
	}
	reports[r.Model] = r
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(reports, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}
