package ui

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kitagry/yourturn/internal/reply"
)

// actionsFile is an aggregated ACTIONS.md whose heading changes on every
// rebuild, as the master's build script wrote it.
func actionsFile(n int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# todo — rebuild %d\n\n## 🟢\n", n)
	for i := 1; i <= 8; i++ {
		fmt.Fprintf(&b, "- **🟢 alpha#%d Question %d?**\n", i, i)
		for j := range 12 {
			fmt.Fprintf(&b, "  - detail %d of %d\n", j, i)
		}
	}
	return b.String()
}

// TestProgramSurvivesFileReplacedWhileReplying runs the real program, file
// watching included, while the file is replaced by rename over and over, and
// checks that keys keep working and that a reply typed meanwhile is sent
// whole.
func TestProgramSurvivesFileReplacedWhileReplying(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ACTIONS.md")
	if err := os.WriteFile(path, []byte(actionsFile(0)), 0o644); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var sent []reply.Reply
	m := New(Options{
		Path:       path,
		Poll:       50 * time.Millisecond,
		ReplyDelay: 100 * time.Millisecond,
		Reply: func(_ context.Context, r reply.Reply) error {
			mu.Lock()
			defer mu.Unlock()
			sent = append(sent, r)
			return nil
		},
	})
	p := tea.NewProgram(m,
		tea.WithContext(t.Context()),
		tea.WithInput(nil),
		tea.WithOutput(io.Discard),
		tea.WithWindowSize(50, 8),
		tea.WithoutSignalHandler(),
	)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		for n := 1; ; n++ {
			select {
			case <-stop:
				return
			case <-time.After(20 * time.Millisecond):
			}
			tmp := filepath.Join(dir, "ACTIONS.md.tmp")
			if err := os.WriteFile(tmp, []byte(actionsFile(n)), 0o644); err != nil {
				t.Error(err)
				return
			}
			if err := os.Rename(tmp, path); err != nil {
				t.Error(err)
				return
			}
		}
	})

	done := make(chan tea.Model, 1)
	go func() {
		final, err := p.Run()
		if err != nil {
			t.Error(err)
		}
		done <- final
	}()

	slow := func(keys ...string) {
		for _, k := range keys {
			time.Sleep(40 * time.Millisecond)
			p.Send(key(k))
		}
	}
	time.Sleep(200 * time.Millisecond)
	// Open item 3, taller than the screen, and scroll into its body: j
	// scrolls before it moves on. Then reply to it.
	slow("j", "j", "enter", "j", "j")
	time.Sleep(200 * time.Millisecond)
	slow("R")
	slow(strings.Split("hello world", "")...)
	slow("enter")
	time.Sleep(500 * time.Millisecond)
	close(stop)
	wg.Wait()
	time.Sleep(300 * time.Millisecond)
	slow("q", "q")

	var final *Model
	select {
	case fm := <-done:
		final = fm.(*Model)
	case <-time.After(5 * time.Second):
		p.Kill()
		// Run's goroutine reports through t; let it finish before the test does.
		<-done
		t.Fatal("program did not quit: keys stopped being handled")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(sent) != 1 || sent[0].ID != "alpha#3" || sent[0].Text != "hello world" {
		t.Errorf("sent = %+v, want one reply \"hello world\" to alpha#3", sent)
	}
	if it, _ := final.current(); it.ID != "alpha#3" {
		t.Errorf("cursor on %q, want alpha#3", it.ID)
	}
	if !final.expanded["alpha#3"] {
		t.Error("alpha#3 lost its expansion")
	}
	// enter shows the item from its title (offset 3, below the heading and
	// items 1 and 2); the two j scroll 2 lines into it.
	if final.offset != 5 {
		t.Errorf("offset = %d, want 5: a reload moved the scroll position", final.offset)
	}
	if !strings.Contains(final.doc.Title, "rebuild") || final.doc.Title == "todo — rebuild 0" {
		t.Errorf("never reloaded: title %q", final.doc.Title)
	}
}
