package ui

import "github.com/charmbracelet/lipgloss"

// Palette is the full set of colors a theme controls. Every field is an
// AdaptiveColor pair (light/dark terminal background) except where noted.
type Palette struct {
	Accent, AccentDark     string
	Accent2, Accent2Dark   string
	Muted, MutedDark       string
	Faint, FaintDark       string
	New, NewDark           string
	Danger, DangerDark     string
	Warn, WarnDark         string
	OK, OKDark             string
	TCP, TCPDark           string
	UDP, UDPDark           string
	Selected, SelectedDark string
	Zebra, ZebraDark       string
	Border, BorderDark     string
	BadgeText              string // fixed, not adaptive: text color on the solid accent badge
}

// Themes are the built-in named palettes selectable via config.yml's
// `theme:` key.
var Themes = map[string]Palette{
	"default": {
		Accent: "#0060D0", AccentDark: "#7DD3FC",
		Accent2: "#7C3AED", Accent2Dark: "#C4B5FD",
		Muted: "#6B7280", MutedDark: "#8A93A6",
		Faint: "#9AA4B2", FaintDark: "#5B6478",
		New: "#B4530A", NewDark: "#FFB454",
		Danger: "#B00020", DangerDark: "#FF6B6B",
		Warn: "#946800", WarnDark: "#F5D272",
		OK: "#0F7A3D", OKDark: "#5CE0A0",
		TCP: "#0060D0", TCPDark: "#7DD3FC",
		UDP: "#7C3AED", UDPDark: "#D8B4FE",
		Selected: "#DCEBFF", SelectedDark: "#1B3350",
		Zebra: "#F4F6F9", ZebraDark: "#171C27",
		Border: "#D0D5DD", BorderDark: "#2E3648",
		BadgeText: "#0B0E14",
	},
	"dracula": {
		Accent: "#8BE9FD", AccentDark: "#8BE9FD",
		Accent2: "#BD93F9", Accent2Dark: "#BD93F9",
		Muted: "#6272A4", MutedDark: "#6272A4",
		Faint: "#44475A", FaintDark: "#6272A4",
		New: "#FFB86C", NewDark: "#FFB86C",
		Danger: "#FF5555", DangerDark: "#FF5555",
		Warn: "#F1FA8C", WarnDark: "#F1FA8C",
		OK: "#50FA7B", OKDark: "#50FA7B",
		TCP: "#8BE9FD", TCPDark: "#8BE9FD",
		UDP: "#FF79C6", UDPDark: "#FF79C6",
		Selected: "#44475A", SelectedDark: "#44475A",
		Zebra: "#2A2C3B", ZebraDark: "#282A36",
		Border: "#44475A", BorderDark: "#44475A",
		BadgeText: "#282A36",
	},
	"nord": {
		Accent: "#88C0D0", AccentDark: "#88C0D0",
		Accent2: "#B48EAD", Accent2Dark: "#B48EAD",
		Muted: "#4C566A", MutedDark: "#7B8394",
		Faint: "#4C566A", FaintDark: "#5E6779",
		New: "#D08770", NewDark: "#D08770",
		Danger: "#BF616A", DangerDark: "#BF616A",
		Warn: "#EBCB8B", WarnDark: "#EBCB8B",
		OK: "#A3BE8C", OKDark: "#A3BE8C",
		TCP: "#81A1C1", TCPDark: "#81A1C1",
		UDP: "#B48EAD", UDPDark: "#B48EAD",
		Selected: "#434C5E", SelectedDark: "#3B4252",
		Zebra: "#2E3440", ZebraDark: "#2A2E39",
		Border: "#4C566A", BorderDark: "#3B4252",
		BadgeText: "#2E3440",
	},
	"mono": {
		Accent: "#000000", AccentDark: "#FFFFFF",
		Accent2: "#333333", Accent2Dark: "#CCCCCC",
		Muted: "#666666", MutedDark: "#999999",
		Faint: "#999999", FaintDark: "#666666",
		New: "#000000", NewDark: "#FFFFFF",
		Danger: "#000000", DangerDark: "#FFFFFF",
		Warn: "#444444", WarnDark: "#BBBBBB",
		OK: "#000000", OKDark: "#FFFFFF",
		TCP: "#000000", TCPDark: "#FFFFFF",
		UDP: "#444444", UDPDark: "#BBBBBB",
		Selected: "#DDDDDD", SelectedDark: "#333333",
		Zebra: "#F0F0F0", ZebraDark: "#1A1A1A",
		Border: "#AAAAAA", BorderDark: "#555555",
		BadgeText: "#FFFFFF",
	},
}

