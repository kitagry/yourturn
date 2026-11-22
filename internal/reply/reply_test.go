package reply

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestMessage(t *testing.T) {
	if got := (Reply{ID: "alpha#3", Text: "A"}).Message(); got != "alpha#3 A" {
		t.Errorf("Message() = %q", got)
	}
}

func TestCommandPassesReplyAsEnvAndStdin(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out")
	// The reply text holds shell syntax: it must arrive as data, not run.
	cmd := Command{Shell: `printf '%s|%s|%s|%s|' "$YOURTURN_ID" "$YOURTURN_TEXT" "$YOURTURN_MESSAGE" "$YOURTURN_FILE" > "$OUT"; cat >> "$OUT"`}
	t.Setenv("OUT", out)
	r := Reply{ID: "alpha#3", Text: `B $(touch pwned) "q"`, File: "/tmp/tasks/ACTIONS.md"}
	if err := cmd.Send(t.Context(), r); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	want := `alpha#3|B $(touch pwned) "q"|alpha#3 B $(touch pwned) "q"|/tmp/tasks/ACTIONS.md|alpha#3 B $(touch pwned) "q"` + "\n"
	if diff := cmp.Diff(want, string(b)); diff != "" {
		t.Errorf("command saw (-want +got):\n%s", diff)
	}
	if _, err := os.Stat("pwned"); err == nil {
		_ = os.Remove("pwned")
		t.Error("reply text was run by the shell")
	}
}

func TestCommandFailureCarriesStderr(t *testing.T) {
	cmd := Command{Shell: `echo "first" >&2; echo "coordinator is blocked" >&2; exit 4`}
	err := cmd.Send(t.Context(), Reply{ID: "1", Text: "OK"})
	if err == nil || !strings.Contains(err.Error(), "coordinator is blocked") {
		t.Errorf("err = %v, want the last stderr line", err)
	}
}

func TestCommandTimeout(t *testing.T) {
	cmd := Command{Shell: "sleep 5", Timeout: 100 * time.Millisecond}
	start := time.Now()
	if err := cmd.Send(t.Context(), Reply{ID: "1", Text: "OK"}); err == nil {
		t.Error("want a timeout error")
	}
	if time.Since(start) > 3*time.Second {
		t.Error("timeout did not stop the command")
	}
}
