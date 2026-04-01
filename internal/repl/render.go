package repl

import (
	"fmt"
	"strings"
)

// ANSI style codes for terminal markdown rendering
const (
	ansiReset     = "\033[0m"
	ansiBold      = "\033[1m"
	ansiDim       = "\033[2m"
	ansiItalic    = "\033[3m"
	ansiUnderline = "\033[4m"
	ansiStrike    = "\033[9m"

	// Colors
	cBlue    = "\033[38;5;39m"
	cCyan    = "\033[38;5;51m"
	cGreen   = "\033[38;5;114m"
	cYellow  = "\033[38;5;221m"
	cRed     = "\033[38;5;203m"
	cMagenta = "\033[38;5;170m"
	cGray    = "\033[38;5;242m"
	cWhite   = "\033[38;5;255m"
	cOrange  = "\033[38;5;208m"

	// Backgrounds
	bgCodeBlock = "\033[48;5;236m"
	bgInline    = "\033[48;5;238m"
)

// RenderMarkdown converts markdown text to ANSI-colored terminal output.
func RenderMarkdown(text string) string {
	lines := strings.Split(text, "\n")
	var out strings.Builder
	inCodeBlock := false
	codeLang := ""

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// Code block toggle
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			if !inCodeBlock {
				inCodeBlock = true
				codeLang = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "```"))
				if codeLang != "" {
					out.WriteString(fmt.Sprintf("  %s%s %s %s\n", cGray, "╭─", codeLang, ansiReset))
				} else {
					out.WriteString(fmt.Sprintf("  %s╭─%s\n", cGray, ansiReset))
				}
				continue
			}
			inCodeBlock = false
			codeLang = ""
			out.WriteString(fmt.Sprintf("  %s╰─%s\n", cGray, ansiReset))
			continue
		}

		if inCodeBlock {
			highlighted := highlightCodeLine(line, codeLang)
			out.WriteString(fmt.Sprintf("  %s│%s %s%s%s\n", cGray, ansiReset, bgCodeBlock, highlighted, ansiReset))
			continue
		}

		// Headers
		if strings.HasPrefix(line, "#### ") {
			out.WriteString(fmt.Sprintf("%s%s%s%s\n", ansiBold, cCyan, strings.TrimPrefix(line, "#### "), ansiReset))
			continue
		}
		if strings.HasPrefix(line, "### ") {
			out.WriteString(fmt.Sprintf("%s%s%s%s\n", ansiBold, cCyan, strings.TrimPrefix(line, "### "), ansiReset))
			continue
		}
		if strings.HasPrefix(line, "## ") {
			out.WriteString(fmt.Sprintf("%s%s%s%s\n", ansiBold, cBlue, strings.TrimPrefix(line, "## "), ansiReset))
			continue
		}
		if strings.HasPrefix(line, "# ") {
			out.WriteString(fmt.Sprintf("%s%s%s%s\n", ansiBold, cBlue, strings.TrimPrefix(line, "# "), ansiReset))
			continue
		}

		// Horizontal rule
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" || trimmed == "***" || trimmed == "___" {
			out.WriteString(fmt.Sprintf("%s────────────────────────────────%s\n", cGray, ansiReset))
			continue
		}

		// Bullet lists
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			indent := len(line) - len(strings.TrimLeft(line, " "))
			content := trimmed[2:]
			out.WriteString(strings.Repeat(" ", indent))
			out.WriteString(fmt.Sprintf("  %s•%s %s\n", cBlue, ansiReset, renderInline(content)))
			continue
		}

		// Numbered lists
		if len(trimmed) > 2 && trimmed[0] >= '0' && trimmed[0] <= '9' && strings.Contains(trimmed[:4], ". ") {
			idx := strings.Index(trimmed, ". ")
			num := trimmed[:idx]
			content := trimmed[idx+2:]
			indent := len(line) - len(strings.TrimLeft(line, " "))
			out.WriteString(strings.Repeat(" ", indent))
			out.WriteString(fmt.Sprintf("  %s%s.%s %s\n", cBlue, num, ansiReset, renderInline(content)))
			continue
		}

		// Regular paragraph
		out.WriteString(renderInline(line))
		out.WriteString("\n")
	}

	return out.String()
}

