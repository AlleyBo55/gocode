package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	bubbletea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/AlleyBo55/gocode/internal/agent"
	"github.com/AlleyBo55/gocode/internal/apitypes"
	"github.com/AlleyBo55/gocode/internal/skills"
)

// Mode represents the current agent mode.
type Mode int

const (
	ModeBuild Mode = iota
	ModePlan
)

func (m Mode) String() string {
	if m == ModePlan {
		return "Plan"
	}
	return "Build"
}

// Config holds TUI configuration.
type Config struct {
	Version  string
	Model    string
	MaxTurns int
	Skills   []skills.Skill
}

// ChatMessage represents a message in the chat.
type ChatMessage struct {
	Role    string // "user", "assistant", "tool", "thinking"
	Content string
	IsError bool
}

// Model is the main bubbletea model.
type Model struct {
	runtime     *agent.ConversationRuntime
	config      Config
	width       int
	height      int
	mode        Mode
	input       string
	messages    []ChatMessage
	streaming   bool
	streamBuf   string
	statusMsg   string
	quitting    bool
	scroll      int
	showDiff    bool
	diffContent string
	ctx         context.Context
	cancel      context.CancelFunc
}

// Message types for async operations.
type streamChunkMsg struct{ text string }
type streamDoneMsg struct{}
type streamErrorMsg struct{ err error }
type toolStartMsg struct {
	name    string
	summary string
}
type toolEndMsg struct {
	name    string
	success bool
}

