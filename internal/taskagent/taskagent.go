package taskagent

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// TaskState represents the lifecycle state of a task.
type TaskState string

const (
	TaskRunning   TaskState = "running"
	TaskCompleted TaskState = "completed"
	TaskFailed    TaskState = "failed"
	TaskStopped   TaskState = "stopped"
)

// Task represents a background agent task.
type Task struct {
	ID          string
	Description string
	State       TaskState
	Output      *OutputBuffer
	Cancel      context.CancelFunc
	Ctx         context.Context
	CreatedAt   time.Time
}

// OutputBuffer is a thread-safe buffer for task output.
type OutputBuffer struct {
	mu   sync.Mutex
	data []string
}

// Append adds a line to the buffer.
func (b *OutputBuffer) Append(line string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, line)
}

// Lines returns a copy of all buffered lines.
func (b *OutputBuffer) Lines() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, len(b.data))
	copy(out, b.data)
	return out
}

// TaskManager manages background agent tasks.
type TaskManager struct {
	mu     sync.Mutex
	tasks  map[string]*Task
	nextID int
}

// NewTaskManager creates a task manager.
func NewTaskManager() *TaskManager {
	return &TaskManager{
		tasks:  make(map[string]*Task),
		nextID: 1,
	}
}

// Create spawns a new background agent task. The provided work function
// receives the task's context and output buffer. When work returns, the
// task transitions to completed (nil error) or failed (non-nil error).
func (m *TaskManager) Create(description string, work func(ctx context.Context, out *OutputBuffer) error) (*Task, error) {
	m.mu.Lock()
	id := fmt.Sprintf("task-%d", m.nextID)
	m.nextID++

	ctx, cancel := context.WithCancel(context.Background())
	buf := &OutputBuffer{}
	t := &Task{
		ID:          id,
		Description: description,
		State:       TaskRunning,
		Output:      buf,
		Cancel:      cancel,
		Ctx:         ctx,
		CreatedAt:   time.Now(),
	}
	m.tasks[id] = t
	m.mu.Unlock()

	go func() {
		err := work(ctx, buf)
		m.mu.Lock()
		defer m.mu.Unlock()
		// Only transition if still running (Stop may have already set stopped).
		if t.State == TaskRunning {
			if err != nil {
				t.State = TaskFailed
			} else {
				t.State = TaskCompleted
			}
		}
	}()

	return t, nil
}

// Get returns a task by ID.
func (m *TaskManager) Get(id string) (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[id]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", id)
	}
	return t, nil
}

// List returns all tasks.
func (m *TaskManager) List() []*Task {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Task, 0, len(m.tasks))
	for _, t := range m.tasks {
		out = append(out, t)
	}
	return out
}

// Stop cancels a running task.
func (m *TaskManager) Stop(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[id]
	if !ok {
		return fmt.Errorf("task not found: %s", id)
	}
	if t.State != TaskRunning {
		return fmt.Errorf("task %s is not running (state: %s)", id, t.State)
	}
	t.Cancel()
	t.State = TaskStopped
	return nil
}

// Output returns buffered output for a task.
func (m *TaskManager) Output(id string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[id]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", id)
	}
	return t.Output.Lines(), nil
}

// Update modifies the task description.
func (m *TaskManager) Update(id string, description string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[id]
	if !ok {
		return fmt.Errorf("task not found: %s", id)
	}
	t.Description = description
	return nil
}