// renderInline handles bold, italic, code, strikethrough within a line.
func renderInline(s string) string {
	var out strings.Builder
	runes := []rune(s)
	i := 0

	for i < len(runes) {
		// Bold + italic ***text***
		if i+2 < len(runes) && runes[i] == '*' && runes[i+1] == '*' && runes[i+2] == '*' {
			end := findClosing(runes, i+3, "***")
			if end >= 0 {
				out.WriteString(ansiBold + ansiItalic + cWhite + string(runes[i+3:end]) + ansiReset)
				i = end + 3
				continue
			}
		}
		// Bold **text**
		if i+1 < len(runes) && runes[i] == '*' && runes[i+1] == '*' {
			end := findClosing(runes, i+2, "**")
			if end >= 0 {
				out.WriteString(ansiBold + cWhite + string(runes[i+2:end]) + ansiReset)
				i = end + 2
				continue
			}
		}
		// Italic *text*
		if runes[i] == '*' && (i == 0 || runes[i-1] == ' ') {
			end := findClosing(runes, i+1, "*")
			if end >= 0 && end > i+1 {
				out.WriteString(ansiItalic + string(runes[i+1:end]) + ansiReset)
				i = end + 1
				continue
			}
		}
		// Inline code `text`
		if runes[i] == '`' {
			end := findClosingRune(runes, i+1, '`')
			if end >= 0 {
				out.WriteString(bgInline + cGreen + " " + string(runes[i+1:end]) + " " + ansiReset)
				i = end + 1
				continue
			}
		}
		// Strikethrough ~~text~~
		if i+1 < len(runes) && runes[i] == '~' && runes[i+1] == '~' {
			end := findClosing(runes, i+2, "~~")
			if end >= 0 {
				out.WriteString(ansiStrike + cGray + string(runes[i+2:end]) + ansiReset)
				i = end + 2
				continue
			}
		}
		// Links [text](url)
		if runes[i] == '[' {
			closeBracket := findClosingRune(runes, i+1, ']')
			if closeBracket >= 0 && closeBracket+1 < len(runes) && runes[closeBracket+1] == '(' {
				closeParen := findClosingRune(runes, closeBracket+2, ')')
				if closeParen >= 0 {
					linkText := string(runes[i+1 : closeBracket])
					linkURL := string(runes[closeBracket+2 : closeParen])
					out.WriteString(ansiUnderline + cBlue + linkText + ansiReset + cGray + " (" + linkURL + ")" + ansiReset)
					i = closeParen + 1
					continue
				}
			}
		}
		out.WriteRune(runes[i])
		i++
	}
	return out.String()
}

