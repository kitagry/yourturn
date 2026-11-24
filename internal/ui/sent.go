package ui

import (
	"encoding/json/v2"
	"os"
	"path/filepath"
	"time"
)

// sentTTL is how long a sent mark is kept; items are usually answered and
// gone well before.
const sentTTL = 14 * 24 * time.Hour

type sentEntry struct {
	Text string    `json:"text"`
	At   time.Time `json:"at"`
	// Receipt names the reply in the reply status file; empty when the reply
	// command did not give one.
	Receipt string `json:"receipt,omitempty"`
}

// sentFile maps an ACTIONS.md's absolute path to its items' sent replies,
// by item key, so the marks survive restarting the viewer.
type sentFile map[string]map[string]sentEntry

func readSent(path string) sentFile {
	f := sentFile{}
	b, err := os.ReadFile(path)
	if err != nil {
		return f
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return sentFile{}
	}
	return f
}

// saveSent records one sent reply, dropping expired ones. It rereads the
// file first so viewers of other files sharing it are not overwritten.
func saveSent(path, actionsFile, key string, e sentEntry, now time.Time) error {
	return updateSent(path, now, func(f sentFile) {
		if f[actionsFile] == nil {
			f[actionsFile] = map[string]sentEntry{}
		}
		f[actionsFile][key] = e
	})
}

// forgetSent drops the marks of the given item keys of one ACTIONS.md.
func forgetSent(path, actionsFile string, keys []string, now time.Time) error {
	return updateSent(path, now, func(f sentFile) {
		for _, k := range keys {
			delete(f[actionsFile], k)
		}
	})
}

// updateSent rereads the file, applies edit and writes it back.
func updateSent(path string, now time.Time, edit func(sentFile)) error {
	f := readSent(path)
	edit(f)
	for file, entries := range f {
		for k, old := range entries {
			if now.Sub(old.At) > sentTTL {
				delete(entries, k)
			}
		}
		if len(entries) == 0 {
			delete(f, file)
		}
	}
	b, err := json.Marshal(f, json.Deterministic(true))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	// A temp file of its own: viewers of other files may save at the same
	// time.
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	// Cleans up when a step fails; after the rename there is nothing to remove.
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
