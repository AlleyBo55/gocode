package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// solutionAgent applies the task's reference solution: what a perfect agent
// would produce. It proves each task is solvable and its hidden tests are
// correct, without spending a token.
type solutionAgent struct{ taskDir string }

func (a solutionAgent) Run(_ context.Context, workdir, _ string) (Usage, error) {
	return Usage{InputTokens: 100, OutputTokens: 10}, copyTree(filepath.Join(a.taskDir, "_solution"), workdir, nil)
}

// noopAgent changes nothing: the untouched fixture must fail, or the task
// cannot tell a working agent from a broken one.
type noopAgent struct{}

func (noopAgent) Run(context.Context, string, string) (Usage, error) { return Usage{}, nil }

// cheatingAgent deletes every test and, if the task ships one, replaces the
// implementation with something that would only pass a weakened test.
type cheatingAgent struct{}

func (cheatingAgent) Run(_ context.Context, workdir, _ string) (Usage, error) {
	return Usage{}, filepath.WalkDir(workdir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, "_test.go") {
			return os.Remove(p)
		}
		return err
	})
}

func tasksRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "tasks"))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestEveryTaskIsSolvableAndDiscriminating(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles every fixture twice; skipped under -short")
	}
	tasks, err := LoadTasks(tasksRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) < 10 {
		t.Fatalf("only %d tasks; the suite needs at least 10 to say anything", len(tasks))
	}
	// Each task compiles its fixture twice via a nested `go test`, which uses
	// every core. Running them in parallel starved the rest of the module's
	// suite on a loaded machine (the hooks tests spawn shells under a 5s
	// deadline and timed out), so the tasks run one at a time. The fixtures
	// are tiny; the whole loop is around fifteen seconds on a warm cache.
	for _, task := range tasks {
		task := task
		t.Run(task.ID, func(t *testing.T) {
			if !dirExists(filepath.Join(task.Dir, "_solution")) {
				t.Fatal("task has no _solution directory; it cannot be validated")
			}
			if !dirExists(filepath.Join(task.Dir, "_hidden")) {
				t.Fatal("task has no _hidden tests; a visible-only task can be passed by editing the test")
			}

			solved := RunTask(context.Background(), task, "test-model", solutionAgent{task.Dir}, false)
			if !solved.Passed {
				t.Errorf("reference solution fails:\n%s\n%s", solved.Error, solved.TestOutput)
			}
			if solved.InputTokens != 100 || solved.OutputTokens != 10 {
				t.Errorf("usage not recorded: %+v", solved)
			}

			unsolved := RunTask(context.Background(), task, "test-model", noopAgent{}, false)
			if unsolved.Passed {
				t.Error("untouched fixture passes: the task does not test anything")
			}
			if unsolved.Error != "" {
				t.Errorf("a failing fixture is a score, not a harness error: %s", unsolved.Error)
			}
		})
	}
}

func TestDeletingTestsDoesNotPass(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles a fixture; skipped under -short")
	}
	tasks, err := LoadTasks(tasksRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	// One task is enough: the restore-and-hide step is task-independent.
	res := RunTask(context.Background(), tasks[0], "test-model", cheatingAgent{}, false)
	if res.Passed {
		t.Fatal("an agent that deletes the tests must not score a pass")
	}
	if res.Error != "" {
		t.Errorf("unexpected harness error: %s", res.Error)
	}
}

func TestLoadTasksRejectsBrokenTaskDirs(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("ok/task.json", `{"prompt":"do it","timeout_seconds":7,"test_args":["-race"]}`)
	write("ok/fixture/go.mod", "module x\n\ngo 1.21\n")
	write("_ignored/task.json", `{"prompt":"hidden helper dir"}`)
	tasks, err := LoadTasks(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].ID != "ok" || tasks[0].Timeout != 7 || len(tasks[0].TestArgs) != 1 {
		t.Errorf("tasks = %+v", tasks)
	}

	write("noprompt/task.json", `{}`)
	write("noprompt/fixture/go.mod", "module y\n")
	if _, err := LoadTasks(root); err == nil || !strings.Contains(err.Error(), "no prompt") {
		t.Errorf("missing prompt should be rejected, got %v", err)
	}
	if err := os.RemoveAll(filepath.Join(root, "noprompt")); err != nil {
		t.Fatal(err)
	}

	write("nomod/task.json", `{"prompt":"x"}`)
	if _, err := LoadTasks(root); err == nil || !strings.Contains(err.Error(), "go.mod") {
		t.Errorf("fixture without go.mod should be rejected, got %v", err)
	}
}

func TestParseUsageFindsTheEnvelopeAfterLogNoise(t *testing.T) {
	out := []byte("[skills] loaded 3\nsome log line\n{\n  \"result\": \"done\",\n  \"usage\": {\"input_tokens\": 1234, \"output_tokens\": 56, \"total_cost\": 0.01}\n}\n")
	u, err := parseUsage(out)
	if err != nil || u.InputTokens != 1234 || u.OutputTokens != 56 {
		t.Errorf("usage = %+v, %v", u, err)
	}
	if u, err := parseUsage([]byte(`{"usage":{"input_tokens":1,"output_tokens":2}}`)); err != nil || u.InputTokens != 1 {
		t.Errorf("bare object: %+v %v", u, err)
	}
	if _, err := parseUsage([]byte("no json here")); err == nil {
		t.Error("missing envelope must be an error so a crashed agent is not recorded as free")
	}
}

func TestRenderTableAndSummary(t *testing.T) {
	tasks := []Task{{ID: "01-a"}, {ID: "02-b"}}
	byModel := map[string][]Result{
		"claude-sonnet-4-6": {
			{Task: "01-a", Passed: true, Duration: 12, InputTokens: 10_000, OutputTokens: 1_000, CostUSD: 0.045},
			{Task: "02-b", Passed: false, Duration: 30, InputTokens: 20_000, OutputTokens: 2_000, CostUSD: 0.09, Error: "agent timed out"},
		},
		"my-finetune": {
			{Task: "01-a", Passed: true, Duration: 5},
			{Task: "02-b", Passed: true, Duration: 6},
		},
	}
	out := RenderTable("v0.9.0", tasks, byModel)

	for _, want := range []string{
		"gocode v0.9.0 — 2 tasks",
		"claude-sonnet-4-6", "my-finetune",
		"1/2 (50%)", "2/2 (100%)",
		"30k in / 3k out",
		"$0.14",
		"n/a (no price data)",
		"✗   30s $0.090 !",
		"! = the agent run itself failed",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q:\n%s", want, out)
		}
	}

	s := Summarize("claude-sonnet-4-6", byModel["claude-sonnet-4-6"])
	if s.Passed != 1 || s.Total != 2 || s.InputTokens != 30_000 || s.Unpriced {
		t.Errorf("summary = %+v", s)
	}
	if s := Summarize("my-finetune", nil); !s.Unpriced || s.Total != 0 {
		t.Errorf("unpriced summary = %+v", s)
	}
}

func TestRunTaskRecordsCostFromPricingTable(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles a fixture; skipped under -short")
	}
	tasks, err := LoadTasks(tasksRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	res := RunTask(context.Background(), tasks[0], "claude-sonnet-4-6", solutionAgent{tasks[0].Dir}, false)
	// 100 in at $3/M + 10 out at $15/M.
	if want := (100*3.0 + 10*15.0) / 1_000_000; res.CostUSD < want-1e-12 || res.CostUSD > want+1e-12 {
		t.Errorf("cost = %v, want %v", res.CostUSD, want)
	}
}
