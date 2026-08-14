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
		b.WriteString(styleTag.Render("❯ ") + m.filterInput.View())
		b.WriteByte('\n')
	}
	b.WriteString(m.renderTable())
	b.WriteByte('\n')
	b.WriteString(styleDivider.Render(strings.Repeat("─", innerWidth(m.width))))
	b.WriteByte('\n')
	b.WriteString(m.renderStatusBar())

	// lipgloss's Width() sizes the padded content box, with the border
	// added outside that — so the box needs the padding (2 cols) added
	// back on top of innerWidth (which is what the divider/table lines
	// inside are actually sized to).
	base := styleAppBorder.Width(innerWidth(m.width) + 2).Render(b.String())

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

// innerWidth is the content width available inside the app's outer
// border + 1-column padding on each side.
func innerWidth(termWidth int) int {
	w := termWidth - 4
	if w < 40 {
		w = 40
	}
	return w
}

func (m Model) renderTitle() string {
	badge := styleBadge.Render("portop")
	tagline := styleMuted.Render(" what's really using your ports?")

	scope := "LISTEN"
	if m.showEstablished {
		scope = "LISTEN+ESTABLISHED"
	}
	tags := styleTag.Render("["+scope+"]") + " " +
		styleTag.Render(fmt.Sprintf("[%s]", m.ipFilter)) + " " +
		styleTag.Render(fmt.Sprintf("[sort: %s]", m.sort))

	return badge + tagline + "   " + tags
}

func (m Model) renderStatusBar() string {
	count := fmt.Sprintf("%d/%d sockets", len(m.filtered), len(m.rows))

	var msg string
	if m.statusMsg != "" {
		if m.statusIsErr {
			msg = styleDanger.Render(m.statusMsg)
		} else {
			msg = styleOk.Render(m.statusMsg)
		}
	} else if m.lastErr != nil {
		msg = styleDanger.Render("scan error: " + m.lastErr.Error())
	}

	line1 := styleMuted.Render(count) + "   " + msg
	line2 := renderKeyHints()
	return line1 + "\n" + line2
}

func renderKeyHints() string {
	hints := [][2]string{
		{"enter", "details"}, {"k", "kill"}, {"o", "open"}, {"f", "search"},
		{"v", "ipv4/6"}, {"e", "established"}, {"s", "sort"}, {"c", "copy"},
		{"n", "clear new"}, {"?", "help"}, {"q", "quit"},
	}
	parts := make([]string, len(hints))
	for i, h := range hints {
		parts[i] = styleKey.Render(h[0]) + styleFaint.Render(" "+h[1])
	}
	return strings.Join(parts, styleFaint.Render("  ·  "))
}

func (m Model) renderDetail() string {
	row, ok := m.selected()
	if !ok {
		return styleModal.Render("no row selected")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", styleTitle.Render("Process details"))
	fmt.Fprintf(&b, "%s %s :%d (%s)\n\n",
		stateStyle(string(row.State)).Render("●"), row.ProcessName, row.LocalPort, row.Protocol)

	if m.detailErr != nil {
		b.WriteString(styleDanger.Render("could not read /proc: " + m.detailErr.Error()))
	} else if m.detailInfo == nil {
		b.WriteString(styleMuted.Render("loading..."))
	} else {
		info := m.detailInfo
		field := func(label, val string) {
			fmt.Fprintf(&b, "%s %s\n", styleTag.Render(padTrunc(label, 11)), val)
		}
		field("PID:", strconv.Itoa(info.PID))
		field("User:", orDash(info.User))
		field("Cmdline:", orDash(info.Cmdline))
		field("Executable:", orDash(info.Exe))
		field("Cwd:", orDash(info.Cwd))
		field("Threads:", strconv.Itoa(info.NumThreads))
		field("Open files:", strconv.Itoa(info.OpenFiles))
		field("RSS:", fmt.Sprintf("%.1f MiB", float64(info.RSSBytes)/1024/1024))
		if !info.StartTime.IsZero() {
			field("Started:", info.StartTime.Format("2006-01-02 15:04:05"))
		}
		if row.SystemdUnit != "" {
			field("Systemd:", row.SystemdUnit)
		}
		if row.ContainerName != "" {
			field("Container:", row.ContainerName)
		}
	}

	b.WriteString("\n" + styleMuted.Render("esc/enter to close"))
	return styleModal.Width(m.modalWidth()).Render(b.String())
}

func (m Model) renderConfirmKill() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", styleDanger.Render("Kill this process?"))
	fmt.Fprintf(&b, "%s (PID %s) listening on :%d\n\n",
		m.killTarget.ProcessName, strconv.Itoa(m.killTarget.PID), m.killTarget.LocalPort)
	b.WriteString(styleKey.Render("y") + styleFaint.Render(" terminate (SIGTERM)   "))
	b.WriteString(styleKey.Render("f") + styleFaint.Render(" force (SIGKILL)   "))
	b.WriteString(styleKey.Render("esc") + styleFaint.Render(" cancel"))
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
