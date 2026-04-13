package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/Tocyuki/rdq/internal/history"
	"github.com/aws/aws-sdk-go-v2/service/rdsdata"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
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

	// History picker state.
	historyStore *history.Store
	historyList  list.Model
	historyOpen  bool

	// Row inspector state. inspecting hides the table and shows the JSON
	// view of a single row using inspectorVP. inspectedRow remembers which
	// row index is currently displayed so the footer can show "row N/M".
	inspecting   bool
	inspectorVP  viewport.Model
	inspectedRow int

	// Transient status message (e.g. CSV export confirmation). Cleared on
	// the next execute or Esc.
	flashMessage string
}

// historyItem adapts history.Entry to the list.Item interface so the picker
// can render and filter entries.
type historyItem struct {
	entry history.Entry
}

func (i historyItem) FilterValue() string { return i.entry.SQL }
func (i historyItem) Title() string       { return summarizeSQL(i.entry.SQL) }
func (i historyItem) Description() string {
	icon := "✓"
	if !i.entry.Ok {
		icon = "✗"
	}
	stamp := i.entry.At.Local().Format("2006-01-02 15:04:05")
	return fmt.Sprintf("%s %s · %dms", icon, stamp, i.entry.DurationMS)
}

// summarizeSQL collapses whitespace and truncates so a multi-line statement
// fits on one line in the picker.
func summarizeSQL(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	return truncate(s, 80)
}

// newModel constructs an initialized Model. It does not perform any I/O; the
// first ExecuteStatement is triggered when the user presses run.
func newModel(client *rdsdata.Client, tgt target, store *history.Store) Model {
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
	inspector := viewport.New(0, 0)

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	hm := help.New()
	hm.Styles.ShortKey = helpKeyStyle
	hm.Styles.ShortDesc = helpDescStyle
	hm.Styles.ShortSeparator = helpSepStyle
	hm.Styles.FullKey = helpKeyStyle
	hm.Styles.FullDesc = helpDescStyle
	hm.Styles.FullSeparator = helpSepStyle

	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.Foreground(colorAccent).BorderForeground(colorAccent)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.Foreground(colorAccent).BorderForeground(colorAccent)
	hl := list.New(nil, delegate, 0, 0)
	hl.Title = "SQL history (type to filter, Enter to load, Esc to cancel)"
	hl.SetShowStatusBar(true)
	hl.SetFilteringEnabled(true)
	hl.SetShowHelp(false)

	return Model{
		editor:       editor,
		table:        tbl,
		jsonView:     vp,
		inspectorVP:  inspector,
		helpModel:    hm,
		spin:         sp,
		keys:         defaultKeyMap(),
		client:       client,
		target:       tgt,
		focus:        focusEditor,
		mode:         viewTable,
		historyStore: store,
		historyList:  hl,
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
		m.flashMessage = ""
		m.recordHistory(msg)
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
		// History picker takes precedence over global bindings so the user
		// can type / and other characters into the filter without them being
		// consumed as shortcuts.
		if m.historyOpen {
			cmds = append(cmds, m.updateHistoryPicker(msg))
			break
		}
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

// recordHistory appends the just-finished execution to the history store.
// Failures are logged silently; history is non-essential so we never block
// the UI on a write error.
func (m *Model) recordHistory(msg executeMsg) {
	if m.historyStore == nil || strings.TrimSpace(msg.SQL) == "" {
		return
	}
	entry := history.Entry{
		Profile:    m.target.profile,
		Database:   m.target.database,
		SQL:        msg.SQL,
		At:         time.Now().UTC(),
		DurationMS: msg.Duration.Milliseconds(),
	}
	if msg.Err != nil {
		entry.Ok = false
		entry.ErrorMsg = msg.Err.Error()
	} else {
		entry.Ok = true
	}
	_ = m.historyStore.Append(entry)
}

// handleKey implements the global key bindings (run, focus, toggle, history,
// export, quit). It returns handled=true when the message was consumed and
// should not be forwarded to the focused pane.
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
		m.flashMessage = ""
		return tea.Batch(m.spin.Tick, runStatement(m.client, m.target, m.editor.Value())), true

	case key.Matches(msg, m.keys.Focus):
		m.toggleFocus()
		return nil, true

	case key.Matches(msg, m.keys.ToggleView):
		// Switching the underlying view mode also exits the inspector so
		// the user lands somewhere consistent.
		m.inspecting = false
		if m.mode == viewTable {
			m.mode = viewJSON
		} else {
			m.mode = viewTable
		}
		return nil, true

	case key.Matches(msg, m.keys.Inspect):
		// Enter only opens the row inspector when the user is actually
		// looking at table results. In every other state (editor focus,
		// JSON view, no result) Enter falls through to the focused pane
		// so it can act as a newline / scroll / etc.
		if m.focus != focusResults || m.mode != viewTable || m.result == nil || len(m.result.Rows) == 0 {
			return nil, false
		}
		m.openInspector(m.table.Cursor())
		return nil, true

	case key.Matches(msg, m.keys.History):
		return m.openHistoryPicker(), true

	case key.Matches(msg, m.keys.ExportCSV):
		m.handleExportCSV()
		return nil, true

	case key.Matches(msg, m.keys.Clear):
		if m.inspecting {
			m.inspecting = false
			return nil, true
		}
		if m.lastErr != nil || m.flashMessage != "" {
			m.lastErr = nil
			m.flashMessage = ""
			return nil, true
		}

	case key.Matches(msg, m.keys.Help):
		m.helpModel.ShowAll = !m.helpModel.ShowAll
		return nil, true
	}
	return nil, false
}

