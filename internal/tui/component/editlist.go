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

// EditList key-hint lines rendered below the rows. The variant shown depends on
// whether a row is open for text entry, so the visible affordance always matches
// what the keys do.
const (
	editListHint     = "a add · e edit · d delete · space on/off · ctrl+s save · esc cancel"
	editListEditHint = "enter keep · esc discard"
)

// EditEntry is one row of an EditList: a line of text and whether it is in
// force. The component never interprets Text, so the caller decides what it
// means.
type EditEntry struct {
	Text    string
	Enabled bool
}

// EditList is a focusable editor for a list of switchable text entries,
// rendered as a titled box. Rows are added, edited, deleted and turned on or off
// in place; it emits msg.SubmitMsg (its ID, with an empty Value) on submit
// (ctrl+s) and msg.DismissMsg on cancel (esc), and the caller reads the edited
// rows back with Entries. While a row is open for text entry every key is
// literal, so letters bound to actions elsewhere (a/e/d) are typed rather than
// triggering them, and esc closes the row rather than the list.
type EditList struct {
	id          string
	title       string
	placeholder string
	entries     []EditEntry
	cursor      int
	offset      int // first visible row
	editing     bool
	adding      bool // the row being edited is new, so leaving it blank drops it
	draft       []rune
	col         int // cursor column within the draft, as a rune index
	width       int
	height      int
	focused     bool
	theme       theme.Theme
	keys        keymap.KeyMap
}

var (
	_ tui.Component = (*EditList)(nil)
	_ tui.Sizeable  = (*EditList)(nil)
	_ tui.Focusable = (*EditList)(nil)
	_ tui.Themeable = (*EditList)(nil)
)

// NewEditList builds an empty editor identified by id (used on emitted
// messages). placeholder is shown, muted, while there are no entries.
func NewEditList(id, title, placeholder string, th theme.Theme, keys keymap.KeyMap) *EditList {
	return &EditList{
		id:          id,
		title:       title,
		placeholder: placeholder,
		theme:       th,
		keys:        keys,
	}
}

// Init implements tui.Component.
func (el *EditList) Init() tea.Cmd { return nil }

// SetEntries replaces the rows, parking the cursor on the first one and closing
// any row left open. The entries are copied, so later edits do not reach the
// caller's slice until it reads Entries back.
func (el *EditList) SetEntries(entries []EditEntry) {
	el.entries = append([]EditEntry(nil), entries...)
	el.cursor, el.offset = 0, 0
	el.editing, el.adding = false, false
	el.draft, el.col = nil, 0
}

// Entries returns a copy of the current rows, including any edits made so far.
func (el *EditList) Entries() []EditEntry {
	return append([]EditEntry(nil), el.entries...)
}

// Index returns the cursor position.
func (el *EditList) Index() int { return el.cursor }

// Editing reports whether a row is open for text entry.
func (el *EditList) Editing() bool { return el.editing }

// SetSize implements tui.Sizeable.
func (el *EditList) SetSize(width, height int) {
	el.width, el.height = width, height
	el.clampOffset()
}

// Focus implements tui.Focusable.
func (el *EditList) Focus() { el.focused = true }

// Blur implements tui.Focusable.
func (el *EditList) Blur() { el.focused = false }

// Focused implements tui.Focusable.
func (el *EditList) Focused() bool { return el.focused }

// SetTheme implements tui.Themeable.
func (el *EditList) SetTheme(th theme.Theme) { el.theme = th }

// Update edits the rows while focused: keys go to the row open for text entry
// when there is one, and drive the list itself otherwise.
func (el *EditList) Update(m tea.Msg) tea.Cmd {
	if !el.focused {
		return nil
	}
	km, ok := m.(tea.KeyMsg)
	if !ok {
		return nil
	}
	if el.editing {
		el.edit(km)
		el.clampOffset()
		return nil
	}
	return el.browse(km)
}

// browse applies a key to the list: moving the cursor, toggling, adding,
// editing or deleting a row, or leaving with submit (ctrl+s) or cancel (esc).
func (el *EditList) browse(km tea.KeyMsg) tea.Cmd {
	if key.Matches(km, el.keys.Submit) {
		id := el.id
		return func() tea.Msg { return msg.SubmitMsg{ID: id} }
	}
	switch {
	case key.Matches(km, el.keys.Up):
		el.move(-1)
		return nil
	case key.Matches(km, el.keys.Down):
		el.move(1)
		return nil
	}
	switch km.Type {
	case tea.KeyEsc:
		id := el.id
		return func() tea.Msg { return msg.DismissMsg{ID: id} }
	case tea.KeySpace:
		el.toggle()
	case tea.KeyEnter:
		el.editEntry()
	case tea.KeyRunes:
		switch string(km.Runes) {
		case "a":
			el.addEntry()
		case "e":
			el.editEntry()
		case "d":
			el.deleteEntry()
		}
	}
	return nil
}

