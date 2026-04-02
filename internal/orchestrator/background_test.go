package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestNewBackgroundManager(t *testing.T) {
	bm := NewBackgroundManager(5)
	if bm.maxSlots != 5 {
		t.Errorf("expected maxSlots=5, got %d", bm.maxSlots)
	}
	if cap(bm.results) != 5 {
		t.Errorf("expected results channel capacity=5, got %d", cap(bm.results))
	}
}

func TestSpawn_Success(t *testing.T) {
	bm := NewBackgroundManager(5)
	err := bm.Spawn(context.Background(), "agent-1", func(ctx context.Context) AgentResult {
		return AgentResult{Output: "done"}
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := <-bm.Results()
	if result.AgentName != "agent-1" {
		t.Errorf("expected AgentName=%q, got %q", "agent-1", result.AgentName)
	}
	if result.Output != "done" {
		t.Errorf("expected Output=%q, got %q", "done", result.Output)
	}
	if result.Err != nil {
		t.Errorf("unexpected error in result: %v", result.Err)
	}
}

func TestSpawn_MaxSlotsEnforced(t *testing.T) {
	bm := NewBackgroundManager(2)

	// Use a channel to block the goroutines so they stay "running"
	block := make(chan struct{})
	defer close(block)

	for i := 0; i < 2; i++ {
		name := "agent-" + string(rune('a'+i))
		err := bm.Spawn(context.Background(), name, func(ctx context.Context) AgentResult {
			<-block
			return AgentResult{Output: "ok"}
		})
		if err != nil {
			t.Fatalf("spawn %d: unexpected error: %v", i, err)
		}
	}

	// Third spawn should fail
	err := bm.Spawn(context.Background(), "agent-c", func(ctx context.Context) AgentResult {
		return AgentResult{Output: "should not run"}
	})
	if err == nil {
		t.Fatal("expected error when max slots reached")
	}
}

func TestSpawn_DuplicateNameRejected(t *testing.T) {
	bm := NewBackgroundManager(5)
	block := make(chan struct{})
	defer close(block)

	err := bm.Spawn(context.Background(), "dup", func(ctx context.Context) AgentResult {
		<-block
		return AgentResult{Output: "first"}
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = bm.Spawn(context.Background(), "dup", func(ctx context.Context) AgentResult {
		return AgentResult{Output: "second"}
	})
	if err == nil {
		t.Fatal("expected error for duplicate agent name")
	}
}

func TestSpawn_DerivedContextCancellation(t *testing.T) {
	bm := NewBackgroundManager(5)
	parentCtx, parentCancel := context.WithCancel(context.Background())

	err := bm.Spawn(parentCtx, "cancellable", func(ctx context.Context) AgentResult {
		<-ctx.Done()
		return AgentResult{Output: "cancelled", Err: ctx.Err()}
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parentCancel()

	result := <-bm.Results()
	if result.Err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", result.Err)
	}
}

func TestCancelAll(t *testing.T) {
	bm := NewBackgroundManager(5)

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		name := "agent-" + string(rune('a'+i))
		wg.Add(1)
		err := bm.Spawn(context.Background(), name, func(ctx context.Context) AgentResult {
			wg.Done()
			<-ctx.Done()
			return AgentResult{Err: ctx.Err()}
		})
		if err != nil {
			t.Fatalf("spawn %s: unexpected error: %v", name, err)
		}
	}

	// Wait for all goroutines to start
	wg.Wait()

	bm.CancelAll()

	// Collect all results
	for i := 0; i < 3; i++ {
		select {
		case result := <-bm.Results():
			if result.Err != context.Canceled {
				t.Errorf("agent %s: expected context.Canceled, got %v", result.AgentName, result.Err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for cancelled agent results")
		}
	}
}

func TestResults_ChannelReceivesAll(t *testing.T) {
	bm := NewBackgroundManager(5)

	for i := 0; i < 3; i++ {
		name := "agent-" + string(rune('a'+i))
		output := "result-" + string(rune('a'+i))
		err := bm.Spawn(context.Background(), name, func(ctx context.Context) AgentResult {
			return AgentResult{Output: output}
		})
		if err != nil {
			t.Fatalf("spawn %s: unexpected error: %v", name, err)
		}
	}

	results := make(map[string]string)
	for i := 0; i < 3; i++ {
		select {
		case r := <-bm.Results():
			results[r.AgentName] = r.Output
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for results")
		}
	}

	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
}

func TestSpawn_ErrorResultDoesNotAffectOthers(t *testing.T) {
	bm := NewBackgroundManager(5)

	err := bm.Spawn(context.Background(), "failing", func(ctx context.Context) AgentResult {
		return AgentResult{Err: context.DeadlineExceeded}
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = bm.Spawn(context.Background(), "succeeding", func(ctx context.Context) AgentResult {
		return AgentResult{Output: "success"}
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	results := make(map[string]AgentResult)
	for i := 0; i < 2; i++ {
		select {
		case r := <-bm.Results():
			results[r.AgentName] = r
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for results")
		}
	}

	if results["failing"].Err != context.DeadlineExceeded {
		t.Errorf("failing agent: expected DeadlineExceeded, got %v", results["failing"].Err)
	}
	if results["succeeding"].Output != "success" {
		t.Errorf("succeeding agent: expected %q, got %q", "success", results["succeeding"].Output)
	}
}

func TestSpawn_SlotFreedAfterCompletion(t *testing.T) {
	bm := NewBackgroundManager(1)

	// First agent completes immediately
	err := bm.Spawn(context.Background(), "first", func(ctx context.Context) AgentResult {
		return AgentResult{Output: "done"}
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Wait for it to finish and free the slot
	<-bm.Results()

	// Now we should be able to spawn another
	err = bm.Spawn(context.Background(), "second", func(ctx context.Context) AgentResult {
		return AgentResult{Output: "also done"}
	})
	if err != nil {
		t.Fatalf("expected slot to be freed, got error: %v", err)
	}

	result := <-bm.Results()
	if result.Output != "also done" {
		t.Errorf("expected %q, got %q", "also done", result.Output)
	}
}
