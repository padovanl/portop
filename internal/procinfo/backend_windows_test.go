package procinfo

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestWindowsLoadCurrentProcess(t *testing.T) {
	info, err := Load(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if info.Name == "" || info.Exe == "" || info.User == "" || info.RSSBytes == 0 || info.NumThreads < 1 ||
		info.StartTime.IsZero() || info.StartTime.After(time.Now()) {
		t.Fatalf("incomplete process info: %+v", info)
	}
	if !strings.Contains(strings.ToLower(info.Cmdline), strings.ToLower(info.Name)) {
		t.Errorf("command line %q does not mention %q", info.Cmdline, info.Name)
	}
	if wd, _ := os.Getwd(); !strings.EqualFold(info.Cwd, wd) {
		t.Errorf("Cwd = %q, want %q", info.Cwd, wd)
	}
	again, err := Load(os.Getpid())
	if err != nil || !again.StartTime.Equal(info.StartTime) {
		t.Errorf("start time is not stable: %v vs %v (%v)", info.StartTime, again.StartTime, err)
	}
}
