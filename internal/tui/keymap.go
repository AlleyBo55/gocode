package tui

import bubbletea "github.com/charmbracelet/bubbletea"

func isEnter(msg bubbletea.KeyMsg) bool {
	return msg.Type == bubbletea.KeyEnter
}

func isTab(msg bubbletea.KeyMsg) bool {
	return msg.Type == bubbletea.KeyTab
}

func isCtrlC(msg bubbletea.KeyMsg) bool {
	return msg.Type == bubbletea.KeyCtrlC
}

func isEscape(msg bubbletea.KeyMsg) bool {
	return msg.Type == bubbletea.KeyEscape
}

func isBackspace(msg bubbletea.KeyMsg) bool {
	return msg.Type == bubbletea.KeyBackspace
}

func isCtrlD(msg bubbletea.KeyMsg) bool {
	return msg.Type == bubbletea.KeyCtrlD
}

func helpBar(width int) string {
	keys := []struct{ key, desc string }{
		{"Enter", "send"},
		{"Tab", "mode"},
		{"Ctrl+D", "diff"},
		{"Esc", "cancel"},
		{"Ctrl+C", "quit"},
		{"/help", "commands"},
	}
	var parts []string
	for _, k := range keys {
		parts = append(parts, helpKeyStyle.Render(k.key)+" "+helpStyle.Render(k.desc))
	}
	bar := ""
	for i, p := range parts {
		if i > 0 {
			bar += helpStyle.Render("  │  ")
		}
		bar += p
	}
	return helpStyle.Width(width).Render(bar)
}
