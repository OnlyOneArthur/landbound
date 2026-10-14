package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/OnlyOneArthur/landbound/internal/demo"
	tea "github.com/charmbracelet/bubbletea"
)

func key(s string) tea.KeyMsg {
	switch s {
	case " ":
		return tea.KeyMsg{Type: tea.KeySpace}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}
func update(t *testing.T, m model, msg tea.Msg) (model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	got, ok := next.(model)
	if !ok {
		t.Fatalf("Update returned %T, want model", next)
	}
	return got, cmd
}

func TestTogglePrivateRule(t *testing.T) {
	m := initialModel()
	if m.privateAllowed {
		t.Fatal("private rule should start denied")
	}
	m, _ = update(t, m, key(" "))
	if !m.privateAllowed {
		t.Fatal("space should allow the private rule")
	}
	m, _ = update(t, m, key(" "))
	if m.privateAllowed {
		t.Fatal("space should deny the private rule again")
	}
}

func TestToggleNetworkRule(t *testing.T) {
	m := initialModel()
	if m.networkAllowed {
		t.Fatal("network rule should start denied")
	}
	m, _ = update(t, m, key("n"))
	if !m.networkAllowed {
		t.Fatal("n should allow the network rule")
	}
	m, _ = update(t, m, key("n"))
	if m.networkAllowed {
		t.Fatal("n should deny the network rule again")
	}
}

func TestToggleIgnoredWhileBusy(t *testing.T) {
	m := initialModel()
	m.busy = true
	m, _ = update(t, m, key(" "))
	if m.privateAllowed {
		t.Fatal("space must not change the rule while a run is in flight")
	}
	m, _ = update(t, m, key("n"))
	if m.networkAllowed {
		t.Fatal("n must not change the rule while a run is in flight")
	}
}

func TestEnterStartsRunOnce(t *testing.T) {
	m := initialModel()
	m, cmd := update(t, m, key("enter"))
	if !m.busy {
		t.Fatal("enter should mark the model busy")
	}
	if cmd == nil {
		t.Fatal("enter should return a command to run the fixture")
	}
	// A second enter while busy must not queue another run.
	m2, cmd2 := update(t, m, key("enter"))
	if cmd2 != nil {
		t.Fatal("enter while busy should not start another run")
	}
	if !m2.busy {
		t.Fatal("model should stay busy")
	}
}

func TestResultMessageClearsBusyAndStoresReport(t *testing.T) {
	m := initialModel()
	m.busy = true
	report := demo.Report{ABI: 4}
	m, _ = update(t, m, resultMsg{report: report})
	if m.busy {
		t.Fatal("result should clear the busy flag")
	}
	if m.err != nil {
		t.Fatalf("unexpected error: %v", m.err)
	}
	if m.report.ABI != 4 {
		t.Fatalf("report not stored: %+v", m.report)
	}
}

func TestResultMessageStoresError(t *testing.T) {
	m := initialModel()
	m.busy = true
	want := errors.New("boom")
	m, _ = update(t, m, resultMsg{err: want})
	if m.err == nil || m.err.Error() != want.Error() {
		t.Fatalf("error not stored: %v", m.err)
	}
	if m.busy {
		t.Fatal("result should clear the busy flag even on error")
	}
}

func TestQuitKeys(t *testing.T) {
	for _, k := range []string{"q", "ctrl+c"} {
		m := initialModel()
		var msg tea.Msg
		if k == "ctrl+c" {
			msg = tea.KeyMsg{Type: tea.KeyCtrlC}
		} else {
			msg = key(k)
		}
		m, cmd := update(t, m, msg)
		if !m.quitting {
			t.Fatalf("%q should set quitting", k)
		}
		if cmd == nil {
			t.Fatalf("%q should return a quit command", k)
		}
	}
}

func TestViewStates(t *testing.T) {
	m := initialModel()
	if v := m.View(); !strings.Contains(v, "not run yet") {
		t.Fatalf("initial view should prompt a run, got:\n%s", v)
	}

	m.busy = true
	if v := m.View(); !strings.Contains(v, "running") {
		t.Fatalf("busy view should say running, got:\n%s", v)
	}

	m = initialModel()
	m.err = errors.New("boom")
	if v := m.View(); !strings.Contains(v, "ERROR") {
		t.Fatalf("error view should surface the error, got:\n%s", v)
	}

	m = initialModel()
	m.report = demo.Report{
		ABI: 4,
		Restricted: []demo.ReadResult{
			{Path: "public/message.txt", OK: true},
			{Path: "private/secret.txt", Error: "permission denied"},
		},
	}
	v := m.View()
	if !strings.Contains(v, "ABI 4") || !strings.Contains(v, "success") {
		t.Fatalf("result view missing expected content, got:\n%s", v)
	}

	m = initialModel()
	m.report = demo.Report{
		ABI:              4,
		NetworkSupported: true,
		Restricted: []demo.ReadResult{
			{Path: "public/message.txt", OK: true},
			{Path: "private/secret.txt", Error: "permission denied"},
		},
		NetworkRestricted: &demo.NetResult{Target: "127.0.0.1:9", Error: "permission denied"},
	}
	if v := m.View(); !strings.Contains(v, "loopback connect denied") {
		t.Fatalf("network denial should be surfaced, got:\n%s", v)
	}

	m = initialModel()
	m.report = demo.Report{ABI: 2, NetworkSupported: false}
	if v := m.View(); !strings.Contains(v, "network: unsupported") {
		t.Fatalf("unsupported network scenario should be stated, got:\n%s", v)
	}

	m.quitting = true
	if v := m.View(); strings.Contains(v, "LANDBOUND") {
		t.Fatalf("quitting view should be blank, got:\n%s", v)
	}
}

func TestHelpToggle(t *testing.T) {
	m := initialModel()
	m, _ = update(t, m, key("?"))
	if !m.showHelp {
		t.Fatal("? should open help")
	}
	v := m.View()
	if !strings.Contains(v, "LANDBOUND") || !strings.Contains(v, "What this is not") {
		t.Fatalf("help view missing expected content, got:\n%s", v)
	}
	m, _ = update(t, m, key("?"))
	if m.showHelp {
		t.Fatal("? should close help")
	}
	if v := m.View(); !strings.Contains(v, "Fixture tree") {
		t.Fatalf("closing help should return to the main view, got:\n%s", v)
	}
}

func TestViewPrivateRuleLabel(t *testing.T) {
	m := initialModel()
	if v := m.View(); !strings.Contains(v, "DENIED") {
		t.Fatalf("default view should show DENIED, got:\n%s", v)
	}
	m.privateAllowed = true
	if v := m.View(); !strings.Contains(v, "ALLOWED") {
		t.Fatalf("toggled view should show ALLOWED, got:\n%s", v)
	}
}

func TestViewNetworkRuleLabel(t *testing.T) {
	m := initialModel()
	// Before any run the kernel capability is unknown, so the view must not
	// claim the rule is enforceable.
	if v := m.View(); !strings.Contains(v, "unsupported on this kernel") {
		t.Fatalf("unrun view should not claim network support, got:\n%s", v)
	}
	m.report = demo.Report{ABI: 4, NetworkSupported: true}
	if v := m.View(); !strings.Contains(v, "DENIED") {
		t.Fatalf("supported view should show the denied network rule, got:\n%s", v)
	}
	m.networkAllowed = true
	if v := m.View(); !strings.Contains(v, "ALLOWED") {
		t.Fatalf("toggled network view should show ALLOWED, got:\n%s", v)
	}
}
