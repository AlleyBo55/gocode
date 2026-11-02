package main

// Compare mode runs several agent CLIs on the same tasks with the same
// model. Every run gets a fresh HOME and a fresh git repo holding the
// fixture, runs under one macOS sandbox profile, and reaches the model only
// through the metering proxy, so pass rate, time and cost are measured the
// same way for every agent.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/AlleyBo55/gocode/internal/apiclient"
)

const sandboxExec = "/usr/bin/sandbox-exec"

// knownAgents are the CLIs compare mode knows how to drive headless.
var knownAgents = []string{"gocode", "opencode", "crush", "claurst"}

type agentSpec struct {
	Name   string
	Binary string // empty for gocode means the binary this runner builds
}

func parseAgentSpecs(values []string) ([]agentSpec, error) {
	var specs []agentSpec
	for _, v := range values {
		name, bin, _ := strings.Cut(v, "=")
		name = strings.TrimSpace(name)
		if !slices.Contains(knownAgents, name) {
			return nil, fmt.Errorf("unknown agent %q (known: %s)", name, strings.Join(knownAgents, ", "))
		}
		if slices.ContainsFunc(specs, func(s agentSpec) bool { return s.Name == name }) {
			return nil, fmt.Errorf("agent %q given twice", name)
		}
		if bin == "" && name != "gocode" {
			p, err := exec.LookPath(name)
			if err != nil {
				return nil, fmt.Errorf("%s is not on PATH; pass -agent %s=/path/to/%s", name, name, name)
			}
			bin = p
		}
		if bin != "" {
			abs, err := filepath.Abs(bin)
			if err != nil {
				return nil, err
			}
			bin = abs
		}
		specs = append(specs, agentSpec{Name: name, Binary: bin})
	}
	return specs, nil
}

type compareConfig struct {
	Model     string
	Agents    []agentSpec
	Tasks     []Task
	Runs      int
	Parallel  int
	MaxTurns  int
	BudgetUSD float64
	OutDir    string
	Keep      bool
	AgentPath []string
	Catalog   string
}

// CompareFile is what a comparison writes to its results directory.
type CompareFile struct {
	Model          string         `json:"model"`
	StartedAt      time.Time      `json:"started_at"`
	FinishedAt     time.Time      `json:"finished_at"`
	Runs           int            `json:"runs"`
	MaxTurns       int            `json:"max_turns"`
	BudgetUSD      float64        `json:"budget_usd"`
	SpentUSD       float64        `json:"spent_usd"`
	Price          anthropicPrice `json:"price_per_mtok"`
	Agents         []AgentInfo    `json:"agents"`
	SandboxProfile string         `json:"sandbox_profile"`
	Results        []Result       `json:"results"`
}

type AgentInfo struct {
	Name    string   `json:"name"`
	Version string   `json:"version"`
	Command []string `json:"command"`
}

type sharedDirs struct {
	homes, gocache, gomodcache, gopath string
}

type taskCtxKey struct{}
type runNumCtxKey struct{}

