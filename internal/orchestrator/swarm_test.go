package orchestrator

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/AlleyBo55/gocode/internal/agent"
	"github.com/AlleyBo55/gocode/internal/apiclient"
	"github.com/AlleyBo55/gocode/internal/apitypes"
	"github.com/AlleyBo55/gocode/internal/swarm"
)

// scriptProvider answers each request with the next scripted reply and lets
// the test observe every request the sub-agent made.
type scriptProvider struct {
	replies  []func(req apitypes.MessageRequest) *apitypes.MessageResponse
	requests []apitypes.MessageRequest
}

func (p *scriptProvider) SendMessage(_ context.Context, req apitypes.MessageRequest) (*apitypes.MessageResponse, error) {
	p.requests = append(p.requests, req)
	i := len(p.requests) - 1
	if i >= len(p.replies) {
		return textReply("done"), nil
	}
	return p.replies[i](req), nil
}

func (p *scriptProvider) StreamMessage(context.Context, apitypes.MessageRequest) (<-chan apitypes.StreamEvent, error) {
	ch := make(chan apitypes.StreamEvent)
	close(ch)
	return ch, nil
}

func (p *scriptProvider) Kind() apiclient.ProviderKind { return apiclient.ProviderAnthropic }

func textReply(text string) *apitypes.MessageResponse {
	return &apitypes.MessageResponse{Role: "assistant", StopReason: "end_turn", Content: []apitypes.OutputContentBlock{{Kind: "text", Text: text}}}
}

func toolReply(id, name, args string) *apitypes.MessageResponse {
	return &apitypes.MessageResponse{Role: "assistant", StopReason: "tool_use", Content: []apitypes.OutputContentBlock{
		{Kind: "tool_use", ID: id, Name: name, Input: json.RawMessage(args)},
	}}
}

func routerFor(p apiclient.Provider) *apiclient.ModelRouter {
	fp := apiclient.NewFallbackProvider([]apiclient.FallbackEntry{{Model: "test-model", Provider: p}}, nil)
	return apiclient.NewModelRouter(map[apiclient.TaskCategory]*apiclient.FallbackProvider{
		apiclient.CategoryQuick: fp, apiclient.CategoryDeep: fp,
	})
}

func TestDelegateJoinsTheSwarmAndCanBeMessagedMidRun(t *testing.T) {
	mgr := swarm.NewSwarmManager(10)
	if err := mgr.Register(swarm.MainAgent, nil, "main"); err != nil {
		t.Fatal(err)
	}

	var seenDuringRun []swarm.AgentInfo
	p := &scriptProvider{}
	p.replies = []func(apitypes.MessageRequest) *apitypes.MessageResponse{
		// Turn 1: the sub-agent is registered by now. A sibling (main here)
		// messages it, and it calls list_agents.
		func(apitypes.MessageRequest) *apitypes.MessageResponse {
			seenDuringRun = mgr.ListAgents()
			if err := mgr.SendMessage(swarm.MainAgent, "deep-worker-1", "focus on go.mod"); err != nil {
				t.Errorf("main could not message the running sub-agent: %v", err)
			}
			return toolReply("t1", "list_agents", `{}`)
		},
		// Turn 2: it should now see both the tool result and the message.
		func(apitypes.MessageRequest) *apitypes.MessageResponse {
			return toolReply("t2", "send_agent_message", `{"to":"main","message":"go.mod says example.com"}`)
		},
		func(apitypes.MessageRequest) *apitypes.MessageResponse { return textReply("finished") },
	}

	o := NewOrchestrator(routerFor(p), agent.NewStaticExecutor()).WithSwarm(mgr)
	out, err := o.Delegate(context.Background(), "deep-worker", "inspect the module")
	if err != nil || out != "finished" {
		t.Fatalf("out=%q err=%v", out, err)
	}

	// Registered under an instance name while running, gone afterwards.
	names := map[string]bool{}
	for _, a := range seenDuringRun {
		names[a.Name] = true
	}
	if !names["deep-worker-1"] || !names[swarm.MainAgent] {
		t.Errorf("during the run the swarm should hold main and deep-worker-1, saw %v", names)
	}
	for _, a := range mgr.ListAgents() {
		if a.Name == "deep-worker-1" {
			t.Error("sub-agent must leave the swarm when it finishes")
		}
	}

	// The sub-agent was offered the swarm tools in addition to its own.
	tools := map[string]bool{}
	for _, d := range p.requests[0].Tools {
		tools[d.Name] = true
	}
	if !tools[swarm.ToolSendMessage] || !tools[swarm.ToolListAgents] {
		t.Errorf("swarm tools not offered to the sub-agent: %v", tools)
	}
	if !strings.Contains(p.requests[0].System, `agent "deep-worker-1" in a swarm`) {
		t.Errorf("system prompt should name the instance: %q", p.requests[0].System)
	}

	// Turn 2 saw: prompt, assistant tool_use, tool_result (agent list), inbox.
	second := p.requests[1].Messages
	if len(second) != 4 {
		t.Fatalf("turn 2 had %d messages: %+v", len(second), second)
	}
	list := second[2].Content[0]
	if list.Kind != "tool_result" || list.IsError || !strings.Contains(list.Content, "deep-worker-1\trunning\tdeep\t(you)") || !strings.Contains(list.Content, "main\trunning") {
		t.Errorf("list_agents result = %+v", list)
	}
	if second[3].Role != "user" || !strings.Contains(second[3].Content[0].Text, "[message from main]\nfocus on go.mod") {
		t.Errorf("message from main not delivered before turn 2: %+v", second[3])
	}

	// Its message to main arrived with the right sender.
	inbox := mgr.Drain(swarm.MainAgent)
	if len(inbox) != 1 || inbox[0].From != "deep-worker-1" || inbox[0].Content != "go.mod says example.com" {
		t.Errorf("main inbox = %+v", inbox)
	}
	// And send_agent_message reported success to the model.
	sent := p.requests[2].Messages
	res := sent[len(sent)-1].Content[0]
	if res.Kind != "tool_result" || res.IsError || !strings.Contains(res.Content, "delivered to main") {
		t.Errorf("send result = %+v", res)
	}
}

