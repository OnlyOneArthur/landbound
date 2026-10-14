package tui

import tea "github.com/charmbracelet/bubbletea"

// Run enters the alternate screen, runs the TUI loop, and restores the terminal.
func Run() error {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
