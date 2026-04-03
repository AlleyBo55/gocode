package swarm

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestNewSwarmManager(t *testing.T) {
	sm := NewSwarmManager(0)
	if sm == nil {
		t.Fatal("NewSwarmManager returned nil")
	}
	if sm.maxAgents != 10 {
		t.Errorf("expected default maxAgents=10, got %d", sm.maxAgents)
	}

	sm2 := NewSwarmManager(5)
	if sm2.maxAgents != 5 {
		t.Errorf("expected maxAgents=5, got %d", sm2.maxAgents)
	}
}

func TestRegister(t *testing.T) {
	sm := NewSwarmManager(10)
	err := sm.Register("agent-a", []string{"code"}, "worker")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	agents := sm.ListAgents()
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].Name != "agent-a" {
		t.Errorf("expected name agent-a, got %s", agents[0].Name)
	}
	if agents[0].Status != StatusRunning {
		t.Errorf("expected status running, got %s", agents[0].Status)
	}
}

func TestRegister_Duplicate(t *testing.T) {
	sm := NewSwarmManager(10)
	_ = sm.Register("dup", nil, "")
	err := sm.Register("dup", nil, "")
	if err == nil {
		t.Fatal("expected error registering duplicate agent")
	}
}

func TestRegister_Full(t *testing.T) {
	sm := NewSwarmManager(2)
	_ = sm.Register("a1", nil, "")
	_ = sm.Register("a2", nil, "")
	err := sm.Register("a3", nil, "")
	if err == nil {
		t.Fatal("expected error when swarm is full")
	}
}

func TestUnregister(t *testing.T) {
	sm := NewSwarmManager(10)
	_ = sm.Register("rm-me", nil, "")
	sm.Unregister("rm-me")

	agents := sm.ListAgents()
	if len(agents) != 0 {
		t.Errorf("expected 0 agents after unregister, got %d", len(agents))
	}
}

func TestUpdateStatus(t *testing.T) {
	sm := NewSwarmManager(10)
	_ = sm.Register("worker", nil, "")
	sm.UpdateStatus("worker", StatusCompleted)

	agents := sm.ListAgents()
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].Status != StatusCompleted {
		t.Errorf("expected status completed, got %s", agents[0].Status)
	}
}

func TestSendMessage(t *testing.T) {
	sm := NewSwarmManager(10)
	_ = sm.Register("sender", nil, "")
	_ = sm.Register("receiver", nil, "")

	err := sm.SendMessage("sender", "receiver", "hello")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	msg, ok := sm.ReceiveMessage("receiver")
	if !ok {
		t.Fatal("expected to receive a message")
	}
	if msg.From != "sender" || msg.To != "receiver" || msg.Content != "hello" {
		t.Errorf("unexpected message: %+v", msg)
	}
}

func TestSendMessage_NotFound(t *testing.T) {
	sm := NewSwarmManager(10)
	err := sm.SendMessage("a", "ghost", "hi")
	if err == nil {
		t.Fatal("expected error sending to non-existent agent")
	}
}

func TestReceiveMessage_Empty(t *testing.T) {
	sm := NewSwarmManager(10)
	_ = sm.Register("lonely", nil, "")

	_, ok := sm.ReceiveMessage("lonely")
	if ok {
		t.Fatal("expected no message for agent with empty mailbox")
	}
}

func TestMailbox_NonBlocking(t *testing.T) {
	mb := NewMailbox()

	// Receive on empty mailbox should return immediately
	done := make(chan struct{})
	go func() {
		_, ok := mb.Receive()
		if ok {
			t.Error("expected no message from empty mailbox")
		}
		close(done)
	}()

	select {
	case <-done:
		// good — returned without blocking
	case <-time.After(time.Second):
		t.Fatal("Receive blocked on empty mailbox")
	}

	// Send on non-full mailbox should return immediately
	done2 := make(chan struct{})
	go func() {
		ok := mb.Send(SwarmMessage{Content: "test"})
		if !ok {
			t.Error("expected Send to succeed")
		}
		close(done2)
	}()

	select {
	case <-done2:
		// good
	case <-time.After(time.Second):
		t.Fatal("Send blocked on non-full mailbox")
	}
}

func TestConcurrentSendReceive(t *testing.T) {
	sm := NewSwarmManager(10)
	_ = sm.Register("producer", nil, "")
	_ = sm.Register("consumer", nil, "")

	const n = 50
	var wg sync.WaitGroup

	// Spawn goroutines that send messages concurrently
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = sm.SendMessage("producer", "consumer", fmt.Sprintf("msg-%d", i))
		}(i)
	}
	wg.Wait()

	// Drain all messages
	received := 0
	for {
		_, ok := sm.ReceiveMessage("consumer")
		if !ok {
			break
		}
		received++
	}
	if received != n {
		t.Errorf("expected %d messages, received %d", n, received)
	}
}

func TestSendMessageTool_Execute(t *testing.T) {
	sm := NewSwarmManager(10)
	_ = sm.Register("alice", nil, "")
	_ = sm.Register("bob", nil, "")

	tool := &SendMessageTool{Swarm: sm, FromName: "alice"}
	result := tool.Execute(map[string]interface{}{
		"to":      "bob",
		"message": "hey bob",
	})
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}

	// Verify the message was actually delivered
	msg, ok := sm.ReceiveMessage("bob")
	if !ok {
		t.Fatal("expected bob to have a message")
	}
	if msg.Content != "hey bob" || msg.From != "alice" {
		t.Errorf("unexpected message: %+v", msg)
	}
}

func TestSendMessageTool_MissingTo(t *testing.T) {
	sm := NewSwarmManager(10)
	tool := &SendMessageTool{Swarm: sm, FromName: "x"}

	result := tool.Execute(map[string]interface{}{
		"message": "hello",
	})
	if result.Success {
		t.Fatal("expected failure for missing 'to' param")
	}
	if result.Error == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestSendMessageTool_TargetNotFound(t *testing.T) {
	sm := NewSwarmManager(10)
	_ = sm.Register("sender", nil, "")

	tool := &SendMessageTool{Swarm: sm, FromName: "sender"}
	result := tool.Execute(map[string]interface{}{
		"to":      "nobody",
		"message": "hello",
	})
	if result.Success {
		t.Fatal("expected failure for non-existent target")
	}
	if result.Error == "" {
		t.Fatal("expected non-empty error message")
	}
}
