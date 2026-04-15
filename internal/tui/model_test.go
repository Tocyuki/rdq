package tui

import (
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Tocyuki/rdq/internal/bedrock"
	"github.com/Tocyuki/rdq/internal/connection"
	"github.com/Tocyuki/rdq/internal/state"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// makeResult builds a synthetic queryResult with the given column and row
// counts so refreshTable's table-update path can be exercised without an
// actual RDS Data API round trip.
func makeResult(cols, rows int) *queryResult {
	r := &queryResult{
		Columns: make([]string, cols),
		Rows:    make([][]any, rows),
	}
	for c := 0; c < cols; c++ {
		r.Columns[c] = "col" + strconv.Itoa(c)
	}
	for i := 0; i < rows; i++ {
		row := make([]any, cols)
		for c := 0; c < cols; c++ {
			row[c] = "v" + strconv.Itoa(i) + "_" + strconv.Itoa(c)
		}
		r.Rows[i] = row
	}
	return r
}

// TestRefreshTableHandlesShrinkingColumnCount guards against the original
// panic where bubbles/table.SetColumns synchronously rendered stale rows
// against the new (narrower) column slice and indexed out of range.
func TestRefreshTableHandlesShrinkingColumnCount(t *testing.T) {
	m := newModel(nil, target{}, nil, nil, "", "", aws.Config{})

	m.result = makeResult(10, 5)
	m.refreshTable()

	m.result = makeResult(9, 3)
	m.refreshTable()

	m.result = makeResult(1, 0)
	m.refreshTable()

	m.result = nil
	m.refreshTable()
}

func TestRefreshTableHandlesGrowingColumnCount(t *testing.T) {
	m := newModel(nil, target{}, nil, nil, "", "", aws.Config{})

	m.result = makeResult(2, 4)
	m.refreshTable()

	m.result = makeResult(15, 2)
	m.refreshTable()
}

// TestModelPickerIncrementalSearch is the regression for the bug where
// typing into the model picker did not narrow the visible items. The fix
// drives bubbles/list synchronously via SetFilterText inside drivePicker,
// so this test feeds keystrokes through Update and asserts the visible
// slice collapses to the matching item without any async pump.
func TestModelPickerIncrementalSearch(t *testing.T) {
	m := newModel(nil, target{}, nil, nil, "", "", aws.Config{})
	m.width, m.height = 100, 40
	m.layout()

	tm, _ := m.Update(modelsLoadedMsg{models: []bedrock.ModelInfo{
		{ID: "claude-1", Name: "Claude Sonnet"},
		{ID: "claude-2", Name: "Claude Haiku"},
		{ID: "llama-1", Name: "Llama 3"},
	}})
	m = tm.(Model)
	m.modelPickerOpen = true

	send := func(model Model, r rune) Model {
		next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		return next.(Model)
	}

	for _, r := range "lla" {
		m = send(m, r)
	}

	visible := m.modelList.VisibleItems()
	if len(visible) != 1 {
		t.Fatalf("expected 1 visible item after typing 'lla', got %d: %+v", len(visible), visible)
	}
	if got := visible[0].(modelItem).model.ID; got != "llama-1" {
		t.Errorf("expected llama-1, got %s", got)
	}

	// Backspace twice: "lla" → "ll" → "l". "l" is a substring of all
	// three labels (Claude, Claude, Llama), so all three become visible.
	for i := 0; i < 2; i++ {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		m = next.(Model)
	}
	if got := m.modelList.FilterValue(); got != "l" {
		t.Errorf("after two backspaces expected filter 'l', got %q", got)
	}
	visible = m.modelList.VisibleItems()
	if len(visible) != 3 {
		t.Errorf("after backspaces (filter 'l') expected 3 visible, got %d", len(visible))
	}
}

// TestModelPickerSpaceInFilter ensures that pressing the space bar in the
// model picker is appended to the filter text. bubbletea delivers space as
// tea.KeySpace, not tea.KeyRunes, so without an explicit case the filter
// silently drops it and "Claude Sonnet" is impossible to type.
func TestModelPickerSpaceInFilter(t *testing.T) {
	m := newModel(nil, target{}, nil, nil, "", "", aws.Config{})
	m.width, m.height = 100, 40
	m.layout()

	tm, _ := m.Update(modelsLoadedMsg{models: []bedrock.ModelInfo{
		{ID: "claude-1", Name: "Claude Sonnet"},
		{ID: "claude-2", Name: "Claude Haiku"},
		{ID: "llama-1", Name: "Llama 3"},
	}})
	m = tm.(Model)
	m.modelPickerOpen = true

	send := func(model Model, msg tea.KeyMsg) Model {
		next, _ := model.Update(msg)
		return next.(Model)
	}

	// Type "claude" then a space then "s" → filter "claude s" should
	// uniquely match "Claude Sonnet".
	for _, r := range "claude" {
		m = send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m = send(m, tea.KeyMsg{Type: tea.KeySpace})
	m = send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})

	if got := m.modelList.FilterValue(); got != "claude s" {
		t.Fatalf("expected filter %q, got %q", "claude s", got)
	}
	visible := m.modelList.VisibleItems()
	if len(visible) != 1 {
		t.Fatalf("expected 1 visible item after typing 'claude s', got %d: %+v", len(visible), visible)
	}
	if got := visible[0].(modelItem).model.ID; got != "claude-1" {
		t.Errorf("expected claude-1, got %s", got)
	}
}

