//go:build windows

package fsutil

import (
	"os"

	"golang.org/x/sys/windows"
)

// Lock the first byte range; any fixed range works as long as every
// caller uses the same one.
const lockBytes = 1

func lockFile(f *os.File) error {
	ol := new(windows.Overlapped)
	return windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, lockBytes, 0, ol)
}

func unlockFile(f *os.File) error {
	ol := new(windows.Overlapped)
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, lockBytes, 0, ol)
}
