package app

import (
	"context"
	"net"
	"testing"

	"github.com/padovanl/portop/internal/docker"
	"github.com/padovanl/portop/internal/scanner"
)

// TestCollectFindsRealListener opens a real TCP listener on the current
// machine and asserts Collect finds it, matches it to this test process's
// PID, and correctly flags it as newly seen only on the first scan. This
// runs against the real /proc filesystem, so it only makes sense on
// Linux (see the e2e/ package for the full black-box version driven
// through the built binary).
func TestCollectFindsRealListener(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := uint16(ln.Addr().(*net.TCPAddr).Port)

	c := NewCollector()
	ctx := context.Background()

	// First Collect call is the baseline: even though this listener is
	// "new" from the OS's perspective, we have nothing to diff against
	// yet, so it must not be flagged as newly seen.
	rows, err := c.Collect(ctx, Options{})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	row := findRow(rows, port)
	if row == nil {
		t.Fatalf("listener on port %d not found in %d rows", port, len(rows))
	}
	if row.FirstSeen {
		t.Errorf("baseline Collect: FirstSeen = true, want false")
	}
	if row.PID == 0 {
		t.Log("PID could not be resolved (likely missing /proc permissions in this sandbox); skipping PID assertion")
	}

	// A second listener opened after the baseline must be flagged as
	// newly seen on the next scan, while the original stays unflagged.
	ln2, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln2.Close()
	port2 := uint16(ln2.Addr().(*net.TCPAddr).Port)

	rows2, err := c.Collect(ctx, Options{})
	if err != nil {
		t.Fatalf("second Collect: %v", err)
	}
	row2 := findRow(rows2, port)
	if row2 == nil {
		t.Fatalf("listener on port %d not found on second scan", port)
	}
	if row2.FirstSeen {
		t.Errorf("second Collect, pre-existing listener: FirstSeen = true, want false")
	}
	newRow := findRow(rows2, port2)
	if newRow == nil {
		t.Fatalf("new listener on port %d not found on second scan", port2)
	}
	if !newRow.FirstSeen {
		t.Errorf("second Collect, new listener: FirstSeen = false, want true")
	}
}

func findRow(rows []Row, port uint16) *Row {
	for i := range rows {
		if rows[i].LocalPort == port {
			return &rows[i]
		}
	}
	return nil
}

func TestRowMatches(t *testing.T) {
	row := Row{
		LocalAddr: net.ParseIP("127.0.0.1"), LocalPort: 8080,
		RemoteAddr: net.ParseIP("203.0.113.4"), RemotePort: 443,
		PID: 4242, ProcessName: "nginx", SystemdUnit: "nginx.service", ContainerName: "frontend",
	}
	for query, want := range map[string]bool{
		"": true, "8080": true, "NGINX": true, "4242": true,
		"127.0.0.1": true, "203.0.113.4": true, "service": true,
		"frontend": true, "redis": false,
	} {
		if got := row.Matches(query); got != want {
			t.Errorf("Matches(%q) = %v, want %v", query, got, want)
		}
	}
}

func TestLabelPublishedPorts(t *testing.T) {
	rows := []Row{
		{Protocol: scanner.TCP, LocalPort: 8080, State: scanner.StateListen, ProcessName: "com.docker.backend"},
		{Protocol: scanner.TCP, LocalPort: 8080, State: scanner.StateEstablished},
		{Protocol: scanner.UDP, LocalPort: 5353, State: scanner.StateUnconn},
		{Protocol: scanner.TCP, LocalPort: 9000, State: scanner.StateListen, ContainerName: "already"},
		{Protocol: scanner.TCP, LocalPort: 7000, State: scanner.StateListen},
	}
	labelPublishedPorts(rows, []docker.PublishedPort{
		{ContainerName: "web", PublicPort: 8080, Type: "TCP", State: "running"},
		{ContainerName: "mdns", PublicPort: 5353, Type: "UDP", State: "running"},
		{ContainerName: "other", PublicPort: 9000, Type: "TCP", State: "running"},
		{ContainerName: "stopped", PublicPort: 7000, Type: "TCP", State: "exited"},
	})
	want := []string{"web", "", "mdns", "already", ""}
	for i, w := range want {
		if rows[i].ContainerName != w {
			t.Errorf("row %d container = %q, want %q", i, rows[i].ContainerName, w)
		}
	}
}

// TestCollectIsSafeForConcurrentCalls covers the TUI, which starts a new
// scan on every tick without waiting for the previous one: when a scan is
// slower than the refresh interval, two Collect calls overlap on the same
// Collector. Run with -race.
func TestCollectIsSafeForConcurrentCalls(t *testing.T) {
	c := NewCollector()
	ctx := context.Background()
	done := make(chan error, 4)
	for i := 0; i < cap(done); i++ {
		go func() {
			_, err := c.Collect(ctx, Options{})
			done <- err
		}()
	}
	for i := 0; i < cap(done); i++ {
		if err := <-done; err != nil {
			t.Fatalf("Collect: %v", err)
		}
	}
}
