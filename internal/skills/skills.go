// Package skills provides a skill loader and activator for domain-tuned agent profiles.
// Skills are JSON files containing a name, system prompt, tool permission list, and
// optional MCP server configurations. Built-in skills (git-master, frontend-ui-ux, etc.)
// are always available; user-defined skills are loaded from .gocode/skills/.
package skills

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/AlleyBo55/gocode/internal/agent"
)

// Skill is a domain-tuned agent profile loaded from JSON.
type Skill struct {
	Name         string            `json:"name"`
	SystemPrompt string            `json:"system_prompt"`
	ToolPerms    []string          `json:"tool_permissions"`
	MCPServers   []MCPServerConfig `json:"mcp_servers,omitempty"`
}

// MCPServerConfig defines an MCP server to start as a child process.
type MCPServerConfig struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// SkillConfig holds the configuration returned by Activate so the caller
// can create or reconfigure a ConversationRuntime accordingly.
type SkillConfig struct {
	SystemPrompt string
	ToolPerms    []string
}

// SkillLoader loads and validates skills from a directory and built-in definitions.
type SkillLoader struct {
	dir      string
	builtins map[string]Skill
	loaded   []Skill
}

// NewSkillLoader creates a loader with built-in skills.
// If dir is empty, it defaults to ".gocode/skills/".
func NewSkillLoader(dir string) *SkillLoader {
	if dir == "" {
		dir = ".gocode/skills/"
	}
	return &SkillLoader{
		dir:      dir,
		builtins: defaultBuiltins(),
	}
}

// allFileTools is the standard set of file-related tool permissions.
var allFileTools = []string{
	"bashtool", "filereadtool", "fileedittool", "filewritetool",
	"globtool", "greptool",
}

// allTools is the full set of tool permissions (file tools + web/mcp).
var allTools = []string{
	"bashtool", "filereadtool", "fileedittool", "filewritetool",
	"globtool", "greptool", "webtool", "mcptool",
}

