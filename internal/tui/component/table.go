package component

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/bapatchirag/revision/internal/tui"
	"github.com/bapatchirag/revision/internal/tui/keymap"
	"github.com/bapatchirag/revision/internal/tui/msg"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

// Column describes one column of a Table. A Width greater than zero is a fixed
// column width in cells; a Width of zero (or less) makes the column flexible,
// sharing the leftover width equally with the other flexible columns.
type Column struct {
	Title string
	Width int
}

// Table is a generic, scrollable, multi-column table with a header row. Like
// List it stays domain-agnostic: the injected render func turns an item into
// one cell string per column. Navigation, selection and activation mirror List,
// emitting msg.SelectedMsg / msg.ActivatedMsg while focused.
type Table[T any] struct {
	id           string
	columns      []Column
	items        []T
	render       func(T) []string
	natWidths    []int
	cursor       int
	offset       int
	xOffset      int
	contentWidth int
	width        int
	height       int
	focused      bool
	theme        theme.Theme
	keys         keymap.KeyMap
}

var (
	_ tui.Component = (*Table[int])(nil)
	_ tui.Sizeable  = (*Table[int])(nil)
	_ tui.Focusable = (*Table[int])(nil)
	_ tui.Themeable = (*Table[int])(nil)
)

// NewTable builds a table identified by id (used on emitted messages), with the
// given columns, rendering items into cells with render.
func NewTable[T any](id string, columns []Column, render func(T) []string, th theme.Theme, keys keymap.KeyMap) *Table[T] {
	t := &Table[T]{id: id, columns: columns, render: render, theme: th, keys: keys}
	t.refreshWidths()
	return t
}

// Init implements tui.Component.
func (t *Table[T]) Init() tea.Cmd { return nil }

// SetItems replaces the table rows, keeping the cursor in range.
func (t *Table[T]) SetItems(items []T) {
	t.items = items
	t.refreshWidths()
	t.clampCursor()
	t.clampOffset()
	t.clampXOffset()
}

// Items returns the current rows.
func (t *Table[T]) Items() []T { return t.items }

// Index returns the cursor position.
func (t *Table[T]) Index() int { return t.cursor }

// Selected returns the item under the cursor and whether one exists.
func (t *Table[T]) Selected() (T, bool) {
	if t.cursor >= 0 && t.cursor < len(t.items) {
		return t.items[t.cursor], true
	}
	var zero T
	return zero, false
}

// SetSize implements tui.Sizeable.
func (t *Table[T]) SetSize(width, height int) {
	t.width, t.height = width, height
	t.clampOffset()
	t.clampXOffset()
}

// Focus implements tui.Focusable.
func (t *Table[T]) Focus() { t.focused = true }

// Blur implements tui.Focusable.
func (t *Table[T]) Blur() { t.focused = false }

// Focused implements tui.Focusable.
func (t *Table[T]) Focused() bool { return t.focused }

// SetTheme implements tui.Themeable.
func (t *Table[T]) SetTheme(th theme.Theme) { t.theme = th }

// Update handles navigation while focused and emits SelectedMsg/ActivatedMsg.
func (t *Table[T]) Update(m tea.Msg) tea.Cmd {
	if !t.focused {
		return nil
	}
	km, ok := m.(tea.KeyMsg)
	if !ok {
		return nil
	}

	prev := t.cursor
	switch {
	case key.Matches(km, t.keys.Up):
		t.cursor--
	case key.Matches(km, t.keys.Down):
		t.cursor++
	case key.Matches(km, t.keys.Top):
		t.cursor = 0
	case key.Matches(km, t.keys.Bottom):
		t.cursor = len(t.items) - 1
	case key.Matches(km, t.keys.Left):
		t.xOffset -= hScrollStep
	case key.Matches(km, t.keys.Right):
		t.xOffset += hScrollStep
	case key.Matches(km, t.keys.LineStart):
		t.xOffset = 0
	case key.Matches(km, t.keys.LineEnd):
		t.xOffset = t.contentWidth
	case key.Matches(km, t.keys.Enter):
		if _, ok := t.Selected(); ok {
			id, idx := t.id, t.cursor
			return func() tea.Msg { return msg.ActivatedMsg{ID: id, Index: idx} }
		}
		return nil
	default:
		return nil
	}

	t.clampCursor()
	t.clampOffset()
	t.clampXOffset()
	if t.cursor != prev {
		id, idx := t.id, t.cursor
		return func() tea.Msg { return msg.SelectedMsg{ID: id, Index: idx} }
	}
	return nil
}

