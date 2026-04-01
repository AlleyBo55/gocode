package commandgraph

import (
	"fmt"
	"strings"

	"github.com/gocode/gocode/internal/models"
)

// CommandGraph segments commands into builtin, plugin-like, and skill-like categories.
type CommandGraph struct {
	Builtins   []models.PortingModule `json:"builtins"`
	PluginLike []models.PortingModule `json:"plugin_like"`
	SkillLike  []models.PortingModule `json:"skill_like"`
}

// BuildCommandGraph segments the given commands into categories based on source_hint.
func BuildCommandGraph(commands []models.PortingModule) *CommandGraph {
	var builtins, plugins, skills []models.PortingModule
	for _, cmd := range commands {
		hint := strings.ToLower(cmd.SourceHint)
		switch {
		case strings.Contains(hint, "plugin"):
			plugins = append(plugins, cmd)
		case strings.Contains(hint, "skill"):
			skills = append(skills, cmd)
		default:
			builtins = append(builtins, cmd)
		}
	}
	return &CommandGraph{
		Builtins:   builtins,
		PluginLike: plugins,
		SkillLike:  skills,
	}
}

// Flattened returns all commands across all categories.
func (cg *CommandGraph) Flattened() []models.PortingModule {
	result := make([]models.PortingModule, 0, len(cg.Builtins)+len(cg.PluginLike)+len(cg.SkillLike))
	result = append(result, cg.Builtins...)
	result = append(result, cg.PluginLike...)
	result = append(result, cg.SkillLike...)
	return result
}

// Render returns a Markdown-formatted summary of the command graph.
func (cg *CommandGraph) Render() string {
	return fmt.Sprintf("# Command Graph\n\nBuiltins: %d\nPlugin-like commands: %d\nSkill-like commands: %d\n",
		len(cg.Builtins), len(cg.PluginLike), len(cg.SkillLike))
}
