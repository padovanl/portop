//go:build e2e

// Package e2e black-box tests the real, compiled portop binary: it
// builds the binary once, opens real listening sockets, execs portop
// against the live system, and asserts on what actually comes back.
// This is what CI runs as the required "e2e" check.
package e2e

import (
	"encoding/json"
	"net"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type row struct {
	Protocol  string `json:"protocol"`
	LocalPort int    `json:"local_port"`
	State     string `json:"state"`
	PID       int    `json:"pid"`
	Process   string `json:"process"`
}

func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "portop")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/portop")
	cmd.Dir = ".."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/portop failed: %v\n%s", err, out)
	}
	return bin
}

func TestJSONSnapshotFindsRealListener(t *testing.T) {
	bin := buildBinary(t)

	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	out, err := exec.Command(bin, "--json", "--no-dns", "--no-systemd", "--no-docker").Output()
	if err != nil {
		t.Fatalf("portop --json failed: %v", err)
	}

	var rows []row
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("invalid JSON from portop --json: %v\n%s", err, out)
	}

	found := false
	for _, r := range rows {
		if r.LocalPort == port && r.State == "LISTEN" && r.Protocol == "TCP" {
			found = true
		}
	}
	if !found {
		t.Fatalf("listener on 127.0.0.1:%d not found among %d rows returned by the real binary", port, len(rows))
	}
}

func TestListenFlagExcludesEstablished(t *testing.T) {
	bin := buildBinary(t)

	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	accepted := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err == nil {
			accepted <- c
		}
	}()

	conn, err := net.DialTimeout("tcp4", ln.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	select {
	case c := <-accepted:
		defer c.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("connection was not accepted in time")
	}

	out, err := exec.Command(bin, "--json", "--listen", "--no-dns", "--no-systemd", "--no-docker").Output()
	if err != nil {
		t.Fatalf("portop --json --listen failed: %v", err)
	}

	var rows []row
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	for _, r := range rows {
		if r.State != "LISTEN" {
			t.Errorf("--listen leaked a non-LISTEN row: %+v", r)
		}
	}
}

func TestVersionFlag(t *testing.T) {
	bin := buildBinary(t)
	out, err := exec.Command(bin, "--version").Output()
	if err != nil {
		t.Fatalf("portop --version failed: %v", err)
	}
	if !strings.Contains(string(out), "portop") {
		t.Errorf("--version output = %q, want it to mention portop", out)
	}
}