func runCompare(ctx context.Context, cfg compareConfig) error {
	if runtime.GOOS != "darwin" {
		return errors.New("compare mode sandboxes each agent with macOS sandbox-exec, so it runs on macOS only")
	}
	if _, err := os.Stat(sandboxExec); err != nil {
		return fmt.Errorf("%s not found: %w", sandboxExec, err)
	}
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		return errors.New("compare mode needs ANTHROPIC_API_KEY; agents never see it, the metering proxy adds it to each request")
	}
	model := apiclient.ResolveModelAlias(cfg.Model)
	price, priced := priceFor(model, 0)
	if !strings.HasPrefix(model, "claude-") || !priced {
		return fmt.Errorf("compare mode meters Anthropic's API and needs a priced claude-* model; got %q", model)
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if real, err := filepath.EvalSymlinks(userHome); err == nil {
		userHome = real
	}

	root, err := os.MkdirTemp("", "gocode-compare-")
	if err != nil {
		return err
	}
	if root, err = filepath.EvalSymlinks(root); err != nil {
		return err
	}
	if cfg.Keep {
		fmt.Fprintf(os.Stderr, "scratch root kept at %s\n", root)
	} else {
		defer os.RemoveAll(root)
	}
	shared := sharedDirs{
		homes:      filepath.Join(root, "homes"),
		gocache:    filepath.Join(root, "gocache"),
		gomodcache: filepath.Join(root, "gomodcache"),
		gopath:     filepath.Join(root, "gopath"),
	}
	for _, d := range []string{shared.homes, shared.gocache, shared.gomodcache, shared.gopath} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}

	m, err := newMeter("https://api.anthropic.com", key, cfg.BudgetUSD)
	if err != nil {
		return err
	}
	defer m.Close()

	started := time.Now().UTC()
	runDir := filepath.Join(cfg.OutDir, started.Format("20060102-150405")+"_compare_"+sanitize(model))
	logDir := filepath.Join(runDir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return err
	}

	path, readable := agentPATH(cfg.AgentPath, cfg.Agents, userHome)
	profile := sandboxProfile(userHome, readable)
	catalog := cfg.Catalog
	if catalog != "" {
		if catalog, err = filepath.Abs(catalog); err != nil {
			return err
		}
	}

	var agents []*cliAgent
	for _, s := range cfg.Agents {
		a := &cliAgent{
			Name: s.Name, Binary: s.Binary, Model: model, MaxTurns: cfg.MaxTurns,
			meter: m, shared: shared, path: path, profile: profile,
			logDir: logDir, catalog: catalog, keep: cfg.Keep,
		}
		a.Version = a.version()
		fmt.Fprintf(os.Stderr, "%s: %s (%s)\n", a.Name, a.Version, a.Binary)
		agents = append(agents, a)
	}

	fmt.Fprintln(os.Stderr, "warming the shared Go build cache")
	warmGoCache(ctx, cfg.Tasks, agents[0].baseEnv(filepath.Join(root, "warm-home"), ""))

	fmt.Fprintf(os.Stderr, "model %s, %d tasks, %d agent(s), %d run(s), parallel=%d per agent, budget $%.2f\n",
		model, len(cfg.Tasks), len(agents), cfg.Runs, cfg.Parallel, cfg.BudgetUSD)
	byAgent := map[string][]Result{}
	for run := 1; run <= cfg.Runs && ctx.Err() == nil && !m.Exhausted(); run++ {
		var wg sync.WaitGroup
		var mu sync.Mutex
		for _, a := range agents {
			wg.Add(1)
			go func(a *cliAgent) {
				defer wg.Done()
				rs := runAgentTasks(ctx, a, cfg.Tasks, run, cfg.Parallel, cfg.Keep, m)
				mu.Lock()
				byAgent[a.Name] = append(byAgent[a.Name], rs...)
				mu.Unlock()
			}(a)
		}
		wg.Wait()
	}

	cf := CompareFile{
		Model: model, StartedAt: started, FinishedAt: time.Now().UTC(),
		Runs: cfg.Runs, MaxTurns: cfg.MaxTurns, BudgetUSD: cfg.BudgetUSD, SpentUSD: m.Spent(),
		Price: price, SandboxProfile: strings.ReplaceAll(profile, userHome, "~"),
	}
	for _, a := range agents {
		cf.Agents = append(cf.Agents, AgentInfo{Name: a.Name, Version: a.Version, Command: a.args("<workdir>", "<task prompt>")})
		cf.Results = append(cf.Results, byAgent[a.Name]...)
	}
	if err := writeJSONFile(filepath.Join(runDir, "results.json"), cf); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(runDir, "report.md"), []byte(renderComparison(cf, cfg.Tasks, true)), 0o644); err != nil {
		return err
	}
	fmt.Print(renderComparison(cf, cfg.Tasks, false))
	fmt.Fprintf(os.Stderr, "results, report and per-run logs in %s\n", runDir)
	return nil
}

