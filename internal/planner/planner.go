package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AlleyBo55/gocode/internal/agent"
	"github.com/AlleyBo55/gocode/internal/apiclient"
	"github.com/AlleyBo55/gocode/internal/apitypes"
)

const plannerSystemPrompt = `You are a planning agent. Your job is to help the user create a structured plan before any code execution begins.

When given a task, follow this process:
1. First, ask clarifying questions to understand scope, constraints, and expected outcomes. Format each question on its own line starting with "? " (question mark followed by a space).
2. After receiving answers, produce a structured plan in JSON format with this schema:
{
  "summary": "brief task summary",
  "ambiguities": ["list of unresolved ambiguities"],
  "steps": [{"description": "what to do", "rationale": "why"}],
  "scope": "estimated scope description"
}

When asking questions, output ONLY the questions (one per line, each starting with "? ").
When producing the plan, output ONLY the JSON object.`

// QuestionCallback is invoked for each clarifying question during planning.
// The caller provides the answer; returning an error aborts the planning session.
type QuestionCallback func(question string) (answer string, err error)

// PlanDocument is the structured output of a planning session.
type PlanDocument struct {
	Summary     string     `json:"summary"`
	Ambiguities []string   `json:"ambiguities"`
	Steps       []PlanStep `json:"steps"`
	Scope       string     `json:"scope"`
}

// PlanStep describes a single step in the plan.
type PlanStep struct {
	Description string `json:"description"`
	Rationale   string `json:"rationale"`
}

// PlannerAgent conducts interview-style planning via an LLM conversation.
type PlannerAgent struct {
	runtime *agent.ConversationRuntime
}

// NewPlannerAgent creates a PlannerAgent with a planning-specific system prompt.
func NewPlannerAgent(provider apiclient.Provider, model string) *PlannerAgent {
	rt := agent.NewConversationRuntime(agent.RuntimeOptions{
		Provider:     provider,
		Executor:     agent.NewStaticExecutor(), // no tools needed for planning
		Model:        model,
		SystemPrompt: plannerSystemPrompt,
		MaxTokens:    4096,
		MaxIterations: 1, // we drive the loop ourselves
	})
	return &PlannerAgent{runtime: rt}
}

// Plan runs the interactive planning session. It sends the task to the LLM,
// collects clarifying questions, asks the user via the callback, sends answers
// back, and parses the final plan document.
func (p *PlannerAgent) Plan(ctx context.Context, task string, askUser QuestionCallback) (*PlanDocument, error) {
	// Step 1: Send the task and get clarifying questions
	resp, err := p.runtime.SendUserMessage(ctx, fmt.Sprintf("Please analyze this task and ask clarifying questions:\n\n%s", task))
	if err != nil {
		return nil, fmt.Errorf("planner: failed to get questions: %w", err)
	}

	responseText := extractText(resp)
	questions := parseQuestions(responseText)

	// Step 2: If there are questions, ask the user and collect answers
	if len(questions) > 0 {
		var answersBuilder strings.Builder
		answersBuilder.WriteString("Here are the answers to your questions:\n\n")

		for _, q := range questions {
			answer, err := askUser(q)
			if err != nil {
				return nil, fmt.Errorf("planner: question callback failed: %w", err)
			}
			answersBuilder.WriteString(fmt.Sprintf("Q: %s\nA: %s\n\n", q, answer))
		}

		// Step 3: Send answers and request the plan
		resp, err = p.runtime.SendUserMessage(ctx,
			answersBuilder.String()+"\nNow produce the structured plan as a JSON object.")
		if err != nil {
			return nil, fmt.Errorf("planner: failed to get plan: %w", err)
		}
		responseText = extractText(resp)
	}

	// Step 4: Parse the plan document from the response
	plan, err := parsePlanDocument(responseText)
	if err != nil {
		return nil, fmt.Errorf("planner: failed to parse plan: %w", err)
	}

	return plan, nil
}

// extractText concatenates all text blocks from a message response.
func extractText(resp *apitypes.MessageResponse) string {
	var parts []string
	for _, block := range resp.Content {
		if block.Kind == "text" && block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// parseQuestions extracts questions from the LLM response.
// Questions are lines starting with "? " or numbered lines containing "?".
func parseQuestions(text string) []string {
	var questions []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Lines starting with "? " are explicit questions
		if strings.HasPrefix(line, "? ") {
			questions = append(questions, strings.TrimPrefix(line, "? "))
			continue
		}
		// Numbered questions like "1. What is...?" or "1) How should...?"
		if len(line) > 2 && (line[0] >= '0' && line[0] <= '9') && strings.HasSuffix(line, "?") {
			// Strip the number prefix
			q := stripNumberPrefix(line)
			if q != "" {
				questions = append(questions, q)
			}
		}
	}
	return questions
}

// stripNumberPrefix removes leading "N. " or "N) " from a line.
func stripNumberPrefix(line string) string {
	for i, ch := range line {
		if ch >= '0' && ch <= '9' {
			continue
		}
		if ch == '.' || ch == ')' {
			return strings.TrimSpace(line[i+1:])
		}
		break
	}
	return line
}

// parsePlanDocument extracts and parses a JSON PlanDocument from the LLM response text.
func parsePlanDocument(text string) (*PlanDocument, error) {
	// Try to find JSON in the response — it may be wrapped in markdown code fences
	jsonStr := extractJSON(text)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON plan found in response")
	}

	var plan PlanDocument
	if err := json.Unmarshal([]byte(jsonStr), &plan); err != nil {
		return nil, fmt.Errorf("invalid plan JSON: %w", err)
	}

	if plan.Summary == "" {
		return nil, fmt.Errorf("plan missing required field: summary")
	}

	return &plan, nil
}

// extractJSON finds the first JSON object in the text, handling optional
// markdown code fences (```json ... ```).
func extractJSON(text string) string {
	// Try markdown-fenced JSON first
	if idx := strings.Index(text, "```json"); idx >= 0 {
		start := idx + len("```json")
		end := strings.Index(text[start:], "```")
		if end >= 0 {
			return strings.TrimSpace(text[start : start+end])
		}
	}
	if idx := strings.Index(text, "```"); idx >= 0 {
		start := idx + len("```")
		end := strings.Index(text[start:], "```")
		if end >= 0 {
			candidate := strings.TrimSpace(text[start : start+end])
			if strings.HasPrefix(candidate, "{") {
				return candidate
			}
		}
	}

	// Try to find a raw JSON object
	braceStart := strings.Index(text, "{")
	if braceStart < 0 {
		return ""
	}
	// Find the matching closing brace
	depth := 0
	for i := braceStart; i < len(text); i++ {
		switch text[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return text[braceStart : i+1]
			}
		}
	}
	return ""
}
