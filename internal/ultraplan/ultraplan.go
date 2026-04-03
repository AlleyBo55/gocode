// Package ultraplan provides deep planning via the strongest available model.
//
// Ported from Claude Code's src/utils/ultraplan/ — but improved:
// - Multi-model: uses ModelRouter's "ultrabrain" category (best available model)
// - Fallback: if primary model is down, FallbackProvider chains to next best
// - Concurrent: runs as background agent via orchestrator, user keeps chatting
// - Composable: reuses existing orchestrator/router/fallback infrastructure
//
// Claude Code's version is locked to a single remote Opus session. Ours picks
// the strongest brain across all configured providers.
package ultraplan

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/AlleyBo55/gocode/internal/agent"
	"github.com/AlleyBo55/gocode/internal/apiclient"
	"github.com/AlleyBo55/gocode/internal/apitypes"
)

// DefaultTimeout is the maximum wall-clock time for an ULTRAPLAN session.
// Claude Code uses 30 minutes. We match that.
const DefaultTimeout = 30 * time.Minute

// MaxTokens is the output token budget for ultraplan responses.
// Deep planning needs room to think — 32K is generous but bounded.
const MaxTokens = 32768

// MaxIterations is the agent loop cap. Higher than normal (30) because
// planning may involve many tool calls (file reads, grep, etc.).
const MaxIterations = 60

// ultraplanRe matches "ultraplan" as a whole word, case-insensitive.
var ultraplanRe = regexp.MustCompile(`(?i)\bultraplan\b`)

// HasKeyword returns true if text contains the "ultraplan" trigger word.
// Slash commands (starting with "/") are excluded to avoid false triggers
// when the user types "/ultraplan" — that's handled by the REPL directly.
func HasKeyword(text string) bool {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "/") {
		return false
	}
	return ultraplanRe.MatchString(trimmed)
}

// systemPrompt is the planning agent's system prompt. It's designed to
// produce structured, actionable plans — not just analysis.
const systemPrompt = `You are an expert software architect performing deep planning.
You have access to all tools. Use them aggressively — read files, search the
codebase, understand the full picture before proposing anything.

Your job is to produce a PLAN, not to implement it. The plan must be:
1. Specific enough that a junior developer could follow it
2. Ordered by dependency (what must happen first)
3. Scoped with clear boundaries (what's in, what's out)
4. Risk-aware (what could go wrong, what's the fallback)

Structure your output as:

## Summary
One paragraph: what we're doing and why.

## Scope
- IN: what this plan covers
- OUT: what it explicitly does NOT cover

## Steps
Numbered steps. For each:
- What to do (specific files, functions, commands)
- Why (rationale)
- Verification (how to confirm this step worked)
- Dependencies (which prior steps must be done first)

## Risks
What could go wrong. For each risk: likelihood, impact, mitigation.

## Open Questions
Things you couldn't determine from the codebase that need human input.`

// Planner runs deep planning sessions using the strongest available model.
type Planner struct {
	router   *apiclient.ModelRouter
	executor agent.ToolExecutor
}

// NewPlanner creates an ULTRAPLAN planner wired to the model router.
func NewPlanner(router *apiclient.ModelRouter, executor agent.ToolExecutor) *Planner {
	return &Planner{router: router, executor: executor}
}

// Plan runs a synchronous deep planning session. It routes to the "ultrabrain"
// model category (strongest available model with automatic fallback) and gives
// the agent full tool access with a 30-minute timeout.
func (p *Planner) Plan(ctx context.Context, task string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	// Route to the strongest available model via ultrabrain category.
	// The FallbackProvider handles rate limits and failover automatically.
	provider, err := p.router.Route(apiclient.CategoryUltrabrain)
	if err != nil {
		return "", fmt.Errorf("ultraplan: no ultrabrain provider configured: %w", err)
	}

	// Create a dedicated ConversationRuntime for the planning session.
	// Full tool access, high token budget, extended iteration limit.
	rt := agent.NewConversationRuntime(agent.RuntimeOptions{
		Provider:      provider,
		Executor:      p.executor,
		Model:         "", // let FallbackProvider pick the best model
		MaxTokens:     MaxTokens,
		MaxIterations: MaxIterations,
		SystemPrompt:  systemPrompt,
		PermMode:      agent.DangerFullAccess, // planning agent needs full read access
	})

	resp, err := rt.SendUserMessage(ctx, task)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("ultraplan timed out after %v", DefaultTimeout)
		}
		return "", fmt.Errorf("ultraplan failed: %w", err)
	}

	return extractText(resp), nil
}

// PlanBackground runs the planning session in a background goroutine.
// Returns a channel that receives exactly one result.
func (p *Planner) PlanBackground(ctx context.Context, task string) <-chan Result {
	ch := make(chan Result, 1)
	go func() {
		defer close(ch)
		output, err := p.Plan(ctx, task)
		ch <- Result{Output: output, Err: err}
	}()
	return ch
}

// Result is the outcome of a background planning session.
type Result struct {
	Output string
	Err    error
}

// extractText collects all text blocks from a response.
func extractText(resp *apitypes.MessageResponse) string {
	if resp == nil {
		return ""
	}
	var parts []string
	for _, block := range resp.Content {
		if block.Kind == "text" && block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n")
}
