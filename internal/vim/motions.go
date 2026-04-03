package vim

import "unicode"

// clampCursor clamps pos to [0, bufLen-1], or 0 if the buffer is empty.
func clampCursor(pos, bufLen int) int {
	if bufLen == 0 {
		return 0
	}
	if pos < 0 {
		return 0
	}
	if pos >= bufLen {
		return bufLen - 1
	}
	return pos
}

// ApplyMotion moves the cursor according to the given motion.
// Returns the new cursor position (clamped to buffer bounds).
// For single-line input buffers (no newlines), j/k are no-ops.
func ApplyMotion(s *VimState, m Motion, count int) int {
	if count < 1 {
		count = 1
	}
	bufLen := len(s.Buffer)
	cur := s.Cursor

	switch m {
	case MotionLeft:
		return clampCursor(cur-count, bufLen)

	case MotionRight:
		return clampCursor(cur+count, bufLen)

	case MotionDown:
		return moveDown(s.Buffer, cur, count)

	case MotionUp:
		return moveUp(s.Buffer, cur, count)

	case MotionWordForward:
		return moveWordForward(s.Buffer, cur, count)

	case MotionWordBack:
		return moveWordBack(s.Buffer, cur, count)

	case MotionWordEnd:
		return moveWordEnd(s.Buffer, cur, count)

	case MotionLineStart:
		return moveLineStart(s.Buffer, cur)

	case MotionLineEnd:
		return moveLineEnd(s.Buffer, cur)

	case MotionDocStart:
		return 0

	case MotionDocEnd:
		return clampCursor(bufLen-1, bufLen)

	case MotionFindChar:
		return moveFindChar(s.Buffer, cur, s.FindChar, true, count)

	case MotionFindCharBack:
		return moveFindChar(s.Buffer, cur, s.FindChar, false, count)

	default:
		return cur
	}
}

// isWordChar returns true for [a-zA-Z0-9_].
func isWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// charClass returns a classification for word-boundary detection:
//
//	0 = whitespace, 1 = word char, 2 = punctuation/symbol
func charClass(r rune) int {
	if unicode.IsSpace(r) {
		return 0
	}
	if isWordChar(r) {
		return 1
	}
	return 2
}

// moveWordForward moves forward count words (vim 'w' motion).
func moveWordForward(buf []rune, pos, count int) int {
	n := len(buf)
	for i := 0; i < count && pos < n-1; i++ {
		// Skip current word/punct class
		cls := charClass(buf[pos])
		for pos < n-1 && charClass(buf[pos]) == cls {
			pos++
		}
		// Skip whitespace
		for pos < n-1 && unicode.IsSpace(buf[pos]) {
			pos++
		}
	}
	return clampCursor(pos, n)
}

// moveWordBack moves backward count words (vim 'b' motion).
func moveWordBack(buf []rune, pos, count int) int {
	for i := 0; i < count && pos > 0; i++ {
		// Move back past any whitespace
		for pos > 0 && unicode.IsSpace(buf[pos-1]) {
			pos--
		}
		if pos == 0 {
			break
		}
		// Move back through the current word/punct class
		cls := charClass(buf[pos-1])
		for pos > 0 && charClass(buf[pos-1]) == cls {
			pos--
		}
	}
	return clampCursor(pos, len(buf))
}

// moveWordEnd moves to end of current/next word count times (vim 'e' motion).
func moveWordEnd(buf []rune, pos, count int) int {
	n := len(buf)
	for i := 0; i < count && pos < n-1; i++ {
		// Move at least one position forward
		pos++
		// Skip whitespace
		for pos < n-1 && unicode.IsSpace(buf[pos]) {
			pos++
		}
		// Move to end of current word/punct class
		if pos < n-1 {
			cls := charClass(buf[pos])
			for pos < n-1 && charClass(buf[pos+1]) == cls {
				pos++
			}
		}
	}
	return clampCursor(pos, n)
}

// moveLineStart finds the start of the current line (after previous \n or buffer start).
func moveLineStart(buf []rune, pos int) int {
	for pos > 0 && buf[pos-1] != '\n' {
		pos--
	}
	return pos
}

// moveLineEnd finds the end of the current line (before next \n or buffer end).
func moveLineEnd(buf []rune, pos int) int {
	n := len(buf)
	if n == 0 {
		return 0
	}
	for pos < n-1 && buf[pos+1] != '\n' {
		pos++
	}
	return pos
}

// moveDown moves the cursor down count lines in a multi-line buffer.
func moveDown(buf []rune, pos, count int) int {
	n := len(buf)
	if n == 0 {
		return 0
	}
	// Find column offset from start of current line
	col := 0
	for i := pos - 1; i >= 0 && buf[i] != '\n'; i-- {
		col++
	}
	cur := pos
	for i := 0; i < count; i++ {
		// Find end of current line (next \n or end of buffer)
		eol := cur
		for eol < n && buf[eol] != '\n' {
			eol++
		}
		if eol >= n {
			// Already on last line, no-op
			break
		}
		// Move past the \n to start of next line
		nextLineStart := eol + 1
		// Find end of next line
		nextLineEnd := nextLineStart
		for nextLineEnd < n && buf[nextLineEnd] != '\n' {
			nextLineEnd++
		}
		// Place cursor at same column or end of next line
		target := nextLineStart + col
		if target >= nextLineEnd {
			target = nextLineEnd - 1
			if target < nextLineStart {
				target = nextLineStart
			}
		}
		cur = target
	}
	return clampCursor(cur, n)
}

// moveUp moves the cursor up count lines in a multi-line buffer.
func moveUp(buf []rune, pos, count int) int {
	n := len(buf)
	if n == 0 {
		return 0
	}
	// Find column offset from start of current line
	col := 0
	for i := pos - 1; i >= 0 && buf[i] != '\n'; i-- {
		col++
	}
	cur := pos
	for i := 0; i < count; i++ {
		// Find start of current line
		lineStart := cur
		for lineStart > 0 && buf[lineStart-1] != '\n' {
			lineStart--
		}
		if lineStart == 0 {
			// Already on first line, no-op
			break
		}
		// Move to end of previous line (the \n before lineStart)
		prevLineEnd := lineStart - 1 // this is the \n character
		// Find start of previous line
		prevLineStart := prevLineEnd
		for prevLineStart > 0 && buf[prevLineStart-1] != '\n' {
			prevLineStart--
		}
		// Place cursor at same column or end of previous line
		target := prevLineStart + col
		if target >= prevLineEnd {
			target = prevLineEnd - 1
			if target < prevLineStart {
				target = prevLineStart
			}
		}
		cur = target
	}
	return clampCursor(cur, n)
}

// moveFindChar moves to the next/previous occurrence of ch.
func moveFindChar(buf []rune, pos int, ch rune, forward bool, count int) int {
	n := len(buf)
	cur := pos
	for i := 0; i < count; i++ {
		if forward {
			found := false
			for j := cur + 1; j < n; j++ {
				if buf[j] == ch {
					cur = j
					found = true
					break
				}
			}
			if !found {
				return cur // stay at last found or original
			}
		} else {
			found := false
			for j := cur - 1; j >= 0; j-- {
				if buf[j] == ch {
					cur = j
					found = true
					break
				}
			}
			if !found {
				return cur
			}
		}
	}
	return clampCursor(cur, n)
}
