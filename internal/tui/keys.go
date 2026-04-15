package tui

import "github.com/charmbracelet/bubbles/key"

// keyMap groups the TUI keybindings so the bubbles help component can render
// them automatically and tests can introspect the bindings.
type keyMap struct {
	Run              key.Binding
	Focus            key.Binding
	ToggleView       key.Binding
	Inspect          key.Binding
	History          key.Binding
	Ask              key.Binding
	Assist           key.Binding
	SwitchModel      key.Binding
	SwitchLanguage   key.Binding
	SwitchTarget     key.Binding
	SwitchSecret     key.Binding
	SwitchProfile    key.Binding
	ToggleProduction key.Binding
	ExportCSV        key.Binding
	Clear            key.Binding
	Help             key.Binding
	Quit             key.Binding
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
		Ask: key.NewBinding(
			key.WithKeys("ctrl+g"),
			key.WithHelp("^G", "ask AI (generate SQL)"),
		),
		Assist: key.NewBinding(
			key.WithKeys("f6"),
			key.WithHelp("F6", "review / analyze / explain"),
		),
		SwitchModel: key.NewBinding(
			key.WithKeys("ctrl+o"),
			key.WithHelp("^O", "switch model"),
		),
		SwitchLanguage: key.NewBinding(
			key.WithKeys("ctrl+l"),
			key.WithHelp("^L", "switch language"),
		),
		SwitchTarget: key.NewBinding(
			key.WithKeys("ctrl+t"),
			key.WithHelp("^T", "switch cluster"),
		),
		SwitchSecret: key.NewBinding(
			key.WithKeys("ctrl+\\"),
			key.WithHelp("^\\", "switch secret"),
		),
		SwitchProfile: key.NewBinding(
			key.WithKeys("ctrl+p"),
			key.WithHelp("^P", "switch profile"),
		),
		ToggleProduction: key.NewBinding(
			key.WithKeys("f7"),
			key.WithHelp("F7", "toggle production flag"),
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
	return []key.Binding{k.Run, k.Ask, k.Assist, k.SwitchModel, k.SwitchLanguage, k.SwitchProfile, k.SwitchTarget, k.SwitchSecret, k.History, k.ExportCSV, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Run, k.Focus, k.ToggleView, k.Inspect},
		{k.Ask, k.Assist, k.SwitchModel, k.SwitchLanguage},
		{k.SwitchProfile, k.SwitchTarget, k.SwitchSecret, k.ToggleProduction},
		{k.History, k.ExportCSV, k.Clear, k.Help, k.Quit},
	}
}
