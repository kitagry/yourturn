// Package watch tells when a file may have changed.
package watch

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// debounce merges a burst of events (truncate + write, tmp + rename) into one
// notification.
const debounce = 100 * time.Millisecond

// Watch notifies on the returned channel when path may have changed, until
// ctx ends. Notifications are coalesced: the channel holds at most one.
//
// The parent directory is watched rather than the file, because a file
// replaced by rename (how ACTIONS.md is usually rewritten) leaves a watch on
// the old inode deaf. A stat every poll backs fsnotify up for when it cannot
// watch (no inotify instance left, the directory does not exist yet, inotify
// watch limits, network file systems).
func Watch(ctx context.Context, path string, poll time.Duration) <-chan struct{} {
	path = filepath.Clean(path)
	out := make(chan struct{}, 1)
	notify := func() {
		select {
		case out <- struct{}{}:
		default:
		}
	}

	// Without a watcher (e.g. inotify instances used up) only polling runs.
	w, err := fsnotify.NewWatcher()
	if err != nil {
		w = nil
	}
	var events <-chan fsnotify.Event
	var errs <-chan error
	addWatch := func() {
		if w == nil {
			return
		}
		if err := w.Add(filepath.Dir(path)); err == nil {
			events, errs = w.Events, w.Errors
		}
	}
	addWatch()
	// Taken before returning, so a change right after Watch returns is seen.
	last := stat(path)

	go func() {
		defer func() {
			if w != nil {
				_ = w.Close()
			}
		}()
		ticker := time.NewTicker(poll)
		defer ticker.Stop()
		var timer *time.Timer
		var fire <-chan time.Time
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-events:
				if !ok {
					events = nil
					continue
				}
				if filepath.Clean(ev.Name) != path {
					continue
				}
				if timer == nil {
					timer = time.NewTimer(debounce)
				} else {
					timer.Reset(debounce)
				}
				fire = timer.C
			case _, ok := <-errs:
				if !ok {
					errs = nil
				}
			case <-fire:
				fire = nil
				last = stat(path)
				notify()
			case <-ticker.C:
				if events == nil {
					addWatch()
				}
				if now := stat(path); now != last {
					last = now
					notify()
				}
			}
		}
	}()
	return out
}

type fileState struct {
	exists  bool
	size    int64
	modTime time.Time
}

func stat(path string) fileState {
	fi, err := os.Stat(path)
	if err != nil {
		return fileState{}
	}
	return fileState{exists: true, size: fi.Size(), modTime: fi.ModTime()}
}
