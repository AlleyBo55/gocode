package vim

// VimMode represents the current editing mode.
type VimMode int

const (
	ModeNormal          VimMode = iota
	ModeInsert                  // default for new input
	ModeVisual                  // visual selection
	ModeOperatorPending         // waiting for motion/text-object after d/c/y
	ModeSearch                  // / or ? search mode
)

// String returns the mode name for display.
func (m VimMode) String() string {
	switch m {
	case ModeNormal:
		return "NORMAL"
	case ModeInsert:
		return "INSERT"
	case ModeVisual:
		return "VISUAL"
	case ModeOperatorPending:
		return "OP-PENDING"
	case ModeSearch:
		return "SEARCH"
	default:
		return "UNKNOWN"
	}
}

// Operator represents a pending operator (d, c, y).
type Operator int

const (
	OpNone   Operator = iota
	OpDelete          // d
	OpChange          // c
	OpYank            // y
)

// Motion represents a cursor motion command.
type Motion int

const (
	MotionNone         Motion = iota
	MotionLeft                // h
	MotionDown                // j
	MotionUp                  // k
	MotionRight               // l
	MotionWordForward          // w
	MotionWordBack             // b
	MotionWordEnd              // e
	MotionLineStart            // 0
	MotionLineEnd              // $
	MotionDocStart             // gg
	MotionDocEnd               // G
	MotionFindChar             // f{char}
	MotionFindCharBack         // F{char}
)

// TextObject represents a text object (iw, aw, i", a", etc.).
type TextObject int

const (
	TextObjNone       TextObject = iota
	TextObjInnerWord             // iw
	TextObjAWord                 // aw
	TextObjInnerQuote            // i"
	TextObjAQuote                // a"
	TextObjInnerParen            // i(
	TextObjAParen                // a(
)

// VimState holds the complete state of the vim input handler.
type VimState struct {
	Mode        VimMode  // current editing mode
	Buffer      []rune   // the input text buffer
	Cursor      int      // cursor position in buffer
	PendingOp   Operator // operator waiting for motion/text-object
	Count       int      // numeric prefix (e.g., 3dw)
	CountBuf    string   // accumulating count digits
	SearchDir   int      // 1 for forward (/), -1 for backward (?)
	SearchBuf   string   // current search query being typed
	LastSearch  string   // last completed search
	Yanked      string   // yank register
	ReplaceChar bool     // waiting for replacement char (r command)
	FindChar    rune     // target char for f/F motion
	FindForward bool     // true for f, false for F
}

// NewVimState creates a VimState in insert mode (default for new input).
func NewVimState() *VimState {
	return &VimState{
		Mode: ModeInsert,
	}
}

// PersistentState holds state that persists across input lines.
type PersistentState struct {
	Enabled    bool   // whether vim mode is active
	LastSearch string // persists across lines
	Yanked     string // persists across lines
}

// NewPersistentState creates default persistent state.
func NewPersistentState() *PersistentState {
	return &PersistentState{}
}
