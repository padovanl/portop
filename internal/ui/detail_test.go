package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

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

// Labels are never cut short, and a long command line wraps in the value
// column instead of running back under the labels.
func TestDetailFieldsLayout(t *testing.T) {
	m := mouseTestModel()
	m.width = 100
	m.openDetail(m.filtered[0])
	long := "python3 " + strings.Repeat("/very/long/path", 12) + " --flag value"
	model, _ := m.Update(detailResultMsg{pid: m.filtered[0].PID, info: procinfo.Info{
		PID: m.filtered[0].PID, Cmdline: long, Exe: "/usr/bin/python3", OpenFiles: 4}})
	view := ansi.Strip(model.(Model).renderDetail())
	for _, label := range []string{"Executable:", "Open files:", "Cmdline:"} {
		if !strings.Contains(view, label) {
			t.Errorf("label %q missing or truncated:\n%s", label, view)
		}
	}
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "/very/long/path") && !strings.Contains(line, "Cmdline:") {
			if i := strings.Index(line, "/very"); i < 15 {
				t.Errorf("wrapped command line starts at column %d, under the labels:\n%s", i, line)
			}
		}
	}
}

// A modal covers only its own columns; the table stays visible around it.
func TestOverlayKeepsTheTableAroundTheModal(t *testing.T) {
	base := strings.Join([]string{"aaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbb", "cccccccccccccccccccc"}, "\n")
	got := strings.Split(overlay(base, "XXXX", 20, 3), "\n")
	if ansi.Strip(got[1]) != "bbbbbbbbXXXXbbbbbbbb" || ansi.Strip(got[0]) != "aaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("overlay = %q", got)
	}
}
