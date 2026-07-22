// Package theme holds the color palette injected into every component. It is
// deliberately domain-agnostic: roles are named by intent (Accent, Muted,
// Error…), never by SVN concept, so components stay reusable.
package theme

import "github.com/charmbracelet/lipgloss"

// Theme is a palette of semantic colors shared across the UI.
type Theme struct {
	Text          lipgloss.Color // primary foreground
	Muted         lipgloss.Color // secondary / subtle text
	Accent        lipgloss.Color // titles, highlights
	Selection     lipgloss.Color // selected row foreground
	Border        lipgloss.Color // unfocused panel border
	BorderFocused lipgloss.Color // focused panel border
	Success       lipgloss.Color
	Warning       lipgloss.Color
	Error         lipgloss.Color
	Info          lipgloss.Color
}

// Default returns the standard revision palette.
func Default() Theme {
	return Theme{
		Text:          lipgloss.Color("252"),
		Muted:         lipgloss.Color("241"),
		Accent:        lipgloss.Color("39"),
		Selection:     lipgloss.Color("212"),
		Border:        lipgloss.Color("240"),
		BorderFocused: lipgloss.Color("39"),
		Success:       lipgloss.Color("42"),
		Warning:       lipgloss.Color("214"),
		Error:         lipgloss.Color("196"),
		Info:          lipgloss.Color("39"),
	}
}
