package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/kitagry/yourturn/internal/actions"
)

// tallFile has items whose bodies are taller than a small screen.
func tallFile(title string, ids ...int) string {
	var b strings.Builder
	b.WriteString("# " + title + "\n\n")
	for _, i := range ids {
		fmt.Fprintf(&b, "- **🟢 alpha#%d Question %d?**\n", i, i)
		for j := range 12 {
			fmt.Fprintf(&b, "  - detail %d of %d\n", j, i)
		}
	}
	return b.String()
}

func reload(hs *harness, src string) {
	send(hs, loadedMsg{doc: actions.Parse(src), seq: hs.m.appliedSeq + 1})
}

func TestReloadKeepsScrollInsideOpenItem(t *testing.T) {
	hs := newTest(t, tallFile("v1", 1, 2, 3), 50, 8)
	send(hs, key("j"))
	send(hs, key("enter"))
	send(hs, key("j"))
	send(hs, key("j"))
	before := hs.m.offset
	beforeScreen := strings.Join(screen(hs)[1:7], "\n")
	reload(hs, tallFile("v2", 1, 2, 3))
	if hs.m.offset != before {
		t.Errorf("offset %d -> %d after reload", before, hs.m.offset)
	}
	if got := strings.Join(screen(hs)[1:7], "\n"); got != beforeScreen {
		t.Errorf("screen moved:\nbefore\n%s\nafter\n%s", beforeScreen, got)
	}
}

func TestReloadKeepsScrollWhenItemsAboveGo(t *testing.T) {
	hs := newTest(t, tallFile("v1", 1, 2, 3), 50, 8)
	send(hs, key("G"))
	send(hs, key("enter"))
	send(hs, key("j"))
	beforeScreen := strings.Join(screen(hs)[1:7], "\n")
	// Item 1 is answered: everything shifts up a line in the file.
	reload(hs, tallFile("v2", 2, 3))
	if got := strings.Join(screen(hs)[1:7], "\n"); got != beforeScreen {
		t.Errorf("screen moved:\nbefore\n%s\nafter\n%s", beforeScreen, got)
	}
	if cursorID(hs) != "alpha#3" {
		t.Errorf("cursor = %q", cursorID(hs))
	}
}

func TestEmptyReadKeepsWhatIsShown(t *testing.T) {
	hs := newTest(t, sample, 60, 20)
	send(hs, key("j"))
	send(hs, key("enter"))
	// A writer that truncates before writing can be caught in between.
	reload(hs, "")
	reload(hs, sample)
	if cursorID(hs) != "2" || !hs.m.expanded["2"] {
		t.Errorf("cursor = %q expanded=%v after an empty read", cursorID(hs), hs.m.expanded)
	}
	reload(hs, "")
	if !hs.m.doc.Parsed {
		t.Error("an empty read replaced the list")
	}
}

func TestStaleLoadIsIgnored(t *testing.T) {
	hs := newTest(t, sample, 60, 20)
	send(hs, loadedMsg{doc: actions.Parse("# new\n\n- **🔴 9 New?**\n"), seq: 5})
	send(hs, loadedMsg{doc: actions.Parse("# old\n\n- **🔴 8 Old?**\n"), seq: 4})
	if hs.m.doc.Title != "new" {
		t.Errorf("title = %q: an older read overwrote a newer one", hs.m.doc.Title)
	}
}

func TestReloadWhileTypingWaits(t *testing.T) {
	rh := newReplyTest(t, "")
	send(rh.harness, key("j"))
	send(rh.harness, key("R"))
	typeText(rh.harness, "hel")
	changed := strings.Replace(sample, "🔴 1 Urgent", "✅ 1 Urgent", 1)
	reload(rh.harness, changed)
	typeText(rh.harness, "lo")
	if v := rh.m.input.Value(); v != "hello" || !rh.m.inputting {
		t.Fatalf("input = %q inputting=%v: a reload disturbed the reply", v, rh.m.inputting)
	}
	if !strings.Contains(strings.Join(screen(rh.harness), "\n"), "Urgent question?") {
		t.Error("list changed under the reply being typed")
	}
	send(rh.harness, key("enter"))
	// The held reload is applied once the reply is queued.
	if strings.Contains(strings.Join(screen(rh.harness), "\n"), "Urgent question?") {
		t.Error("held reload not applied after the reply")
	}
	if cursorID(rh.harness) != "2" {
		t.Errorf("cursor = %q, want 2", cursorID(rh.harness))
	}
	if st := rh.m.replies["2"]; st.state != statePending || st.text != "hello" {
		t.Errorf("reply state = %+v", st)
	}
}

func TestReloadWhilePickingWaits(t *testing.T) {
	hs := newTest(t, sample, 60, 20)
	send(hs, key("j"))
	send(hs, key("o"))
	reload(hs, sample)
	if len(hs.m.picking) != 2 {
		t.Fatal("reload closed the link chooser")
	}
	send(hs, key("2"))
	if len(hs.opened) != 1 {
		t.Errorf("opened = %v", hs.opened)
	}
}

func TestStaleErrorIsIgnored(t *testing.T) {
	hs := newTest(t, sample, 60, 20)
	send(hs, loadedMsg{doc: actions.Parse(sample), seq: 5})
	send(hs, loadedMsg{err: errors.New("boom"), seq: 4})
	if hs.m.loadErr != nil {
		t.Errorf("loadErr = %v: an older failed read showed over a newer one", hs.m.loadErr)
	}
}

func TestErrorWhileTypingKeepsHeldRead(t *testing.T) {
	rh := newReplyTest(t, "")
	send(rh.harness, key("j"))
	send(rh.harness, key("R"))
	changed := strings.Replace(sample, "🔴 1 Urgent", "✅ 1 Urgent", 1)
	send(rh.harness, loadedMsg{doc: actions.Parse(changed), seq: 1})
	send(rh.harness, loadedMsg{err: errors.New("boom"), seq: 2})
	send(rh.harness, key("esc"))
	if strings.Contains(strings.Join(screen(rh.harness), "\n"), "Urgent question?") {
		t.Error("a failed read dropped the good read held while typing")
	}
}
