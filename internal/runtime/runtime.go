package runtime

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gocode/gocode/internal/commands"
	"github.com/gocode/gocode/internal/execution"
	"github.com/gocode/gocode/internal/history"
	"github.com/gocode/gocode/internal/queryengine"
	"github.com/gocode/gocode/internal/session"
	"github.com/gocode/gocode/internal/setup"
	"github.com/gocode/gocode/internal/systeminit"
	"github.com/gocode/gocode/internal/tools"
)

// RoutedMatch represents a prompt routing match against a command or tool.
type RoutedMatch struct {
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	SourceHint string `json:"source_hint"`
	Score      int    `json:"score"`
}

// RuntimeSession holds full session state after bootstrap.
type RuntimeSession struct {
	Prompt           string
	Engine           *queryengine.QueryEnginePort
	History          *history.HistoryLog
	RoutedMatches    []RoutedMatch
	SetupReport      setup.SetupReport
	SystemInitMessage string
}

// AsMarkdown returns a Markdown-formatted summary of the runtime session.
func (rs *RuntimeSession) AsMarkdown() string {
	var b strings.Builder
	b.WriteString("# Runtime Session\n\n")
	b.WriteString(fmt.Sprintf("Prompt: %s\n", rs.Prompt))
	b.WriteString(fmt.Sprintf("System Init Message Length: %d\n", len(rs.SystemInitMessage)))
	b.WriteString(fmt.Sprintf("Routed Matches: %d\n\n", len(rs.RoutedMatches)))
	for _, m := range rs.RoutedMatches {
		b.WriteString(fmt.Sprintf("- [%s] %s (score: %d, source: %s)\n", m.Kind, m.Name, m.Score, m.SourceHint))
	}

	b.WriteString("\n")
	b.WriteString(rs.History.Render())
	b.WriteString("\n")
	b.WriteString(rs.SetupReport.Render())

	return b.String()
}

// PortRuntime orchestrates prompt routing and session bootstrap.
type PortRuntime struct {
	CmdRegistry  *commands.CommandRegistry
	ToolRegistry *tools.ToolRegistry
	ExecRegistry *execution.ExecutionRegistry
	SessionStore *session.SessionStore
}

// NewPortRuntime creates a new PortRuntime with the given registries and session store.
func NewPortRuntime(
	cmdReg *commands.CommandRegistry,
	toolReg *tools.ToolRegistry,
	execReg *execution.ExecutionRegistry,
	sessionStore *session.SessionStore,
) *PortRuntime {
	return &PortRuntime{
		CmdRegistry:  cmdReg,
		ToolRegistry: toolReg,
		ExecRegistry: execReg,
		SessionStore: sessionStore,
	}
}

// RoutePrompt tokenizes the prompt and scores each registered command and tool
// by counting substring matches. Results are sorted by score descending and
// limited to the top `limit` entries.
func (pr *PortRuntime) RoutePrompt(prompt string, limit int) []RoutedMatch {
	tokens := tokenize(prompt)
	if len(tokens) == 0 {
		return nil
	}

	var matches []RoutedMatch

	// Score commands
	for _, cmd := range pr.CmdRegistry.FindCommands("", 0) {
		score := scoreEntry(tokens, cmd.Name, cmd.SourceHint)
		if score > 0 {
			matches = append(matches, RoutedMatch{
				Kind:       "command",
				Name:       cmd.Name,
				SourceHint: cmd.SourceHint,
				Score:      score,
			})
		}
	}

	// Score tools
	for _, tool := range pr.ToolRegistry.FindTools("", 0) {
		score := scoreEntry(tokens, tool.Name, tool.SourceHint)
		if score > 0 {
			matches = append(matches, RoutedMatch{
				Kind:       "tool",
				Name:       tool.Name,
				SourceHint: tool.SourceHint,
				Score:      score,
			})
		}
	}

	// Sort by score descending
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Score > matches[j].Score
	})

	// Limit results
	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}

	return matches
}

// BootstrapSession initializes all subsystems and returns a fully configured RuntimeSession.
func (pr *PortRuntime) BootstrapSession(prompt string, limit int) *RuntimeSession {
	// Run setup
	report := setup.RunSetup(".", true)

	// Build system init message
	sysInit := systeminit.BuildSystemInitMessage(pr.CmdRegistry, pr.ToolRegistry, true)

	// Route prompt
	routed := pr.RoutePrompt(prompt, limit)

	// Create query engine
	config := queryengine.NewDefaultConfig()
	engine := queryengine.FromWorkspace(config, pr.SessionStore)

	// Create history log
	hist := &history.HistoryLog{}
	hist.Append("bootstrap", "Session bootstrapped")

	return &RuntimeSession{
		Prompt:            prompt,
		Engine:            engine,
		History:           hist,
		RoutedMatches:     routed,
		SetupReport:       report,
		SystemInitMessage: sysInit,
	}
}

// tokenize splits a prompt into lowercase tokens by whitespace.
func tokenize(prompt string) []string {
	fields := strings.Fields(prompt)
	tokens := make([]string, len(fields))
	for i, f := range fields {
		tokens[i] = strings.ToLower(f)
	}
	return tokens
}

// scoreEntry counts how many tokens appear as substrings in the entry's name or source hint.
func scoreEntry(tokens []string, name, sourceHint string) int {
	nameLower := strings.ToLower(name)
	hintLower := strings.ToLower(sourceHint)
	score := 0
	for _, tok := range tokens {
		if strings.Contains(nameLower, tok) || strings.Contains(hintLower, tok) {
			score++
		}
	}
	return score
}
