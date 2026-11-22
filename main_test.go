package main

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePath(t *testing.T) {
	cwd := t.TempDir()
	taskDir := t.TempDir()
	home := t.TempDir()
	touch := func(p string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	env := map[string]string{"HERDR_TASK_DIR": taskDir}
	getenv := func(k string) string { return env[k] }

	if got := resolvePath([]string{"x.md"}, cwd, home, getenv); got != "x.md" {
		t.Errorf("argument: got %q", got)
	}
	// Nothing exists: wait for the file in the current directory.
	if got, want := resolvePath(nil, cwd, home, getenv), filepath.Join(cwd, "ACTIONS.md"); got != want {
		t.Errorf("none: got %q, want %q", got, want)
	}
	touch(filepath.Join(home, "tasks", "ACTIONS.md"))
	if got, want := resolvePath(nil, cwd, home, getenv), filepath.Join(home, "tasks", "ACTIONS.md"); got != want {
		t.Errorf("home tasks: got %q, want %q", got, want)
	}
	touch(filepath.Join(taskDir, "ACTIONS.md"))
	if got, want := resolvePath(nil, cwd, home, getenv), filepath.Join(taskDir, "ACTIONS.md"); got != want {
		t.Errorf("task dir: got %q, want %q", got, want)
	}
	touch(filepath.Join(cwd, "ACTIONS.md"))
	if got, want := resolvePath(nil, cwd, home, getenv), filepath.Join(cwd, "ACTIONS.md"); got != want {
		t.Errorf("cwd: got %q, want %q", got, want)
	}
}

func TestSentFile(t *testing.T) {
	if got := sentFile("/home/u", func(string) string { return "" }); got != "/home/u/.local/state/yourturn/sent.json" {
		t.Errorf("default: got %q", got)
	}
	if got := sentFile("/home/u", func(k string) string { return map[string]string{"XDG_STATE_HOME": "/s"}[k] }); got != "/s/yourturn/sent.json" {
		t.Errorf("XDG_STATE_HOME: got %q", got)
	}
}

func TestMouseFlags(t *testing.T) {
	tests := map[string]struct {
		env  string
		args []string
		want bool
	}{
		"unset is off":            {want: false},
		"env on":                  {env: "1", want: true},
		"env true":                {env: "true", want: true},
		"env off":                 {env: "0", want: false},
		"unknown env is off":      {env: "maybe", want: false},
		"no-mouse beats env":      {env: "1", args: []string{"--no-mouse"}, want: false},
		"mouse=false beats env":   {env: "1", args: []string{"--mouse=false"}, want: false},
		"mouse beats env":         {env: "0", args: []string{"--mouse"}, want: true},
		"no-mouse beats mouse":    {args: []string{"--mouse", "--no-mouse"}, want: false},
		"no-mouse=false is no-op": {env: "1", args: []string{"--no-mouse=false"}, want: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			fs := flag.NewFlagSet("yourturn", flag.ContinueOnError)
			mouse := mouseFlags(fs, tt.env)
			if err := fs.Parse(tt.args); err != nil {
				t.Fatal(err)
			}
			if got := mouse(); got != tt.want {
				t.Errorf("mouse() = %v, want %v", got, tt.want)
			}
		})
	}
}