// defaultBuiltins returns the built-in skill definitions.
func defaultBuiltins() map[string]Skill {
	return map[string]Skill{
		"git-master": {
			Name: "git-master",
			SystemPrompt: "You are a Git expert agent. Focus on atomic commits with clear messages, " +
				"interactive rebase for clean history, and safe branch management. " +
				"Always verify the current branch and status before making changes. " +
				"Prefer small, focused commits over large monolithic ones.",
			ToolPerms: []string{"bashtool", "filereadtool", "fileedittool"},
		},
		"frontend-ui-ux": {
			Name: "frontend-ui-ux",
			SystemPrompt: "You are a frontend UI/UX expert agent. Follow a design-first approach: " +
				"understand the visual requirements before writing code. " +
				"Prioritize accessibility, responsive design, and consistent styling. " +
				"Use semantic HTML and modern CSS patterns.",
			ToolPerms: []string{"filereadtool", "fileedittool", "filewritetool", "globtool", "greptool"},
		},
		"nothing-design": {
			Name: "nothing-design",
			SystemPrompt: `You are a Nothing design system expert. Apply these strict principles:

VISUAL IDENTITY:
- Monochrome only. Color is an event, not a default — use it only for critical alerts or active states.
- OLED blacks (#000000) as primary background. Pure white (#FFFFFF) for primary text.
- No gradients, no shadows, no blur effects. Every pixel must be intentional.

TYPOGRAPHY (three-layer hierarchy):
- Display: Space Grotesk, 48-72px, weight 700, letter-spacing -0.02em
- Body: Space Grotesk, 14-16px, weight 400, line-height 1.5
- Metadata: Space Mono, 11-12px, weight 400, uppercase, letter-spacing 0.08em

LAYOUT & COMPONENTS:
- Grid-based layouts with mathematical spacing (8px base unit).
- Segmented progress bars (discrete steps, not continuous fills).
- Mechanical toggles with hard snap states — no spring animations.
- Dot-matrix patterns for decorative elements. Circular icon containers.
- No skeleton loaders — use opacity transitions or segmented loading indicators.

PHILOSOPHY:
- Subtract, don't add. Structure is the ornament.
- Every element must earn its place. If removing it doesn't break function, remove it.
- Interfaces should feel like precision instruments, not friendly companions.

OUTPUT FORMATS: HTML/CSS, React with Tailwind CSS, or SwiftUI as appropriate.`,
			ToolPerms: append([]string{}, allFileTools...),
		},
		"golang-best-practices": {
			Name: "golang-best-practices",
			SystemPrompt: `You are a Go best practices expert. Apply these guidelines to all Go code:

CODE STYLE:
- Use := for non-zero-value initialization, var for zero-value declarations.
- Early returns to reduce nesting. Guard clauses at function top.
- Switch over if-else chains when comparing a value against multiple options.
- Maximum 4 parameters per function; use an options struct beyond that.
- Range loops over index-based iteration. Prefer for range with Go 1.22+ integer ranges.
- Keep functions short and focused. One function, one responsibility.

ERROR HANDLING:
- Always check errors. Never use _ for error returns unless explicitly justified.
- Wrap errors with fmt.Errorf("context: %w", err) for stack context.
- Use errors.Is() and errors.As() for error comparison, never == on wrapped errors.
- Single handling rule: log OR return an error, never both.
- Error strings are lowercase, no punctuation. Example: "opening config file: %w"
- Sentinel errors (var ErrNotFound = errors.New("not found")) for expected conditions.
- Use slog for structured logging. Never log.Fatal in library code.
- Never panic for expected errors. Reserve panic for truly unrecoverable programmer bugs.

NAMING:
- MixedCaps (exported) and mixedCaps (unexported). No underscores in Go names.
- Short receiver names (1-2 chars): func (s *Server) Handle(...)
- Interface names use -er suffix when single-method: Reader, Writer, Closer.
- Package names are short, lowercase, singular: config not configs, util not utils.

TESTING:
- Table-driven tests with named subtests: for _, tt := range tests { t.Run(tt.name, ...) }
- Use testify/assert and testify/require for clear assertions.
- Test file next to source: foo.go -> foo_test.go.
- Use t.Helper() in test helper functions. Use t.Cleanup() for teardown.
- Test behavior, not implementation. Name tests TestFunction_Scenario_Expected.`,
			ToolPerms: []string{"bashtool", "filereadtool", "fileedittool", "filewritetool", "greptool", "globtool"},
		},
		"clone-website": {
			Name: "clone-website",
			SystemPrompt: `You are a website cloning specialist. Reverse-engineer and rebuild websites as pixel-perfect clones.

ANALYSIS PHASE:
- Extract CSS via getComputedStyle on every visible element. Capture all custom properties.
- Download all assets: fonts (check @font-face), images, SVGs, favicons.
- Identify interaction models: click-driven, scroll-driven, hover states, transitions.
- Extract EVERY state — hover, focus, active, disabled, loading, error — not just defaults.
- Map responsive breakpoints by resizing viewport and capturing layout shifts.
- Document the component hierarchy before writing any code.

BUILD PROCESS (strict order):
1. Foundation: global CSS reset, CSS custom properties for colors/spacing, font loading, base typography.
2. Components: build each component in isolation. Match padding, margin, border-radius exactly.
3. Assembly: compose components into page layouts. Verify spacing between sections.
4. Interactions: add hover states, transitions, scroll behaviors, animations.
5. Visual QA: side-by-side comparison at each breakpoint. Fix pixel-level discrepancies.

TECH STACK:
- Next.js App Router + TypeScript for structure.
- Tailwind CSS for utility classes, with custom theme config matching the source design tokens.
- shadcn/ui as component primitives where applicable.
- Framer Motion for complex animations.

RULES:
- Requires browser MCP tool for live page inspection.
- Never approximate — measure exact values. Use computed styles, not guesses.
- Preserve original class naming patterns in comments for reference.
- All images must have proper alt text and dimensions to prevent layout shift.`,
			ToolPerms: append([]string{}, allTools...),
		},
		"nextjs-best-practices": {
			Name: "nextjs-best-practices",
			SystemPrompt: `You are a Next.js expert. Apply these best practices to all Next.js code:

FILE CONVENTIONS:
- page.tsx, layout.tsx, loading.tsx, error.tsx, not-found.tsx, route.ts are special files.
- Colocate components in the same route folder or a shared _components directory.
- Use route groups (parentheses) for organization without affecting URL structure.

REACT SERVER COMPONENTS (RSC):
- Components are Server Components by default. Only add 'use client' when needed (hooks, event handlers, browser APIs).
- Never make async client components. Async data fetching belongs in Server Components.
- Props passed from Server to Client components must be serializable (no functions, no classes).
- Use composition: pass Server Components as children to Client Components.

NEXT.JS 15+ SPECIFICS:
- params, searchParams, cookies(), and headers() are now async — always await them.
- Use 'use cache' directive for cacheable server functions.
- 'use server' marks server actions. Place at top of function or top of file.

ERROR HANDLING:
- error.tsx catches errors in a route segment. Must be a Client Component.
- global-error.tsx catches root layout errors. Wrap with its own html/body tags.
- not-found.tsx for 404 states. Trigger with notFound() function.

DATA PATTERNS:
- Avoid request waterfalls: use Promise.all() for parallel fetches.
- Wrap slow data in Suspense boundaries with meaningful fallbacks.
- Use React.cache() or Next.js unstable_cache() for request deduplication.
- Route handlers (route.ts): GET handler conflicts with page.tsx in same folder — avoid this.

OPTIMIZATION:
- Always use next/image over <img>. Set width/height or fill prop.
- Use next/font for font loading — no external stylesheet requests.
- Analyze bundle with @next/bundle-analyzer. Use next/dynamic for heavy client components.
- Prefer server-side data fetching over client-side useEffect patterns.`,
			ToolPerms: append([]string{}, allTools...),
		},
		"react-best-practices": {
			Name: "react-best-practices",
			SystemPrompt: `You are a React performance optimization expert. Apply these patterns:

ELIMINATE WATERFALLS:
- Defer await: start fetches early, await late. Don't block rendering on sequential fetches.
- Use Promise.all() for independent data requirements.
- Wrap async boundaries with Suspense and provide meaningful fallback UI.
- Prefetch data on hover/focus for navigation targets.

BUNDLE SIZE:
- Import directly from module paths, not barrel files: import { Button } from './Button' not from './components'.
- Use next/dynamic or React.lazy() for heavy components (charts, editors, maps).
- Defer third-party scripts with next/script strategy="lazyOnload".
- Audit bundle regularly. Tree-shake unused exports.

SERVER-SIDE OPTIMIZATION:
- Use React.cache() to deduplicate identical requests within a render pass.
- Minimize serialization cost: don't pass large objects from Server to Client Components.
- Fetch data in parallel on the server. Use streaming for progressive rendering.

RE-RENDER OPTIMIZATION:
- Extract expensive subtrees into separate components that receive stable props.
- Use React.memo() only when profiling shows unnecessary re-renders.
- Functional setState: setCount(prev => prev + 1) instead of setCount(count + 1).
- Lazy state initialization: useState(() => expensiveComputation()) for costly defaults.
- Never define components inside other components — this recreates the component on every render.
- Use useTransition for non-urgent state updates (filtering, sorting large lists).
- useDeferredValue for expensive derived computations.

RENDERING PATTERNS:
- Use content-visibility: auto for long scrollable lists to skip off-screen rendering.
- Conditional rendering: prefer ternary (condition ? <A/> : <B/>) over && (avoids rendering 0/empty string).
- Key prop must be stable and unique — never use array index for dynamic lists.
- Virtualize lists with more than ~100 items (react-window, @tanstack/virtual).`,
			ToolPerms: append([]string{}, allTools...),
		},
		"web-design-guidelines": {
			Name: "web-design-guidelines",
			SystemPrompt: `You are a web interface design reviewer. Review UI code for compliance with these guidelines:

ACCESSIBILITY:
- Semantic HTML: use <nav>, <main>, <article>, <section>, <aside>, <header>, <footer> appropriately.
- All interactive elements must be keyboard accessible. Tab order must be logical.
- Focus management: visible focus indicators (min 2px outline), focus trap in modals, restore focus on close.
- ARIA attributes: aria-label for icon-only buttons, aria-expanded for toggles, aria-live for dynamic content.
- Color contrast: minimum 4.5:1 for normal text, 3:1 for large text (WCAG AA).
- Never rely on color alone to convey information — use icons, text, or patterns as secondary indicators.

RESPONSIVE DESIGN:
- Mobile-first approach. Base styles for small screens, enhance with min-width media queries.
- Touch targets: minimum 44x44px for interactive elements on touch devices.
- Fluid typography with clamp(): font-size: clamp(1rem, 2.5vw, 1.5rem).
- Test at 320px, 768px, 1024px, 1440px breakpoints minimum.

INTERACTION STATES:
- Every interactive element needs: default, hover, focus, active, disabled states.
- Loading states: show progress indication within 100ms of user action.
- Error states: inline validation with clear error messages near the input.
- Empty states: helpful messaging with a clear call to action.

ANIMATION & MOTION:
- Respect prefers-reduced-motion: disable non-essential animations.
- Transitions should be 150-300ms. Use ease-out for entrances, ease-in for exits.
- No animation on page load that blocks content visibility.

DARK MODE:
- Support prefers-color-scheme media query.
- Use CSS custom properties for theme colors. Never hardcode colors in components.
- Test contrast ratios in both light and dark themes.

FORMS:
- Labels must be associated with inputs (htmlFor/id or wrapping).
- Provide autocomplete attributes for common fields (name, email, address).
- Show validation state visually and with aria-invalid/aria-describedby.`,
			ToolPerms: append([]string{}, allTools...),
		},
	}
}

