package ui

import (
	"net"
	"testing"

	"github.com/padovanl/portop/internal/app"
	"github.com/padovanl/portop/internal/scanner"
)

func sampleRows() []app.Row {
	return []app.Row{
		{Protocol: scanner.TCP, LocalAddr: net.ParseIP("0.0.0.0"), LocalPort: 8080, State: scanner.StateListen, PID: 100, ProcessName: "node", CPUPercent: 2.5},
		{Protocol: scanner.TCP, LocalAddr: net.ParseIP("127.0.0.1"), LocalPort: 22, State: scanner.StateListen, PID: 1, ProcessName: "sshd", CPUPercent: 0.1},
		{Protocol: scanner.TCP, LocalAddr: net.ParseIP("::1"), LocalPort: 5432, IPv6: true, State: scanner.StateListen, PID: 50, ProcessName: "postgres", CPUPercent: 1.4},
		{Protocol: scanner.TCP, LocalAddr: net.ParseIP("127.0.0.1"), LocalPort: 443, RemoteAddr: net.ParseIP("93.184.216.34"), RemotePort: 51000, State: scanner.StateEstablished, PID: 100, ProcessName: "node"},
	}
}

func TestRefilterDefaultShowsOnlyListen(t *testing.T) {
	m := New(Config{})
	m.rows = sampleRows()
	m.refilter()
	for _, r := range m.filtered {
		if r.State != scanner.StateListen {
			t.Errorf("expected only LISTEN rows by default, found state %s", r.State)
		}
	}
	if len(m.filtered) != 3 {
		t.Errorf("filtered len = %d, want 3", len(m.filtered))
	}
}

func TestRefilterShowEstablishedIncludesIt(t *testing.T) {
	m := New(Config{})
	m.rows = sampleRows()
	m.showEstablished = true
	m.refilter()
	if len(m.filtered) != 4 {
		t.Errorf("filtered len = %d, want 4", len(m.filtered))
	}
}

func TestRefilterIPv4Only(t *testing.T) {
	m := New(Config{})
	m.rows = sampleRows()
	m.ipFilter = ipv4Only
	m.refilter()
	for _, r := range m.filtered {
		if r.IPv6 {
			t.Errorf("ipv4Only should exclude IPv6 rows, got %+v", r)
		}
	}
	if len(m.filtered) != 2 {
		t.Errorf("filtered len = %d, want 2", len(m.filtered))
	}
}

func TestRefilterQueryMatchesPortOrProcess(t *testing.T) {
	m := New(Config{})
	m.rows = sampleRows()
	m.filterInput.SetValue("ssh")
	m.refilter()
	if len(m.filtered) != 1 || m.filtered[0].ProcessName != "sshd" {
		t.Errorf("expected only sshd row, got %+v", m.filtered)
	}

	m.filterInput.SetValue("8080")
	m.refilter()
	if len(m.filtered) != 1 || m.filtered[0].LocalPort != 8080 {
		t.Errorf("expected only :8080 row, got %+v", m.filtered)
	}
}

func TestSortRowsByCPUDescending(t *testing.T) {
	rows := sampleRows()[:3] // three LISTEN rows with distinct CPU%
	sortRows(rows, sortByCPU)
	for i := 1; i < len(rows); i++ {
		if rows[i-1].CPUPercent < rows[i].CPUPercent {
			t.Errorf("rows not sorted by CPU descending: %v before %v", rows[i-1].CPUPercent, rows[i].CPUPercent)
		}
	}
}

func TestSortRowsByPort(t *testing.T) {
	rows := sampleRows()[:3]
	sortRows(rows, sortByPort)
	for i := 1; i < len(rows); i++ {
		if rows[i-1].LocalPort > rows[i].LocalPort {
			t.Errorf("rows not sorted by port ascending")
		}
	}
}

func TestCursorClampedAfterFilterShrinks(t *testing.T) {
	m := New(Config{})
	m.rows = sampleRows()
	m.showEstablished = true
	m.refilter()
	m.cursor = len(m.filtered) - 1

	m.filterInput.SetValue("sshd")
	m.refilter()
	if m.cursor >= len(m.filtered) {
		t.Errorf("cursor %d not clamped to filtered len %d", m.cursor, len(m.filtered))
	}
}

func TestPadTrunc(t *testing.T) {
	if got := padTrunc("abc", 6); got != "abc   " {
		t.Errorf("padTrunc short = %q", got)
	}
	if got := padTrunc("abcdefgh", 5); len([]rune(got)) != 5 {
		t.Errorf("padTrunc long len = %d, want 5 (got %q)", len([]rune(got)), got)
	}
}
