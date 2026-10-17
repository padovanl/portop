package notify

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func TestEscapeAppleScript(t *testing.T) {
	cases := map[string]string{
		`hello`:        `hello`,
		`say "hi"`:     `say \"hi\"`,
		`back\slash`:   `back\\slash`,
		`both " and \`: `both \" and \\`,
	}
	for in, want := range cases {
		if got := escapeAppleScript(in); got != want {
			t.Errorf("escapeAppleScript(%q) = %q, want %q", in, got, want)
		}
	}
}

// A process name is chosen by whoever starts the process, so it must reach
// notify-send as text, never as an option.
func TestNotifySendEndsOptionParsing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("notify-send is the Linux notifier")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notify-send"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	cmd := command("linux", "portop: new port listening", "--action=x on :8080")
	if cmd == nil {
		t.Fatal("no command built although notify-send is on PATH")
	}
	want := []string{"notify-send", "--", "portop: new port listening", "--action=x on :8080"}
	if !slices.Equal(cmd.Args, want) {
		t.Fatalf("args = %q, want %q", cmd.Args, want)
	}
}
