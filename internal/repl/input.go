package repl

import (
	"bufio"
	"io"
	"strings"
)

// SlashCommand represents a parsed slash command.
type SlashCommand int

const (
	CmdNone SlashCommand = iota
	CmdExit
	CmdClear
	CmdCost
	CmdPlan
	CmdInitDeep
	CmdSkill
	CmdCompact
	CmdModel
	CmdHelp
	CmdDiff
	CmdUndo
	CmdStatus
	CmdReview
	CmdPermissions
	CmdDoctor
	CmdRedo
	CmdConnect
	CmdShare
)

// ParseSlashCommand checks if input is a slash command.
func ParseSlashCommand(input string) SlashCommand {
	trimmed := strings.TrimSpace(input)
	lower := strings.ToLower(trimmed)
	// Handle commands that may have arguments.
	if lower == "/skill" || strings.HasPrefix(lower, "/skill ") {
		return CmdSkill
	}
	if lower == "/model" || strings.HasPrefix(lower, "/model ") {
		return CmdModel
	}
	if lower == "/help" || strings.HasPrefix(lower, "/help ") {
		return CmdHelp
	}
	switch lower {
	case "/exit":
		return CmdExit
	case "/clear":
		return CmdClear
	case "/cost":
		return CmdCost
	case "/plan":
		return CmdPlan
	case "/init-deep":
		return CmdInitDeep
	case "/compact":
		return CmdCompact
	case "/diff":
		return CmdDiff
	case "/undo":
		return CmdUndo
	case "/redo":
		return CmdRedo
	case "/status":
		return CmdStatus
	case "/review":
		return CmdReview
	case "/permissions":
		return CmdPermissions
	case "/doctor":
		return CmdDoctor
	case "/connect":
		return CmdConnect
	case "/share":
		return CmdShare
	default:
		return CmdNone
	}
}

// ReadInput reads a potentially multi-line input from the reader.
// Lines ending with backslash are treated as continuations.
func ReadInput(scanner *bufio.Scanner) (string, error) {
	var lines []string
	for {
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return "", err
			}
			// EOF
			if len(lines) > 0 {
				return joinContinuation(lines), nil
			}
			return "", io.EOF
		}
		line := scanner.Text()
		if strings.HasSuffix(line, "\\") {
			lines = append(lines, strings.TrimSuffix(line, "\\"))
			continue
		}
		lines = append(lines, line)
		return joinContinuation(lines), nil
	}
}

func joinContinuation(lines []string) string {
	return strings.Join(lines, "\n")
}
