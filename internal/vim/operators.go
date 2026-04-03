package vim

import "unicode"

// ApplyOperatorMotion executes an operator (d/c/y) combined with a motion.
func ApplyOperatorMotion(s *VimState, op Operator, m Motion, count int) {
	if len(s.Buffer) == 0 {
		return
	}
	target := ApplyMotion(s, m, count)
	start, end := s.Cursor, target
	if start > end {
		start, end = end, start
	}
	end++
	if end > len(s.Buffer) {
		end = len(s.Buffer)
	}
	applyOpRange(s, op, start, end)
}

// ApplyOperatorTextObject executes an operator combined with a text object.
func ApplyOperatorTextObject(s *VimState, op Operator, obj TextObject) {
	if len(s.Buffer) == 0 {
		return
	}
	start, end := ResolveTextObjectRange(s.Buffer, s.Cursor, obj)
	if start == end {
		return
	}
	applyOpRange(s, op, start, end)
}

func applyOpRange(s *VimState, op Operator, start, end int) {
	yanked := string(s.Buffer[start:end])
	switch op {
	case OpDelete:
		s.Yanked = yanked
		s.Buffer = append(s.Buffer[:start], s.Buffer[end:]...)
		s.Cursor = clampCursor(start, len(s.Buffer))
		s.Mode = ModeNormal
	case OpChange:
		s.Yanked = yanked
		s.Buffer = append(s.Buffer[:start], s.Buffer[end:]...)
		s.Cursor = clampCursor(start, len(s.Buffer))
		s.Mode = ModeInsert
	case OpYank:
		s.Yanked = yanked
		s.Mode = ModeNormal
	}
	s.PendingOp = OpNone
}

// ResolveTextObjectRange returns the [start, end) range for a text object.
func ResolveTextObjectRange(buf []rune, cursor int, obj TextObject) (int, int) {
	n := len(buf)
	if n == 0 || cursor < 0 || cursor >= n {
		return 0, 0
	}
	switch obj {
	case TextObjInnerWord:
		return resolveInnerWord(buf, cursor)
	case TextObjAWord:
		return resolveAWord(buf, cursor)
	case TextObjInnerQuote:
		return resolveInnerQuote(buf, cursor, '"')
	case TextObjAQuote:
		return resolveAQuote(buf, cursor, '"')
	case TextObjInnerParen:
		return resolveInnerParen(buf, cursor)
	case TextObjAParen:
		return resolveAParen(buf, cursor)
	default:
		return cursor, cursor
	}
}

func resolveInnerWord(buf []rune, cursor int) (int, int) {
	cls := charClass(buf[cursor])
	start := cursor
	for start > 0 && charClass(buf[start-1]) == cls {
		start--
	}
	end := cursor + 1
	for end < len(buf) && charClass(buf[end]) == cls {
		end++
	}
	return start, end
}

func resolveAWord(buf []rune, cursor int) (int, int) {
	start, end := resolveInnerWord(buf, cursor)
	trailingEnd := end
	for trailingEnd < len(buf) && unicode.IsSpace(buf[trailingEnd]) {
		trailingEnd++
	}
	if trailingEnd > end {
		return start, trailingEnd
	}
	for start > 0 && unicode.IsSpace(buf[start-1]) {
		start--
	}
	return start, end
}

func resolveInnerQuote(buf []rune, cursor int, quote rune) (int, int) {
	open := -1
	for i := cursor; i >= 0; i-- {
		if buf[i] == quote {
			open = i
			break
		}
	}
	if open == -1 {
		for i := cursor; i < len(buf); i++ {
			if buf[i] == quote {
				open = i
				break
			}
		}
	}
	if open == -1 {
		return cursor, cursor
	}
	close := -1
	for i := open + 1; i < len(buf); i++ {
		if buf[i] == quote {
			close = i
			break
		}
	}
	if close == -1 {
		return cursor, cursor
	}
	return open + 1, close
}

