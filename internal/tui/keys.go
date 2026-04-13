package tui

import "github.com/charmbracelet/bubbles/key"

// keyMap groups the TUI keybindings so the bubbles help component can render
// them automatically and tests can introspect the bindings.
type keyMap struct {
	Run        key.Binding
	Focus      key.Binding
	ToggleView key.Binding
	Inspect    key.Binding
	History    key.Binding
	ExportCSV  key.Binding
	Clear      key.Binding
	Help       key.Binding
	Quit       key.Binding
}

func defaultKeyMap() keyMap {
	return keyMap{
		Run: key.NewBinding(
			key.WithKeys("f5", "ctrl+r"),
			key.WithHelp("F5/^R", "run"),
		),
		Focus: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "focus"),
		),
		ToggleView: key.NewBinding(
			key.WithKeys("ctrl+j"),
			key.WithHelp("^J", "table/json"),
		),
		Inspect: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "inspect row"),
		),
		History: key.NewBinding(
			key.WithKeys("ctrl+h"),
			key.WithHelp("^H", "history"),
		),
		ExportCSV: key.NewBinding(
			key.WithKeys("ctrl+e"),
			key.WithHelp("^E", "export csv"),
		),
		Clear: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "clear"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("^C", "quit"),
		),
	}
}

// ShortHelp / FullHelp implement bubbles/help.KeyMap so the help bar can
// render the bindings in compact and expanded forms.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Run, k.Focus, k.Inspect, k.History, k.ExportCSV, k.ToggleView, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Run, k.Focus, k.ToggleView, k.Inspect},
		{k.History, k.ExportCSV, k.Clear},
		{k.Help, k.Quit},
	}
}