// NewModel creates the TUI model.
func NewModel(rt *agent.ConversationRuntime, cfg Config) Model {
	ctx, cancel := context.WithCancel(context.Background())
	return Model{
		runtime:  rt,
		config:   cfg,
		mode:     ModeBuild,
		messages: []ChatMessage{},
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Init implements bubbletea.Model.
func (m Model) Init() bubbletea.Cmd {
	return nil
}

// Update implements bubbletea.Model.
func (m Model) Update(msg bubbletea.Msg) (bubbletea.Model, bubbletea.Cmd) {
	switch msg := msg.(type) {
	case bubbletea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case bubbletea.KeyMsg:
		if isCtrlC(msg) {
			if m.streaming {
				m.cancel()
				m.streaming = false
				m.statusMsg = "Cancelled"
				// Reset context for next request
				m.ctx, m.cancel = context.WithCancel(context.Background())
				return m, nil
			}
			m.quitting = true
			return m, bubbletea.Quit
		}
		if isEscape(msg) && m.streaming {
			m.cancel()
			m.streaming = false
			m.statusMsg = "Cancelled"
			m.ctx, m.cancel = context.WithCancel(context.Background())
			return m, nil
		}
		if m.streaming {
			return m, nil // ignore input while streaming
		}
		if isTab(msg) {
			if m.mode == ModeBuild {
				m.mode = ModePlan
			} else {
				m.mode = ModeBuild
			}
			return m, nil
		}
		if isCtrlD(msg) {
			m.showDiff = !m.showDiff
			if m.showDiff {
				m.refreshDiff()
			}
			return m, nil
		}
		if isEnter(msg) {
			text := strings.TrimSpace(m.input)
			if text == "" {
				return m, nil
			}
			m.messages = append(m.messages, ChatMessage{Role: "user", Content: text})
			m.input = ""
			m.streaming = true
			m.statusMsg = "Thinking..."
			// Fresh context for this request
			m.ctx, m.cancel = context.WithCancel(context.Background())
			return m, m.sendMessage(text)
		}
		if isBackspace(msg) {
			if len(m.input) > 0 {
				m.input = m.input[:len(m.input)-1]
			}
			return m, nil
		}
		// Regular character input
		if msg.Type == bubbletea.KeyRunes {
			m.input += string(msg.Runes)
		} else if msg.Type == bubbletea.KeySpace {
			m.input += " "
		}
		return m, nil

	case streamResultMsg:
		m.streaming = false
		// Add tool events first
		m.messages = append(m.messages, msg.toolEvents...)
		// Add assistant response
		if msg.text != "" {
			m.messages = append(m.messages, ChatMessage{Role: "assistant", Content: msg.text})
		}
		usage := m.runtime.GetUsage()
		m.statusMsg = fmt.Sprintf("%d tokens", usage.TotalTokens())
		// Refresh diff after each assistant response
		if m.showDiff {
			m.refreshDiff()
		}
		return m, nil

	case streamErrorMsg:
		m.streaming = false
		m.messages = append(m.messages, ChatMessage{
			Role:    "assistant",
			Content: fmt.Sprintf("Error: %v", msg.err),
			IsError: true,
		})
		m.statusMsg = "Error"
		return m, nil
	}

	return m, nil
}

// sendMessage starts streaming the LLM response.
// It collects the full streamed response (since bubbletea Cmd returns one Msg)
// and returns it as a single assembled message.
func (m Model) sendMessage(text string) bubbletea.Cmd {
	rt := m.runtime
	ctx := m.ctx
	return func() bubbletea.Msg {
		var eventCh <-chan apitypes.StreamEvent
		var err error

		// Check for image paths in input
		imagePath := extractTUIImagePath(text)
		if imagePath != "" {
			textPart := strings.Replace(text, imagePath, "", 1)
			textPart = strings.TrimSpace(textPart)
			if textPart == "" {
				textPart = "What's in this image?"
			}
			imgMsg, imgErr := apitypes.UserImageAndText(textPart, imagePath)
			if imgErr != nil {
				return streamErrorMsg{err: imgErr}
			}
			eventCh, err = rt.StreamWithMessage(ctx, imgMsg)
		} else {
			eventCh, err = rt.StreamUserMessage(ctx, text)
		}
		if err != nil {
			return streamErrorMsg{err: err}
		}
		var fullText strings.Builder
		var toolEvents []ChatMessage
		for ev := range eventCh {
			switch ev.Kind {
			case "content_block_delta":
				if ev.BlockDelta != nil && ev.BlockDelta.Kind == "text_delta" {
					fullText.WriteString(ev.BlockDelta.Text)
				}
			case "content_block_start":
				if ev.ContentBlock != nil && ev.ContentBlock.Kind == "tool_use" {
					toolEvents = append(toolEvents, ChatMessage{
						Role:    "tool",
						Content: "⚡ " + ev.ContentBlock.Name,
					})
				}
			}
		}
		return streamResultMsg{
			text:       fullText.String(),
			toolEvents: toolEvents,
		}
	}
}

// extractTUIImagePath finds the first word in input that looks like an image file path.
func extractTUIImagePath(input string) string {
	imageExts := []string{".png", ".jpg", ".jpeg", ".gif", ".webp"}
	words := strings.Fields(input)
	for _, word := range words {
		lower := strings.ToLower(word)
		for _, ext := range imageExts {
			if strings.HasSuffix(lower, ext) {
				if _, err := os.Stat(word); err == nil {
					return word
				}
			}
		}
	}
	return ""
}

// streamResultMsg carries the complete streamed response.
type streamResultMsg struct {
	text       string
	toolEvents []ChatMessage
}

// View implements bubbletea.Model.
func (m Model) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}
	if m.width == 0 {
		return "Loading..."
	}

	w := m.width

	// Header
	modeStr := m.mode.String()
	header := headerStyle.Width(w).Render(
		fmt.Sprintf(" 🐹 gocode %s │ %s │ %s", m.config.Version, m.config.Model, modeStr),
	)

	// Help bar
	help := helpBar(w)

	// Status bar
	branch := gitBranch()
	modeTag := modeBuildStyle.Render(" BUILD ")
	if m.mode == ModePlan {
		modeTag = modePlanStyle.Render(" PLAN ")
	}
	statusText := m.statusMsg
	if m.streaming {
		statusText = "⏳ " + statusText
	}
	statusRight := modeTag + " " + statusBarStyle.Render(branch)
	statusLeft := statusBarStyle.Render(" " + statusText)
	gap := w - lipgloss.Width(statusLeft) - lipgloss.Width(statusRight)
	if gap < 0 {
		gap = 0
	}
	statusLine := statusLeft + strings.Repeat(" ", gap) + statusRight

	// Input area
	prompt := inputPromptStyle.Render("you> ")
	cursor := "█"
	if m.streaming {
		cursor = ""
	}
	inputLine := prompt + inputStyle.Render(m.input+cursor)

	// Calculate chat area height
	// header(1) + chat + status(1) + input(1) + help(1) = height
	chatHeight := m.height - 4
	if chatHeight < 1 {
		chatHeight = 1
	}

	// Render chat messages
	chatWidth := w
	if m.showDiff {
		chatWidth = w * 70 / 100
	}

	var chatLines []string
	if len(m.messages) == 0 {
		// Show gopher welcome art centered
		welcome := GopherWelcome()
		for _, line := range strings.Split(welcome, "\n") {
			chatLines = append(chatLines, line)
		}
	} else {
		for _, msg := range m.messages {
			chatLines = append(chatLines, renderChatMessage(msg, chatWidth-2)...)
		}
	}

	// Scroll to bottom: show last chatHeight lines
	if len(chatLines) > chatHeight {
		chatLines = chatLines[len(chatLines)-chatHeight:]
	}

	// Pad to fill height
	for len(chatLines) < chatHeight {
		chatLines = append(chatLines, "")
	}

	chat := strings.Join(chatLines, "\n")

	if m.showDiff {
		diffWidth := w - chatWidth
		diffPanel := renderDiff(m.diffContent, diffWidth, chatHeight)
		diffStyled := diffPanelStyle.Width(diffWidth).Height(chatHeight).Render(diffPanel)
		chat = lipgloss.JoinHorizontal(lipgloss.Top, chat, diffStyled)
	}

	return header + "\n" + chat + "\n" + statusLine + "\n" + inputLine + "\n" + help
}

