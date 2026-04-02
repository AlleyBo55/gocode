package repl

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ANSI color codes
const (
	reset   = "\033[0m"
	bold    = "\033[1m"
	dim     = "\033[2m"
	b4      = "\033[38;5;39m"  // bright blue
	b6      = "\033[38;5;51m"  // cyan-blue
	white   = "\033[38;5;255m"
	gray    = "\033[38;5;242m"
	green   = "\033[38;5;114m"
	yellow  = "\033[38;5;221m"
	boxChar = "\033[38;5;60m"
)

// The gopher — same pixel art as Claude Code's claw mascot, but blue
var gopher = []string{
	b4 + `        ╻╻` + reset,
	b4 + `       ╭┫┣╮` + reset,
	b4 + `      ╭┫╰╯┣╮` + reset,
	b4 + `    ╭━┫┃  ┃┣━╮` + reset,
	b4 + `    ┃ ╰┫  ┣╯ ┃` + reset,
	b4 + `    ╰━━┫  ┣━━╯` + reset,
	b4 + `       ┃  ┃` + reset,
	b4 + `       ╹  ╹` + reset,
}

// BannerConfig holds the info displayed in the welcome banner.
type BannerConfig struct {
	Version  string
	Model    string
	MaxTurns int
	Cwd      string
}

// PrintBanner renders the Claude-Code-style welcome screen.
func PrintBanner(w io.Writer, cfg BannerConfig) {
	cwd := cfg.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	if home, err := os.UserHomeDir(); err == nil {
		if rel, err := filepath.Rel(home, cwd); err == nil && !strings.HasPrefix(rel, "..") {
			cwd = "~/" + rel
		}
	}

	version := cfg.Version
	if version == "" {
		version = "dev"
	}

	width := 64
	title := fmt.Sprintf(" gocode %s ", version)
	padTotal := width - 2 - len(title)
	padLeft := padTotal / 2
	padRight := padTotal - padLeft

	fmt.Fprintf(w, "%s╭%s%s%s%s%s%s%s╮%s\n",
		boxChar, strings.Repeat("─", padLeft), reset, bold+b6, title, reset, boxChar, strings.Repeat("─", padRight), reset)
	printEmptyLine(w, width)

	// Left: gopher + model info, Right: tips + models
	leftLines := []string{""}
	leftLines = append(leftLines, gopher...)
	leftLines = append(leftLines, "")
	leftLines = append(leftLines, fmt.Sprintf(" %s%s%s • Max %s%d%sx", bold+white, cfg.Model, reset, bold+white, cfg.MaxTurns, reset))
	leftLines = append(leftLines, fmt.Sprintf(" %s%s%s", dim+gray, cwd, reset))

	rightLines := []string{
		green + bold + "Tips" + reset,
		gray + " /exit      " + white + "quit session" + reset,
		gray + " /clear     " + white + "reset history" + reset,
		gray + " /cost      " + white + "token usage" + reset,
		gray + " Ctrl+D     " + white + "quit (EOF)" + reset,
		"",
		yellow + bold + "Models" + reset,
		gray + " --model " + white + "sonnet" + gray + "  Claude" + reset,
		gray + " --model " + white + "gpt5" + gray + "    GPT-5.4" + reset,
		gray + " --model " + white + "gemini" + gray + "  Gemini 3.1" + reset,
		gray + " --model " + white + "grok" + gray + "    Grok 4.20" + reset,
	}

	maxLines := len(leftLines)
	if len(rightLines) > maxLines {
		maxLines = len(rightLines)
	}
	for len(leftLines) < maxLines {
		leftLines = append(leftLines, "")
	}
	for len(rightLines) < maxLines {
		rightLines = append(rightLines, "")
	}

	leftColWidth := 30
	for i := 0; i < maxLines; i++ {
		left := leftLines[i]
		right := rightLines[i]
		lv := visibleLen(left)
		rv := visibleLen(right)
		lpad := leftColWidth - lv
		if lpad < 0 {
			lpad = 0
		}
		rpad := width - 2 - leftColWidth - rv
		if rpad < 0 {
			rpad = 0
		}
		fmt.Fprintf(w, "%s│%s%s%s%s%s%s│%s\n",
			boxChar, reset, left, strings.Repeat(" ", lpad), right, strings.Repeat(" ", rpad), boxChar, reset)
	}

	printEmptyLine(w, width)
	fmt.Fprintf(w, "%s╰%s╯%s\n\n", boxChar, strings.Repeat("─", width-2), reset)
}

func printPaddedLine(w io.Writer, content string, width int) {
	vis := visibleLen(content)
	pad := width - 2 - vis
	if pad < 0 {
		pad = 0
	}
	fmt.Fprintf(w, "%s│%s%s%s%s│%s\n", boxChar, reset, content, strings.Repeat(" ", pad), boxChar, reset)
}

func printEmptyLine(w io.Writer, width int) {
	fmt.Fprintf(w, "%s│%s%s%s│%s\n", boxChar, reset, strings.Repeat(" ", width-2), boxChar, reset)
}

// visibleLen returns the length of a string excluding ANSI escape codes.
func visibleLen(s string) int {
	inEscape := false
	count := 0
	for _, r := range s {
		if r == '\033' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		count++
	}
	return count
}
