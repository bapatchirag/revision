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

// List is a generic, scrollable, single-column list. Items are turned into
// display strings by the injected render func, which keeps the component
// domain-agnostic: it never needs to know what T is.
type List[T any] struct {
	id      string
	items   []T
	render  func(T) string
	cursor  int
	offset  int
	width   int
	height  int
	focused bool
	theme   theme.Theme
	keys    keymap.KeyMap
}

var (
	_ tui.Component = (*List[int])(nil)
	_ tui.Sizeable  = (*List[int])(nil)
	_ tui.Focusable = (*List[int])(nil)
	_ tui.Themeable = (*List[int])(nil)
)

// NewList builds a list identified by id (used on emitted messages), rendering
// items with render.
func NewList[T any](id string, render func(T) string, th theme.Theme, keys keymap.KeyMap) *List[T] {
	return &List[T]{id: id, render: render, theme: th, keys: keys}
}

// Init implements tui.Component.
func (l *List[T]) Init() tea.Cmd { return nil }

// SetItems replaces the list contents, keeping the cursor in range.
func (l *List[T]) SetItems(items []T) {
	l.items = items
	l.clampCursor()
	l.clampOffset()
}

// Items returns the current items.
func (l *List[T]) Items() []T { return l.items }

// Index returns the cursor position.
func (l *List[T]) Index() int { return l.cursor }

// Selected returns the item under the cursor and whether one exists.
func (l *List[T]) Selected() (T, bool) {
	if l.cursor >= 0 && l.cursor < len(l.items) {
		return l.items[l.cursor], true
	}
	var zero T
	return zero, false
}

// SetSize implements tui.Sizeable.
func (l *List[T]) SetSize(width, height int) {
	l.width, l.height = width, height
	l.clampOffset()
}

// Focus implements tui.Focusable.
func (l *List[T]) Focus() { l.focused = true }

// Blur implements tui.Focusable.
func (l *List[T]) Blur() { l.focused = false }

// Focused implements tui.Focusable.
func (l *List[T]) Focused() bool { return l.focused }

// SetTheme implements tui.Themeable.
func (l *List[T]) SetTheme(th theme.Theme) { l.theme = th }

// Update handles navigation while focused and emits SelectedMsg/ActivatedMsg.
func (l *List[T]) Update(m tea.Msg) tea.Cmd {
	if !l.focused {
		return nil
	}
	km, ok := m.(tea.KeyMsg)
	if !ok {
		return nil
	}

	prev := l.cursor
	switch {
	case key.Matches(km, l.keys.Up):
		l.cursor--
	case key.Matches(km, l.keys.Down):
		l.cursor++
	case key.Matches(km, l.keys.Top):
		l.cursor = 0
	case key.Matches(km, l.keys.Bottom):
		l.cursor = len(l.items) - 1
	case key.Matches(km, l.keys.Enter):
		if _, ok := l.Selected(); ok {
			id, idx := l.id, l.cursor
			return func() tea.Msg { return msg.ActivatedMsg{ID: id, Index: idx} }
		}
		return nil
	default:
		return nil
	}

	l.clampCursor()
	l.clampOffset()
	if l.cursor != prev {
		id, idx := l.id, l.cursor
		return func() tea.Msg { return msg.SelectedMsg{ID: id, Index: idx} }
	}
	return nil
}

// View renders the visible window of rows, marking the cursor while focused.
func (l *List[T]) View() string {
	if len(l.items) == 0 {
		return ""
	}
	end := len(l.items)
	if l.height > 0 && l.offset+l.height < end {
		end = l.offset + l.height
	}

	sel := lipgloss.NewStyle().Foreground(l.theme.Selection).Bold(true)
	lines := make([]string, 0, end-l.offset)
	for i := l.offset; i < end; i++ {
		row := l.render(l.items[i])
		if i == l.cursor && l.focused {
			lines = append(lines, sel.Render("> ")+row)
		} else {
			lines = append(lines, "  "+row)
		}
	}
	return strings.Join(lines, "\n")
}

func (l *List[T]) clampCursor() {
	if l.cursor > len(l.items)-1 {
		l.cursor = len(l.items) - 1
	}
	if l.cursor < 0 {
		l.cursor = 0
	}
}

func (l *List[T]) clampOffset() {
	if l.height <= 0 {
		l.offset = 0
		return
	}
	if l.cursor < l.offset {
		l.offset = l.cursor
	}
	if l.cursor >= l.offset+l.height {
		l.offset = l.cursor - l.height + 1
	}
	maxOff := len(l.items) - l.height
	if maxOff < 0 {
		maxOff = 0
	}
	if l.offset > maxOff {
		l.offset = maxOff
	}
	if l.offset < 0 {
		l.offset = 0
	}
}
