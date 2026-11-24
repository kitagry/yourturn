package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/google/go-cmp/cmp"

	"github.com/kitagry/yourturn/internal/actions"
)

const receipt = "r-20261124T152000-a1b2"

// statusLine is one event of the reply status file at hh:mm local time.
func statusLine(key, state string, hh, mm int) string {
	at := time.Date(2026, 11, 24, hh, mm, 0, 0, time.Local)
	return key + "\t" + state + "\t" + at.Format(time.RFC3339) + "\n"
}

func writeStatus(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.Join(lines, "")), 0o600); err != nil {
		t.Fatal(err)
	}
}

// newStatusTest is a reply test whose reply command prints a receipt and
// that watches a reply status file.
func newStatusTest(t *testing.T, sentFile, statusFile string) *replyHarness {
	t.Helper()
	rh := &replyHarness{harness: &harness{}, receipt: receipt}
	rh.m = newReplyModel(rh, sentFile, statusFile)
	send(rh.harness, tea.WindowSizeMsg{Width: 70, Height: 20})
	send(rh.harness, loadedMsg{doc: actions.Parse(sample)})
	return rh
}

func replyTo(t *testing.T, rh *replyHarness, k, text string) {
	t.Helper()
	send(rh.harness, key("R"))
	typeText(rh.harness, text)
	send(rh.harness, key("enter"))
	fire(t, rh, k)
}

func shown(hs *harness) string { return strings.Join(screen(hs), "\n") }

func TestStatusIsShownNextToTheSentMark(t *testing.T) {
	dir := t.TempDir()
	status := filepath.Join(dir, ".reply-status")
	writeStatus(t, status, statusLine(receipt, "sent", 15, 20))
	rh := newStatusTest(t, "", status)
	replyTo(t, rh, "1", "A")
	if got := shown(rh.harness); !strings.Contains(got, `↳ sent "A" 15:20`) || strings.Contains(got, "relayed") {
		t.Fatalf("before relay:\n%s", got)
	}

	writeStatus(t, status,
		statusLine(receipt, "sent", 15, 20),
		statusLine(receipt, "relayed", 15, 32),
		statusLine("r-other", "acked", 15, 33),
	)
	send(rh.harness, replyStatusChangedMsg{})
	if got := shown(rh.harness); !strings.Contains(got, `↳ sent "A" 15:20 · relayed 15:32`) || strings.Contains(got, "acked") {
		t.Errorf("relayed:\n%s", got)
	}

	writeStatus(t, status,
		statusLine(receipt, "sent", 15, 20),
		statusLine(receipt, "relayed", 15, 32),
		statusLine(receipt, "acked", 15, 40),
	)
	send(rh.harness, replyStatusChangedMsg{})
	if got := shown(rh.harness); !strings.Contains(got, `↳ sent "A" 15:20 · relayed 15:32 · acked 15:40`) {
		t.Errorf("acked:\n%s", got)
	}
}

func TestStatusWithoutRelayedStillShowsAcked(t *testing.T) {
	// Replies to the master's own items are acked without being relayed.
	status := filepath.Join(t.TempDir(), ".reply-status")
	writeStatus(t, status, statusLine(receipt, "acked", 15, 40))
	rh := newStatusTest(t, "", status)
	replyTo(t, rh, "1", "A")
	if got := shown(rh.harness); !strings.Contains(got, `↳ sent "A" 15:20 · acked 15:40`) {
		t.Errorf("screen:\n%s", got)
	}
}

func TestReadReplyStatusIgnoresWhatItDoesNotKnow(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".reply-status")
	writeStatus(t, path,
		"garbage\n",
		"\n",
		"r-1\tsent\tnot-a-time\n",
		"r-1\tteleported\t2026-11-24T15:30:00+09:00\n",
		"r-1\trelayed\t2026-11-24T15:32:00+09:00\textra\n",
		"r-1\tacked\t2026-11-24T15:40:00+09:00\n",
		// An earlier event of the same state is kept.
		"r-1\tacked\t2026-11-24T15:50:00+09:00\n",
	)
	got := readReplyStatus(path)
	// The line with a fourth field is skipped too.
	want := replyEvents{"r-1": {"acked": time.Date(2026, 11, 24, 6, 40, 0, 0, time.UTC)}}
	if diff := cmp.Diff(want, got, cmp.Comparer(time.Time.Equal)); diff != "" {
		t.Errorf("events (-want +got):\n%s", diff)
	}
	if got := readReplyStatus(filepath.Join(t.TempDir(), "missing")); len(got) != 0 {
		t.Errorf("missing file gave %v", got)
	}
}

