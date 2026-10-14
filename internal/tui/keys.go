package tui

import keybind "github.com/charmbracelet/bubbles/key"

// keyMap is the full set of bindings the TUI understands. It implements
// help.KeyMap so the footer and the expanded help view are driven from one
// source of truth.
type keyMap struct {
	TogglePrivate keybind.Binding
	ToggleNetwork keybind.Binding
	Run           keybind.Binding
	Help          keybind.Binding
	Quit          keybind.Binding
}

func defaultKeyMap() keyMap {
	return keyMap{
		TogglePrivate: keybind.NewBinding(
			keybind.WithKeys(" "),
			keybind.WithHelp("space", "toggle private-file rule"),
		),
		ToggleNetwork: keybind.NewBinding(
			keybind.WithKeys("n"),
			keybind.WithHelp("n", "toggle loopback network rule"),
		),
		Run: keybind.NewBinding(
			keybind.WithKeys("enter"),
			keybind.WithHelp("enter", "run fixture"),
		),
		Help: keybind.NewBinding(
			keybind.WithKeys("?"),
			keybind.WithHelp("?", "more"),
		),
		Quit: keybind.NewBinding(
			keybind.WithKeys("q", "ctrl+c"),
			keybind.WithHelp("q", "quit"),
		),
	}
}

// ShortHelp implements help.KeyMap for the footer line.
func (k keyMap) ShortHelp() []keybind.Binding {
	return []keybind.Binding{k.TogglePrivate, k.ToggleNetwork, k.Run, k.Help, k.Quit}
}

// FullHelp implements help.KeyMap for the expanded help view.
func (k keyMap) FullHelp() [][]keybind.Binding {
	return [][]keybind.Binding{
		{k.TogglePrivate, k.ToggleNetwork},
		{k.Run, k.Help, k.Quit},
	}
}
