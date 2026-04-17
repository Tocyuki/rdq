package tui

import (
	"os"
	"strings"

	"github.com/Tocyuki/rdq/internal/runner"
)

// queryResult aliases runner.Result so the existing TUI code (result fields,
// method calls, pointer receivers) keeps working while the core logic lives
// in the reusable runner package.
type queryResult = runner.Result

// shortenPath replaces $HOME with ~ for compact display in the status line.
// Kept here because it is a TUI-only presentation helper with no API consumer.
func shortenPath(p string) string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" && strings.HasPrefix(p, home) {
		return "~" + p[len(home):]
	}
	return p
}
