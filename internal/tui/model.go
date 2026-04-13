package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/rdsdata"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// focusArea identifies which pane currently receives keyboard input.
type focusArea int

const (
	focusEditor focusArea = iota
	focusResults
)

// viewMode toggles the result rendering between table and JSON.
type viewMode int

const (
	viewTable viewMode = iota
	viewJSON
)

// Model is the bubbletea Model for the rdq TUI.
type Model struct {
	width, height int

	editor    textarea.Model
	table     table.Model
	jsonView  viewport.Model
	helpModel help.Model
	spin      spinner.Model
	keys      keyMap

	client *rdsdata.Client
	target target

	focus     focusArea
	mode      viewMode
	executing bool

	result   *queryResult
	jsonText string
	lastErr  error
	duration time.Duration
}

// newModel constructs an initialized Model. It does not perform any I/O; the
// first ExecuteStatement is triggered when the user presses run.
func newModel(client *rdsdata.Client, tgt target) Model {
	editor := textarea.New()
	editor.Placeholder = "Type SQL here. Press F5 or Ctrl+R to run."
	editor.Prompt = ""
	editor.ShowLineNumbers = true
	editor.CharLimit = 0
	editor.SetHeight(8)
	editor.Focus()

	tbl := table.New(table.WithFocused(false))
	tbl.SetStyles(table.Styles{
		Header:   lipgloss.NewStyle().Bold(true).Foreground(colorAccent).BorderBottom(true).BorderForeground(colorBorder),
		Cell:     lipgloss.NewStyle(),
		Selected: lipgloss.NewStyle().Background(colorAccent).Foreground(lipgloss.Color("0")),
	})

	vp := viewport.New(0, 0)

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	hm := help.New()
	hm.Styles.ShortKey = helpKeyStyle
	hm.Styles.ShortDesc = helpDescStyle
	hm.Styles.ShortSeparator = helpSepStyle
	hm.Styles.FullKey = helpKeyStyle
	hm.Styles.FullDesc = helpDescStyle
	hm.Styles.FullSeparator = helpSepStyle

	return Model{
		editor:    editor,
		table:     tbl,
		jsonView:  vp,
		helpModel: hm,
		spin:      sp,
		keys:      defaultKeyMap(),
		client:    client,
		target:    tgt,
		focus:     focusEditor,
		mode:      viewTable,
	}
}

// Init starts the spinner ticker so it animates whenever executing is true.
func (m Model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.spin.Tick)
}

// Update is the canonical bubbletea reducer. It dispatches messages to the
// focused pane after handling global key bindings and async results.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()

	case spinner.TickMsg:
		if m.executing {
			var cmd tea.Cmd
			m.spin, cmd = m.spin.Update(msg)
			cmds = append(cmds, cmd)
		}

	case executeMsg:
		m.executing = false
		m.duration = msg.Duration
		if msg.Err != nil {
			m.lastErr = msg.Err
			m.result = nil
			m.jsonText = ""
		} else {
			m.lastErr = nil
			m.result = msg.Result
			m.jsonText = msg.Result.toJSON()
			m.refreshTable()
			m.refreshJSON()
			m.focus = focusResults
			m.editor.Blur()
			m.table.Focus()
		}

	case tea.KeyMsg:
		if cmd, handled := m.handleKey(msg); handled {
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		} else {
			cmds = append(cmds, m.routeKey(msg))
		}
	}

	return m, tea.Batch(cmds...)
}

// handleKey implements the global key bindings (run, focus, toggle, quit).
// It returns handled=true when the message was consumed and should not be
// forwarded to the focused pane.
func (m *Model) handleKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		return tea.Quit, true

	case key.Matches(msg, m.keys.Run):
		if m.executing {
			return nil, true
		}
		m.executing = true
		m.lastErr = nil
		return tea.Batch(m.spin.Tick, runStatement(m.client, m.target, m.editor.Value())), true

	case key.Matches(msg, m.keys.Focus):
		m.toggleFocus()
		return nil, true

	case key.Matches(msg, m.keys.ToggleView):
		if m.mode == viewTable {
			m.mode = viewJSON
		} else {
			m.mode = viewTable
		}
		return nil, true

	case key.Matches(msg, m.keys.Clear):
		if m.lastErr != nil {
			m.lastErr = nil
			return nil, true
		}

	case key.Matches(msg, m.keys.Help):
		m.helpModel.ShowAll = !m.helpModel.ShowAll
		return nil, true
	}
	return nil, false
}

// routeKey forwards an unconsumed key message to whichever pane currently
// holds focus.
func (m *Model) routeKey(msg tea.KeyMsg) tea.Cmd {
	var cmd tea.Cmd
	switch m.focus {
	case focusEditor:
		m.editor, cmd = m.editor.Update(msg)
	case focusResults:
		if m.mode == viewTable {
			m.table, cmd = m.table.Update(msg)
		} else {
			m.jsonView, cmd = m.jsonView.Update(msg)
		}
	}
	return cmd
}

// toggleFocus moves keyboard focus between the editor and the results pane,
// updating per-component focus state so the cursor blink and table highlight
// follow accordingly.
func (m *Model) toggleFocus() {
	switch m.focus {
	case focusEditor:
		m.focus = focusResults
		m.editor.Blur()
		m.table.Focus()
	case focusResults:
		m.focus = focusEditor
		m.table.Blur()
		m.editor.Focus()
	}
}

