package tui

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Tocyuki/rdq/internal/bedrock"
	"github.com/Tocyuki/rdq/internal/connection"
	"github.com/Tocyuki/rdq/internal/history"
	"github.com/Tocyuki/rdq/internal/schema"
	"github.com/Tocyuki/rdq/internal/state"
	"github.com/atotto/clipboard"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rdsdata"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// yankWindow is the maximum interval between two y presses for them to
// register as a vim-style "yy" yank inside the explain overlay.
const yankWindow = 600 * time.Millisecond

// errClipboardCopy is wrapped into m.lastErr when copyResultContext fails to
// hand the payload to the OS clipboard — typically on a headless Linux CI
// runner with no xclip/xsel installed. Tests match on it via errors.Is so
// they can tolerate the benign headless failure without coupling to the
// wrapped error text.
var errClipboardCopy = errors.New("copy failed")

// askKind discriminates the three flows that share the askInput overlay.
type askKind int

const (
	askKindGenerate askKind = iota
	askKindReview
	askKindAnalyze
)

// flashLifetime is how long a transient flash message stays on screen
// before auto-clearing. 2.5s is long enough to read but short enough that
// the next user action sees a clean status line.
const flashLifetime = 2500 * time.Millisecond

// askInputHeight is the visible row count of the natural-language ask /
// review / analyze textarea overlay. Four lines is enough to show a
// short multi-line prompt without dominating the screen.
const askInputHeight = 4

// clearFlashMsg is delivered by a tea.Tick scheduled when a flash message
// is set. The token must match the current flashToken at receive time;
// otherwise a newer flash has already replaced this one and the clear is
// silently dropped (so a fresh flash isn't wiped by a stale timer).
type clearFlashMsg struct {
	token uint64
}

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
	// colCursor tracks the "column under focus" inside the results
	// table. bubbles/table v1 only has a row cursor, so we maintain a
	// parallel column index and move it via vim-style h/l. Persisted in
	// the footer as "col N/M · name" so the user always knows where the
	// cursor sits.
	colCursor int

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

	// AI ask state. bedrockModel is the cached model ID (resolved from
	// state or set by the picker). askInput overlays the editor for
	// natural-language input. modelList is shown the first time a model is
	// needed; pendingAsk / pendingExplain chain the model picker into the
	// follow-up flow automatically. snapshot holds the schema once it has
	// loaded asynchronously.
	bedrockClient   *bedrock.Client
	bedrockModel    string
	bedrockLanguage string
	snapshot        *schema.Snapshot
	askInput        textarea.Model
	askOpen         bool
	askExecuting    bool
	askKind         askKind
	// pending* hold the editor SQL / result snapshot captured when F6
	// opens the focus-area overlay, so the model sees the state the
	// user actually pointed at even if they mutate the editor / result
	// while typing.
	pendingReviewSQL   string
	pendingAnalyzeSQL  string
	pendingAnalyzeBlob string
	modelList          list.Model
	modelPickerOpen    bool
	languageList       list.Model
	languagePickerOpen bool
	pendingAsk         bool
	pendingExplain     bool
	// askChat is the running multi-turn conversation history for Ask AI
	// within this TUI session. Each Ctrl+G round trip appends the user
	// prompt and the assistant's reply, so successive prompts inherit
	// context (e.g. "now sort the previous query by created_at"). The
	// slice is reset only on TUI restart.
	askChat []bedrock.Message

	// AI error explanation overlay. When explainOpen is true the results
	// pane shows the analysis in explainVP instead of the raw error.
	// explainExecuting toggles the spinner during the Bedrock round trip;
	// lastSQL caches the most recently executed statement so the analyst
	// has the source text even after the editor has changed. explainText
	// keeps the raw markdown so vim-style yy can copy it to the clipboard.
	// lastYank tracks the timestamp of the previous y press so two y's
	// within a short window register as the yank command.
	explainOpen      bool
	explainExecuting bool
	explainVP        viewport.Model
	explainText      string
	lastSQL          string
	lastYank         time.Time
	// aiBusyLabel is the text shown next to the spinner in the results
	// pane while explainExecuting is true. Set by startExplain /
	// startReview / startAnalyze to reflect the current operation; an
	// empty string falls back to a generic "asking AI..." label.
	aiBusyLabel string
	// lastG tracks the timestamp of the previous lone "g" press inside a
	// viewport overlay so that two presses within yankWindow act as the
	// vim "gg" go-to-top command.
	lastG time.Time

	// Cluster / secret switching overlays. awsConfig is held so the
	// in-TUI cluster picker can call DescribeDBClusters / ListSecrets on
	// demand. clusterList is populated by an async cmd; once a cluster is
	// chosen we asynchronously resolve its secrets into secretList. The
	// selected cluster is parked in pendingCluster while waiting for the
	// secret resolution to finish.
	awsConfig         aws.Config
	clusterList       list.Model
	clusterPickerOpen bool
	clusterLoading    bool
	secretList        list.Model
	secretPickerOpen  bool
	secretLoading     bool
	pendingCluster    connection.ClusterInfo
	// forceSecretPicker is set when the user pressed Ctrl+\ to switch
	// secrets within the current cluster. It tells the secretsLoadedMsg
	// handler to always show the picker, even if there is only a single
	// candidate (the default behaviour for cluster switching is to skip
	// the picker on a unique match).
	forceSecretPicker bool

	// Profile switching overlay. Reuses the same picker UX as cluster /
	// secret. profileSwitching is true between "user picked a profile"
	// and "new aws.Config arrived" so the footer can show progress.
	profileList       list.Model
	profilePickerOpen bool
	profileLoading    bool
	profileSwitching  bool

	// Database picker shown mid cluster-switch so the user always
	// confirms the target database name instead of silently inheriting
	// whatever was in m.target.database from the previous cluster.
	// pendingOldCluster / pendingOldSecret snapshot the pre-switch
	// target so an Esc while the picker is open reverts the entire
	// cluster switch.
	databaseList       list.Model
	databasePickerOpen bool
	pendingOldCluster  string
	pendingOldSecret   string

	// Production-environment flag + prompt. isProduction drives the
	// warning theme (red borders, PRODUCTION banner) applied at render
	// time. productionPromptOpen makes a small Yes/No picker the only
	// thing that accepts input, forcing the user to answer when the
	// flag has never been set for the active profile. The prompt opens
	// automatically on TUI start + after Ctrl+P profile switch when the
	// state file has no stored value, and can be reopened any time via
	// F7 to flip the setting.
	isProduction        bool
	productionList      list.Model
	productionPromptOpen bool

	// Transient status message (e.g. CSV export confirmation, yy yank).
	// flashToken is bumped every time flashMessage is set so a tea.Tick
	// auto-clear can ignore stale ticks for messages that have already
	// been replaced by a newer one.
	flashMessage string
	flashToken   uint64

	// Result search state ("/" opens the prompt, n/N navigate matches).
	// searchOpen is true while the input prompt is visible and consumes
	// keys. searchQuery holds the committed query that drives highlight
	// and navigation; it survives mode toggles and only clears on a new
	// SQL execution or explicit cancel. jsonRaw is the plain (pre-Chroma)
	// JSON text used both for matching and for line-level rerendering.
	searchInput  textinput.Model
	searchOpen   bool
	searchQuery  string
	tableHits    []tableHit
	jsonHits     []jsonHit
	searchCursor int
	jsonRaw      string
}

// historyItem adapts history.Entry to the list.Item interface so the picker
// can render and filter entries. A leading ★ marks favorites so they stand
// out in the list view.
type historyItem struct {
	entry history.Entry
}

func (i historyItem) FilterValue() string { return i.entry.SQL }
func (i historyItem) Title() string {
	prefix := "  "
	if i.entry.Favorite {
		prefix = "★ "
	}
	return prefix + summarizeSQL(i.entry.SQL)
}
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
func newModel(client *rdsdata.Client, tgt target, store *history.Store, bedrockClient *bedrock.Client, bedrockModel, bedrockLanguage string, awsCfg aws.Config) Model {
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
	explain := viewport.New(0, 0)
	// SetHorizontalStep is required because viewport's default step of
	// 0 makes ScrollLeft / ScrollRight no-ops on the h/l bindings below.
	const horizontalStep = 4
	vp.SetHorizontalStep(horizontalStep)
	inspector.SetHorizontalStep(horizontalStep)
	explain.SetHorizontalStep(horizontalStep)

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
	hl.Title = "SQL history (type to filter, Enter to load, ^F to favorite, Esc to cancel)"
	hl.SetShowStatusBar(true)
	hl.SetFilteringEnabled(true)
	hl.SetShowHelp(false)
	hl.Filter = containsFilter

	ml := list.New(nil, delegate, 0, 0)
	ml.Title = "Bedrock model (type to filter, Enter to select, Esc to cancel)"
	ml.SetShowStatusBar(true)
	ml.SetFilteringEnabled(true)
	ml.SetShowHelp(false)
	ml.Filter = containsFilter

	ll := list.New(languageItems(), delegate, 0, 0)
	ll.Title = "Response language (type to filter, Enter to select, Esc to cancel)"
	ll.SetShowStatusBar(true)
	ll.SetFilteringEnabled(true)
	ll.SetShowHelp(false)
	ll.Filter = containsFilter

	cl := list.New(nil, delegate, 0, 0)
	cl.Title = "RDS cluster (type to filter, Enter to select, Esc to cancel)"
	cl.SetShowStatusBar(true)
	cl.SetFilteringEnabled(true)
	cl.SetShowHelp(false)
	cl.Filter = containsFilter

	sl := list.New(nil, delegate, 0, 0)
	sl.Title = "Secret (type to filter, Enter to select, Esc to cancel)"
	sl.SetShowStatusBar(true)
	sl.SetFilteringEnabled(true)
	sl.SetShowHelp(false)
	sl.Filter = containsFilter

	pl := list.New(nil, delegate, 0, 0)
	pl.Title = "AWS profile (type to filter, Enter to select, Esc to cancel)"
	pl.SetShowStatusBar(true)
	pl.SetFilteringEnabled(true)
	pl.SetShowHelp(false)
	pl.Filter = containsFilter

	dl := list.New(nil, delegate, 0, 0)
	dl.Title = "Database (type new name or pick from history, Enter to confirm, Esc to cancel)"
	dl.SetShowStatusBar(true)
	dl.SetFilteringEnabled(true)
	dl.SetShowHelp(false)
	dl.Filter = containsFilter

	// Production flag picker. Fixed 2 rows, non-filterable because the
	// whole point is a binary confirmation and leaving filtering on
	// would let the user accidentally type themselves into an empty
	// list with no way to commit.
	prodList := list.New([]list.Item{
		productionItem{
			title:       "Yes — this is a PRODUCTION environment",
			description: "rdq will paint the UI red and show a PRODUCTION banner",
			value:       true,
		},
		productionItem{
			title:       "No — non-production (dev / staging / local)",
			description: "normal blue theme",
			value:       false,
		},
	}, delegate, 0, 0)
	prodList.Title = "Is this profile a production environment? (Enter to confirm, Esc to cancel)"
	prodList.SetShowStatusBar(false)
	prodList.SetFilteringEnabled(false)
	prodList.SetShowHelp(false)

	ask := textarea.New()
	ask.Placeholder = "Ask in natural language (e.g. \"top 10 active users this week\")"
	ask.CharLimit = 4000
	ask.Prompt = ""
	ask.ShowLineNumbers = false
	ask.SetHeight(askInputHeight)
	// Repurpose the textarea so plain Enter can act as "submit" at the
	// updateAskInput layer. Without this override the textarea swallows
	// Enter as InsertNewline and our submit branch never fires. Newline
	// is bound to Ctrl+J (LF, 0x0A) and Alt+Enter (\x1b\r) — both arrive
	// as distinct byte sequences on every POSIX terminal, unlike
	// Shift+Enter which requires kitty keyboard protocol / CSI u
	// support that bubbletea v1 does not parse.
	ask.KeyMap.InsertNewline.SetKeys("alt+enter", "ctrl+j")

	searchIn := textinput.New()
	searchIn.Prompt = "/"
	searchIn.Placeholder = "search in results (case-insensitive)"
	searchIn.CharLimit = 256

	m := Model{
		editor:          editor,
		table:           tbl,
		jsonView:        vp,
		inspectorVP:     inspector,
		explainVP:       explain,
		helpModel:       hm,
		spin:            sp,
		keys:            defaultKeyMap(),
		client:          client,
		target:          tgt,
		focus:           focusEditor,
		mode:            viewTable,
		historyStore:    store,
		historyList:     hl,
		bedrockClient:   bedrockClient,
		bedrockModel:    bedrockModel,
		bedrockLanguage: bedrockLanguage,
		askInput:        ask,
		modelList:       ml,
		languageList:    ll,
		clusterList:     cl,
		secretList:      sl,
		profileList:     pl,
		databaseList:    dl,
		productionList:  prodList,
		awsConfig:       awsCfg,
		searchInput:     searchIn,
	}
	// Hydrate the production flag from state for the initial profile so
	// the warning theme is live on first render. When no value is stored
	// yet we open the prompt immediately so the user is forced to answer
	// before running any queries.
	if answered := m.loadProductionFlag(); !answered {
		m.openProductionPrompt()
	}
	return m
}

