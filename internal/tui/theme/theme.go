// Package theme holds the color palette injected into every component. It is
// deliberately domain-agnostic: roles are named by intent (Accent, Muted,
// Error…), never by SVN concept, so components stay reusable.
package theme

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Theme is a palette of semantic colors shared across the UI.
type Theme struct {
	Text          lipgloss.Color // primary foreground
	Muted         lipgloss.Color // secondary / subtle text
	Accent        lipgloss.Color // titles, highlights
	Selection     lipgloss.Color // selected row foreground
	SelectionBg   lipgloss.Color // selected row highlight-bar background
	Border        lipgloss.Color // unfocused panel border
	BorderFocused lipgloss.Color // focused panel border
	Success       lipgloss.Color
	Warning       lipgloss.Color
	Error         lipgloss.Color
	Info          lipgloss.Color
}

// Auto is the terminal-adaptive palette built from ANSI-256 indices. It is the
// default: the colors resolve against whatever the terminal's own scheme
// defines, so it blends into any terminal theme (at the cost of looking
// different across terminals). The named themes below use true-color hex so they
// render identically everywhere.
func Auto() Theme {
	return Theme{
		Text:          lipgloss.Color("252"),
		Muted:         lipgloss.Color("241"),
		Accent:        lipgloss.Color("39"),
		Selection:     lipgloss.Color("212"),
		SelectionBg:   lipgloss.Color("238"),
		Border:        lipgloss.Color("240"),
		BorderFocused: lipgloss.Color("39"),
		Success:       lipgloss.Color("42"),
		Warning:       lipgloss.Color("214"),
		Error:         lipgloss.Color("196"),
		Info:          lipgloss.Color("39"),
	}
}

// Default returns the palette used when no theme is selected. It is Auto, so
// existing callers and golden tests keep the original ANSI-256 look.
func Default() Theme { return Auto() }

// Everforest is modeled on the Everforest Dark palette: a soft, low-contrast
// forest-green scheme.
func Everforest() Theme {
	return Theme{
		Text:          lipgloss.Color("#d3c6aa"),
		Muted:         lipgloss.Color("#859289"),
		Accent:        lipgloss.Color("#a7c080"),
		Selection:     lipgloss.Color("#a7c080"),
		SelectionBg:   lipgloss.Color("#475258"),
		Border:        lipgloss.Color("#4f585e"),
		BorderFocused: lipgloss.Color("#a7c080"),
		Success:       lipgloss.Color("#a7c080"),
		Warning:       lipgloss.Color("#dbbc7f"),
		Error:         lipgloss.Color("#e67e80"),
		Info:          lipgloss.Color("#7fbbb3"),
	}
}

// Dracula is modeled on the Dracula palette: a vivid purple accent with a pink
// highlight.
func Dracula() Theme {
	return Theme{
		Text:          lipgloss.Color("#f8f8f2"),
		Muted:         lipgloss.Color("#6272a4"),
		Accent:        lipgloss.Color("#bd93f9"),
		Selection:     lipgloss.Color("#ff79c6"),
		SelectionBg:   lipgloss.Color("#44475a"),
		Border:        lipgloss.Color("#44475a"),
		BorderFocused: lipgloss.Color("#bd93f9"),
		Success:       lipgloss.Color("#50fa7b"),
		Warning:       lipgloss.Color("#ffb86c"),
		Error:         lipgloss.Color("#ff5555"),
		Info:          lipgloss.Color("#8be9fd"),
	}
}

// Nord is modeled on the Nord palette: a cool, arctic-frost blue scheme.
func Nord() Theme {
	return Theme{
		Text:          lipgloss.Color("#d8dee9"),
		Muted:         lipgloss.Color("#616e88"),
		Accent:        lipgloss.Color("#88c0d0"),
		Selection:     lipgloss.Color("#88c0d0"),
		SelectionBg:   lipgloss.Color("#3b4252"),
		Border:        lipgloss.Color("#434c5e"),
		BorderFocused: lipgloss.Color("#88c0d0"),
		Success:       lipgloss.Color("#a3be8c"),
		Warning:       lipgloss.Color("#ebcb8b"),
		Error:         lipgloss.Color("#bf616a"),
		Info:          lipgloss.Color("#81a1c1"),
	}
}

