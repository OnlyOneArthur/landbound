package tui

import (
	"context"
	"fmt"

	"github.com/OnlyOneArthur/landbound/internal/demo"
	tea "github.com/charmbracelet/bubbletea"
)

type resultMsg struct {
	report demo.Report
	err    error
}

type model struct {
	privateAllowed bool
	busy           bool
	showHelp       bool
	report         demo.Report
	err            error
	quitting       bool
}

func initialModel() model { return model{} }

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case resultMsg:
		m.busy, m.report, m.err = false, msg.report, msg.err
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case " ":
			if !m.busy {
				m.privateAllowed = !m.privateAllowed
			}
		case "enter":
			if !m.busy {
				m.busy = true
				allowed := m.privateAllowed
				return m, func() tea.Msg {
					r, err := demo.Run(context.Background(), allowed)
					return resultMsg{report: r, err: err}
				}
			}
		case "?":
			m.showHelp = !m.showHelp
		}
	}
	return m, nil
}

func (m model) View() string {
	if m.quitting {
		return "\n"
	}
	if m.showHelp {
		return helpView()
	}
	status := "not run yet — press enter"
	if m.busy {
		status = "running fixture…"
	} else if m.err != nil {
		status = "ERROR: " + m.err.Error()
	} else if m.report.ABI > 0 {
		pub := mark(m.report.Restricted[0])
		priv := mark(m.report.Restricted[1])
		status = fmt.Sprintf("Landlock ABI %d\nrestricted results: public %s | private %s", m.report.ABI, pub, priv)
	}
	private := "DENIED (default)"
	if m.privateAllowed {
		private = "ALLOWED"
	}
	return fmt.Sprintf(`LANDBOUND

Fixture tree                 Rule
├── public/
│   └── message.txt          ✓ read
└── private/
    └── secret.txt           %s

[SPACE] toggle private rule   [ENTER] run fixture   [?] help   [Q] quit

%s
`, private, status)
}

func helpView() string {
	return `LANDBOUND — help

What this does
  Builds a temporary fixture with a public and a private file, then runs a
  bundled child process under a Landlock allowlist and reports what it could
  read. The public file is always allowed; the private file is denied unless
  you toggle the rule.

Keys
  SPACE   toggle the private-file rule
  ENTER   run the fixture under the current rule
  ?       show or hide this help
  Q       quit

What this is not
  Landlock restricts the launched child and its descendants. It does not
  monitor other processes and does not emit a stream of every denied access.
  Results here prove specific read attempts on this kernel only.

Press ? to return.
`
}

func mark(r demo.ReadResult) string {
	if r.OK {
		return "✓ success"
	}
	if r.Error != "" {
		return "✗ " + r.Error
	}
	return "? not reported"
}
