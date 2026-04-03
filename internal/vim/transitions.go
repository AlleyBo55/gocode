package vim

import (
	"strconv"
	"strings"
)

// HandleKey processes a single keypress and updates the VimState.
// Returns:
// - submit: true if the user wants to submit the input (:w or Enter in insert mode)
// - quit: true if the user wants to quit (:q)
func HandleKey(s *VimState, key rune) (submit, quit bool) {
	switch s.Mode {
	case ModeInsert:
		return handleInsert(s, key)
	case ModeNormal:
		return handleNormal(s, key)
	case ModeOperatorPending:
		return handleOperatorPending(s, key)
	case ModeSearch:
		return handleSearch(s, key)
	case ModeVisual:
		return handleVisual(s, key)
	}
	return false, false
}

// --- Insert Mode ---

func handleInsert(s *VimState, key rune) (submit, quit bool) {
	switch key {
	case 27: // Escape
		s.Mode = ModeNormal
		if s.Cursor > 0 && s.Cursor >= len(s.Buffer) {
			s.Cursor = len(s.Buffer) - 1
		}
		return false, false
	case '\n', '\r': // Enter → submit
		return true, false
	case 127, 8: // Backspace (DEL or BS)
		if s.Cursor > 0 {
			s.Buffer = append(s.Buffer[:s.Cursor-1], s.Buffer[s.Cursor:]...)
			s.Cursor--
		}
		return false, false
	default:
		// Insert character at cursor position
		newBuf := make([]rune, 0, len(s.Buffer)+1)
		newBuf = append(newBuf, s.Buffer[:s.Cursor]...)
		newBuf = append(newBuf, key)
		newBuf = append(newBuf, s.Buffer[s.Cursor:]...)
		s.Buffer = newBuf
		s.Cursor++
		return false, false
	}
}

// --- Normal Mode multi-key state ---

type normalCtx struct {
	waitingG       bool
	waitingF       bool
	findForward    bool
	waitingColon   bool
	colonBuf       string
	waitingTextObj bool   // in operator-pending: waiting for text object char after i/a
	textObjInner   bool   // true = inner (i), false = around (a)
}

var nctx normalCtx