func runAgentTasks(ctx context.Context, a *cliAgent, tasks []Task, run, parallel int, keep bool, m *meter) []Result {
	results := make([]Result, len(tasks))
	sem := make(chan struct{}, max(parallel, 1))
	var wg sync.WaitGroup
	for i, t := range tasks {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, t Task) {
			defer wg.Done()
			defer func() { <-sem }()
			if ctx.Err() != nil || m.Exhausted() {
				results[i] = Result{Task: t.ID, Model: a.Model, Agent: a.Name, Run: run, Skipped: true, Error: "skipped: interrupted or over budget"}
				return
			}
			fmt.Fprintf(os.Stderr, "[%s run %d] %s: running\n", a.Name, run, t.ID)
			tctx := context.WithValue(context.WithValue(ctx, taskCtxKey{}, t.ID), runNumCtxKey{}, run)
			r := RunTask(tctx, t, a.Model, a, keep)
			r.Agent, r.Run = a.Name, run
			results[i] = r
			mark := "FAIL"
			if r.Passed {
				mark = "pass"
			}
			if r.Error != "" {
				mark += " (" + firstLine(r.Error, 120) + ")"
			}
			fmt.Fprintf(os.Stderr, "[%s run %d] %s: %s %.0fs $%.3f, $%.2f spent so far\n", a.Name, run, t.ID, mark, r.Duration, r.CostUSD, m.Spent())
		}(i, t)
	}
	wg.Wait()
	return results
}

// cliAgent drives one agent CLI in its headless mode. It implements Agent.
type cliAgent struct {
	Name, Binary, Version, Model string
	MaxTurns                     int

	meter   *meter
	shared  sharedDirs
	path    string
	profile string
	logDir  string
	catalog string
	keep    bool
}

func (a *cliAgent) Run(ctx context.Context, workdir, prompt string) (Usage, error) {
	task, _ := ctx.Value(taskCtxKey{}).(string)
	run, _ := ctx.Value(runNumCtxKey{}).(int)
	home, err := a.newHome(fmt.Sprintf("%s-%s-r%d-", a.Name, task, run))
	if err != nil {
		return Usage{}, err
	}
	if !a.keep {
		defer os.RemoveAll(home)
	}
	if err := gitInit(workdir, a.baseEnv(home, "")); err != nil {
		return Usage{}, err
	}

	token := a.meter.newRun()
	extra, err := a.setup(home, token)
	if err != nil {
		a.meter.finishRun(token)
		return Usage{}, err
	}
	logPath := filepath.Join(a.logDir, fmt.Sprintf("%s_%s_run%d.log", a.Name, task, run))
	logFile, err := os.Create(logPath)
	if err != nil {
		a.meter.finishRun(token)
		return Usage{}, err
	}

	cmd := exec.CommandContext(ctx, sandboxExec, append([]string{"-p", a.profile, a.Binary}, a.args(workdir, prompt)...)...)
	cmd.Dir = workdir
	cmd.Env = append(a.baseEnv(home, token), extra...)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	// Its own process group, so a timeout also takes down the language
	// servers and shells the agent started.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 10 * time.Second
	runErr := cmd.Run()
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	logFile.Close()

	mu := a.meter.finishRun(token)
	usage := Usage{
		InputTokens: mu.InputTokens, OutputTokens: mu.OutputTokens,
		Metered: true, CostUSD: mu.CostUSD, Requests: mu.Requests,
		CacheReadTokens: mu.CacheRead, CacheWriteTokens: mu.CacheWrite, UnpricedModels: mu.Unpriced,
	}
	switch {
	case ctx.Err() != nil:
		return usage, errors.New("agent timed out")
	case runErr != nil:
		return usage, fmt.Errorf("%s exited: %v: %s", a.Name, runErr, lastLines(logPath, 3))
	case mu.Requests == 0:
		return usage, fmt.Errorf("%s made no metered API calls (%d HTTP errors): %s", a.Name, mu.HTTPErrors, lastLines(logPath, 3))
	}
	return usage, nil
}

// args is the agent's headless command line: the task prompt, the model,
// and whatever flag the agent needs to run tools without asking.
func (a *cliAgent) args(workdir, prompt string) []string {
	turns := fmt.Sprint(a.MaxTurns)
	switch a.Name {
	case "gocode":
		return []string{"prompt", "--model", a.Model, "--max-turns", turns, "--no-stream", "--output-format", "json", prompt}
	case "claurst":
		return []string{"--print", "--model", a.Model, "--permission-mode", "bypass-permissions", "--max-turns", turns, "--cwd", workdir, prompt}
	case "crush":
		// crush run approves every tool call in its session by itself.
		return []string{"run", "--quiet", "--model", "anthropic/" + a.Model, "--cwd", workdir, prompt}
	case "opencode":
		return []string{"run", "--model", "anthropic/" + a.Model, "--auto", prompt}
	}
	return nil
}

