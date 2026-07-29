package component

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/bapatchirag/revision/internal/tui"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

// hintSep joins adjacent key hints; hintMore marks hints dropped for width.
const (
	hintSep  = " · "
	hintMore = "…"
)

// StatusBar is a single-line bar showing left-aligned contextual key hints and
// right-aligned context (such as a load state). It is not focusable.
type StatusBar struct {
	hints []string
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

// SetHints sets the left-aligned key hints, shown in the given order. An empty
// slice leaves the left side blank.
func (s *StatusBar) SetHints(hints []string) { s.hints = hints }

// SetRight sets the right-aligned text (typically a load state).
func (s *StatusBar) SetRight(text string) { s.right = text }

// SetSize implements tui.Sizeable; only the width is used.
func (s *StatusBar) SetSize(width, _ int) { s.width = width }

// SetTheme implements tui.Themeable.
func (s *StatusBar) SetTheme(th theme.Theme) { s.theme = th }

// View renders the bar, dropping the hints that do not fit beside the
// right-aligned text.
func (s *StatusBar) View() string {
	style := lipgloss.NewStyle().Foreground(s.theme.Muted)
	if s.width <= 0 {
		return style.Render(strings.TrimSpace(strings.Join(s.hints, hintSep) + " " + s.right))
	}
	avail := s.width
	if s.right != "" {
		avail -= ansi.StringWidth(s.right) + 1 // 1 column keeps the two sides apart
	}
	left := s.fitHints(avail)
	gap := s.width - ansi.StringWidth(left) - ansi.StringWidth(s.right)
	if gap < 1 {
		return style.Render(fitLine(left, s.width))
	}
	return style.Render(left + strings.Repeat(" ", gap) + s.right)
}

// fitHints joins the hints into avail columns, keeping whole hints and ending
// with an ellipsis when any had to be dropped.
func (s *StatusBar) fitHints(avail int) string {
	if len(s.hints) == 0 || avail <= 0 {
		return ""
	}
	full := strings.Join(s.hints, hintSep)
	if ansi.StringWidth(full) <= avail {
		return full
	}
	kept := ""
	for i, h := range s.hints {
		if i > 0 {
			h = hintSep + h
		}
		if ansi.StringWidth(kept+h+hintSep+hintMore) > avail {
			break
		}
		kept += h
	}
	if kept == "" {
		// Not even the first hint fits alongside the ellipsis; clip it instead.
		return ansi.Truncate(s.hints[0], avail, hintMore)
	}
	return kept + hintSep + hintMore
}
