package procinfo

import (
	"os"
	"testing"
	"time"
)

func TestDarwinLoadCurrentProcess(t *testing.T) {
	info, err := Load(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if info.Name == "" || info.Cmdline == "" || info.User == "" || info.RSSBytes == 0 || info.StartTime.IsZero() || info.StartTime.After(time.Now()) || info.Cwd == "" || info.OpenFiles < 1 {
		t.Fatalf("incomplete process info: %+v", info)
	}
}
