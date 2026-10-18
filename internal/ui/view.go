package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"

	"github.com/padovanl/portop/internal/terminaltext"
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
	case modeSettings:
		return overlay(base, m.renderSettings(), m.width, m.height)
	}
	return base
}

// innerWidth is the content width available inside the app's outer
// border + 1-column padding on each side.
func innerWidth(termWidth int) int {
	return max(termWidth-4, 40)
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

	compact := badge + "   " + tags
	full := badge + tagline + "   " + tags
	if lipgloss.Width(full) <= innerWidth(m.width) {
		return full
	}
	return compact
}

func (m Model) renderStatusBar() string {
	count := fmt.Sprintf("%d/%d sockets", len(m.filtered), len(m.rows))

	var msg string
	switch {
	case m.statusMsg != "" && m.statusIsErr:
		msg = styleDanger.Render(terminaltext.Sanitize(m.statusMsg))
	case m.statusMsg != "":
		msg = styleOk.Render(terminaltext.Sanitize(m.statusMsg))
	case m.lastErr != nil:
		msg = styleDanger.Render(terminaltext.Sanitize("scan error: " + m.lastErr.Error()))
	default:
		if hint := unresolvedStatusHint(m.filtered); hint != "" {
			msg = styleFaint.Render(hint)
		}
	}

	line1 := styleMuted.Render(count) + "   " + msg
	line2 := renderKeyHints()
	return line1 + "\n" + line2
}

func renderKeyHints() string {
	hints := []struct {
		binding key.Binding
		desc    string
	}{
		{keys.Enter, "details"}, {keys.Kill, "kill"}, {keys.Open, "open"}, {keys.Filter, "search"},
		{keys.Protocol, "ipv4/6"}, {keys.Established, "established"}, {keys.Sort, "sort"}, {keys.Copy, "copy"},
		{keys.NewMark, "clear new"}, {keys.Help, "help"}, {keys.Quit, "quit"},
	}
	parts := make([]string, len(hints))
	for i, h := range hints {
		label := h.binding.Keys()[0]
		parts[i] = styleKey.Render(label) + styleFaint.Render(" "+h.desc)
	}
	return strings.Join(parts, styleFaint.Render("  ·  "))
}

func (m Model) renderDetail() string {
	row := m.detailRow

	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", styleTitle.Render("Process details"))
	fmt.Fprintf(&b, "%s %s :%d (%s)\n\n",
		stateStyle(string(row.State)).Render("●"), terminaltext.Sanitize(row.ProcessName), row.LocalPort, row.Protocol)

	if m.detailErr != nil {
		b.WriteString(styleDanger.Render(terminaltext.Sanitize("could not read process details: " + m.detailErr.Error())))
	} else if m.detailInfo == nil {
		b.WriteString(styleMuted.Render("loading..."))
	} else {
		info := m.detailInfo
		// Labels get a fixed column wide enough for the longest one, and a
		// long value (a command line) wraps inside its own column.
		const labelWidth = 13
		valueWidth := max(m.modalWidth()-4-2-labelWidth, 10) // border, padding
		field := func(label, val string) {
			value := lipgloss.NewStyle().Width(valueWidth).Render(terminaltext.Sanitize(val))
			b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, styleTag.Render(padTrunc(label, labelWidth)), value) + "\n")
		}
		field("PID:", strconv.Itoa(info.PID))
		if len(row.Workers) > 0 {
			field("Workers:", workerList(row.Workers))
		}
		field("User:", orDash(info.User))
		field("Cmdline:", orDash(info.Cmdline))
		field("Executable:", orDash(info.Exe))
		field("Cwd:", orDash(info.Cwd))
		threads := "-"
		if info.NumThreads > 0 {
			threads = strconv.Itoa(info.NumThreads)
		}
		field("Threads:", threads)
		openFiles := "-"
		if info.OpenFiles >= 0 {
			openFiles = strconv.Itoa(info.OpenFiles)
		}
		field("Open files:", openFiles)
		field("RSS:", fmt.Sprintf("%.1f MiB", float64(info.RSSBytes)/1024/1024))
		if !info.StartTime.IsZero() {
			field("Started:", info.StartTime.Format("2006-01-02 15:04:05"))
		}
		if row.SystemdUnit != "" {
			field(platformLabels.unitField, row.SystemdUnit)
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
	fmt.Fprintf(&b, "%s (PID %s) listening on :%d\n",
		terminaltext.Sanitize(m.killTarget.ProcessName), strconv.Itoa(m.killTarget.PID), m.killTarget.LocalPort)
	if n := len(m.killTarget.Workers); n > 0 {
		b.WriteString(styleMuted.Render(fmt.Sprintf("main process of %d worker(s) sharing the port; they usually exit with it", n)) + "\n")
	}
	b.WriteString("\n")
	b.WriteString(styleKey.Render("y") + styleFaint.Render(" "+platformLabels.terminate+"   "))
	b.WriteString(styleKey.Render("f") + styleFaint.Render(" "+platformLabels.force+"   "))
	b.WriteString(styleKey.Render(keys.Escape.Keys()[0]) + styleFaint.Render(" cancel"))
	return styleModal.Width(m.modalWidth()).Render(b.String())
}