func (t *Table[T]) headerCells() []string {
	titles := make([]string, len(t.columns))
	for i, c := range t.columns {
		titles[i] = c.Title
	}
	return titles
}

// rowText fits each cell to its column width and joins the cells with a single
// space, so the header and body rows always line up.
func (t *Table[T]) rowText(cells []string, widths []int) string {
	parts := make([]string, len(widths))
	for i := range widths {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		parts[i] = fitLine(cell, widths[i])
	}
	return strings.Join(parts, " ")
}

// View renders the header row followed by the visible window of body rows,
// marking the cursor while focused. Vertical and horizontal scrollbars occupy the
// right column and bottom row whenever the rows overflow that axis.
func (t *Table[T]) View() string {
	widths := t.natWidths
	innerW, innerH, vBar, hBar := scrollLayout(len(t.items), t.contentWidth, t.width, t.height, 1)

	head := lipgloss.NewStyle().Foreground(t.theme.Accent).Bold(true)
	header := windowLine(head.Render("  "+t.rowText(t.headerCells(), widths)), t.xOffset, innerW)
	if vBar {
		header += vScrollTrack
	}
	out := []string{header}

	start, end := t.offset, len(t.items)
	if bodyH := innerH - 1; bodyH < end-start {
		end = start + bodyH
		if end < start {
			end = start
		}
	}

	sel := lipgloss.NewStyle().Foreground(t.theme.Selection).Bold(true)
	body := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		row := t.rowText(t.render(t.items[i]), widths)
		if i == t.cursor && t.focused {
			row = sel.Render("> ") + row
		} else {
			row = "  " + row
		}
		body = append(body, windowLine(row, t.xOffset, innerW))
	}
	if hBar {
		for len(body) < innerH-1 {
			body = append(body, windowLine("", t.xOffset, innerW))
		}
	}
	if vBar {
		appendVScrollbar(body, len(t.items), t.offset)
	}
	out = append(out, body...)
	if hBar {
		out = append(out, horizontalBarRow(t.contentWidth, t.xOffset, innerW, vBar))
	}
	return strings.Join(out, "\n")
}

// refreshWidths recomputes the natural column widths and the resulting content
// width from the current columns and items; called whenever either changes.
func (t *Table[T]) refreshWidths() {
	t.natWidths = t.naturalColWidths()
	t.contentWidth = 2
	for i, w := range t.natWidths {
		if i > 0 {
			t.contentWidth++
		}
		t.contentWidth += w
	}
}

// naturalColWidths resolves each column to its natural content width: fixed
// columns keep their configured width; flexible (Width <= 0) columns take the
// widest of their title and cell contents. Rows are rendered at these widths and
// scrolled horizontally when they exceed the panel, rather than being squeezed.
func (t *Table[T]) naturalColWidths() []int {
	widths := make([]int, len(t.columns))
	for i, c := range t.columns {
		if c.Width > 0 {
			widths[i] = c.Width
			continue
		}
		w := ansi.StringWidth(c.Title)
		for _, it := range t.items {
			cells := t.render(it)
			if i < len(cells) {
				if cw := ansi.StringWidth(cells[i]); cw > w {
					w = cw
				}
			}
		}
		if w < 1 {
			w = 1
		}
		widths[i] = w
	}
	return widths
}

func (t *Table[T]) clampCursor() {
	if t.cursor > len(t.items)-1 {
		t.cursor = len(t.items) - 1
	}
	if t.cursor < 0 {
		t.cursor = 0
	}
}

func (t *Table[T]) clampOffset() {
	_, innerH, _, _ := scrollLayout(len(t.items), t.contentWidth, t.width, t.height, 1)
	win := innerH - 1 // the header row consumes one line
	if win < 1 {
		t.offset = 0
		return
	}
	if t.cursor < t.offset {
		t.offset = t.cursor
	}
	if t.cursor >= t.offset+win {
		t.offset = t.cursor - win + 1
	}
	maxOff := len(t.items) - win
	if maxOff < 0 {
		maxOff = 0
	}
	if t.offset > maxOff {
		t.offset = maxOff
	}
	if t.offset < 0 {
		t.offset = 0
	}
}

func (t *Table[T]) clampXOffset() {
	innerW, _, _, _ := scrollLayout(len(t.items), t.contentWidth, t.width, t.height, 1)
	maxX := t.contentWidth - innerW
	if maxX < 0 {
		maxX = 0
	}
	if t.xOffset > maxX {
		t.xOffset = maxX
	}
	if t.xOffset < 0 {
		t.xOffset = 0
	}
}