// GetSkill looks up a loaded skill by name. Must be called after LoadAll().
func (l *SkillLoader) GetSkill(name string) (Skill, bool) {
	for _, s := range l.loaded {
		if s.Name == name {
			return s, true
		}
	}
	return Skill{}, false
}

// LoadAll loads built-in skills plus user-defined skills from the configured directory.
// It returns all successfully loaded skills and a slice of errors for any files that
// failed to load or validate.
func (l *SkillLoader) LoadAll() ([]Skill, []error) {
	var skills []Skill
	var errs []error

	// Add built-in skills first.
	for _, s := range l.builtins {
		skills = append(skills, s)
	}

	// Load user-defined skills from directory.
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		// Directory not existing is fine — just return built-ins.
		if os.IsNotExist(err) {
			l.loaded = skills
			return skills, nil
		}
		l.loaded = skills
		return skills, []error{fmt.Errorf("reading skills directory %s: %w", l.dir, err)}
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(l.dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			errs = append(errs, fmt.Errorf("reading %s: %w", path, err))
			continue
		}

		var s Skill
		if err := json.Unmarshal(data, &s); err != nil {
			errs = append(errs, fmt.Errorf("parsing %s: %w", path, err))
			continue
		}

		if err := Validate(s); err != nil {
			errs = append(errs, fmt.Errorf("validating %s: %w", path, err))
			continue
		}

		// User-defined skills override built-ins with the same name.
		overridden := false
		for i, existing := range skills {
			if existing.Name == s.Name {
				skills[i] = s
				overridden = true
				break
			}
		}
		if !overridden {
			skills = append(skills, s)
		}
	}

	l.loaded = skills
	return skills, errs
}

