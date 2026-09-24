package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func main() {
	var models multiFlag
	flag.Var(&models, "model", "model to evaluate (repeatable)")
	binary := flag.String("gocode", "", "gocode binary to run; default builds ./cmd/gocode from this checkout")
	tasksDir := flag.String("tasks", "", "tasks directory; default evals/tasks in this checkout")
	only := flag.String("only", "", "comma-separated task ids to run (default all)")
	parallel := flag.Int("parallel", 1, "tasks to run at once per model")
	maxTurns := flag.Int("max-turns", 40, "agent loop iterations per task")
	outDir := flag.String("out", "", "results directory; default evals/results in this checkout")
	keep := flag.Bool("keep", false, "keep scratch directories for inspection")
	flag.Parse()

	if len(models) == 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./evals/runner -model <alias-or-id> [-model ...]")
		fmt.Fprintln(os.Stderr, "This spends tokens: roughly 13 agent runs per model, each a small Go task.")
		os.Exit(2)
	}

	root, err := moduleRoot()
	if err != nil {
		fatal("finding module root (run from inside the gocode checkout): %v", err)
	}
	if *tasksDir == "" {
		*tasksDir = filepath.Join(root, "evals", "tasks")
	}
	if *outDir == "" {
		*outDir = filepath.Join(root, "evals", "results")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if *binary == "" {
		*binary, err = buildGocode(ctx, root)
		if err != nil {
			fatal("building gocode: %v", err)
		}
		defer os.RemoveAll(filepath.Dir(*binary))
	}
	version := gocodeVersion(ctx, *binary)

	tasks, err := LoadTasks(*tasksDir)
	if err != nil {
		fatal("loading tasks: %v", err)
	}
	tasks = filterTasks(tasks, *only)
	if len(tasks) == 0 {
		fatal("no tasks selected")
	}

	fmt.Fprintf(os.Stderr, "gocode %s, %d tasks, %d model(s), parallel=%d\n", version, len(tasks), len(models), *parallel)
	byModel := map[string][]Result{}
	for _, model := range models {
		started := time.Now().UTC()
		results := runAll(withModel(ctx, model), tasks, model, GocodeAgent{Binary: *binary, MaxTurns: *maxTurns}, *parallel, *keep)
		byModel[model] = results
		if err := writeRun(*outDir, RunFile{GocodeVersion: version, Model: model, StartedAt: started, Summary: Summarize(model, results), Results: results}); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not write results: %v\n", err)
		}
		if ctx.Err() != nil {
			break
		}
	}
	fmt.Print(RenderTable(version, tasks, byModel))
}

func runAll(ctx context.Context, tasks []Task, model string, agent Agent, parallel int, keep bool) []Result {
	if parallel < 1 {
		parallel = 1
	}
	results := make([]Result, len(tasks))
	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	for i, t := range tasks {
		if ctx.Err() != nil {
			results[i] = Result{Task: t.ID, Model: model, Error: "interrupted"}
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, t Task) {
			defer wg.Done()
			defer func() { <-sem }()
			fmt.Fprintf(os.Stderr, "[%s] %s: running\n", model, t.ID)
			results[i] = RunTask(ctx, t, model, agent, keep)
			mark := "FAIL"
			if results[i].Passed {
				mark = "pass"
			}
			fmt.Fprintf(os.Stderr, "[%s] %s: %s (%.0fs)\n", model, t.ID, mark, results[i].Duration)
		}(i, t)
	}
	wg.Wait()
	return results
}

func writeRun(dir string, run RunFile) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	name := fmt.Sprintf("%s_%s.json", run.StartedAt.Format("20060102-150405"), sanitize(run.Model))
	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "results written to %s\n", path)
	return nil
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '.' {
			return r
		}
		return '_'
	}, s)
}

func moduleRoot() (string, error) {
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		return "", err
	}
	gomod := strings.TrimSpace(string(out))
	if gomod == "" || gomod == os.DevNull {
		return "", fmt.Errorf("not inside a Go module")
	}
	return filepath.Dir(gomod), nil
}

func buildGocode(ctx context.Context, root string) (string, error) {
	dir, err := os.MkdirTemp("", "gocode-eval-bin-")
	if err != nil {
		return "", err
	}
	bin := filepath.Join(dir, "gocode")
	cmd := exec.CommandContext(ctx, "go", "build", "-o", bin, "./cmd/gocode")
	cmd.Dir = root
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	return bin, nil
}

func gocodeVersion(ctx context.Context, bin string) string {
	out, err := exec.CommandContext(ctx, bin, "--version").Output()
	if err != nil {
		return "unknown"
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return "unknown"
	}
	return fields[len(fields)-1]
}

func filterTasks(tasks []Task, only string) []Task {
	if strings.TrimSpace(only) == "" {
		return tasks
	}
	want := map[string]bool{}
	for _, id := range strings.Split(only, ",") {
		want[strings.TrimSpace(id)] = true
	}
	var out []Task
	for _, t := range tasks {
		if want[t.ID] {
			out = append(out, t)
		}
	}
	return out
}

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
