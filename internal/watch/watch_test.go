package watch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func waitChange(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatalf("no change notified after %s", what)
	}
}

func drain(ch <-chan struct{}) {
	for {
		select {
		case <-ch:
		case <-time.After(300 * time.Millisecond):
			return
		}
	}
}

func write(t *testing.T, path, s string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWatchNotifiesOnFileEvents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ACTIONS.md")
	write(t, path, "a")
	// A long poll so only fsnotify can deliver within the test's timeout.
	ch := Watch(t.Context(), path, time.Hour)

	write(t, path, "b")
	waitChange(t, ch, "write")
	drain(ch)

	tmp := filepath.Join(dir, "ACTIONS.md.tmp")
	write(t, tmp, "c")
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
	waitChange(t, ch, "tmp + rename")
	drain(ch)

	// The watch must survive the rename above (it watches the directory).
	write(t, path, "d")
	waitChange(t, ch, "write after rename")
	drain(ch)

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	waitChange(t, ch, "remove")
	drain(ch)
	write(t, path, "e")
	waitChange(t, ch, "re-create")
}

func TestWatchIgnoresOtherFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ACTIONS.md")
	write(t, path, "a")
	ch := Watch(t.Context(), path, time.Hour)
	write(t, filepath.Join(dir, "TASK.md"), "x")
	select {
	case <-ch:
		t.Fatal("notified for another file")
	case <-time.After(500 * time.Millisecond):
	}
}

func TestWatchPollsWhenDirectoryAppearsLater(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "later")
	path := filepath.Join(dir, "ACTIONS.md")
	ch := Watch(t.Context(), path, 50*time.Millisecond)
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, path, "a")
	waitChange(t, ch, "creating the file in a new directory")
}
