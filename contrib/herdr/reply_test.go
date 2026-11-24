// Tests for yourturn-reply.sh, run with a temporary tasks root and a fake
// task-ask.sh.
package herdr

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

type env struct {
	root, scripts, askLog string
}

func setup(t *testing.T, askExit string) env {
	t.Helper()
	e := env{root: t.TempDir(), scripts: t.TempDir()}
	e.askLog = filepath.Join(e.scripts, "ask.log")
	fake := "#!/bin/sh\nprintf '%s|%s|%s\\n' \"$1\" \"$2\" \"$3\" >> " + e.askLog + "\nexit " + askExit + "\n"
	if err := os.WriteFile(filepath.Join(e.scripts, "task-ask.sh"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	return e
}

func (e env) run(t *testing.T, mode, id, text, file string) (string, error) {
	t.Helper()
	cmd := exec.Command("bash", "yourturn-reply.sh")
	cmd.Env = append(os.Environ(),
		"HERDR_TASKS_ROOT="+e.root,
		"HERDR_TASK_SCRIPTS="+e.scripts,
		"YOURTURN_HERDR_MODE="+mode,
		"YOURTURN_ID="+id,
		"YOURTURN_TEXT="+text,
		"YOURTURN_MESSAGE="+id+" "+text,
		"YOURTURN_FILE="+file,
	)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// inbox returns the inbox lines without their timestamps.
func (e env) inbox(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(e.root, ".inbox"))
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		_, rest, _ := strings.Cut(l, "\t")
		out = append(out, rest)
	}
	return out
}

func (e env) asks(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(e.askLog)
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
}

func TestInboxMode(t *testing.T) {
	e := setup(t, "0")
	master := filepath.Join(e.root, "ACTIONS.md")
	taskFile := filepath.Join(e.root, "alpha", "ACTIONS.md")
	for _, c := range []struct{ id, text, file string }{
		{"alpha#3", "A", master},
		{"M2", "OK", master},
		{"4", "自由文 です", taskFile},
		{"ci-maintenance#CI-9", "OK", master},
		{"A266", "B", taskFile},
		{"M12", "C", taskFile},
	} {
		if out, err := e.run(t, "inbox", c.id, c.text, c.file); err != nil {
			t.Fatalf("%s: %v: %s", c.id, err, out)
		}
	}
	want := []string{
		"alpha\t[yourturn] human reply: alpha#3 A",
		"master\t[yourturn] human reply: M2 OK",
		"alpha\t[yourturn] human reply: alpha#4 自由文 です",
		"ci-maintenance\t[yourturn] human reply: ci-maintenance#CI-9 OK",
		"alpha\t[yourturn] human reply: alpha#A266 B",
		"alpha\t[yourturn] human reply: alpha#M12 C",
	}
	if diff := cmp.Diff(want, e.inbox(t)); diff != "" {
		t.Errorf("inbox (-want +got):\n%s", diff)
	}
	if e.asks(t) != nil {
		t.Errorf("inbox mode asked a coordinator: %v", e.asks(t))
	}
}

func TestDefaultModeIsInbox(t *testing.T) {
	e := setup(t, "0")
	if out, err := e.run(t, "", "alpha#1", "B", filepath.Join(e.root, "ACTIONS.md")); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if len(e.inbox(t)) != 1 {
		t.Errorf("inbox = %v", e.inbox(t))
	}
}

func TestCoordinatorMode(t *testing.T) {
	e := setup(t, "0")
	if out, err := e.run(t, "coordinator", "alpha#3", "A", filepath.Join(e.root, "ACTIONS.md")); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if diff := cmp.Diff([]string{"alpha|[yourturn] 3 A|--no-wait"}, e.asks(t)); diff != "" {
		t.Errorf("task-ask (-want +got):\n%s", diff)
	}
	// master still hears about it.
	if diff := cmp.Diff([]string{"alpha\t[yourturn] human reply (already sent to the coordinator): alpha#3 A"}, e.inbox(t)); diff != "" {
		t.Errorf("inbox (-want +got):\n%s", diff)
	}
	// Master's own items have no coordinator: they go to the inbox.
	if out, err := e.run(t, "coordinator", "M1", "OK", filepath.Join(e.root, "ACTIONS.md")); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if len(e.asks(t)) != 1 || len(e.inbox(t)) != 2 {
		t.Errorf("M item: asks=%v inbox=%v", e.asks(t), e.inbox(t))
	}
}

func TestCoordinatorBlockedFails(t *testing.T) {
	e := setup(t, "4")
	out, err := e.run(t, "coordinator", "alpha#3", "A", filepath.Join(e.root, "ACTIONS.md"))
	if err == nil || !strings.Contains(out, "prompt") {
		t.Errorf("err=%v out=%q, want a failure about the prompt", err, out)
	}
	if e.inbox(t) != nil {
		t.Errorf("inbox written for a reply that was not delivered: %v", e.inbox(t))
	}
}

func TestUnknownTaskForBareNumberFails(t *testing.T) {
	e := setup(t, "0")
	out, err := e.run(t, "inbox", "3", "A", "/elsewhere/ACTIONS.md")
	if err == nil {
		t.Errorf("want an error for a bare number outside the tasks root, got %q", out)
	}
	if e.inbox(t) != nil {
		t.Errorf("inbox written: %v", e.inbox(t))
	}
}

func TestBareNumberWithMissingRootFails(t *testing.T) {
	e := setup(t, "0")
	e.root = filepath.Join(e.root, "missing")
	// Neither directory exists: they must not compare equal as "".
	if out, err := e.run(t, "coordinator", "3", "A", "/nonexistent/x/ACTIONS.md"); err == nil {
		t.Errorf("want an error, got %q", out)
	}
	if e.asks(t) != nil {
		t.Errorf("asked a coordinator of a guessed task: %v", e.asks(t))
	}
}