func findClosing(runes []rune, start int, marker string) int {
	mr := []rune(marker)
	for i := start; i <= len(runes)-len(mr); i++ {
		match := true
		for j := 0; j < len(mr); j++ {
			if runes[i+j] != mr[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func findClosingRune(runes []rune, start int, ch rune) int {
	for i := start; i < len(runes); i++ {
		if runes[i] == ch {
			return i
		}
	}
	return -1
}

// highlightCodeLine applies basic syntax highlighting to a code line.
func highlightCodeLine(line, lang string) string {
	// Keywords by language family
	keywords := map[string][]string{
		"go":         {"func", "return", "if", "else", "for", "range", "var", "const", "type", "struct", "interface", "package", "import", "defer", "go", "chan", "select", "case", "switch", "default", "break", "continue", "map", "nil", "true", "false", "err"},
		"javascript": {"function", "return", "if", "else", "for", "const", "let", "var", "class", "import", "export", "from", "async", "await", "new", "this", "null", "undefined", "true", "false", "try", "catch", "throw"},
		"typescript": {"function", "return", "if", "else", "for", "const", "let", "var", "class", "import", "export", "from", "async", "await", "new", "this", "null", "undefined", "true", "false", "try", "catch", "throw", "interface", "type", "enum"},
		"python":     {"def", "return", "if", "else", "elif", "for", "while", "class", "import", "from", "as", "with", "try", "except", "raise", "None", "True", "False", "self", "lambda", "yield", "async", "await"},
		"rust":       {"fn", "let", "mut", "if", "else", "for", "while", "loop", "match", "struct", "enum", "impl", "trait", "pub", "use", "mod", "self", "Self", "return", "true", "false", "None", "Some", "Ok", "Err"},
		"bash":       {"if", "then", "else", "fi", "for", "do", "done", "while", "case", "esac", "function", "return", "export", "echo", "exit", "cd", "ls", "grep", "sed", "awk"},
	}

	// Normalize lang aliases
	switch strings.ToLower(lang) {
	case "js", "jsx":
		lang = "javascript"
	case "ts", "tsx":
		lang = "typescript"
	case "py":
		lang = "python"
	case "rs":
		lang = "rust"
	case "sh", "zsh", "shell":
		lang = "bash"
	case "golang":
		lang = "go"
	}

	kw, ok := keywords[strings.ToLower(lang)]
	if !ok {
		// Default: just color strings and comments
		return highlightGeneric(line)
	}

	return highlightWithKeywords(line, kw)
}

func highlightWithKeywords(line string, keywords []string) string {
	// Handle comments first
	if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
		return cGray + line + ansiReset
	}

	result := highlightStrings(line)

	// Highlight keywords (word boundary check)
	for _, kw := range keywords {
		result = highlightKeyword(result, kw)
	}

	return result
}

func highlightGeneric(line string) string {
	if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "--") {
		return cGray + line + ansiReset
	}
	return highlightStrings(line)
}

func highlightStrings(line string) string {
	var out strings.Builder
	runes := []rune(line)
	i := 0
	for i < len(runes) {
		if runes[i] == '"' || runes[i] == '\'' || runes[i] == '`' {
			quote := runes[i]
			end := i + 1
			for end < len(runes) && runes[end] != quote {
				if runes[end] == '\\' {
					end++
				}
				end++
			}
			if end < len(runes) {
				end++
			}
			out.WriteString(cGreen + string(runes[i:end]) + ansiReset)
			i = end
			continue
		}
		// Numbers
		if runes[i] >= '0' && runes[i] <= '9' && (i == 0 || !isAlpha(runes[i-1])) {
			end := i
			for end < len(runes) && (runes[end] >= '0' && runes[end] <= '9' || runes[end] == '.') {
				end++
			}
			out.WriteString(cOrange + string(runes[i:end]) + ansiReset)
			i = end
			continue
		}
		out.WriteRune(runes[i])
		i++
	}
	return out.String()
}

func highlightKeyword(line, kw string) string {
	// Simple word-boundary replacement
	result := line
	search := kw
	idx := 0
	for {
		pos := strings.Index(result[idx:], search)
		if pos < 0 {
			break
		}
		absPos := idx + pos
		// Check word boundaries
		before := absPos == 0 || !isAlpha([]rune(result)[absPos-1])
		afterPos := absPos + len(search)
		after := afterPos >= len([]rune(result)) || !isAlpha([]rune(result)[afterPos])
		// Don't replace inside ANSI escape sequences
		if before && after && !inAnsiEscape(result, absPos) {
			replacement := cMagenta + kw + ansiReset
			result = result[:absPos] + replacement + result[afterPos:]
			idx = absPos + len(replacement)
		} else {
			idx = afterPos
		}
	}
	return result
}

func isAlpha(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_'
}

func inAnsiEscape(s string, pos int) bool {
	// Check if pos is inside an ANSI escape sequence
	for i := pos; i >= 0 && i > pos-20; i-- {
		if s[i] == '\033' {
			// Found escape start, check if we're still inside
			for j := i; j <= pos; j++ {
				if s[j] == 'm' {
					if j >= pos {
						return true
					}
					break
				}
			}
		}
	}
	return false
}

// RenderToolOutput formats tool output with dimmed styling.
func RenderToolOutput(name string, output string, isError bool) string {
	var out strings.Builder
	if isError {
		out.WriteString(fmt.Sprintf("  %s✗ %s failed:%s\n", cRed, name, ansiReset))
		out.WriteString(fmt.Sprintf("  %s%s%s\n", cRed+ansiDim, truncate(output, 200), ansiReset))
	} else {
		out.WriteString(fmt.Sprintf("  %s✓ %s%s\n", cGreen, name, ansiReset))
		if output != "" {
			// Show dimmed, truncated output
			lines := strings.Split(output, "\n")
			maxLines := 5
			if len(lines) > maxLines {
				for _, l := range lines[:maxLines] {
					out.WriteString(fmt.Sprintf("  %s%s%s\n", ansiDim+cGray, truncate(l, 80), ansiReset))
				}
				out.WriteString(fmt.Sprintf("  %s... (%d more lines)%s\n", cGray, len(lines)-maxLines, ansiReset))
			} else {
				for _, l := range lines {
					out.WriteString(fmt.Sprintf("  %s%s%s\n", ansiDim+cGray, truncate(l, 80), ansiReset))
				}
			}
		}
	}
	return out.String()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