func TestInstanceNamesAreUniqueAndSequential(t *testing.T) {
	mgr := swarm.NewSwarmManager(10)
	_ = mgr.Register(swarm.MainAgent, nil, "main")
	var systems []string
	p := &scriptProvider{replies: []func(apitypes.MessageRequest) *apitypes.MessageResponse{
		func(r apitypes.MessageRequest) *apitypes.MessageResponse {
			systems = append(systems, r.System)
			return textReply("a")
		},
		func(r apitypes.MessageRequest) *apitypes.MessageResponse {
			systems = append(systems, r.System)
			return textReply("b")
		},
		func(r apitypes.MessageRequest) *apitypes.MessageResponse {
			systems = append(systems, r.System)
			return textReply("c")
		},
	}}
	o := NewOrchestrator(routerFor(p), agent.NewStaticExecutor()).WithSwarm(mgr)
	for _, profile := range []string{"planner", "planner", "debugger"} {
		if _, err := o.Delegate(context.Background(), profile, "x"); err != nil {
			t.Fatal(err)
		}
	}
	for i, want := range []string{`"planner-1"`, `"planner-2"`, `"debugger-1"`} {
		if !strings.Contains(systems[i], want) {
			t.Errorf("run %d should be %s: %q", i, want, systems[i])
		}
	}
}

func TestBackgroundAgentReportsToMain(t *testing.T) {
	mgr := swarm.NewSwarmManager(10)
	_ = mgr.Register(swarm.MainAgent, nil, "main")
	p := &scriptProvider{replies: []func(apitypes.MessageRequest) *apitypes.MessageResponse{
		func(apitypes.MessageRequest) *apitypes.MessageResponse {
			return textReply("The bug is in parse.go line 40.")
		},
	}}
	o := NewOrchestrator(routerFor(p), agent.NewStaticExecutor()).WithSwarm(mgr)

	ch := o.DelegateBackground(context.Background(), "debugger", "find the parse bug")
	select {
	case res := <-ch:
		if res.Err != nil || res.Output != "The bug is in parse.go line 40." {
			t.Fatalf("result = %+v", res)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("background agent did not finish")
	}

	inbox := mgr.Drain(swarm.MainAgent)
	if len(inbox) != 1 {
		t.Fatalf("main inbox = %+v", inbox)
	}
	if inbox[0].From != "debugger-1" || !strings.Contains(inbox[0].Content, `finished task "find the parse bug"`) || !strings.Contains(inbox[0].Content, "parse.go line 40") {
		t.Errorf("report = %+v", inbox[0])
	}
}

func TestBackgroundFailureIsReportedNotSwallowed(t *testing.T) {
	mgr := swarm.NewSwarmManager(10)
	_ = mgr.Register(swarm.MainAgent, nil, "main")
	o := NewOrchestrator(routerFor(&scriptProvider{}), agent.NewStaticExecutor()).WithSwarm(mgr)
	res := <-o.DelegateBackground(context.Background(), "no-such-profile", "x")
	if res.Err == nil {
		t.Fatal("unknown profile must error")
	}
	inbox := mgr.Drain(swarm.MainAgent)
	if len(inbox) != 1 || !strings.Contains(inbox[0].Content, "failed") || !strings.Contains(inbox[0].Content, "unknown sub-agent") {
		t.Errorf("failure not reported to main: %+v", inbox)
	}
}

func TestReportToMainIsBounded(t *testing.T) {
	mgr := swarm.NewSwarmManager(10)
	_ = mgr.Register(swarm.MainAgent, nil, "main")
	o := NewOrchestrator(routerFor(&scriptProvider{}), agent.NewStaticExecutor()).WithSwarm(mgr)
	o.reportToMain("planner-9", strings.Repeat("t", 500), strings.Repeat("o", 50_000), nil)
	inbox := mgr.Drain(swarm.MainAgent)
	if len(inbox) != 1 || len(inbox[0].Content) > 5000 || !strings.Contains(inbox[0].Content, "more bytes]") {
		t.Errorf("report not bounded: %d bytes", len(inbox[0].Content))
	}
}

func TestWithoutSwarmNothingChanges(t *testing.T) {
	p := &scriptProvider{replies: []func(apitypes.MessageRequest) *apitypes.MessageResponse{
		func(apitypes.MessageRequest) *apitypes.MessageResponse { return textReply("plain") },
	}}
	o := NewOrchestrator(routerFor(p), agent.NewStaticExecutor())
	out, err := o.Delegate(context.Background(), "planner", "x")
	if err != nil || out != "plain" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	for _, d := range p.requests[0].Tools {
		if swarm.IsSwarmTool(d.Name) {
			t.Errorf("swarm tools offered without a swarm: %s", d.Name)
		}
	}
	if strings.Contains(p.requests[0].System, "swarm") {
		t.Error("system prompt mentions a swarm that is not attached")
	}
}