// Init starts the spinner ticker and kicks off an asynchronous schema fetch
// so the AI prompt has up-to-date table/column context when the user invokes
// it. The TUI does not block on the fetch; if it is still running when the
// user presses Ctrl+G we send a degraded prompt without schema.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		m.spin.Tick,
		fetchSchemaCmd(m.client, m.target),
	)
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
		if m.executing || m.askExecuting || m.explainExecuting {
			var cmd tea.Cmd
			m.spin, cmd = m.spin.Update(msg)
			cmds = append(cmds, cmd)
		}

	case executeMsg:
		m.executing = false
		m.duration = msg.Duration
		m.flashMessage = ""
		m.lastSQL = msg.SQL
		// A new execution invalidates any previous error analysis.
		m.explainOpen = false
		m.recordHistory(msg)
		if msg.Err != nil {
			m.lastErr = msg.Err
			m.result = nil
			m.jsonText = ""
		} else {
			m.lastErr = nil
			m.result = msg.Result
			m.jsonRaw = msg.Result.toJSON()
			m.jsonText = m.jsonRaw
			m.colCursor = 0
			// clearSearch wipes any previous query + hit state and
			// re-renders both panes, so we do not need to call the
			// refresh helpers a second time below.
			m.clearSearch()
			m.focus = focusResults
			m.editor.Blur()
			m.table.Focus()
		}

	case clearFlashMsg:
		// Drop the flash only if no newer message has replaced it.
		if msg.token == m.flashToken {
			m.flashMessage = ""
		}

	case schemaLoadedMsg:
		if msg.err != nil {
			log.Printf("schema fetch failed: %v", msg.err)
			break
		}
		m.snapshot = msg.snapshot

	case modelsLoadedMsg:
		if msg.err != nil {
			m.lastErr = fmt.Errorf("list bedrock models: %w", msg.err)
			m.modelPickerOpen = false
			m.pendingAsk = false
			m.pendingExplain = false
			break
		}
		items := make([]list.Item, len(msg.models))
		for i, model := range msg.models {
			items[i] = modelItem{model: model}
		}
		m.modelList.SetItems(items)
		// Filtering is driven by drivePicker via SetFilterText; just
		// clear any leftover filter from the previous open so the list
		// shows everything until the user types.
		m.modelList.ResetFilter()

	case askResultMsg:
		m.askExecuting = false
		if msg.err != nil {
			m.lastErr = fmt.Errorf("ask AI: %w", msg.err)
			// Pop the just-sent user turn so retrying with a new
			// prompt does not duplicate the failed message.
			if n := len(m.askChat); n > 0 && m.askChat[n-1].Role == bedrock.RoleUser {
				m.askChat = m.askChat[:n-1]
			}
			break
		}
		m.askChat = append(m.askChat, bedrock.Message{Role: bedrock.RoleAssistant, Text: msg.sql})
		m.applyAskResult(msg.prompt, msg.sql)

	case explainResultMsg:
		m.explainExecuting = false
		m.aiBusyLabel = ""
		if msg.err != nil {
			m.lastErr = fmt.Errorf("explain error: %w", msg.err)
			break
		}
		// Prepend the verbatim error so the user always sees what the
		// database actually returned, regardless of how the LLM chose
		// to summarise it. Composed text is what gets yanked by yy.
		m.showAIOverlay(composeExplainText(m.lastErr, msg.explanation))

	case reviewResultMsg:
		m.explainExecuting = false
		m.aiBusyLabel = ""
		if msg.err != nil {
			m.lastErr = fmt.Errorf("review SQL: %w", msg.err)
			break
		}
		m.showAIOverlay(composeReviewText(msg.sql, msg.review))

	case analyzeResultMsg:
		m.explainExecuting = false
		m.aiBusyLabel = ""
		if msg.err != nil {
			m.lastErr = fmt.Errorf("analyze result: %w", msg.err)
			break
		}
		m.showAIOverlay(composeAnalyzeText(msg.analysis))

	case clustersLoadedMsg:
		m.clusterLoading = false
		if msg.err != nil {
			m.lastErr = fmt.Errorf("list clusters: %w", msg.err)
			m.clusterPickerOpen = false
			break
		}
		items := make([]list.Item, len(msg.clusters))
		for i, c := range msg.clusters {
			items[i] = clusterItem{cluster: c}
		}
		m.clusterList.SetItems(items)
		m.clusterList.ResetFilter()
		m.clusterPickerOpen = true

	case profilesLoadedMsg:
		m.profileLoading = false
		if msg.err != nil {
			m.lastErr = fmt.Errorf("list profiles: %w", msg.err)
			break
		}
		items := make([]list.Item, len(msg.profiles))
		for i, p := range msg.profiles {
			items[i] = profileItem{name: p}
		}
		m.profileList.SetItems(items)
		m.profileList.ResetFilter()
		m.profilePickerOpen = true

	case profileSwitchedMsg:
		m.profileSwitching = false
		if msg.err != nil {
			m.lastErr = fmt.Errorf("switch profile: %w", msg.err)
			break
		}
		cmds = append(cmds, m.applyProfileSwitch(msg.profile, msg.cfg))

	case secretsLoadedMsg:
		m.secretLoading = false
		forced := m.forceSecretPicker
		m.forceSecretPicker = false
		if msg.err != nil {
			m.lastErr = fmt.Errorf("resolve secret: %w", msg.err)
			m.secretPickerOpen = false
			break
		}
		// One unambiguous candidate → switch immediately, no picker.
		// Forced flow (Ctrl+\\) skips this so the user can always see
		// the picker and confirm the choice explicitly.
		if !forced && len(msg.secrets) == 1 && !msg.fallback {
			cmds = append(cmds, m.beginTargetSwitch(msg.cluster, msg.secrets[0].ARN))
			break
		}
		// Multiple candidates (or fallback to all secrets) → show picker.
		items := make([]list.Item, len(msg.secrets))
		for i, s := range msg.secrets {
			items[i] = secretItem{secret: s}
		}
		m.secretList.SetItems(items)
		m.secretList.ResetFilter()
		if msg.fallback {
			m.secretList.Title = "Secret (no cluster match — type to filter, Enter to select, Esc to cancel)"
		} else {
			m.secretList.Title = "Secret (type to filter, Enter to select, Esc to cancel)"
		}
		m.secretPickerOpen = true
		m.pendingCluster = msg.cluster

	case tea.KeyMsg:
		// Picker / overlay modes consume input first so typing into them
		// does not trigger global shortcuts.
		switch {
		case m.searchOpen:
			cmds = append(cmds, m.updateSearchInput(msg))
		case m.askOpen:
			cmds = append(cmds, m.updateAskInput(msg))
		case m.profilePickerOpen:
			cmds = append(cmds, m.updateProfilePicker(msg))
		case m.clusterPickerOpen:
			cmds = append(cmds, m.updateClusterPicker(msg))
		case m.secretPickerOpen:
			cmds = append(cmds, m.updateSecretPicker(msg))
		case m.databasePickerOpen:
			cmds = append(cmds, m.updateDatabasePicker(msg))
		case m.productionPromptOpen:
			cmds = append(cmds, m.updateProductionPrompt(msg))
		case m.modelPickerOpen:
			cmds = append(cmds, m.updateModelPicker(msg))
		case m.languagePickerOpen:
			cmds = append(cmds, m.updateLanguagePicker(msg))
		case m.historyOpen:
			cmds = append(cmds, m.updateHistoryPicker(msg))
		default:
			if cmd, handled := m.handleKey(msg); handled {
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			} else {
				cmds = append(cmds, m.routeKey(msg))
			}
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
		// Re-align the search cursor in the new mode: the hit lists
		// have different lengths, so resetting to 0 and rescrolling is
		// the least surprising behaviour. Then rerender so highlights
		// appear in the view the user just switched to.
		if m.searchQuery != "" {
			m.searchCursor = 0
			m.alignSearchAnchors()
			m.refreshTable()
			m.refreshJSON()
			m.focusSearchCursor()
		}
		return nil, true

	case key.Matches(msg, m.keys.Inspect):
		// Enter is a toggle: a second press while the inspector is open
		// closes it and returns to the table view, mirroring how the
		// user expects "back" to work.
		if m.inspecting {
			m.inspecting = false
			return nil, true
		}
		// Otherwise Enter only opens the row inspector when the user is
		// actually looking at table results. In every other state
		// (editor focus, JSON view, no result) Enter falls through to
		// the focused pane so it can act as a newline / scroll / etc.
		if m.focus != focusResults || m.mode != viewTable || m.result == nil || len(m.result.Rows) == 0 {
			return nil, false
		}
		m.openInspector(m.table.Cursor())
		return nil, true

	case key.Matches(msg, m.keys.History):
		return m.openHistoryPicker(), true

	case key.Matches(msg, m.keys.Ask):
		// Ctrl+G is the SQL generation shortcut: it always opens the
		// natural-language prompt input.
		return m.startAsk(), true

	case key.Matches(msg, m.keys.Assist):
		// F6 is the unified "ask the model about what is currently on
		// screen" shortcut — review / analyze / explain are chosen
		// automatically from focus + pane contents.
		return m.dispatchAssist(), true

	case key.Matches(msg, m.keys.SwitchModel):
		return m.openModelPicker(false), true

	case key.Matches(msg, m.keys.SwitchLanguage):
		return m.openLanguagePicker(), true

	case key.Matches(msg, m.keys.SwitchTarget):
		return m.openClusterPicker(), true

	case key.Matches(msg, m.keys.SwitchSecret):
		return m.openSecretPicker(), true

	case key.Matches(msg, m.keys.SwitchProfile):
		return m.openProfilePicker(), true

	case key.Matches(msg, m.keys.ToggleProduction):
		m.openProductionPrompt()
		return nil, true

	case key.Matches(msg, m.keys.ExportCSV):
		m.handleExportCSV()
		return nil, true

	case key.Matches(msg, m.keys.Clear):
		if m.explainOpen {
			m.explainOpen = false
			return nil, true
		}
		if m.inspecting {
			m.inspecting = false
			return nil, true
		}
		if m.searchQuery != "" {
			// First Esc press while search highlights are active just
			// clears them. A second Esc (no query, no error, no flash)
			// falls through to the no-op.
			m.clearSearch()
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
	// Set the highlighted JSON as-is so long string values stay on a
	// single line. The inspector supports horizontal scrolling (h/l/0/$)
	// to walk across rows that exceed the viewport width.
	m.inspectorVP.SetContent(highlightJSON(out))
	m.inspectorVP.GotoTop()
	m.inspectorVP.SetXOffset(0)
	m.inspectedRow = idx
	m.inspecting = true
	m.lastErr = nil
	// Pin focus to the results pane so the yy yank shortcut and the
	// inspector's own scroll keys keep working until the user closes it.
	m.pinResultsFocus()
}

// pinResultsFocus moves keyboard focus to the results pane so the vim
// navigation keys (h/l/gg/G/yy) and viewport scrolling are live without
// requiring the user to press Tab. Shared by openInspector and
// showAIOverlay, both of which would otherwise leave focus stranded on
// the editor pane while a results-side overlay sits in front.
func (m *Model) pinResultsFocus() {
	m.focus = focusResults
	m.editor.Blur()
	m.table.Focus()
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

// openHistoryPicker loads entries for the current profile/database and
// opens the picker. drivePicker handles incremental search via
// SetFilterText so we don't need to synthesize a "/" keystroke.
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
	// Float favorites to the top of the picker so they are reachable
	// without scrolling. Within each group the existing most-recent-first
	// ordering from the store is preserved.
	favorites := make([]history.Entry, 0, len(entries))
	rest := make([]history.Entry, 0, len(entries))
	for _, e := range entries {
		if e.Favorite {
			favorites = append(favorites, e)
		} else {
			rest = append(rest, e)
		}
	}
	ordered := append(favorites, rest...)
	items := make([]list.Item, len(ordered))
	for i, e := range ordered {
		items[i] = historyItem{entry: e}
	}
	m.historyList.SetItems(items)
	m.historyList.ResetFilter()
	m.historyOpen = true
	m.flashMessage = ""
	return nil
}

// updateHistoryPicker handles key input while the history picker is open.
// Enter loads the selected entry into the editor, Esc cancels, and any
// other key is forwarded to drivePicker which handles navigation +
// incremental search synchronously via SetFilterText.
func (m *Model) updateHistoryPicker(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "enter":
		if item, ok := m.historyList.SelectedItem().(historyItem); ok {
			m.editor.SetValue(item.entry.SQL)
			m.historyOpen = false
			m.focus = focusEditor
			m.editor.Focus()
			m.table.Blur()
			return nil
		}
		return nil
	case "esc":
		m.historyOpen = false
		return nil
	case "ctrl+f":
		m.toggleHistoryFavorite()
		return nil
	}
	drivePicker(&m.historyList, msg)
	return nil
}

