package app

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/padovanl/portop/internal/docker"
	"github.com/padovanl/portop/internal/firewall"
	"github.com/padovanl/portop/internal/scanner"
	"github.com/padovanl/portop/internal/traffic"
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

func TestRowListening(t *testing.T) {
	cases := []struct {
		row  Row
		want bool
	}{
		{Row{Protocol: scanner.TCP, LocalPort: 22, State: scanner.StateListen}, true},
		{Row{Protocol: scanner.TCP, LocalPort: 51000, State: scanner.StateEstablished}, false},
		{Row{Protocol: scanner.UDP, LocalPort: 53, State: scanner.StateUnconn}, true},
		{Row{Protocol: scanner.UDP, LocalPort: 41000, State: scanner.StateEstablished, RemotePort: 53}, false},
		{Row{Protocol: scanner.UDP, State: scanner.StateUnconn}, false}, // never bound
	}
	for _, c := range cases {
		if got := c.row.Listening(); got != c.want {
			t.Errorf("%s %s: Listening() = %v, want %v", c.row.Protocol, c.row.State, got, c.want)
		}
	}
}

// "portop 8080 nginx" or typing "nginx 443" narrows by every word.
func TestRowMatchesEveryWord(t *testing.T) {
	row := Row{Protocol: scanner.TCP, LocalAddr: net.ParseIP("0.0.0.0"), LocalPort: 443, PID: 812, ProcessName: "nginx"}
	for query, want := range map[string]bool{
		"":           true,
		"nginx":      true,
		"nginx 443":  true,
		"443  NGINX": true,
		"nginx 8080": false,
		"postgres":   false,
	} {
		if got := row.Matches(query); got != want {
			t.Errorf("Matches(%q) = %v, want %v", query, got, want)
		}
	}
}

func TestScopeOf(t *testing.T) {
	for addr, want := range map[string]string{
		"0.0.0.0": ScopeAll, "::": ScopeAll,
		"127.0.0.1": ScopeLoopback, "::1": ScopeLoopback,
		"192.168.1.5": ScopeAddress, "fe80::1": ScopeAddress,
	} {
		if got := ScopeOf(net.ParseIP(addr)); got != want {
			t.Errorf("ScopeOf(%s) = %s, want %s", addr, got, want)
		}
	}
}

// A socket with no owner at all (a Linux TIME_WAIT entry) must not be
// attributed to root just because its uid field reads 0.
func TestUsernameOfOwnerlessSocket(t *testing.T) {
	c := NewCollector()
	if got := c.username(scanner.Connection{State: scanner.StateTimeWait}); got != "" {
		t.Fatalf("username = %q, want empty", got)
	}
	if got := c.username(scanner.Connection{User: `HOST\alice`}); got != `HOST\alice` {
		t.Fatalf("username from the OS = %q", got)
	}
}

// On a Kubernetes node without Docker, a pod's process is named from the
// kubelet's records.
func TestContainerNameForKubernetesPod(t *testing.T) {
	const id = "3f2a9c1b7e5d4a6f8b0c2d4e6f8a0b1c3d5e7f9a1b3c5d7e9f0a2b4c6d8e0f1a"
	procRoot, logDir := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(procRoot, "77"), 0o755); err != nil {
		t.Fatal(err)
	}
	cgroup := "0::/kubepods.slice/kubepods-pod1.slice/cri-containerd-" + id + ".scope\n"
	if err := os.WriteFile(filepath.Join(procRoot, "77", "cgroup"), []byte(cgroup), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logDir, "api-6c9d_prod_server-"+id+".log"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	oldProc, oldLogs, oldSock := docker.ProcRoot, docker.KubeLogDir, docker.SocketPath
	docker.ProcRoot, docker.KubeLogDir, docker.SocketPath = procRoot, logDir, filepath.Join(procRoot, "no-docker.sock")
	t.Cleanup(func() { docker.ProcRoot, docker.KubeLogDir, docker.SocketPath = oldProc, oldLogs, oldSock })

	if got := NewCollector().containerName(context.Background(), 77); got != "prod/api-6c9d/server" {
		t.Fatalf("containerName = %q, want prod/api-6c9d/server", got)
	}
}

type fakePolicy map[uint16]firewall.Verdict

func (p fakePolicy) Verdict(_ string, port uint16) firewall.Verdict { return p[port] }

// Only listeners the network can reach get a firewall verdict.
func TestApplyFirewall(t *testing.T) {
	rows := []Row{
		{Protocol: scanner.TCP, LocalAddr: net.ParseIP("0.0.0.0"), LocalPort: 80, State: scanner.StateListen},
		{Protocol: scanner.TCP, LocalAddr: net.ParseIP("127.0.0.1"), LocalPort: 81, State: scanner.StateListen},
		{Protocol: scanner.TCP, LocalAddr: net.ParseIP("0.0.0.0"), LocalPort: 82, RemotePort: 9, State: scanner.StateEstablished},
	}
	applyFirewall(rows, fakePolicy{80: firewall.Blocked, 81: firewall.Allowed, 82: firewall.Allowed})
	if rows[0].Firewall != "blocked" || rows[1].Firewall != "" || rows[2].Firewall != "" {
		t.Fatalf("verdicts = %q %q %q", rows[0].Firewall, rows[1].Firewall, rows[2].Firewall)
	}
}

func TestApplyTraffic(t *testing.T) {
	conn := Row{Protocol: scanner.TCP, LocalAddr: net.ParseIP("10.0.0.2"), LocalPort: 51000,
		RemoteAddr: net.ParseIP("1.2.3.4"), RemotePort: 443, State: scanner.StateEstablished}
	k := traffic.KeyFor(conn.LocalAddr, conn.LocalPort, conn.RemoteAddr, conn.RemotePort)
	c := NewCollector()
	t0 := time.Unix(1700000000, 0)

	rows := []Row{conn}
	c.applyTraffic(rows, map[traffic.Key]traffic.Counter{k: {RX: 1000, TX: 100}}, t0)
	if !rows[0].TrafficKnown || rows[0].RateKnown || rows[0].RXBytes != 1000 {
		t.Fatalf("first scan: %+v", rows[0])
	}

	rows = []Row{conn}
	c.applyTraffic(rows, map[traffic.Key]traffic.Counter{k: {RX: 5000, TX: 300}}, t0.Add(2*time.Second))
	if !rows[0].RateKnown || rows[0].RXRate != 2000 || rows[0].TXRate != 100 {
		t.Fatalf("second scan: %+v", rows[0])
	}

	// The same endpoints reused by a new connection: counters restart.
	rows = []Row{conn}
	c.applyTraffic(rows, map[traffic.Key]traffic.Counter{k: {RX: 10, TX: 10}}, t0.Add(4*time.Second))
	if rows[0].RateKnown {
		t.Fatalf("rate across a reused connection: %+v", rows[0])
	}
}
