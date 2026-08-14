package scanner

import (
	"os"
	"path/filepath"
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
	origRoot := ProcRoot
	ProcRoot = writeProcFixture(t)
	defer func() { ProcRoot = origRoot }()

	conns, err := Scan()
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

	if _, err := Scan(); err != nil {
		t.Fatalf("Scan should tolerate missing tcp6/udp6, got: %v", err)
	}
}
