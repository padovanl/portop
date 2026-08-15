package ui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/padovanl/portop/internal/app"
	"github.com/padovanl/portop/internal/scanner"
	"github.com/padovanl/portop/internal/services"
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

// markerWidth is the leading gutter reserved on every row for the
// selection chevron (▸) or new-port dot (●), so columns stay aligned
// whether or not a given row has a marker.
const markerWidth = 2

// columnsFor picks which columns to display for the given terminal
// width: the core columns (port through CPU%) always fit even in a
// narrow 80-column terminal, and REMOTE/CONTAINER/SYSTEMD are added
// back in, most-useful-first, only as space allows — rather than
// letting rows overflow the box and wrap mid-line. CONTAINER comes
// before SYSTEMD since Docker's a lot more common in day-to-day port
// debugging than needing the systemd unit name.
func columnsFor(showEstablished bool, width int) []column {
	core := []column{
		{"PORT", 11},
		{"PROTO", 5},
		{"STATE", 12},
		{"PROCESS", 18},
		{"PID", 7},
		{"CPU", 6},
	}
	optional := []column{{"CONTAINER", 16}, {"SYSTEMD", 16}}
	if showEstablished {
		optional = append([]column{{"REMOTE", 30}}, optional...)
	}

	budget := innerWidth(width) - markerWidth
	cols := core
	used := totalWidth(core)
	for _, c := range optional {
		if used+c.width > budget {
			break
		}
		cols = append(cols, c)
		used += c.width
	}
	return cols
}

func totalWidth(cols []column) int {
	w := 0
	for _, c := range cols {
		w += c.width
	}
	return w
}

func (m Model) renderTable() string {
	cols := columnsFor(m.showEstablished, m.width)

	var b strings.Builder
	b.WriteString(strings.Repeat(" ", markerWidth))
	for _, c := range cols {
		fmt.Fprint(&b, styleHeader.Render(padTrunc(c.title, c.width)))
	}
	b.WriteByte('\n')
	b.WriteString(styleDivider.Render(strings.Repeat("─", innerWidth(m.width))))
	b.WriteByte('\n')

	if len(m.filtered) == 0 {
		b.WriteString(styleMuted.Render("  no ports match the current filters"))
		return b.String()
	}

	visible := m.visibleRows()
	for i, r := range visible.rows {
		idx := visible.offset + i
		isSelected := idx == m.cursor
		isNew := m.newSeen[keyForRow(r)]
		isZebra := i%2 == 1

		switch {
		case isSelected:
			line := cursorMarker() + plainRow(r, cols)
			b.WriteString(styleRowSelected.Render(line))
		case isNew:
			line := newMarker() + plainRow(r, cols)
			b.WriteString(styleRowNew.Render(line))
		case isZebra:
			line := strings.Repeat(" ", markerWidth) + coloredRow(r, cols)
			b.WriteString(styleRowZebra.Render(line))
		default:
			b.WriteString(strings.Repeat(" ", markerWidth) + coloredRow(r, cols))
		}
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

func cursorMarker() string { return styleCursor.Render("▸") + " " }
func newMarker() string    { return styleRowNew.Render("●") + " " }

type rowWindow struct {
	rows   []app.Row
	offset int
}

// visibleRows returns the slice of m.filtered that fits the current
// terminal height, scrolled to keep the cursor visible.
func (m Model) visibleRows() rowWindow {
	maxVisible := m.height - 9 // app border, title/filter, column header+divider, status bar+divider
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

// cellValue returns the raw (unpadded, unstyled) text for a column.
func cellValue(r app.Row, colTitle string) string {
	switch colTitle {
	case "PORT":
		v := ":" + strconv.Itoa(int(r.LocalPort))
		if r.State == scanner.StateListen {
			if name, ok := services.Lookup(r.LocalPort, string(r.Protocol)); ok {
				v += " " + name
			}
		}
		return v
	case "PROTO":
		v := string(r.Protocol)
		if r.IPv6 {
			v += "6"
		}
		return v
	case "STATE":
		return string(r.State)
	case "PROCESS":
		if r.ProcessName == "" {
			return "-"
		}
		return r.ProcessName
	case "PID":
		if r.PID == 0 {
			return "-"
		}
		return strconv.Itoa(r.PID)
	case "CPU":
		return fmt.Sprintf("%.1f%%", r.CPUPercent)
	case "REMOTE":
		return remoteDisplay(r)
	case "SYSTEMD":
		if r.SystemdUnit == "" {
			return "-"
		}
		return r.SystemdUnit
	case "CONTAINER":
		if r.ContainerName == "" {
			return "-"
		}
		return r.ContainerName
	}
	return ""
}

// plainRow renders a row with no per-cell colors, for rows whose
// selection/new-port highlight already dictates the whole line's color.
func plainRow(r app.Row, cols []column) string {
	var b strings.Builder
	for _, c := range cols {
		fmt.Fprint(&b, padTrunc(cellValue(r, c.title), c.width))
	}
	return b.String()
}

// coloredRow renders a row with semantic per-cell coloring: state,
// protocol and CPU load each get their own color so the table reads at
// a glance without following the cursor.
func coloredRow(r app.Row, cols []column) string {
	var b strings.Builder
	for _, c := range cols {
		padded := padTrunc(cellValue(r, c.title), c.width)
		switch c.title {
		case "STATE":
			fmt.Fprint(&b, stateStyle(string(r.State)).Render(padded))
		case "PROTO":
			fmt.Fprint(&b, protoStyle(string(r.Protocol)).Render(padded))
		case "CPU":
			fmt.Fprint(&b, cpuStyle(r.CPUPercent).Render(padded))
		default:
			fmt.Fprint(&b, padded)
		}
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
