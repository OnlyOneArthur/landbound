package tui

import (
	"context"

	"github.com/OnlyOneArthur/landbound/internal/demo"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// resultMsg carries the outcome of one fixture run back into the update loop.
type resultMsg struct {
	report demo.Report
	err    error
}

type model struct {
	privateAllowed bool
	networkAllowed bool
	busy           bool
	showHelp       bool
	report         demo.Report
	err            error
	quitting       bool

	// width and height come from tea.WindowSizeMsg. Until the first one
	// arrives they are zero and the view falls back to a default width.
	width  int
	height int

	spin spinner.Model
	help help.Model
	keys keyMap
}

func initialModel() model {
	return model{
		spin: newSpinner(),
		help: newHelp(),
		keys: defaultKeyMap(),
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.Width = msg.Width
		return m, nil
	case resultMsg:
		m.busy, m.report, m.err = false, msg.report, msg.err
		return m, nil
	case spinner.TickMsg:
		// Only keep the spinner animation alive while a run is in flight, so
		// the tick chain ends on its own once the result arrives.
		if !m.busy {
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case " ":
		if !m.busy {
			m.privateAllowed = !m.privateAllowed
		}
	case "n", "N":
		if !m.busy {
			m.networkAllowed = !m.networkAllowed
		}
	case "enter":
		if !m.busy {
			m.busy = true
			m.report = demo.Report{}
			m.err = nil
			policy := demo.Policy{
				PrivateFileAllowed: m.privateAllowed,
				NetworkAllowed:     m.networkAllowed,
			}
			return m, tea.Batch(m.spin.Tick, runFixture(policy))
		}
	case "?":
		m.showHelp = !m.showHelp
	}
	return m, nil
}

// runFixture runs the bundled fixture off the update loop and reports back.
func runFixture(policy demo.Policy) tea.Cmd {
	return func() tea.Msg {
		r, err := demo.Run(context.Background(), policy)
		return resultMsg{report: r, err: err}
	}
}
