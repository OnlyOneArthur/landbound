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
// and dark terminals. Colors are used semantically: green means the policy
// permitted the attempt, red means Landlock denied it, yellow means the
// kernel could not answer, grey means unknown or decorative.
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
	// maxFrameWidth caps the framed panels on very wide terminals so the
	// content stays readable instead of stretching edge to edge.
	maxFrameWidth = 76
	// chromeRows is the header plus the blank line above and below the body.
	chromeRows = 3
	// minBoxWidth is the narrowest a framed panel may be drawn.
	minBoxWidth = 12
)

var (
	headerBar    = lipgloss.NewStyle().Background(colPrimary).Foreground(colOnPrimary)
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(colPrimary)
	sectionStyle = lipgloss.NewStyle().Bold(true).Foreground(colPrimary)
	keyStyle     = lipgloss.NewStyle().Bold(true).Foreground(colPrimary)
	labelStyle   = lipgloss.NewStyle().Foreground(colMuted)
	valueStyle   = lipgloss.NewStyle().Foreground(colText)
	okStyle      = lipgloss.NewStyle().Foreground(colAccent)
	errStyle     = lipgloss.NewStyle().Foreground(colDanger)
	warnStyle    = lipgloss.NewStyle().Foreground(colWarn)
	dimStyle     = lipgloss.NewStyle().Foreground(colMuted)
	ruleStyle    = lipgloss.NewStyle().Foreground(colBorder)
	spinStyle    = lipgloss.NewStyle().Foreground(colPrimary)
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

	footer := m.renderFooter()
	body := m.renderBody()
	if m.showHelp {
		body = m.renderHelp()
	}
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

// contentWidth is the usable width for framed content.
func (m model) contentWidth() int {
	w := m.width
	if w <= 0 {
		w = defaultWidth
	}
	w -= 2 // leave a small margin so panels do not touch the terminal edge
	if w > maxFrameWidth {
		w = maxFrameWidth
	}
	if w < minBoxWidth {
		w = minBoxWidth
	}
	return w
}

// renderHeader draws a full-width bar: the product name on the left, and a
// kernel-capability badge on the right. The badge is honest about what is not
// yet known: before the first run the Landlock ABI has not been probed.
func (m model) renderHeader() string {
	title := headerBar.Bold(true).Render(" LANDBOUND ")
	sub := headerBar.Faint(true).Render(" Landlock policy workbench ")
	badge := m.abiBadge()

	if m.width <= 0 {
		return title + sub + " " + badge
	}

	left := title + sub
	if lipgloss.Width(left)+lipgloss.Width(badge) > m.width {
		left = title // drop the subtitle before the badge on narrow terminals
	}
	if lipgloss.Width(left)+lipgloss.Width(badge) > m.width {
		return clampLine(left, m.width)
	}
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(badge)
	return left + headerBar.Render(strings.Repeat(" ", gap)) + badge
}

// abiBadge reports the kernel's Landlock ABI as a semantic status: filled
// circle when network rules are available, half circle when the kernel is too
// old for them, hollow circle when the kernel has not been probed yet.
func (m model) abiBadge() string {
	switch {
	case m.report.ABI == 0:
		return headerBar.Foreground(colMuted).Render("○ ABI unknown ")
	case m.report.ABI < demo.MinNetworkABI:
		return headerBar.Foreground(colWarn).Render(fmt.Sprintf("◐ ABI %d · no network rules ", m.report.ABI))
	default:
		return headerBar.Foreground(colAccent).Render(fmt.Sprintf("● ABI %d ", m.report.ABI))
	}
}

// renderFooter separates the body from a single, always-visible hint line with
// a rule, so the two do not visually merge. The rule is content-wide and
// centered so it lines up with the panels above it rather than the terminal.
func (m model) renderFooter() string {
	if m.width > 0 {
		m.help.Width = m.width
	}
	rule := ruleStyle.Render(strings.Repeat("─", m.contentWidth()))
	return m.center(rule) + "\n" + m.help.View(m.keys)
}

// center places a block horizontally centered within the terminal, or returns
// it unchanged before the terminal width is known.
func (m model) center(s string) string {
	if m.width <= 0 {
		return s
	}
	return lipgloss.PlaceHorizontal(m.width, lipgloss.Center, s)
}

// renderBody stacks the policy panel above the evidence panel, centered in the
// terminal so wide windows do not leave all the slack on one side.
func (m model) renderBody() string {
	w := m.contentWidth()
	inner := w - 4

	border := colBorder
	if m.err != nil {
		border = colDanger
	}

	return m.center(strings.Join([]string{
		panelBox("Policy", colBorder, w, m.policyLines(inner)),
		"",
		panelBox("Evidence", border, w, m.evidenceLines(inner)),
	}, "\n"))
}

// policyLines shows the two toggleable rules with the key that drives each one,
// so the mapping from keystroke to rule is visible without the help overlay.
// Column widths adapt: the state label is semantic and must never truncate, so
// the description yields first, then the key column is dropped entirely on very
// narrow terminals.
func (m model) policyLines(inner int) []string {
	const stateW = 12
	keyW := 6
	if inner < 40 {
		keyW = 0 // too narrow for a key column; help overlay carries the mapping
	}
	descW := inner - keyW - stateW
	if descW < 8 {
		descW = 8
	}
	row := func(key, desc, state string) string {
		keyCell := ""
		if keyW > 0 {
			keyCell = keyStyle.Render(padCell(key, keyW, false))
		}
		return keyCell +
			valueStyle.Render(padCell(desc, descW, false)) +
			padCell(state, stateW, true)
	}

	lines := []string{
		row("space", "read private/secret.txt", m.privateRuleLabel()),
		row("n", "connect loopback TCP", m.networkRuleLabel()),
		"",
	}
	for _, ln := range wrapText("public/message.txt is always readable; it anchors the allowlist.", inner) {
		lines = append(lines, dimStyle.Render(ln))
	}
	return lines
}

func (m model) privateRuleLabel() string {
	if m.privateAllowed {
		return okStyle.Render("✓ ALLOWED")
	}
	return errStyle.Render("✗ DENIED")
}

// networkRuleLabel reports the loopback rule state. Before the first run the
// kernel capability is unknown, and on kernels below ABI 4 the rule cannot be
// enforced; neither case may be presented as a denial.
func (m model) networkRuleLabel() string {
	switch {
	case m.report.ABI == 0:
		return dimStyle.Render("○ unknown")
	case !m.report.NetworkSupported:
		return warnStyle.Render("○ no ABI 4+")
	case m.networkAllowed:
		return okStyle.Render("✓ ALLOWED")
	default:
		return errStyle.Render("✗ DENIED")
	}
}

// evidenceLines renders the last run as a baseline-versus-restricted table.
// Showing the baseline column is what makes the tool's claim checkable: the
// same attempt succeeded without the policy, so a denial is attributable to
// Landlock rather than to a broken fixture.
func (m model) evidenceLines(inner int) []string {
	switch {
	case m.err != nil:
		return styledWrap("ERROR: "+m.err.Error(), inner, errStyle)
	case m.busy:
		return []string{m.spin.View() + dimStyle.Render(" running the fixture under the current policy…")}
	case m.report.ABI == 0:
		return []string{
			valueStyle.Render("Not run yet."),
			"",
			dimStyle.Render("Press enter to run the fixture under the current policy."),
		}
	}

	// The side-by-side table needs room for the label, both columns and the gap
	// between them. Below that, stack each attempt so nothing collides.
	const tableMinInner = 34
	if inner < tableMinInner {
		return m.stackedEvidence(inner)
	}

	labelW, baseW := 9, 12
	restrW := inner - labelW - baseW - 2
	if restrW < 10 {
		labelW, baseW = 8, 8
		restrW = inner - labelW - baseW - 2
	}

	r := m.report
	lines := []string{
		dimStyle.Render(padCell("", labelW, false)) +
			labelStyle.Render(padCell("baseline", baseW, false)) +
			labelStyle.Render(padCell("restricted", restrW, false)),
		ruleStyle.Render(strings.Repeat("─", labelW+baseW+restrW)),
		tableRow(labelW, baseW, restrW, "public",
			readMark(r.Baseline, 0, baseW), readMark(r.Restricted, 0, restrW)),
		tableRow(labelW, baseW, restrW, "private",
			readMark(r.Baseline, 1, baseW), readMark(r.Restricted, 1, restrW)),
		tableRow(labelW, baseW, restrW, "loopback",
			netMark(r.NetworkBaseline, baseW), m.networkMark(restrW)),
		"",
	}
	return append(lines, styledWrap(verdict(r), inner, valueStyle)...)
}

func tableRow(labelW, baseW, restrW int, label, base, restricted string) string {
	return valueStyle.Render(padCell(label, labelW, false)) +
		padCell(base, baseW, false) +
		padCell(restricted, restrW, false)
}

// stackedEvidence renders each attempt as a label followed by its baseline and
// restricted marks on their own lines. It is used on terminals too narrow for
// the side-by-side table, where the columns would collide.
func (m model) stackedEvidence(inner int) []string {
	const markIndent, kindW = 2, 6
	markW := inner - markIndent - kindW
	if markW < 8 {
		markW = 8
	}

	r := m.report
	lines := []string{""}
	attempt := func(label, base, restricted string) {
		lines = append(lines, valueStyle.Render(clampLine(label, inner)))
		lines = append(lines,
			strings.Repeat(" ", markIndent)+labelStyle.Render(padCell("base", kindW, false))+base)
		lines = append(lines,
			strings.Repeat(" ", markIndent)+labelStyle.Render(padCell("restr", kindW, false))+restricted)
	}

	attempt("public/message.txt",
		readMark(r.Baseline, 0, markW), readMark(r.Restricted, 0, markW))
	attempt("private/secret.txt",
		readMark(r.Baseline, 1, markW), readMark(r.Restricted, 1, markW))
	if r.NetworkSupported || r.NetworkBaseline != nil {
		attempt("loopback TCP",
			netMark(r.NetworkBaseline, markW), m.networkMark(markW))
	}

	lines = append(lines, "")
	return append(lines, styledWrap(verdict(r), inner, valueStyle)...)
}

// readMark renders one file result, tolerating a short list so a partial report
// can never panic the view.
func readMark(rs []demo.ReadResult, i, w int) string {
	if i >= len(rs) {
		return dimStyle.Render("—")
	}
	return mark(rs[i], w)
}

// mark renders a single read result within w cells. The error is reduced to its
// trailing reason, and further shortened when the column is too narrow, so a
// cell never truncates mid-word into something misleading.
func mark(r demo.ReadResult, w int) string {
	if r.OK {
		return okStyle.Render("✓ success")
	}
	if r.Error != "" {
		return errStyle.Render("✗ " + fitReason(shortErr(r.Error), w-2))
	}
	return dimStyle.Render("? not reported")
}

func netMark(r *demo.NetResult, w int) string {
	if r == nil {
		return dimStyle.Render("—")
	}
	if r.OK {
		return okStyle.Render("✓ connected")
	}
	if r.Error != "" {
		return errStyle.Render("✗ " + fitReason(shortErr(r.Error), w-2))
	}
	return dimStyle.Render("? not reported")
}

// shortErr keeps the trailing reason of a wrapped Go error so a narrow cell
// still shows the operative word, e.g. "permission denied".
func shortErr(s string) string {
	if i := strings.LastIndex(s, ": "); i >= 0 {
		return s[i+2:]
	}
	return s
}

// fitReason degrades a reason to fit w cells, preferring whole words over a
// truncated fragment: "permission denied" -> "denied" -> "no".
func fitReason(reason string, w int) string {
	if lipgloss.Width(reason) <= w {
		return reason
	}
	words := strings.Fields(reason)
	for i := len(words) - 1; i >= 0; i-- {
		if cand := strings.Join(words[i:], " "); lipgloss.Width(cand) <= w {
			return cand
		}
	}
	return "no"
}

// networkMark is the restricted-side network cell, which must distinguish
// "denied" from "the kernel cannot enforce this at all".
func (m model) networkMark(w int) string {
	switch {
	case m.report.ABI == 0:
		return dimStyle.Render("○ unknown")
	case !m.report.NetworkSupported:
		return warnStyle.Render("○ no ABI 4+")
	default:
		return netMark(m.report.NetworkRestricted, w)
	}
}

// verdict states, in one sentence, how many attempts Landlock denied and why
// the baseline makes those denials attributable to Landlock.
func verdict(r demo.Report) string {
	denied, total := 0, 0
	for _, res := range r.Restricted {
		total++
		if !res.OK {
			denied++
		}
	}
	if r.NetworkSupported && r.NetworkRestricted != nil {
		total++
		if !r.NetworkRestricted.OK {
			denied++
		}
	}
	if denied == 0 {
		if total == 0 {
			return "No attempts were recorded, so there is nothing to attribute."
		}
		return fmt.Sprintf(
			"Landlock denied nothing: all %d attempts matched the policy. Baseline and restricted results agree.",
			total)
	}
	return fmt.Sprintf(
		"Landlock denied %d of %d attempts. The baseline shows the same operations succeeding without the policy, so the denials are attributable to Landlock.",
		denied, total)
}

func (m model) renderHelp() string {
	w := m.contentWidth()
	inner := w - 4

	var lines []string
	lines = append(lines, "")
	lines = append(lines, styledWrap(
		"Runs a bundled child under a Landlock allowlist and reports what it could read. The public file is always allowed; the private file and the loopback connect are denied unless you toggle them.",
		inner, dimStyle)...)
	lines = append(lines, "")
	lines = append(lines, sectionStyle.Render("Keys"))
	for _, k := range []struct{ key, desc string }{
		{"space", "toggle private-file rule"},
		{"n", "toggle loopback network rule"},
		{"enter", "run the fixture"},
		{"?", "toggle this help"},
		{"q", "quit"},
	} {
		lines = append(lines, keyStyle.Render(padCell(k.key, 7, false))+valueStyle.Render(k.desc))
	}
	lines = append(lines, "")
	lines = append(lines, sectionStyle.Render("What this is not"))
	lines = append(lines, styledWrap(
		"Landlock restricts the launched child, not other processes, and emits no stream of every denial. Results prove attempts on this kernel only.",
		inner, dimStyle)...)
	lines = append(lines, "")

	return m.center(panelBox("Help", colPrimary, w, lines))
}

// panelBox draws a bordered panel with its title embedded in the top border,
// the way many well-regarded TUIs label a section. Every line is exactly w
// cells wide, so stacked panels line up on both edges.
func panelBox(title string, border lipgloss.AdaptiveColor, w int, lines []string) string {
	if w < minBoxWidth {
		w = minBoxWidth
	}
	inner := w - 4
	if inner < 1 {
		inner = 1
	}
	bs := lipgloss.NewStyle().Foreground(border)

	var top string
	fill := w - 5 - lipgloss.Width(title)
	if title == "" || fill < 0 {
		top = bs.Render("╭" + strings.Repeat("─", w-2) + "╮")
	} else {
		top = bs.Render("╭─ ") + titleStyle.Render(title) +
			bs.Render(" "+strings.Repeat("─", fill)+"╮")
	}

	out := make([]string, 0, len(lines)+2)
	out = append(out, top)
	for _, ln := range lines {
		out = append(out, bs.Render("│ ")+padCell(ln, inner, false)+bs.Render(" │"))
	}
	out = append(out, bs.Render("╰"+strings.Repeat("─", w-2)+"╯"))
	return strings.Join(out, "\n")
}

// padCell pads s to exactly w cells, ANSI-aware. Text wider than w is clamped
// rather than allowed to break the frame.
func padCell(s string, w int, right bool) string {
	gap := w - lipgloss.Width(s)
	if gap <= 0 {
		return clampLine(s, w)
	}
	if right {
		return strings.Repeat(" ", gap) + s
	}
	return s + strings.Repeat(" ", gap)
}

// wrapText word-wraps plain text to width cells.
func wrapText(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}
	lines := []string{words[0]}
	for _, word := range words[1:] {
		last := len(lines) - 1
		if lipgloss.Width(lines[last])+1+lipgloss.Width(word) <= width {
			lines[last] += " " + word
			continue
		}
		lines = append(lines, word)
	}
	return lines
}

// styledWrap wraps plain text to width and applies one style to every line.
func styledWrap(s string, width int, style lipgloss.Style) []string {
	wrapped := wrapText(s, width)
	out := make([]string, len(wrapped))
	for i, ln := range wrapped {
		out[i] = style.Render(clampLine(ln, width))
	}
	return out
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
