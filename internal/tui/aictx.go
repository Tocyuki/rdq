package tui

import (
	"errors"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Tocyuki/rdq/internal/aictx"
)

// aictxLoadedMsg delivers the result of an asynchronous aictx.Load. content
// is the trimmed prompt context for the active (cluster, database). On any
// error content is empty so the AI flows fall through to schema-only
// prompts — load failures are non-fatal.
type aictxLoadedMsg struct {
	content string
}

// aictxSavedMsg is dispatched after the editor's submit handler tries to
// persist new content. content is the trimmed payload that was actually
// stored ("" if the user submitted blank, which triggers a delete). err
// is non-nil when persistence failed and lets the model flash a status
// message.
type aictxSavedMsg struct {
	content string
	err     error
}

// loadAictxCmd reads the saved (cluster, database) context off disk in the
// background. Empty cluster or database short-circuits to "no context".
func loadAictxCmd(cluster, database string) tea.Cmd {
	return func() tea.Msg {
		if cluster == "" || database == "" {
			return aictxLoadedMsg{}
		}
		content, _ := aictx.LoadContent(cluster, database)
		return aictxLoadedMsg{content: content}
	}
}

// saveAictxCmd persists the content for (cluster, database). An empty
// (after trim) payload triggers a delete instead so the user can clear
// the entry from the editor by submitting a blank value.
func saveAictxCmd(cluster, database, content string) tea.Cmd {
	return func() tea.Msg {
		if cluster == "" || database == "" {
			return aictxSavedMsg{err: errors.New("no cluster/database selected")}
		}
		if strings.TrimSpace(content) == "" {
			err := aictx.Delete(cluster, database)
			return aictxSavedMsg{content: "", err: err}
		}
		c := &aictx.Context{Cluster: cluster, Database: database, Content: content}
		if err := aictx.Save(c); err != nil {
			return aictxSavedMsg{err: err}
		}
		// aictx.Save trims c.Content in place — propagate the canonical
		// (trimmed) value back to the model so a subsequent Ctrl+G uses
		// exactly what is on disk.
		return aictxSavedMsg{content: c.Content}
	}
}