// setup writes the agent's config into its fresh HOME, pointing it at the
// metering proxy with the run's key, and returns extra environment.
func (a *cliAgent) setup(home, token string) ([]string, error) {
	base := a.meter.URL
	switch a.Name {
	case "gocode":
		return []string{"ANTHROPIC_BASE_URL=" + base}, nil
	case "claurst":
		dir := filepath.Join(home, ".claurst")
		err := writeJSONFile(filepath.Join(dir, "settings.json"), map[string]any{
			"version":                1,
			"provider":               "anthropic",
			"hasCompletedOnboarding": true,
			"providers": map[string]any{
				"anthropic": map[string]any{"api_key": token, "api_base": base, "enabled": true},
			},
			// Seatbelt profiles do not nest: inside ours, claurst's own shell
			// sandbox fails every command with exit 71. It runs under the
			// same outer profile as the other agents instead.
			"sandbox": map[string]any{"mode": "off", "network": false, "unsandboxedPrefixes": []string{}},
		})
		env := []string{"CLAURST_HOME=" + dir, "ANTHROPIC_BASE_URL=" + base, "CLAURST_DISABLE_MODELS_FETCH=1"}
		if a.catalog != "" {
			env = append(env, "CLAURST_MODELS_PATH="+a.catalog)
		}
		return env, err
	case "crush":
		err := writeJSONFile(filepath.Join(home, ".config", "crush", "crush.json"), map[string]any{
			"providers": map[string]any{
				"anthropic": map[string]any{"api_key": token, "base_url": base},
			},
			"options": map[string]any{"disable_provider_auto_update": true, "disable_metrics": true},
		})
		return []string{"ANTHROPIC_BASE_URL=" + base, "CRUSH_DISABLE_METRICS=1", "CRUSH_DISABLE_PROVIDER_AUTO_UPDATE=1"}, err
	case "opencode":
		cfg, err := json.Marshal(map[string]any{
			"$schema":    "https://opencode.ai/config.json",
			"autoupdate": false,
			"share":      "disabled",
			"provider": map[string]any{
				"anthropic": map[string]any{"options": map[string]any{"baseURL": base + "/v1", "apiKey": token}},
			},
		})
		return []string{
			"OPENCODE_CONFIG_CONTENT=" + string(cfg),
			"ANTHROPIC_BASE_URL=" + base + "/v1",
			"OPENCODE_DISABLE_AUTOUPDATE=1",
			"OPENCODE_DISABLE_MODELS_FETCH=1",
			"OPENCODE_DISABLE_LSP_DOWNLOAD=1",
		}, err
	}
	return nil, fmt.Errorf("unknown agent %q", a.Name)
}

// baseEnv is the whole environment an agent gets: nothing is inherited, so
// no real API key or shell setup leaks in. Go uses caches shared across runs
// under the scratch root, and never the network.
func (a *cliAgent) baseEnv(home, apiKey string) []string {
	env := []string{
		"HOME=" + home,
		"USER=" + os.Getenv("USER"),
		"LOGNAME=" + os.Getenv("LOGNAME"),
		"PATH=" + a.path,
		"SHELL=/bin/zsh",
		"TMPDIR=" + filepath.Join(home, "tmp"),
		"LANG=en_US.UTF-8",
		"TERM=dumb",
		"NO_COLOR=1",
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"),
		"XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"GOCACHE=" + a.shared.gocache,
		"GOMODCACHE=" + a.shared.gomodcache,
		"GOPATH=" + a.shared.gopath,
		"GOENV=off",
		"GOTOOLCHAIN=local",
		"GOPROXY=off",
		"GOSUMDB=off",
		"GOWORK=off",
		"GOFLAGS=",
		"DO_NOT_TRACK=1",
	}
	if apiKey != "" {
		env = append(env, "ANTHROPIC_API_KEY="+apiKey)
	}
	return env
}

