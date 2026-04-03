package taskagent

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestCreateAndGet(t *testing.T) {
	mgr := NewTaskManager()
	task, err := mgr.Create("test task", func(ctx context.Context, out *OutputBuffer) error {
		<-ctx.Done()
		return nil
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if task.ID != "task-1" {
		t.Errorf("expected id task-1, got %s", task.ID)
	}
	if task.Description != "test task" {
		t.Errorf("expected description 'test task', got %q", task.Description)
	}
	if task.State != TaskRunning {
		t.Errorf("expected state running, got %s", task.State)
	}

	got, err := mgr.Get("task-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != task.ID {
		t.Errorf("Get returned wrong task")
	}

	// Cleanup
	_ = mgr.Stop("task-1")
}

func TestList(t *testing.T) {
	mgr := NewTaskManager()
	for i := 0; i < 3; i++ {
		_, err := mgr.Create(fmt.Sprintf("task %d", i), func(ctx context.Context, out *OutputBuffer) error {
			<-ctx.Done()
			return nil
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
	}
	tasks := mgr.List()
	if len(tasks) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(tasks))
	}

	// Cleanup
	for _, task := range tasks {
		_ = mgr.Stop(task.ID)
	}
}

func TestStopCancelsContext(t *testing.T) {
	mgr := NewTaskManager()
	ctxCh := make(chan context.Context, 1)
	_, err := mgr.Create("stoppable", func(ctx context.Context, out *OutputBuffer) error {
		ctxCh <- ctx
		<-ctx.Done()
		return ctx.Err()
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Wait for goroutine to start and send its context.
	var taskCtx context.Context
	select {
	case taskCtx = <-ctxCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for task goroutine to start")
	}

	if err := mgr.Stop("task-1"); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Context should be cancelled.
	select {
	case <-taskCtx.Done():
		// expected
	case <-time.After(2 * time.Second):
		t.Fatal("context was not cancelled after Stop")
	}

	task, _ := mgr.Get("task-1")
	if task.State != TaskStopped {
		t.Errorf("expected state stopped, got %s", task.State)
	}
}

func TestOutputBuffering(t *testing.T) {
	mgr := NewTaskManager()
	done := make(chan struct{})
	_, err := mgr.Create("output task", func(ctx context.Context, out *OutputBuffer) error {
		for i := 0; i < 5; i++ {
			out.Append(fmt.Sprintf("line %d", i))
		}
		close(done)
		<-ctx.Done()
		return nil
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for output")
	}

	lines, err := mgr.Output("task-1")
	if err != nil {
		t.Fatalf("Output: %v", err)
	}
	if len(lines) != 5 {
		t.Fatalf("expected 5 lines, got %d", len(lines))
	}
	for i, line := range lines {
		expected := fmt.Sprintf("line %d", i)
		if line != expected {
			t.Errorf("line %d: expected %q, got %q", i, expected, line)
		}
	}

	// Cleanup
	_ = mgr.Stop("task-1")
}

func TestConcurrentCreateAndStop(t *testing.T) {
	mgr := NewTaskManager()
	var wg sync.WaitGroup
	n := 20

	// Concurrently create tasks.
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := mgr.Create("concurrent", func(ctx context.Context, out *OutputBuffer) error {
				<-ctx.Done()
				return nil
			})
			if err != nil {
				t.Errorf("Create: %v", err)
			}
		}()
	}
	wg.Wait()

	tasks := mgr.List()
	if len(tasks) != n {
		t.Fatalf("expected %d tasks, got %d", n, len(tasks))
	}

	// Concurrently stop all tasks.
	for _, task := range tasks {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			_ = mgr.Stop(id)
		}(task.ID)
	}
	wg.Wait()

	for _, task := range mgr.List() {
		if task.State != TaskStopped {
			t.Errorf("task %s: expected stopped, got %s", task.ID, task.State)
		}
	}
}

func TestDoubleStop(t *testing.T) {
	mgr := NewTaskManager()
	_, err := mgr.Create("double stop", func(ctx context.Context, out *OutputBuffer) error {
		<-ctx.Done()
		return nil
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := mgr.Stop("task-1"); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	err = mgr.Stop("task-1")
	if err == nil {
		t.Fatal("expected error on double stop")
	}
}

func TestUnknownTaskID(t *testing.T) {
	mgr := NewTaskManager()

	_, err := mgr.Get("nonexistent")
	if err == nil {
		t.Error("Get: expected error for unknown ID")
	}

	err = mgr.Stop("nonexistent")
	if err == nil {
		t.Error("Stop: expected error for unknown ID")
	}

	_, err = mgr.Output("nonexistent")
	if err == nil {
		t.Error("Output: expected error for unknown ID")
	}

	err = mgr.Update("nonexistent", "desc")
	if err == nil {
		t.Error("Update: expected error for unknown ID")
	}
}

func TestTaskCompletesSuccessfully(t *testing.T) {
	mgr := NewTaskManager()
	done := make(chan struct{})
	_, err := mgr.Create("completer", func(ctx context.Context, out *OutputBuffer) error {
		out.Append("done")
		close(done)
		return nil
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	<-done
	// Give goroutine time to update state.
	time.Sleep(50 * time.Millisecond)

	task, _ := mgr.Get("task-1")
	if task.State != TaskCompleted {
		t.Errorf("expected completed, got %s", task.State)
	}

	// Output should still be available after completion.
	lines, _ := mgr.Output("task-1")
	if len(lines) != 1 || lines[0] != "done" {
		t.Errorf("unexpected output: %v", lines)
	}
}

func TestTaskFails(t *testing.T) {
	mgr := NewTaskManager()
	done := make(chan struct{})
	_, err := mgr.Create("failer", func(ctx context.Context, out *OutputBuffer) error {
		close(done)
		return fmt.Errorf("something went wrong")
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	<-done
	time.Sleep(50 * time.Millisecond)

	task, _ := mgr.Get("task-1")
	if task.State != TaskFailed {
		t.Errorf("expected failed, got %s", task.State)
	}
}

func TestSequentialIDs(t *testing.T) {
	mgr := NewTaskManager()
	for i := 1; i <= 5; i++ {
		task, err := mgr.Create("seq", func(ctx context.Context, out *OutputBuffer) error {
			<-ctx.Done()
			return nil
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		expected := fmt.Sprintf("task-%d", i)
		if task.ID != expected {
			t.Errorf("expected %s, got %s", expected, task.ID)
		}
	}
	// Cleanup
	for _, task := range mgr.List() {
		_ = mgr.Stop(task.ID)
	}
}
