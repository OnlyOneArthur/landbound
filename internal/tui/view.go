package tui

import (
	"fmt"
	"strings"

	"github.com/OnlyOneArthur/landbound/internal/demo"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
)

// Palette. Every color is adaptive so the TUI reads correctly on both light
// and dark terminals.
var (
	colPrimary   = lipgloss.AdaptiveColor{Light: "#5A3FD6", Dark: "#7D56F4"}
	colAccent    = lipgloss.AdaptiveColor{Light: "#0A7B4F", Dark: "#04B575"}
	colDanger    = lipgloss.AdaptiveColor{Light: "#C1003B", Dark: "#FF5F87"}
	colWarn      = lipgloss.AdaptiveColor{Light: "#9A6A00", Dark: "#FFCC66"}
	colMuted     = lipgloss.AdaptiveColor{Light: "#8A8A94", Dark: "#6C6C78"}
	colText      = lipgloss.AdaptiveColor{Light: "#2A2A33", Dark: "#D7D7DD"}
	colBorder    = lipgloss.AdaptiveColor{Light: "#C0C0CC", Dark: "#4A4A5A"}
	colOnPrimary = lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#FFFFFF"}
)

const (
	// defaultWidth is the layout width used before the first WindowSizeMsg.
	defaultWidth = 80
	// maxFrameWidth caps the framed boxes on very wide terminals so the
	// content stays readable instead of stretching edge to edge.
	maxFrameWidth = 76
	// chromeRows is the header, blank line and footer rows around the body.
	chromeRows = 3
	// minBoxWidth is the narrowest a framed box may be drawn.
	minBoxWidth = 12
)

var (
	headerBar    = lipgloss.NewStyle().Background(colPrimary).Foreground(colOnPrimary)
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(colPrimary)
	sectionStyle = lipgloss.NewStyle().Bold(true).Foreground(colPrimary)
	keyStyle     = lipgloss.NewStyle().Bold(true).Foreground(colText)
	labelStyle   = lipgloss.NewStyle().Foreground(colMuted)
	okStyle      = lipgloss.NewStyle().Foreground(colAccent)
	errStyle     = lipgloss.NewStyle().Foreground(colDanger)
	warnStyle    = lipgloss.NewStyle().Foreground(colWarn)
	dimStyle     = lipgloss.NewStyle().Foreground(colMuted)
	spinStyle    = lipgloss.NewStyle().Foreground(colPrimary)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colBorder).
			Padding(0, 1)
)

func newHelp() help.Model {
	h := help.New()
	h.Styles.ShortKey = lipgloss.NewStyle().Foreground(colPrimary).Bold(true)
	h.Styles.ShortDesc = dimStyle
	h.Styles.ShortSeparator = dimStyle
	h.Styles.FullKey = h.Styles.ShortKey
	h.Styles.FullDesc = dimStyle
	h.Styles.FullSeparator = dimStyle
	h.Styles.Ellipsis = dimStyle
	return h
}

func newSpinner() spinner.Model {
	return spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(spinStyle))
}

// View renders the header, the body (or help overlay) and the footer. The
// result is always clamped to the terminal width and cropped to its height
// when those are known, so a resize or a tiny terminal can never wrap or
// overflow the alt-screen.
func (m model) View() string {
	if m.quitting {
		return "\n"
	}
	if m.showHelp {
		return m.fit(m.renderHelp())
	}

	footer := m.renderFooter()
	body := m.renderBody()
	if m.height > 0 {
		body = cropLines(body, m.bodyHeight(footer))
	}

	return m.fit(strings.Join([]string{m.renderHeader(), "", body, "", footer}, "\n"))
}

// fit applies the final width clamp and height crop to a fully rendered view.
func (m model) fit(s string) string {
	if m.width > 0 {
		s = clampLines(s, m.width)
	}
	if m.height > 0 {
		s = cropLines(s, m.height)
	}
	return s
}

// bodyHeight is the number of rows left for the body after the chrome. It
// keeps the footer on screen at normal sizes; fit() then hard-crops as a
// final guarantee for degenerate terminals.
func (m model) bodyHeight(footer string) int {
	h := m.height - chromeRows - lipgloss.Height(footer)
	if h < 0 {
		return 0
	}
	return h
}

// contentWidth is the usable width for boxed content.
func (m model) contentWidth() int {
	w := m.width
	if w <= 0 {
		w = defaultWidth
	}
	w -= 2 // leave a small margin so boxes do not touch the terminal edge
	if w > maxFrameWidth {
		w = maxFrameWidth
	}
	if w < minBoxWidth {
		w = minBoxWidth
	}
	return w
}

func (m model) renderHeader() string {
	title := headerBar.Bold(true).Render(" LANDBOUND ")
	sub := dimStyle.Render("Landlock policy workbench")
	if m.width > 0 && m.width < 40 {
		return title
	}
	return title + "  " + sub
}

func (m model) renderFooter() string {
	// Keep the help component's own width in sync with the terminal so it
	// ellipsizes the footer instead of letting it be truncated mid-hint.
	if m.width > 0 {
		m.help.Width = m.width
	}
	return m.help.View(m.keys)
}

// renderBody stacks the fixture/rule panel above the result panel.
func (m model) renderBody() string {
	w := m.contentWidth()
	panels := []string{
		m.renderPolicyPanel(w),
		m.renderResultPanel(w),
	}
	return strings.Join(panels, "\n")
}

