package vim

import (
	"testing"
)

// helper to build a VimState in normal mode with given buffer and cursor.
func normalState(buf string, cursor int) *VimState {
	s := &VimState{
		Mode:   ModeNormal,
		Buffer: []rune(buf),
		Cursor: cursor,
	}
	// Reset global normalCtx between tests.
	nctx = normalCtx{}
	return s
}

// ─── Motion Tests ───

func TestApplyMotion_LeftRight(t *testing.T) {
	s := normalState("hello", 2)

	// h moves left
	if got := ApplyMotion(s, MotionLeft, 1); got != 1 {
		t.Errorf("h from 2: got %d, want 1", got)
	}
	// l moves right
	if got := ApplyMotion(s, MotionRight, 1); got != 3 {
		t.Errorf("l from 2: got %d, want 3", got)
	}
}

func TestApplyMotion_LeftClamp(t *testing.T) {
	s := normalState("abc", 0)
	if got := ApplyMotion(s, MotionLeft, 1); got != 0 {
		t.Errorf("h at 0: got %d, want 0", got)
	}
}

func TestApplyMotion_RightClamp(t *testing.T) {
	s := normalState("abc", 2)
	if got := ApplyMotion(s, MotionRight, 1); got != 2 {
		t.Errorf("l at end: got %d, want 2", got)
	}
}

func TestApplyMotion_WordForward(t *testing.T) {
	s := normalState("hello world foo", 0)
	got := ApplyMotion(s, MotionWordForward, 1)
	if got != 6 {
		t.Errorf("w from 0: got %d, want 6", got)
	}
}

func TestApplyMotion_WordBack(t *testing.T) {
	s := normalState("hello world", 6)
	got := ApplyMotion(s, MotionWordBack, 1)
	if got != 0 {
		t.Errorf("b from 6: got %d, want 0", got)
	}
}

func TestApplyMotion_WordEnd(t *testing.T) {
	s := normalState("hello world", 0)
	got := ApplyMotion(s, MotionWordEnd, 1)
	if got != 4 {
		t.Errorf("e from 0: got %d, want 4", got)
	}
}

func TestApplyMotion_WordForwardMixed(t *testing.T) {
	// word followed by punctuation
	s := normalState("foo.bar baz", 0)
	// w from 'f' should jump to '.' (different char class)
	got := ApplyMotion(s, MotionWordForward, 1)
	if got != 3 {
		t.Errorf("w mixed from 0: got %d, want 3", got)
	}
}

func TestApplyMotion_LineStart(t *testing.T) {
	s := normalState("hello world", 7)
	if got := ApplyMotion(s, MotionLineStart, 1); got != 0 {
		t.Errorf("0 from 7: got %d, want 0", got)
	}
}

func TestApplyMotion_LineEnd(t *testing.T) {
	s := normalState("hello world", 0)
	got := ApplyMotion(s, MotionLineEnd, 1)
	want := len("hello world") - 1
	if got != want {
		t.Errorf("$ from 0: got %d, want %d", got, want)
	}
}

func TestApplyMotion_DocStartEnd(t *testing.T) {
	s := normalState("hello world", 5)
	if got := ApplyMotion(s, MotionDocStart, 1); got != 0 {
		t.Errorf("gg: got %d, want 0", got)
	}
	if got := ApplyMotion(s, MotionDocEnd, 1); got != 10 {
		t.Errorf("G: got %d, want 10", got)
	}
}

func TestApplyMotion_FindChar(t *testing.T) {
	s := normalState("hello world", 0)
	s.FindChar = 'o'
	got := ApplyMotion(s, MotionFindChar, 1)
	if got != 4 {
		t.Errorf("fo from 0: got %d, want 4", got)
	}
}

func TestApplyMotion_FindCharBack(t *testing.T) {
	s := normalState("hello world", 10)
	s.FindChar = 'o'
	got := ApplyMotion(s, MotionFindCharBack, 1)
	if got != 7 {
		t.Errorf("Fo from 10: got %d, want 7", got)
	}
}

func TestApplyMotion_CountMultiplier(t *testing.T) {
	s := normalState("one two three four", 0)
	got := ApplyMotion(s, MotionWordForward, 3)
	// Should skip 3 words: one→two→three→four
	if got != 14 {
		t.Errorf("3w from 0: got %d, want 14", got)
	}
}

