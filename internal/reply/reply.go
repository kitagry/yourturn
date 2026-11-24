// Package reply hands a reply to an item over to a user-configured command,
// which decides where it goes (an agent's pane, a chat, a file...).
package reply

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Reply struct {
	// ID is the item's ID as written in the file ("3", "M2", "task#3").
	ID   string
	Text string
	// File is the ACTIONS.md the item is in, so the command can tell which
	// task a bare number belongs to.
	File string
}

// Message is how a human would write the reply: "<ID> <text>".
func (r Reply) Message() string {
	return r.ID + " " + r.Text
}

// Command runs a shell command per reply.
type Command struct {
	// Shell is run with `sh -c`. The reply is passed in the environment
	// (YOURTURN_ID, YOURTURN_TEXT, YOURTURN_MESSAGE, YOURTURN_FILE) and as
	// the message on stdin, never spliced into the command line, so its text
	// cannot run as shell code.
	Shell string
	// Timeout bounds one run; 0 means DefaultTimeout.
	Timeout time.Duration
}

const DefaultTimeout = 2 * time.Minute

// Send runs the command. A "receipt: <key>" line on its stdout names the
// reply for whatever tracks it later (see ui.Options.ReplyStatus); the last
// one is returned, "" when there is none.
func (c Command) Send(ctx context.Context, r Reply) (string, error) {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", c.Shell)
	cmd.Env = append(os.Environ(),
		"YOURTURN_ID="+r.ID,
		"YOURTURN_TEXT="+r.Text,
		"YOURTURN_MESSAGE="+r.Message(),
		"YOURTURN_FILE="+r.File,
	)
	cmd.Stdin = strings.NewReader(r.Message() + "\n")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	// Children the shell started must not keep the pipes open past the
	// timeout.
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("reply command timed out after %s", timeout)
		}
		if line := lastLine(stderr.String()); line != "" {
			return "", fmt.Errorf("%s (%w)", line, err)
		}
		return "", err
	}
	return receipt(stdout.String()), nil
}

const receiptPrefix = "receipt: "

func receipt(stdout string) string {
	key := ""
	for _, l := range strings.Split(stdout, "\n") {
		k, ok := strings.CutPrefix(strings.TrimRight(l, "\r"), receiptPrefix)
		if ok && k != "" && !strings.ContainsAny(k, " \t") {
			key = k
		}
	}
	return key
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
