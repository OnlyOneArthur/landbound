package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/OnlyOneArthur/landbound/internal/demo"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// widths and heights chosen to cover: no WindowSizeMsg yet (zero), absurdly
// small terminals, a typical terminal and a very wide one.
var sizes = []struct{ w, h int }{
	{0, 0},
	{1, 1},
	{12, 4},
	{20, 8},
	{40, 12},
	{80, 24},
	{100, 30},
	{240, 60},
}

// TestViewNeverExceedsTerminalWidth asserts every rendered line fits inside the
// reported terminal width, which is what keeps the alt-screen from wrapping.
func TestViewNeverExceedsTerminalWidth(t *testing.T) {
	for _, s := range sizes {
		for _, state := range viewStates() {
			m := state.build()
			m.width, m.height = s.w, s.h
			v := m.View()
			if s.w <= 0 {
				continue
			}
			for i, line := range strings.Split(v, "\n") {
				if got := lipgloss.Width(line); got > s.w {
					t.Fatalf("size %dx%d %s: line %d width %d > %d:\n%s",
						s.w, s.h, state.name, i, got, s.w, v)
				}
			}
		}
	}
}

// TestViewNeverPanicsOnTinyTerminal covers the degenerate sizes explicitly; a
// panic here would take the whole TUI down on a resized or odd terminal.
func TestViewNeverPanicsOnTinyTerminal(t *testing.T) {
	for _, s := range sizes {
		for _, state := range viewStates() {
			m := state.build()
			m.width, m.height = s.w, s.h
			if v := m.View(); v == "" {
				t.Fatalf("size %dx%d %s: empty view", s.w, s.h, state.name)
			}
		}
	}
}

// TestViewClampsToHeight asserts the body never grows past the terminal height,
// so a long report cannot push the footer off screen.
func TestViewClampsToHeight(t *testing.T) {
	for _, s := range sizes {
		if s.h <= 0 {
			continue
		}
		m := (viewState{name: "report", report: fullReport()}).build()
		m.width, m.height = s.w, s.h
		if got := lipgloss.Height(m.View()); got > s.h {
			t.Fatalf("size %dx%d: view height %d > %d", s.w, s.h, got, s.h)
		}
	}
}

// TestWindowSizeMsgUpdatesLayout asserts the resize message is actually
// consumed rather than ignored.
func TestWindowSizeMsgUpdatesLayout(t *testing.T) {
	m := initialModel()
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	if m.width != 120 || m.height != 40 {
		t.Fatalf("window size not stored: %dx%d", m.width, m.height)
	}
	if m.help.Width != 120 {
		t.Fatalf("help width not propagated: %d", m.help.Width)
	}
}

// TestContentWidthBounds asserts the content width stays inside the terminal
// and never drops below the minimum, which is what the box sizing relies on.
func TestContentWidthBounds(t *testing.T) {
	cases := []struct{ termW, want int }{
		{0, maxFrameWidth},
		{20, 18},
		{40, 38},
		{100, maxFrameWidth},
		{400, maxFrameWidth},
	}
	for _, c := range cases {
		m := initialModel()
		m.width = c.termW
		if got := m.contentWidth(); got != c.want {
			t.Fatalf("contentWidth(term=%d) = %d, want %d", c.termW, got, c.want)
		}
	}
}

// TestBusySpinnerOnlyWhileRunning asserts the spinner animation is armed on
// start and stopped once the result lands, so the tick loop cannot spin forever.
func TestBusySpinnerOnlyWhileRunning(t *testing.T) {
	m := initialModel()
	m, cmd := update(t, m, key("enter"))
	if !m.busy || cmd == nil {
		t.Fatal("enter should start a run and return a command")
	}

	// A tick while busy keeps the animation alive.
	_, cmd = update(t, m, m.spin.Tick())
	if cmd == nil {
		t.Fatal("spinner tick while busy should re-arm the animation")
	}

	// Once the result arrives, ticks must stop re-arming.
	m, _ = update(t, m, resultMsg{report: fullReport()})
	if m.busy {
		t.Fatal("result should clear busy")
	}
	if _, cmd := update(t, m, m.spin.Tick()); cmd != nil {
		t.Fatal("spinner tick after completion should not re-arm")
	}
}

// TestRunClearsPreviousResult asserts a new run does not keep showing stale
// evidence from the previous run while it is in flight.
func TestRunClearsPreviousResult(t *testing.T) {
	m := initialModel()
	m.report = fullReport()
	m, _ = update(t, m, key("enter"))
	if m.report.ABI != 0 {
		t.Fatalf("stale report survived a new run: %+v", m.report)
	}
	if m.err != nil {
		t.Fatalf("stale error survived a new run: %v", m.err)
	}
}

// TestHelpRendersBoxed asserts the help overlay stays inside the terminal and,
// when there is room for it, still carries the required disclaimer.
func TestHelpRendersBoxed(t *testing.T) {
	for _, s := range sizes {
		m := initialModel()
		m.showHelp = true
		m.width, m.height = s.w, s.h
		v := m.View()
		// A terminal too short to hold the overlay is cropped on purpose, so
		// only require the disclaimer when the height can accommodate it.
		if s.h >= 24 && !strings.Contains(v, "What this is not") {
			t.Fatalf("size %dx%d: help missing disclaimer:\n%s", s.w, s.h, v)
		}
		if s.w > 0 {
			for i, line := range strings.Split(v, "\n") {
				if got := lipgloss.Width(line); got > s.w {
					t.Fatalf("size %dx%d: help line %d width %d > %d", s.w, s.h, i, got, s.w)
				}
			}
		}
	}
}