func TestApplyMotion_EmptyBuffer(t *testing.T) {
	s := normalState("", 0)
	if got := ApplyMotion(s, MotionLeft, 1); got != 0 {
		t.Errorf("h on empty: got %d, want 0", got)
	}
	if got := ApplyMotion(s, MotionRight, 1); got != 0 {
		t.Errorf("l on empty: got %d, want 0", got)
	}
}


// ─── Operator Tests ───

func TestOperator_DeleteWord(t *testing.T) {
	s := normalState("hello world", 0)
	ApplyOperatorMotion(s, OpDelete, MotionWordForward, 1)
	got := string(s.Buffer)
	// w motion lands on 'w' of "world" (index 6), operator range is [0,7) = "hello w"
	if got != "orld" {
		t.Errorf("dw: buffer=%q, want %q", got, "orld")
	}
	if s.Yanked != "hello w" {
		t.Errorf("dw: yanked=%q, want %q", s.Yanked, "hello w")
	}
	if s.Mode != ModeNormal {
		t.Errorf("dw: mode=%v, want Normal", s.Mode)
	}
}

func TestOperator_ChangeWord(t *testing.T) {
	s := normalState("hello world", 0)
	ApplyOperatorMotion(s, OpChange, MotionWordForward, 1)
	got := string(s.Buffer)
	if got != "orld" {
		t.Errorf("cw: buffer=%q, want %q", got, "orld")
	}
	if s.Yanked != "hello w" {
		t.Errorf("cw: yanked=%q, want %q", s.Yanked, "hello w")
	}
	if s.Mode != ModeInsert {
		t.Errorf("cw: mode=%v, want Insert", s.Mode)
	}
}

func TestOperator_YankWord(t *testing.T) {
	s := normalState("hello world", 0)
	ApplyOperatorMotion(s, OpYank, MotionWordForward, 1)
	got := string(s.Buffer)
	if got != "hello world" {
		t.Errorf("yw: buffer=%q, want unchanged", got)
	}
	if s.Yanked != "hello w" {
		t.Errorf("yw: yanked=%q, want %q", s.Yanked, "hello w")
	}
}

func TestOperator_DeleteLine(t *testing.T) {
	s := normalState("hello world", 3)
	DeleteLine(s)
	if len(s.Buffer) != 0 {
		t.Errorf("dd: buffer=%q, want empty", string(s.Buffer))
	}
	if s.Yanked != "hello world" {
		t.Errorf("dd: yanked=%q, want %q", s.Yanked, "hello world")
	}
}

func TestOperator_YankLine(t *testing.T) {
	s := normalState("hello world", 3)
	YankLine(s)
	if string(s.Buffer) != "hello world" {
		t.Errorf("yy: buffer should be unchanged")
	}
	if s.Yanked != "hello world" {
		t.Errorf("yy: yanked=%q, want %q", s.Yanked, "hello world")
	}
}

func TestOperator_DeleteChar(t *testing.T) {
	s := normalState("hello", 0)
	DeleteChar(s)
	got := string(s.Buffer)
	if got != "ello" {
		t.Errorf("x: buffer=%q, want %q", got, "ello")
	}
	if s.Yanked != "h" {
		t.Errorf("x: yanked=%q, want %q", s.Yanked, "h")
	}
}

func TestOperator_Paste(t *testing.T) {
	s := normalState("hllo", 0)
	s.Yanked = "e"
	Paste(s)
	got := string(s.Buffer)
	if got != "hello" {
		t.Errorf("p: buffer=%q, want %q", got, "hello")
	}
}

func TestOperator_ReplaceChar(t *testing.T) {
	s := normalState("hello", 0)
	ReplaceChar(s, 'H')
	got := string(s.Buffer)
	if got != "Hello" {
		t.Errorf("r: buffer=%q, want %q", got, "Hello")
	}
}

// ─── Text Object Tests ───

func TestTextObject_InnerWord(t *testing.T) {
	buf := []rune("hello world")
	start, end := ResolveTextObjectRange(buf, 1, TextObjInnerWord)
	got := string(buf[start:end])
	if got != "hello" {
		t.Errorf("iw: got %q, want %q", got, "hello")
	}
}

func TestTextObject_AWord(t *testing.T) {
	buf := []rune("hello world")
	start, end := ResolveTextObjectRange(buf, 1, TextObjAWord)
	got := string(buf[start:end])
	if got != "hello " {
		t.Errorf("aw: got %q, want %q", got, "hello ")
	}
}