// Validate checks a skill definition for required fields.
// Returns a descriptive error if any required field is missing or empty.
func Validate(s Skill) error {
	var missing []string
	if strings.TrimSpace(s.Name) == "" {
		missing = append(missing, "name")
	}
	if strings.TrimSpace(s.SystemPrompt) == "" {
		missing = append(missing, "system_prompt")
	}
	if len(s.ToolPerms) == 0 {
		missing = append(missing, "tool_permissions")
	}
	if len(missing) > 0 {
		return fmt.Errorf("skill missing required fields: %s", strings.Join(missing, ", "))
	}
	return nil
}

// SkillActivator manages MCP server lifecycle for activated skills and returns
// configuration for the caller to apply to a ConversationRuntime.
type SkillActivator struct {
	mu        sync.Mutex
	processes map[string][]*os.Process // skillName -> list of MCP server processes
}

// NewSkillActivator creates a new activator.
func NewSkillActivator() *SkillActivator {
	return &SkillActivator{
		processes: make(map[string][]*os.Process),
	}
}

// Activate starts any MCP servers defined by the skill and returns a SkillConfig
// that the caller should use to configure a ConversationRuntime (system prompt,
// tool permissions). The executor parameter is provided for future MCP tool
// registration; currently MCP servers are started but tool registration is
// handled externally via the MCP client manager.
func (a *SkillActivator) Activate(skill Skill, rt *agent.ConversationRuntime, executor agent.ToolExecutor) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Start MCP servers if any are configured.
	var procs []*os.Process
	for _, srv := range skill.MCPServers {
		proc, err := startMCPServer(srv)
		if err != nil {
			// Clean up any servers we already started for this skill.
			for _, p := range procs {
				_ = p.Kill()
			}
			return fmt.Errorf("starting MCP server %q for skill %q: %w", srv.Name, skill.Name, err)
		}
		procs = append(procs, proc)
	}

	a.processes[skill.Name] = procs
	return nil
}

