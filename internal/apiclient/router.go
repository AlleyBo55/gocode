package apiclient

import "fmt"

// TaskCategory classifies a task for model routing.
type TaskCategory string

const (
	// CategoryDeep is for autonomous research tasks.
	CategoryDeep TaskCategory = "deep"
	// CategoryQuick is for single-file changes.
	CategoryQuick TaskCategory = "quick"
	// CategoryVisualEngineering is for UI and visual tasks.
	CategoryVisualEngineering TaskCategory = "visual-engineering"
	// CategoryUltrabrain is for complex architecture decisions.
	CategoryUltrabrain TaskCategory = "ultrabrain"
)

// ModelRouter selects a FallbackProvider based on task category.
// It is a lookup table used by the Orchestrator to select the right provider
// when spawning sub-agents. It does not implement Provider directly.
type ModelRouter struct {
	routes map[TaskCategory]*FallbackProvider
}

// NewModelRouter creates a router from a category-to-FallbackProvider mapping.
func NewModelRouter(routes map[TaskCategory]*FallbackProvider) *ModelRouter {
	return &ModelRouter{routes: routes}
}

// Route returns the FallbackProvider for the given category.
// Returns an error if the category is not configured.
func (r *ModelRouter) Route(category TaskCategory) (*FallbackProvider, error) {
	fp, ok := r.routes[category]
	if !ok {
		return nil, fmt.Errorf("no provider configured for category %q", category)
	}
	return fp, nil
}
