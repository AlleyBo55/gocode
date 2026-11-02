package repl

// planModePrompt is prepended to each user turn while plan mode is on. The
// runtime already blocks every non-read tool; this tells the model why, so it
// produces a plan instead of retrying tools that will be refused.
const planModePrompt = "[Plan mode: read-only] Investigate with read, search and fetch tools only. " +
	"Then reply with a numbered implementation plan: the files to change, what changes in each, " +
	"the risks, and how to verify. Do not try to edit files or run commands; those tools are blocked " +
	"until the user leaves plan mode with /plan."
