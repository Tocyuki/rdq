package connection

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/ktr0731/go-fuzzyfinder"
	"golang.org/x/term"
)

// manualEntryLabel is the synthetic fuzzy-finder entry that drops to a free
// text prompt instead of reusing a previous database name.
const manualEntryLabel = "<enter database name manually>"

// PromptDatabase asks the user for a database name. With no history it falls
// back to a plain readline prompt; with history it shows a fuzzy picker over
// previous entries plus a manual-entry escape hatch.
func PromptDatabase(history []string) (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", errors.New("--database requires a TTY for interactive selection; pass a name explicitly")
	}

	if len(history) == 0 {
		return readDatabaseLine()
	}

	options := append([]string{manualEntryLabel}, history...)
	idx, err := fuzzyfinder.Find(options, func(i int) string {
		return options[i]
	}, fuzzyfinder.WithPromptString("Database> "))
	if err != nil {
		if errors.Is(err, fuzzyfinder.ErrAbort) {
			return "", errors.New("database selection aborted")
		}
		return "", fmt.Errorf("fuzzy finder: %w", err)
	}
	if options[idx] == manualEntryLabel {
		return readDatabaseLine()
	}
	return options[idx], nil
}

// readDatabaseLine reads a single non-empty line from stdin.
func readDatabaseLine() (string, error) {
	fmt.Fprint(os.Stderr, "Database name: ")
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("read database name: %w", err)
		}
		return "", errors.New("no database name provided")
	}
	name := strings.TrimSpace(scanner.Text())
	if name == "" {
		return "", errors.New("database name cannot be empty")
	}
	return name, nil
}
