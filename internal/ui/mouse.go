package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/padovanl/portop/internal/app"
	"github.com/padovanl/portop/internal/openurl"
)

const (
	// styleAppBorder uses a one-cell border and one-cell horizontal padding.
	appContentX = 2
	appContentY = 1

	mouseWheelRows    = 3
	doubleClickWindow = 500 * time.Millisecond
)

type mouseClick struct {
	key app.Key
	pid int
	x   int
	y   int
	at  time.Time
}

// handleMouse implements mouse interaction without weakening the existing
// confirmation requirement for process termination.
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	event := tea.MouseEvent(msg)

	if event.Action == tea.MouseActionPress && event.IsWheel() {
		switch m.mode {
		case modeNormal, modeFilter:
			if event.Button == tea.MouseButtonWheelUp {
				m.moveCursor(-mouseWheelRows)
			} else if event.Button == tea.MouseButtonWheelDown {
				m.moveCursor(mouseWheelRows)
			}
		case modeSettings:
			if event.Button == tea.MouseButtonWheelUp {
				m.settingsCursor = max(0, m.settingsCursor-mouseWheelRows)
			} else if event.Button == tea.MouseButtonWheelDown {
				m.settingsCursor = min(settingsRowCount()-1, m.settingsCursor+mouseWheelRows)
			}
		}
		return m, nil
	}

	// Modal clicks are deliberately conservative. Details/help close on a
	// click, while a kill dialog can only be cancelled with the mouse; sending
	// a signal still requires the explicit y/f keyboard confirmation.
	if event.Action == tea.MouseActionPress {
		switch m.mode {
		case modeDetail, modeHelp:
			m.mode = modeNormal
			return m, nil
		case modeConfirmKill:
			m.mode = modeNormal
			return m, nil
		case modeSettings:
			return m, nil
		}
	}

	if m.mode != modeNormal && m.mode != modeFilter {
		return m, nil
	}

	if event.Action == tea.MouseActionMotion {
		if idx, ok := m.rowAt(event.X, event.Y); ok {
			m.hoverCursor = idx
			if event.Button == tea.MouseButtonLeft {
				m.cursor = idx // left-button drag selection
				m.normalizeViewport()
			}
		} else {
			m.hoverCursor = -1
		}
		return m, nil
	}

	if event.Action != tea.MouseActionPress {
		return m, nil
	}

	if event.Button == tea.MouseButtonLeft && event.Y == m.tableHeaderY() {
		if mode, ok := m.sortModeAt(event.X); ok {
			if m.sort == mode {
				m.sortDescending = !m.sortDescending
			} else {
				m.sort = mode
				m.sortDescending = defaultSortDescending(mode)
			}
			m.refilter()
		}
		return m, nil
	}

	idx, ok := m.rowAt(event.X, event.Y)
	if !ok {
		return m, nil
	}
	m.cursor = idx
	m.hoverCursor = idx
	m.normalizeViewport()
	row := m.filtered[idx]

	if m.mode == modeFilter {
		m.filterInput.Blur()
		m.mode = modeNormal
	}

	switch event.Button {
	case tea.MouseButtonLeft:
		now := time.Now()
		key := keyForRow(row)
		// Match a double click by screen position, then relocate the original
		// row by identity so that a refresh cannot open a neighboring row.
		if m.lastClick.x == event.X && m.lastClick.y == event.Y && now.Sub(m.lastClick.at) <= doubleClickWindow {
			originalIdx, found := m.indexOfRow(m.lastClick.key, m.lastClick.pid)
			m.lastClick = mouseClick{}
			if !found {
				return m, nil
			}
			m.cursor = originalIdx
			row = m.filtered[originalIdx]
			if row.PID == 0 {
				m.setStatus(unresolvedHint(), true)
				return m, nil
			}
			m.mode = modeDetail
			m.detailInfo = nil
			m.detailErr = nil
			return m, loadDetailCmd(row.PID)
		}
		m.lastClick = mouseClick{key: key, pid: row.PID, x: event.X, y: event.Y, at: now}
	case tea.MouseButtonRight:
		if row.PID == 0 {
			m.setStatus(unresolvedHint(), true)
		} else {
			m.killTarget = row
			m.mode = modeConfirmKill
		}
	case tea.MouseButtonMiddle:
		url := openurl.URLForPort(row.LocalPort)
		if err := openurl.Open(url); err != nil {
			m.setStatus("could not open "+url+": "+err.Error(), true)
		} else {
			m.setStatus("opened "+url, false)
		}
	}
	return m, nil
}

func (m Model) indexOfRow(key app.Key, pid int) (int, bool) {
	for i, row := range m.filtered {
		if row.PID == pid && keyForRow(row) == key {
			return i, true
		}
	}
	return 0, false
}

func (m Model) tableHeaderY() int {
	y := appContentY + 1 // title occupies the first content row
	if m.mode == modeFilter {
		y++
	}
	return y
}

func (m Model) rowAt(x, y int) (int, bool) {
	if x < appContentX || x >= appContentX+innerWidth(m.width) {
		return 0, false
	}
	rowOnScreen := y - (m.tableHeaderY() + 2) // header and divider
	visible := m.visibleRows()
	if rowOnScreen < 0 || rowOnScreen >= len(visible.rows) {
		return 0, false
	}
	return visible.offset + rowOnScreen, true
}

func (m Model) sortModeAt(x int) (sortMode, bool) {
	relativeX := x - appContentX - markerWidth
	if relativeX < 0 {
		return 0, false
	}
	for _, col := range columnsFor(m.showEstablished, m.width) {
		if relativeX < col.width {
			return sortModeForColumn(col.title)
		}
		relativeX -= col.width
	}
	return 0, false
}