func TestTextObject_InnerQuote(t *testing.T) {
	buf := []rune(`say "hello" now`)
	// cursor on 'h' inside quotes (index 5)
	start, end := ResolveTextObjectRange(buf, 5, TextObjInnerQuote)
	got := string(buf[start:end])
	if got != "hello" {
		t.Errorf("i\": got %q, want %q", got, "hello")
	}
}

func TestTextObject_AQuote(t *testing.T) {
	buf := []rune(`say "hello" now`)
	start, end := ResolveTextObjectRange(buf, 5, TextObjAQuote)
	got := string(buf[start:end])
	if got != `"hello"` {
		t.Errorf("a\": got %q, want %q", got, `"hello"`)
	}
}

func TestTextObject_InnerParen(t *testing.T) {
	buf := []rune("foo(bar baz)end")
	// cursor on 'b' at index 4
	start, end := ResolveTextObjectRange(buf, 4, TextObjInnerParen)
	got := string(buf[start:end])
	if got != "bar baz" {
		t.Errorf("i(: got %q, want %q", got, "bar baz")
	}
}

func TestTextObject_AParen(t *testing.T) {
	buf := []rune("foo(bar baz)end")
	start, end := ResolveTextObjectRange(buf, 4, TextObjAParen)
	got := string(buf[start:end])
	if got != "(bar baz)" {
		t.Errorf("a(: got %q, want %q", got, "(bar baz)")
	}
}


// ─── State Transition Tests (HandleKey) ───

func TestHandleKey_InsertTyping(t *testing.T) {
	s := NewVimState() // starts in insert mode
	nctx = normalCtx{}
	HandleKey(s, 'h')
	HandleKey(s, 'i')
	got := string(s.Buffer)
	if got != "hi" {
		t.Errorf("insert typing: got %q, want %q", got, "hi")
	}
	if s.Cursor != 2 {
		t.Errorf("insert cursor: got %d, want 2", s.Cursor)
	}
}

func TestHandleKey_InsertBackspace(t *testing.T) {
	s := NewVimState()
	nctx = normalCtx{}
	HandleKey(s, 'a')
	HandleKey(s, 'b')
	HandleKey(s, 127) // backspace
	got := string(s.Buffer)
	if got != "a" {
		t.Errorf("backspace: got %q, want %q", got, "a")
	}
}

func TestHandleKey_InsertEscapeToNormal(t *testing.T) {
	s := NewVimState()
	nctx = normalCtx{}
	HandleKey(s, 'x')
	HandleKey(s, 27) // Escape
	if s.Mode != ModeNormal {
		t.Errorf("Esc: mode=%v, want Normal", s.Mode)
	}
}

func TestHandleKey_InsertEnterSubmit(t *testing.T) {
	s := NewVimState()
	nctx = normalCtx{}
	HandleKey(s, 'h')
	HandleKey(s, 'i')
	submit, quit := HandleKey(s, '\n')
	if !submit {
		t.Error("Enter in insert: expected submit=true")
	}
	if quit {
		t.Error("Enter in insert: expected quit=false")
	}
}

func TestHandleKey_NormalI_ToInsert(t *testing.T) {
	s := normalState("hello", 2)
	HandleKey(s, 'i')
	if s.Mode != ModeInsert {
		t.Errorf("i: mode=%v, want Insert", s.Mode)
	}
}

func TestHandleKey_NormalMotionKeys(t *testing.T) {
	s := normalState("hello world", 0)
	HandleKey(s, 'w')
	if s.Cursor != 6 {
		t.Errorf("w: cursor=%d, want 6", s.Cursor)
	}
	HandleKey(s, 'b')
	if s.Cursor != 0 {
		t.Errorf("b: cursor=%d, want 0", s.Cursor)
	}
}

func TestHandleKey_NormalOperatorToPending(t *testing.T) {
	s := normalState("hello world", 0)
	HandleKey(s, 'd')
	if s.Mode != ModeOperatorPending {
		t.Errorf("d: mode=%v, want OperatorPending", s.Mode)
	}
	if s.PendingOp != OpDelete {
		t.Errorf("d: pendingOp=%v, want OpDelete", s.PendingOp)
	}
}

func TestHandleKey_OperatorPending_dd(t *testing.T) {
	s := normalState("hello world", 3)
	HandleKey(s, 'd')
	HandleKey(s, 'd')
	if len(s.Buffer) != 0 {
		t.Errorf("dd: buffer=%q, want empty", string(s.Buffer))
	}
	if s.Mode != ModeNormal {
		t.Errorf("dd: mode=%v, want Normal", s.Mode)
	}
}

