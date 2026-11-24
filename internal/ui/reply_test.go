package ui

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/google/go-cmp/cmp"

	"github.com/kitagry/yourturn/internal/actions"
	"github.com/kitagry/yourturn/internal/reply"
)

type replyHarness struct {
	*harness
	sent []reply.Reply
	fail error
}

func newReplyTest(t *testing.T, sentFile string) *replyHarness {
	t.Helper()
	rh := &replyHarness{harness: &harness{}}
	rh.m = New(Options{
		Path:       "/tmp/tasks/ACTIONS.md",
		ReplyDelay: 5 * time.Second,
		SentFile:   sentFile,
		Reply: func(_ context.Context, r reply.Reply) error {
			rh.sent = append(rh.sent, r)
			return rh.fail
		},
		Now: func() time.Time { return time.Date(2026, 11, 22, 15, 20, 0, 0, time.Local) },
	})
	send(rh.harness, tea.WindowSizeMsg{Width: 60, Height: 20})
	send(rh.harness, loadedMsg{doc: actions.Parse(sample)})
	return rh
}

func typeText(hs *harness, s string) {
	for _, r := range s {
		send(hs, key(string(r)))
	}
}

// fire delivers the delayed send of the item's pending reply, then the
// result of running the command, as the Bubble Tea runtime would.
func fire(t *testing.T, rh *replyHarness, k string) {
	t.Helper()
	st, ok := rh.m.replies[k]
	if !ok || st.state != statePending {
		t.Fatalf("item %s has no pending reply: %+v", k, st)
	}
	send(rh.harness, sendDueMsg{key: k, seq: st.seq})
	if rh.lastCmd == nil {
		t.Fatal("due reply returned no command")
	}
	send(rh.harness, rh.lastCmd())
}

func TestReplySendsAfterDelay(t *testing.T) {
	rh := newReplyTest(t, "")
	send(rh.harness, key("R"))
	if !strings.Contains(strings.Join(screen(rh.harness), "\n"), "reply to 1") {
		t.Fatalf("no reply prompt:\n%s", strings.Join(screen(rh.harness), "\n"))
	}
	// Keys go to the input, not to navigation.
	typeText(rh.harness, "jA")
	send(rh.harness, key("enter"))
	if cursorID(rh.harness) != "1" {
		t.Errorf("typing moved the cursor to %q", cursorID(rh.harness))
	}
	if len(rh.sent) != 0 {
		t.Fatal("sent before the undo delay")
	}
	if !strings.Contains(strings.Join(screen(rh.harness), "\n"), "u to undo") {
		t.Errorf("pending reply not shown:\n%s", strings.Join(screen(rh.harness), "\n"))
	}
	fire(t, rh, "1")
	want := []reply.Reply{{ID: "1", Text: "jA", File: "/tmp/tasks/ACTIONS.md"}}
	if diff := cmp.Diff(want, rh.sent); diff != "" {
		t.Errorf("sent (-want +got):\n%s", diff)
	}
	got := strings.Join(screen(rh.harness), "\n")
	if !strings.Contains(got, `sent "jA" 15:20`) {
		t.Errorf("sent mark not shown:\n%s", got)
	}
}

func TestReplyUndo(t *testing.T) {
	rh := newReplyTest(t, "")
	send(rh.harness, key("R"))
	typeText(rh.harness, "B")
	send(rh.harness, key("enter"))
	seq := rh.m.replies["1"].seq
	send(rh.harness, key("j"))
	// u undoes the latest pending reply even after moving away.
	send(rh.harness, key("u"))
	send(rh.harness, sendDueMsg{key: "1", seq: seq})
	if rh.lastCmd != nil {
		t.Error("undone reply still runs")
	}
	if len(rh.sent) != 0 {
		t.Errorf("undone reply was sent: %v", rh.sent)
	}
	if _, ok := rh.m.replies["1"]; ok {
		t.Error("undone reply still marked")
	}
}

func TestReplyEscAndEmptyCancel(t *testing.T) {
	rh := newReplyTest(t, "")
	send(rh.harness, key("R"))
	typeText(rh.harness, "x")
	send(rh.harness, key("esc"))
	send(rh.harness, key("R"))
	send(rh.harness, key("enter"))
	if len(rh.m.replies) != 0 {
		t.Errorf("cancelled replies are pending: %v", rh.m.replies)
	}
	// After cancelling, keys navigate again.
	send(rh.harness, key("j"))
	if cursorID(rh.harness) != "2" {
		t.Errorf("cursor = %q, want 2", cursorID(rh.harness))
	}
}

func TestReplyFailureKeepsTextForRetry(t *testing.T) {
	rh := newReplyTest(t, "")
	rh.fail = errors.New("coordinator is blocked")
	send(rh.harness, key("R"))
	typeText(rh.harness, "OK")
	send(rh.harness, key("enter"))
	fire(t, rh, "1")
	got := strings.Join(screen(rh.harness), "\n")
	if !strings.Contains(got, "failed") || !strings.Contains(got, "coordinator is blocked") {
		t.Errorf("failure not shown:\n%s", got)
	}
	send(rh.harness, key("R"))
	if v := rh.m.input.Value(); v != "OK" {
		t.Errorf("retry input = %q, want the failed text", v)
	}
}