// TestSpinnerAnimatesWhileBusy asserts the busy indicator actually advances
// through frames rather than freezing on one glyph.
func TestSpinnerAnimatesWhileBusy(t *testing.T) {
	m := initialModel()
	m.busy = true
	seen := map[string]bool{}
	for range 12 {
		next, _ := update(t, m, m.spin.Tick())
		m = next
		seen[m.spin.View()] = true
	}
	if len(seen) < 2 {
		t.Fatalf("spinner did not animate; frames=%v", seen)
	}
}

// TestAdaptivePaletteDefined guards against a palette entry being dropped, which
// would render as an empty style on one of the two terminal backgrounds.
func TestAdaptivePaletteDefined(t *testing.T) {
	for name, c := range map[string]lipgloss.AdaptiveColor{
		"primary": colPrimary, "accent": colAccent, "danger": colDanger,
		"warn": colWarn, "muted": colMuted, "text": colText, "border": colBorder,
	} {
		if c.Light == "" || c.Dark == "" {
			t.Fatalf("palette %q missing a light or dark color: %+v", name, c)
		}
	}
}

// TestStateLabelNeverTruncates asserts the policy state label is semantic and
// must survive at every width: a truncated "✗ DE" or "✗ DENIE" would read as
// something other than a denial.
func TestStateLabelNeverTruncates(t *testing.T) {
	for _, w := range []int{80, 60, 48, 44, 40, 36, 32, 28} {
		m := initialModel()
		m.width, m.height = w, 30
		v := m.View()
		if !strings.Contains(v, "DENIED") {
			t.Fatalf("width %d: private rule state was truncated:\n%s", w, v)
		}
	}
}

// TestNarrowEvidenceStacks asserts that below the threshold where a two-column
// table stops fitting, each attempt is stacked so the baseline and restricted
// results never collide in the same cells.
func TestNarrowEvidenceStacks(t *testing.T) {
	m := initialModel()
	m.report = fullReport()
	m.width, m.height = 32, 40
	v := m.View()
	if !strings.Contains(v, "base") || !strings.Contains(v, "restr") {
		t.Fatalf("stacked layout should label each side:\n%s", v)
	}
	if strings.Contains(v, "baselinerestr") || strings.Contains(v, "succes✓") {
		t.Fatalf("stacked layout collided columns:\n%s", v)
	}
}

// TestReasonNeverTruncatesMidWord asserts a denial reason degrades to a shorter
// whole word rather than a fragment like "permissi".
func TestReasonNeverTruncatesMidWord(t *testing.T) {
	cases := []struct {
		w    int
		want string
	}{
		{20, "permission denied"},
		{12, "denied"},
		{4, "no"},
	}
	for _, c := range cases {
		if got := fitReason("permission denied", c.w); got != c.want {
			t.Fatalf("fitReason(w=%d) = %q, want %q", c.w, got, c.want)
		}
	}
}

// TestShortErrKeepsTrailingReason asserts the wrapped Go error is reduced to the
// operative reason so a narrow cell still says why it failed.
func TestShortErrKeepsTrailingReason(t *testing.T) {
	cases := map[string]string{
		"open /tmp/x/private/secret.txt: permission denied":    "permission denied",
		"dial tcp 127.0.0.1:34567: connect: permission denied": "permission denied",
		"permission denied": "permission denied",
	}
	for in, want := range cases {
		if got := shortErr(in); got != want {
			t.Fatalf("shortErr(%q) = %q, want %q", in, got, want)
		}
	}
}

type viewState struct {
	name   string
	busy   bool
	err    error
	report demo.Report
	help   bool
}

func (s viewState) build() model {
	m := initialModel()
	m.busy = s.busy
	m.err = s.err
	m.report = s.report
	m.showHelp = s.help
	return m
}

func viewStates() []viewState {
	return []viewState{
		{name: "initial"},
		{name: "busy", busy: true},
		{name: "error", err: errors.New("boom: a very long failure message that should be clamped rather than wrap")},
		{name: "report", report: fullReport()},
		{name: "report-no-network", report: demo.Report{
			ABI: 2,
			Restricted: []demo.ReadResult{
				{Path: "public/message.txt", OK: true},
				{Path: "private/secret.txt", Error: "permission denied"},
			},
		}},
		{name: "report-partial", report: demo.Report{ABI: 4, NetworkSupported: true}},
		{name: "help", help: true},
	}
}

func fullReport() demo.Report {
	return demo.Report{
		ABI: 4,
		Baseline: []demo.ReadResult{
			{Path: "public/message.txt", OK: true},
			{Path: "private/secret.txt", OK: true},
		},
		Restricted: []demo.ReadResult{
			{Path: "public/message.txt", OK: true},
			{Path: "private/secret.txt", Error: "permission denied"},
		},
		NetworkSupported:  true,
		NetworkRestricted: &demo.NetResult{Target: "127.0.0.1:9", Error: "connect: permission denied"},
	}
}