// Gruvbox is modeled on the Gruvbox palette: a warm, retro amber/gold scheme.
func Gruvbox() Theme {
	return Theme{
		Text:          lipgloss.Color("#ebdbb2"),
		Muted:         lipgloss.Color("#a89984"),
		Accent:        lipgloss.Color("#fabd2f"),
		Selection:     lipgloss.Color("#fabd2f"),
		SelectionBg:   lipgloss.Color("#3c3836"),
		Border:        lipgloss.Color("#665c54"),
		BorderFocused: lipgloss.Color("#fabd2f"),
		Success:       lipgloss.Color("#b8bb26"),
		Warning:       lipgloss.Color("#fe8019"),
		Error:         lipgloss.Color("#fb4934"),
		Info:          lipgloss.Color("#83a598"),
	}
}

// Cipher is modeled on the "Cipher Dark" VS Code theme (with the user's terminal
// customizations): a navy base with a signature lime-green accent, near-white
// text, and a purple-tinted selection.
func Cipher() Theme {
	return Theme{
		Text:          lipgloss.Color("#e6e9f2"),
		Muted:         lipgloss.Color("#7a7899"),
		Accent:        lipgloss.Color("#9fef00"),
		Selection:     lipgloss.Color("#c88dea"),
		SelectionBg:   lipgloss.Color("#302540"),
		Border:        lipgloss.Color("#313f55"),
		BorderFocused: lipgloss.Color("#9fef00"),
		Success:       lipgloss.Color("#9fef00"),
		Warning:       lipgloss.Color("#ffaf00"),
		Error:         lipgloss.Color("#ff3e3e"),
		Info:          lipgloss.Color("#4dd2e1"),
	}
}

// Named pairs a stable identifier and a human-facing label with a palette.
type Named struct {
	Name  string // stable key stored in config and matched by ByName
	Label string // human-facing label shown in the settings editor
	Theme Theme
}

// registry is the built-in theme set in display order. auto leads as the default
// and the head of the Theme setting.
var registry = []Named{
	{Name: "auto", Label: "Auto", Theme: Auto()},
	{Name: "everforest", Label: "Everforest", Theme: Everforest()},
	{Name: "dracula", Label: "Dracula", Theme: Dracula()},
	{Name: "nord", Label: "Nord", Theme: Nord()},
	{Name: "gruvbox", Label: "Gruvbox", Theme: Gruvbox()},
	{Name: "cipher", Label: "Cipher", Theme: Cipher()},
}

// All returns the built-in themes in display order.
func All() []Named { return append([]Named(nil), registry...) }

// Names returns the theme identifiers in display order.
func Names() []string {
	out := make([]string, len(registry))
	for i, n := range registry {
		out[i] = n.Name
	}
	return out
}

// aliases maps accepted alternate identifiers onto canonical theme names so a
// hand-edited config keeps working: "default" resolves to auto, and the earlier
// color-based names resolve to the popular theme each was built from.
var aliases = map[string]string{
	"default": "auto",
	"green":   "everforest",
	"purple":  "dracula",
	"blue":    "nord",
	"gold":    "gruvbox",
}

// ByName resolves a theme identifier to its palette. The lookup is
// case-insensitive and trims surrounding space; a few aliases (see aliases) are
// accepted so older or more intuitive config values keep working. The boolean
// reports whether name matched a built-in theme or alias; on no match it returns
// Auto so callers always get a usable palette.
func ByName(name string) (Theme, bool) {
	key := strings.ToLower(strings.TrimSpace(name))
	if canon, ok := aliases[key]; ok {
		key = canon
	}
	for _, n := range registry {
		if n.Name == key {
			return n.Theme, true
		}
	}
	return Auto(), false
}
