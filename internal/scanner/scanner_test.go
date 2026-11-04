package scanner

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func writeProcFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	mustWrite := func(rel, content string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	tcpTable := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12345 1 0000000000000000 100 0 0 10 0
`
	mustWrite("net/tcp", tcpTable)
	mustWrite("net/udp", "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n")

	// Process 4242 owns the listening socket via fd 3.
	mustWrite("4242/comm", "nginx\n")
	fdDir := filepath.Join(root, "4242", "fd")
	if err := os.MkdirAll(fdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("socket:[12345]", filepath.Join(fdDir, "3")); err != nil {
		t.Fatal(err)
	}

	return root
}

func TestScanAndResolveProcesses(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("procfs fixture needs symlinks, which require extra privileges on Windows")
	}
	origRoot := ProcRoot
	ProcRoot = writeProcFixture(t)
	defer func() { ProcRoot = origRoot }()

	conns, err := scanProc()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(conns) != 1 {
		t.Fatalf("got %d connections, want 1", len(conns))
	}
	c := conns[0]
	if c.LocalPort != 8080 {
		t.Errorf("LocalPort = %d, want 8080", c.LocalPort)
	}
	if c.State != StateListen {
		t.Errorf("State = %s, want LISTEN", c.State)
	}
	if c.Protocol != TCP {
		t.Errorf("Protocol = %s, want TCP", c.Protocol)
	}

	resolved := ResolveProcesses(conns)
	if resolved[0].PID != 4242 {
		t.Errorf("PID = %d, want 4242", resolved[0].PID)
	}
	if resolved[0].ProcessName != "nginx" {
		t.Errorf("ProcessName = %q, want nginx", resolved[0].ProcessName)
	}
}

func TestScanMissingIPv6TablesIsNotAnError(t *testing.T) {
	origRoot := ProcRoot
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "net"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "net", "tcp"), []byte("header\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "net", "udp"), []byte("header\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ProcRoot = root
	defer func() { ProcRoot = origRoot }()

	if _, err := scanProc(); err != nil {
		t.Fatalf("Scan should tolerate missing tcp6/udp6, got: %v", err)
	}
}

func TestCPUTrackerRetainDropsGonePIDs(t *testing.T) {
	tr := NewCPUTracker()
	tr.prev[10] = cpuSample{ticks: 5}
	tr.prev[20] = cpuSample{ticks: 7}
	tr.Retain(map[int]float64{20: 0})
	if _, ok := tr.prev[10]; ok {
		t.Fatal("pid 10 is gone but its sample was kept")
	}
	if _, ok := tr.prev[20]; !ok {
		t.Fatal("pid 20 is still live but its sample was dropped")
	}
}

// A master and its workers share the listening socket; /proc lists it in
// every one of them, and the row belongs to the master whatever order the
// directories are read in.
func TestResolveProcessesPicksTheMaster(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("procfs fixture needs symlinks, which require extra privileges on Windows")
	}
	origRoot := ProcRoot
	ProcRoot = writeProcFixture(t)
	defer func() { ProcRoot = origRoot }()
	for pid, ppid := range map[string]string{"4242": "1", "4243": "4242", "424": "4242"} {
		dir := filepath.Join(ProcRoot, pid)
		if err := os.MkdirAll(filepath.Join(dir, "fd"), 0o755); err != nil {
			t.Fatal(err)
		}
		if pid != "4242" {
			if err := os.Symlink("socket:[12345]", filepath.Join(dir, "fd", "4")); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "comm"), []byte("nginx\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		stat := pid + " (nginx) S " + ppid + " 1 1 0 -1 0\n"
		if err := os.WriteFile(filepath.Join(dir, "stat"), []byte(stat), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	conns, err := scanProc()
	if err != nil {
		t.Fatal(err)
	}
	c := ResolveProcesses(conns)[0]
	if c.PID != 4242 || len(c.Owners) != 3 || c.Owners[0] != 424 {
		t.Fatalf("PID %d, owners %v: want master 4242 with workers 424 and 4243", c.PID, c.Owners)
	}
}
