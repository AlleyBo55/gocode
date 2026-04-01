package bootstrap

import "fmt"

// BootstrapGraph defines the bootstrap stage dependency graph.
type BootstrapGraph struct {
	Stages []string `json:"stages"`
}

// BuildBootstrapGraph returns the default bootstrap graph with all stages.
func BuildBootstrapGraph() *BootstrapGraph {
	return &BootstrapGraph{
		Stages: []string{
			"top-level prefetch side effects",
			"warning handler and environment guards",
			"CLI parser and pre-action trust gate",
			"setup() + commands/agents parallel load",
			"deferred init after trust",
			"mode routing: local / remote / ssh / teleport / direct-connect / deep-link",
			"query engine submit loop",
		},
	}
}

// Render returns a Markdown-formatted representation of the bootstrap graph.
func (bg *BootstrapGraph) Render() string {
	out := "# Bootstrap Graph\n\n"
	for _, stage := range bg.Stages {
		out += fmt.Sprintf("- %s\n", stage)
	}
	return out
}