func TestReplyWithoutCommandOrID(t *testing.T) {
	hs := newTest(t, sample, 60, 20)
	send(hs, key("R"))
	if hs.m.inputting || !strings.Contains(hs.m.status, "--reply-cmd") {
		t.Errorf("replies without a command: inputting=%v status=%q", hs.m.inputting, hs.m.status)
	}
	rh := newReplyTest(t, "")
	send(rh.harness, loadedMsg{doc: actions.Parse("- **🔴 No number?**\n")})
	send(rh.harness, key("R"))
	if rh.m.inputting || !strings.Contains(rh.m.status, "no number") {
		t.Errorf("item without ID: inputting=%v status=%q", rh.m.inputting, rh.m.status)
	}
}

func TestQuitWithPendingReplyAsksAgain(t *testing.T) {
	rh := newReplyTest(t, "")
	send(rh.harness, key("R"))
	typeText(rh.harness, "A")
	send(rh.harness, key("enter"))
	send(rh.harness, key("q"))
	if rh.lastCmd != nil {
		if _, ok := rh.lastCmd().(tea.QuitMsg); ok {
			t.Fatal("quit while a reply is pending")
		}
	}
	send(rh.harness, key("q"))
	if rh.lastCmd == nil {
		t.Fatal("second q did not quit")
	}
	if _, ok := rh.lastCmd().(tea.QuitMsg); !ok {
		t.Error("second q did not quit")
	}
}

func TestSentMarksPersist(t *testing.T) {
	file := filepath.Join(t.TempDir(), "sent.json")
	rh := newReplyTest(t, file)
	send(rh.harness, key("R"))
	typeText(rh.harness, "A")
	send(rh.harness, key("enter"))
	fire(t, rh, "1")

	again := newReplyTest(t, file)
	if !strings.Contains(strings.Join(screen(again.harness), "\n"), `sent "A" 15:20`) {
		t.Errorf("sent mark lost after restart:\n%s", strings.Join(screen(again.harness), "\n"))
	}
	// Marks are per file: the same ID in another file is not marked.
	other := &replyHarness{harness: &harness{}}
	other.m = New(Options{Path: "/tmp/other/ACTIONS.md", SentFile: file, Reply: func(context.Context, reply.Reply) error { return nil }})
	send(other.harness, tea.WindowSizeMsg{Width: 60, Height: 20})
	send(other.harness, loadedMsg{doc: actions.Parse(sample)})
	if strings.Contains(strings.Join(screen(other.harness), "\n"), "sent") {
		t.Error("mark leaked to another file")
	}
}

func TestUndoBringsBackTheEarlierMark(t *testing.T) {
	rh := newReplyTest(t, "")
	send(rh.harness, key("R"))
	typeText(rh.harness, "A")
	send(rh.harness, key("enter"))
	fire(t, rh, "1")
	send(rh.harness, key("R"))
	typeText(rh.harness, "B")
	send(rh.harness, key("enter"))
	send(rh.harness, key("u"))
	if got := strings.Join(screen(rh.harness), "\n"); !strings.Contains(got, `sent "A" 15:20`) {
		t.Errorf("earlier sent mark lost by undo:\n%s", got)
	}
}

func TestNoReplyWhileOneIsBeingSent(t *testing.T) {
	rh := newReplyTest(t, "")
	send(rh.harness, key("R"))
	typeText(rh.harness, "A")
	send(rh.harness, key("enter"))
	send(rh.harness, sendDueMsg{key: "1", seq: rh.m.replies["1"].seq})
	send(rh.harness, key("R"))
	if rh.m.inputting || !strings.Contains(rh.m.status, "being sent") {
		t.Errorf("reply while sending: inputting=%v status=%q", rh.m.inputting, rh.m.status)
	}
}

func TestExpiredMarksAreNotShown(t *testing.T) {
	file := filepath.Join(t.TempDir(), "sent.json")
	old := time.Date(2026, 11, 1, 15, 20, 0, 0, time.Local)
	if err := saveSent(file, "/tmp/tasks/ACTIONS.md", "1", sentEntry{Text: "A", At: old}, old); err != nil {
		t.Fatal(err)
	}
	rh := newReplyTest(t, file)
	if _, ok := rh.m.replies["1"]; ok {
		t.Error("a mark older than the TTL is shown")
	}
}

func TestReplyToLetteredID(t *testing.T) {
	rh := newReplyTest(t, "")
	send(rh.harness, loadedMsg{doc: actions.Parse("- **🔴 ci-maintenance#CI-9 Fix CI?**\n")})
	send(rh.harness, key("R"))
	typeText(rh.harness, "OK")
	send(rh.harness, key("enter"))
	fire(t, rh, "ci-maintenance#CI-9")
	want := []reply.Reply{{ID: "ci-maintenance#CI-9", Text: "OK", File: "/tmp/tasks/ACTIONS.md"}}
	if diff := cmp.Diff(want, rh.sent); diff != "" {
		t.Errorf("sent (-want +got):\n%s", diff)
	}
}
