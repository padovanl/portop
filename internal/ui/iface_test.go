package ui

import (
	"net"
	"testing"

	"github.com/padovanl/portop/internal/app"
	"github.com/padovanl/portop/internal/scanner"
)

func TestWideTableShowsIfaceAndUser(t *testing.T) {
	var titles []string
	for _, c := range columnsFor(false, 160) {
		titles = append(titles, c.title)
	}
	want := map[string]bool{"IFACE": false, "USER": false}
	for _, title := range titles {
		if _, ok := want[title]; ok {
			want[title] = true
		}
	}
	if !want["IFACE"] || !want["USER"] {
		t.Fatalf("columns at 160 cells = %v, want IFACE and USER", titles)
	}
	row := app.Row{LocalAddr: net.ParseIP("0.0.0.0"), Username: "www-data"}
	if got := cellValue(row, "IFACE"); got != "0.0.0.0" {
		t.Errorf("IFACE = %q", got)
	}
	if got := cellValue(row, "USER"); got != "www-data" {
		t.Errorf("USER = %q", got)
	}
	if got := cellValue(app.Row{}, "USER"); got != "-" {
		t.Errorf("USER of an unknown owner = %q, want -", got)
	}
}

// Sorting by IFACE puts what the network can reach first.
func TestSortByExposure(t *testing.T) {
	rows := []app.Row{
		{Protocol: scanner.TCP, LocalAddr: net.ParseIP("127.0.0.1"), LocalPort: 1},
		{Protocol: scanner.TCP, LocalAddr: net.ParseIP("192.168.1.5"), LocalPort: 2},
		{Protocol: scanner.TCP, LocalAddr: net.ParseIP("::"), LocalPort: 3},
	}
	sortRowsDirection(rows, sortByIface, false)
	if rows[0].LocalPort != 3 || rows[1].LocalPort != 2 || rows[2].LocalPort != 1 {
		t.Fatalf("order = %d %d %d, want all interfaces, one address, loopback", rows[0].LocalPort, rows[1].LocalPort, rows[2].LocalPort)
	}
	if mode, ok := sortModeForColumn("IFACE"); !ok || mode != sortByIface {
		t.Fatal("clicking the IFACE heading does not sort by exposure")
	}
}

func TestProcessCellCountsWorkers(t *testing.T) {
	row := app.Row{ProcessName: "nginx", Workers: []int{101, 102}}
	if got := cellValue(row, "PROCESS"); got != "nginx +2" {
		t.Fatalf("PROCESS = %q, want nginx +2", got)
	}
	if got := workerList([]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}); got != "10 (1, 2, 3, 4, 5, 6, 7, 8, +2 more)" {
		t.Fatalf("workerList = %q", got)
	}
}

func TestFirewallCell(t *testing.T) {
	exposed := app.Row{Protocol: scanner.TCP, LocalAddr: net.ParseIP("0.0.0.0"), LocalPort: 80, State: scanner.StateListen}
	if got := cellValue(exposed, "FIREWALL"); got != "?" {
		t.Errorf("exposed, firewall unreadable = %q, want ?", got)
	}
	exposed.Firewall = "blocked"
	if got := cellValue(exposed, "FIREWALL"); got != "blocked" {
		t.Errorf("verdict = %q", got)
	}
	local := app.Row{Protocol: scanner.TCP, LocalAddr: net.ParseIP("127.0.0.1"), LocalPort: 5432, State: scanner.StateListen}
	if got := cellValue(local, "FIREWALL"); got != "-" {
		t.Errorf("loopback = %q, want -", got)
	}
}

func TestTrafficCell(t *testing.T) {
	if got := cellValue(app.Row{}, "TRAFFIC"); got != "-" {
		t.Errorf("no traffic = %q", got)
	}
	row := app.Row{RateKnown: true, RXRate: 1234, TXRate: 87}
	if got := cellValue(row, "TRAFFIC"); got != "↓1.2K ↑87" {
		t.Errorf("TRAFFIC = %q", got)
	}
	for bps, want := range map[float64]string{0: "0", 999: "999", 12_500: "13K", 2_500_000: "2.5M", 340_000_000: "340M", 3e9: "3.0G"} {
		if got := formatRate(bps); got != want {
			t.Errorf("formatRate(%v) = %q, want %q", bps, got, want)
		}
	}
}