// toggleHistoryFavorite flips the Favorite flag on the entry currently
// highlighted in the history picker, persists it to disk, and refreshes
// the picker so the ★ marker updates immediately. Selection / scroll
// position are preserved.
func (m *Model) toggleHistoryFavorite() {
	if m.historyStore == nil {
		return
	}
	item, ok := m.historyList.SelectedItem().(historyItem)
	if !ok {
		return
	}
	newFav := !item.entry.Favorite
	if err := m.historyStore.SetFavorite(item.entry.At, newFav); err != nil {
		log.Printf("toggle history favorite failed: %v", err)
		return
	}
	cursor := m.historyList.Index()
	items := m.historyList.Items()
	for i, it := range items {
		hi, ok := it.(historyItem)
		if !ok {
			continue
		}
		if hi.entry.At.Equal(item.entry.At) {
			hi.entry.Favorite = newFav
			items[i] = hi
		}
	}
	m.historyList.SetItems(items)
	m.historyList.Select(cursor)
}

// drivePicker is the canonical input handler for our bubbles/list pickers.
// We bypass the list's own filter input plumbing entirely because routing
// async FilterMatchesMsg through our reducer was unreliable across multiple
// attempts. Instead we keep our own filter buffer using the list's public
// SetFilterText / SetFilterState API, which performs filtering synchronously
// — typing a character immediately collapses VisibleItems to the matches.
//
// Navigation (up/down/pageup/pagedown/home/end) is delegated to bubbles/list
// directly via its public cursor methods so it still highlights and paginates
// the filtered slice correctly.
func drivePicker(l *list.Model, msg tea.KeyMsg) {
	switch msg.Type {
	case tea.KeyUp:
		l.CursorUp()
		return
	case tea.KeyDown:
		l.CursorDown()
		return
	case tea.KeyPgUp:
		l.Paginator.PrevPage()
		return
	case tea.KeyPgDown:
		l.Paginator.NextPage()
		return
	case tea.KeyHome:
		l.GoToStart()
		return
	case tea.KeyEnd:
		l.GoToEnd()
		return
	case tea.KeyBackspace:
		runes := []rune(l.FilterValue())
		if len(runes) == 0 {
			return
		}
		runes = runes[:len(runes)-1]
		applyPickerFilter(l, string(runes))
		return
	case tea.KeySpace:
		// bubbletea delivers space as its own KeyType rather than as a
		// KeyRunes message, so without this case typing " " into the
		// filter is silently dropped.
		applyPickerFilter(l, l.FilterValue()+" ")
		return
	}

	if msg.Alt || msg.Type != tea.KeyRunes || len(msg.Runes) == 0 {
		return
	}
	applyPickerFilter(l, l.FilterValue()+string(msg.Runes))
}

// applyPickerFilter sets the filter text synchronously and leaves the list
// in FilterApplied state. This is critical: bubbles/list's DefaultDelegate
// suppresses the cursor selection highlight while filterState == Filtering
// (see defaultitem.go: `isSelected && m.FilterState() != Filtering`), so
// keeping the list in Filtering mode would let the cursor move invisibly.
// FilterApplied gives us both the filtered items AND a visible cursor, and
// the status bar still shows "<filter>" so the user knows what's typed.
func applyPickerFilter(l *list.Model, filter string) {
	if filter == "" {
		l.ResetFilter()
		return
	}
	l.SetFilterText(filter)
}

// containsFilter is a substring matcher we install in place of the
// bubbles/list default. The default uses sahilm/fuzzy, which matches "lla"
// against "Claude" because the two l's and one a appear in order somewhere
// in the string — surprising for users who expect a literal substring
// search like a command palette. Case is folded for convenience.
func containsFilter(term string, targets []string) []list.Rank {
	if term == "" {
		out := make([]list.Rank, len(targets))
		for i := range targets {
			out[i] = list.Rank{Index: i}
		}
		return out
	}
	needle := strings.ToLower(term)
	var out []list.Rank
	for i, t := range targets {
		hay := strings.ToLower(t)
		idx := strings.Index(hay, needle)
		if idx < 0 {
			continue
		}
		matches := make([]int, 0, len(needle))
		for j := 0; j < len(needle); j++ {
			matches = append(matches, idx+j)
		}
		out = append(out, list.Rank{Index: i, MatchedIndexes: matches})
	}
	return out
}

// routeKey forwards an unconsumed key message to whichever pane currently
// holds focus. The explain overlay and row inspector live in the results
// pane, so they only intercept keys while focus is on results — the editor
// stays editable even when an analysis is on screen, and the user can Tab
// over to scroll / yank.
func (m *Model) routeKey(msg tea.KeyMsg) tea.Cmd {
	// "/" opens the incremental search prompt whenever the user is
	// looking at results. n / N cycle through matches of the last
	// committed query (vim style). These shortcuts are results-pane
	// only so they don't shadow printable characters in the editor.
	if m.focus == focusResults && !m.inspecting && !m.explainOpen {
		switch msg.String() {
		case "/":
			return m.openSearch()
		case "n":
			if m.searchQuery != "" {
				m.nextHit(1)
				return nil
			}
		case "N":
			if m.searchQuery != "" {
				m.nextHit(-1)
				return nil
			}
		}
	}

	// vim-style yy yank works whenever focus is on the results pane,
	// regardless of which sub-mode (table / json / inspector / explain)
	// is showing. copyResultContext picks the right payload.
	if m.focus == focusResults && msg.String() == "y" {
		if !m.lastYank.IsZero() && time.Since(m.lastYank) <= yankWindow {
			m.copyResultContext()
			m.lastYank = time.Time{}
			if m.flashMessage != "" {
				return m.scheduleFlashClear()
			}
			return nil
		}
		m.lastYank = time.Now()
		return nil
	}
	if m.focus == focusResults {
		m.lastYank = time.Time{}
	}

	if m.explainOpen && m.focus == focusResults {
		return m.updateOverlayViewport(&m.explainVP, msg)
	}
	if m.inspecting && m.focus == focusResults {
		return m.updateOverlayViewport(&m.inspectorVP, msg)
	}
	var cmd tea.Cmd
	switch m.focus {
	case focusEditor:
		m.editor, cmd = m.editor.Update(msg)
	case focusResults:
		if m.mode == viewTable {
			// vim-style horizontal movement on the column cursor.
			// bubbles/table only tracks rows, so we maintain the
			// column index ourselves and show it in the footer.
			switch msg.String() {
			case "h", "left":
				if m.colCursor > 0 {
					m.colCursor--
					m.refreshTable()
				}
				return nil
			case "l", "right":
				if m.result != nil && m.colCursor < len(m.result.Columns)-1 {
					m.colCursor++
					m.refreshTable()
				}
				return nil
			case "0", "home":
				if m.colCursor != 0 {
					m.colCursor = 0
					m.refreshTable()
				}
				return nil
			case "$", "end":
				if m.result != nil && len(m.result.Columns) > 0 {
					last := len(m.result.Columns) - 1
					if m.colCursor != last {
						m.colCursor = last
						m.refreshTable()
					}
				}
				return nil
			}
			m.table, cmd = m.table.Update(msg)
		} else {
			// JSON view shares the inspector / explain vim keys
			// for top/bottom and horizontal navigation. Consume the
			// key here so it isn't also handed to viewport.Update as
			// a printable rune.
			if m.handleViewportNav(&m.jsonView, msg) {
				return nil
			}
			m.jsonView, cmd = m.jsonView.Update(msg)
		}
	}
	return cmd
}

// toggleFocus moves keyboard focus between the editor and the results pane,
// updating per-component focus state so the cursor blink and table highlight
// follow accordingly.
func (m *Model) toggleFocus() {
	// While the row inspector is open the results pane is the only
	// sensible focus target — the user is reading / scrolling / yanking
	// row JSON, and switching to the editor would silently disable yy.
	// Tab is therefore a no-op until the inspector is closed.
	if m.inspecting {
		return
	}
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
	statusH := 4
	if m.isProduction {
		// Production banner adds one line to the status bar, so we
		// must shrink the available content region by the same amount
		// to avoid the editor / results being cut off at the bottom.
		statusH = 5
	}
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

	m.explainVP.Width = innerW
	m.explainVP.Height = resultsH

	m.helpModel.Width = m.width

	// History and model pickers take over the whole window minus status +
	// help. The askInput needs only the inner width.
	pickerH := m.height - statusH - helpH - 2
	if pickerH < 6 {
		pickerH = 6
	}
	m.historyList.SetSize(m.width, pickerH)
	m.modelList.SetSize(m.width, pickerH)
	m.languageList.SetSize(m.width, pickerH)
	m.clusterList.SetSize(m.width, pickerH)
	m.secretList.SetSize(m.width, pickerH)
	m.profileList.SetSize(m.width, pickerH)
	m.databaseList.SetSize(m.width, pickerH)
	m.productionList.SetSize(m.width, pickerH)
	m.askInput.SetWidth(innerW)
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
	widths := m.result.Widths()

	// Each rendered column reserves perColumnPad cells for bubbles/table's
	// internal cell padding, plus an extra cursorMarkerWidth so the "▸ "
	// prefix on the active column does not push the title past the column
	// width and corrupt the layout. We add the marker padding to *every*
	// column rather than only the active one so column widths stay stable
	// when the cursor moves.
	const perColumnPad = 2
	const cursorMarkerWidth = 2 // "▸ "
	const widthSlack = 6

	innerW := m.table.Width()
	if innerW <= 0 {
		innerW = m.width
	}
	available := innerW - widthSlack
	if available < 1 {
		available = 1
	}

	start := 0
	end := len(m.result.Columns)
	if m.colCursor < 0 {
		m.colCursor = 0
	}
	if m.colCursor >= len(m.result.Columns) {
		m.colCursor = len(m.result.Columns) - 1
	}
	colWidthFor := func(i int) int { return widths[i] + perColumnPad + cursorMarkerWidth }
	if len(m.result.Columns) > 0 {
		// Grow a window [start, end) around colCursor that fits within
		// `available`. Columns before the cursor are added only if
		// there is room left after the cursor's own column is secured.
		start = m.colCursor
		end = m.colCursor + 1
		used := colWidthFor(m.colCursor)
		// Try to extend forward first (keeps cursor on the left edge
		// when scrolling right), then backward.
		for end < len(m.result.Columns) && used+colWidthFor(end) <= available {
			used += colWidthFor(end)
			end++
		}
		for start > 0 && used+colWidthFor(start-1) <= available {
			start--
			used += colWidthFor(start)
		}
	}

	visible := m.result.Columns[start:end]
	cols := make([]table.Column, len(visible))
	for i, c := range visible {
		title := "  " + c
		if start+i == m.colCursor {
			// Mark the cursor column so the user can see where it is
			// even without a real cell-level highlight (bubbles/table
			// v1 only highlights rows). The leading marker has the
			// same width as the empty padding on inactive columns so
			// switching the cursor never reflows the layout.
			title = "▸ " + c
		}
		cols[i] = table.Column{Title: title, Width: colWidthFor(start + i)}
	}

	curHit := m.currentTableHit()
	rows := make([]table.Row, len(m.result.Rows))
	for i, r := range m.result.Rows {
		row := make(table.Row, len(visible))
		for j := range visible {
			absoluteCol := start + j
			w := columnWidthCap
			if absoluteCol < len(widths) {
				w = widths[absoluteCol]
			}
			var cell any
			if absoluteCol < len(r) {
				cell = r[absoluteCol]
			}
			shown := truncate(formatCell(cell), w)
			if m.searchQuery != "" {
				shown = highlightCell(shown, m.searchQuery, curHit, i, absoluteCol)
			}
			// Indent rows by the marker width so cell content lines
			// up under the column title (which always carries the
			// 2-cell marker prefix).
			row[j] = "  " + shown
		}
		rows[i] = row
	}
	m.table.SetRows(nil)
	m.table.SetColumns(cols)
	m.table.SetRows(rows)
	// SetRows(nil) leaves the underlying cursor at -1 because bubbles/table
	// only clamps the cursor *down* (cursor > len-1), never up from a
	// negative. Without this reset openInspector(-1) is silently a no-op
	// the first time the user presses Enter on a fresh result.
	if len(rows) > 0 && m.table.Cursor() < 0 {
		m.table.SetCursor(0)
	}
}

