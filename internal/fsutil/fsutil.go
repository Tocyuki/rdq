// Package fsutil holds the small file-system primitives shared by the
// state / history / schema stores: an atomic write helper and an
// inter-process lock.
//
// Several rdq processes can touch ~/.rdq at the same time (a TUI session,
// the GUI server, and `rdq exec` from a script). Each store does
// read-modify-write cycles on a whole file, so without a lock two processes
// can both read the old contents and the second rename silently discards
// the first one's change.
package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteFileAtomic writes data to path via a temp file in the same
// directory followed by a rename, so readers never observe a partially
// written file. The parent directory is created with 0700 and the file
// ends up with perm. The temp file is fsynced before the rename so a
// crash cannot leave a zero-length file behind the new name.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	fail := func(step string, err error) error {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("%s %s: %w", step, path, err)
	}
	if _, err := tmp.Write(data); err != nil {
		return fail("write", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		return fail("chmod", err)
	}
	if err := tmp.Sync(); err != nil {
		return fail("sync", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close %s: %w", path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename %s: %w", path, err)
	}
	return nil
}

// WithLock runs fn while holding an exclusive inter-process lock tied to
// path. The lock lives in a sidecar "<path>.lock" file (the data file
// itself is replaced by rename, so it cannot carry the lock). The lock is
// advisory: it only coordinates rdq processes, which is all we need.
func WithLock(path string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create directory for lock: %w", err)
	}
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open lock file: %w", err)
	}
	defer f.Close()
	if err := lockFile(f); err != nil {
		return fmt.Errorf("lock %s: %w", path, err)
	}
	defer unlockFile(f)
	return fn()
}
