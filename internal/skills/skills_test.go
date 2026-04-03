package skills

import (
	"sort"
	"testing"
)

func TestDefaultBuiltinsContains16Skills(t *testing.T) {
	builtins := defaultBuiltins()
	if got := len(builtins); got != 16 {
		t.Fatalf("expected 16 built-in skills, got %d", got)
	}
}

func TestWave2SkillsExistWithNonEmptyPrompts(t *testing.T) {
	wave2 := []string{"loop", "stuck", "debug", "verify", "simplify", "remember", "skillify", "batch"}
	builtins := defaultBuiltins()

	for _, name := range wave2 {
		t.Run(name, func(t *testing.T) {
			skill, ok := builtins[name]
			if !ok {
				t.Fatalf("skill %q not found in defaultBuiltins", name)
			}
			if skill.SystemPrompt == "" {
				t.Fatalf("skill %q has empty SystemPrompt", name)
			}
			if len(skill.ToolPerms) == 0 {
				t.Fatalf("skill %q has no tool permissions", name)
			}
		})
	}
}

func TestSkillLoaderLoadsAll16Builtins(t *testing.T) {
	// Use a non-existent directory so only built-ins are loaded.
	loader := NewSkillLoader(t.TempDir())
	skills, errs := loader.LoadAll()
	if len(errs) > 0 {
		t.Fatalf("unexpected errors loading skills: %v", errs)
	}
	if got := len(skills); got != 16 {
		names := make([]string, len(skills))
		for i, s := range skills {
			names[i] = s.Name
		}
		sort.Strings(names)
		t.Fatalf("expected 16 skills from LoadAll, got %d: %v", got, names)
	}
}

func TestAllBuiltinSkillsPassValidation(t *testing.T) {
	builtins := defaultBuiltins()
	for name, skill := range builtins {
		t.Run(name, func(t *testing.T) {
			if err := Validate(skill); err != nil {
				t.Fatalf("built-in skill %q failed validation: %v", name, err)
			}
		})
	}
}
