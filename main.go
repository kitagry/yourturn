// Command yourturn shows an ACTIONS.md, the list of what your agents are
// waiting on you for, in the terminal and keeps it up to date.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kitagry/yourturn/internal/reply"
	"github.com/kitagry/yourturn/internal/ui"
)

// version is set by goreleaser.
var version = "dev"

func main() {
	noMouse := flag.Bool("no-mouse", false, "leave the mouse to the terminal (click links directly; no wheel scrolling)")
	poll := flag.Duration("poll", 5*time.Second, "interval of the stat check that backs up file watching")
	replyCmd := flag.String("reply-cmd", os.Getenv("YOURTURN_REPLY_CMD"), "shell command that sends a reply (R); gets YOURTURN_ID, YOURTURN_TEXT, YOURTURN_MESSAGE, YOURTURN_FILE and the message on stdin (default $YOURTURN_REPLY_CMD)")
	replyDelay := flag.Duration("reply-delay", 5*time.Second, "how long a reply can be undone (u) before it is sent")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: yourturn [flags] [file]

Shows an ACTIONS.md and redraws it when the file changes. Without a file it
uses the first that exists of ./ACTIONS.md, $HERDR_TASK_DIR/ACTIONS.md and
~/tasks/ACTIONS.md.

With --reply-cmd, R replies to the selected item: the command gets the reply
in the environment and on stdin, and decides where it goes.

Flags:
`)
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}

	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	path := resolvePath(flag.Args(), cwd, home, os.Getenv)
	opts := ui.Options{Path: path, Open: openURL, NoMouse: *noMouse, Poll: *poll, ReplyDelay: *replyDelay, SentFile: sentFile(home, os.Getenv)}
	if *replyCmd != "" {
		opts.Reply = reply.Command{Shell: *replyCmd}.Send
	}
	m := ui.New(opts)
	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "yourturn:", err)
		os.Exit(1)
	}
}

func resolvePath(args []string, cwd, home string, getenv func(string) string) string {
	if len(args) > 0 {
		return args[0]
	}
	candidates := []string{filepath.Join(cwd, "ACTIONS.md")}
	if dir := getenv("HERDR_TASK_DIR"); dir != "" {
		candidates = append(candidates, filepath.Join(dir, "ACTIONS.md"))
	}
	if home != "" {
		candidates = append(candidates, filepath.Join(home, "tasks", "ACTIONS.md"))
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return candidates[0]
}

// sentFile is where the marks of sent replies are kept.
func sentFile(home string, getenv func(string) string) string {
	dir := getenv("XDG_STATE_HOME")
	if dir == "" {
		if home == "" {
			return ""
		}
		dir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(dir, "yourturn", "sent.json")
}

func openURL(url string) error {
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	return exec.Command(name, url).Run()
}
