package ui

import (
	"strings"
	"testing"

	"github.com/padovanl/portop/internal/app"
	"github.com/padovanl/portop/internal/scanner"
)

// TIME_WAIT sockets belong to no process at all: more privileges would not
// resolve them, so they are not "hidden" rows.
func TestClosingSocketsAreNotHidden(t *testing.T) {
	rows := []app.Row{
		{Protocol: scanner.TCP, LocalPort: 443, State: scanner.StateTimeWait},
		{Protocol: scanner.TCP, LocalPort: 53, State: scanner.StateListen},
	}
	if n := countUnresolved(rows); n != 1 {
		t.Fatalf("countUnresolved = %d, want 1 (only the listener)", n)
	}
	if hint := unresolvedHint(rows[0]); !strings.Contains(hint, "closing") {
		t.Fatalf("hint for a TIME_WAIT row = %q", hint)
	}
}

// The status bar counts what the table shows, not rows a filter hides.
func TestUnresolvedHintCountsVisibleRows(t *testing.T) {
	if isRoot() {
		t.Skip("the hint is not shown to root")
	}
	m := New(Config{}) // listen-only view
	m.rows = []app.Row{
		{Protocol: scanner.TCP, LocalPort: 22, State: scanner.StateListen, PID: 1, ProcessName: "sshd"},
		{Protocol: scanner.TCP, LocalPort: 51000, State: scanner.StateEstablished},
	}
	m.refilter()
	if strings.Contains(m.renderStatusBar(), "hidden") {
		t.Fatalf("status bar reports hidden rows the view does not show:\n%s", m.renderStatusBar())
	}
}
