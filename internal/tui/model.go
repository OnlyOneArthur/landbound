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
	networkAllowed bool
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
		case "n", "N":
			if !m.busy {
				m.networkAllowed = !m.networkAllowed
			}
		case "enter":
			if !m.busy {
				m.busy = true
				policy := demo.Policy{
					PrivateFileAllowed: m.privateAllowed,
					NetworkAllowed:     m.networkAllowed,
				}
				return m, func() tea.Msg {
					r, err := demo.Run(context.Background(), policy)
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
		status = fmt.Sprintf("Landlock ABI %d\nrestricted results: public %s | private %s",
			m.report.ABI, restrictedMark(m.report, 0), restrictedMark(m.report, 1))
		status += "\n" + networkStatus(m.report)
	}
	private := "DENIED (default)"
	if m.privateAllowed {
		private = "ALLOWED"
	}
	net := "unsupported on this kernel"
	if m.report.NetworkSupported {
		net = "DENIED (default)"
		if m.networkAllowed {
			net = "ALLOWED"
		}
	}
	return fmt.Sprintf(`LANDBOUND

Fixture tree                 Rule
├── public/
│   └── message.txt          ✓ read
└── private/
    └── secret.txt           %s

Loopback TCP connect         %s

[SPACE] private rule   [N] network rule   [ENTER] run   [?] help   [Q] quit

%s
`, private, net, status)
}

func helpView() string {
	return `LANDBOUND — help

What this does
  Builds a temporary fixture with a public and a private file, then runs a
  bundled child process under a Landlock allowlist and reports what it could
  read. The public file is always allowed; the private file is denied unless
  you toggle the rule.

  On kernels with Landlock ABI 4+ it also attempts one loopback TCP connect
  and reports whether the network rule allowed or denied it. On older kernels
  that scenario is reported as unsupported rather than guessed.

Keys
  SPACE   toggle the private-file rule
  N       toggle the loopback network rule
  ENTER   run the fixture under the current rules
  ?       show or hide this help
  Q       quit

What this is not
  Landlock restricts the launched child and its descendants. It does not
  monitor other processes and does not emit a stream of every denied access.
  Results here prove specific read and connect attempts on this kernel only.

Press ? to return.
`
}

// restrictedMark renders the read result at index i, tolerating a report that
// carries no restricted results so the view can never panic on partial data.
func restrictedMark(r demo.Report, i int) string {
	if i >= len(r.Restricted) {
		return "? not reported"
	}
	return mark(r.Restricted[i])
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

func networkStatus(r demo.Report) string {
	if !r.NetworkSupported {
		return fmt.Sprintf("network: unsupported (needs Landlock ABI %d+)", demo.MinNetworkABI)
	}
	if r.NetworkRestricted == nil {
		return "network: not reported"
	}
	if r.NetworkRestricted.OK {
		return "network: loopback connect succeeded (allowed)"
	}
	return "network: loopback connect denied — " + r.NetworkRestricted.Error
}