// edit applies a key to the row open for text entry: ←/→/home/end move the
// cursor, alt+←/alt+→ move it by a word, backspace and alt+backspace delete a
// rune or a word, enter keeps the row, esc discards the edit, and runes are
// inserted.
func (el *EditList) edit(km tea.KeyMsg) {
	switch {
	case key.Matches(km, el.keys.WordLeft):
		el.col = wordStart(el.draft, el.col)
		return
	case key.Matches(km, el.keys.WordRight):
		el.col = wordEnd(el.draft, el.col)
		return
	case key.Matches(km, el.keys.DeleteWordLeft):
		el.draft, el.col = cutWordLeft(el.draft, el.col)
		return
	case key.Matches(km, el.keys.DeleteWordRight):
		el.draft = cutWordRight(el.draft, el.col)
		return
	}
	switch km.Type {
	case tea.KeyEnter:
		el.commitEdit()
	case tea.KeyEsc:
		el.cancelEdit()
	case tea.KeyLeft:
		if el.col > 0 {
			el.col--
		}
	case tea.KeyRight:
		if el.col < len(el.draft) {
			el.col++
		}
	case tea.KeyHome:
		el.col = 0
	case tea.KeyEnd:
		el.col = len(el.draft)
	case tea.KeyBackspace:
		el.backspace()
	case tea.KeySpace:
		el.insert([]rune{' '})
	case tea.KeyRunes:
		el.insert(km.Runes)
	}
}

// addEntry appends an enabled row and opens it for text entry.
func (el *EditList) addEntry() {
	el.entries = append(el.entries, EditEntry{Enabled: true})
	el.cursor = len(el.entries) - 1
	el.adding = true
	el.startEdit()
}

// editEntry opens the row under the cursor for text entry.
func (el *EditList) editEntry() {
	if len(el.entries) == 0 {
		return
	}
	el.startEdit()
}

func (el *EditList) startEdit() {
	el.editing = true
	el.draft = []rune(el.entries[el.cursor].Text)
	el.col = len(el.draft)
	el.clampOffset()
}

// commitEdit keeps the edited text, dropping a row that was added and left
// blank. Blanking an existing row leaves its text as it was.
func (el *EditList) commitEdit() {
	switch text := strings.TrimSpace(string(el.draft)); {
	case text != "":
		el.entries[el.cursor].Text = text
	case el.adding:
		el.removeAt(el.cursor)
	}
	el.stopEdit()
}

// cancelEdit abandons the edit, dropping the row when it was only just added.
func (el *EditList) cancelEdit() {
	if el.adding {
		el.removeAt(el.cursor)
	}
	el.stopEdit()
}

func (el *EditList) stopEdit() {
	el.editing, el.adding = false, false
	el.draft, el.col = nil, 0
	el.clampCursor()
	el.clampOffset()
}

// deleteEntry removes the row under the cursor.
func (el *EditList) deleteEntry() {
	if len(el.entries) == 0 {
		return
	}
	el.removeAt(el.cursor)
	el.clampCursor()
	el.clampOffset()
}

func (el *EditList) removeAt(i int) {
	if i < 0 || i >= len(el.entries) {
		return
	}
	el.entries = append(el.entries[:i], el.entries[i+1:]...)
}

// toggle flips whether the row under the cursor is in force.
func (el *EditList) toggle() {
	if len(el.entries) == 0 {
		return
	}
	el.entries[el.cursor].Enabled = !el.entries[el.cursor].Enabled
}

// move steps the cursor by delta, clamped into range.
func (el *EditList) move(delta int) {
	el.cursor += delta
	el.clampCursor()
	el.clampOffset()
}

// insert adds rs at the cursor within the draft and advances the column past
// them.
func (el *EditList) insert(rs []rune) {
	if len(rs) == 0 {
		return
	}
	if el.col > len(el.draft) {
		el.col = len(el.draft)
	}
	next := make([]rune, 0, len(el.draft)+len(rs))
	next = append(next, el.draft[:el.col]...)
	next = append(next, rs...)
	next = append(next, el.draft[el.col:]...)
	el.draft = next
	el.col += len(rs)
}

// backspace deletes the rune before the cursor within the draft.
func (el *EditList) backspace() {
	if el.col == 0 {
		return
	}
	if el.col > len(el.draft) {
		el.col = len(el.draft)
	}
	el.draft = append(el.draft[:el.col-1], el.draft[el.col:]...)
	el.col--
}

