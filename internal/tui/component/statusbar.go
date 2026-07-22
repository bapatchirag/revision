package component

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/bapatchirag/revision/internal/tui"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

// StatusBar is a single-line bar showing left-aligned contextual key hints and
// right-aligned context (such as repo and revision). It is not focusable.
type StatusBar struct {
	left  string
	right string
	width int
	theme theme.Theme
}

var (
	_ tui.Component = (*StatusBar)(nil)
	_ tui.Sizeable  = (*StatusBar)(nil)
	_ tui.Themeable = (*StatusBar)(nil)
)

// NewStatusBar builds an empty status bar.
func NewStatusBar(th theme.Theme) *StatusBar {
	return &StatusBar{theme: th}
}

// Init implements tui.Component.
func (s *StatusBar) Init() tea.Cmd { return nil }

// Update implements tui.Component; the status bar is passive.
func (s *StatusBar) Update(tea.Msg) tea.Cmd { return nil }

// SetLeft sets the left-aligned text (typically key hints).
func (s *StatusBar) SetLeft(text string) { s.left = text }

// SetRight sets the right-aligned text (typically repo/revision context).
func (s *StatusBar) SetRight(text string) { s.right = text }

// SetSize implements tui.Sizeable; only the width is used.
func (s *StatusBar) SetSize(width, _ int) { s.width = width }

// SetTheme implements tui.Themeable.
func (s *StatusBar) SetTheme(th theme.Theme) { s.theme = th }

// View renders the bar, truncating to the available width.
func (s *StatusBar) View() string {
	hint := lipgloss.NewStyle().Foreground(s.theme.Muted)
	if s.width <= 0 {
		return hint.Render(strings.TrimSpace(s.left + " " + s.right))
	}
	left := ansi.Truncate(s.left, s.width, "…")
	gap := s.width - ansi.StringWidth(left) - ansi.StringWidth(s.right)
	if gap < 1 {
		return hint.Render(fitLine(left, s.width))
	}
	return hint.Render(left + strings.Repeat(" ", gap) + s.right)
}