// ActivateConfig returns the SkillConfig for a skill without starting MCP servers.
// Use this to obtain the system prompt and tool permissions for runtime configuration.
func (a *SkillActivator) ActivateConfig(skill Skill) SkillConfig {
	return SkillConfig{
		SystemPrompt: skill.SystemPrompt,
		ToolPerms:    skill.ToolPerms,
	}
}

// Deactivate stops all MCP servers associated with the given skill name
// and removes them from the process map.
func (a *SkillActivator) Deactivate(skillName string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	procs, ok := a.processes[skillName]
	if !ok {
		return nil
	}

	var errs []error
	for _, p := range procs {
		if err := p.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			errs = append(errs, fmt.Errorf("killing MCP process (pid %d): %w", p.Pid, err))
		}
	}
	delete(a.processes, skillName)

	if len(errs) > 0 {
		return fmt.Errorf("deactivating skill %q: %w", skillName, errors.Join(errs...))
	}
	return nil
}

// DeactivateAll stops all MCP servers for all active skills.
func (a *SkillActivator) DeactivateAll() error {
	a.mu.Lock()
	names := make([]string, 0, len(a.processes))
	for name := range a.processes {
		names = append(names, name)
	}
	a.mu.Unlock()

	var errs []error
	for _, name := range names {
		if err := a.Deactivate(name); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// startMCPServer spawns an MCP server child process from the given config.
func startMCPServer(cfg MCPServerConfig) (*os.Process, error) {
	cmd := exec.Command(cfg.Command, cfg.Args...)

	// Build environment: inherit current env + add skill-specific vars.
	cmd.Env = os.Environ()
	for k, v := range cfg.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	// MCP servers communicate over stdio, so we set up pipes.
	cmd.Stdin = nil  // will be connected by MCP client manager
	cmd.Stdout = nil // will be connected by MCP client manager
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("exec %s: %w", cfg.Command, err)
	}

	return cmd.Process, nil
}