func (a *cliAgent) newHome(prefix string) (string, error) {
	home, err := os.MkdirTemp(a.shared.homes, prefix)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Join(home, "tmp"), 0o755); err != nil {
		return "", err
	}
	gitconfig := "[user]\n\tname = eval\n\temail = eval@localhost\n[init]\n\tdefaultBranch = main\n"
	return home, os.WriteFile(filepath.Join(home, ".gitconfig"), []byte(gitconfig), 0o644)
}

func (a *cliAgent) version() string {
	home, err := a.newHome(a.Name + "-version-")
	if err != nil {
		return "unknown"
	}
	defer os.RemoveAll(home)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, a.Binary, "--version")
	cmd.Env = a.baseEnv(home, "")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return firstLine(strings.TrimSpace(string(out)), 80)
}

// gitInit commits the fixture, so every agent starts in a repository as it
// would on a real project.
func gitInit(dir string, env []string) error {
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}, {"commit", "-q", "--no-verify", "-m", "fixture"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(slices.Clone(env), "GIT_CONFIG_NOSYSTEM=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %s: %v: %s", args[0], err, bytes.TrimSpace(out))
		}
	}
	return nil
}

// agentPATH builds the agents' PATH from the extra directories plus wherever
// go, gopls, git and rg live, and lists the ones inside the user's home that
// the sandbox must leave readable.
func agentPATH(extra []string, specs []agentSpec, userHome string) (string, []string) {
	var dirs []string
	add := func(d string) {
		if real, err := filepath.EvalSymlinks(d); err == nil {
			d = real
		}
		if d != "" && !slices.Contains(dirs, d) {
			dirs = append(dirs, d)
		}
	}
	for _, d := range extra {
		if abs, err := filepath.Abs(d); err == nil {
			add(abs)
		}
	}
	for _, tool := range []string{"go", "gopls", "git", "rg"} {
		if p, err := exec.LookPath(tool); err == nil {
			add(filepath.Dir(p))
		}
	}
	for _, d := range []string{"/usr/bin", "/bin", "/usr/sbin", "/sbin"} {
		add(d)
	}
	var readable []string
	keep := func(p string) {
		if real, err := filepath.EvalSymlinks(p); err == nil {
			p = real
		}
		if within(p, userHome) && !slices.Contains(readable, p) {
			readable = append(readable, p)
		}
	}
	for _, d := range dirs {
		keep(d)
	}
	for _, s := range specs {
		if s.Binary != "" {
			keep(s.Binary)
		}
	}
	if out, err := exec.Command("go", "env", "GOROOT").Output(); err == nil {
		keep(strings.TrimSpace(string(out)))
	}
	return strings.Join(dirs, ":"), readable
}

func within(p, root string) bool {
	return p == root || strings.HasPrefix(p, root+string(filepath.Separator))
}

// sandboxProfile hides the user's home from the agent, for reads and writes,
// apart from the tool directories it needs, and blocks the network apart
// from loopback, where the metering proxy listens. The scratch workdir, the
// agent's HOME and the shared Go caches all live in the temp directory.
func sandboxProfile(userHome string, readable []string) string {
	var b strings.Builder
	b.WriteString("(version 1)\n(allow default)\n")
	fmt.Fprintf(&b, "(deny file-write* (subpath %q))\n", userHome)
	fmt.Fprintf(&b, "(deny file-read* (subpath %q))\n", userHome)
	for _, p := range readable {
		fmt.Fprintf(&b, "(allow file-read* (subpath %q))\n", p)
	}
	b.WriteString("(deny network-outbound)\n")
	b.WriteString("(allow network-outbound (remote ip \"localhost:*\"))\n")
	return b.String()
}

// warmGoCache compiles every fixture once with the shared cache, with each
// task's test flags too, so no agent pays for building the standard library.
func warmGoCache(ctx context.Context, tasks []Task, env []string) {
	for _, t := range tasks {
		dir, err := os.MkdirTemp("", "gocode-compare-warm-")
		if err != nil {
			return
		}
		if copyTree(filepath.Join(t.Dir, "fixture"), dir, nil) == nil {
			for _, extra := range [][]string{nil, t.TestArgs} {
				args := append([]string{"test", "-count=1", "-run", "^$", "./..."}, extra...)
				cmd := exec.CommandContext(ctx, "go", args...)
				cmd.Dir, cmd.Env = dir, env
				_ = cmd.Run() // an unfinished fixture may not compile; the stdlib still gets cached
			}
		}
		os.RemoveAll(dir)
	}
}

