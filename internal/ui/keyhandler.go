package ui

import (
	"strconv"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/padovan93/portop/internal/app"
	"github.com/padovan93/portop/internal/openurl"
)

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeFilter:
		return m.handleFilterKey(msg)
	case modeConfirmKill:
		return m.handleConfirmKillKey(msg)
	case modeDetail:
		return m.handleDetailKey(msg)
	case modeHelp:
		return m.handleHelpKey(msg)
	default:
		return m.handleNormalKey(msg)
	}
}

func (m Model) handleNormalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Quit):
		m.quitting = true
		return m, tea.Quit

	case key.Matches(msg, keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}
	case key.Matches(msg, keys.Down):
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
		}
	case key.Matches(msg, keys.Top):
		m.cursor = 0
	case key.Matches(msg, keys.Bottom):
		m.cursor = maxInt(0, len(m.filtered)-1)

	case key.Matches(msg, keys.Enter):
		if row, ok := m.selected(); ok && row.PID != 0 {
			m.mode = modeDetail
			m.detailInfo = nil
			m.detailErr = nil
			return m, loadDetailCmd(row.PID)
		}
		m.setStatus("no process associated with this row", true)

	case key.Matches(msg, keys.Kill):
		if row, ok := m.selected(); ok && row.PID != 0 {
			m.killTarget = row
			m.mode = modeConfirmKill
		} else {
			m.setStatus("no process associated with this row", true)
		}

	case key.Matches(msg, keys.Open):
		if row, ok := m.selected(); ok {
			url := openurl.URLForPort(row.LocalPort)
			if err := openurl.Open(url); err != nil {
				m.setStatus("could not open "+url+": "+err.Error(), true)
			} else {
				m.setStatus("opened "+url, false)
			}
		}

	case key.Matches(msg, keys.Copy):
		if row, ok := m.selected(); ok {
			text := row.ProcessName + " :" + strconv.Itoa(int(row.LocalPort)) + " (PID " + strconv.Itoa(row.PID) + ")"
			if err := clipboard.WriteAll(text); err != nil {
				m.setStatus("clipboard unavailable: "+err.Error(), true)
			} else {
				m.setStatus("copied: "+text, false)
			}
		}

	case key.Matches(msg, keys.Filter):
		m.mode = modeFilter
		return m, m.filterInput.Focus()

	case key.Matches(msg, keys.Protocol):
		m.ipFilter = (m.ipFilter + 1) % 3
		m.refilter()

	case key.Matches(msg, keys.Established):
		m.showEstablished = !m.showEstablished
		m.refilter()

	case key.Matches(msg, keys.Sort):
		m.sort = (m.sort + 1) % sortModeCount
		m.refilter()

	case key.Matches(msg, keys.NewMark):
		m.newSeen = make(map[app.Key]bool)

	case key.Matches(msg, keys.Refresh):
		return m, m.collectCmd()

	case key.Matches(msg, keys.Help):
		m.mode = modeHelp
	}
	return m, nil
}

func (m Model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.filterInput.SetValue("")
		m.filterInput.Blur()
		m.mode = modeNormal
		m.refilter()
		return m, nil
	case "enter":
		m.filterInput.Blur()
		m.mode = modeNormal
		m.refilter()
		return m, nil
	}
	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	m.refilter()
	return m, cmd
}

func (m Model) handleConfirmKillKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		return m, killCmd(m.killTarget, false)
	case "f", "F":
		return m, killCmd(m.killTarget, true)
	default:
		m.mode = modeNormal
	}
	return m, nil
}

func (m Model) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "enter", "q":
		m.mode = modeNormal
	}
	return m, nil
}

func (m Model) handleHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = modeNormal
	return m, nil
}

// refilter recomputes m.filtered from m.rows given the current filter
// text, protocol/state toggles and sort mode, and clamps the cursor.
func (m *Model) refilter() {
	query := strings.ToLower(strings.TrimSpace(m.filterInput.Value()))

	out := make([]app.Row, 0, len(m.rows))
	for _, r := range m.rows {
		if !m.showEstablished && r.State != "LISTEN" {
			continue
		}
		if m.ipFilter == ipv4Only && r.IPv6 {
			continue
		}
		if m.ipFilter == ipv6Only && !r.IPv6 {
			continue
		}
		if query != "" && !rowMatches(r, query) {
			continue
		}
		out = append(out, r)
	}

	sortRows(out, m.sort)

	m.filtered = out
	if m.cursor >= len(m.filtered) {
		m.cursor = maxInt(0, len(m.filtered)-1)
	}
}

func rowMatches(r app.Row, query string) bool {
	if strings.Contains(strconv.Itoa(int(r.LocalPort)), query) {
		return true
	}
	if strings.Contains(strconv.Itoa(r.PID), query) {
		return true
	}
	if strings.Contains(strings.ToLower(r.ProcessName), query) {
		return true
	}
	if strings.Contains(strings.ToLower(r.SystemdUnit), query) {
		return true
	}
	if strings.Contains(strings.ToLower(r.ContainerName), query) {
		return true
	}
	return false
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
