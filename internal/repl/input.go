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
)

// ParseSlashCommand checks if input is a slash command.
func ParseSlashCommand(input string) SlashCommand {
	trimmed := strings.TrimSpace(input)
	switch strings.ToLower(trimmed) {
	case "/exit":
		return CmdExit
	case "/clear":
		return CmdClear
	case "/cost":
		return CmdCost
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
