package ui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/padovan93/portop/internal/app"
)

func sortRows(rows []app.Row, mode sortMode) {
	sort.SliceStable(rows, func(i, j int) bool {
		switch mode {
		case sortByProcess:
			if rows[i].ProcessName != rows[j].ProcessName {
				return rows[i].ProcessName < rows[j].ProcessName
			}
		case sortByPID:
			if rows[i].PID != rows[j].PID {
				return rows[i].PID < rows[j].PID
			}
		case sortByCPU:
			if rows[i].CPUPercent != rows[j].CPUPercent {
				return rows[i].CPUPercent > rows[j].CPUPercent
			}
		default: // sortByPort
			if rows[i].LocalPort != rows[j].LocalPort {
				return rows[i].LocalPort < rows[j].LocalPort
			}
		}
		return rows[i].Protocol < rows[j].Protocol
	})
}

type column struct {
	title string
	width int
}

func columnsFor(showEstablished bool) []column {
	cols := []column{
		{"PORT", 11},
		{"PROTO", 5},
		{"STATE", 11},
		{"PROCESS", 18},
		{"PID", 7},
		{"CPU", 6},
	}
	if showEstablished {
		cols = append(cols, column{"REMOTE", 30})
	}
	cols = append(cols, column{"SYSTEMD", 16}, column{"CONTAINER", 16})
	return cols
}

func (m Model) renderTable() string {
	cols := columnsFor(m.showEstablished)

	var b strings.Builder
	for _, c := range cols {
		fmt.Fprint(&b, styleHeader.Render(padTrunc(c.title, c.width)))
	}
	b.WriteByte('\n')

	if len(m.filtered) == 0 {
		b.WriteString(styleMuted.Render("  no ports match the current filters"))
		return b.String()
	}

	visible := m.visibleRows()
	for i, r := range visible.rows {
		idx := visible.offset + i
		line := m.renderRow(r, cols)
		if idx == m.cursor {
			b.WriteString(styleRowSelected.Render(line))
		} else if m.newSeen[keyForRow(r)] {
			b.WriteString(styleRowNew.Render(line))
		} else {
			b.WriteString(styleRow.Render(line))
		}
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

type rowWindow struct {
	rows   []app.Row
	offset int
}

// visibleRows returns the slice of m.filtered that fits the current
// terminal height, scrolled to keep the cursor visible.
func (m Model) visibleRows() rowWindow {
	maxVisible := m.height - 8 // header, column titles, status bar, margins
	if maxVisible < 3 {
		maxVisible = 3
	}
	if len(m.filtered) <= maxVisible {
		return rowWindow{rows: m.filtered, offset: 0}
	}
	start := m.cursor - maxVisible/2
	if start < 0 {
		start = 0
	}
	if start+maxVisible > len(m.filtered) {
		start = len(m.filtered) - maxVisible
	}
	return rowWindow{rows: m.filtered[start : start+maxVisible], offset: start}
}

func (m Model) renderRow(r app.Row, cols []column) string {
	var b strings.Builder
	for _, c := range cols {
		var val string
		switch c.title {
		case "PORT":
			val = ":" + strconv.Itoa(int(r.LocalPort))
		case "PROTO":
			val = string(r.Protocol)
			if r.IPv6 {
				val += "6"
			}
		case "STATE":
			val = string(r.State)
		case "PROCESS":
			val = r.ProcessName
			if val == "" {
				val = "-"
			}
		case "PID":
			if r.PID == 0 {
				val = "-"
			} else {
				val = strconv.Itoa(r.PID)
			}
		case "CPU":
			val = fmt.Sprintf("%.1f%%", r.CPUPercent)
		case "REMOTE":
			val = remoteDisplay(r)
		case "SYSTEMD":
			val = r.SystemdUnit
			if val == "" {
				val = "-"
			}
		case "CONTAINER":
			val = r.ContainerName
			if val == "" {
				val = "-"
			}
		}
		fmt.Fprint(&b, padTrunc(val, c.width))
	}
	return b.String()
}

func remoteDisplay(r app.Row) string {
	if r.RemotePort == 0 {
		return "-"
	}
	host := r.RemoteHost
	if host == "" {
		host = r.RemoteAddr.String()
	}
	return host + ":" + strconv.Itoa(int(r.RemotePort))
}

func padTrunc(s string, width int) string {
	runes := []rune(s)
	if len(runes) > width-1 {
		if width-2 > 0 {
			runes = append(runes[:width-2], '…')
		} else {
			runes = runes[:width]
		}
	}
	s = string(runes)
	if pad := width - len([]rune(s)); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}