// TestModelPickerCursorAndSelect verifies that after narrowing the picker
// the user can move the cursor with KeyDown and confirm the highlighted
// row with Enter. This is the regression for "selection doesn't work after
// filtering" — the test must end with the picker closed and the second
// filtered item recorded as the active Bedrock model.
func TestModelPickerCursorAndSelect(t *testing.T) {
	m := newModel(nil, target{}, nil, nil, "", "", aws.Config{})
	m.width, m.height = 100, 40
	m.layout()

	tm, _ := m.Update(modelsLoadedMsg{models: []bedrock.ModelInfo{
		{ID: "claude-1", Name: "Claude Sonnet"},
		{ID: "claude-2", Name: "Claude Haiku"},
		{ID: "llama-1", Name: "Llama 3"},
	}})
	m = tm.(Model)
	m.modelPickerOpen = true

	send := func(model Model, msg tea.KeyMsg) Model {
		next, _ := model.Update(msg)
		return next.(Model)
	}

	for _, r := range "claude" {
		m = send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if got := len(m.modelList.VisibleItems()); got != 2 {
		t.Fatalf("expected 2 visible after filtering on claude, got %d", got)
	}

	m = send(m, tea.KeyMsg{Type: tea.KeyDown})
	if idx := m.modelList.Index(); idx != 1 {
		t.Fatalf("expected cursor at 1 after KeyDown, got %d", idx)
	}

	m = send(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.modelPickerOpen {
		t.Errorf("expected picker to close after Enter")
	}
	if m.bedrockModel != "claude-2" {
		t.Errorf("expected claude-2 to be selected, got %q", m.bedrockModel)
	}
}

// TestClusterPickerCursorAndSelect mirrors TestModelPickerCursorAndSelect
// for the in-TUI cluster switcher (Ctrl+T). Filters the list to two items
// then walks the cursor with KeyDown to make sure secondary entries are
// reachable. This is the regression for "cluster picker: filter narrows
// items but arrows do not move the cursor".
func TestClusterPickerCursorAndSelect(t *testing.T) {
	m := newModel(nil, target{}, nil, nil, "", "", aws.Config{})
	m.width, m.height = 100, 40
	m.layout()

	tm, _ := m.Update(clustersLoadedMsg{clusters: []connection.ClusterInfo{
		{ARN: "arn:aws:rds:::cluster:studypocket-dev-a", Identifier: "studypocket-dev-a", Engine: "aurora-mysql"},
		{ARN: "arn:aws:rds:::cluster:studypocket-dev-b", Identifier: "studypocket-dev-b", Engine: "aurora-postgresql"},
		{ARN: "arn:aws:rds:::cluster:other-cluster", Identifier: "other-cluster", Engine: "aurora-mysql"},
	}})
	m = tm.(Model)
	if !m.clusterPickerOpen {
		t.Fatal("expected cluster picker to open after clustersLoadedMsg")
	}

	send := func(model Model, msg tea.KeyMsg) Model {
		next, _ := model.Update(msg)
		return next.(Model)
	}

	for _, r := range "studypocket" {
		m = send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if got := len(m.clusterList.VisibleItems()); got != 2 {
		t.Fatalf("expected 2 visible after filter, got %d", got)
	}
	// FilterApplied is mandatory: under Filtering the DefaultDelegate
	// suppresses the selection highlight so the cursor moves invisibly.
	if got := m.clusterList.FilterState(); got != list.FilterApplied {
		t.Errorf("expected FilterApplied to keep cursor highlight visible, got %v", got)
	}

	m = send(m, tea.KeyMsg{Type: tea.KeyDown})
	if idx := m.clusterList.Index(); idx != 1 {
		t.Errorf("expected cursor 1 after KeyDown, got %d", idx)
	}
}

// TestSecretPickerNormalAutoSelectsSingleMatch is the existing
// non-regression: when the cluster picker resolved a single secret it
// should auto-apply without showing the secret picker.
func TestSecretPickerNormalAutoSelectsSingleMatch(t *testing.T) {
	m := newModel(nil, target{}, nil, nil, "", "", aws.Config{})
	m.width, m.height = 100, 40
	m.layout()
	m.target.cluster = "arn:cluster"
	m.pendingCluster = connection.ClusterInfo{ARN: "arn:cluster", Identifier: "c"}

	tm, _ := m.Update(secretsLoadedMsg{
		cluster: connection.ClusterInfo{ARN: "arn:cluster", Identifier: "c"},
		secrets: []connection.SecretInfo{{ARN: "arn:s1", Name: "only"}},
	})
	m = tm.(Model)

	if m.secretPickerOpen {
		t.Errorf("secret picker should not open for a single auto-resolved secret")
	}
	if m.target.secret != "arn:s1" {
		t.Errorf("expected single secret to be auto-applied, got %q", m.target.secret)
	}
}

// TestSecretPickerForceShowsEvenWithSingleMatch covers Ctrl+\: even if
// only one secret matches, the picker must still appear because the user
// explicitly asked to switch.
func TestSecretPickerForceShowsEvenWithSingleMatch(t *testing.T) {
	m := newModel(nil, target{}, nil, nil, "", "", aws.Config{})
	m.width, m.height = 100, 40
	m.layout()
	m.target.cluster = "arn:cluster"
	m.pendingCluster = connection.ClusterInfo{ARN: "arn:cluster", Identifier: "c"}
	m.forceSecretPicker = true

	tm, _ := m.Update(secretsLoadedMsg{
		cluster: connection.ClusterInfo{ARN: "arn:cluster", Identifier: "c"},
		secrets: []connection.SecretInfo{{ARN: "arn:s1", Name: "only"}},
	})
	m = tm.(Model)

	if !m.secretPickerOpen {
		t.Errorf("expected secret picker to open in forced mode even with a single match")
	}
	if m.target.secret == "arn:s1" {
		t.Errorf("forced flow must not auto-apply the single match")
	}
	if m.forceSecretPicker {
		t.Errorf("forceSecretPicker should be reset after handling secretsLoadedMsg")
	}
}

// TestSecretPickerCursorAndSelect is the same regression for the secret
// picker overlay reached after a cluster has been picked.
func TestSecretPickerCursorAndSelect(t *testing.T) {
	m := newModel(nil, target{}, nil, nil, "", "", aws.Config{})
	m.width, m.height = 100, 40
	m.layout()

	tm, _ := m.Update(secretsLoadedMsg{
		cluster: connection.ClusterInfo{ARN: "arn:cluster", Identifier: "c"},
		secrets: []connection.SecretInfo{
			{ARN: "arn:s1", Name: "studypocket-dev-secret-1"},
			{ARN: "arn:s2", Name: "studypocket-dev-secret-2"},
		},
		fallback: true,
	})
	m = tm.(Model)
	if !m.secretPickerOpen {
		t.Fatal("expected secret picker to open after secretsLoadedMsg")
	}

	send := func(model Model, msg tea.KeyMsg) Model {
		next, _ := model.Update(msg)
		return next.(Model)
	}

	for _, r := range "studypocket" {
		m = send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if got := len(m.secretList.VisibleItems()); got != 2 {
		t.Fatalf("expected 2 visible after filter, got %d", got)
	}
	if got := m.secretList.FilterState(); got != list.FilterApplied {
		t.Errorf("expected FilterApplied to keep cursor highlight visible, got %v", got)
	}

	m = send(m, tea.KeyMsg{Type: tea.KeyDown})
	if idx := m.secretList.Index(); idx != 1 {
		t.Errorf("expected cursor 1 after KeyDown, got %d", idx)
	}
}

// TestYankInspectorViaKeystrokes drives the model through the same key
// sequence a user would: open the inspector with Enter, then press y twice
// quickly. The clipboard library is no-op in CI but the flashMessage path
// proves the yank reached copyResultContext successfully.
func TestYankInspectorViaKeystrokes(t *testing.T) {
	m := newModel(nil, target{}, nil, nil, "", "", aws.Config{})
	m.width, m.height = 100, 40
	m.layout()
	m.result = makeResult(2, 3)
	m.jsonText = m.result.toJSON()
	m.refreshTable()
	m.focus = focusResults
	m.table.Focus()

	send := func(model Model, msg tea.KeyMsg) Model {
		next, _ := model.Update(msg)
		return next.(Model)
	}

	// Enter opens the inspector for row 0
	m = send(m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.inspecting {
		t.Fatalf("expected inspecting after Enter (lastErr=%v)", m.lastErr)
	}
	if m.focus != focusResults {
		t.Errorf("expected focus to stay on results inside inspector")
	}

	// yy must hit the inspecting branch and call rowJSON without
	// complaint. clipboard.WriteAll may legitimately fail in headless
	// CI (no xclip/xsel on ubuntu-latest); copyResultContext wraps those
	// failures with errClipboardCopy, so we accept matches via errors.Is
	// but still reject any other lastErr (e.g. a rowJSON failure) to
	// keep the inspector branch under test.
	m = send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if m.lastErr != nil && !errors.Is(m.lastErr, errClipboardCopy) {
		t.Errorf("unexpected error after yy in inspector: %v", m.lastErr)
	}
	// Either flashMessage (clipboard OK) or lastErr (copy failed) must
	// fire — if neither does, the yy branch wasn't reached at all.
	if m.flashMessage == "" && m.lastErr == nil {
		t.Errorf("expected yy to either flash or surface an error, got neither")
	}
}

// TestAskChatAccumulatesHistory drives askResultMsg directly to verify
// that a successful response appends both the user prompt and the
// assistant reply to askChat, so the next Ctrl+G round trip carries the
// previous turn's context.
func TestAskChatAccumulatesHistory(t *testing.T) {
	m := newModel(nil, target{}, nil, nil, "", "", aws.Config{})
	m.width, m.height = 100, 40
	m.layout()

	// Simulate the user typing into the ask input and pressing Enter.
	// updateAskInput appends the user message before dispatching, so we
	// mirror that here without invoking the network round trip.
	m.askChat = append(m.askChat, bedrock.Message{Role: bedrock.RoleUser, Text: "list active users"})

	tm, _ := m.Update(askResultMsg{prompt: "list active users", sql: "SELECT * FROM users WHERE active = true;"})
	m = tm.(Model)

	if got := len(m.askChat); got != 2 {
		t.Fatalf("expected 2 messages after first turn, got %d: %+v", got, m.askChat)
	}
	if m.askChat[0].Role != bedrock.RoleUser || m.askChat[1].Role != bedrock.RoleAssistant {
		t.Errorf("expected user→assistant order, got %v", m.askChat)
	}
	if m.askChat[1].Text != "SELECT * FROM users WHERE active = true;" {
		t.Errorf("assistant text not preserved: %q", m.askChat[1].Text)
	}

	// Second turn: another user prompt + reply.
	m.askChat = append(m.askChat, bedrock.Message{Role: bedrock.RoleUser, Text: "now sort by created_at desc"})
	tm, _ = m.Update(askResultMsg{prompt: "now sort by created_at desc", sql: "SELECT * FROM users WHERE active = true ORDER BY created_at DESC;"})
	m = tm.(Model)

	if got := len(m.askChat); got != 4 {
		t.Fatalf("expected 4 messages after second turn, got %d", got)
	}
}

// TestAskChatRollbackOnError ensures that a failed ask call does not leave
// a dangling user message in the chat history; otherwise the next retry
// would send a duplicate of the prompt to the model.
func TestAskChatRollbackOnError(t *testing.T) {
	m := newModel(nil, target{}, nil, nil, "", "", aws.Config{})
	m.askChat = append(m.askChat, bedrock.Message{Role: bedrock.RoleUser, Text: "broken question"})

	tm, _ := m.Update(askResultMsg{prompt: "broken question", err: errors.New("boom")})
	m = tm.(Model)

	if got := len(m.askChat); got != 0 {
		t.Errorf("expected user turn to be rolled back on error, got %d messages: %+v", got, m.askChat)
	}
	if m.lastErr == nil {
		t.Error("expected lastErr to be set after error")
	}
}

// TestAskChatResetOnTargetSwitch verifies the chat is reset when the user
// switches cluster, so the next Ask AI call does not carry context that no
// longer matches the active schema. The clearing now happens during
// finalizeTargetSwitch (after the database picker commit) rather than in
// the earlier beginTargetSwitch, so the test drives the full switch.
func TestAskChatResetOnTargetSwitch(t *testing.T) {
	m := newModel(nil, target{}, nil, nil, "", "", aws.Config{})
	m.askChat = []bedrock.Message{
		{Role: bedrock.RoleUser, Text: "old prompt"},
		{Role: bedrock.RoleAssistant, Text: "old reply"},
	}

	m.beginTargetSwitch(connection.ClusterInfo{ARN: "arn:new", Identifier: "new"}, "arn:secret")
	m.finalizeTargetSwitch("mydb")

	if len(m.askChat) != 0 {
		t.Errorf("expected askChat to be cleared on target switch, got %+v", m.askChat)
	}
}

// TestExecuteErrorDoesNotAutoExplain locks in the new behavior: an SQL
// error must not trigger explainExecuting on its own. The user is now
// responsible for pressing Ctrl+G after focusing the results pane.
func TestExecuteErrorDoesNotAutoExplain(t *testing.T) {
	bd := &bedrock.Client{}
	m := newModel(nil, target{}, nil, bd, "claude", "Japanese", aws.Config{})

	tm, _ := m.Update(executeMsg{
		SQL: "SELECT * FROM userz",
		Err: errors.New("table not found"),
	})
	m = tm.(Model)

	if m.explainExecuting {
		t.Error("execute error must not auto-trigger explain anymore")
	}
	if m.lastErr == nil {
		t.Error("lastErr should still be set")
	}
}

// TestF6OnEditorOpensReviewPrompt verifies that pressing F6 with editor
// focus + non-empty SQL opens the ask input in askKindReview state, and
// pressing Enter (with no focus area) actually launches the review.
func TestF6OnEditorOpensReviewPrompt(t *testing.T) {
	bd := &bedrock.Client{}
	m := newModel(nil, target{}, nil, bd, "claude", "Japanese", aws.Config{})
	m.width, m.height = 100, 40
	m.layout()
	m.focus = focusEditor
	m.editor.SetValue("SELECT * FROM users")

	// F6
	tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyF6})
	m = tm.(Model)
	if !m.askOpen {
		t.Fatalf("expected ask input to open after F6")
	}
	if m.askKind != askKindReview {
		t.Errorf("expected askKindReview, got %v", m.askKind)
	}
	if m.pendingReviewSQL != "SELECT * FROM users" {
		t.Errorf("expected SQL snapshot in pendingReviewSQL, got %q", m.pendingReviewSQL)
	}

	// Enter with empty input → review starts (general). The textarea's
	// InsertNewline binding has been moved off plain Enter so this hits
	// our submit branch instead of inserting a line break.
	tm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = tm.(Model)
	if m.askOpen {
		t.Errorf("expected ask input to close after enter")
	}
	if !m.explainExecuting {
		t.Errorf("expected explainExecuting=true after submitting review")
	}
	if m.aiBusyLabel != "reviewing SQL..." {
		t.Errorf("expected reviewing SQL label, got %q", m.aiBusyLabel)
	}

	// Simulate the Bedrock round trip completing and ensure focus
	// jumps to the results pane so the user can immediately drive the
	// review overlay with vim keys instead of having to Tab.
	tm, _ = m.Update(reviewResultMsg{sql: "SELECT * FROM users", review: "looks good"})
	m = tm.(Model)
	if !m.explainOpen {
		t.Errorf("expected explain overlay to open after reviewResultMsg")
	}
	if m.focus != focusResults {
		t.Errorf("expected focus to switch to results pane after review result, got %v", m.focus)
	}
}

// TestF6OnResultsOpensAnalyzePrompt verifies F6 on results with rows
// snapshots the data and opens askKindAnalyze.
func TestF6OnResultsOpensAnalyzePrompt(t *testing.T) {
	bd := &bedrock.Client{}
	m := newModel(nil, target{}, nil, bd, "claude", "Japanese", aws.Config{})
	m.width, m.height = 100, 40
	m.layout()
	m.result = makeResult(2, 2)
	m.refreshTable()
	m.lastSQL = "SELECT id, name FROM users"
	m.focus = focusResults

	tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyF6})
	m = tm.(Model)
	if !m.askOpen {
		t.Fatalf("expected ask input to open")
	}
	if m.askKind != askKindAnalyze {
		t.Errorf("expected askKindAnalyze, got %v", m.askKind)
	}
	if m.pendingAnalyzeSQL != "SELECT id, name FROM users" {
		t.Errorf("expected SQL snapshot, got %q", m.pendingAnalyzeSQL)
	}
	if m.pendingAnalyzeBlob == "" {
		t.Errorf("expected result blob to be captured")
	}

	tm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = tm.(Model)
	if m.askOpen {
		t.Errorf("expected ask input to close after enter")
	}
	if !m.explainExecuting {
		t.Errorf("expected explainExecuting=true after submitting analyze")
	}
	if m.aiBusyLabel != "analyzing result..." {
		t.Errorf("expected analyzing result label, got %q", m.aiBusyLabel)
	}
}

// TestComposeExplainTextPrependsError guarantees that the explain overlay
// always shows the verbatim database error before the LLM analysis, even
// if the model rephrased or omitted it.
func TestComposeExplainTextPrependsError(t *testing.T) {
	err := errors.New("ERROR 1146 (42S02): Table 'studypocket.userz' doesn't exist")
	out := composeExplainText(err, "Looks like a typo. Try `users` instead.")

	if !contains(out, "## Database error") {
		t.Errorf("expected Database error heading:\n%s", out)
	}
	if !contains(out, "Table 'studypocket.userz' doesn't exist") {
		t.Errorf("expected verbatim error preserved:\n%s", out)
	}
	if !contains(out, "## Analysis") {
		t.Errorf("expected Analysis heading:\n%s", out)
	}
	if !contains(out, "Try `users` instead.") {
		t.Errorf("expected analysis body preserved:\n%s", out)
	}
}

// TestComposeExplainTextNilError falls through to the analysis-only path
// without crashing — defensive guard for callers that might lose lastErr.
func TestComposeExplainTextNilError(t *testing.T) {
	out := composeExplainText(nil, "Just the analysis.")
	if out != "Just the analysis." {
		t.Errorf("expected raw analysis when err is nil, got %q", out)
	}
}

// TestFlashAutoClearOnYank: after a yy yank the flashMessage is cleared
// when the matching clearFlashMsg fires, but a stale tick from an earlier
// flash must not blank a freshly-replaced message.
func TestFlashAutoClearOnYank(t *testing.T) {
	m := newModel(nil, target{}, nil, nil, "", "", aws.Config{})
	m.width, m.height = 100, 40
	m.layout()
	m.result = makeResult(2, 2)
	m.refreshTable()
	m.focus = focusResults
	m.table.Focus()

	// First flash: simulate a successful yank by setting flash + grabbing
	// the token from scheduleFlashClear.
	m.flashMessage = "✓ result CSV yanked to clipboard"
	cmd1 := m.scheduleFlashClear()
	if cmd1 == nil {
		t.Fatal("expected schedule cmd")
	}
	token1 := m.flashToken

	// A newer flash arrives before the first tick fires.
	m.flashMessage = "✓ row JSON yanked to clipboard"
	_ = m.scheduleFlashClear()
	if m.flashToken == token1 {
		t.Fatal("flashToken should have advanced for the second message")
	}

	// Stale tick from cmd1 should be ignored.
	tm, _ := m.Update(clearFlashMsg{token: token1})
	m = tm.(Model)
	if m.flashMessage == "" {
		t.Errorf("stale clearFlashMsg should not blank a newer flash, got empty")
	}

	// The second tick (current token) clears it.
	tm, _ = m.Update(clearFlashMsg{token: m.flashToken})
	m = tm.(Model)
	if m.flashMessage != "" {
		t.Errorf("expected current clearFlashMsg to clear the flash, got %q", m.flashMessage)
	}
}

// TestInspectorFooterShowsFlash ensures that a yank flash inside the row
// inspector is visible in the footer instead of being hidden behind the
// static "inspecting row N/M" hint.
func TestInspectorFooterShowsFlash(t *testing.T) {
	m := newModel(nil, target{}, nil, nil, "", "", aws.Config{})
	m.width, m.height = 100, 40
	m.layout()
	m.result = makeResult(2, 2)
	m.refreshTable()
	m.focus = focusResults
	m.openInspector(0)
	if !m.inspecting {
		t.Fatal("openInspector did not flip inspecting")
	}

	m.flashMessage = "✓ row JSON yanked to clipboard"
	footer := m.renderFooter()
	if !contains(footer, "row JSON yanked") {
		t.Errorf("expected inspector footer to render flash message, got %q", footer)
	}
}

// TestJSONViewVimNavigation: gg/G navigation inside the JSON results view
// (Ctrl+J toggle) jumps to top / bottom via the same handleViewportNav
// helper used by the inspector and explain overlays.
func TestJSONViewVimNavigation(t *testing.T) {
	m := newModel(nil, target{}, nil, nil, "", "", aws.Config{})
	m.width, m.height = 100, 40
	m.layout()
	m.jsonView.Width = 40
	m.jsonView.Height = 5
	long := ""
	for i := 0; i < 50; i++ {
		long += "{\"k\":\"v\"}\n"
	}
	m.jsonView.SetContent(long)
	m.jsonView.GotoTop()
	m.mode = viewJSON
	m.focus = focusResults
	m.result = makeResult(1, 1) // not nil so render paths are valid

	send := func(model Model, r rune) Model {
		next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		return next.(Model)
	}

	if y := m.jsonView.YOffset; y != 0 {
		t.Fatalf("expected initial JSON view YOffset 0, got %d", y)
	}
	m = send(m, 'G')
	if y := m.jsonView.YOffset; y == 0 {
		t.Errorf("expected JSON view YOffset > 0 after G, got %d", y)
	}
	m = send(m, 'g')
	m = send(m, 'g')
	if y := m.jsonView.YOffset; y != 0 {
		t.Errorf("expected JSON view YOffset 0 after gg, got %d", y)
	}
}

// TestTableColumnCursorMovesWithHL verifies that pressing l in the
// results table advances colCursor to the right and h moves it back, with
// $/0 jumping to the ends. The bubbles/table cursor itself stays put
// because it only tracks rows.
func TestTableColumnCursorMovesWithHL(t *testing.T) {
	m := newModel(nil, target{}, nil, nil, "", "", aws.Config{})
	m.width, m.height = 120, 40
	m.layout()
	m.result = makeResult(5, 3) // 5 columns, 3 rows
	m.refreshTable()
	m.focus = focusResults
	m.table.Focus()

	send := func(model Model, r rune) Model {
		next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		return next.(Model)
	}

	if m.colCursor != 0 {
		t.Fatalf("expected colCursor to start at 0, got %d", m.colCursor)
	}
	m = send(m, 'l')
	if m.colCursor != 1 {
		t.Errorf("expected colCursor=1 after 'l', got %d", m.colCursor)
	}
	m = send(m, 'l')
	m = send(m, 'l')
	if m.colCursor != 3 {
		t.Errorf("expected colCursor=3 after three 'l', got %d", m.colCursor)
	}
	m = send(m, 'h')
	if m.colCursor != 2 {
		t.Errorf("expected colCursor=2 after 'h', got %d", m.colCursor)
	}
	m = send(m, '$')
	if m.colCursor != 4 {
		t.Errorf("expected colCursor=4 after '$', got %d", m.colCursor)
	}
	m = send(m, '0')
	if m.colCursor != 0 {
		t.Errorf("expected colCursor=0 after '0', got %d", m.colCursor)
	}
	// j/k still move the row cursor (delegated to bubbles/table).
	m = send(m, 'j')
	if m.table.Cursor() != 1 {
		t.Errorf("expected row cursor to advance with 'j', got %d", m.table.Cursor())
	}
}

// TestTableFooterShowsCursorPosition asserts that the success footer
// includes the row N/M and col K/L name fragments while the results
// pane is focused on a populated table.
func TestTableFooterShowsCursorPosition(t *testing.T) {
	m := newModel(nil, target{}, nil, nil, "", "", aws.Config{})
	m.width, m.height = 120, 40
	m.layout()
	m.result = makeResult(3, 4)
	m.refreshTable()
	m.focus = focusResults
	m.table.Focus()
	m.colCursor = 1

	footer := m.renderFooter()
	if !contains(footer, "row 1/4") {
		t.Errorf("expected 'row 1/4' in footer, got %q", footer)
	}
	if !contains(footer, "col 2/3") {
		t.Errorf("expected 'col 2/3' in footer, got %q", footer)
	}
	if !contains(footer, "col1") {
		t.Errorf("expected column name 'col1' in footer, got %q", footer)
	}
}

// TestInspectorVimNavigation drives the inspector through the new vim-style
// keys (G to bottom, gg to top). The underlying viewport is given a tall
// content body so YOffset can actually move; afterwards we assert that
// G non-zero offset and gg returns to 0.
func TestInspectorVimNavigation(t *testing.T) {
	m := newModel(nil, target{}, nil, nil, "", "", aws.Config{})
	m.width, m.height = 100, 40
	m.layout()
	// Force the viewport small relative to its content so vertical
	// scrolling has somewhere to go.
	m.inspectorVP.Width = 40
	m.inspectorVP.Height = 5
	long := ""
	for i := 0; i < 50; i++ {
		long += "row\n"
	}
	m.inspectorVP.SetContent(long)
	m.inspectorVP.GotoTop()
	m.inspecting = true
	m.focus = focusResults

	send := func(model Model, r rune) Model {
		next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		return next.(Model)
	}

	if y := m.inspectorVP.YOffset; y != 0 {
		t.Fatalf("expected initial YOffset 0, got %d", y)
	}
	m = send(m, 'G')
	if y := m.inspectorVP.YOffset; y == 0 {
		t.Errorf("expected YOffset > 0 after G, got %d", y)
	}
	m = send(m, 'g')
	m = send(m, 'g')
	if y := m.inspectorVP.YOffset; y != 0 {
		t.Errorf("expected YOffset 0 after gg, got %d", y)
	}
}

// TestInspectorScrollLeftRight: the row inspector keeps long string
// values on a single line and supports horizontal scrolling via h/l/0/$.
// We feed the viewport one wide line, push the viewport.Width down so the
// line overflows, then assert that l moves XOffset forward, h moves it
// back to 0, and $ jumps to the rightmost column.
func TestInspectorScrollLeftRight(t *testing.T) {
	m := newModel(nil, target{}, nil, nil, "", "", aws.Config{})
	m.width, m.height = 100, 40
	m.layout()
	m.inspectorVP.Width = 20
	m.inspectorVP.Height = 5
	long := strings.Repeat("ABCDEFGHIJ", 20) // 200 chars on a single line
	m.inspectorVP.SetContent(long)
	m.inspectorVP.SetXOffset(0)
	m.inspecting = true
	m.focus = focusResults
	m.result = makeResult(1, 1)

	send := func(model Model, r rune) Model {
		next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		return next.(Model)
	}

	m = send(m, 'l')
	if pct := m.inspectorVP.HorizontalScrollPercent(); pct == 0 {
		t.Fatalf("expected horizontal scroll percent > 0 after l, got %f", pct)
	}
	m = send(m, 'h')
	if pct := m.inspectorVP.HorizontalScrollPercent(); pct != 0 {
		// One l followed by one h should land back at 0 because both
		// share the same step.
		t.Fatalf("expected horizontal scroll percent 0 after h, got %f", pct)
	}
	m = send(m, '$')
	if pct := m.inspectorVP.HorizontalScrollPercent(); pct < 0.99 {
		t.Fatalf("expected horizontal scroll percent ~1.0 after $, got %f", pct)
	}
	m = send(m, '0')
	if pct := m.inspectorVP.HorizontalScrollPercent(); pct != 0 {
		t.Fatalf("expected horizontal scroll percent 0 after 0, got %f", pct)
	}
}

// TestInspectorFooterShowsLine checks that the inspector footer includes
// a "line N/M" segment so the user can see where the cursor is.
func TestInspectorFooterShowsLine(t *testing.T) {
	m := newModel(nil, target{}, nil, nil, "", "", aws.Config{})
	m.width, m.height = 100, 40
	m.layout()
	m.result = makeResult(2, 3)
	m.refreshTable()
	m.focus = focusResults
	m.openInspector(0)
	if !m.inspecting {
		t.Fatal("openInspector did not flip inspecting")
	}

	footer := m.renderFooter()
	if !contains(footer, "line ") {
		t.Errorf("expected 'line N/M' segment in footer, got %q", footer)
	}
	if !contains(footer, "h/l/gg/G nav") {
		t.Errorf("expected nav hint in footer, got %q", footer)
	}
}

// TestInspectorFooterShowsLineAndXOffset puts the inspector in a state
// where YOffset and XOffset are both non-zero and checks that the footer
// surfaces both positions, so the user can tell where they are after
// scrolling vertically and horizontally.
func TestInspectorFooterShowsLineAndXOffset(t *testing.T) {
	m := newModel(nil, target{}, nil, nil, "", "", aws.Config{})
	m.width, m.height = 100, 40
	m.layout()
	m.result = makeResult(1, 1)
	m.refreshTable()
	m.focus = focusResults
	m.openInspector(0)
	if !m.inspecting {
		t.Fatal("openInspector did not flip inspecting")
	}
	// Override the inspector content and dimensions so we can drive
	// both axes deterministically without depending on layout sizes.
	m.inspectorVP.Width = 10
	m.inspectorVP.Height = 5
	long := strings.Repeat("X", 100) + "\n" + strings.Repeat("Y", 100)
	m.inspectorVP.SetContent(long)
	m.inspectorVP.SetXOffset(8)

	footer := m.renderFooter()
	if !contains(footer, "line 1/") {
		t.Errorf("expected 'line 1/' in footer, got %q", footer)
	}
	if !contains(footer, "x ") {
		t.Errorf("expected 'x %%' segment in footer, got %q", footer)
	}
}

// TestTabIsBlockedWhileInspecting guards the focus pin: pressing Tab while
// the inspector is open must not move focus to the editor, otherwise yy
// would silently stop working.
func TestTabIsBlockedWhileInspecting(t *testing.T) {
	m := newModel(nil, target{}, nil, nil, "", "", aws.Config{})
	m.width, m.height = 100, 40
	m.layout()
	m.result = makeResult(1, 2)
	m.refreshTable()
	m.focus = focusResults
	m.table.Focus()
	m.openInspector(0)
	if !m.inspecting {
		t.Fatal("expected inspecting after openInspector")
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	if m.focus != focusResults {
		t.Errorf("Tab should be a no-op while inspecting, got focus=%v", m.focus)
	}
}

// TestYankPayloadByContext verifies that yankPayload picks the right
// content for each results sub-mode: explain overlay, row inspector, JSON
// view, table view. This is the regression for "yy only worked in explain".
func TestYankPayloadByContext(t *testing.T) {
	m := newModel(nil, target{}, nil, nil, "", "", aws.Config{})
	m.result = makeResult(2, 3)
	m.jsonText = m.result.toJSON()

	// Table view (default mode after a successful execution).
	m.mode = viewTable
	payload, label, err := m.yankPayload()
	if err != nil {
		t.Fatalf("table view: %v", err)
	}
	if label != "result CSV" {
		t.Errorf("table view label = %q, want %q", label, "result CSV")
	}
	if !contains(payload, "col0") || !contains(payload, "v1_1") {
		t.Errorf("table view payload missing CSV header/row:\n%s", payload)
	}

	// JSON view.
	m.mode = viewJSON
	payload, label, err = m.yankPayload()
	if err != nil {
		t.Fatalf("json view: %v", err)
	}
	if label != "result JSON" {
		t.Errorf("json view label = %q", label)
	}
	if payload != m.jsonText {
		t.Errorf("json view payload mismatch")
	}

	// Row inspector.
	m.openInspector(1)
	if !m.inspecting {
		t.Fatal("openInspector did not flip inspecting")
	}
	payload, label, err = m.yankPayload()
	if err != nil {
		t.Fatalf("inspector: %v", err)
	}
	if label != "row JSON" {
		t.Errorf("inspector label = %q", label)
	}
	if !contains(payload, "v1_0") {
		t.Errorf("inspector payload missing row data:\n%s", payload)
	}

	// Explain overlay takes precedence over everything else.
	m.explainOpen = true
	m.explainText = "## error explanation"
	payload, label, err = m.yankPayload()
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	if label != "explanation" {
		t.Errorf("explain label = %q", label)
	}
	if payload != "## error explanation" {
		t.Errorf("explain payload = %q", payload)
	}

	// SQL error only: executeMsg sets lastErr and clears result on
	// failure. yy should copy the raw error string.
	m2 := newModel(nil, target{}, nil, nil, "", "", aws.Config{})
	sqlErr := errors.New(`ERROR: relation "foo" does not exist`)
	m2.lastErr = sqlErr
	payload, label, err = m2.yankPayload()
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if label != "error" {
		t.Errorf("error label = %q, want %q", label, "error")
	}
	if payload != sqlErr.Error() {
		t.Errorf("error payload = %q, want %q", payload, sqlErr.Error())
	}

	// Explain overlay still wins when both are present — the composed
	// explanation already embeds the original error.
	m2.explainOpen = true
	m2.explainText = "## analysis"
	payload, label, _ = m2.yankPayload()
	if label != "explanation" || payload != "## analysis" {
		t.Errorf("explain should beat error: label=%q payload=%q", label, payload)
	}
}

// TestYankErrorPreservesLastErr verifies that yanking an on-screen SQL
// error does not clear it — the user asked to copy what's visible, not
// to dismiss it.
func TestYankErrorPreservesLastErr(t *testing.T) {
	m := newModel(nil, target{}, nil, nil, "", "", aws.Config{})
	sqlErr := errors.New(`ERROR: syntax error near "FROM"`)
	m.lastErr = sqlErr
	m.focus = focusResults

	m.copyResultContext()

	if errors.Is(m.lastErr, errClipboardCopy) {
		// Headless CI: clipboard write failed; nothing else to check.
		return
	}
	if m.lastErr != sqlErr {
		t.Errorf("SQL error should remain visible after yank, got %v", m.lastErr)
	}
	if m.flashMessage != "✓ error yanked to clipboard" {
		t.Errorf("flashMessage = %q", m.flashMessage)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// TestCanExplainError covers the gating logic that decides whether Ctrl+G
// should fall through to the error analyst flow. Bedrock must be wired up,
// the empty-SQL sentinel must be ignored, and "no error" must short-circuit.
func TestCanExplainError(t *testing.T) {
	bd := &bedrock.Client{}

	cases := []struct {
		name   string
		client *bedrock.Client
		err    error
		want   bool
	}{
		{"no client", nil, errors.New("syntax"), false},
		{"no error", bd, nil, false},
		{"empty sql sentinel", bd, errEmptySQLValue{}, false},
		{"real error", bd, errors.New("table not found"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newModel(nil, target{}, nil, tc.client, "", "", aws.Config{})
			m.lastErr = tc.err
			if got := m.canExplainError(); got != tc.want {
				t.Errorf("canExplainError() = %v, want %v", got, tc.want)
			}
		})
	}

	// Once the explanation is on screen Ctrl+G should stop flipping to
	// explain and fall through to the ask-AI flow instead.
	t.Run("explain overlay still allows re-run", func(t *testing.T) {
		// F8 should be re-invocable even while the previous explanation
		// is on screen — the user can refresh the analysis after running
		// a new failing query.
		m := newModel(nil, target{}, nil, bd, "", "", aws.Config{})
		m.lastErr = errors.New("table not found")
		m.explainOpen = true
		if !m.canExplainError() {
			t.Error("canExplainError should stay true while explainOpen; the user can re-explain")
		}
	})
	t.Run("currently explaining", func(t *testing.T) {
		m := newModel(nil, target{}, nil, bd, "", "", aws.Config{})
		m.lastErr = errors.New("table not found")
		m.explainExecuting = true
		if m.canExplainError() {
			t.Error("canExplainError should be false while explainExecuting")
		}
	})
}

// searchResult returns a small result with predictable content so the
// search tests can pick known needles that hit in specific rows / cells.
// Columns: name, city. Rows:
//
//	alice  tokyo
//	bob    osaka
//	carol  tokyo    <- "tokyo" hits twice (rows 0 and 2)
//	dave   sapporo
func searchResult() *queryResult {
	return &queryResult{
		Columns: []string{"name", "city"},
		Rows: [][]any{
			{"alice", "tokyo"},
			{"bob", "osaka"},
			{"carol", "tokyo"},
			{"dave", "sapporo"},
		},
	}
}

// TestComputeTableHits exercises the pure match computation directly so
// we can assert positions without going through the full bubbletea loop.
func TestComputeTableHits(t *testing.T) {
	r := searchResult()

	hits := computeTableHits(r, "tokyo")
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits for tokyo, got %d: %+v", len(hits), hits)
	}
	if hits[0].row != 0 || hits[0].cell != 1 {
		t.Errorf("hit[0] = %+v, want row=0 cell=1", hits[0])
	}
	if hits[1].row != 2 || hits[1].cell != 1 {
		t.Errorf("hit[1] = %+v, want row=2 cell=1", hits[1])
	}
	if hits[0].length != len("tokyo") {
		t.Errorf("hit length = %d, want %d", hits[0].length, len("tokyo"))
	}

	// Case-insensitive: "BOB" matches "bob".
	if got := computeTableHits(r, "BOB"); len(got) != 1 || got[0].row != 1 {
		t.Errorf("BOB hits = %+v, want 1 hit on row 1", got)
	}

	// Empty query / nil result → no hits.
	if got := computeTableHits(r, ""); got != nil {
		t.Errorf("empty query should yield nil, got %+v", got)
	}
	if got := computeTableHits(nil, "tokyo"); got != nil {
		t.Errorf("nil result should yield nil, got %+v", got)
	}
}

// TestComputeJSONHits verifies line-level matching against the plain
// text used in refreshJSON's match branch.
func TestComputeJSONHits(t *testing.T) {
	raw := "[\n  {\n    \"name\": \"alice\",\n    \"city\": \"tokyo\"\n  },\n  {\n    \"name\": \"bob\",\n    \"city\": \"tokyo\"\n  }\n]"

	hits := computeJSONHits(raw, "tokyo")
	if len(hits) != 2 {
		t.Fatalf("expected 2 json hits, got %d: %+v", len(hits), hits)
	}
	if hits[0].line == hits[1].line {
		t.Errorf("hits should live on different lines, got %+v", hits)
	}

	if got := computeJSONHits("", "tokyo"); got != nil {
		t.Errorf("empty raw should yield nil, got %+v", got)
	}
}

// TestSearchOpenAndCommit drives the full "/" → type → Enter flow through
// Update and asserts the search state is populated and the table cursor
// jumped to the first match.
func TestSearchOpenAndCommit(t *testing.T) {
	m := newModel(nil, target{}, nil, nil, "", "", aws.Config{})
	m.width, m.height = 120, 40
	m.layout()
	m.result = searchResult()
	m.jsonRaw = m.result.toJSON()
	m.jsonText = m.jsonRaw
	m.refreshTable()
	m.refreshJSON()
	m.focus = focusResults
	m.editor.Blur()
	m.table.Focus()

	var model tea.Model = m
	// "/" opens the prompt.
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !model.(Model).searchOpen {
		t.Fatal("expected searchOpen after /")
	}

	// Type "tokyo" into the prompt.
	for _, r := range "tokyo" {
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Enter commits.
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = model.(Model)

	if m.searchOpen {
		t.Error("searchOpen should be false after Enter")
	}
	if m.searchQuery != "tokyo" {
		t.Errorf("searchQuery = %q, want tokyo", m.searchQuery)
	}
	if len(m.tableHits) != 2 {
		t.Errorf("tableHits = %d, want 2", len(m.tableHits))
	}
	if m.table.Cursor() != 0 {
		t.Errorf("table cursor = %d, want first hit row 0", m.table.Cursor())
	}
	if m.colCursor != 1 {
		t.Errorf("colCursor = %d, want 1 (city column)", m.colCursor)
	}
}

// TestSearchNextPrev confirms n/N cycle through the hit list with
// wrap-around on both ends.
func TestSearchNextPrev(t *testing.T) {
	m := newModel(nil, target{}, nil, nil, "", "", aws.Config{})
	m.width, m.height = 120, 40
	m.layout()
	m.result = searchResult()
	m.jsonRaw = m.result.toJSON()
	m.jsonText = m.jsonRaw
	m.refreshTable()
	m.refreshJSON()
	m.focus = focusResults
	m.editor.Blur()
	m.table.Focus()

	// Set up a committed search directly (no textinput round trip).
	m.searchQuery = "tokyo"
	m.runSearch()
	m.searchCursor = 0
	m.alignSearchAnchors()
	m.refreshTable()
	m.refreshJSON()
	m.focusSearchCursor()

	var model tea.Model = m
	// n: advance from 0 → 1.
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = model.(Model)
	if m.searchCursor != 1 {
		t.Errorf("after n, cursor = %d, want 1", m.searchCursor)
	}
	if m.table.Cursor() != 2 {
		t.Errorf("after n, table cursor = %d, want 2", m.table.Cursor())
	}

	// n again: wrap 1 → 0.
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = model.(Model)
	if m.searchCursor != 0 {
		t.Errorf("after n wrap, cursor = %d, want 0", m.searchCursor)
	}

	// N: wrap 0 → 1 (going backwards).
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	m = model.(Model)
	if m.searchCursor != 1 {
		t.Errorf("after N wrap, cursor = %d, want 1", m.searchCursor)
	}
}

// TestSearchEscCancelsInput verifies Esc while the prompt is open leaves
// any previously committed query intact.
func TestSearchEscCancelsInput(t *testing.T) {
	m := newModel(nil, target{}, nil, nil, "", "", aws.Config{})
	m.width, m.height = 120, 40
	m.layout()
	m.result = searchResult()
	m.jsonRaw = m.result.toJSON()
	m.jsonText = m.jsonRaw
	m.refreshTable()
	m.focus = focusResults
	m.editor.Blur()
	m.table.Focus()

	// Apply "tokyo" first so there is a committed query to preserve.
	m.searchQuery = "tokyo"
	m.runSearch()

	// Open the prompt, type garbage, Esc → committed query unchanged.
	var model tea.Model = m
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "zzz" {
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = model.(Model)
	if m.searchOpen {
		t.Error("Esc should close the search prompt")
	}
	if m.searchQuery != "tokyo" {
		t.Errorf("Esc should preserve committed query, got %q", m.searchQuery)
	}
	if len(m.tableHits) != 2 {
		t.Errorf("hits should still be populated, got %d", len(m.tableHits))
	}
}

// TestSearchExecuteMsgClears verifies a fresh SQL execution wipes the
// search so stale hits never outlive the result they came from.
func TestSearchExecuteMsgClears(t *testing.T) {
	m := newModel(nil, target{}, nil, nil, "", "", aws.Config{})
	m.width, m.height = 120, 40
	m.layout()
	m.result = searchResult()
	m.jsonRaw = m.result.toJSON()
	m.jsonText = m.jsonRaw
	m.refreshTable()
	m.searchQuery = "tokyo"
	m.runSearch()
	if len(m.tableHits) == 0 {
		t.Fatal("setup: expected hits to seed the test")
	}

	var model tea.Model = m
	model, _ = model.Update(executeMsg{Result: makeResult(2, 2), SQL: "SELECT 1"})
	m = model.(Model)
	if m.searchQuery != "" {
		t.Errorf("executeMsg should clear searchQuery, got %q", m.searchQuery)
	}
	if len(m.tableHits) != 0 {
		t.Errorf("executeMsg should clear tableHits, got %d", len(m.tableHits))
	}
}

// TestSearchViewToggleResetsCursor verifies Ctrl+J switches view mode and
// resets the hit cursor to the first match in the new mode, so the
// highlighted "current" hit belongs to whatever the user now sees.
func TestSearchViewToggleResetsCursor(t *testing.T) {
	m := newModel(nil, target{}, nil, nil, "", "", aws.Config{})
	m.width, m.height = 120, 40
	m.layout()
	m.result = searchResult()
	m.jsonRaw = m.result.toJSON()
	m.jsonText = m.jsonRaw
	m.refreshTable()
	m.refreshJSON()
	m.focus = focusResults
	m.editor.Blur()
	m.table.Focus()
	m.searchQuery = "tokyo"
	m.runSearch()
	m.searchCursor = 1 // advance past the first hit

	var model tea.Model = m
	// Ctrl+J toggles the view.
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	m = model.(Model)

	if m.mode != viewJSON {
		t.Errorf("mode = %v, want viewJSON", m.mode)
	}
	if m.searchCursor != 0 {
		t.Errorf("searchCursor should reset to 0 on view toggle, got %d", m.searchCursor)
	}
	if m.currentHitCount() == 0 {
		t.Errorf("expected JSON hits after toggle, got 0")
	}
}

// TestSearchNoMatchesFlash verifies the user-facing feedback when a
// committed query matches nothing — the flash message surfaces the miss
// so the empty-looking table isn't mistaken for "search is broken".
func TestSearchNoMatchesFlash(t *testing.T) {
	m := newModel(nil, target{}, nil, nil, "", "", aws.Config{})
	m.width, m.height = 120, 40
	m.layout()
	m.result = searchResult()
	m.jsonRaw = m.result.toJSON()
	m.jsonText = m.jsonRaw
	m.refreshTable()
	m.focus = focusResults
	m.editor.Blur()
	m.table.Focus()

	var model tea.Model = m
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "zzz" {
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = model.(Model)

	if len(m.tableHits) != 0 {
		t.Errorf("tableHits = %d, want 0", len(m.tableHits))
	}
	if m.flashMessage == "" || !contains(m.flashMessage, "no matches") {
		t.Errorf("flashMessage = %q, want contains 'no matches'", m.flashMessage)
	}
}

// setupDatabaseTest builds a Model wired up for the database-picker tests:
// a real target struct with a profile name, an isolated state file so
// persistTarget() does not touch the developer's real ~/.rdq/state.json,
// and layout() called so the picker has dimensions to render into.
func setupDatabaseTest(t *testing.T) Model {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("RDQ_STATE_FILE", filepath.Join(dir, "state.json"))
	m := newModel(nil, target{profile: "testprofile", database: "olddb"}, nil, nil, "", "", aws.Config{})
	m.width, m.height = 120, 40
	m.layout()
	return m
}

// TestBeginTargetSwitchOpensDatabasePicker verifies that confirming a
// cluster (via the cached-secret fast path) opens the database picker
// rather than finalising the switch immediately. Schema fetch must be
// deferred until the database is confirmed.
func TestBeginTargetSwitchOpensDatabasePicker(t *testing.T) {
	m := setupDatabaseTest(t)
	// Seed the per-profile state with a cached cluster→secret pairing
	// so the cluster picker takes the fast path and we can observe the
	// handoff to the database picker without running AWS SDK calls.
	st, err := state.Load()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	ps := st.Get("testprofile")
	ps.ClusterSecrets = map[string]string{"arn:new-cluster": "arn:new-secret"}
	st.Set("testprofile", ps)
	if err := st.Save(); err != nil {
		t.Fatalf("save state: %v", err)
	}

	cluster := connection.ClusterInfo{ARN: "arn:new-cluster", Identifier: "new-cluster"}
	cmd := m.beginTargetSwitch(cluster, "arn:new-secret")
	// beginTargetSwitch returns the picker-open cmd (which is nil in
	// this implementation) — the important thing is that schema fetch
	// has NOT been kicked off yet.
	_ = cmd

	if !m.databasePickerOpen {
		t.Fatal("expected databasePickerOpen after beginTargetSwitch")
	}
	if m.target.cluster != "arn:new-cluster" {
		t.Errorf("target.cluster = %q, want arn:new-cluster", m.target.cluster)
	}
	if m.target.secret != "arn:new-secret" {
		t.Errorf("target.secret = %q, want arn:new-secret", m.target.secret)
	}
	if m.target.database != "olddb" {
		t.Errorf("target.database should stay as olddb until the picker commits, got %q", m.target.database)
	}
	if m.snapshot != nil {
		t.Error("schema snapshot should not be cleared yet — user may still Esc")
	}
}

// TestDatabasePickerEnterCommitsTypedName exercises the typed-filter
// commit path: no history, user types a fresh database name, Enter
// applies it to m.target.database.
func TestDatabasePickerEnterCommitsTypedName(t *testing.T) {
	m := setupDatabaseTest(t)
	cluster := connection.ClusterInfo{ARN: "arn:new", Identifier: "new"}
	m.beginTargetSwitch(cluster, "arn:secret")

	// Type a new database name into the picker filter.
	applyPickerFilter(&m.databaseList, "staging_db")
	cmd := m.updateDatabasePicker(tea.KeyMsg{Type: tea.KeyEnter})
	// The returned cmd is the schema fetch for the new target — we
	// don't execute it, just confirm it was produced.
	if cmd == nil {
		t.Error("updateDatabasePicker Enter should return a schema-fetch cmd")
	}
	if m.databasePickerOpen {
		t.Error("picker should close after Enter")
	}
	if m.target.database != "staging_db" {
		t.Errorf("target.database = %q, want staging_db", m.target.database)
	}
}

// TestDatabasePickerEnterCommitsHistorySelection verifies that when the
// filter is empty and a history row is highlighted, Enter commits the
// highlighted row's database name.
func TestDatabasePickerEnterCommitsHistorySelection(t *testing.T) {
	m := setupDatabaseTest(t)
	// Seed a DatabaseHistory for the profile so openDatabasePicker
	// populates the list with real rows.
	st, _ := state.Load()
	ps := st.Get("testprofile")
	ps.DatabaseHistory = []string{"proddb", "stagingdb", "devdb"}
	st.Set("testprofile", ps)
	_ = st.Save()

	cluster := connection.ClusterInfo{ARN: "arn:new", Identifier: "new"}
	m.beginTargetSwitch(cluster, "arn:secret")

	if count := len(m.databaseList.Items()); count != 3 {
		t.Fatalf("expected 3 history items in picker, got %d", count)
	}
	// No filter → the first row (proddb) is selected by default.
	cmd := m.updateDatabasePicker(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Error("Enter should return a schema-fetch cmd")
	}
	if m.target.database != "proddb" {
		t.Errorf("target.database = %q, want proddb", m.target.database)
	}
}

// TestDatabasePickerEscRevertsSwitch confirms the Esc path: the
// pre-switch cluster and secret are restored and the database stays
// unchanged, so the user lands back exactly where they started before
// pressing Ctrl+T.
func TestDatabasePickerEscRevertsSwitch(t *testing.T) {
	m := setupDatabaseTest(t)
	m.target.cluster = "arn:old-cluster"
	m.target.secret = "arn:old-secret"

	cluster := connection.ClusterInfo{ARN: "arn:new", Identifier: "new"}
	m.beginTargetSwitch(cluster, "arn:new-secret")

	if m.target.cluster != "arn:new" {
		t.Fatalf("setup sanity: target.cluster = %q, want arn:new after begin", m.target.cluster)
	}

	cmd := m.updateDatabasePicker(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Errorf("Esc path should return nil cmd, got %v", cmd)
	}
	if m.databasePickerOpen {
		t.Error("picker should close after Esc")
	}
	if m.target.cluster != "arn:old-cluster" {
		t.Errorf("target.cluster should revert to old, got %q", m.target.cluster)
	}
	if m.target.secret != "arn:old-secret" {
		t.Errorf("target.secret should revert to old, got %q", m.target.secret)
	}
	if m.target.database != "olddb" {
		t.Errorf("target.database should stay as olddb, got %q", m.target.database)
	}
	if m.flashMessage == "" || !strings.Contains(m.flashMessage, "cancel") {
		t.Errorf("flashMessage should mention cancel, got %q", m.flashMessage)
	}
}

// TestDatabasePickerEmptyInputRejected verifies an empty commit is
// bounced back as an error without closing the picker: the user is
// forced to actually enter something.
func TestDatabasePickerEmptyInputRejected(t *testing.T) {
	m := setupDatabaseTest(t)
	// No history, no filter: Enter on a completely empty picker.
	cluster := connection.ClusterInfo{ARN: "arn:new", Identifier: "new"}
	m.beginTargetSwitch(cluster, "arn:secret")

	cmd := m.updateDatabasePicker(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Errorf("empty Enter should return nil cmd, got %v", cmd)
	}
	if !m.databasePickerOpen {
		t.Error("picker should stay open on empty commit")
	}
	if m.lastErr == nil {
		t.Error("expected lastErr to be set on empty commit")
	}
	if m.target.database != "olddb" {
		t.Errorf("target.database should stay as olddb, got %q", m.target.database)
	}
}

// TestFinalizeTargetSwitchPersistsHistory locks in that committing a
// database name pushes it to the front of DatabaseHistory in the state
// file, so the picker will surface it first on the next cluster switch.
func TestFinalizeTargetSwitchPersistsHistory(t *testing.T) {
	m := setupDatabaseTest(t)
	m.target.cluster = "arn:cluster"
	m.target.secret = "arn:secret"

	_ = m.finalizeTargetSwitch("newdb")

	st, err := state.Load()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	ps := st.Get("testprofile")
	if ps.Database != "newdb" {
		t.Errorf("persisted Database = %q, want newdb", ps.Database)
	}
	if len(ps.DatabaseHistory) == 0 || ps.DatabaseHistory[0] != "newdb" {
		t.Errorf("DatabaseHistory[0] = %v, want newdb first", ps.DatabaseHistory)
	}
}

// setupProductionTest is the production-flag counterpart of
// setupDatabaseTest: isolated state file + a non-empty profile so
// loadProductionFlag actually consults the state backend. The
// returned Model has layout() called so renderStatus / layout
// assertions work.
func setupProductionTest(t *testing.T) Model {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("RDQ_STATE_FILE", filepath.Join(dir, "state.json"))
	m := newModel(nil, target{profile: "testprofile"}, nil, nil, "", "", aws.Config{})
	m.width, m.height = 120, 40
	m.layout()
	return m
}

// TestNewModelOpensProductionPromptWhenFlagUnset verifies the first
// activation of a profile auto-opens the production picker, so the
// user cannot run queries before classifying the environment.
func TestNewModelOpensProductionPromptWhenFlagUnset(t *testing.T) {
	m := setupProductionTest(t)
	if !m.productionPromptOpen {
		t.Error("expected productionPromptOpen to be true on fresh profile")
	}
	if m.isProduction {
		t.Error("isProduction should default to false before user answers")
	}
}

// TestNewModelSkipsPromptWhenFlagAlreadySet verifies a profile with a
// stored IsProduction value skips the auto-prompt and hydrates the
// flag into the Model.
func TestNewModelSkipsPromptWhenFlagAlreadySet(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RDQ_STATE_FILE", filepath.Join(dir, "state.json"))

	// Seed the state file with IsProduction=true so newModel should
	// read and honor it without opening the prompt.
	st, err := state.Load()
	if err != nil {
		t.Fatalf("state load: %v", err)
	}
	ps := st.Get("testprofile")
	prod := true
	ps.IsProduction = &prod
	st.Set("testprofile", ps)
	if err := st.Save(); err != nil {
		t.Fatalf("state save: %v", err)
	}

	m := newModel(nil, target{profile: "testprofile"}, nil, nil, "", "", aws.Config{})
	if m.productionPromptOpen {
		t.Error("expected prompt to stay closed when flag is already set")
	}
	if !m.isProduction {
		t.Error("isProduction should hydrate to true from state")
	}
}

// TestProductionPromptEnterYesPersists walks the Yes path end-to-end:
// highlight row 0 (Yes), press Enter, verify isProduction + flash +
// state file are all updated.
func TestProductionPromptEnterYesPersists(t *testing.T) {
	m := setupProductionTest(t)
	// Highlight "Yes" (row 0) explicitly so the assertion is stable
	// regardless of whichever row openProductionPrompt selected.
	m.productionList.Select(0)

	cmd := m.updateProductionPrompt(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Error("expected a flash-clear cmd from Enter")
	}
	if m.productionPromptOpen {
		t.Error("prompt should close after Enter")
	}
	if !m.isProduction {
		t.Error("isProduction should be true after Enter on Yes row")
	}
	if !strings.Contains(m.flashMessage, "PRODUCTION") {
		t.Errorf("flashMessage = %q, want to mention PRODUCTION", m.flashMessage)
	}

	// Verify persistence survived the call.
	st, err := state.Load()
	if err != nil {
		t.Fatalf("state load: %v", err)
	}
	ps := st.Get("testprofile")
	if ps.IsProduction == nil || !*ps.IsProduction {
		t.Errorf("persisted IsProduction = %v, want *true", ps.IsProduction)
	}
}

// TestProductionPromptEscKeepsFlagNil verifies Esc does NOT persist
// anything — so a fresh profile that was Esc'd out will re-prompt on
// the next activation rather than silently defaulting to "non-prod".
func TestProductionPromptEscKeepsFlagNil(t *testing.T) {
	m := setupProductionTest(t)

	_ = m.updateProductionPrompt(tea.KeyMsg{Type: tea.KeyEsc})

	if m.productionPromptOpen {
		t.Error("prompt should close after Esc")
	}
	st, err := state.Load()
	if err != nil {
		t.Fatalf("state load: %v", err)
	}
	ps := st.Get("testprofile")
	if ps.IsProduction != nil {
		t.Errorf("IsProduction should stay nil after Esc, got %v", *ps.IsProduction)
	}
}

// TestF7ReopensProductionPrompt confirms the F7 keybinding can reopen
// the picker any time for a profile that already has a stored value,
// so the user can change their mind later without editing state.json.
func TestF7ReopensProductionPrompt(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RDQ_STATE_FILE", filepath.Join(dir, "state.json"))
	st, _ := state.Load()
	ps := st.Get("testprofile")
	no := false
	ps.IsProduction = &no
	st.Set("testprofile", ps)
	_ = st.Save()

	m := newModel(nil, target{profile: "testprofile"}, nil, nil, "", "", aws.Config{})
	if m.productionPromptOpen {
		t.Fatal("setup: prompt should be closed initially")
	}

	var model tea.Model = m
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyF7})
	m = model.(Model)

	if !m.productionPromptOpen {
		t.Error("F7 should reopen the production prompt")
	}
}

// TestProductionBannerInRenderStatus verifies the ⚠ PRODUCTION ⚠
// banner actually appears in the rendered status bar when the flag is
// active, and is absent otherwise. This is the regression for the
// "warning feel" guarantee.
func TestProductionBannerInRenderStatus(t *testing.T) {
	m := setupProductionTest(t)
	m.productionPromptOpen = false

	m.isProduction = true
	out := m.renderStatus()
	if !strings.Contains(out, "PRODUCTION") {
		t.Errorf("production banner missing from renderStatus:\n%s", out)
	}

	m.isProduction = false
	out = m.renderStatus()
	if strings.Contains(out, "PRODUCTION") {
		t.Errorf("PRODUCTION should not appear when flag is false:\n%s", out)
	}
}