func (m Model) renderHelp() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("Keybindings") + "\n\n")
	rows := []struct {
		binding key.Binding
		desc    string
	}{
		{keys.Up, "move up"},
		{keys.Down, "move down"},
		{keys.PageUp, "move one page up"},
		{keys.PageDown, "move one page down"},
		{keys.Top, "jump to top"},
		{keys.Bottom, "jump to bottom"},
		{keys.Enter, "process details"},
		{keys.Kill, platformLabels.killHelp},
		{keys.Open, "open http(s)://HOST:PORT in the browser"},
		{keys.Filter, "filter/search by port, process or PID"},
		{keys.Protocol, "toggle IPv4 / IPv6 / both"},
		{keys.Established, "show/hide ESTABLISHED connections"},
		{keys.Sort, "cycle sort column"},
		{keys.Copy, "copy selected row to clipboard"},
		{keys.NewMark, "clear the new-port highlight"},
		{keys.Refresh, "refresh now"},
		{keys.Help, "this help"},
		{keys.Settings, "open settings"},
		{keys.Quit, "quit"},
	}
	for _, r := range rows {
		label := strings.Join(r.binding.Keys(), "/")
		fmt.Fprintf(&b, "%s  %s\n", styleKey.Render(padTrunc(label, 10)), r.desc)
	}
	b.WriteString("\n" + styleMuted.Render("Listening port color: ") + styleDanger.Render("every interface") +
		styleMuted.Render(" · ") + styleWarn.Render("one address") + styleMuted.Render(" · ") + styleOk.Render("loopback only") + "\n")
	b.WriteString("\n" + styleMuted.Render("press any key to close"))
	return styleModal.Width(m.modalWidth()).Render(b.String())
}

// renderSettings draws the "," settings screen: a live theme picker and
// the full remappable keybinding list, both editable in place.
func (m Model) renderSettings() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("Settings") + "\n\n")

	themeLine := fmt.Sprintf("Theme: %-12s %d/%d", ThemeNames[m.settingsThemeIdx], m.settingsThemeIdx+1, len(ThemeNames))
	if m.settingsCursor == settingsThemeRow {
		b.WriteString(styleRowSelected.Render(themeLine) + "\n\n")
	} else {
		b.WriteString(themeLine + "\n\n")
	}

	n := len(KeyActions)
	// Fixed chrome around the action list: modal border+padding (4),
	// title+blank (2), theme+blank (2), the tag line (1), blank+reset
	// (2), blank+hint (2) — the rest of the modal's height budget goes
	// to action rows, scrolled to keep the cursor in view exactly like
	// the main table does.
	maxRows := max(3, m.height-13)
	start := 0
	if n > maxRows {
		center := m.settingsCursor - 1
		if center < 0 {
			center = 0
		}
		if center > n-1 {
			center = n - 1
		}
		start = center - maxRows/2
		if start < 0 {
			start = 0
		}
		if start+maxRows > n {
			start = n - maxRows
		}
	}
	end := min(start+maxRows, n)

	tag := styleTag.Render("Keybindings") + styleFaint.Render(" (enter to rebind)")
	if n > maxRows && m.settingsCursor >= 1 && m.settingsCursor <= n {
		tag += styleFaint.Render(fmt.Sprintf("  %d/%d", m.settingsCursor, n))
	}
	b.WriteString(tag + "\n")

	bindings := CurrentKeyBindings()
	for i := start; i < end; i++ {
		action := KeyActions[i]
		row := i + 1
		label := strings.Join(bindings[action], "/")
		if m.settingsCapturing && m.settingsCursor == row {
			label = styleDanger.Render("press a key...")
		}
		line := fmt.Sprintf("%-28s %s", actionDesc[action], label)
		if m.settingsCursor == row {
			b.WriteString(styleRowSelected.Render(line) + "\n")
		} else {
			b.WriteString(line + "\n")
		}
	}

	resetLine := "Reset keybindings to defaults"
	if m.settingsCursor == settingsResetRow() {
		b.WriteString("\n" + styleRowSelected.Render(resetLine) + "\n")
	} else {
		b.WriteString("\n" + styleMuted.Render(resetLine) + "\n")
	}

	esc := keys.Escape.Keys()[0]
	hint := "↑/↓ move · ←/→ browse themes · enter rebind/reset · " + esc + " close"
	if m.settingsCapturing {
		hint = "press any key to bind it · " + esc + " to cancel"
	}
	if m.settingsNote != "" {
		b.WriteString("\n" + styleDanger.Render(m.settingsNote))
	}
	b.WriteString("\n" + styleMuted.Render(hint))
	return styleModal.Width(m.modalWidth()).Render(b.String())
}

// modalWidth keeps popups a comfortable, mostly-fixed size instead of
// stretching to the width of their widest line (e.g. a long cmdline in
// the detail view) or the full terminal width.
func (m Model) modalWidth() int {
	return min(max(m.width-12, 40), 88)
}

// workerList shows up to eight worker PIDs and how many more there are.
func workerList(pids []int) string {
	const shown = 8
	parts := make([]string, 0, shown+1)
	for i, pid := range pids {
		if i == shown {
			parts = append(parts, "+"+strconv.Itoa(len(pids)-shown)+" more")
			break
		}
		parts = append(parts, strconv.Itoa(pid))
	}
	return strconv.Itoa(len(pids)) + " (" + strings.Join(parts, ", ") + ")"
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// overlay centers a modal within the terminal-sized base view.
func overlay(base, modal string, width, height int) string {
	baseLines := strings.Split(base, "\n")
	// Pad short tables so the modal still uses the full terminal height.
	for len(baseLines) < height {
		baseLines = append(baseLines, "")
	}
	modalLines := strings.Split(modal, "\n")

	top := (len(baseLines) - len(modalLines)) / 2
	if top < 0 {
		top = 0
	}

	left := (max(20, width) - lipgloss.Width(modal)) / 2
	if left < 0 {
		left = 0
	}
	pad := strings.Repeat(" ", left)

	for i, line := range modalLines {
		row := top + i
		if row < 0 || row >= len(baseLines) {
			continue
		}
		baseLines[row] = pad + line
	}
	return strings.Join(baseLines, "\n")
}