func resolveAQuote(buf []rune, cursor int, quote rune) (int, int) {
	start, end := resolveInnerQuote(buf, cursor, quote)
	if start == end {
		return start, end
	}
	return start - 1, end + 1
}

func resolveInnerParen(buf []rune, cursor int) (int, int) {
	depth := 0
	open := -1
	for i := cursor; i >= 0; i-- {
		if buf[i] == ')' && i != cursor {
			depth++
		} else if buf[i] == '(' {
			if depth == 0 {
				open = i
				break
			}
			depth--
		}
	}
	if open == -1 {
		return cursor, cursor
	}
	depth = 0
	close := -1
	for i := open + 1; i < len(buf); i++ {
		if buf[i] == '(' {
			depth++
		} else if buf[i] == ')' {
			if depth == 0 {
				close = i
				break
			}
			depth--
		}
	}
	if close == -1 {
		return cursor, cursor
	}
	return open + 1, close
}

func resolveAParen(buf []rune, cursor int) (int, int) {
	start, end := resolveInnerParen(buf, cursor)
	if start == end {
		return start, end
	}
	return start - 1, end + 1
}

// Paste inserts the yanked text at the cursor position (vim 'p' command).
func Paste(s *VimState) {
	if s.Yanked == "" {
		return
	}
	runes := []rune(s.Yanked)
	insertPos := s.Cursor + 1
	if insertPos > len(s.Buffer) {
		insertPos = len(s.Buffer)
	}
	newBuf := make([]rune, 0, len(s.Buffer)+len(runes))
	newBuf = append(newBuf, s.Buffer[:insertPos]...)
	newBuf = append(newBuf, runes...)
	newBuf = append(newBuf, s.Buffer[insertPos:]...)
	s.Buffer = newBuf
	s.Cursor = insertPos + len(runes) - 1
	if s.Cursor < 0 {
		s.Cursor = 0
	}
}

// DeleteChar deletes the character at cursor (vim 'x' command).
func DeleteChar(s *VimState) {
	if len(s.Buffer) == 0 || s.Cursor >= len(s.Buffer) {
		return
	}
	s.Yanked = string(s.Buffer[s.Cursor : s.Cursor+1])
	s.Buffer = append(s.Buffer[:s.Cursor], s.Buffer[s.Cursor+1:]...)
	s.Cursor = clampCursor(s.Cursor, len(s.Buffer))
}

// ReplaceChar replaces the character at cursor with ch (vim 'r' command).
func ReplaceChar(s *VimState, ch rune) {
	if len(s.Buffer) == 0 || s.Cursor >= len(s.Buffer) {
		return
	}
	s.Buffer[s.Cursor] = ch
}

// DeleteLine deletes the entire current line (vim 'dd' command).
func DeleteLine(s *VimState) {
	if len(s.Buffer) == 0 {
		return
	}
	lineStart := moveLineStart(s.Buffer, s.Cursor)
	lineEnd := s.Cursor
	for lineEnd < len(s.Buffer) && s.Buffer[lineEnd] != '\n' {
		lineEnd++
	}
	if lineEnd < len(s.Buffer) && s.Buffer[lineEnd] == '\n' {
		lineEnd++
	}
	s.Yanked = string(s.Buffer[lineStart:lineEnd])
	s.Buffer = append(s.Buffer[:lineStart], s.Buffer[lineEnd:]...)
	s.Cursor = clampCursor(lineStart, len(s.Buffer))
}

// YankLine yanks the entire current line (vim 'yy' command).
func YankLine(s *VimState) {
	if len(s.Buffer) == 0 {
		return
	}
	lineStart := moveLineStart(s.Buffer, s.Cursor)
	lineEnd := s.Cursor
	for lineEnd < len(s.Buffer) && s.Buffer[lineEnd] != '\n' {
		lineEnd++
	}
	if lineEnd < len(s.Buffer) && s.Buffer[lineEnd] == '\n' {
		lineEnd++
	}
	s.Yanked = string(s.Buffer[lineStart:lineEnd])
}
