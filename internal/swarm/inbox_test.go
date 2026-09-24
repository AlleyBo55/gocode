package swarm

import (
	"reflect"
	"strings"
	"testing"
)

func TestDrainReturnsAllPendingInOrderThenNothing(t *testing.T) {
	s := NewSwarmManager(5)
	_ = s.Register(MainAgent, nil, "main")
	_ = s.Register("worker-1", nil, "deep")
	for _, m := range []string{"first", "second", "third"} {
		if err := s.SendMessage("worker-1", MainAgent, m); err != nil {
			t.Fatal(err)
		}
	}
	got := s.Drain(MainAgent)
	if len(got) != 3 || got[0].Content != "first" || got[2].Content != "third" || got[0].From != "worker-1" {
		t.Errorf("drain = %+v", got)
	}
	if again := s.Drain(MainAgent); len(again) != 0 {
		t.Errorf("second drain should be empty, got %d", len(again))
	}
	if unknown := s.Drain("nobody"); unknown != nil {
		t.Errorf("unregistered agent should drain nil, got %v", unknown)
	}
}

func TestFormatInboxNamesTheSender(t *testing.T) {
	got := FormatInbox([]SwarmMessage{{From: "planner-2", Content: "plan ready"}, {From: MainAgent, Content: "stop"}})
	want := []string{"[message from planner-2]\nplan ready", "[message from main]\nstop"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q", got)
	}
	if got := FormatInbox(nil); len(got) != 0 {
		t.Errorf("nil inbox should format to nothing, got %q", got)
	}
}

func TestExecuteToolSendAndList(t *testing.T) {
	s := NewSwarmManager(5)
	_ = s.Register(MainAgent, nil, "main")
	_ = s.Register("deep-worker-1", []string{"filereadtool"}, "deep")

	res, ok := s.ExecuteTool("deep-worker-1", ToolSendMessage, map[string]interface{}{"to": MainAgent, "message": "found it"})
	if !ok || res.IsError || !strings.Contains(res.Output, "delivered to main") {
		t.Fatalf("send = %+v ok=%v", res, ok)
	}
	if msgs := s.Drain(MainAgent); len(msgs) != 1 || msgs[0].From != "deep-worker-1" || msgs[0].Content != "found it" {
		t.Errorf("main inbox = %+v", msgs)
	}

	res, _ = s.ExecuteTool("deep-worker-1", ToolSendMessage, map[string]interface{}{"to": "ghost", "message": "hi"})
	if !res.IsError || !strings.Contains(res.Output, "not found") || !strings.Contains(res.Output, "list_agents") {
		t.Errorf("unknown target should point at list_agents: %+v", res)
	}
	res, _ = s.ExecuteTool("deep-worker-1", ToolSendMessage, map[string]interface{}{"to": MainAgent})
	if !res.IsError {
		t.Error("missing message must be an error")
	}

	res, ok = s.ExecuteTool("deep-worker-1", "LIST_AGENTS", nil)
	if !ok || res.IsError {
		t.Fatalf("list = %+v ok=%v", res, ok)
	}
	lines := strings.Split(strings.TrimSpace(res.Output), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "deep-worker-1\trunning\tdeep\t(you)") || !strings.HasPrefix(lines[1], "main\trunning") {
		t.Errorf("list output = %q", res.Output)
	}

	if _, ok := s.ExecuteTool("x", "bashtool", nil); ok {
		t.Error("non-swarm tools must not be claimed")
	}
	if !IsSwarmTool("send_agent_message") || !IsSwarmTool("List_Agents") || IsSwarmTool("bashtool") {
		t.Error("IsSwarmTool")
	}
	if defs := ToolDefs(); len(defs) != 2 || defs[0].Name != ToolSendMessage || !strings.Contains(defs[0].Description, `"main"`) {
		t.Errorf("defs = %+v", defs)
	}
}