func writeJSONFile(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func lastLines(path string, n int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(tail(string(data), n), "\n")
	for i, l := range lines {
		lines[i] = firstLine(l, 200)
	}
	return strings.Join(lines, " | ")
}

func firstLine(s string, limit int) string {
	s, _, _ = strings.Cut(s, "\n")
	if utf8.RuneCountInString(s) > limit {
		s = string([]rune(s)[:limit]) + "..."
	}
	return s
}

type agentStats struct {
	Attempted, Passed, Errors, Skipped int
	Cost                               float64
	Requests, In, CacheRead, CacheW    int
	Out                                int
	Durations                          []float64
	Unpriced                           []string
}

func statsFor(rs []Result) agentStats {
	var s agentStats
	for _, r := range rs {
		if r.Skipped {
			s.Skipped++
			continue
		}
		s.Attempted++
		if r.Passed {
			s.Passed++
		}
		if r.Error != "" {
			s.Errors++
		}
		s.Cost += r.CostUSD
		s.Requests += r.Requests
		s.In += r.InputTokens
		s.CacheRead += r.CacheReadTokens
		s.CacheW += r.CacheWriteTokens
		s.Out += r.OutputTokens
		s.Durations = append(s.Durations, r.Duration)
		for _, m := range r.UnpricedModels {
			if !slices.Contains(s.Unpriced, m) {
				s.Unpriced = append(s.Unpriced, m)
			}
		}
	}
	return s
}

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := slices.Clone(xs)
	slices.Sort(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

func taskCell(rs []Result) string {
	s := statsFor(rs)
	if s.Attempted == 0 {
		return "skipped"
	}
	mark := fmt.Sprintf("%d/%d", s.Passed, s.Attempted)
	if s.Attempted == 1 {
		mark = "✗"
		if s.Passed == 1 {
			mark = "✓"
		}
	}
	cell := fmt.Sprintf("%s %.0fs $%.3f", mark, median(s.Durations), s.Cost/float64(s.Attempted))
	if s.Errors > 0 {
		cell += " !"
	}
	return cell
}

// renderComparison prints tasks down and agents across, then one summary
// row per measure, as aligned text or as a Markdown report.
func renderComparison(cf CompareFile, tasks []Task, markdown bool) string {
	byAgent := map[string][]Result{}
	for _, r := range cf.Results {
		byAgent[r.Agent] = append(byAgent[r.Agent], r)
	}
	header := []string{"task"}
	for _, a := range cf.Agents {
		header = append(header, a.Name)
	}
	rows := [][]string{header}
	for _, t := range tasks {
		row := []string{t.ID}
		for _, a := range cf.Agents {
			var rs []Result
			for _, r := range byAgent[a.Name] {
				if r.Task == t.ID {
					rs = append(rs, r)
				}
			}
			row = append(row, taskCell(rs))
		}
		rows = append(rows, row)
	}
	summary := func(label string, f func(s agentStats) string) {
		row := []string{label}
		for _, a := range cf.Agents {
			row = append(row, f(statsFor(byAgent[a.Name])))
		}
		rows = append(rows, row)
	}
	summary("pass rate", func(s agentStats) string {
		if s.Attempted == 0 {
			return "-"
		}
		return fmt.Sprintf("%d/%d (%.0f%%)", s.Passed, s.Attempted, 100*float64(s.Passed)/float64(s.Attempted))
	})
	summary("$ per solved task", func(s agentStats) string {
		if s.Passed == 0 {
			return "n/a"
		}
		return fmt.Sprintf("$%.3f", s.Cost/float64(s.Passed))
	})
	summary("total cost", func(s agentStats) string { return fmt.Sprintf("$%.2f", s.Cost) })
	summary("median time per task", func(s agentStats) string { return fmt.Sprintf("%.0fs", median(s.Durations)) })
	summary("API requests", func(s agentStats) string { return fmt.Sprint(s.Requests) })
	summary("tokens in / cache read / cache write / out", func(s agentStats) string {
		return fmt.Sprintf("%s / %s / %s / %s", human(s.In), human(s.CacheRead), human(s.CacheW), human(s.Out))
	})
	summary("agent errors", func(s agentStats) string {
		out := fmt.Sprint(s.Errors)
		if s.Skipped > 0 {
			out += fmt.Sprintf(", %d skipped", s.Skipped)
		}
		if len(s.Unpriced) > 0 {
			out += ", unpriced: " + strings.Join(s.Unpriced, " ")
		}
		return out
	})

	var b strings.Builder
	if markdown {
		fmt.Fprintf(&b, "# Agent comparison on %s\n\n", cf.Model)
		fmt.Fprintf(&b, "%s · %d tasks × %d run(s) · scored by hidden tests · cost metered at the API · $%.2f spent\n\n",
			cf.StartedAt.Format("2006-01-02"), len(tasks), cf.Runs, cf.SpentUSD)
	} else {
		fmt.Fprintf(&b, "%s, %d tasks × %d run(s), $%.2f spent\n", cf.Model, len(tasks), cf.Runs, cf.SpentUSD)
	}
	b.WriteString(formatTable(rows, markdown))
	b.WriteString("\nEach cell: pass mark (or passes/runs), median wall time, mean cost per attempt. ! = the agent process failed or timed out; it was still scored on the files it left.\n")
	if !markdown {
		return b.String()
	}
	p := cf.Price
	b.WriteString("\n## How this was measured\n\n")
	for _, a := range cf.Agents {
		fmt.Fprintf(&b, "- **%s** %s: `%s`\n", a.Name, a.Version, strings.Join(a.Command, " "))
	}
	fmt.Fprintf(&b, "- One model for every agent: `%s`, each agent at its own defaults otherwise. Turn caps: %d where the agent has a flag for it; every task has a %s wall-clock limit from its task.json.\n",
		cf.Model, cf.MaxTurns, "300 s")
	b.WriteString("- Every run starts from a fresh HOME and a fresh git repo holding the fixture, with full tool auto-approval, under one macOS sandbox profile for all agents: no reads or writes under the user's home except the Go tool directories, no network except loopback.\n")
	b.WriteString("- Scoring: visible tests are restored after the agent finishes, hidden tests it never saw are added, then `go test ./...` (with `-race` where the task asks) decides pass or fail.\n")
	fmt.Fprintf(&b, "- Cost: every API request each agent made, including title, summary and sub-agent calls, passed through one local proxy that read the usage the API returned and priced it at Anthropic list price (per million tokens for %s: input $%.2f, output $%.2f, 5-minute cache write $%.2f, 1-hour cache write $%.2f, cache read $%.2f). Agents never saw the real key.\n",
		cf.Model, p.Input, p.Output, p.CacheWrite5m, p.CacheWrite1h, p.CacheRead)
	b.WriteString("- claurst's own shell sandbox was off, because macOS sandbox profiles do not nest; it ran under the same outer profile as the others.\n")
	b.WriteString("- Fifteen small Go tasks is a smoke test, not a benchmark. Re-run with `-runs 3` before trusting a gap of one or two tasks.\n")
	return b.String()
}

func formatTable(rows [][]string, markdown bool) string {
	var b strings.Builder
	if markdown {
		for i, r := range rows {
			b.WriteString("| " + strings.Join(r, " | ") + " |\n")
			if i == 0 {
				seps := make([]string, len(r))
				for j := range seps {
					seps[j] = "---"
				}
				b.WriteString("|" + strings.Join(seps, "|") + "|\n")
			}
		}
		return b.String()
	}
	widths := make([]int, len(rows[0]))
	for _, r := range rows {
		for j, c := range r {
			widths[j] = max(widths[j], utf8.RuneCountInString(c))
		}
	}
	for _, r := range rows {
		for j, c := range r {
			if j > 0 {
				b.WriteString("  ")
			}
			b.WriteString(c + strings.Repeat(" ", widths[j]-utf8.RuneCountInString(c)))
		}
		b.WriteString("\n")
	}
	return b.String()
}
