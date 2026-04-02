package tui

import "github.com/charmbracelet/lipgloss"

// Go brand colors
// Primary: #00ADD8 (Go blue) → closest 256-color: 38
// Secondary: #00A29C (Go teal) → closest: 37
// Dark: #1A1A2E (deep navy) → closest: 234
// Accent: #CE3262 (Go pink from logo) → closest: 168
// Light: #E0EBF5 (light blue tint) → closest: 153

var (
	// Go Blue — the signature color
	goBlue = lipgloss.Color("38")  // #00ADD8
	goTeal = lipgloss.Color("37")  // #00A29C
	goPink = lipgloss.Color("168") // #CE3262
	goDark = lipgloss.Color("234") // deep navy bg
	goGray = lipgloss.Color("245") // neutral gray
	goLight = lipgloss.Color("153") // light blue tint
	goWhite = lipgloss.Color("255")
	goBlack = lipgloss.Color("16")

	// Header bar — Go blue background, white text
	headerStyle = lipgloss.NewStyle().
			Background(goBlue).
			Foreground(goBlack).
			Bold(true).
			Padding(0, 1)

	// Status bar — dark navy background
	statusBarStyle = lipgloss.NewStyle().
			Background(goDark).
			Foreground(goGray).
			Padding(0, 1)

	// Mode indicators
	modeBuildStyle = lipgloss.NewStyle().
			Background(goBlue).
			Foreground(goBlack).
			Bold(true).
			Padding(0, 1)

	modePlanStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("178")). // amber/yellow for plan
			Foreground(goBlack).
			Bold(true).
			Padding(0, 1)

	// Chat message prefixes — Go blue for assistant, teal for user
	userStyle = lipgloss.NewStyle().
			Foreground(goTeal).
			Bold(true)

	assistantStyle = lipgloss.NewStyle().
			Foreground(goBlue).
			Bold(true)

	toolStyle = lipgloss.NewStyle().
			Foreground(goGray)

	thinkingStyle = lipgloss.NewStyle().
			Foreground(goGray).
			Italic(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(goPink)

	// Input area — teal prompt, white text
	inputPromptStyle = lipgloss.NewStyle().
				Foreground(goTeal).
				Bold(true)

	inputStyle = lipgloss.NewStyle().
			Foreground(goWhite)

	// Help bar
	helpStyle = lipgloss.NewStyle().
			Foreground(goGray)

	helpKeyStyle = lipgloss.NewStyle().
			Foreground(goBlue).
			Bold(true)

	// Diff panel styles
	diffPanelStyle = lipgloss.NewStyle().
			BorderLeft(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(goGray).
			Padding(0, 1)

	diffAddStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("114")) // green

	diffRemoveStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("203")) // red

	diffHeaderStyle = lipgloss.NewStyle().
			Foreground(goBlue).
			Bold(true)

	// Gopher ASCII art for empty state
	_ = gopherArt // unused placeholder
	gopherArt = string(goBlue)
)

// GopherWelcome returns the Go gopher welcome message for empty chat state.
func GopherWelcome() string {
	gopher := lipgloss.NewStyle().Foreground(goBlue).Render(`
        ╻╻
       ╭┫┣╮
      ╭┫╰╯┣╮
    ╭━┫┃  ┃┣━╮
    ┃ ╰┫  ┣╯ ┃
    ╰━━┫  ┣━━╯
       ┃  ┃
       ╹  ╹`)

	title := lipgloss.NewStyle().
		Foreground(goBlue).
		Bold(true).
		Render("  gocode")

	subtitle := lipgloss.NewStyle().
		Foreground(goGray).
		Render("  Type a message to start coding.")

	return gopher + "\n\n" + title + "\n" + subtitle
}
