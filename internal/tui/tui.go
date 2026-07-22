// Package tui defines the domain-agnostic contracts shared by every reusable
// terminal-UI component. This package and internal/tui/component form the
// foundation layer: they never import the SVN domain (internal/svn) or the app
// composition layer (internal/app). A reusability-guard test enforces this.
package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/tui/theme"
)

// Component is the minimal contract every renderable TUI element satisfies.
//
// Update mutates the component in place and returns only a command, so callers
// never need a type assertion to recover a concrete type — this is what lets
// generic components compose without leaking their element type.
type Component interface {
	Init() tea.Cmd
	Update(tea.Msg) tea.Cmd
	View() string
}

// Sizeable is implemented by components that occupy a fixed rectangle and need
// to be told their width and height.
type Sizeable interface {
	SetSize(width, height int)
}

// Focusable is implemented by components that can receive input focus. A
// component acts on key input only while focused.
type Focusable interface {
	Focus()
	Blur()
	Focused() bool
}

// Themeable is implemented by components whose palette can be swapped at
// runtime.
type Themeable interface {
	SetTheme(theme.Theme)
}