// layout reflows the panes when the terminal is resized. It is also called
// once at startup via the initial WindowSizeMsg bubbletea sends.
func (m *Model) layout() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	statusH := 2
	helpH := 1
	footerH := 1
	available := m.height - statusH - helpH - footerH - 4 // borders
	if available < 6 {
		available = 6
	}
	editorH := available / 2
	if editorH < 4 {
		editorH = 4
	}
	resultsH := available - editorH
	if resultsH < 4 {
		resultsH = 4
	}

	innerW := m.width - 2
	if innerW < 20 {
		innerW = 20
	}

	m.editor.SetWidth(innerW)
	m.editor.SetHeight(editorH)

	m.table.SetWidth(innerW)
	m.table.SetHeight(resultsH)

	m.jsonView.Width = innerW
	m.jsonView.Height = resultsH

	m.helpModel.Width = m.width
}

// refreshTable rebuilds the bubbles table model from the latest queryResult
// using column widths sized to fit content.
//
// Important ordering: bubbles/table v1.0.0's SetColumns synchronously calls
// UpdateViewport, which iterates over the existing rows and indexes into the
// new columns. If the previous rows are wider than the new columns we'd hit
// an "index out of range" panic in renderRow. Clearing rows first sidesteps
// that hazard entirely.
func (m *Model) refreshTable() {
	if m.result == nil {
		m.table.SetRows(nil)
		m.table.SetColumns(nil)
		return
	}
	widths := columnWidths(m.result.Columns, m.result.Rows)
	cols := make([]table.Column, len(m.result.Columns))
	for i, c := range m.result.Columns {
		cols[i] = table.Column{Title: c, Width: widths[i] + 2}
	}
	rows := make([]table.Row, len(m.result.Rows))
	for i, r := range m.result.Rows {
		row := make(table.Row, len(r))
		for j, cell := range r {
			w := columnWidthCap
			if j < len(widths) {
				w = widths[j]
			}
			row[j] = truncate(cell, w)
		}
		rows[i] = row
	}
	m.table.SetRows(nil)
	m.table.SetColumns(cols)
	m.table.SetRows(rows)
}

// refreshJSON updates the JSON viewport with the cached jsonText.
func (m *Model) refreshJSON() {
	m.jsonView.SetContent(m.jsonText)
	m.jsonView.GotoTop()
}

// View renders the full TUI: status bar, editor, results pane, status line,
// and help bar.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "loading..."
	}

	status := m.renderStatus()
	editorBox := m.renderEditor()
	resultsBox := m.renderResults()
	footer := m.renderFooter()
	helpLine := m.helpModel.View(m.keys)

	return strings.Join([]string{
		status,
		editorBox,
		resultsBox,
		footer,
		helpBarStyle.Render(helpLine),
	}, "\n")
}

func (m Model) renderStatus() string {
	line1 := fmt.Sprintf(
		"%s %s   %s %s",
		statusKeyStyle.Render("profile"),
		nonEmpty(m.target.profile),
		statusKeyStyle.Render("region"),
		nonEmpty(m.target.region),
	)
	line2 := fmt.Sprintf(
		"%s %s   %s %s",
		statusKeyStyle.Render("cluster"),
		nonEmpty(shortARN(m.target.cluster)),
		statusKeyStyle.Render("db"),
		nonEmpty(m.target.database),
	)
	return statusStyle.Render(line1 + "\n" + line2)
}

func (m Model) renderEditor() string {
	style := editorBoxStyle
	if m.focus == focusEditor {
		style = editorBoxFocused
	}
	return style.Render(m.editor.View())
}

func (m Model) renderResults() string {
	style := resultBoxStyle
	if m.focus == focusResults {
		style = resultBoxFocused
	}
	var body string
	switch {
	case m.executing:
		body = fmt.Sprintf("%s running...", m.spin.View())
	case m.lastErr != nil:
		body = errorStyle.Render(m.lastErr.Error())
	case m.result == nil:
		body = helpStyle.Render("(no results yet)")
	case m.mode == viewTable:
		body = m.table.View()
	default:
		body = jsonStyle.Render(m.jsonView.View())
	}
	return style.Render(body)
}

func (m Model) renderFooter() string {
	if m.executing {
		return helpStyle.Render("running...")
	}
	if m.lastErr != nil {
		return errorStyle.Render("error · " + truncate(m.lastErr.Error(), m.width-10))
	}
	if m.result == nil {
		return helpStyle.Render("ready")
	}
	parts := []string{
		fmt.Sprintf("%d rows", len(m.result.Rows)),
		fmt.Sprintf("%dms", m.duration.Milliseconds()),
	}
	if m.result.Updated > 0 {
		parts = append(parts, fmt.Sprintf("%d updated", m.result.Updated))
	}
	if m.mode == viewJSON {
		parts = append(parts, "json")
	}
	return successStyle.Render(strings.Join(parts, " · "))
}

// nonEmpty falls back to a placeholder when a status value is missing.
func nonEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// shortARN trims a long ARN down to the resource portion so the status bar
// stays readable on narrow terminals.
func shortARN(arn string) string {
	if arn == "" {
		return ""
	}
	if idx := strings.LastIndex(arn, ":"); idx >= 0 && idx < len(arn)-1 {
		return arn[idx+1:]
	}
	return arn
}
