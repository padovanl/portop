package ui

import (
	"fmt"
	"net"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/padovanl/portop/internal/app"
	"github.com/padovanl/portop/internal/scanner"
)

func mouseTestModel() Model {
	m := New(Config{ShowEstablished: true})
	m.width = 100
	m.height = 20
	m.rows = sampleRows()
	m.refilter()
	return m
}

func mouseMsg(x, y int, button tea.MouseButton, action tea.MouseAction) tea.MouseMsg {
	return tea.MouseMsg(tea.MouseEvent{X: x, Y: y, Button: button, Action: action})
}

func renderedHeaderY(t *testing.T, m Model) int {
	t.Helper()
	for y, line := range strings.Split(m.View(), "\n") {
		if strings.Contains(line, "PORT") && strings.Contains(line, "PROTO") && strings.Contains(line, "STATE") {
			return y
		}
	}
	t.Fatal("rendered table header not found")
	return -1
}

func TestPageUpAndPageDownMoveByVisiblePage(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 20
	for i := 0; i < 30; i++ {
		m.rows = append(m.rows, app.Row{
			Protocol:    scanner.TCP,
			LocalAddr:   net.ParseIP("127.0.0.1"),
			LocalPort:   uint16(1000 + i),
			State:       scanner.StateListen,
			PID:         i + 1,
			ProcessName: "process",
		})
	}
	m.refilter()

	model, _ := m.handleNormalKey(tea.KeyMsg{Type: tea.KeyPgDown})
	m = model.(Model)
	if m.cursor != m.pageSize() {
		t.Fatalf("Page Down cursor = %d, want %d", m.cursor, m.pageSize())
	}

	model, _ = m.handleNormalKey(tea.KeyMsg{Type: tea.KeyPgUp})
	m = model.(Model)
	if m.cursor != 0 {
		t.Fatalf("Page Up cursor = %d, want 0", m.cursor)
	}

	m.cursor = len(m.filtered) - 2
	model, _ = m.handleNormalKey(tea.KeyMsg{Type: tea.KeyPgDown})
	m = model.(Model)
	if m.cursor != len(m.filtered)-1 {
		t.Fatalf("Page Down should clamp at bottom: cursor = %d", m.cursor)
	}
}

func TestMouseClickSelectsVisibleRow(t *testing.T) {
	m := mouseTestModel()
	dataY := m.tableHeaderY() + 2
	originalSort := m.sort
	originalDirection := m.sortDescending

	model, _ := m.handleMouse(mouseMsg(appContentX, dataY+2, tea.MouseButtonLeft, tea.MouseActionPress))
	m = model.(Model)
	if m.cursor != 2 {
		t.Fatalf("cursor = %d, want clicked row 2", m.cursor)
	}
	if m.sort != originalSort || m.sortDescending != originalDirection {
		t.Fatalf("row click changed sort: mode=%v descending=%v", m.sort, m.sortDescending)
	}
}

func TestMouseRowsAlignWithRenderedTable(t *testing.T) {
	for _, width := range []int{80, 100, 120} {
		t.Run(fmt.Sprintf("width_%d", width), func(t *testing.T) {
			m := mouseTestModel()
			m.width = width
			headerY := renderedHeaderY(t, m)
			if headerY != m.tableHeaderY() {
				t.Fatalf("rendered header y = %d, mouse header y = %d", headerY, m.tableHeaderY())
			}

			want := m.filtered[0]
			model, _ := m.handleMouse(mouseMsg(appContentX, headerY+2, tea.MouseButtonRight, tea.MouseActionPress))
			m = model.(Model)
			if m.mode != modeConfirmKill || m.killTarget.PID != want.PID || keyForRow(m.killTarget) != keyForRow(want) {
				t.Fatalf("right-click targeted %+v, want first rendered row %+v", m.killTarget, want)
			}
		})
	}
}

func TestMouseClickDoesNotRecenterViewport(t *testing.T) {
	m := New(Config{})
	m.width = 100
	m.height = 15 // six visible data rows
	for i := 0; i < 30; i++ {
		m.rows = append(m.rows, app.Row{
			Protocol:    scanner.TCP,
			LocalAddr:   net.ParseIP("127.0.0.1"),
			LocalPort:   uint16(1000 + i),
			State:       scanner.StateListen,
			PID:         i + 1,
			ProcessName: "process",
		})
	}
	m.refilter()
	m.cursor = 10
	m.viewportStart = 10
	before := m.visibleRows()
	dataY := m.tableHeaderY() + 2

	// Select the last visible row. It must remain at that screen position.
	model, _ := m.handleMouse(mouseMsg(appContentX, dataY+len(before.rows)-1, tea.MouseButtonLeft, tea.MouseActionPress))
	m = model.(Model)
	after := m.visibleRows()
	if after.offset != before.offset {
		t.Fatalf("row click moved viewport from %d to %d", before.offset, after.offset)
	}
	if m.cursor != before.offset+len(before.rows)-1 {
		t.Fatalf("cursor = %d, want clicked row %d", m.cursor, before.offset+len(before.rows)-1)
	}
}

func TestMouseHoverAndDragRows(t *testing.T) {
	m := mouseTestModel()
	dataY := m.tableHeaderY() + 2

	model, _ := m.handleMouse(mouseMsg(appContentX, dataY+1, tea.MouseButtonNone, tea.MouseActionMotion))
	m = model.(Model)
	if m.hoverCursor != 1 || m.cursor != 0 {
		t.Fatalf("hover should highlight without selecting: hover=%d cursor=%d", m.hoverCursor, m.cursor)
	}

	model, _ = m.handleMouse(mouseMsg(appContentX, dataY+3, tea.MouseButtonLeft, tea.MouseActionMotion))
	m = model.(Model)
	if m.hoverCursor != 3 || m.cursor != 3 {
		t.Fatalf("left drag should select: hover=%d cursor=%d", m.hoverCursor, m.cursor)
	}

	model, _ = m.handleMouse(mouseMsg(0, 0, tea.MouseButtonNone, tea.MouseActionMotion))
	m = model.(Model)
	if m.hoverCursor != -1 {
		t.Fatalf("hover outside table = %d, want -1", m.hoverCursor)
	}
}