// refreshJSON updates the JSON viewport content. When no search is active the
// whole body is rendered through Chroma so numbers / strings / booleans
// stand out with monokai colours. When a search query is active the matched
// lines are rebuilt from the plain-text jsonRaw with lipgloss match
// highlights — those lines temporarily lose Chroma colouring so the
// highlighted spans are unambiguous. Non-matching lines keep Chroma output.
//
// The viewport's YOffset is preserved on rerenders that happen while a
// search is active (n/N navigation) so focusSearchCursor can then jump
// the viewport to the right line. A fresh result lands here via the
// executeMsg path, which calls clearSearch → refreshJSON with an empty
// query, so the GotoTop() we used to do stays accessible via that branch.
func (m *Model) refreshJSON() {
	if m.searchQuery == "" {
		m.jsonView.SetContent(highlightJSON(m.jsonText))
		m.jsonView.GotoTop()
		return
	}

	offset := m.jsonView.YOffset
	// Group hits by line so highlightJSONLine only has to consider each
	// line once. Sorted iteration not required because we index by line.
	byLine := make(map[int]bool, len(m.jsonHits))
	for _, h := range m.jsonHits {
		byLine[h.line] = true
	}
	curHit := m.currentJSONHit()

	rawLines := strings.Split(m.jsonRaw, "\n")
	coloredLines := strings.Split(highlightJSON(m.jsonText), "\n")
	// Defensive: Chroma should produce one output line per input line,
	// but if counts drift we fall back to plain text to avoid an index
	// panic.
	out := make([]string, len(rawLines))
	for i, raw := range rawLines {
		if byLine[i] {
			out[i] = highlightJSONLine(raw, m.searchQuery, curHit, i)
			continue
		}
		if i < len(coloredLines) {
			out[i] = coloredLines[i]
		} else {
			out[i] = raw
		}
	}
	m.jsonView.SetContent(strings.Join(out, "\n"))
	m.jsonView.SetYOffset(offset)
}

