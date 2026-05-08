// Package aictx persists per-(cluster, database) free-form context that is
// injected into Bedrock system prompts to improve NL→SQL accuracy.
//
// One file per (cluster, database) lives at
// ~/.rdq/aictx/<sha256(cluster:database)[:16]>.json (overridable via
// RDQ_AICTX_DIR). The on-disk schema is a single Context struct serialised
// as JSON; there is no automatic TTL.
package aictx

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MaxContentBytes caps the size of a stored context. The limit exists to keep
// Bedrock prompts within a reasonable budget even when stacked alongside a
// large schema snapshot. UTF-8 bytes, not runes.
const MaxContentBytes = 16384

// Context is the user-authored prompt context for a single (cluster, database)
// pair. Content is free-form text (Markdown is welcome) and is injected
// verbatim into the system prompt.
type Context struct {
	Cluster   string    `json:"cluster"`
	Database  string    `json:"database"`
	Content   string    `json:"content"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Load reads a saved context from disk. A missing file is not an error and
// returns (nil, nil); callers should treat that as "no context configured".
func Load(cluster, database string) (*Context, error) {
	path, err := contextPath(cluster, database)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read aictx %s: %w", path, err)
	}
	var c Context
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse aictx %s: %w", path, err)
	}
	return &c, nil
}

// LoadContent is a convenience wrapper that returns just the trimmed content
// for prompt injection, swallowing missing-file errors. Other I/O errors are
// returned so callers can log them.
func LoadContent(cluster, database string) (string, error) {
	c, err := Load(cluster, database)
	if err != nil {
		return "", err
	}
	if c == nil {
		return "", nil
	}
	return strings.TrimSpace(c.Content), nil
}

// Save writes a context to disk atomically. Empty content after trimming is
// rejected — call Delete instead to clear an entry.
//
// Save mutates c in place: Content is trimmed and UpdatedAt is stamped to
// the current UTC time before serialisation. Callers that render the
// post-save state (HTTP responses, TUI flash messages) can therefore
// re-use the same struct instead of re-loading from disk.
func Save(c *Context) error {
	if c == nil {
		return errors.New("nil context")
	}
	if c.Cluster == "" || c.Database == "" {
		return errors.New("cluster and database are required")
	}
	trimmed := strings.TrimSpace(c.Content)
	if trimmed == "" {
		return errors.New("content is empty; use Delete to remove")
	}
	if len(trimmed) > MaxContentBytes {
		return fmt.Errorf("content exceeds %d bytes", MaxContentBytes)
	}
	c.Content = trimmed
	c.UpdatedAt = time.Now().UTC()

	path, err := contextPath(c.Cluster, c.Database)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create aictx dir: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal aictx: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".aictx-*.json")
	if err != nil {
		return fmt.Errorf("create temp aictx file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp aictx file: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename aictx: %w", err)
	}
	return nil
}

// Delete removes the on-disk context for a (cluster, database) pair. Missing
// files are treated as success.
func Delete(cluster, database string) error {
	path, err := contextPath(cluster, database)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove aictx %s: %w", path, err)
	}
	return nil
}

// contextPath returns the on-disk path for the (cluster, database) context.
// The directory is RDQ_AICTX_DIR if set, otherwise ~/.rdq/aictx/.
func contextPath(cluster, database string) (string, error) {
	dir := os.Getenv("RDQ_AICTX_DIR")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		dir = filepath.Join(home, ".rdq", "aictx")
	}
	hash := sha256.Sum256([]byte(cluster + ":" + database))
	name := hex.EncodeToString(hash[:])[:16] + ".json"
	return filepath.Join(dir, name), nil
}
