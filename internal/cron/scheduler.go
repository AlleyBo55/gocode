package cron

import (
"context"
"fmt"
"sync"
"time"
)

// Task represents a scheduled cron task.
type Task struct {
	ID        string
	Cron      string
	Prompt    string
	Recurring bool
	Fields    *Fields
	CreatedAt time.Time
	LastRun   time.Time
	NextRun   time.Time
	cancel    context.CancelFunc
}

// Scheduler manages recurring cron tasks with precise timer-based scheduling.
type Scheduler struct {
	mu     sync.Mutex
	tasks  map[string]*Task
	nextID int
	onFire func(task *Task)
}

// NewScheduler creates a scheduler. onFire is called when a task fires.
func NewScheduler(onFire func(task *Task)) *Scheduler {
	return &Scheduler{tasks: make(map[string]*Task), nextID: 1, onFire: onFire}
}

// Create schedules a new cron task. Returns the task ID.
func (s *Scheduler) Create(cronExpr, prompt string, recurring bool) (string, error) {
	fields, err := Parse(cronExpr)
	if err != nil {
		return "", fmt.Errorf("invalid cron: %w", err)
	}
	s.mu.Lock()
	id := fmt.Sprintf("cron-%d", s.nextID)
	s.nextID++
	ctx, cancel := context.WithCancel(context.Background())
	now := time.Now()
	task := &Task{
		ID: id, Cron: cronExpr, Prompt: prompt, Recurring: recurring,
		Fields: fields, CreatedAt: now, NextRun: NextRun(fields, now), cancel: cancel,
	}
	s.tasks[id] = task
	s.mu.Unlock()
	go s.runTimer(ctx, task)
	return id, nil
}

// Delete cancels and removes a task.
func (s *Scheduler) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task not found: %s", id)
	}
	t.cancel()
	delete(s.tasks, id)
	return nil
}

// List returns all tasks.
func (s *Scheduler) List() []*Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		out = append(out, t)
	}
	return out
}

// StopAll cancels everything.
func (s *Scheduler) StopAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.tasks {
		t.cancel()
	}
	s.tasks = make(map[string]*Task)
}

func (s *Scheduler) runTimer(ctx context.Context, task *Task) {
	for {
		s.mu.Lock()
		next := task.NextRun
		s.mu.Unlock()

		delay := time.Until(next)
		if delay < 0 {
			delay = 0
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}

		if ctx.Err() != nil {
			return
		}

		s.mu.Lock()
		task.LastRun = time.Now()
		s.mu.Unlock()

		if s.onFire != nil {
			go s.onFire(task)
		}

		if !task.Recurring {
			s.mu.Lock()
			delete(s.tasks, task.ID)
			s.mu.Unlock()
			return
		}

		s.mu.Lock()
		task.NextRun = NextRun(task.Fields, time.Now())
		s.mu.Unlock()
	}
}
