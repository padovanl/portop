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
	"solarized": {
		Accent: "#268BD2", AccentDark: "#268BD2",
		Accent2: "#6C71C4", Accent2Dark: "#6C71C4",
		Muted: "#657B83", MutedDark: "#657B83",
		Faint: "#586E75", FaintDark: "#586E75",
		New: "#CB4B16", NewDark: "#CB4B16",
		Danger: "#DC322F", DangerDark: "#DC322F",
		Warn: "#B58900", WarnDark: "#B58900",
		OK: "#859900", OKDark: "#859900",
		TCP: "#268BD2", TCPDark: "#268BD2",
		UDP: "#6C71C4", UDPDark: "#6C71C4",
		Selected: "#073642", SelectedDark: "#073642",
		Zebra: "#04303C", ZebraDark: "#04303C",
		Border: "#586E75", BorderDark: "#586E75",
		BadgeText: "#002B36",
	},
	"gruvbox": {
		Accent: "#83A598", AccentDark: "#83A598",
		Accent2: "#D3869B", Accent2Dark: "#D3869B",
		Muted: "#928374", MutedDark: "#928374",
		Faint: "#7C6F64", FaintDark: "#7C6F64",
		New: "#FE8019", NewDark: "#FE8019",
		Danger: "#FB4934", DangerDark: "#FB4934",
		Warn: "#FABD2F", WarnDark: "#FABD2F",
		OK: "#B8BB26", OKDark: "#B8BB26",
		TCP: "#83A598", TCPDark: "#83A598",
		UDP: "#D3869B", UDPDark: "#D3869B",
		Selected: "#504945", SelectedDark: "#504945",
		Zebra: "#32302F", ZebraDark: "#32302F",
		Border: "#7C6F64", BorderDark: "#7C6F64",
		BadgeText: "#282828",
	},
	"catppuccin": {
		Accent: "#89B4FA", AccentDark: "#89B4FA",
		Accent2: "#CBA6F7", Accent2Dark: "#CBA6F7",
		Muted: "#A6ADC8", MutedDark: "#A6ADC8",
		Faint: "#6C7086", FaintDark: "#6C7086",
		New: "#FAB387", NewDark: "#FAB387",
		Danger: "#F38BA8", DangerDark: "#F38BA8",
		Warn: "#F9E2AF", WarnDark: "#F9E2AF",
		OK: "#A6E3A1", OKDark: "#A6E3A1",
		TCP: "#89DCEB", TCPDark: "#89DCEB",
		UDP: "#CBA6F7", UDPDark: "#CBA6F7",
		Selected: "#45475A", SelectedDark: "#45475A",
		Zebra: "#313244", ZebraDark: "#313244",
		Border: "#6C7086", BorderDark: "#6C7086",
		BadgeText: "#1E1E2E",
	},
	"tokyo-night": {
		Accent: "#7AA2F7", AccentDark: "#7AA2F7",
		Accent2: "#BB9AF7", Accent2Dark: "#BB9AF7",
		Muted: "#565F89", MutedDark: "#565F89",
		Faint: "#414868", FaintDark: "#414868",
		New: "#FF9E64", NewDark: "#FF9E64",
		Danger: "#F7768E", DangerDark: "#F7768E",
		Warn: "#E0AF68", WarnDark: "#E0AF68",
		OK: "#9ECE6A", OKDark: "#9ECE6A",
		TCP: "#7DCFFF", TCPDark: "#7DCFFF",
		UDP: "#BB9AF7", UDPDark: "#BB9AF7",
		Selected: "#292E42", SelectedDark: "#292E42",
		Zebra: "#1F2335", ZebraDark: "#1F2335",
		Border: "#565F89", BorderDark: "#565F89",
		BadgeText: "#1A1B26",
	},
	"monokai": {
		Accent: "#66D9EF", AccentDark: "#66D9EF",
		Accent2: "#AE81FF", Accent2Dark: "#AE81FF",
		Muted: "#75715E", MutedDark: "#75715E",
		Faint: "#49483E", FaintDark: "#49483E",
		New: "#FD971F", NewDark: "#FD971F",
		Danger: "#F92672", DangerDark: "#F92672",
		Warn: "#E6DB74", WarnDark: "#E6DB74",
		OK: "#A6E22E", OKDark: "#A6E22E",
		TCP: "#66D9EF", TCPDark: "#66D9EF",
		UDP: "#AE81FF", UDPDark: "#AE81FF",
		Selected: "#49483E", SelectedDark: "#49483E",
		Zebra: "#2E2F28", ZebraDark: "#2E2F28",
		Border: "#75715E", BorderDark: "#75715E",
		BadgeText: "#272822",
	},
	"darcula": {
		Accent: "#6897BB", AccentDark: "#6897BB",
		Accent2: "#9876AA", Accent2Dark: "#9876AA",
		Muted: "#808080", MutedDark: "#808080",
		Faint: "#5A5A5A", FaintDark: "#5A5A5A",
		New: "#CC7832", NewDark: "#CC7832",
		Danger: "#BC3F3C", DangerDark: "#BC3F3C",
		Warn: "#FFC66D", WarnDark: "#FFC66D",
		OK: "#6A8759", OKDark: "#6A8759",
		TCP: "#6897BB", TCPDark: "#6897BB",
		UDP: "#9876AA", UDPDark: "#9876AA",
		Selected: "#3A3D41", SelectedDark: "#3A3D41",
		Zebra: "#313335", ZebraDark: "#313335",
		Border: "#606060", BorderDark: "#606060",
		BadgeText: "#2B2B2B",
	},
	"vscode": {
		Accent: "#569CD6", AccentDark: "#569CD6",
		Accent2: "#C586C0", Accent2Dark: "#C586C0",
		Muted: "#808080", MutedDark: "#808080",
		Faint: "#6A6A6A", FaintDark: "#6A6A6A",
		New: "#CE9178", NewDark: "#CE9178",
		Danger: "#F44747", DangerDark: "#F44747",
		Warn: "#DCDCAA", WarnDark: "#DCDCAA",
		OK: "#4EC9B0", OKDark: "#4EC9B0",
		TCP: "#569CD6", TCPDark: "#569CD6",
		UDP: "#C586C0", UDPDark: "#C586C0",
		Selected: "#264F78", SelectedDark: "#264F78",
		Zebra: "#252526", ZebraDark: "#252526",
		Border: "#454545", BorderDark: "#454545",
		BadgeText: "#1E1E1E",
	},
	"ubuntu": {
		Accent: "#E95420", AccentDark: "#E95420",
		Accent2: "#C061CB", Accent2Dark: "#C061CB",
		Muted: "#AEA79F", MutedDark: "#AEA79F",
		Faint: "#6B6963", FaintDark: "#6B6963",
		New: "#F99B4A", NewDark: "#F99B4A",
		Danger: "#C01C28", DangerDark: "#C01C28",
		Warn: "#C4A000", WarnDark: "#C4A000",
		OK: "#26A269", OKDark: "#26A269",
		TCP: "#E95420", TCPDark: "#E95420",
		UDP: "#C061CB", UDPDark: "#C061CB",
		Selected: "#4A2545", SelectedDark: "#4A2545",
		Zebra: "#33122B", ZebraDark: "#33122B",
		Border: "#77216F", BorderDark: "#77216F",
		BadgeText: "#300A24",
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

// ThemeNames lists the built-in themes in a stable, deliberate order
// (not map iteration order) for the settings screen's left/right cycling.
// Same lineup and names as padovanl/pkgtui's own theme picker (solarized
// through ubuntu), plus this project's own default/mono.
var ThemeNames = []string{
	"default", "dracula", "nord", "solarized", "gruvbox", "catppuccin",
	"tokyo-night", "monokai", "darcula", "vscode", "ubuntu", "mono",
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
		Border(lipgloss.ThickBorder()).
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
		Border(lipgloss.ThickBorder()).
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
