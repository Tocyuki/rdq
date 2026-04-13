package tui

import "github.com/charmbracelet/lipgloss"

// Color palette tuned for both light and dark terminal backgrounds.
var (
	colorAccent  = lipgloss.AdaptiveColor{Light: "#1f6feb", Dark: "#7aa2f7"}
	colorMuted   = lipgloss.AdaptiveColor{Light: "#57606a", Dark: "#9aa5ce"}
	colorError   = lipgloss.AdaptiveColor{Light: "#cf222e", Dark: "#f7768e"}
	colorSuccess = lipgloss.AdaptiveColor{Light: "#1a7f37", Dark: "#9ece6a"}
	colorBorder  = lipgloss.AdaptiveColor{Light: "#d0d7de", Dark: "#3b4261"}

	// Brighter palette dedicated to the help bar so shortcut hints stay
	// readable on terminals with dim default foregrounds.
	colorHelpKey  = lipgloss.AdaptiveColor{Light: "#0550ae", Dark: "#bb9af7"}
	colorHelpDesc = lipgloss.AdaptiveColor{Light: "#1f2328", Dark: "#c0caf5"}
	colorHelpSep  = lipgloss.AdaptiveColor{Light: "#8c959f", Dark: "#565f89"}
)

var (
	statusStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Padding(0, 1)

	statusKeyStyle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true)

	editorBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder)

	editorBoxFocused = editorBoxStyle.
				BorderForeground(colorAccent)

	resultBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder)

	resultBoxFocused = resultBoxStyle.
				BorderForeground(colorAccent)

	errorStyle = lipgloss.NewStyle().
			Foreground(colorError).
			Bold(true).
			Padding(0, 1)

	successStyle = lipgloss.NewStyle().
			Foreground(colorSuccess).
			Padding(0, 1)

	helpStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Padding(0, 1)

	// helpBarStyle wraps the rendered help line. It only adds padding —
	// foreground colors are intentionally left to the inner bubbles/help
	// Styles so that nested lipgloss escapes do not fight each other.
	helpBarStyle = lipgloss.NewStyle().Padding(0, 1)

	helpKeyStyle = lipgloss.NewStyle().
			Foreground(colorHelpKey).
			Bold(true)

	helpDescStyle = lipgloss.NewStyle().
			Foreground(colorHelpDesc)

	helpSepStyle = lipgloss.NewStyle().
			Foreground(colorHelpSep)

	jsonStyle = lipgloss.NewStyle().
			Foreground(colorMuted)
)
