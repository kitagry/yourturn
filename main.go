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

	"github.com/kitagry/yourturn/internal/ui"
)

// version is set by goreleaser.
var version = "dev"

func main() {
	noMouse := flag.Bool("no-mouse", false, "leave the mouse to the terminal (click links directly; no wheel scrolling)")
	poll := flag.Duration("poll", 5*time.Second, "interval of the stat check that backs up file watching")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: yourturn [flags] [file]

Shows an ACTIONS.md and redraws it when the file changes. Without a file it
uses the first that exists of ./ACTIONS.md, $HERDR_TASK_DIR/ACTIONS.md and
~/tasks/ACTIONS.md.

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
	m := ui.New(ui.Options{Path: path, Open: openURL, NoMouse: *noMouse, Poll: *poll})
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

func openURL(url string) error {
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	return exec.Command(name, url).Run()
}