// renderPolicyPanel shows the fixture tree and the two rules side by side.
func (m model) renderPolicyPanel(w int) string {
	tree := []string{
		sectionStyle.Render("Fixture tree"),
		dimStyle.Render("├── public/"),
		dimStyle.Render("│   └── message.txt"),
		dimStyle.Render("└── private/"),
		dimStyle.Render("    └── secret.txt"),
	}
	rules := []string{
		sectionStyle.Render("Rules"),
		labelStyle.Render("read public/message.txt") + "   " + okStyle.Render("✓ always allowed"),
		labelStyle.Render("read private/secret.txt") + "   " + m.privateRuleLabel(),
		labelStyle.Render("loopback TCP connect") + "      " + m.networkRuleLabel(),
	}
	left := strings.Join(tree, "\n")
	right := strings.Join(rules, "\n")
	inner := w - 4
	if inner < 20 || lipgloss.Width(right) > inner-lipgloss.Width(left)-2 {
		// Too narrow to sit side by side; stack instead.
		return boxStyle.Width(inner).Render(left + "\n\n" + right)
	}
	return boxStyle.Width(inner).Render(
		lipgloss.JoinHorizontal(lipgloss.Top, left, "   ", right),
	)
}

func (m model) privateRuleLabel() string {
	if m.privateAllowed {
		return okStyle.Render("✓ ALLOWED")
	}
	return errStyle.Render("✗ DENIED (default)")
}

// networkRuleLabel reports the loopback rule state. Before a run the kernel
// capability is unknown, so it must not claim the rule is enforceable.
func (m model) networkRuleLabel() string {
	if !m.report.NetworkSupported {
		if m.report.ABI > 0 {
			return dimStyle.Render(fmt.Sprintf("unsupported on this kernel (needs ABI %d+)", demo.MinNetworkABI))
		}
		return dimStyle.Render("unsupported on this kernel")
	}
	if m.networkAllowed {
		return okStyle.Render("✓ ALLOWED")
	}
	return errStyle.Render("✗ DENIED (default)")
}

// renderResultPanel shows the latest run's evidence, or a prompt/status.
func (m model) renderResultPanel(w int) string {
	var lines []string
	lines = append(lines, sectionStyle.Render("Result"))

	switch {
	case m.err != nil:
		lines = append(lines, errStyle.Render("ERROR: "+m.err.Error()))
	case m.busy:
		lines = append(lines, m.spin.View()+dimStyle.Render(" running fixture…"))
	case m.report.ABI == 0:
		lines = append(lines, dimStyle.Render("not run yet — press enter to run the fixture"))
	default:
		lines = append(lines, m.resultLines()...)
	}
	return boxStyle.Width(w - 4).Render(strings.Join(lines, "\n"))
}

func (m model) resultLines() []string {
	r := m.report
	out := []string{dimStyle.Render(fmt.Sprintf("Landlock ABI %d", r.ABI))}
	for i, res := range r.Restricted {
		label := res.Path
		if i == 0 {
			label = "public/message.txt"
		}
		if i == 1 {
			label = "private/secret.txt"
		}
		out = append(out, fmt.Sprintf("%-22s %s", label, mark(res)))
	}
	out = append(out, networkStatus(r))
	return out
}

func (m model) renderHelp() string {
	w := m.contentWidth()
	// Every line here is kept short enough to fit inside the box at the
	// default width; lipgloss word-wraps anything longer, which looks broken.
	body := strings.Join([]string{
		titleStyle.Render("LANDBOUND — help"),
		"",
		sectionStyle.Render("What this does"),
		"  Runs a bundled child under a Landlock allowlist and",
		"  reports what it could read. Public is always allowed;",
		"  private is denied unless you toggle the rule.",
		"",
		"  On ABI 4+ it also attempts one loopback TCP connect,",
		"  allowed or denied by the network rule. Older kernels",
		"  report that scenario as unsupported rather than guessing.",
		"",
		sectionStyle.Render("Keys"),
		"  " + keyStyle.Render("SPACE") + "  toggle private-file rule",
		"  " + keyStyle.Render("N") + "      toggle loopback network rule",
		"  " + keyStyle.Render("ENTER") + "  run fixture under current rules",
		"  " + keyStyle.Render("?") + "      show or hide this help",
		"  " + keyStyle.Render("Q") + "      quit",
		"",
		sectionStyle.Render("What this is not"),
		"  Landlock restricts the launched child, not other",
		"  processes, and emits no per-denial stream. Results",
		"  prove attempts on this kernel only.",
	}, "\n")
	return boxStyle.Width(w - 4).Render(body)
}

// mark renders a single read result, tolerating partial data so the view can
// never panic on an unexpected report shape.
func mark(r demo.ReadResult) string {
	if r.OK {
		return okStyle.Render("✓ success")
	}
	if r.Error != "" {
		return errStyle.Render("✗ " + r.Error)
	}
	return dimStyle.Render("? not reported")
}

func networkStatus(r demo.Report) string {
	if !r.NetworkSupported {
		return warnStyle.Render(fmt.Sprintf(
			"network: unsupported (needs Landlock ABI %d+)", demo.MinNetworkABI))
	}
	if r.NetworkRestricted == nil {
		return dimStyle.Render("network: not reported")
	}
	if r.NetworkRestricted.OK {
		return okStyle.Render("network: loopback connect succeeded (allowed)")
	}
	return errStyle.Render("network: loopback connect denied — " + r.NetworkRestricted.Error)
}

func clampLine(line string, width int) string {
	if width <= 0 || lipgloss.Width(line) <= width {
		return line
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(line)
}

func clampLines(s string, width int) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = clampLine(line, width)
	}
	return strings.Join(lines, "\n")
}

func cropLines(s string, n int) string {
	if n <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[:n], "\n")
}
