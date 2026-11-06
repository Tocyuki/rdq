package fsutil

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
)

func TestWriteFileAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "f.json")
	if err := WriteFileAtomic(path, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(path, []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "two" {
		t.Fatalf("content = %q, %v", got, err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("perm = %v, want 0600", info.Mode().Perm())
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("temp files left behind: %v", entries)
	}
}

// TestWithLockSerializesReadModifyWrite runs many concurrent increments
// through separate file handles; without mutual exclusion updates are lost.
func TestWithLockSerializesReadModifyWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "counter")
	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := WithLock(path, func() error {
				data, _ := os.ReadFile(path)
				v, _ := strconv.Atoi(string(data))
				return WriteFileAtomic(path, []byte(strconv.Itoa(v+1)), 0o600)
			})
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	data, _ := os.ReadFile(path)
	if string(data) != strconv.Itoa(n) {
		t.Fatalf("counter = %s, want %d (lost updates)", data, n)
	}
}
