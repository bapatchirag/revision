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

// MenuItem is a single selectable action with an optional key hint. When
// Header is set the item is a non-selectable section heading and the cursor
// skips over it.
type MenuItem struct {
	Label  string
	Key    string
	Header bool
}

// MenuSection builds a non-selectable section heading row.
func MenuSection(label string) MenuItem { return MenuItem{Label: label, Header: true} }

// Menu is a centered popup listing actions (the "?" menu). While focused it
// emits ActivatedMsg for the chosen item and DismissMsg on cancel.
type Menu struct {
	id      string
	title   string
	items   []MenuItem
	cursor  int
	width   int
	columns int
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
	mn := &Menu{id: id, title: title, items: items, theme: th, keys: keys}
	mn.clampCursor()
	return mn
}

// Init implements tui.Component.
func (mn *Menu) Init() tea.Cmd { return nil }

// SetTitle replaces the menu's heading so a single menu can be reused with a
// context-dependent title (e.g. embedding a version number).
func (mn *Menu) SetTitle(title string) { mn.title = title }

// Index returns the cursor position.
func (mn *Menu) Index() int { return mn.cursor }

// SetIndex moves the cursor to i (clamped into range) so a caller can preselect
// an item, e.g. the currently active choice when opening a menu.
func (mn *Menu) SetIndex(i int) {
	mn.cursor = i
	mn.clampCursor()
}

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
		mn.move(-1)
	case key.Matches(km, mn.keys.Down):
		mn.move(1)
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

// SetColumns lays the items out across n side-by-side columns, keeping the box
// short enough for a long item list to fit on screen.
func (mn *Menu) SetColumns(n int) {
	if n < 1 {
		n = 1
	}
	mn.columns = n
}

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
	cols := mn.columns
	if cols < 1 {
		cols = 1
	}
	const colGap = 2
	colW := (innerW - (cols-1)*colGap) / cols
	bounds := mn.columnBounds(cols)
	perCol := 0
	for c := 0; c < cols; c++ {
		if n := bounds[c+1] - bounds[c]; n > perCol {
			perCol = n
		}
	}
	if perCol < 1 {
		perCol = 1
	}
	cells := make([]string, len(mn.items))
	for i, it := range mn.items {
		cells[i] = mn.renderItem(it, i, colW)
	}
	rows := make([]string, perCol)
	for r := 0; r < perCol; r++ {
		parts := make([]string, 0, cols)
		for c := 0; c < cols; c++ {
			if i := bounds[c] + r; i < bounds[c+1] {
				parts = append(parts, cells[i])
			} else {
				parts = append(parts, strings.Repeat(" ", colW))
			}
		}
		rows[r] = strings.Join(parts, strings.Repeat(" ", colGap))
	}
	return box(strings.Join(rows, "\n"), mn.title, "", innerW, perCol, mn.theme, mn.focused)
}

// columnBounds returns cols+1 item indices delimiting each column, preferring
// to break on a section heading so a section is never split across columns.
func (mn *Menu) columnBounds(cols int) []int {
	n := len(mn.items)
	bounds := make([]int, cols+1)
	bounds[cols] = n
	for c := 1; c < cols; c++ {
		target := c * n / cols
		best := target
		bestDist := n
		for i := bounds[c-1] + 1; i < n; i++ {
			if !mn.items[i].Header {
				continue
			}
			if d := abs(i - target); d < bestDist {
				best, bestDist = i, d
			}
		}
		if best < bounds[c-1] {
			best = bounds[c-1]
		}
		bounds[c] = best
	}
	return bounds
}

func abs(i int) int {
	if i < 0 {
		return -i
	}
	return i
}

// renderItem renders one menu row (heading or action) within width cells.
func (mn *Menu) renderItem(it MenuItem, i, width int) string {
	if it.Header {
		return lipgloss.NewStyle().Foreground(mn.theme.Accent).Bold(true).Render(fitLine(it.Label, width))
	}
	prefix := "  "
	selected := i == mn.cursor
	if selected && mn.focused {
		prefix = lipgloss.NewStyle().Foreground(mn.theme.Selection).Bold(true).Render("> ")
	}
	row := prefix + mn.itemBody(it, width-2)
	if selected {
		row = highlightLine(row, width, lipgloss.NewStyle().Background(mn.theme.SelectionBg))
	}
	return row
}

// itemBody lays label on the left and key on the right within width cells,
// truncating the label when the pair cannot fit.
func (mn *Menu) itemBody(it MenuItem, width int) string {
	if it.Key == "" {
		return fitLine(it.Label, width)
	}
	label := it.Label
	if room := width - len(it.Key) - 1; len(label) > room {
		label = fitLine(label, max(room, 0))
	}
	gap := width - len(label) - len(it.Key)
	if gap < 1 {
		gap = 1
	}
	return label + strings.Repeat(" ", gap) + it.Key
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
	if mn.selectable(mn.cursor) {
		return
	}
	// Landed on a section heading: take the next selectable row in either
	// direction.
	for i := mn.cursor; i < len(mn.items); i++ {
		if mn.selectable(i) {
			mn.cursor = i
			return
		}
	}
	for i := mn.cursor; i >= 0; i-- {
		if mn.selectable(i) {
			mn.cursor = i
			return
		}
	}
}

// move steps the cursor by step, skipping headings and stopping at the ends.
func (mn *Menu) move(step int) {
	for i := mn.cursor + step; i >= 0 && i < len(mn.items); i += step {
		if mn.selectable(i) {
			mn.cursor = i
			return
		}
	}
}

func (mn *Menu) selectable(i int) bool {
	return i >= 0 && i < len(mn.items) && !mn.items[i].Header
}
