package ui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	subtleStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	errorStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
)

// stateColor maps a single-character status code to a display color.
func stateColor(code string) lipgloss.Color {
	switch code {
	case "M":
		return lipgloss.Color("214") // orange
	case "A":
		return lipgloss.Color("42") // green
	case "D":
		return lipgloss.Color("196") // red
	case "C":
		return lipgloss.Color("201") // magenta
	case "?":
		return lipgloss.Color("240") // grey
	default:
		return lipgloss.Color("252")
	}
}