// View renders the full TUI: status bar, editor, results pane, status line,
// and help bar. Picker / overlay states (history, model picker, ask input)
// take over the editor + results region while open.
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

	if m.modelPickerOpen {
		return strings.Join([]string{
			status,
			m.modelList.View(),
			helpBarStyle.Render(helpLine),
		}, "\n")
	}

	if m.languagePickerOpen {
		return strings.Join([]string{
			status,
			m.languageList.View(),
			helpBarStyle.Render(helpLine),
		}, "\n")
	}

	if m.profilePickerOpen {
		return strings.Join([]string{
			status,
			m.profileList.View(),
			helpBarStyle.Render(helpLine),
		}, "\n")
	}

	if m.clusterPickerOpen {
		return strings.Join([]string{
			status,
			m.clusterList.View(),
			helpBarStyle.Render(helpLine),
		}, "\n")
	}

	if m.secretPickerOpen {
		return strings.Join([]string{
			status,
			m.secretList.View(),
			helpBarStyle.Render(helpLine),
		}, "\n")
	}

	if m.databasePickerOpen {
		return strings.Join([]string{
			status,
			m.databaseList.View(),
			helpBarStyle.Render(helpLine),
		}, "\n")
	}

	if m.productionPromptOpen {
		return strings.Join([]string{
			status,
			m.productionList.View(),
			helpBarStyle.Render(helpLine),
		}, "\n")
	}

	if m.askOpen {
		// While the natural-language input is open the global keymap is
		// inert (only Enter/Esc are routed). Replace the help bar with a
		// minimal hint so users aren't tempted to press shortcuts that
		// would silently no-op.
		askBox := editorBoxFocused.Render(m.askInput.View())
		var hint string
		switch m.askKind {
		case askKindReview:
			hint = "enter to review (empty = general) · alt+enter / ctrl+j for newline · esc to cancel"
		case askKindAnalyze:
			hint = "enter to analyze (empty = overview) · alt+enter / ctrl+j for newline · esc to cancel"
		default:
			hint = "enter to ask · alt+enter / ctrl+j for newline · esc to cancel"
		}
		askHelp := helpBarStyle.Render(hint)
		return strings.Join([]string{
			status,
			askBox,
			m.renderResults(),
			askHelp,
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
	// Pick the accent style for the key labels so production mode
	// visibly repaints every field name in red.
	keyStyle := statusKeyStyle
	if m.isProduction {
		keyStyle = productionStatusKeyStyle
	}

	profileLabel := nonEmpty(m.target.profile)
	if m.target.profile == "" {
		profileLabel = helpStyle.Render("(direct credentials · ephemeral)")
	}
	line1 := fmt.Sprintf(
		"%s %s   %s %s",
		keyStyle.Render("profile"),
		profileLabel,
		keyStyle.Render("region"),
		nonEmpty(m.target.region),
	)
	line2 := fmt.Sprintf(
		"%s %s   %s %s",
		keyStyle.Render("cluster"),
		nonEmpty(shortARN(m.target.cluster)),
		keyStyle.Render("db"),
		nonEmpty(m.target.database),
	)
	line3 := fmt.Sprintf(
		"%s %s",
		keyStyle.Render("secret"),
		nonEmpty(shortARN(m.target.secret)),
	)
	line4 := fmt.Sprintf(
		"%s %s",
		keyStyle.Render("model"),
		m.formatModelStatus(),
	)
	body := statusStyle.Render(line1 + "\n" + line2 + "\n" + line3 + "\n" + line4)

	if m.isProduction {
		// The banner stretches across the full inner width so it can
		// never be confused with the regular status bar on narrow
		// terminals. lipgloss .Width() sets a minimum rendered width;
		// the inner padding already centres the text around it.
		bannerText := "⚠  PRODUCTION  ⚠  — queries will run against a production database"
		banner := productionBannerStyle.Width(m.width).Render(bannerText)
		return banner + "\n" + body
	}
	return body
}

// formatModelStatus returns the human-friendly summary of the active Bedrock
// model and response language. The shortened ID stays compact; an unset
// model gets a hint pointing to the switch shortcut so users always know
// how to pick one.
func (m Model) formatModelStatus() string {
	if m.bedrockModel == "" {
		return helpStyle.Render("(^O to select)")
	}
	out := shortenModelID(m.bedrockModel)
	if m.bedrockLanguage != "" {
		out += helpStyle.Render(" · " + m.bedrockLanguage)
	}
	return out
}

func (m Model) renderEditor() string {
	if m.focus == focusEditor {
		if m.isProduction {
			return productionBoxFocused.Render(m.editor.View())
		}
		return editorBoxFocused.Render(m.editor.View())
	}
	return editorBoxStyle.Render(m.editor.View())
}

func (m Model) renderResults() string {
	style := resultBoxStyle
	if m.focus == focusResults {
		if m.isProduction {
			style = productionBoxFocused
		} else {
			style = resultBoxFocused
		}
	}
	var body string
	switch {
	case m.executing:
		body = fmt.Sprintf("%s running...", m.spin.View())
	case m.explainExecuting:
		label := m.aiBusyLabel
		if label == "" {
			label = "asking AI..."
		}
		body = fmt.Sprintf("%s %s", m.spin.View(), label)
	case m.explainOpen:
		body = jsonStyle.Render(m.explainVP.View())
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
	if m.searchOpen {
		// The input prompt replaces the normal footer while the user is
		// typing a query. textinput renders its own "/" prefix (set in
		// newModel) and a blinking caret.
		hint := helpStyle.Render(" · enter apply · esc cancel")
		return m.searchInput.View() + hint
	}
	if m.askExecuting {
		return helpStyle.Render(fmt.Sprintf("%s generating SQL with AI...", m.spin.View()))
	}
	// explainExecuting is intentionally not handled here — the results
	// pane already shows the spinner + "analyzing error..." text in its
	// larger canvas, and duplicating it in the footer just produces two
	// side-by-side copies of the same message.
	if m.executing {
		return helpStyle.Render("running...")
	}
	if m.explainOpen {
		// Visual confirmation of the yank takes precedence so the user
		// sees the copy actually happened before the helper hint comes
		// back on the next render tick.
		if m.flashMessage != "" {
			return successStyle.Render(m.flashMessage)
		}
		return successStyle.Render("explanation · esc close · ↑/↓ scroll · h/l/gg/G nav · yy copy")
	}
	if m.lastErr != nil {
		// The full error message is rendered in the results pane —
		// duplicating it here would just produce two identical lines.
		// Footer only shows a short hint about how to invoke explain.
		hint := "error"
		if m.canExplainError() {
			hint += " · Tab → F6 to explain"
		}
		return errorStyle.Render(hint)
	}
	if m.inspecting && m.result != nil {
		// Flash messages (e.g. "✓ row JSON yanked to clipboard") take
		// precedence over the static hint so the user actually sees the
		// confirmation when they press yy inside the inspector.
		if m.flashMessage != "" {
			return successStyle.Render(m.flashMessage)
		}
		line := m.inspectorVP.YOffset + 1
		total := m.inspectorVP.TotalLineCount()
		if total < line {
			total = line
		}
		xPct := int(m.inspectorVP.HorizontalScrollPercent() * 100)
		return successStyle.Render(fmt.Sprintf(
			"inspecting row %d/%d · line %d/%d · x %d%% · h/l/gg/G nav · yy copy · enter/esc close",
			m.inspectedRow+1, len(m.result.Rows), line, total, xPct,
		))
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
	// Cursor position indicator: row N/M · col K/L (<name>). Only shown
	// when the results pane has focus and there is something to count,
	// so the footer stays short during normal editor work.
	if m.focus == focusResults && m.mode == viewTable && len(m.result.Rows) > 0 && len(m.result.Columns) > 0 {
		rowIdx := m.table.Cursor()
		if rowIdx < 0 {
			rowIdx = 0
		}
		col := m.colCursor
		if col < 0 {
			col = 0
		}
		if col >= len(m.result.Columns) {
			col = len(m.result.Columns) - 1
		}
		parts = append(parts, fmt.Sprintf("row %d/%d", rowIdx+1, len(m.result.Rows)))
		parts = append(parts, fmt.Sprintf("col %d/%d %s", col+1, len(m.result.Columns), m.result.Columns[col]))
	}
	if m.focus == focusResults {
		parts = append(parts, "yy to copy")
	}
	if m.searchQuery != "" {
		total := m.currentHitCount()
		if total == 0 {
			parts = append(parts, fmt.Sprintf("search %q: 0 matches", m.searchQuery))
		} else {
			parts = append(parts, fmt.Sprintf("search %q: %d/%d · n/N", m.searchQuery, m.searchCursor+1, total))
		}
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

// modelItem adapts bedrock.ModelInfo to the list.Item interface so the
// model picker can render and filter inference profiles.
type modelItem struct {
	model bedrock.ModelInfo
}

func (i modelItem) FilterValue() string { return i.model.Name + " " + i.model.ID }
func (i modelItem) Title() string       { return i.model.Name }
func (i modelItem) Description() string {
	if i.model.Description != "" {
		return i.model.Description + " · " + i.model.ID
	}
	return i.model.ID
}

// profileItem adapts a profile name to the bubbles/list interface for the
// in-TUI profile switcher (Ctrl+Y).
type profileItem struct {
	name string
}

func (i profileItem) FilterValue() string { return i.name }
func (i profileItem) Title() string       { return i.name }
func (i profileItem) Description() string { return "" }

// clusterItem and secretItem adapt connection.ClusterInfo / SecretInfo to
// the bubbles/list interface so the in-TUI cluster switcher (Ctrl+T) can
// reuse the same picker UX as model / language / history pickers.
type clusterItem struct {
	cluster connection.ClusterInfo
}

func (i clusterItem) FilterValue() string {
	return i.cluster.Identifier + " " + i.cluster.Engine + " " + i.cluster.Endpoint + " " + i.cluster.ARN
}
func (i clusterItem) Title() string {
	if i.cluster.Engine != "" {
		return fmt.Sprintf("%s [%s]", i.cluster.Identifier, i.cluster.Engine)
	}
	return i.cluster.Identifier
}
func (i clusterItem) Description() string {
	if i.cluster.Endpoint != "" {
		return i.cluster.Endpoint + " · " + shortARN(i.cluster.ARN)
	}
	return shortARN(i.cluster.ARN)
}

type secretItem struct {
	secret connection.SecretInfo
}

func (i secretItem) FilterValue() string {
	return i.secret.Name + " " + i.secret.Description + " " + i.secret.ARN
}
func (i secretItem) Title() string { return i.secret.Name }
func (i secretItem) Description() string {
	if i.secret.Description != "" {
		return i.secret.Description + " · " + shortARN(i.secret.ARN)
	}
	return shortARN(i.secret.ARN)
}

// databaseItem wraps a previously-used database name so the in-TUI
// database picker (shown during cluster switch) can render the profile's
// DatabaseHistory alongside the free-form filter input. The free-form
// input comes from the list's own filter buffer — we commit it directly
// on Enter when it's non-empty so the user can type a brand-new name
// without needing an explicit "manual entry" sentinel row.
type databaseItem struct {
	name string
}

func (i databaseItem) FilterValue() string { return i.name }
func (i databaseItem) Title() string       { return i.name }
func (i databaseItem) Description() string { return "from history" }

// productionItem is a fixed 2-entry "Yes / No" list row for the
// production-flag picker. The boolean value is the one that gets
// persisted; the title and description carry the human-readable copy.
type productionItem struct {
	title       string
	description string
	value       bool
}

func (i productionItem) FilterValue() string { return i.title }
func (i productionItem) Title() string       { return i.title }
func (i productionItem) Description() string { return i.description }

// languageOption is a fixed entry in the language picker. Code is the value
// stored in state and embedded into the system prompt; Label is the human
// label shown in the picker UI.
type languageOption struct {
	Code  string
	Label string
}

// languageChoices enumerates the languages we surface in the picker. Users
// can still override via --language with any free-form value, but the
// picker keeps the common cases one keystroke away.
var languageChoices = []languageOption{
	{Code: "Japanese", Label: "日本語 (Japanese)"},
	{Code: "English", Label: "English"},
	{Code: "Chinese", Label: "中文 (Chinese)"},
	{Code: "Korean", Label: "한국어 (Korean)"},
	{Code: "Spanish", Label: "Español (Spanish)"},
	{Code: "French", Label: "Français (French)"},
}

// languageItems materializes the fixed language list as bubbles/list items.
func languageItems() []list.Item {
	items := make([]list.Item, len(languageChoices))
	for i, l := range languageChoices {
		items[i] = languageItem{option: l}
	}
	return items
}

type languageItem struct {
	option languageOption
}

func (i languageItem) FilterValue() string { return i.option.Label + " " + i.option.Code }
func (i languageItem) Title() string       { return i.option.Label }
func (i languageItem) Description() string { return i.option.Code }

// updateOverlayViewport routes a key into a viewport overlay, giving
// handleViewportNav first crack at the vim bindings (gg/G/h/l/0/$)
// before falling through to the viewport's own Update. Shared by the
// explain and inspector branches in routeKey so they stay in sync.
func (m *Model) updateOverlayViewport(vp *viewport.Model, msg tea.KeyMsg) tea.Cmd {
	if m.handleViewportNav(vp, msg) {
		return nil
	}
	var cmd tea.Cmd
	*vp, cmd = vp.Update(msg)
	return cmd
}

// handleViewportNav adds vim-style navigation to a viewport overlay (row
// inspector / explain panel / JSON view). Returns true when the key was
// consumed so the caller can skip the normal viewport.Update path.
//
// Supported keys:
//
//	gg          → jump to top (two g presses within yankWindow)
//	G           → jump to bottom
//	h / left    → horizontal scroll left
//	l / right   → horizontal scroll right
//	0 / home    → jump to leftmost column
//	$ / end     → jump to rightmost column
//
// Single g without a follow-up is a vim "operator pending" no-op; the
// model just remembers the timestamp so the next g completes the gg.
// Any other key clears the pending state.
func (m *Model) handleViewportNav(vp *viewport.Model, msg tea.KeyMsg) bool {
	switch msg.String() {
	case "g":
		if !m.lastG.IsZero() && time.Since(m.lastG) <= yankWindow {
			vp.GotoTop()
			m.lastG = time.Time{}
			return true
		}
		m.lastG = time.Now()
		return true
	case "G":
		vp.GotoBottom()
		m.lastG = time.Time{}
		return true
	case "h", "left":
		vp.ScrollLeft(viewportHorizontalStep)
		m.lastG = time.Time{}
		return true
	case "l", "right":
		vp.ScrollRight(viewportHorizontalStep)
		m.lastG = time.Time{}
		return true
	case "0", "home":
		vp.SetXOffset(0)
		m.lastG = time.Time{}
		return true
	case "$", "end":
		// viewport has no exported "max XOffset" accessor, so push a
		// huge value and let the internal clamp do its job.
		vp.SetXOffset(1 << 30)
		m.lastG = time.Time{}
		return true
	}
	m.lastG = time.Time{}
	return false
}

// viewportHorizontalStep is how many cells one h/l press moves the
// viewport. Matches the SetHorizontalStep call in newModel so manual
// scrolls and SetHorizontalStep behave consistently.
const viewportHorizontalStep = 4

// clearPendingAssist resets the askKind and the pending snapshot fields
// so the next Ctrl+G defaults to plain SQL generation. Called from every
// path that exits the assist overlay (Esc, Enter, error inside
// startReviewFocused / startAnalyzeFocused).
func (m *Model) clearPendingAssist() {
	m.askKind = askKindGenerate
	m.pendingReviewSQL = ""
	m.pendingAnalyzeSQL = ""
	m.pendingAnalyzeBlob = ""
}

// dispatchAssist is the single entry point behind F6. It picks review /
// analyze / explain based on the current focus and what is on screen:
//
//  1. Results focus + error on screen        → startExplain (no input)
//  2. Results focus + result rows on screen  → ask input (focus area) → startAnalyze
//  3. Editor focus  + non-empty SQL          → ask input (focus area) → startReview
//  4. Otherwise                              → surface a helpful hint
//
// Ask AI (natural-language SQL generation) lives on Ctrl+G and is
// intentionally not part of this dispatcher.
func (m *Model) dispatchAssist() tea.Cmd {
	if m.bedrockClient == nil {
		m.lastErr = fmt.Errorf("bedrock client is not configured")
		return nil
	}
	if m.focus == focusResults {
		if m.canExplainError() {
			return m.startExplain()
		}
		if m.result != nil && len(m.result.Columns) > 0 {
			return m.openAnalyzePrompt()
		}
		m.lastErr = fmt.Errorf("nothing to analyze — run a query first or focus the editor to review SQL")
		return nil
	}
	if strings.TrimSpace(m.editor.Value()) != "" {
		return m.openReviewPrompt()
	}
	m.lastErr = fmt.Errorf("nothing to review — write a SQL statement first")
	return nil
}

// openAssistOverlay opens the askInput textarea in the given assist
// kind. Returns either a textarea.Blink cmd (overlay opened) or, when no
// Bedrock model has been chosen yet, the model picker chained via
// pendingExplain so the user lands back in this flow afterwards.
func (m *Model) openAssistOverlay(kind askKind, placeholder string) tea.Cmd {
	if m.bedrockModel == "" {
		m.pendingExplain = true
		return m.openModelPicker(false)
	}
	m.askKind = kind
	m.askInput.SetValue("")
	m.askInput.Placeholder = placeholder
	return m.focusAskInput()
}

func (m *Model) openReviewPrompt() tea.Cmd {
	sql := strings.TrimSpace(m.editor.Value())
	if sql == "" {
		m.lastErr = fmt.Errorf("editor is empty — nothing to review")
		return nil
	}
	m.pendingReviewSQL = sql
	return m.openAssistOverlay(askKindReview, "Focus area (Enter for general review)")
}

func (m *Model) openAnalyzePrompt() tea.Cmd {
	if m.result == nil || len(m.result.Columns) == 0 {
		m.lastErr = fmt.Errorf("no result to analyze — run a SELECT first")
		return nil
	}
	var sb strings.Builder
	if err := m.result.writeCSV(&sb); err != nil {
		m.lastErr = fmt.Errorf("encode result for analysis: %w", err)
		return nil
	}
	m.pendingAnalyzeBlob = truncateForPrompt(sb.String(), analyzePromptLimit)
	m.pendingAnalyzeSQL = strings.TrimSpace(m.lastSQL)
	if m.pendingAnalyzeSQL == "" {
		m.pendingAnalyzeSQL = strings.TrimSpace(m.editor.Value())
	}
	return m.openAssistOverlay(askKindAnalyze, "Focus area (Enter for general overview)")
}

// startReviewFocused dispatches a SQL review against pendingReviewSQL.
// focus may be empty for a balanced general review.
func (m *Model) startReviewFocused(focus string) tea.Cmd {
	if m.bedrockClient == nil {
		m.lastErr = fmt.Errorf("bedrock client is not configured")
		return nil
	}
	if m.bedrockModel == "" {
		m.pendingExplain = true
		return m.openModelPicker(false)
	}
	if m.pendingReviewSQL == "" {
		m.lastErr = fmt.Errorf("no SQL captured for review")
		return nil
	}
	m.explainExecuting = true
	m.explainOpen = false
	m.aiBusyLabel = "reviewing SQL..."
	m.flashMessage = ""
	systemPrompt := bedrock.BuildReviewSystemPrompt(m.target.database, m.bedrockLanguage, m.snapshot)
	cmd := reviewCmd(m.bedrockClient, m.bedrockModel, systemPrompt, m.pendingReviewSQL, focus)
	return tea.Batch(m.spin.Tick, cmd)
}

// startAnalyzeFocused dispatches a result analysis against
// pendingAnalyzeBlob. focus may be empty for a balanced overview.
func (m *Model) startAnalyzeFocused(focus string) tea.Cmd {
	if m.bedrockClient == nil {
		m.lastErr = fmt.Errorf("bedrock client is not configured")
		return nil
	}
	if m.bedrockModel == "" {
		m.pendingExplain = true
		return m.openModelPicker(false)
	}
	if m.pendingAnalyzeBlob == "" {
		m.lastErr = fmt.Errorf("no result captured for analysis")
		return nil
	}
	m.explainExecuting = true
	m.explainOpen = false
	m.aiBusyLabel = "analyzing result..."
	m.flashMessage = ""
	systemPrompt := bedrock.BuildAnalysisSystemPrompt(m.target.database, m.bedrockLanguage, m.snapshot)
	cmd := analyzeCmd(m.bedrockClient, m.bedrockModel, systemPrompt, m.pendingAnalyzeSQL, m.pendingAnalyzeBlob, focus)
	return tea.Batch(m.spin.Tick, cmd)
}

// analyzePromptLimit caps the result blob fed into the analysis prompt so
// large tables don't blow past Bedrock's context window. ~12k characters is
// roughly 3-4k tokens, well within Claude's input budget.
const analyzePromptLimit = 12000

// truncateForPrompt cuts overly long text and adds a marker so the model
// knows the input was clipped.
func truncateForPrompt(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "\n... (truncated for prompt size)"
}

// composeReviewText wraps the LLM's review with a header that shows the
// SQL being reviewed verbatim. The composed text is what yy yanks.
func composeReviewText(sql, review string) string {
	review = strings.TrimSpace(review)
	sql = strings.TrimSpace(sql)
	var b strings.Builder
	b.WriteString("## SQL under review\n\n```sql\n")
	b.WriteString(sql)
	b.WriteString("\n```\n\n## Review\n\n")
	b.WriteString(review)
	return b.String()
}

// composeAnalyzeText wraps the analysis with a heading. The result blob is
// not echoed here because it is already visible in the table / JSON view
// behind the overlay.
func composeAnalyzeText(analysis string) string {
	return "## Result analysis\n\n" + strings.TrimSpace(analysis)
}

// showAIOverlay drops the composed AI markdown into the explain viewport.
// Long lines stay intact and the user can pan with h/l (or 0/$) to read
// across; SetHorizontalStep is wired up in newModel so those keys move.
// Stores the raw text on the model so yy yank still copies the original
// markdown.
//
// Focus is forced to the results pane so the vim navigation keys work
// without a manual Tab. This matters most for SQL review, which is
// launched from the editor pane and would otherwise leave focus on the
// editor while the review markdown sits inert behind it.
func (m *Model) showAIOverlay(text string) {
	m.explainText = text
	m.explainVP.SetContent(text)
	m.explainVP.GotoTop()
	m.explainVP.SetXOffset(0)
	m.explainOpen = true
	m.pinResultsFocus()
}

// composeExplainText assembles the final markdown shown in the explain
// overlay: the verbatim database error in a fenced block, followed by the
// LLM's analysis. Both halves are kept even if the LLM happens to repeat
// the error in its prose — the goal is to guarantee the user can see the
// real error message regardless of how the model summarised it.
func composeExplainText(err error, analysis string) string {
	analysis = strings.TrimSpace(analysis)
	if err == nil {
		return analysis
	}
	errText := strings.TrimSpace(err.Error())
	if errText == "" {
		return analysis
	}
	var b strings.Builder
	b.WriteString("## Database error\n\n```\n")
	b.WriteString(errText)
	b.WriteString("\n```\n\n## Analysis\n\n")
	b.WriteString(analysis)
	return b.String()
}

// scheduleFlashClear bumps flashToken and returns a tea.Cmd that fires a
// clearFlashMsg after flashLifetime. Callers should invoke this right
// after setting m.flashMessage so the message auto-disappears, while a
// freshly-replaced message is protected by the token check.
func (m *Model) scheduleFlashClear() tea.Cmd {
	m.flashToken++
	token := m.flashToken
	return tea.Tick(flashLifetime, func(time.Time) tea.Msg {
		return clearFlashMsg{token: token}
	})
}

// copyResultContext picks the most useful payload for the current results
// view and copies it to the system clipboard. Precedence:
//
//  1. explain overlay → the raw markdown explanation
//  2. row inspector  → the JSON of the inspected row
//  3. JSON view      → the full result as a JSON array
//  4. table view     → the full result as CSV (the same format export uses)
//
// All paths surface a flashMessage on success and a lastErr on failure so
// the footer makes the outcome obvious.
func (m *Model) copyResultContext() {
	payload, label, err := m.yankPayload()
	if err != nil {
		m.lastErr = err
		m.flashMessage = ""
		return
	}
	if strings.TrimSpace(payload) == "" {
		m.lastErr = fmt.Errorf("nothing to copy")
		m.flashMessage = ""
		return
	}
	if err := clipboard.WriteAll(payload); err != nil {
		m.lastErr = fmt.Errorf("%w: %w", errClipboardCopy, err)
		m.flashMessage = ""
		return
	}
	// Keep the SQL error visible when the user yanked it — they
	// asked to copy what's on screen, not to dismiss it.
	if label != "error" {
		m.lastErr = nil
	}
	m.flashMessage = "✓ " + label + " yanked to clipboard"
}

// yankPayload returns the string the user expects to land in the clipboard
// when they press yy in the current results view, along with a short label
// for the success message.
func (m *Model) yankPayload() (string, string, error) {
	switch {
	case m.explainOpen:
		return m.explainText, "explanation", nil
	case m.inspecting:
		if m.result == nil {
			return "", "", fmt.Errorf("nothing to copy")
		}
		row, err := m.result.rowJSON(m.inspectedRow)
		if err != nil {
			return "", "", fmt.Errorf("copy row: %w", err)
		}
		return row, "row JSON", nil
	case m.result != nil && m.mode == viewJSON:
		return m.jsonText, "result JSON", nil
	case m.result != nil:
		var sb strings.Builder
		if err := m.result.writeCSV(&sb); err != nil {
			return "", "", fmt.Errorf("copy table: %w", err)
		}
		return sb.String(), "result CSV", nil
	case m.lastErr != nil:
		return m.lastErr.Error(), "error", nil
	}
	return "", "", fmt.Errorf("nothing to copy")
}

// openSearch shows the "/" prompt so the user can type a query. The previous
// committed query is pre-filled so pressing "/" + Enter repeats the last
// search (vim muscle memory). Returns the cursor-blink command from the
// textinput so the blinking caret starts immediately.
func (m *Model) openSearch() tea.Cmd {
	if m.result == nil {
		// No results to search — surface the reason in the status line
		// instead of silently swallowing the key. Schedule the
		// auto-clear so the hint doesn't stick forever.
		m.flashMessage = "nothing to search"
		return m.scheduleFlashClear()
	}
	m.searchInput.SetValue(m.searchQuery)
	m.searchInput.CursorEnd()
	m.searchOpen = true
	m.flashMessage = ""
	return m.searchInput.Focus()
}

// cancelSearch closes the prompt without applying the in-progress input.
// The previously committed query (if any) is left intact, so existing
// highlights and n/N navigation still work after an Esc.
func (m *Model) cancelSearch() {
	m.searchOpen = false
	m.searchInput.Blur()
}

// commitSearch captures the prompt value as the active query, recomputes
// the table / JSON hit sets, and jumps to the first match in the current
// view mode. Empty input clears the search entirely (behaves like Esc for
// new searches but also wipes any previous query).
//
// Ordering note: alignSearchAnchors has to run *before* refreshTable because
// refreshTable grows its column window around m.colCursor, and we need
// that anchor pointing at the matched column first. focusSearchCursor then
// runs *after* refreshTable because bubbles/table.SetRows (called inside
// refreshTable) resets the row cursor as a side effect.
func (m *Model) commitSearch() {
	q := strings.TrimSpace(m.searchInput.Value())
	m.searchOpen = false
	m.searchInput.Blur()
	if q == "" {
		m.clearSearch()
		return
	}
	m.searchQuery = q
	m.runSearch()
	m.searchCursor = 0
	m.alignSearchAnchors()
	m.refreshTable()
	m.refreshJSON()
	m.focusSearchCursor()
	total := m.currentHitCount()
	if total == 0 {
		m.flashMessage = fmt.Sprintf("no matches for %q", q)
	} else {
		m.flashMessage = ""
	}
}

// clearSearch resets every piece of search state so the views render without
// highlights. Called from commitSearch("" ), executeMsg (new results), and
// the explicit Esc-clear path.
func (m *Model) clearSearch() {
	m.searchQuery = ""
	m.tableHits = nil
	m.jsonHits = nil
	m.searchCursor = 0
	m.searchInput.SetValue("")
	m.refreshTable()
	m.refreshJSON()
}

// runSearch recomputes hits for both modes against the current result +
// jsonRaw. The computation is mode-agnostic so a later Ctrl+J toggle can
// reuse the pre-computed slice without re-scanning the data.
func (m *Model) runSearch() {
	m.tableHits = computeTableHits(m.result, m.searchQuery)
	m.jsonHits = computeJSONHits(m.jsonRaw, m.searchQuery)
}

// currentHitCount returns the number of hits relevant to the active view.
// Table mode counts tableHits; JSON mode counts jsonHits.
func (m Model) currentHitCount() int {
	if m.mode == viewJSON {
		return len(m.jsonHits)
	}
	return len(m.tableHits)
}

// nextHit advances the cursor forward (delta=+1) or backward (delta=-1)
// through the active mode's hit list, wrapping around at the boundaries
// like vim's n/N. No-ops when no hits exist.
//
// Ordering: align → refresh → focus. See commitSearch for why alignment
// must happen before the refresh and cursor focusing must happen after.
func (m *Model) nextHit(delta int) {
	total := m.currentHitCount()
	if total == 0 {
		return
	}
	m.searchCursor = ((m.searchCursor+delta)%total + total) % total
	m.alignSearchAnchors()
	m.refreshTable()
	m.refreshJSON()
	m.focusSearchCursor()
}

// currentTableHit / currentJSONHit return a pointer to the active hit for
// the respective mode, or nil when the search is inactive or the cursor
// is out of range. Used by the rendering helpers to decide which span to
// paint with the "current" colour.
func (m Model) currentTableHit() *tableHit {
	if m.searchQuery == "" || len(m.tableHits) == 0 {
		return nil
	}
	if m.searchCursor < 0 || m.searchCursor >= len(m.tableHits) {
		return nil
	}
	hit := m.tableHits[m.searchCursor]
	return &hit
}

func (m Model) currentJSONHit() *jsonHit {
	if m.searchQuery == "" || len(m.jsonHits) == 0 {
		return nil
	}
	if m.searchCursor < 0 || m.searchCursor >= len(m.jsonHits) {
		return nil
	}
	hit := m.jsonHits[m.searchCursor]
	return &hit
}

// alignSearchAnchors updates the pieces of view state that refreshTable /
// refreshJSON consult when they rebuild their content, so that a subsequent
// refresh lays out the right column window / highlights around the active
// hit. Must run *before* the refresh helpers; focusSearchCursor then sets
// the actual cursor position inside the freshly refreshed views.
//
// Concretely: refreshTable grows its column window around m.colCursor, so
// we have to move that anchor first; otherwise the matched cell can be
// completely outside the visible columns when the refresh runs.
func (m *Model) alignSearchAnchors() {
	if m.mode == viewJSON {
		return
	}
	hit := m.currentTableHit()
	if hit == nil {
		return
	}
	if m.result != nil && hit.cell >= 0 && hit.cell < len(m.result.Columns) {
		m.colCursor = hit.cell
	}
}

// focusSearchCursor lands the actual cursor on the current hit after the
// refresh helpers have rebuilt the views. Table mode uses the bubbles/table
// row cursor (which scrolls the inner viewport automatically); JSON mode
// sets the viewport YOffset so the matched line is roughly centred.
//
// Split from alignSearchAnchors because bubbles/table.SetRows — called by
// refreshTable — resets the row cursor as a side effect, so the row cursor
// must be applied *after* refresh.
func (m *Model) focusSearchCursor() {
	if m.mode == viewJSON {
		hit := m.currentJSONHit()
		if hit == nil {
			return
		}
		target := hit.line - m.jsonView.Height/2
		if target < 0 {
			target = 0
		}
		m.jsonView.SetYOffset(target)
		return
	}
	hit := m.currentTableHit()
	if hit == nil {
		return
	}
	m.table.SetCursor(hit.row)
}

// canExplainError reports whether the current error is something Bedrock can
// usefully analyse. The empty-SQL sentinel is excluded because there is
// nothing to explain, and the bedrock client must be configured. The
// check is used by the F8 explain shortcut and by the status bar hint to
// decide whether the operation is offered to the user.
func (m Model) canExplainError() bool {
	if m.lastErr == nil || m.bedrockClient == nil {
		return false
	}
	if _, ok := m.lastErr.(errEmptySQLValue); ok {
		return false
	}
	if m.explainExecuting {
		return false
	}
	return true
}

// startExplain captures the failed SQL + error message and dispatches a
// Bedrock analysis. When no model has been picked yet the model selector
// runs first; pendingExplain ensures the picker continues into the analyst
// flow on confirmation.
func (m *Model) startExplain() tea.Cmd {
	if m.bedrockClient == nil {
		m.lastErr = fmt.Errorf("bedrock client is not configured")
		return nil
	}
	if m.bedrockModel == "" {
		m.pendingExplain = true
		return m.openModelPicker(false)
	}
	if m.lastErr == nil || m.lastSQL == "" {
		return nil
	}
	systemPrompt := bedrock.BuildErrorExplanationPrompt(m.target.database, m.bedrockLanguage, m.snapshot)
	userPrompt := bedrock.BuildErrorUserPrompt(m.lastSQL, m.lastErr.Error())
	m.explainExecuting = true
	m.explainOpen = false
	m.aiBusyLabel = "analyzing error..."
	m.flashMessage = ""
	conv := []bedrock.Message{{Role: bedrock.RoleUser, Text: userPrompt}}
	return tea.Batch(m.spin.Tick, explainCmd(m.bedrockClient, m.bedrockModel, systemPrompt, conv))
}

// startAsk is the entry point for the Ctrl+G shortcut. When no model is
// cached yet it kicks off the picker and remembers to chain into the ask
// input afterwards. Otherwise it opens the ask input directly.
func (m *Model) startAsk() tea.Cmd {
	if m.bedrockClient == nil {
		m.lastErr = fmt.Errorf("bedrock client is not configured")
		return nil
	}
	if m.bedrockModel == "" {
		return m.openModelPicker(true)
	}
	return m.openAskInput()
}

// openModelPicker shows the Bedrock model selection list. When pendingAsk
// is true the picker chains into the ask input on selection; otherwise the
// user lands back in normal mode after picking. Used by both Ctrl+G (first
// time) and Ctrl+O (manual switch).
func (m *Model) openModelPicker(pendingAsk bool) tea.Cmd {
	if m.bedrockClient == nil {
		m.lastErr = fmt.Errorf("bedrock client is not configured")
		return nil
	}
	m.modelPickerOpen = true
	m.pendingAsk = pendingAsk
	m.lastErr = nil
	m.flashMessage = ""
	return loadModelsCmd(m.bedrockClient)
}

// openAskInput reveals the natural-language input overlay and gives it
// keyboard focus.
func (m *Model) openAskInput() tea.Cmd {
	m.askInput.SetValue("")
	m.askInput.Placeholder = "Ask in natural language (e.g. \"top 10 active users this week\")"
	m.askKind = askKindGenerate
	return m.focusAskInput()
}

// focusAskInput is the shared tail of openAskInput / openAssistOverlay.
// It flips the askOpen flag, focuses the textarea, clears any stale flash
// message, and returns the textarea blink cmd so the cursor animates.
func (m *Model) focusAskInput() tea.Cmd {
	m.askOpen = true
	m.flashMessage = ""
	return m.askInput.Focus()
}

// updateSearchInput handles keystrokes while the "/" search prompt is open.
// Enter commits the query, Esc cancels without changing the previously
// applied query. Every other key is forwarded to the textinput so typing,
// backspace and cursor movement work as expected.
func (m *Model) updateSearchInput(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		m.cancelSearch()
		return nil
	case "enter":
		m.commitSearch()
		// commitSearch may set a "no matches" flash that needs the
		// auto-clear tick, same as every other transient message.
		if m.flashMessage != "" {
			return m.scheduleFlashClear()
		}
		return nil
	}
	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	return cmd
}

// updateAskInput handles input while the natural-language prompt is open.
// Enter submits; Alt+Enter or Ctrl+J inserts a newline (see the
// InsertNewline rebind in newModel for why those are the two variants);
// Esc cancels. The prompt is appended to the running askChat history
// before dispatch so multi-turn context is preserved across Ctrl+G
// invocations within the session.
func (m *Model) updateAskInput(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		m.askOpen = false
		m.askInput.Blur()
		m.clearPendingAssist()
		return nil
	case "enter":
		return m.submitAskInput()
	}
	var cmd tea.Cmd
	m.askInput, cmd = m.askInput.Update(msg)
	return cmd
}

// submitAskInput finalises whatever assist mode the overlay is in and
// dispatches the appropriate Bedrock call. Empty prompts are allowed for
// review / analyze (the model produces a balanced general response) but
// rejected for the generate path because there is nothing to translate.
func (m *Model) submitAskInput() tea.Cmd {
	prompt := strings.TrimSpace(m.askInput.Value())

	// review and analyze share the same close / dispatch / clear shape;
	// the only difference is which focused-start function runs.
	var dispatch func(string) tea.Cmd
	switch m.askKind {
	case askKindReview:
		dispatch = m.startReviewFocused
	case askKindAnalyze:
		dispatch = m.startAnalyzeFocused
	}
	if dispatch != nil {
		m.askOpen = false
		m.askInput.Blur()
		cmd := dispatch(prompt)
		m.clearPendingAssist()
		return cmd
	}

	// askKindGenerate path.
	if prompt == "" {
		return nil
	}
	if m.askExecuting {
		return nil
	}
	m.askExecuting = true
	m.askOpen = false
	m.askInput.Blur()
	systemPrompt := bedrock.BuildSystemPrompt(m.target.database, m.bedrockLanguage, m.snapshot)
	m.askChat = append(m.askChat, bedrock.Message{Role: bedrock.RoleUser, Text: prompt})
	// Defensive copy so the goroutine can't observe a slice that the
	// askResultMsg handler is about to mutate.
	conv := append([]bedrock.Message(nil), m.askChat...)
	return tea.Batch(m.spin.Tick, askCmd(m.bedrockClient, m.bedrockModel, systemPrompt, conv, prompt))
}

// updateModelPicker handles input while the Bedrock model picker is open.
// Enter selects the highlighted model, persists it to state, and (if
// pendingAsk is set) chains directly into the ask input.
func (m *Model) updateModelPicker(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "enter":
		if item, ok := m.modelList.SelectedItem().(modelItem); ok {
			m.bedrockModel = item.model.ID
			if err := m.persistBedrockSettings(); err != nil {
				log.Printf("save bedrock model failed: %v", err)
			}
			m.modelPickerOpen = false
			// First-run flow: a fresh model selection without a cached
			// language gets routed through the language picker before
			// chaining into the pending follow-up.
			if m.bedrockLanguage == "" {
				return m.openLanguagePicker()
			}
			return m.continuePending(item.model.Name)
		}
	case "esc":
		m.modelPickerOpen = false
		m.pendingAsk = false
		m.pendingExplain = false
		return nil
	}
	drivePicker(&m.modelList, msg)
	return nil
}

// openProfilePicker fires off an async ListProfiles scan of the local AWS
// config / credentials files. The picker overlay is opened from
// profilesLoadedMsg so the user does not see an empty list.
func (m *Model) openProfilePicker() tea.Cmd {
	if m.profileLoading || m.profilePickerOpen {
		return nil
	}
	m.profileLoading = true
	m.lastErr = nil
	m.flashMessage = "loading profiles..."
	return loadProfilesCmd()
}

// updateProfilePicker handles input while the profile picker is open.
// Selecting a profile triggers an async LoadConfig; ESC cancels.
func (m *Model) updateProfilePicker(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "enter":
		if item, ok := m.profileList.SelectedItem().(profileItem); ok {
			m.profilePickerOpen = false
			m.profileSwitching = true
			m.flashMessage = "switching to profile " + item.name + "..."
			return switchProfileCmd(item.name)
		}
		return nil
	case "esc":
		m.profilePickerOpen = false
		m.flashMessage = ""
		return nil
	}
	drivePicker(&m.profileList, msg)
	return nil
}

// applyProfileSwitch swaps the active AWS config + clients and re-resolves
// the connection target from the new profile's state cache. If the new
// profile has cached cluster + secret + database we reuse them and refresh
// schema; if anything is missing we surface a helpful flash message and
// leave the user on the cluster picker so they can pick interactively.
func (m *Model) applyProfileSwitch(profile string, cfg aws.Config) tea.Cmd {
	m.awsConfig = cfg
	m.client = rdsdata.NewFromConfig(cfg)
	m.bedrockClient = bedrock.New(cfg)
	m.target.profile = profile
	m.target.region = cfg.Region

	// Switching from ephemeral mode to a real profile enables history;
	// the inverse (real → ephemeral) is impossible from inside the TUI
	// because the picker only lists named profiles, but we still guard
	// for completeness.
	if profile != "" && m.historyStore == nil {
		if s, err := history.New(); err != nil {
			log.Printf("history enable failed after profile switch: %v", err)
		} else {
			m.historyStore = s
		}
	}

	// Drop the previous cluster's results / snapshot so AI prompts and
	// the table view do not mix data across profiles. Also reset the
	// running Ask AI chat so a fresh session starts after every profile
	// switch — the previous schema is no longer relevant.
	m.result = nil
	m.jsonText = ""
	m.lastErr = nil
	m.snapshot = nil
	m.askChat = nil

	// Hydrate the production flag from the new profile's state and
	// re-run layout() so the banner line is accounted for in the
	// editor/results heights. If the new profile has never been
	// classified the picker opens automatically before any query can
	// run.
	answered := m.loadProductionFlag()
	m.layout()
	if !answered {
		m.openProductionPrompt()
	}

	st, err := state.Load()
	if err != nil {
		m.flashMessage = "switched profile but state load failed: " + err.Error()
		return m.openClusterPicker()
	}
	ps := st.Get(profile)

	// Restore Bedrock model + language so the new profile's preferences
	// are honored for the AI picker / status bar.
	m.bedrockModel = ps.BedrockModel
	m.bedrockLanguage = ps.BedrockLanguage

	if ps.Cluster != "" && ps.Secret != "" && ps.Database != "" {
		m.target.cluster = ps.Cluster
		m.target.secret = ps.Secret
		m.target.database = ps.Database
		m.flashMessage = "profile: " + profile + " · " + shortARN(ps.Cluster)
		return fetchSchemaCmd(m.client, m.target)
	}

	// New / incomplete profile → drop the user straight into the cluster
	// picker so they can finish the setup interactively.
	m.target.cluster = ""
	m.target.secret = ""
	m.target.database = ps.Database
	m.flashMessage = "profile: " + profile + " · pick a cluster"
	return m.openClusterPicker()
}

// openSecretPicker triggers the secret picker for the current cluster
// without consulting the per-profile cluster→secret cache. This is the
// Ctrl+\ handler — it lets the user pick a different secret for the
// already-selected cluster (e.g. read-only vs. admin credentials).
func (m *Model) openSecretPicker() tea.Cmd {
	if m.target.cluster == "" {
		m.lastErr = fmt.Errorf("select a cluster first")
		return nil
	}
	if m.secretLoading || m.secretPickerOpen {
		return nil
	}
	cluster := connection.ClusterInfo{
		ARN:        m.target.cluster,
		Identifier: shortARN(m.target.cluster),
	}
	m.pendingCluster = cluster
	m.secretLoading = true
	m.forceSecretPicker = true
	m.lastErr = nil
	m.flashMessage = "loading secrets..."
	return loadSuggestedSecretsCmd(m.awsConfig, cluster)
}

// openClusterPicker fires off an async DescribeDBClusters call. The picker
// itself is opened from clustersLoadedMsg so the user does not see an
// empty list while the API request is in flight.
func (m *Model) openClusterPicker() tea.Cmd {
	if m.clusterLoading || m.clusterPickerOpen {
		return nil
	}
	m.clusterLoading = true
	m.lastErr = nil
	m.flashMessage = "loading clusters..."
	return loadClustersCmd(m.awsConfig)
}

// updateClusterPicker handles input while the cluster picker is open.
// Selecting a cluster first checks the per-profile cluster→secret cache so
// previously-paired clusters switch instantly with no AWS round trip; only
// genuinely new clusters fall through to the AWS-side suggestion path.
func (m *Model) updateClusterPicker(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "enter":
		if item, ok := m.clusterList.SelectedItem().(clusterItem); ok {
			m.clusterPickerOpen = false
			if cached := m.lookupCachedSecret(item.cluster.ARN); cached != "" {
				m.flashMessage = "switching to " + item.cluster.Identifier + "..."
				return m.beginTargetSwitch(item.cluster, cached)
			}
			m.pendingCluster = item.cluster
			m.secretLoading = true
			m.flashMessage = "resolving secret for " + item.cluster.Identifier + "..."
			return loadSuggestedSecretsCmd(m.awsConfig, item.cluster)
		}
		return nil
	case "esc":
		m.clusterPickerOpen = false
		m.flashMessage = ""
		return nil
	}
	drivePicker(&m.clusterList, msg)
	return nil
}

// lookupCachedSecret returns the secret ARN previously paired with this
// cluster within the active profile, or "" when no record exists. The
// cluster-keyed map is the primary source; the legacy single-secret cache
// is consulted as a fallback so users on stale state files still get the
// fast-path behavior on first migration.
func (m Model) lookupCachedSecret(clusterARN string) string {
	if clusterARN == "" {
		return ""
	}
	st, err := state.Load()
	if err != nil {
		return ""
	}
	ps := st.Get(m.target.profile)
	if s, ok := ps.ClusterSecrets[clusterARN]; ok && s != "" {
		return s
	}
	if ps.Cluster == clusterARN && ps.Secret != "" {
		return ps.Secret
	}
	return ""
}

// updateSecretPicker handles input while the secret picker is open.
// Confirming a secret starts the target switch (cluster + secret) which
// chains into the database picker for final confirmation.
func (m *Model) updateSecretPicker(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "enter":
		if item, ok := m.secretList.SelectedItem().(secretItem); ok {
			m.secretPickerOpen = false
			m.forceSecretPicker = false
			cluster := m.pendingCluster
			m.pendingCluster = connection.ClusterInfo{}
			return m.beginTargetSwitch(cluster, item.secret.ARN)
		}
		return nil
	case "esc":
		m.secretPickerOpen = false
		m.forceSecretPicker = false
		m.pendingCluster = connection.ClusterInfo{}
		m.flashMessage = ""
		return nil
	}
	drivePicker(&m.secretList, msg)
	return nil
}

