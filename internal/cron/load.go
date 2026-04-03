package cron

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// persistedEntry mirrors the JSON shape written by ScheduleCronTool.persist().
type persistedEntry struct {
	ID          string `json:"id"`
	Expression  string `json:"expression"`
	Description string `json:"description"`
	Recurring   bool   `json:"recurring"`
	CreatedAt   string `json:"created_at"`
}

// LoadSchedules reads persisted cron entries from dataDir/cron.json and
// re-creates them in the scheduler. Returns the number of loaded schedules.
func (s *Scheduler) LoadSchedules(dataDir string) (int, error) {
	path := filepath.Join(dataDir, "cron.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("reading cron.json: %w", err)
	}

	var entries []persistedEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return 0, fmt.Errorf("parsing cron.json: %w", err)
	}

	loaded := 0
	for _, e := range entries {
		fields, err := Parse(e.Expression)
		if err != nil {
			continue
		}

		createdAt := time.Now()
		if e.CreatedAt != "" {
			if t, parseErr := time.Parse(time.RFC3339, e.CreatedAt); parseErr == nil {
				createdAt = t
			}
		}

		s.mu.Lock()
		ctx, cancel := context.WithCancel(context.Background())
		task := &Task{
			ID:        e.ID,
			Cron:      e.Expression,
			Prompt:    e.Description,
			Recurring: e.Recurring,
			Fields:    fields,
			CreatedAt: createdAt,
			NextRun:   NextRun(fields, time.Now()),
			cancel:    cancel,
		}
		s.tasks[task.ID] = task
		if n := extractIDNum(e.ID); n >= s.nextID {
			s.nextID = n + 1
		}
		s.mu.Unlock()

		go s.runTimer(ctx, task)
		loaded++
	}
	return loaded, nil
}

// extractIDNum extracts the numeric suffix from an ID like "cron-3".
func extractIDNum(id string) int {
	if len(id) > 5 && id[:5] == "cron-" {
		n := 0
		for _, c := range id[5:] {
			if c >= '0' && c <= '9' {
				n = n*10 + int(c-'0')
			} else {
				return 0
			}
		}
		return n
	}
	return 0
}
