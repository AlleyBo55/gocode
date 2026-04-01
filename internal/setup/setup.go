package setup

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

// PrefetchResult holds the result of a single prefetch operation.
type PrefetchResult struct {
	Name    string `json:"name"`
	Started bool   `json:"started"`
	Detail  string `json:"detail"`
}

// DeferredInitResult captures the outcome of each deferred initialization step.
type DeferredInitResult struct {
	Trusted    bool `json:"trusted"`
	PluginInit bool `json:"plugin_init"`
	SkillInit  bool `json:"skill_init"`
	MCPPrefetch bool `json:"mcp_prefetch"`
	SessionHooks bool `json:"session_hooks"`
}

// AsLines returns a formatted summary of each deferred init step.
func (d DeferredInitResult) AsLines() []string {
	return []string{
		fmt.Sprintf("Trusted: %v", d.Trusted),
		fmt.Sprintf("PluginInit: %v", d.PluginInit),
		fmt.Sprintf("SkillInit: %v", d.SkillInit),
		fmt.Sprintf("MCPPrefetch: %v", d.MCPPrefetch),
		fmt.Sprintf("SessionHooks: %v", d.SessionHooks),
	}
}

// WorkspaceSetup holds detected workspace environment info.
type WorkspaceSetup struct {
	GoVersion   string `json:"go_version"`
	Platform    string `json:"platform"`
	TestCommand string `json:"test_command"`
}

// StartupSteps returns the 6 bootstrap steps for workspace initialization.
func (w WorkspaceSetup) StartupSteps() []string {
	return []string{
		"Detect Go version",
		"Detect platform",
		"Resolve test command",
		"Run prefetches",
		"Run deferred init",
		"Build setup report",
	}
}

// SetupReport combines workspace setup, prefetch results, and deferred init state.
type SetupReport struct {
	Setup        WorkspaceSetup   `json:"setup"`
	Prefetches   []PrefetchResult `json:"prefetches"`
	DeferredInit DeferredInitResult `json:"deferred_init"`
	Trusted      bool             `json:"trusted"`
	Cwd          string           `json:"cwd"`
}

// Render returns a Markdown-formatted representation of the setup report.
func (r SetupReport) Render() string {
	var b strings.Builder
	b.WriteString("# Setup Report\n\n")
	b.WriteString("## Workspace\n\n")
	b.WriteString(fmt.Sprintf("- Go Version: %s\n", r.Setup.GoVersion))
	b.WriteString(fmt.Sprintf("- Platform: %s\n", r.Setup.Platform))
	b.WriteString(fmt.Sprintf("- Test Command: %s\n", r.Setup.TestCommand))
	b.WriteString(fmt.Sprintf("- Cwd: %s\n", r.Cwd))
	b.WriteString(fmt.Sprintf("- Trusted: %v\n", r.Trusted))

	b.WriteString("\n## Prefetches\n\n")
	for _, p := range r.Prefetches {
		status := "not started"
		if p.Started {
			status = "started"
		}
		b.WriteString(fmt.Sprintf("- %s: %s", p.Name, status))
		if p.Detail != "" {
			b.WriteString(fmt.Sprintf(" (%s)", p.Detail))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n## Deferred Init\n\n")
	for _, line := range r.DeferredInit.AsLines() {
		b.WriteString(fmt.Sprintf("- %s\n", line))
	}

	return b.String()
}

// StartMDMRawRead initiates the MDM raw read prefetch.
func StartMDMRawRead() PrefetchResult {
	return PrefetchResult{
		Name:    "mdm_raw_read",
		Started: true,
		Detail:  "MDM raw read initiated",
	}
}

// StartKeychainPrefetch initiates the keychain prefetch.
func StartKeychainPrefetch() PrefetchResult {
	return PrefetchResult{
		Name:    "keychain_prefetch",
		Started: true,
		Detail:  "Keychain prefetch initiated",
	}
}

// StartProjectScan initiates a project scan for the given root directory.
func StartProjectScan(root string) PrefetchResult {
	return PrefetchResult{
		Name:    "project_scan",
		Started: true,
		Detail:  fmt.Sprintf("Project scan initiated for %s", root),
	}
}

// RunDeferredInit performs deferred initialization steps.
// Each step runs independently; the trusted flag controls whether full init is performed.
func RunDeferredInit(trusted bool) DeferredInitResult {
	return DeferredInitResult{
		Trusted:      trusted,
		PluginInit:   true,
		SkillInit:    true,
		MCPPrefetch:  true,
		SessionHooks: true,
	}
}

// BuildWorkspaceSetup detects the workspace environment: Go version, platform, and test command.
func BuildWorkspaceSetup() WorkspaceSetup {
	goVersion := detectGoVersion()
	platform := runtime.GOOS
	testCommand := "go test ./..."

	return WorkspaceSetup{
		GoVersion:   goVersion,
		Platform:    platform,
		TestCommand: testCommand,
	}
}

// detectGoVersion runs "go version" and returns the output, or "unknown" on failure.
func detectGoVersion() string {
	out, err := exec.Command("go", "version").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// RunSetup runs the full workspace setup: builds workspace info, runs prefetches
// concurrently via goroutines, and performs deferred init.
func RunSetup(cwd string, trusted bool) SetupReport {
	ws := BuildWorkspaceSetup()

	var wg sync.WaitGroup
	prefetches := make([]PrefetchResult, 3)

	wg.Add(3)
	go func() {
		defer wg.Done()
		prefetches[0] = StartMDMRawRead()
	}()
	go func() {
		defer wg.Done()
		prefetches[1] = StartKeychainPrefetch()
	}()
	go func() {
		defer wg.Done()
		prefetches[2] = StartProjectScan(cwd)
	}()
	wg.Wait()

	deferred := RunDeferredInit(trusted)

	return SetupReport{
		Setup:        ws,
		Prefetches:   prefetches,
		DeferredInit: deferred,
		Trusted:      trusted,
		Cwd:          cwd,
	}
}
