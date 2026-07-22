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

// MenuItem is a single selectable action with an optional key hint.
type MenuItem struct {
	Label string
	Key   string
}

// Menu is a centered popup listing actions (the "?" menu). While focused it
// emits ActivatedMsg for the chosen item and DismissMsg on cancel.
type Menu struct {
	id      string
	title   string
	items   []MenuItem
	cursor  int
	width   int
	focused bool
	theme   theme.Theme
	keys    keymap.KeyMap
}

var (
	_ tui.Component = (*Menu)(nil)
	_ tui.Sizeable  = (*Menu)(nil)
	_ tui.Focusable = (*Menu)(nil)
	_ tui.Themeable = (*Menu)(nil)
)

// NewMenu builds a menu popup.
func NewMenu(id, title string, items []MenuItem, th theme.Theme, keys keymap.KeyMap) *Menu {
	return &Menu{id: id, title: title, items: items, theme: th, keys: keys}
}

// Init implements tui.Component.
func (mn *Menu) Init() tea.Cmd { return nil }

// Index returns the cursor position.
func (mn *Menu) Index() int { return mn.cursor }

// Update handles navigation, activation and dismissal while focused.
func (mn *Menu) Update(m tea.Msg) tea.Cmd {
	if !mn.focused {
		return nil
	}
	km, ok := m.(tea.KeyMsg)
	if !ok {
		return nil
	}
	id := mn.id
	switch {
	case key.Matches(km, mn.keys.Up):
		mn.cursor--
	case key.Matches(km, mn.keys.Down):
		mn.cursor++
	case key.Matches(km, mn.keys.Enter):
		idx := mn.cursor
		return func() tea.Msg { return msg.ActivatedMsg{ID: id, Index: idx} }
	case key.Matches(km, mn.keys.Cancel):
		return func() tea.Msg { return msg.DismissMsg{ID: id} }
	default:
		return nil
	}
	mn.clampCursor()
	return nil
}

// SetSize implements tui.Sizeable; only the width is used (height follows the
// item count).
func (mn *Menu) SetSize(width, _ int) { mn.width = width }

// Focus implements tui.Focusable.
func (mn *Menu) Focus() { mn.focused = true }

// Blur implements tui.Focusable.
func (mn *Menu) Blur() { mn.focused = false }

// Focused implements tui.Focusable.
func (mn *Menu) Focused() bool { return mn.focused }

// SetTheme implements tui.Themeable.
func (mn *Menu) SetTheme(th theme.Theme) { mn.theme = th }

// View renders the menu as a titled box of rows.
func (mn *Menu) View() string {
	innerW := mn.width - 2
	if mn.width <= 0 {
		innerW = mn.intrinsicWidth()
	}
	sel := lipgloss.NewStyle().Foreground(mn.theme.Selection).Bold(true)
	rows := make([]string, len(mn.items))
	for i, it := range mn.items {
		prefix := "  "
		if i == mn.cursor && mn.focused {
			prefix = sel.Render("> ")
		}
		rows[i] = prefix + mn.itemBody(it, innerW-2)
	}
	return box(strings.Join(rows, "\n"), mn.title, innerW, len(mn.items), mn.theme, mn.focused)
}

// itemBody lays label on the left and key on the right within width cells.
func (mn *Menu) itemBody(it MenuItem, width int) string {
	if it.Key == "" {
		return fitLine(it.Label, width)
	}
	gap := width - len(it.Label) - len(it.Key)
	if gap < 1 {
		gap = 1
	}
	return it.Label + strings.Repeat(" ", gap) + it.Key
}

func (mn *Menu) intrinsicWidth() int {
	w := len(mn.title) + 2
	for _, it := range mn.items {
		if l := len(it.Label) + len(it.Key) + 6; l > w {
			w = l
		}
	}
	return w
}

func (mn *Menu) clampCursor() {
	if mn.cursor < 0 {
		mn.cursor = 0
	}
	if mn.cursor > len(mn.items)-1 {
		mn.cursor = len(mn.items) - 1
	}
}
