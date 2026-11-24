// Tests for yourturn-reply.sh, run with a temporary tasks root and a fake
// task-ask.sh.
package herdr

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

// replyInMessage is the " (reply <key>)" in the middle of a task-ask line.
var replyInMessage = regexp.MustCompile(` \(reply r-[0-9]{8}T[0-9]{6}-[0-9a-f]{4}\)`)

var replySuffix = regexp.MustCompile(` \(reply (r-[0-9]{8}T[0-9]{6}-[0-9a-f]{4})\)$`)

// inbox returns the inbox lines without their timestamps and without the
// " (reply <key>)" the script ends them with; rawInbox keeps it.
func (e env) inbox(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, l := range e.rawInbox(t) {
		out = append(out, replySuffix.ReplaceAllString(l, ""))
	}
	return out
}

func stripKeys(lines []string) []string {
	var out []string
	for _, l := range lines {
		out = append(out, replyInMessage.ReplaceAllString(l, ""))
	}
	return out
}

func (e env) rawInbox(t *testing.T) []string {
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
		{"patient-api-go#A266c", "D", master},
		{"A266c", "E", taskFile},
		{"M2a", "F", master},
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
		"patient-api-go\t[yourturn] human reply: patient-api-go#A266c D",
		"alpha\t[yourturn] human reply: alpha#A266c E",
		"master\t[yourturn] human reply: M2a F",
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
	if diff := cmp.Diff([]string{"alpha|[yourturn] 3 A|--no-wait"}, stripKeys(e.asks(t))); diff != "" {
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

// runStdout is run with the standard output apart from the errors.
func (e env) runStdout(t *testing.T, mode, id, text, file string) string {
	t.Helper()
	cmd := exec.Command("bash", "yourturn-reply.sh")
	cmd.Env = append(os.Environ(),
		"HERDR_TASKS_ROOT="+e.root,
		"HERDR_TASK_SCRIPTS="+e.scripts,
		"YOURTURN_HERDR_MODE="+mode,
		"YOURTURN_ID="+id,
		"YOURTURN_TEXT="+text,
		"YOURTURN_FILE="+file,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%v: %s", err, stderr.String())
	}
	return stdout.String()
}

// status returns the events of the reply status file as [key, state].
func (e env) status(t *testing.T) [][2]string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(e.root, ".reply-status"))
	if err != nil {
		return nil
	}
	var out [][2]string
	for _, l := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		f := strings.Split(l, "\t")
		if len(f) != 3 {
			t.Fatalf("status line %q is not <key>\\t<state>\\t<time>", l)
		}
		if !regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(Z|[+-]\d\d:\d\d)$`).MatchString(f[2]) {
			t.Errorf("time %q is not RFC 3339", f[2])
		}
		out = append(out, [2]string{f[0], f[1]})
	}
	return out
}

func TestReplyIsTrackedByKey(t *testing.T) {
	e := setup(t, "0")
	master := filepath.Join(e.root, "ACTIONS.md")
	out := e.runStdout(t, "inbox", "alpha#3", "A", master)
	m := regexp.MustCompile(`(?m)^receipt: (r-[0-9]{8}T[0-9]{6}-[0-9a-f]{4})$`).FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("no receipt line in stdout %q", out)
	}
	key := m[1]
	raw := e.rawInbox(t)
	if len(raw) != 1 || !strings.HasSuffix(raw[0], "human reply: alpha#3 A (reply "+key+")") {
		t.Errorf("inbox = %q, want it to end with the key %s", raw, key)
	}
	if diff := cmp.Diff([][2]string{{key, "sent"}}, e.status(t)); diff != "" {
		t.Errorf("status (-want +got):\n%s", diff)
	}

	// Every reply has a key of its own.
	out2 := e.runStdout(t, "inbox", "alpha#3", "A", master)
	if strings.Contains(out2, key) {
		t.Errorf("the key %s was used twice", key)
	}
}

func TestCoordinatorModeCarriesTheKeyToo(t *testing.T) {
	e := setup(t, "0")
	out := e.runStdout(t, "coordinator", "alpha#3", "A", filepath.Join(e.root, "ACTIONS.md"))
	key := strings.TrimPrefix(strings.TrimSpace(out), "receipt: ")
	asks := e.asks(t)
	if want := "alpha|[yourturn] 3 A (reply " + key + ")|--no-wait"; len(asks) != 1 || asks[0] != want {
		t.Errorf("task-ask = %q, want %q", asks, want)
	}
	raw := e.rawInbox(t)
	if len(raw) != 1 || !strings.HasSuffix(raw[0], "alpha#3 A (reply "+key+")") {
		t.Errorf("inbox = %q", raw)
	}
	if diff := cmp.Diff([][2]string{{key, "sent"}}, e.status(t)); diff != "" {
		t.Errorf("status (-want +got):\n%s", diff)
	}
}

func TestUndeliveredReplyIsNotTracked(t *testing.T) {
	e := setup(t, "4")
	cmd := exec.Command("bash", "yourturn-reply.sh")
	cmd.Env = append(os.Environ(), "HERDR_TASKS_ROOT="+e.root, "HERDR_TASK_SCRIPTS="+e.scripts,
		"YOURTURN_HERDR_MODE=coordinator", "YOURTURN_ID=alpha#3", "YOURTURN_TEXT=A",
		"YOURTURN_FILE="+filepath.Join(e.root, "ACTIONS.md"))
	out, err := cmd.Output()
	if err == nil || strings.Contains(string(out), "receipt") {
		t.Errorf("err=%v stdout=%q, want a failure without a receipt", err, out)
	}
	if e.status(t) != nil {
		t.Errorf("status written for a reply that was not delivered: %v", e.status(t))
	}
}