func handleNormal(s *VimState, key rune) (submit, quit bool) {
	// Handle waiting-for-char states first
	if s.ReplaceChar {
		s.ReplaceChar = false
		ReplaceChar(s, key)
		return false, false
	}
	if nctx.waitingF {
		nctx.waitingF = false
		s.FindChar = key
		s.FindForward = nctx.findForward
		count := resolveCount(s)
		if s.FindForward {
			s.Cursor = ApplyMotion(s, MotionFindChar, count)
		} else {
			s.Cursor = ApplyMotion(s, MotionFindCharBack, count)
		}
		return false, false
	}
	if nctx.waitingG {
		nctx.waitingG = false
		if key == 'g' {
			s.Cursor = ApplyMotion(s, MotionDocStart, 1)
			resolveCount(s) // clear count
		}
		return false, false
	}
	if nctx.waitingColon {
		return handleColonInput(s, key)
	}

	switch {
	// --- Mode transitions to Insert ---
	case key == 'i':
		clearPending(s)
		s.Mode = ModeInsert
	case key == 'a':
		clearPending(s)
		if len(s.Buffer) > 0 {
			s.Cursor++
			if s.Cursor > len(s.Buffer) {
				s.Cursor = len(s.Buffer)
			}
		}
		s.Mode = ModeInsert
	case key == 'A':
		clearPending(s)
		s.Cursor = moveLineEnd(s.Buffer, s.Cursor)
		if len(s.Buffer) > 0 {
			s.Cursor++
			if s.Cursor > len(s.Buffer) {
				s.Cursor = len(s.Buffer)
			}
		}
		s.Mode = ModeInsert
	case key == 'I':
		clearPending(s)
		s.Cursor = moveLineStart(s.Buffer, s.Cursor)
		s.Mode = ModeInsert
	case key == 'o':
		clearPending(s)
		eol := s.Cursor
		for eol < len(s.Buffer) && s.Buffer[eol] != '\n' {
			eol++
		}
		newBuf := make([]rune, 0, len(s.Buffer)+1)
		newBuf = append(newBuf, s.Buffer[:eol]...)
		newBuf = append(newBuf, '\n')
		newBuf = append(newBuf, s.Buffer[eol:]...)
		s.Buffer = newBuf
		s.Cursor = eol + 1
		s.Mode = ModeInsert
	case key == 'O':
		clearPending(s)
		lineStart := moveLineStart(s.Buffer, s.Cursor)
		newBuf := make([]rune, 0, len(s.Buffer)+1)
		newBuf = append(newBuf, s.Buffer[:lineStart]...)
		newBuf = append(newBuf, '\n')
		newBuf = append(newBuf, s.Buffer[lineStart:]...)
		s.Buffer = newBuf
		s.Cursor = lineStart
		s.Mode = ModeInsert

	// --- Escape ---
	case key == 27:
		clearPending(s)

	// --- Count prefix ---
	case key >= '1' && key <= '9':
		s.CountBuf += string(key)
	case key == '0':
		if s.CountBuf == "" {
			s.Cursor = ApplyMotion(s, MotionLineStart, 1)
		} else {
			s.CountBuf += string(key)
		}

	// --- Basic motions ---
	case key == 'h':
		s.Cursor = ApplyMotion(s, MotionLeft, resolveCount(s))
	case key == 'j':
		s.Cursor = ApplyMotion(s, MotionDown, resolveCount(s))
	case key == 'k':
		s.Cursor = ApplyMotion(s, MotionUp, resolveCount(s))
	case key == 'l':
		s.Cursor = ApplyMotion(s, MotionRight, resolveCount(s))

	// --- Word motions ---
	case key == 'w':
		s.Cursor = ApplyMotion(s, MotionWordForward, resolveCount(s))
	case key == 'b':
		s.Cursor = ApplyMotion(s, MotionWordBack, resolveCount(s))
	case key == 'e':
		s.Cursor = ApplyMotion(s, MotionWordEnd, resolveCount(s))

	// --- Line/doc motions ---
	case key == '$':
		resolveCount(s)
		s.Cursor = ApplyMotion(s, MotionLineEnd, 1)
	case key == 'g':
		nctx.waitingG = true
	case key == 'G':
		resolveCount(s)
		s.Cursor = ApplyMotion(s, MotionDocEnd, 1)

	// --- Find char ---
	case key == 'f':
		nctx.waitingF = true
		nctx.findForward = true
	case key == 'F':
		nctx.waitingF = true
		nctx.findForward = false

	// --- Operators ---
	case key == 'd':
		s.PendingOp = OpDelete
		s.Count = resolveCount(s)
		s.Mode = ModeOperatorPending
	case key == 'c':
		s.PendingOp = OpChange
		s.Count = resolveCount(s)
		s.Mode = ModeOperatorPending
	case key == 'y':
		s.PendingOp = OpYank
		s.Count = resolveCount(s)
		s.Mode = ModeOperatorPending

	// --- Single-key operations ---
	case key == 'p':
		clearPending(s)
		Paste(s)
	case key == 'x':
		clearPending(s)
		DeleteChar(s)
	case key == 'r':
		s.ReplaceChar = true

	// --- Search ---
	case key == '/':
		clearPending(s)
		s.Mode = ModeSearch
		s.SearchDir = 1
		s.SearchBuf = ""
	case key == '?':
		clearPending(s)
		s.Mode = ModeSearch
		s.SearchDir = -1
		s.SearchBuf = ""

	// --- Ex commands ---
	case key == ':':
		clearPending(s)
		nctx.waitingColon = true
		nctx.colonBuf = ""
	}

	return false, false
}

// --- Operator Pending Mode ---

func handleOperatorPending(s *VimState, key rune) (submit, quit bool) {
	op := s.PendingOp
	count := s.Count
	if count < 1 {
		count = 1
	}

	// If waiting for text object char (after i or a in operator-pending)
	if nctx.waitingTextObj {
		nctx.waitingTextObj = false
		obj := resolveTextObjKey(key, nctx.textObjInner)
		if obj != TextObjNone {
			ApplyOperatorTextObject(s, op, obj)
		} else {
			s.PendingOp = OpNone
			s.Mode = ModeNormal
			clearPending(s)
		}
		return false, false
	}

	switch {
	// Same operator again -> line operation (dd, cc, yy)
	case key == 'd' && op == OpDelete:
		DeleteLine(s)
		s.PendingOp = OpNone
		s.Mode = ModeNormal
	case key == 'c' && op == OpChange:
		DeleteLine(s)
		s.PendingOp = OpNone
		s.Mode = ModeInsert
	case key == 'y' && op == OpYank:
		YankLine(s)
		s.PendingOp = OpNone
		s.Mode = ModeNormal

	// Escape -> cancel
	case key == 27:
		s.PendingOp = OpNone
		s.Mode = ModeNormal
		clearPending(s)

	// Text objects: i/a prefix
	case key == 'i':
		nctx.waitingTextObj = true
		nctx.textObjInner = true
	case key == 'a':
		nctx.waitingTextObj = true
		nctx.textObjInner = false

	// Motion keys
	case key == 'h':
		ApplyOperatorMotion(s, op, MotionLeft, count)
	case key == 'j':
		ApplyOperatorMotion(s, op, MotionDown, count)
	case key == 'k':
		ApplyOperatorMotion(s, op, MotionUp, count)
	case key == 'l':
		ApplyOperatorMotion(s, op, MotionRight, count)
	case key == 'w':
		ApplyOperatorMotion(s, op, MotionWordForward, count)
	case key == 'b':
		ApplyOperatorMotion(s, op, MotionWordBack, count)
	case key == 'e':
		ApplyOperatorMotion(s, op, MotionWordEnd, count)
	case key == '0':
		ApplyOperatorMotion(s, op, MotionLineStart, count)
	case key == '$':
		ApplyOperatorMotion(s, op, MotionLineEnd, count)
	case key == 'G':
		ApplyOperatorMotion(s, op, MotionDocEnd, count)

	default:
		// Unknown key -> cancel operator
		s.PendingOp = OpNone
		s.Mode = ModeNormal
		clearPending(s)
	}

	return false, false
}