func (el *EditList) clampCursor() {
	if el.cursor > len(el.entries)-1 {
		el.cursor = len(el.entries) - 1
	}
	if el.cursor < 0 {
		el.cursor = 0
	}
}

func (el *EditList) clampOffset() {
	listH := el.listHeight()
	if el.cursor < el.offset {
		el.offset = el.cursor
	}
	if el.cursor >= el.offset+listH {
		el.offset = el.cursor - listH + 1
	}
	maxOff := len(el.entries) - listH
	if maxOff < 0 {
		maxOff = 0
	}
	if el.offset > maxOff {
		el.offset = maxOff
	}
	if el.offset < 0 {
		el.offset = 0
	}
}

// listHeight is the number of rows on screen: the box height minus its border,
// the spacer and the hint, or the entry count when the height is unset.
func (el *EditList) listHeight() int {
	if el.height > 0 {
		if h := el.height - 4; h >= 1 {
			return h
		}
		return 1
	}
	if len(el.entries) < 1 {
		return 1
	}
	return len(el.entries)
}

// View renders the editor as a titled box: one row per entry (its on/off box
// followed by its text, windowed to the available height) above a key hint whose
// wording matches what the keys currently do. The row under the cursor is
// highlighted, and the row open for text entry shows a reverse-video cursor.
func (el *EditList) View() string {
	innerW := el.width - 2
	if el.width <= 0 {
		innerW = el.intrinsicWidth()
	}
	if innerW < 1 {
		innerW = 1
	}

	sel := lipgloss.NewStyle().Foreground(el.theme.Selection).Bold(true)
	bar := lipgloss.NewStyle().Background(el.theme.SelectionBg)
	muted := lipgloss.NewStyle().Foreground(el.theme.Muted)

	listH := el.listHeight()
	rows := make([]string, 0, listH+2)
	if len(el.entries) == 0 {
		rows = append(rows, fitLine(muted.Render(el.placeholder), innerW))
	}
	for i := 0; i < listH && el.offset+i < len(el.entries); i++ {
		idx := el.offset + i
		active := idx == el.cursor
		prefix := "  "
		if active && el.focused {
			prefix = sel.Render("> ")
		}
		row := prefix + el.entryBody(idx, innerW-2)
		if active {
			row = highlightLine(row, innerW, bar)
		}
		rows = append(rows, row)
	}
	for len(rows) < listH {
		rows = append(rows, fitLine("", innerW))
	}
	rows = append(rows, fitLine("", innerW), fitLine(muted.Render(el.hint()), innerW))

	return box(strings.Join(rows, "\n"), el.title, "", innerW, len(rows), el.theme, el.focused)
}

// entryBody renders one row — its on/off box followed by its text — fit to
// exactly width cells. A row that is not in force is muted.
func (el *EditList) entryBody(i, width int) string {
	e := el.entries[i]
	mark := "[ ] "
	if e.Enabled {
		mark = "[x] "
	}
	textW := width - ansi.StringWidth(mark)
	if textW < 1 {
		textW = 1
	}
	text := e.Text
	switch {
	case el.editing && i == el.cursor:
		text = el.draftText()
	case !e.Enabled:
		text = lipgloss.NewStyle().Foreground(el.theme.Muted).Render(text)
	}
	return fitLine(mark+fitLine(text, textW), width)
}

// draftText renders the row being edited, drawing a reverse-video cursor at the
// edit column while the list is focused.
func (el *EditList) draftText() string {
	if !el.focused {
		return string(el.draft)
	}
	col := el.col
	if col > len(el.draft) {
		col = len(el.draft)
	}
	left := string(el.draft[:col])
	cur, right := " ", ""
	if col < len(el.draft) {
		cur = string(el.draft[col])
		right = string(el.draft[col+1:])
	}
	cursor := lipgloss.NewStyle().Reverse(true).Render(cur)
	return left + cursor + right
}

// hint returns the key-hint line matching what the keys currently do.
func (el *EditList) hint() string {
	if el.editing {
		return editListEditHint
	}
	return editListHint
}

// intrinsicWidth sizes the box to its content when no width has been set.
func (el *EditList) intrinsicWidth() int {
	w := ansi.StringWidth(el.title) + 2
	if n := ansi.StringWidth(el.placeholder) + 2; len(el.entries) == 0 && n > w {
		w = n
	}
	for _, e := range el.entries {
		if n := ansi.StringWidth(e.Text) + 8; n > w {
			w = n
		}
	}
	if n := ansi.StringWidth(editListHint) + 2; n > w {
		w = n
	}
	if w < 32 {
		w = 32
	}
	return w
}
