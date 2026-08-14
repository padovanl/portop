package procinfo

import (
	"os"
	"os/user"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	root := t.TempDir()
	ProcRoot = root
	defer func() { ProcRoot = "/proc" }()

	pidDir := filepath.Join(root, "4242")
	mustMkdir(t, pidDir)
	mustMkdir(t, filepath.Join(pidDir, "fd"))
	mustWrite(t, filepath.Join(pidDir, "fd", "0"), "")
	mustWrite(t, filepath.Join(pidDir, "fd", "1"), "")

	mustWrite(t, filepath.Join(pidDir, "comm"), "nginx\n")
	mustWrite(t, filepath.Join(pidDir, "cmdline"), "nginx\x00-g\x00daemon off;\x00")

	me, err := user.Current()
	if err != nil {
		t.Skip("cannot resolve current user in this environment")
	}
	mustWrite(t, filepath.Join(pidDir, "status"), "Name:\tnginx\nUid:\t"+me.Uid+"\t"+me.Uid+"\t"+me.Uid+"\t"+me.Uid+"\n")

	if err := os.Symlink("/usr/sbin/nginx", filepath.Join(pidDir, "exe")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/var/www", filepath.Join(pidDir, "cwd")); err != nil {
		t.Fatal(err)
	}

	// state ppid pgrp session tty tpgid flags minflt cminflt majflt cmajflt
	// utime stime cutime cstime priority nice numThreads(17) itrealvalue
	// starttime(19) vsize rss(21)
	statFields := []string{
		"S", "1", "1", "1", "0", "-1", "0", "0", "0", "0", "0",
		"10", "5", "0", "0", "20", "0", "3", "0",
		"500", "0", "2048",
	}
	statLine := "4242 (nginx) "
	for i, f := range statFields {
		if i > 0 {
			statLine += " "
		}
		statLine += f
	}
	mustWrite(t, filepath.Join(pidDir, "stat"), statLine+"\n")

	mustWrite(t, filepath.Join(root, "stat"), "cpu  0 0 0 0 0 0 0 0 0 0\nbtime 1000000000\n")

	info, err := Load(4242)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if info.Name != "nginx" {
		t.Errorf("Name = %q, want nginx", info.Name)
	}
	if info.Cmdline != "nginx -g daemon off;" {
		t.Errorf("Cmdline = %q, want %q", info.Cmdline, "nginx -g daemon off;")
	}
	if info.Exe != "/usr/sbin/nginx" {
		t.Errorf("Exe = %q, want /usr/sbin/nginx", info.Exe)
	}
	if info.Cwd != "/var/www" {
		t.Errorf("Cwd = %q, want /var/www", info.Cwd)
	}
	if info.User != me.Username {
		t.Errorf("User = %q, want %q", info.User, me.Username)
	}
	if info.OpenFiles != 2 {
		t.Errorf("OpenFiles = %d, want 2", info.OpenFiles)
	}
	if info.NumThreads != 3 {
		t.Errorf("NumThreads = %d, want 3", info.NumThreads)
	}
	wantRSS := uint64(2048) * uint64(os.Getpagesize())
	if info.RSSBytes != wantRSS {
		t.Errorf("RSSBytes = %d, want %d", info.RSSBytes, wantRSS)
	}
	wantStart := time.Unix(1000000000, 0).Add(5 * time.Second)
	if !info.StartTime.Equal(wantStart) {
		t.Errorf("StartTime = %v, want %v", info.StartTime, wantStart)
	}
}

func TestLoadMissingPID(t *testing.T) {
	ProcRoot = t.TempDir()
	defer func() { ProcRoot = "/proc" }()
	if _, err := Load(999999); err == nil {
		t.Error("expected error for missing pid, got nil")
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