func TestHandleKey_OperatorPending_dw(t *testing.T) {
	s := normalState("hello world", 0)
	HandleKey(s, 'd')
	HandleKey(s, 'w')
	got := string(s.Buffer)
	if got != "orld" {
		t.Errorf("dw: buffer=%q, want %q", got, "orld")
	}
}

func TestHandleKey_OperatorPending_EscapeCancel(t *testing.T) {
	s := normalState("hello", 0)
	HandleKey(s, 'd')
	if s.Mode != ModeOperatorPending {
		t.Fatalf("d: mode=%v, want OperatorPending", s.Mode)
	}
	HandleKey(s, 27) // Escape
	if s.Mode != ModeNormal {
		t.Errorf("Esc cancel: mode=%v, want Normal", s.Mode)
	}
	if s.PendingOp != OpNone {
		t.Errorf("Esc cancel: pendingOp=%v, want OpNone", s.PendingOp)
	}
}

func TestHandleKey_SearchMode(t *testing.T) {
	s := normalState("hello world", 0)
	// Enter search mode
	HandleKey(s, '/')
	if s.Mode != ModeSearch {
		t.Fatalf("/: mode=%v, want Search", s.Mode)
	}
	// Type query
	HandleKey(s, 'w')
	HandleKey(s, 'o')
	if s.SearchBuf != "wo" {
		t.Errorf("search typing: buf=%q, want %q", s.SearchBuf, "wo")
	}
	// Execute search
	HandleKey(s, '\n')
	if s.Mode != ModeNormal {
		t.Errorf("search Enter: mode=%v, want Normal", s.Mode)
	}
	if s.Cursor != 6 {
		t.Errorf("search result: cursor=%d, want 6", s.Cursor)
	}
}

func TestHandleKey_SearchEscapeCancel(t *testing.T) {
	s := normalState("hello world", 0)
	HandleKey(s, '/')
	HandleKey(s, 'x')
	HandleKey(s, 27) // Escape
	if s.Mode != ModeNormal {
		t.Errorf("search Esc: mode=%v, want Normal", s.Mode)
	}
	if s.SearchBuf != "" {
		t.Errorf("search Esc: buf=%q, want empty", s.SearchBuf)
	}
}

func TestHandleKey_ColonW_Submit(t *testing.T) {
	s := normalState("hello", 0)
	HandleKey(s, ':')
	HandleKey(s, 'w')
	submit, quit := HandleKey(s, '\n')
	if !submit {
		t.Error(":w: expected submit=true")
	}
	if quit {
		t.Error(":w: expected quit=false")
	}
}

func TestHandleKey_ColonQ_Quit(t *testing.T) {
	s := normalState("hello", 0)
	HandleKey(s, ':')
	HandleKey(s, 'q')
	submit, quit := HandleKey(s, '\n')
	if submit {
		t.Error(":q: expected submit=false")
	}
	if !quit {
		t.Error(":q: expected quit=true")
	}
}

func TestHandleKey_ColonWQ(t *testing.T) {
	s := normalState("hello", 0)
	HandleKey(s, ':')
	HandleKey(s, 'w')
	HandleKey(s, 'q')
	submit, quit := HandleKey(s, '\n')
	if !submit {
		t.Error(":wq: expected submit=true")
	}
	if !quit {
		t.Error(":wq: expected quit=true")
	}
}

// ─── Type Tests ───

func TestVimMode_String(t *testing.T) {
	tests := []struct {
		mode VimMode
		want string
	}{
		{ModeNormal, "NORMAL"},
		{ModeInsert, "INSERT"},
		{ModeVisual, "VISUAL"},
		{ModeOperatorPending, "OP-PENDING"},
		{ModeSearch, "SEARCH"},
		{VimMode(99), "UNKNOWN"},
	}
	for _, tt := range tests {
		if got := tt.mode.String(); got != tt.want {
			t.Errorf("VimMode(%d).String()=%q, want %q", tt.mode, got, tt.want)
		}
	}
}

func TestNewVimState_StartsInsert(t *testing.T) {
	s := NewVimState()
	if s.Mode != ModeInsert {
		t.Errorf("NewVimState: mode=%v, want Insert", s.Mode)
	}
}

func TestNewPersistentState_StartsDisabled(t *testing.T) {
	ps := NewPersistentState()
	if ps.Enabled {
		t.Error("NewPersistentState: expected Enabled=false")
	}
}