var (
	colorAccent   lipgloss.AdaptiveColor
	colorAccent2  lipgloss.AdaptiveColor
	colorMuted    lipgloss.AdaptiveColor
	colorFaint    lipgloss.AdaptiveColor
	colorNew      lipgloss.AdaptiveColor
	colorDanger   lipgloss.AdaptiveColor
	colorWarn     lipgloss.AdaptiveColor
	colorOk       lipgloss.AdaptiveColor
	colorTCP      lipgloss.AdaptiveColor
	colorUDP      lipgloss.AdaptiveColor
	colorSelected lipgloss.AdaptiveColor
	colorZebra    lipgloss.AdaptiveColor
	colorBorder   lipgloss.AdaptiveColor

	styleAppBorder   lipgloss.Style
	styleBadge       lipgloss.Style
	styleTitle       lipgloss.Style
	styleTag         lipgloss.Style
	styleHeader      lipgloss.Style
	styleDivider     lipgloss.Style
	styleRowZebra    lipgloss.Style
	styleRowSelected lipgloss.Style
	styleRowNew      lipgloss.Style
	styleMuted       lipgloss.Style
	styleFaint       lipgloss.Style
	styleDanger      lipgloss.Style
	styleOk          lipgloss.Style
	styleWarn        lipgloss.Style

	styleStateListen      lipgloss.Style
	styleStateEstablished lipgloss.Style
	styleStateTransient   lipgloss.Style
	styleStateOther       lipgloss.Style

	styleProtoTCP lipgloss.Style
	styleProtoUDP lipgloss.Style

	styleModal  lipgloss.Style
	styleKey    lipgloss.Style
	styleCursor lipgloss.Style
)

func init() {
	ApplyPalette(Themes["default"])
}

// ApplyPalette (re)builds every style from a Palette. Called once at
// startup with the configured theme (default is applied at package init
// so the UI works even if nothing ever calls this again, e.g. in tests).
func ApplyPalette(p Palette) {
	ac := func(light, dark string) lipgloss.AdaptiveColor {
		return lipgloss.AdaptiveColor{Light: light, Dark: dark}
	}

	colorAccent = ac(p.Accent, p.AccentDark)
	colorAccent2 = ac(p.Accent2, p.Accent2Dark)
	colorMuted = ac(p.Muted, p.MutedDark)
	colorFaint = ac(p.Faint, p.FaintDark)
	colorNew = ac(p.New, p.NewDark)
	colorDanger = ac(p.Danger, p.DangerDark)
	colorWarn = ac(p.Warn, p.WarnDark)
	colorOk = ac(p.OK, p.OKDark)
	colorTCP = ac(p.TCP, p.TCPDark)
	colorUDP = ac(p.UDP, p.UDPDark)
	colorSelected = ac(p.Selected, p.SelectedDark)
	colorZebra = ac(p.Zebra, p.ZebraDark)
	colorBorder = ac(p.Border, p.BorderDark)

	styleAppBorder = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent2).
		Padding(0, 1)

	styleBadge = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(p.BadgeText)).
		Background(colorAccent).
		Padding(0, 1)

	styleTitle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	styleTag = lipgloss.NewStyle().Foreground(colorAccent2).Bold(true)
	styleHeader = lipgloss.NewStyle().Bold(true).Foreground(colorAccent2)
	styleDivider = lipgloss.NewStyle().Foreground(colorBorder)
	styleRowZebra = lipgloss.NewStyle().Background(colorZebra)
	styleRowSelected = lipgloss.NewStyle().Background(colorSelected).Bold(true)
	styleRowNew = lipgloss.NewStyle().Foreground(colorNew).Bold(true)
	styleMuted = lipgloss.NewStyle().Foreground(colorMuted)
	styleFaint = lipgloss.NewStyle().Foreground(colorFaint)
	styleDanger = lipgloss.NewStyle().Foreground(colorDanger).Bold(true)
	styleOk = lipgloss.NewStyle().Foreground(colorOk)
	styleWarn = lipgloss.NewStyle().Foreground(colorWarn)

	styleStateListen = lipgloss.NewStyle().Foreground(colorOk).Bold(true)
	styleStateEstablished = lipgloss.NewStyle().Foreground(colorAccent)
	styleStateTransient = lipgloss.NewStyle().Foreground(colorWarn)
	styleStateOther = lipgloss.NewStyle().Foreground(colorFaint)

	styleProtoTCP = lipgloss.NewStyle().Foreground(colorTCP).Bold(true)
	styleProtoUDP = lipgloss.NewStyle().Foreground(colorUDP).Bold(true)

	styleModal = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent2).
		Padding(1, 2)

	styleKey = lipgloss.NewStyle().Foreground(colorAccent2).Bold(true)
	styleCursor = lipgloss.NewStyle().Foreground(colorAccent2).Bold(true)
}

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
