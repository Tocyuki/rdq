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

	// colorProdBanner is a solid red used for the PRODUCTION banner
	// background so the line is impossible to miss even on cluttered
	// terminals. Foreground stays high-contrast white.
	colorProdBanner = lipgloss.Color("196")
	// colorProdAccent / colorProdBorder replace colorAccent / colorBorder
	// when the active profile is flagged production so focused boxes and
	// status key labels visibly shift into "danger mode".
	colorProdAccent = lipgloss.AdaptiveColor{Light: "#a40e26", Dark: "#ff5c6c"}
	colorProdBorder = lipgloss.AdaptiveColor{Light: "#cf222e", Dark: "#f7768e"}
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

	// productionBannerStyle paints the ⚠ PRODUCTION ⚠ line at the top of
	// the status bar. Solid red background + white bold foreground so it
	// reads as a warning sign regardless of terminal palette. Padding
	// stretches the banner across the visible line when rendered with a
	// fixed width.
	productionBannerStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("231")).
				Background(colorProdBanner).
				Bold(true).
				Padding(0, 1)

	// productionStatusKeyStyle is the red variant of statusKeyStyle used
	// on the profile/region/cluster/db/secret/model labels while the
	// active profile is flagged as production.
	productionStatusKeyStyle = lipgloss.NewStyle().
					Foreground(colorProdAccent).
					Bold(true)

	// productionBoxFocused replaces editorBoxFocused / resultBoxFocused
	// on the currently focused pane so the red border reinforces "this
	// session is dangerous". Unfocused boxes keep the normal gray border
	// so the user can still tell which pane has focus.
	productionBoxFocused = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorProdBorder)
)
