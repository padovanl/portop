package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder
	b.WriteString(m.renderTitle())
	b.WriteByte('\n')
	if m.mode == modeFilter {
		b.WriteString("search: " + m.filterInput.View())
		b.WriteByte('\n')
	}
	b.WriteString(m.renderTable())
	b.WriteByte('\n')
	b.WriteString(m.renderStatusBar())

	base := b.String()

	switch m.mode {
	case modeDetail:
		return overlay(base, m.renderDetail(), m.width, m.height)
	case modeConfirmKill:
		return overlay(base, m.renderConfirmKill(), m.width, m.height)
	case modeHelp:
		return overlay(base, m.renderHelp(), m.width, m.height)
	}
	return base
}

func (m Model) renderTitle() string {
	title := styleTitle.Render("portop") + styleMuted.Render(" — what's really using your ports?")
	filters := fmt.Sprintf("[%s] [sort: %s]", m.ipFilter, m.sort)
	if m.showEstablished {
		filters = "[LISTEN+ESTABLISHED] " + filters
	} else {
		filters = "[LISTEN] " + filters
	}
	return title + "   " + styleMuted.Render(filters)
}

func (m Model) renderStatusBar() string {
	count := fmt.Sprintf("%d/%d sockets", len(m.filtered), len(m.rows))
	help := "enter details · k kill · o open · f search · v ipv4/6 · e established · s sort · c copy · n clear new · ? help · q quit"

	var msg string
	if m.statusMsg != "" {
		if m.statusIsErr {
			msg = styleDanger.Render(m.statusMsg)
		} else {
			msg = styleMuted.Render(m.statusMsg)
		}
	} else if m.lastErr != nil {
		msg = styleDanger.Render("scan error: " + m.lastErr.Error())
	}

	line1 := styleMuted.Render(count) + "   " + msg
	line2 := styleMuted.Render(help)
	return styleStatusBar.Width(maxInt(20, m.width)).Render(line1 + "\n" + line2)
}

func (m Model) renderDetail() string {
	row, ok := m.selected()
	if !ok {
		return styleModal.Render("no row selected")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", styleTitle.Render("Process details"))
	fmt.Fprintf(&b, "%s :%d (%s)\n\n", row.ProcessName, row.LocalPort, row.Protocol)

	if m.detailErr != nil {
		b.WriteString(styleDanger.Render("could not read /proc: " + m.detailErr.Error()))
	} else if m.detailInfo == nil {
		b.WriteString(styleMuted.Render("loading..."))
	} else {
		info := m.detailInfo
		fmt.Fprintf(&b, "PID:        %d\n", info.PID)
		fmt.Fprintf(&b, "User:       %s\n", orDash(info.User))
		fmt.Fprintf(&b, "Cmdline:    %s\n", orDash(info.Cmdline))
		fmt.Fprintf(&b, "Executable: %s\n", orDash(info.Exe))
		fmt.Fprintf(&b, "Cwd:        %s\n", orDash(info.Cwd))
		fmt.Fprintf(&b, "Threads:    %d\n", info.NumThreads)
		fmt.Fprintf(&b, "Open files: %d\n", info.OpenFiles)
		fmt.Fprintf(&b, "RSS:        %.1f MiB\n", float64(info.RSSBytes)/1024/1024)
		if !info.StartTime.IsZero() {
			fmt.Fprintf(&b, "Started:    %s\n", info.StartTime.Format("2006-01-02 15:04:05"))
		}
		if row.SystemdUnit != "" {
			fmt.Fprintf(&b, "Systemd:    %s\n", row.SystemdUnit)
		}
		if row.ContainerName != "" {
			fmt.Fprintf(&b, "Container:  %s\n", row.ContainerName)
		}
	}

	b.WriteString("\n" + styleMuted.Render("esc/enter to close"))
	return styleModal.Width(m.modalWidth()).Render(b.String())
}

func (m Model) renderConfirmKill() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", styleTitle.Render("Kill this process?"))
	fmt.Fprintf(&b, "%s (PID %s) listening on :%d\n\n",
		m.killTarget.ProcessName, strconv.Itoa(m.killTarget.PID), m.killTarget.LocalPort)
	b.WriteString(styleKey.Render("y") + " terminate (SIGTERM)   ")
	b.WriteString(styleKey.Render("f") + " force (SIGKILL)   ")
	b.WriteString(styleKey.Render("esc") + " cancel")
	return styleModal.Width(m.modalWidth()).Render(b.String())
}

func (m Model) renderHelp() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("Keybindings") + "\n\n")
	rows := [][2]string{
		{"↑ / ↓", "move the cursor"},
		{"g / G", "jump to top / bottom"},
		{"enter", "process details"},
		{"k", "kill process (then y=SIGTERM, f=SIGKILL)"},
		{"o", "open http(s)://localhost:PORT in the browser"},
		{"f  /", "filter/search by port, process or PID"},
		{"v", "toggle IPv4 / IPv6 / both"},
		{"e", "show/hide ESTABLISHED connections"},
		{"s", "cycle sort column"},
		{"c", "copy selected row to clipboard"},
		{"n", "clear the new-port highlight"},
		{"r", "refresh now"},
		{"?", "this help"},
		{"q", "quit"},
	}
	for _, r := range rows {
		fmt.Fprintf(&b, "%s  %s\n", styleKey.Render(padTrunc(r[0], 8)), r[1])
	}
	b.WriteString("\n" + styleMuted.Render("press any key to close"))
	return styleModal.Width(m.modalWidth()).Render(b.String())
}

// modalWidth keeps popups a comfortable, mostly-fixed size instead of
// stretching to the width of their widest line (e.g. a long cmdline in
// the detail view) or the full terminal width.
func (m Model) modalWidth() int {
	w := m.width - 12
	if w > 88 {
		w = 88
	}
	if w < 40 {
		w = 40
	}
	return w
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// overlay centers `top` over `base`. Bubble Tea has no built-in
// popup/layer primitive, so we render the modal as its own block below
// the base view; this keeps behavior identical across terminal widths
// without needing manual ANSI cursor positioning.
func overlay(base, top string, width, height int) string {
	return base + "\n\n" + lipgloss.PlaceHorizontal(maxInt(20, width), lipgloss.Center, top)
}
