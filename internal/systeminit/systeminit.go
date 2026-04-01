package systeminit

import (
	"fmt"
	"strings"

	"github.com/gocode/gocode/internal/commands"
	"github.com/gocode/gocode/internal/setup"
	"github.com/gocode/gocode/internal/tools"
)

// BuildSystemInitMessage assembles the system initialization message for a new session.
func BuildSystemInitMessage(cmdReg *commands.CommandRegistry, toolReg *tools.ToolRegistry, trusted bool) string {
	report := setup.RunSetup(".", trusted)
	cmds := cmdReg.GetCommands(true, true)
	allTools := toolReg.GetTools(false, true, nil)
	builtins := cmdReg.BuiltInCommandNames()

	var b strings.Builder
	b.WriteString("# System Init\n\n")
	b.WriteString(fmt.Sprintf("Trusted: %v\n", report.Trusted))
	b.WriteString(fmt.Sprintf("Built-in command names: %d\n", len(builtins)))
	b.WriteString(fmt.Sprintf("Loaded command entries: %d\n", len(cmds)))
	b.WriteString(fmt.Sprintf("Loaded tool entries: %d\n\n", len(allTools)))
	b.WriteString("Startup steps:\n")
	for _, step := range report.Setup.StartupSteps() {
		b.WriteString(fmt.Sprintf("- %s\n", step))
	}
	return b.String()
}
