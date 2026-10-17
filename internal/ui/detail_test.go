package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/padovanl/portop/internal/procinfo"
)

// The table refreshes and re-sorts while the details are open; the view
// must keep describing the process it was opened for.
func TestDetailViewKeepsItsRowAcrossRefreshes(t *testing.T) {
	m := mouseTestModel()
	m.cursor = 0
	opened := m.filtered[0]
	model, _ := m.handleNormalKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = model.(Model)

	// A refresh puts another process under the cursor.
	m.cursor = len(m.filtered) - 1
	if m.filtered[m.cursor].ProcessName == opened.ProcessName {
		t.Fatal("test rows need a different process at the end")
	}
	model, _ = m.Update(detailResultMsg{pid: opened.PID, info: procinfo.Info{PID: opened.PID}})
	m = model.(Model)
	if view := m.renderDetail(); !strings.Contains(view, opened.ProcessName) || strings.Contains(view, m.filtered[m.cursor].ProcessName) {
		t.Fatalf("details of %s show another row:\n%s", opened.ProcessName, view)
	}
}

// Details that arrive after the user moved on to another process are
// dropped instead of replacing the newer ones.
func TestDetailViewDropsStaleResults(t *testing.T) {
	m := mouseTestModel()
	m.openDetail(m.filtered[0])
	model, _ := m.Update(detailResultMsg{pid: m.filtered[0].PID + 999, info: procinfo.Info{Name: "stale"}})
	m = model.(Model)
	if m.detailInfo != nil {
		t.Fatalf("stale details were shown: %+v", m.detailInfo)
	}
}