// openDatabasePicker loads the profile's database history into the
// picker list and marks it open. The list items are the history entries
// (most-recent first), and the picker's filter text doubles as a free
// form entry field: the Enter handler commits a typed filter verbatim
// when it is non-empty, so the user can type a brand-new database name
// without having a matching history entry.
func (m *Model) openDatabasePicker(cluster connection.ClusterInfo) tea.Cmd {
	history := m.databaseHistory()
	items := make([]list.Item, 0, len(history))
	for _, name := range history {
		if name == "" {
			continue
		}
		items = append(items, databaseItem{name: name})
	}
	m.databaseList.SetItems(items)
	m.databaseList.ResetFilter()
	label := cluster.Identifier
	if label == "" {
		label = shortARN(cluster.ARN)
	}
	m.databaseList.Title = fmt.Sprintf(
		"Database for %s (type new name or pick from history, Enter to confirm, Esc to cancel)",
		label,
	)
	m.databasePickerOpen = true
	m.flashMessage = ""
	return nil
}

// databaseHistory pulls the database history slice off the profile state
// file. Falls back to an empty slice on any error (missing file,
// ephemeral mode with no profile name) so the picker still opens cleanly
// and the user can type a fresh name.
func (m Model) databaseHistory() []string {
	if m.target.profile == "" {
		return nil
	}
	st, err := state.Load()
	if err != nil {
		return nil
	}
	return st.Get(m.target.profile).DatabaseHistory
}

