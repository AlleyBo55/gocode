package repl

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ANSI color codes — Go brand palette
const (
	reset   = "\033[0m"
	bold    = "\033[1m"
	dim     = "\033[2m"
	goBlueC = "\033[38;5;38m"  // Go blue #00ADD8
	goTealC = "\033[38;5;37m"  // Go teal #00A29C
	white   = "\033[38;5;255m"
	gray    = "\033[38;5;242m"
	green   = "\033[38;5;114m"
	yellow  = "\033[38;5;221m"
	boxChar = "\033[38;5;60m"
)

// BannerConfig holds the info displayed in the welcome banner.
type BannerConfig struct {
	Version   string
	Model     string
	MaxTurns  int
	Cwd       string
	BuddyLine string // optional — empty means no buddy
}

// PrintBanner renders the OpenCode-style welcome screen with Go blue GOCODE branding.
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

	// Get git branch
	branch := ""
	if out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
		branch = strings.TrimSpace(string(out))
	}

	// ASCII art logo in Go blue
	logo := []string{
		goBlueC + `   ██████   ██████   ██████  ██████  ██████  ███████` + reset,
		goBlueC + `  ██       ██    ██ ██      ██    ██ ██   ██ ██     ` + reset,
		goBlueC + `  ██   ███ ██    ██ ██      ██    ██ ██   ██ █████  ` + reset,
		goBlueC + `  ██    ██ ██    ██ ██      ██    ██ ██   ██ ██     ` + reset,
		goBlueC + `   ██████   ██████   ██████  ██████  ██████  ███████` + reset,
	}

	fmt.Fprintln(w)
	for _, line := range logo {
		fmt.Fprintln(w, "  "+line)
	}
	fmt.Fprintln(w)

	// Info line
	fmt.Fprintf(w, "  %s%s%s %s%s%s", goBlueC+bold, version, reset, gray, cwd, reset)
	if branch != "" {
		fmt.Fprintf(w, " %s(%s)%s", goTealC, branch, reset)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w)

	// Model + turns
	fmt.Fprintf(w, "  %smodel%s %s%s%s  %sturns%s %s%d%s\n",
		gray, reset, white+bold, cfg.Model, reset,
		gray, reset, white+bold, cfg.MaxTurns, reset)

	// Buddy line (optional)
	if cfg.BuddyLine != "" {
		fmt.Fprintf(w, "  %sbuddy%s %s%s%s\n", gray, reset, goTealC, cfg.BuddyLine, reset)
	}
	fmt.Fprintln(w)

	// Separator
	fmt.Fprintf(w, "  %s%s%s\n", gray, strings.Repeat("─", 56), reset)
	fmt.Fprintln(w)

	// Commands in two columns
	cmds := []struct{ cmd, desc string }{
		{"/help", "commands"},
		{"/cost", "token usage"},
		{"/compact", "compact history"},
		{"/diff", "show changes"},
		{"/skill", "list skills"},
		{"/status", "session info"},
		{"/undo", "stash changes"},
		{"/doctor", "check env"},
	}

	for i := 0; i < len(cmds); i += 2 {
		left := fmt.Sprintf("  %s%-10s%s %s%-16s%s", goBlueC, cmds[i].cmd, reset, gray, cmds[i].desc, reset)
		right := ""
		if i+1 < len(cmds) {
			right = fmt.Sprintf("%s%-10s%s %s%s%s", goBlueC, cmds[i+1].cmd, reset, gray, cmds[i+1].desc, reset)
		}
		fmt.Fprintf(w, "%s  %s\n", left, right)
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %sTab%s %sswitch mode%s  %sCtrl+C%s %squit%s  %sCtrl+D%s %sEOF%s\n",
		white+bold, reset, gray, reset,
		white+bold, reset, gray, reset,
		white+bold, reset, gray, reset)
	fmt.Fprintf(w, "  %s%s%s\n", gray, strings.Repeat("─", 56), reset)
	fmt.Fprintln(w)
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
