package ui

import "github.com/charmbracelet/lipgloss"

var (
	colorAccent   = lipgloss.AdaptiveColor{Light: "#0060D0", Dark: "#5FB0FF"}
	colorMuted    = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#8A93A6"}
	colorNew      = lipgloss.AdaptiveColor{Light: "#B4530A", Dark: "#FFB454"}
	colorDanger   = lipgloss.AdaptiveColor{Light: "#B00020", Dark: "#FF6B6B"}
	colorSelected = lipgloss.AdaptiveColor{Light: "#E8F0FE", Dark: "#233047"}
	colorBorder   = lipgloss.AdaptiveColor{Light: "#D0D5DD", Dark: "#3A4356"}

	styleTitle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)

	styleHeader = lipgloss.NewStyle().Bold(true).Foreground(colorMuted)

	styleRow = lipgloss.NewStyle()

	styleRowSelected = lipgloss.NewStyle().
				Background(colorSelected).
				Bold(true)

	styleRowNew = lipgloss.NewStyle().Foreground(colorNew).Bold(true)

	styleMuted = lipgloss.NewStyle().Foreground(colorMuted)

	styleDanger = lipgloss.NewStyle().Foreground(colorDanger).Bold(true)

	styleStatusBar = lipgloss.NewStyle().
			Foreground(colorMuted).
			BorderTop(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(colorBorder)

	styleModal = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Padding(1, 2)

	styleKey = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
)