func TestReceiptSurvivesARestart(t *testing.T) {
	dir := t.TempDir()
	sent := filepath.Join(dir, "sent.json")
	status := filepath.Join(dir, ".reply-status")
	rh := newStatusTest(t, sent, status)
	replyTo(t, rh, "1", "A")

	writeStatus(t, status, statusLine(receipt, "relayed", 15, 32))
	again := newStatusTest(t, sent, status)
	send(again.harness, loadedMsg{doc: actions.Parse(sample)})
	if got := shown(again.harness); !strings.Contains(got, `↳ sent "A" 15:20 · relayed 15:32`) {
		t.Errorf("after restart:\n%s", got)
	}
}

func TestDoneItemsLoseTheirMark(t *testing.T) {
	for _, c := range []struct{ name, doc string }{
		{"checked", "- **✅ 1 Urgent question?**\n- **🟠 2 Weekly question?**\n"},
		{"gone", "- **🟠 2 Weekly question?**\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			sent := filepath.Join(dir, "sent.json")
			status := filepath.Join(dir, ".reply-status")
			rh := newStatusTest(t, sent, status)
			replyTo(t, rh, "1", "A")
			rh.m.replies["2"] = replyState{state: stateSent, text: "B", at: rh.m.now()} // no receipt: stays

			send(rh.harness, loadedMsg{doc: actions.Parse(c.doc), seq: 5})
			if c.name == "gone" {
				// One read without the item may be a file mid-rewrite.
				if _, ok := rh.m.replies["1"]; !ok {
					t.Fatal("the mark was dropped after one read without the item")
				}
				send(rh.harness, loadedMsg{doc: actions.Parse(c.doc), seq: 6})
			}
			if _, ok := rh.m.replies["1"]; ok {
				t.Error("the mark of a done item is kept")
			}
			if _, ok := rh.m.replies["2"]; !ok {
				t.Error("the mark of an item still open was dropped")
			}
			if _, ok := readSent(sent)[rh.m.path]["1"]; ok {
				t.Error("the mark of a done item is kept in the file")
			}
		})
	}
}

func TestDoneKeepsMarksWithoutReceipt(t *testing.T) {
	// Without a receipt nothing tells the reply apart from an older one, so
	// the marks behave as they always did.
	rh := newReplyTest(t, "")
	replyTo(t, rh, "1", "A")
	send(rh.harness, loadedMsg{doc: actions.Parse("- **✅ 1 Urgent question?**\n"), seq: 5})
	if _, ok := rh.m.replies["1"]; !ok {
		t.Error("a mark without a receipt was dropped")
	}
}

func TestNoStatusFileShowsTheSameAsBefore(t *testing.T) {
	rh := newStatusTest(t, "", "")
	replyTo(t, rh, "1", "A")
	got := shown(rh.harness)
	if !strings.Contains(got, `↳ sent "A" 15:20`) || strings.Contains(got, "relayed") {
		t.Errorf("screen:\n%s", got)
	}
}

func TestReplyStatusFileIsWatched(t *testing.T) {
	status := filepath.Join(t.TempDir(), ".reply-status")
	m := New(Options{Path: "/tmp/tasks/ACTIONS.md", ReplyStatus: status, Poll: 50 * time.Millisecond})
	m.Init()
	writeStatus(t, status, statusLine(receipt, "acked", 15, 40))
	done := make(chan tea.Msg, 1)
	go func() { done <- m.waitReplyStatus()() }()
	select {
	case msg := <-done:
		if _, ok := msg.(replyStatusChangedMsg); !ok {
			t.Errorf("got %T", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a change of the status file was not noticed")
	}
}
