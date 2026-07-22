// Package keymap defines the shared key bindings injected into components. Each
// component reads only the bindings relevant to it; the app owns global keys
// such as focus switching and refresh.
package keymap

import "github.com/charmbracelet/bubbles/key"

// KeyMap is the full set of bindings the UI understands.
type KeyMap struct {
	Up       key.Binding
	Down     key.Binding
	Left     key.Binding
	Right    key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Top      key.Binding
	Bottom   key.Binding

	Enter   key.Binding
	Confirm key.Binding
	Cancel  key.Binding
	Back    key.Binding

	FocusNext key.Binding
	FocusPrev key.Binding

	Refresh key.Binding
	Help    key.Binding
	Quit    key.Binding
}

// Default returns the standard, lazygit-flavored bindings.
func Default() KeyMap {
	return KeyMap{
		Up:        key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:      key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Left:      key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "left")),
		Right:     key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "right")),
		PageUp:    key.NewBinding(key.WithKeys("pgup", "K"), key.WithHelp("PgUp", "page up")),
		PageDown:  key.NewBinding(key.WithKeys("pgdown", "J"), key.WithHelp("PgDn", "page down")),
		Top:       key.NewBinding(key.WithKeys("home", "g"), key.WithHelp("g", "top")),
		Bottom:    key.NewBinding(key.WithKeys("end", "G"), key.WithHelp("G", "bottom")),
		Enter:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
		Confirm:   key.NewBinding(key.WithKeys("enter", "y"), key.WithHelp("enter", "confirm")),
		Cancel:    key.NewBinding(key.WithKeys("esc", "n"), key.WithHelp("esc", "cancel")),
		Back:      key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		FocusNext: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next panel")),
		FocusPrev: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev panel")),
		Refresh:   key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "refresh")),
		Help:      key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:      key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}
