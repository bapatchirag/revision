package component

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bapatchirag/revision/internal/tui"
	"github.com/bapatchirag/revision/internal/tui/keymap"
	"github.com/bapatchirag/revision/internal/tui/msg"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

// Modal is a centered confirmation popup. While focused it emits ConfirmMsg on
// confirm and DismissMsg on cancel, both tagged with the modal's ID.
type Modal struct {
	id      string
	title   string
	message string
	width   int
	focused bool
	theme   theme.Theme
	keys    keymap.KeyMap
}

var (
	_ tui.Component = (*Modal)(nil)
	_ tui.Sizeable  = (*Modal)(nil)
	_ tui.Focusable = (*Modal)(nil)
	_ tui.Themeable = (*Modal)(nil)
)

// NewModal builds a confirmation modal.
func NewModal(id, title, message string, th theme.Theme, keys keymap.KeyMap) *Modal {
	return &Modal{id: id, title: title, message: message, theme: th, keys: keys}
}

// SetPrompt replaces the modal's title and message so a single modal can be
// reused for different confirmations (e.g. revert vs. delete).
func (mo *Modal) SetPrompt(title, message string) {
	mo.title, mo.message = title, message
}

// Init implements tui.Component.
func (mo *Modal) Init() tea.Cmd { return nil }

// Update handles confirm/cancel while focused.
func (mo *Modal) Update(m tea.Msg) tea.Cmd {
	if !mo.focused {
		return nil
	}
	km, ok := m.(tea.KeyMsg)
	if !ok {
		return nil
	}
	id := mo.id
	switch {
	case key.Matches(km, mo.keys.Confirm):
		return func() tea.Msg { return msg.ConfirmMsg{ID: id} }
	case key.Matches(km, mo.keys.Cancel):
		return func() tea.Msg { return msg.DismissMsg{ID: id} }
	}
	return nil
}

// SetSize implements tui.Sizeable; only the width is used (height follows the
// message).
func (mo *Modal) SetSize(width, _ int) { mo.width = width }

// Focus implements tui.Focusable.
func (mo *Modal) Focus() { mo.focused = true }

// Blur implements tui.Focusable.
func (mo *Modal) Blur() { mo.focused = false }

// Focused implements tui.Focusable.
func (mo *Modal) Focused() bool { return mo.focused }

// SetTheme implements tui.Themeable.
func (mo *Modal) SetTheme(th theme.Theme) { mo.theme = th }

// View renders the modal as a titled box. When sized, a long message is
// word-wrapped to the box width; unsized (width ≤ 0) the box hugs its content.
func (mo *Modal) View() string {
	hint := "enter confirm · esc cancel"
	if mo.width <= 0 {
		body := []string{mo.message, "", hint}
		innerW := maxWidth(append([]string{" " + mo.title + " "}, body...))
		return box(strings.Join(body, "\n"), mo.title, innerW, len(body), mo.theme, mo.focused)
	}
	innerW := mo.width - 2
	wrapped := lipgloss.NewStyle().Width(innerW).Render(mo.message)
	body := append(strings.Split(wrapped, "\n"), "", hint)
	return box(strings.Join(body, "\n"), mo.title, innerW, len(body), mo.theme, mo.focused)
}