// openProductionPrompt shows the Yes/No picker that decides whether the
// active profile is a production environment. Called automatically when
// the state file has no stored value (first activation of a profile),
// and on demand via the F7 keybinding so the user can toggle the flag
// later. The initially-highlighted row reflects the current flag so a
// simple Enter press preserves the existing setting on F7 reopens.
func (m *Model) openProductionPrompt() {
	if m.productionList.SelectedItem() != nil {
		// Start the cursor on the row matching the current flag so F7
		// reopens land on the user's previous answer. bubbles/list's
		// Select(n int) sets the cursor without other side effects.
		if m.isProduction {
			m.productionList.Select(0)
		} else {
			m.productionList.Select(1)
		}
	}
	m.productionPromptOpen = true
	m.flashMessage = ""
}

// updateProductionPrompt handles input while the production prompt is
// open. Enter persists the highlighted value to state and applies it
// live; Esc closes the picker without persisting — which means the
// next activation of this profile will re-ask, so the user cannot
// accidentally bypass the confirmation on a fresh profile.
//
// Typed runes are intentionally not forwarded to the list filter: the
// picker is non-filterable (2 fixed rows), so we handle up/down
// directly and drop everything else instead of letting drivePicker
// push junk into the filter buffer.
func (m *Model) updateProductionPrompt(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "enter":
		if item, ok := m.productionList.SelectedItem().(productionItem); ok {
			m.isProduction = item.value
			m.productionPromptOpen = false
			// Toggling the flag changes the status bar height, so the
			// editor / results heights need to be recomputed before
			// the next render — without this the bottom line clips
			// until the next terminal resize.
			m.layout()
			if err := m.persistIsProduction(item.value); err != nil {
				log.Printf("save is_production failed: %v", err)
			}
			if item.value {
				m.flashMessage = "⚠ PRODUCTION mode enabled"
			} else {
				m.flashMessage = "non-production mode"
			}
			return m.scheduleFlashClear()
		}
		return nil
	case "esc":
		m.productionPromptOpen = false
		m.flashMessage = ""
		return nil
	}
	switch msg.Type {
	case tea.KeyUp:
		m.productionList.CursorUp()
	case tea.KeyDown:
		m.productionList.CursorDown()
	}
	return nil
}