func TestMouseWheelMovesAndClampsCursor(t *testing.T) {
	m := mouseTestModel()

	model, _ := m.handleMouse(mouseMsg(10, 10, tea.MouseButtonWheelDown, tea.MouseActionPress))
	m = model.(Model)
	if m.cursor != 3 {
		t.Fatalf("wheel down cursor = %d, want 3", m.cursor)
	}

	model, _ = m.handleMouse(mouseMsg(10, 10, tea.MouseButtonWheelUp, tea.MouseActionPress))
	m = model.(Model)
	if m.cursor != 0 {
		t.Fatalf("wheel up cursor = %d, want 0", m.cursor)
	}
}

func TestMouseHeaderClickSortsAndTogglesDirection(t *testing.T) {
	m := mouseTestModel()
	// PROCESS begins after the marker, PORT, PROTO, and STATE columns.
	processX := appContentX + markerWidth + 11 + 8 + 12

	model, _ := m.handleMouse(mouseMsg(processX, m.tableHeaderY(), tea.MouseButtonLeft, tea.MouseActionPress))
	m = model.(Model)
	if m.sort != sortByProcess || m.sortDescending {
		t.Fatalf("first PROCESS click: sort=%v descending=%v", m.sort, m.sortDescending)
	}
	if m.filtered[0].ProcessName != "node" || m.filtered[len(m.filtered)-1].ProcessName != "sshd" {
		t.Fatalf("rows not sorted by process ascending: %+v", m.filtered)
	}

	model, _ = m.handleMouse(mouseMsg(processX, m.tableHeaderY(), tea.MouseButtonLeft, tea.MouseActionPress))
	m = model.(Model)
	if !m.sortDescending || m.filtered[0].ProcessName != "sshd" {
		t.Fatalf("second PROCESS click should reverse order: descending=%v first=%q", m.sortDescending, m.filtered[0].ProcessName)
	}
}

func TestProtocolHeaderRendersBothSortDirections(t *testing.T) {
	m := mouseTestModel()
	protocolX := appContentX + markerWidth + 11

	model, _ := m.handleMouse(mouseMsg(protocolX, renderedHeaderY(t, m), tea.MouseButtonLeft, tea.MouseActionPress))
	m = model.(Model)
	if view := m.View(); !strings.Contains(view, "PROTO ↑") {
		t.Fatal("ascending protocol sort indicator is not visible")
	}

	model, _ = m.handleMouse(mouseMsg(protocolX, renderedHeaderY(t, m), tea.MouseButtonLeft, tea.MouseActionPress))
	m = model.(Model)
	if view := m.View(); !strings.Contains(view, "PROTO ↓") {
		t.Fatal("descending protocol sort indicator is not visible")
	}
}

func TestMouseCPUHeaderDefaultsToDescending(t *testing.T) {
	m := mouseTestModel()
	cpuX := appContentX + markerWidth + 11 + 8 + 12 + 18 + 7

	model, _ := m.handleMouse(mouseMsg(cpuX, m.tableHeaderY(), tea.MouseButtonLeft, tea.MouseActionPress))
	m = model.(Model)
	if m.sort != sortByCPU || !m.sortDescending {
		t.Fatalf("CPU click: sort=%v descending=%v, want CPU descending", m.sort, m.sortDescending)
	}
}

func TestMouseDoubleClickOpensDetails(t *testing.T) {
	m := mouseTestModel()
	dataY := m.tableHeaderY() + 2
	click := mouseMsg(appContentX, dataY, tea.MouseButtonLeft, tea.MouseActionPress)

	model, _ := m.handleMouse(click)
	m = model.(Model)
	model, cmd := m.handleMouse(click)
	m = model.(Model)
	if m.mode != modeDetail || cmd == nil {
		t.Fatalf("double click: mode=%v cmd nil=%v, want detail with load command", m.mode, cmd == nil)
	}
}

func TestMouseRightClickRequiresExistingKillConfirmation(t *testing.T) {
	m := mouseTestModel()
	dataY := m.tableHeaderY() + 2

	model, cmd := m.handleMouse(mouseMsg(appContentX, dataY, tea.MouseButtonRight, tea.MouseActionPress))
	m = model.(Model)
	if m.mode != modeConfirmKill || cmd != nil {
		t.Fatalf("right click: mode=%v cmd nil=%v, want confirmation and no signal command", m.mode, cmd == nil)
	}

	model, cmd = m.handleMouse(mouseMsg(0, 0, tea.MouseButtonLeft, tea.MouseActionPress))
	m = model.(Model)
	if m.mode != modeNormal || cmd != nil {
		t.Fatalf("click on confirmation should cancel: mode=%v cmd nil=%v", m.mode, cmd == nil)
	}
}

func TestMouseClickLeavesFilterModeAndUsesShiftedTable(t *testing.T) {
	m := mouseTestModel()
	m.mode = modeFilter
	m.filterInput.Focus()
	dataY := m.tableHeaderY() + 2

	model, _ := m.handleMouse(mouseMsg(appContentX, dataY+1, tea.MouseButtonLeft, tea.MouseActionPress))
	m = model.(Model)
	if m.mode != modeNormal || m.cursor != 1 {
		t.Fatalf("filtered click: mode=%v cursor=%d, want normal and row 1", m.mode, m.cursor)
	}
}