// resolveTextObjKey maps a char after i/a to a TextObject.
func resolveTextObjKey(key rune, inner bool) TextObject {
	if inner {
		switch key {
		case 'w':
			return TextObjInnerWord
		case '"':
			return TextObjInnerQuote
		case '(', ')':
			return TextObjInnerParen
		}
	} else {
		switch key {
		case 'w':
			return TextObjAWord
		case '"':
			return TextObjAQuote
		case '(', ')':
			return TextObjAParen
		}
	}
	return TextObjNone
}

// --- Search Mode ---

func handleSearch(s *VimState, key rune) (submit, quit bool) {
	switch key {
	case '\n', '\r':
		if s.SearchBuf != "" {
			s.LastSearch = s.SearchBuf
			executeSearch(s)
		}
		s.Mode = ModeNormal
	case 27:
		s.SearchBuf = ""
		s.Mode = ModeNormal
	case 127, 8:
		if len(s.SearchBuf) > 0 {
			s.SearchBuf = s.SearchBuf[:len(s.SearchBuf)-1]
		}
	default:
		s.SearchBuf += string(key)
	}
	return false, false
}

func executeSearch(s *VimState) {
	if s.SearchBuf == "" {
		return
	}
	target := []rune(s.SearchBuf)
	buf := s.Buffer
	n := len(buf)
	tLen := len(target)
	if tLen == 0 || n == 0 {
		return
	}

	if s.SearchDir >= 0 {
		// Forward: start after cursor
		for i := s.Cursor + 1; i <= n-tLen; i++ {
			if matchAt(buf, i, target) {
				s.Cursor = i
				return
			}
		}
		// Wrap around
		for i := 0; i <= s.Cursor && i <= n-tLen; i++ {
			if matchAt(buf, i, target) {
				s.Cursor = i
				return
			}
		}
	} else {
		// Backward: start before cursor
		for i := s.Cursor - 1; i >= 0; i-- {
			if i+tLen <= n && matchAt(buf, i, target) {
				s.Cursor = i
				return
			}
		}
		// Wrap around
		for i := n - tLen; i > s.Cursor; i-- {
			if i >= 0 && matchAt(buf, i, target) {
				s.Cursor = i
				return
			}
		}
	}
}

func matchAt(buf []rune, pos int, target []rune) bool {
	for i, r := range target {
		if buf[pos+i] != r {
			return false
		}
	}
	return true
}

// --- Visual Mode ---

func handleVisual(s *VimState, key rune) (submit, quit bool) {
	switch key {
	case 27:
		s.Mode = ModeNormal
	case 'd':
		DeleteChar(s)
		s.Mode = ModeNormal
	case 'y':
		if s.Cursor < len(s.Buffer) {
			s.Yanked = string(s.Buffer[s.Cursor : s.Cursor+1])
		}
		s.Mode = ModeNormal
	case 'c':
		DeleteChar(s)
		s.Mode = ModeInsert
	}
	return false, false
}

// --- Colon command handling ---

func handleColonInput(s *VimState, key rune) (submit, quit bool) {
	switch key {
	case '\n', '\r':
		cmd := strings.TrimSpace(nctx.colonBuf)
		nctx.waitingColon = false
		nctx.colonBuf = ""
		return executeColonCmd(cmd)
	case 27:
		nctx.waitingColon = false
		nctx.colonBuf = ""
	case 127, 8:
		if len(nctx.colonBuf) > 0 {
			nctx.colonBuf = nctx.colonBuf[:len(nctx.colonBuf)-1]
		}
	default:
		nctx.colonBuf += string(key)
	}
	return false, false
}

func executeColonCmd(cmd string) (submit, quit bool) {
	switch cmd {
	case "w":
		return true, false
	case "q":
		return false, true
	case "wq", "x":
		return true, true
	}
	return false, false
}

// --- Helpers ---

func resolveCount(s *VimState) int {
	if s.CountBuf == "" {
		return 1
	}
	n, err := strconv.Atoi(s.CountBuf)
	s.CountBuf = ""
	if err != nil || n < 1 {
		return 1
	}
	return n
}

func clearPending(s *VimState) {
	s.PendingOp = OpNone
	s.Count = 0
	s.CountBuf = ""
	s.ReplaceChar = false
	nctx = normalCtx{}
}
