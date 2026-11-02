package repl

import "strings"

// knownCommands is the set a typo is matched against. Keep in sync with
// ParseSlashCommand and the /help list.
var knownCommands = []string{
	"help", "exit", "clear", "compact", "cost", "model", "skill", "plan",
	"init-deep", "diff", "undo", "redo", "status", "review", "permissions",
	"doctor", "connect", "share", "commit", "memory", "tasks", "worktree",
	"voice", "ultraplan", "goal", "vim", "output-style",
}

// isLoneCommandToken reports whether input is a single "/word" with no spaces
// and no inner slash — the shape of a mistyped command, as opposed to prose
// that merely starts with "/" (e.g. "/etc/passwd is the file").
func isLoneCommandToken(input string) bool {
	s := strings.TrimSpace(input)
	if len(s) < 2 || s[0] != '/' {
		return false
	}
	for _, r := range s[1:] {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-') {
			return false
		}
	}
	return true
}

// suggestCommand returns the closest known command to name and whether it is
// close enough to be worth offering (edit distance <= 2, or a clear prefix).
func suggestCommand(name string) (string, bool) {
	name = strings.ToLower(name)
	best, bestDist := "", 1<<30
	for _, c := range knownCommands {
		if c == name {
			return c, true
		}
		d := levenshtein(name, c)
		// A short typo that is a prefix of a command counts as very close.
		if strings.HasPrefix(c, name) && len(name) >= 2 {
			d = min(d, 1)
		}
		if d < bestDist {
			best, bestDist = c, d
		}
	}
	return best, bestDist <= 2
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, min(cur[j-1]+1, prev[j-1]+cost))
		}
		prev = cur
	}
	return prev[len(rb)]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