// renderChatMessage formats a single chat message into display lines.
func renderChatMessage(msg ChatMessage, maxWidth int) []string {
	var prefix string
	switch msg.Role {
	case "user":
		prefix = userStyle.Render("you> ")
	case "assistant":
		if msg.IsError {
			prefix = errorStyle.Render("err> ")
		} else {
			prefix = assistantStyle.Render("assistant> ")
		}
	case "tool":
		if msg.IsError {
			prefix = errorStyle.Render("  ")
		} else {
			prefix = toolStyle.Render("  ")
		}
	case "thinking":
		prefix = thinkingStyle.Render("💭 ")
	default:
		prefix = ""
	}

	content := msg.Content
	if msg.IsError && msg.Role == "assistant" {
		content = errorStyle.Render(content)
	} else if msg.Role == "tool" {
		if msg.IsError {
			content = errorStyle.Render(content)
		} else {
			content = toolStyle.Render(content)
		}
	} else if msg.Role == "thinking" {
		content = thinkingStyle.Render(content)
	}

	// Wrap long lines
	lines := strings.Split(content, "\n")
	var result []string
	for i, line := range lines {
		if i == 0 {
			result = append(result, prefix+line)
		} else {
			// Indent continuation lines
			pad := strings.Repeat(" ", lipgloss.Width(prefix))
			result = append(result, pad+line)
		}
	}
	return result
}

// gitBranch returns the current git branch or empty string.
func gitBranch() string {
	out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// refreshDiff runs git diff and stores the result.
func (m *Model) refreshDiff() {
	out, err := exec.Command("git", "diff").Output()
	if err != nil {
		m.diffContent = "(git diff failed)"
		return
	}
	m.diffContent = string(out)
	if strings.TrimSpace(m.diffContent) == "" {
		m.diffContent = "(no changes)"
	}
}

// renderDiff formats diff content with colored +/- lines.
func renderDiff(diff string, width, height int) string {
	lines := strings.Split(diff, "\n")
	var rendered []string
	for _, line := range lines {
		if len(rendered) >= height {
			break
		}
		// Truncate long lines
		if len(line) > width-2 {
			line = line[:width-5] + "..."
		}
		switch {
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			rendered = append(rendered, diffAddStyle.Render(line))
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			rendered = append(rendered, diffRemoveStyle.Render(line))
		case strings.HasPrefix(line, "@@"):
			rendered = append(rendered, diffHeaderStyle.Render(line))
		case strings.HasPrefix(line, "diff "):
			rendered = append(rendered, diffHeaderStyle.Render(line))
		default:
			rendered = append(rendered, toolStyle.Render(line))
		}
	}
	// Pad to height
	for len(rendered) < height {
		rendered = append(rendered, "")
	}
	return strings.Join(rendered, "\n")
}

// Run starts the TUI program.
func Run(rt *agent.ConversationRuntime, cfg Config) error {
	m := NewModel(rt, cfg)
	p := bubbletea.NewProgram(m, bubbletea.WithAltScreen())
	_, err := p.Run()
	return err
}
