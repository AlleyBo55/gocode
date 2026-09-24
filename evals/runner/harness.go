// The eval runner scores gocode against a fixed set of small coding tasks,
// one model at a time, and prints a pass-rate table. Each task is a tiny Go
// module with a bug or a gap; the agent works in a scratch copy and is scored
// by the module's own tests, including hidden ones it never saw.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/AlleyBo55/gocode/internal/apiclient"
)

// Task is one directory under evals/tasks.
type Task struct {
	ID       string
	Dir      string   // evals/tasks/<id>
	Prompt   string   `json:"prompt"`
	Timeout  int      `json:"timeout_seconds,omitempty"` // for the agent; default 300
	TestArgs []string `json:"test_args,omitempty"`       // extra args for go test, e.g. ["-race"]
}

// Result is one task scored against one model.
type Result struct {
	Task         string  `json:"task"`
	Model        string  `json:"model"`
	Passed       bool    `json:"passed"`
	Duration     float64 `json:"duration_seconds"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	Error        string  `json:"error,omitempty"`       // harness or agent failure, not a test failure
	TestOutput   string  `json:"test_output,omitempty"` // tail of go test output when it failed
}

// Usage is what an agent run reports back about its spend.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// Agent performs a task in workdir. The real one shells out to gocode; the
// harness tests substitute agents that apply the reference solution, do
// nothing, or cheat.
type Agent interface {
	Run(ctx context.Context, workdir, prompt string) (Usage, error)
}

// LoadTasks reads every task directory under root, sorted by id.
func LoadTasks(root string) ([]Task, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var tasks []Task
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), "_") || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		dir := filepath.Join(root, e.Name())
		data, err := os.ReadFile(filepath.Join(dir, "task.json"))
		if err != nil {
			return nil, fmt.Errorf("task %s: %w", e.Name(), err)
		}
		t := Task{ID: e.Name(), Dir: dir}
		if err := json.Unmarshal(data, &t); err != nil {
			return nil, fmt.Errorf("task %s: task.json: %w", e.Name(), err)
		}
		if strings.TrimSpace(t.Prompt) == "" {
			return nil, fmt.Errorf("task %s: task.json has no prompt", e.Name())
		}
		if _, err := os.Stat(filepath.Join(dir, "fixture", "go.mod")); err != nil {
			return nil, fmt.Errorf("task %s: fixture must be a Go module (fixture/go.mod): %w", e.Name(), err)
		}
		if t.Timeout <= 0 {
			t.Timeout = 300
		}
		tasks = append(tasks, t)
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	return tasks, nil
}

// RunTask copies the fixture to a scratch directory, lets the agent work in
// it, restores the visible tests, adds the hidden ones, and runs go test.
// The scratch directory is removed unless keep is set.
func RunTask(ctx context.Context, t Task, model string, agent Agent, keep bool) Result {
	res := Result{Task: t.ID, Model: model}
	start := time.Now()
	defer func() { res.Duration = time.Since(start).Seconds() }()

	workdir, err := os.MkdirTemp("", "gocode-eval-"+t.ID+"-")
	if err != nil {
		res.Error = "mkdtemp: " + err.Error()
		return res
	}
	if !keep {
		defer os.RemoveAll(workdir)
	} else {
		fmt.Fprintf(os.Stderr, "[%s] workdir kept at %s\n", t.ID, workdir)
	}

	fixture := filepath.Join(t.Dir, "fixture")
	if err := copyTree(fixture, workdir, nil); err != nil {
		res.Error = "copy fixture: " + err.Error()
		return res
	}

	agentCtx, cancel := context.WithTimeout(ctx, time.Duration(t.Timeout)*time.Second)
	usage, agentErr := agent.Run(agentCtx, workdir, t.Prompt)
	cancel()
	res.InputTokens, res.OutputTokens = usage.InputTokens, usage.OutputTokens
	if price, ok := apiclient.PriceForModel(model); ok {
		res.CostUSD = (float64(usage.InputTokens)*price.Input + float64(usage.OutputTokens)*price.Output) / 1_000_000
	}
	if agentErr != nil {
		// Still score: a partial edit that happens to pass is a pass, and a
		// timeout with no edits is a fail either way. Record the error so
		// the table can say why.
		res.Error = agentErr.Error()
	}

	// The agent may have edited or deleted tests. Put the originals back,
	// then add the ones it never saw.
	if err := copyTree(fixture, workdir, func(p string) bool { return strings.HasSuffix(p, "_test.go") }); err != nil {
		res.Error = joinErr(res.Error, "restore tests: "+err.Error())
		return res
	}
	if hidden := filepath.Join(t.Dir, "_hidden"); dirExists(hidden) {
		if err := copyTree(hidden, workdir, nil); err != nil {
			res.Error = joinErr(res.Error, "copy hidden tests: "+err.Error())
			return res
		}
	}

	passed, out, err := goTest(ctx, workdir, t.TestArgs)
	if err != nil {
		res.Error = joinErr(res.Error, "go test: "+err.Error())
	}
	res.Passed = passed
	if !passed {
		res.TestOutput = tail(out, 40)
	}
	return res
}

// goTest runs the module's tests with a clean cache and no workspace, so a
// go.work in a parent directory cannot pull the scratch module into it.
func goTest(ctx context.Context, dir string, extra []string) (bool, string, error) {
	tctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	args := append([]string{"test", "./...", "-count=1"}, extra...)
	cmd := exec.CommandContext(tctx, "go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	if err == nil {
		return true, out.String(), nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, out.String(), nil // tests or compile failed: a score, not a harness error
	}
	return false, out.String(), err
}

// GocodeAgent runs the real binary in one-shot prompt mode.
type GocodeAgent struct {
	Binary   string
	MaxTurns int
}

func (g GocodeAgent) Run(ctx context.Context, workdir, prompt string) (Usage, error) {
	maxTurns := g.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 40
	}
	cmd := exec.CommandContext(ctx, g.Binary, "prompt",
		"--model", modelFromContext(ctx),
		"--max-turns", fmt.Sprint(maxTurns),
		"--no-stream",
		"--output-format", "json",
		prompt,
	)
	cmd.Dir = workdir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()

	usage, parseErr := parseUsage(stdout.Bytes())
	if runErr != nil {
		if ctx.Err() != nil {
			return usage, fmt.Errorf("agent timed out")
		}
		return usage, fmt.Errorf("gocode exited: %v: %s", runErr, tail(stderr.String(), 5))
	}
	if parseErr != nil {
		return usage, fmt.Errorf("could not read usage from gocode output: %v", parseErr)
	}
	return usage, nil
}

type modelKey struct{}

// withModel threads the model name to the agent without widening the Agent
// interface for the fake agents that do not need it.
func withModel(ctx context.Context, model string) context.Context {
	return context.WithValue(ctx, modelKey{}, model)
}

func modelFromContext(ctx context.Context) string {
	if m, ok := ctx.Value(modelKey{}).(string); ok {
		return m
	}
	return ""
}

// parseUsage finds the structured-output envelope in gocode's stdout. Log
// lines may precede it, so scan for the last top-level JSON object.
func parseUsage(stdout []byte) (Usage, error) {
	idx := bytes.LastIndex(stdout, []byte("\n{"))
	switch {
	case idx >= 0:
		stdout = stdout[idx+1:]
	case bytes.HasPrefix(bytes.TrimSpace(stdout), []byte("{")):
		stdout = bytes.TrimSpace(stdout)
	default:
		return Usage{}, errors.New("no JSON object in output")
	}
	var env struct {
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(stdout, &env); err != nil {
		return Usage{}, err
	}
	return Usage{InputTokens: env.Usage.InputTokens, OutputTokens: env.Usage.OutputTokens}, nil
}

// copyTree copies src into dst, creating directories as needed and
// overwriting files. When only is set, just the files it accepts are copied.
func copyTree(src, dst string, only func(rel string) bool) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if only != nil && !only(rel) {
			return nil
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func joinErr(a, b string) string {
	if a == "" {
		return b
	}
	return a + "; " + b
}

func tail(s string, lines int) string {
	parts := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(parts) > lines {
		parts = parts[len(parts)-lines:]
	}
	return strings.Join(parts, "\n")
}

// Summary is one model's aggregate over a run.
type Summary struct {
	Model        string
	Passed       int
	Total        int
	InputTokens  int
	OutputTokens int
	CostUSD      float64
	Unpriced     bool
}

func Summarize(model string, results []Result) Summary {
	s := Summary{Model: model, Total: len(results)}
	_, priced := apiclient.PriceForModel(model)
	s.Unpriced = !priced
	for _, r := range results {
		if r.Passed {
			s.Passed++
		}
		s.InputTokens += r.InputTokens
		s.OutputTokens += r.OutputTokens
		s.CostUSD += r.CostUSD
	}
	return s
}

// RenderTable prints tasks down, models across, with a per-model summary.
func RenderTable(version string, tasks []Task, byModel map[string][]Result) string {
	models := make([]string, 0, len(byModel))
	for m := range byModel {
		models = append(models, m)
	}
	sort.Strings(models)

	cell := func(r *Result) string {
		if r == nil {
			return "-"
		}
		mark := "✗"
		if r.Passed {
			mark = "✓"
		}
		s := fmt.Sprintf("%s %4.0fs", mark, r.Duration)
		if r.CostUSD > 0 {
			s += fmt.Sprintf(" $%.3f", r.CostUSD)
		}
		if r.Error != "" {
			s += " !"
		}
		return s
	}
	lookup := func(model, task string) *Result {
		for i := range byModel[model] {
			if byModel[model][i].Task == task {
				return &byModel[model][i]
			}
		}
		return nil
	}

	taskWidth := len("est. cost")
	for _, t := range tasks {
		taskWidth = max(taskWidth, len(t.ID))
	}
	colWidth := 18
	for _, m := range models {
		colWidth = max(colWidth, len(m)+2)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "gocode %s — %d tasks\n", version, len(tasks))
	fmt.Fprintf(&b, "%-*s", taskWidth, "task")
	for _, m := range models {
		fmt.Fprintf(&b, "  %-*s", colWidth, m)
	}
	b.WriteString("\n")
	for _, t := range tasks {
		fmt.Fprintf(&b, "%-*s", taskWidth, t.ID)
		for _, m := range models {
			fmt.Fprintf(&b, "  %-*s", colWidth, cell(lookup(m, t.ID)))
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "%-*s", taskWidth, "pass rate")
	for _, m := range models {
		s := Summarize(m, byModel[m])
		pct := 0.0
		if s.Total > 0 {
			pct = float64(s.Passed) / float64(s.Total) * 100
		}
		fmt.Fprintf(&b, "  %-*s", colWidth, fmt.Sprintf("%d/%d (%.0f%%)", s.Passed, s.Total, pct))
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "%-*s", taskWidth, "tokens")
	for _, m := range models {
		s := Summarize(m, byModel[m])
		fmt.Fprintf(&b, "  %-*s", colWidth, fmt.Sprintf("%s in / %s out", human(s.InputTokens), human(s.OutputTokens)))
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "%-*s", taskWidth, "est. cost")
	for _, m := range models {
		s := Summarize(m, byModel[m])
		if s.Unpriced {
			fmt.Fprintf(&b, "  %-*s", colWidth, "n/a (no price data)")
		} else {
			fmt.Fprintf(&b, "  %-*s", colWidth, fmt.Sprintf("$%.2f", s.CostUSD))
		}
	}
	b.WriteString("\n")
	if hasErrors(byModel) {
		b.WriteString("! = the agent run itself failed (timeout or crash); see the results file for details\n")
	}
	return b.String()
}

func hasErrors(byModel map[string][]Result) bool {
	for _, rs := range byModel {
		for _, r := range rs {
			if r.Error != "" {
				return true
			}
		}
	}
	return false
}

func human(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.0fk", float64(n)/1_000)
	default:
		return fmt.Sprint(n)
	}
}

// RunFile is what gets written to evals/results.
type RunFile struct {
	GocodeVersion string    `json:"gocode_version"`
	Model         string    `json:"model"`
	StartedAt     time.Time `json:"started_at"`
	Summary       Summary   `json:"summary"`
	Results       []Result  `json:"results"`
}