// persistIsProduction writes the IsProduction flag to the profile state
// file. Ephemeral mode (empty profile name) is a no-op because direct
// credentials sessions do not touch state at all.
func (m *Model) persistIsProduction(value bool) error {
	if m.target.profile == "" {
		return nil
	}
	st, err := state.Load()
	if err != nil {
		return err
	}
	ps := st.Get(m.target.profile)
	v := value
	ps.IsProduction = &v
	st.Set(m.target.profile, ps)
	return st.Save()
}

// loadProductionFlag re-reads the IsProduction flag from the state file
// and syncs it into the Model. Returns true if the flag had a stored
// value; callers use that to decide whether to auto-open the prompt
// (nil → prompt, non-nil → keep in sync with state and move on).
func (m *Model) loadProductionFlag() bool {
	m.isProduction = false
	if m.target.profile == "" {
		return true // ephemeral mode: treat as "already answered (false)"
	}
	st, err := state.Load()
	if err != nil {
		return false
	}
	ps := st.Get(m.target.profile)
	if ps.IsProduction == nil {
		return false
	}
	m.isProduction = *ps.IsProduction
	return true
}

// updateDatabasePicker handles input while the database picker is open.
// On Enter the typed filter text wins if it is non-empty (so "type a new
// name, Enter" always works even when that name is not in the history);
// otherwise the highlighted history item is committed. Esc reverts the
// whole cluster switch by restoring the pre-switch cluster / secret, so
// the user lands back exactly where they started before they pressed
// Ctrl+T.
func (m *Model) updateDatabasePicker(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "enter":
		name := strings.TrimSpace(m.databaseList.FilterValue())
		if name == "" {
			// No typed filter — fall back to the highlighted history
			// row (if any). If nothing is selected we cannot commit.
			if item, ok := m.databaseList.SelectedItem().(databaseItem); ok {
				name = item.name
			}
		}
		if name == "" {
			m.lastErr = fmt.Errorf("database name cannot be empty")
			return nil
		}
		m.databasePickerOpen = false
		return m.finalizeTargetSwitch(name)
	case "esc":
		m.target.cluster = m.pendingOldCluster
		m.target.secret = m.pendingOldSecret
		m.pendingOldCluster = ""
		m.pendingOldSecret = ""
		m.databasePickerOpen = false
		m.flashMessage = "cluster switch cancelled"
		return nil
	}
	drivePicker(&m.databaseList, msg)
	return nil
}

// beginTargetSwitch is the first half of the cluster-switch flow. It
// swaps cluster + secret onto m.target and then opens the database picker
// so the user is always forced to confirm (or re-type) the target DB name
// before any query runs against the new environment. The stale result /
// schema clearing and the schema fetch are deferred to finalizeTargetSwitch
// — the user may still Esc out of the database picker, in which case
// nothing about the session changes.
//
// pendingOldCluster / pendingOldSecret snapshot the pre-switch values so
// the Esc path in updateDatabasePicker can revert the target cleanly.
func (m *Model) beginTargetSwitch(cluster connection.ClusterInfo, secretARN string) tea.Cmd {
	m.pendingOldCluster = m.target.cluster
	m.pendingOldSecret = m.target.secret
	m.target.cluster = cluster.ARN
	m.target.secret = secretARN
	m.lastErr = nil
	m.flashMessage = ""
	return m.openDatabasePicker(cluster)
}

// finalizeTargetSwitch is the second half of the cluster-switch flow,
// triggered once the user confirms a database name in the picker. It
// drops stale results / schema / ask-chat for the previous cluster,
// persists the new target (cluster + secret + database) to the profile
// state file, and kicks off a fresh schema fetch so the AI prompts have
// up-to-date table / column context for the new database.
func (m *Model) finalizeTargetSwitch(database string) tea.Cmd {
	m.target.database = database
	m.pendingOldCluster = ""
	m.pendingOldSecret = ""
	// Drop stale state from the previous cluster now that the switch
	// is committed. Ask AI chat is also reset because the new cluster's
	// schema is unrelated to the previous conversation.
	m.result = nil
	m.jsonText = ""
	m.jsonRaw = ""
	m.lastErr = nil
	m.snapshot = nil
	m.askChat = nil
	m.clearSearch()
	m.flashMessage = "switched to " + shortARN(m.target.cluster) + " · " + database
	if err := m.persistTarget(); err != nil {
		log.Printf("save cluster/secret/database failed: %v", err)
	}
	return fetchSchemaCmd(m.client, m.target)
}

// persistTarget writes the active cluster + secret back to the per-profile
// state file so the next rdq invocation reuses them automatically. The
// cluster→secret pairing is also recorded in ClusterSecrets so future
// switches between known clusters skip the picker entirely.
//
// Ephemeral mode (empty profile name) skips persistence entirely so direct
// credentials sessions leave nothing on disk.
func (m *Model) persistTarget() error {
	if m.target.profile == "" {
		return nil
	}
	st, err := state.Load()
	if err != nil {
		return err
	}
	ps := st.Get(m.target.profile)
	ps.Cluster = m.target.cluster
	ps.Secret = m.target.secret
	ps.Database = m.target.database
	if ps.ClusterSecrets == nil {
		ps.ClusterSecrets = map[string]string{}
	}
	if m.target.cluster != "" && m.target.secret != "" {
		ps.ClusterSecrets[m.target.cluster] = m.target.secret
	}
	st.Set(m.target.profile, ps)
	return st.Save()
}

// continuePending resumes whatever flow was queued behind the model /
// language pickers (Ctrl+G ask input or auto-explain). When neither is
// pending it just surfaces a flash message confirming the new selection.
func (m *Model) continuePending(modelName string) tea.Cmd {
	if m.pendingAsk {
		m.pendingAsk = false
		return m.openAskInput()
	}
	if m.pendingExplain {
		m.pendingExplain = false
		return m.startExplain()
	}
	if modelName != "" {
		m.flashMessage = "bedrock model: " + modelName
	} else {
		m.flashMessage = "bedrock language: " + m.bedrockLanguage
	}
	return nil
}

// openLanguagePicker shows the response-language picker. Filtering is
// driven directly via SetFilterText inside drivePicker, so we just clear
// any leftover filter and let the user start typing.
func (m *Model) openLanguagePicker() tea.Cmd {
	m.languagePickerOpen = true
	m.flashMessage = ""
	m.languageList.ResetFilter()
	return nil
}

// updateLanguagePicker handles input while the language picker is open.
// Selection persists the picked language and chains into the queued ask /
// explain flow if any.
func (m *Model) updateLanguagePicker(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "enter":
		if item, ok := m.languageList.SelectedItem().(languageItem); ok {
			m.bedrockLanguage = item.option.Code
			if err := m.persistBedrockSettings(); err != nil {
				log.Printf("save bedrock language failed: %v", err)
			}
			m.languagePickerOpen = false
			return m.continuePending("")
		}
	case "esc":
		m.languagePickerOpen = false
		m.pendingAsk = false
		m.pendingExplain = false
		return nil
	}
	drivePicker(&m.languageList, msg)
	return nil
}

// persistBedrockSettings saves the current model + language pair to the
// per-profile state file so subsequent runs skip both pickers. Both fields
// are written even when only one changed; ProfileState is small enough that
// the extra write is irrelevant.
func (m *Model) persistBedrockSettings() error {
	if m.target.profile == "" {
		return nil
	}
	st, err := state.Load()
	if err != nil {
		return err
	}
	ps := st.Get(m.target.profile)
	ps.BedrockModel = m.bedrockModel
	ps.BedrockLanguage = m.bedrockLanguage
	st.Set(m.target.profile, ps)
	return st.Save()
}

// applyAskResult replaces the editor's contents with the AI-generated SQL,
// prefixed by a comment recording the prompt. Previous editor content is
// discarded so the user can iterate on prompts without the editor growing
// every time — the SQL history picker (Ctrl+H) is the recovery path if a
// previously executed draft needs to come back.
func (m *Model) applyAskResult(prompt, sql string) {
	var b strings.Builder
	fmt.Fprintf(&b, "-- ask: %s\n", strings.ReplaceAll(prompt, "\n", " "))
	b.WriteString(strings.TrimSpace(sql))
	b.WriteString("\n")
	m.editor.SetValue(b.String())
	m.editor.CursorEnd()
	m.focus = focusEditor
	m.editor.Focus()
	m.table.Blur()
	m.flashMessage = "AI generated SQL replaced editor (review then F5)"
}
