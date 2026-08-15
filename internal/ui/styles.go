package ui

import "github.com/charmbracelet/lipgloss"

var (
	colorAccent   = lipgloss.AdaptiveColor{Light: "#0060D0", Dark: "#7DD3FC"}
	colorAccent2  = lipgloss.AdaptiveColor{Light: "#7C3AED", Dark: "#C4B5FD"}
	colorMuted    = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#8A93A6"}
	colorFaint    = lipgloss.AdaptiveColor{Light: "#9AA4B2", Dark: "#5B6478"}
	colorNew      = lipgloss.AdaptiveColor{Light: "#B4530A", Dark: "#FFB454"}
	colorDanger   = lipgloss.AdaptiveColor{Light: "#B00020", Dark: "#FF6B6B"}
	colorWarn     = lipgloss.AdaptiveColor{Light: "#946800", Dark: "#F5D272"}
	colorOk       = lipgloss.AdaptiveColor{Light: "#0F7A3D", Dark: "#5CE0A0"}
	colorTCP      = lipgloss.AdaptiveColor{Light: "#0060D0", Dark: "#7DD3FC"}
	colorUDP      = lipgloss.AdaptiveColor{Light: "#7C3AED", Dark: "#D8B4FE"}
	colorSelected = lipgloss.AdaptiveColor{Light: "#DCEBFF", Dark: "#1B3350"}
	colorZebra    = lipgloss.AdaptiveColor{Light: "#F4F6F9", Dark: "#171C27"}
	colorBorder   = lipgloss.AdaptiveColor{Light: "#D0D5DD", Dark: "#2E3648"}

	styleAppBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent2).
			Padding(0, 1)

	styleBadge = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#0B0E14")).
			Background(colorAccent).
			Padding(0, 1)

	styleTitle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)

	styleTag = lipgloss.NewStyle().
			Foreground(colorAccent2).
			Bold(true)

	styleHeader = lipgloss.NewStyle().Bold(true).Foreground(colorAccent2)

	styleDivider = lipgloss.NewStyle().Foreground(colorBorder)

	styleRowZebra = lipgloss.NewStyle().Background(colorZebra)

	styleRowSelected = lipgloss.NewStyle().
				Background(colorSelected).
				Bold(true)

	styleRowNew = lipgloss.NewStyle().Foreground(colorNew).Bold(true)

	styleMuted = lipgloss.NewStyle().Foreground(colorMuted)
	styleFaint = lipgloss.NewStyle().Foreground(colorFaint)

	styleDanger = lipgloss.NewStyle().Foreground(colorDanger).Bold(true)
	styleOk     = lipgloss.NewStyle().Foreground(colorOk)
	styleWarn   = lipgloss.NewStyle().Foreground(colorWarn)

	styleStateListen      = lipgloss.NewStyle().Foreground(colorOk).Bold(true)
	styleStateEstablished = lipgloss.NewStyle().Foreground(colorAccent)
	styleStateTransient   = lipgloss.NewStyle().Foreground(colorWarn)
	styleStateOther       = lipgloss.NewStyle().Foreground(colorFaint)

	styleProtoTCP = lipgloss.NewStyle().Foreground(colorTCP).Bold(true)
	styleProtoUDP = lipgloss.NewStyle().Foreground(colorUDP).Bold(true)

	styleModal = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent2).
			Padding(1, 2)

	styleKey = lipgloss.NewStyle().Foreground(colorAccent2).Bold(true)

	styleCursor = lipgloss.NewStyle().Foreground(colorAccent2).Bold(true)
)

// cpuStyle returns a heat-scaled style for a CPU percentage, so a process
// pegging the core jumps out without needing to read the number.
func cpuStyle(pct float64) lipgloss.Style {
	switch {
	case pct >= 20:
		return styleDanger
	case pct >= 5:
		return styleWarn
	case pct > 0:
		return lipgloss.NewStyle()
	default:
		return styleFaint
	}
}

// stateStyle color-codes the STATE column so LISTEN/ESTABLISHED/transient
// states are distinguishable at a glance.
func stateStyle(state string) lipgloss.Style {
	switch state {
	case "LISTEN":
		return styleStateListen
	case "ESTABLISHED":
		return styleStateEstablished
	case "TIME_WAIT", "CLOSE_WAIT", "SYN_SENT", "SYN_RECV", "LAST_ACK", "CLOSING", "FIN_WAIT1", "FIN_WAIT2":
		return styleStateTransient
	default:
		return styleStateOther
	}
}

func protoStyle(proto string) lipgloss.Style {
	if proto == "UDP" {
		return styleProtoUDP
	}
	return styleProtoTCP
}