// openInspector loads the JSON of the row at idx into the inspector viewport
// and switches the result pane to inspect mode. The original table cursor is
// preserved so closing the inspector with Esc returns the user to the same
// row they were looking at.
func (m *Model) openInspector(idx int) {
	if m.result == nil || idx < 0 || idx >= len(m.result.Rows) {
		return
	}
	out, err := m.result.rowJSON(idx)
	if err != nil {
		m.lastErr = fmt.Errorf("inspect row: %w", err)
		return
	}
	m.inspectorVP.SetContent(out)
	m.inspectorVP.GotoTop()
	m.inspectedRow = idx
	m.inspecting = true
	m.lastErr = nil
}

// handleExportCSV writes the current result to a timestamped file in cwd and
// stores the outcome in flashMessage / lastErr for the footer to display.
func (m *Model) handleExportCSV() {
	if m.result == nil || len(m.result.Columns) == 0 {
		m.lastErr = fmt.Errorf("nothing to export")
		m.flashMessage = ""
		return
	}
	path, err := m.result.exportCSV()
	if err != nil {
		m.lastErr = fmt.Errorf("export failed: %w", err)
		m.flashMessage = ""
		return
	}
	m.lastErr = nil
	m.flashMessage = "exported " + shortenPath(path)
}

// openHistoryPicker loads entries for the current profile/database, populates
// the list, and synthesizes a "/" keystroke so the user is dropped straight
// into the incremental search prompt.
func (m *Model) openHistoryPicker() tea.Cmd {
	if m.historyStore == nil {
		m.lastErr = fmt.Errorf("history is unavailable")
		return nil
	}
	entries, err := m.historyStore.Load(m.target.profile, m.target.database)
	if err != nil {
		m.lastErr = fmt.Errorf("load history: %w", err)
		return nil
	}
	items := make([]list.Item, len(entries))
	for i, e := range entries {
		items[i] = historyItem{entry: e}
	}
	m.historyList.SetItems(items)
	m.historyList.ResetFilter()
	m.historyOpen = true
	m.flashMessage = ""

	if len(items) == 0 {
		// Nothing to filter; still open the picker so the user sees the
		// "No items" message rather than silently doing nothing.
		return nil
	}
	// Auto-enter filter mode for incremental search the moment the picker
	// opens. We synthesize a "/" keypress because bubbles/list does not
	// expose a public API to set the filter state directly.
	var cmd tea.Cmd
	m.historyList, cmd = m.historyList.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	return cmd
}

// updateHistoryPicker handles key input while the history picker is open.
// Enter loads the selected entry into the editor, Esc cancels (or clears the
// active filter first), and everything else is forwarded to bubbles/list.
func (m *Model) updateHistoryPicker(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "enter":
		if m.historyList.FilterState() == list.Filtering {
			// First Enter while typing: let bubbles/list apply the filter
			// rather than treating it as "select".
			break
		}
		if item, ok := m.historyList.SelectedItem().(historyItem); ok {
			m.editor.SetValue(item.entry.SQL)
			m.historyOpen = false
			m.focus = focusEditor
			m.editor.Focus()
			m.table.Blur()
			return nil
		}

	case "esc":
		// When a filter is active, let the list clear it on the first Esc.
		// A second Esc (Unfiltered state) closes the picker.
		if m.historyList.FilterState() == list.Unfiltered {
			m.historyOpen = false
			return nil
		}
	}

	var cmd tea.Cmd
	m.historyList, cmd = m.historyList.Update(msg)
	return cmd
}

// routeKey forwards an unconsumed key message to whichever pane currently
// holds focus. When the inspector is open it gets the keys so the user can
// scroll through the row's JSON.
func (m *Model) routeKey(msg tea.KeyMsg) tea.Cmd {
	if m.inspecting {
		var cmd tea.Cmd
		m.inspectorVP, cmd = m.inspectorVP.Update(msg)
		return cmd
	}
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

	m.inspectorVP.Width = innerW
	m.inspectorVP.Height = resultsH

	m.helpModel.Width = m.width

	// The history picker takes over the whole window minus status + help.
	pickerH := m.height - statusH - helpH - 2
	if pickerH < 6 {
		pickerH = 6
	}
	m.historyList.SetSize(m.width, pickerH)
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
			row[j] = truncate(formatCell(cell), w)
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
// and help bar. When the history picker is open it replaces the editor +
// results panes with the list view.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "loading..."
	}

	status := m.renderStatus()
	helpLine := m.helpModel.View(m.keys)

	if m.historyOpen {
		return strings.Join([]string{
			status,
			m.historyList.View(),
			helpBarStyle.Render(helpLine),
		}, "\n")
	}

	return strings.Join([]string{
		status,
		m.renderEditor(),
		m.renderResults(),
		m.renderFooter(),
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
	case m.inspecting:
		body = jsonStyle.Render(m.inspectorVP.View())
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
	if m.inspecting && m.result != nil {
		return successStyle.Render(fmt.Sprintf("inspecting row %d/%d · esc to close", m.inspectedRow+1, len(m.result.Rows)))
	}
	if m.flashMessage != "" {
		return successStyle.Render(m.flashMessage)
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
