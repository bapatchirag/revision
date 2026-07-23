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

// textAreaHint is the fixed key-hint line rendered below the editable region.
const textAreaHint = "ctrl+s submit · esc cancel"

// TextArea is a focusable, multi-line text editor rendered as a titled box,
// used for entering free-form text such as a commit message. While focused it
// edits an in-memory buffer and emits msg.SubmitMsg with the full text on
// submit (ctrl+s) and msg.DismissMsg on cancel (esc). Editing keys are matched
// by key type, so letters bound to navigation elsewhere (h/j/k/l, y/n) are
// entered here as literal text rather than triggering those actions.
type TextArea struct {
	id          string
	title       string
	placeholder string
	lines       []string // buffer; always holds at least one line
	row         int      // cursor row
	col         int      // cursor column, as a rune index within the row
	offset      int      // first visible row
	width       int
	height      int
	focused     bool
	theme       theme.Theme
	keys        keymap.KeyMap
}

var (
	_ tui.Component = (*TextArea)(nil)
	_ tui.Sizeable  = (*TextArea)(nil)
	_ tui.Focusable = (*TextArea)(nil)
	_ tui.Themeable = (*TextArea)(nil)
)

// NewTextArea builds an empty editor identified by id (used on emitted
// messages). placeholder is shown, muted, while the buffer is empty.
func NewTextArea(id, title, placeholder string, th theme.Theme, keys keymap.KeyMap) *TextArea {
	return &TextArea{
		id:          id,
		title:       title,
		placeholder: placeholder,
		lines:       []string{""},
		theme:       th,
		keys:        keys,
	}
}

// Init implements tui.Component.
func (ta *TextArea) Init() tea.Cmd { return nil }

// Value returns the whole buffer as a single newline-joined string.
func (ta *TextArea) Value() string { return strings.Join(ta.lines, "\n") }

// SetValue replaces the buffer contents and places the cursor at the end.
func (ta *TextArea) SetValue(s string) {
	if s == "" {
		ta.lines = []string{""}
	} else {
		ta.lines = strings.Split(s, "\n")
	}
	ta.row = len(ta.lines) - 1
	ta.col = len([]rune(ta.lines[ta.row]))
	ta.clampOffset()
}

// Reset clears the buffer back to a single empty line.
func (ta *TextArea) Reset() {
	ta.lines = []string{""}
	ta.row, ta.col, ta.offset = 0, 0, 0
}

// SetSize implements tui.Sizeable.
func (ta *TextArea) SetSize(width, height int) {
	ta.width, ta.height = width, height
	ta.clampOffset()
}

// Focus implements tui.Focusable.
func (ta *TextArea) Focus() { ta.focused = true }

// Blur implements tui.Focusable.
func (ta *TextArea) Blur() { ta.focused = false }

// Focused implements tui.Focusable.
func (ta *TextArea) Focused() bool { return ta.focused }

// SetTheme implements tui.Themeable.
func (ta *TextArea) SetTheme(th theme.Theme) { ta.theme = th }

// Update edits the buffer while focused. Submit (ctrl+s) emits SubmitMsg and
// cancel (esc) emits DismissMsg; every other editing key is consumed here.
func (ta *TextArea) Update(m tea.Msg) tea.Cmd {
	if !ta.focused {
		return nil
	}
	km, ok := m.(tea.KeyMsg)
	if !ok {
		return nil
	}
	if key.Matches(km, ta.keys.Submit) {
		id, val := ta.id, ta.Value()
		return func() tea.Msg { return msg.SubmitMsg{ID: id, Value: val} }
	}
	switch km.Type {
	case tea.KeyEsc:
		id := ta.id
		return func() tea.Msg { return msg.DismissMsg{ID: id} }
	case tea.KeyEnter:
		ta.insertNewline()
	case tea.KeyBackspace:
		ta.backspace()
	case tea.KeySpace:
		ta.insertRunes([]rune{' '})
	case tea.KeyLeft:
		ta.moveLeft()
	case tea.KeyRight:
		ta.moveRight()
	case tea.KeyUp:
		ta.moveUp()
	case tea.KeyDown:
		ta.moveDown()
	case tea.KeyRunes:
		ta.insertRunes(km.Runes)
	}
	ta.clampOffset()
	return nil
}

// View renders the editor as a titled box: the editable region (windowed to the
// available height, with the cursor shown while focused) above a key hint.
func (ta *TextArea) View() string {
	innerW := ta.width - 2
	if ta.width <= 0 {
		innerW = ta.intrinsicWidth()
	}
	if innerW < 1 {
		innerW = 1
	}

	textH := ta.textHeight()
	rows := make([]string, 0, textH+1)
	if ta.isEmpty() && ta.placeholder != "" && !ta.focused {
		muted := lipgloss.NewStyle().Foreground(ta.theme.Muted)
		rows = append(rows, fitLine(muted.Render(ta.placeholder), innerW))
		for len(rows) < textH {
			rows = append(rows, fitLine("", innerW))
		}
	} else {
		for i := 0; i < textH; i++ {
			idx := ta.offset + i
			if idx < len(ta.lines) {
				rows = append(rows, ta.renderLine(idx, innerW))
			} else {
				rows = append(rows, fitLine("", innerW))
			}
		}
	}
	hint := lipgloss.NewStyle().Foreground(ta.theme.Muted).Render(textAreaHint)
	rows = append(rows, fitLine(hint, innerW))

	return box(strings.Join(rows, "\n"), ta.title, innerW, len(rows), ta.theme, ta.focused)
}

// renderLine fits row i to innerW, drawing a reverse-video cursor at the current
// column when the editor is focused and i is the cursor row.
func (ta *TextArea) renderLine(i, innerW int) string {
	line := ta.lines[i]
	if !ta.focused || i != ta.row {
		return fitLine(line, innerW)
	}
	runes := []rune(line)
	col := ta.col
	if col > len(runes) {
		col = len(runes)
	}
	left := string(runes[:col])
	cur, right := " ", ""
	if col < len(runes) {
		cur = string(runes[col])
		right = string(runes[col+1:])
	}
	cursor := lipgloss.NewStyle().Reverse(true).Render(cur)
	return fitLine(left+cursor+right, innerW)
}

func (ta *TextArea) currentRunes() []rune { return []rune(ta.lines[ta.row]) }

// insertRunes inserts rs at the cursor and advances the column past them.
func (ta *TextArea) insertRunes(rs []rune) {
	line := ta.currentRunes()
	if ta.col > len(line) {
		ta.col = len(line)
	}
	next := make([]rune, 0, len(line)+len(rs))
	next = append(next, line[:ta.col]...)
	next = append(next, rs...)
	next = append(next, line[ta.col:]...)
	ta.lines[ta.row] = string(next)
	ta.col += len(rs)
}

// insertNewline splits the current line at the cursor into two lines.
func (ta *TextArea) insertNewline() {
	line := ta.currentRunes()
	if ta.col > len(line) {
		ta.col = len(line)
	}
	left, right := string(line[:ta.col]), string(line[ta.col:])
	ta.lines[ta.row] = left

	tail := make([]string, len(ta.lines)-ta.row-1)
	copy(tail, ta.lines[ta.row+1:])
	ta.lines = append(ta.lines[:ta.row+1], append([]string{right}, tail...)...)
	ta.row++
	ta.col = 0
}

// backspace deletes the rune before the cursor, joining lines at column zero.
func (ta *TextArea) backspace() {
	if ta.col > 0 {
		line := ta.currentRunes()
		ta.lines[ta.row] = string(line[:ta.col-1]) + string(line[ta.col:])
		ta.col--
		return
	}
	if ta.row == 0 {
		return
	}
	prev := []rune(ta.lines[ta.row-1])
	ta.col = len(prev)
	ta.lines[ta.row-1] = string(prev) + ta.lines[ta.row]
	ta.lines = append(ta.lines[:ta.row], ta.lines[ta.row+1:]...)
	ta.row--
}

func (ta *TextArea) moveLeft() {
	switch {
	case ta.col > 0:
		ta.col--
	case ta.row > 0:
		ta.row--
		ta.col = len(ta.currentRunes())
	}
}

func (ta *TextArea) moveRight() {
	switch {
	case ta.col < len(ta.currentRunes()):
		ta.col++
	case ta.row < len(ta.lines)-1:
		ta.row++
		ta.col = 0
	}
}

func (ta *TextArea) moveUp() {
	if ta.row == 0 {
		return
	}
	ta.row--
	if n := len(ta.currentRunes()); ta.col > n {
		ta.col = n
	}
}

func (ta *TextArea) moveDown() {
	if ta.row >= len(ta.lines)-1 {
		return
	}
	ta.row++
	if n := len(ta.currentRunes()); ta.col > n {
		ta.col = n
	}
}

func (ta *TextArea) isEmpty() bool {
	return len(ta.lines) == 1 && ta.lines[0] == ""
}

// textHeight is the number of editable rows: the box height minus its border
// and the hint line, or the buffer length when the height is unset.
func (ta *TextArea) textHeight() int {
	if ta.height > 0 {
		if h := ta.height - 3; h >= 1 {
			return h
		}
		return 1
	}
	if len(ta.lines) < 1 {
		return 1
	}
	return len(ta.lines)
}

// intrinsicWidth sizes the box to its content when no width has been set.
func (ta *TextArea) intrinsicWidth() int {
	w := ansi.StringWidth(ta.title) + 2
	for _, l := range ta.lines {
		if n := ansi.StringWidth(l) + 2; n > w {
			w = n
		}
	}
	if n := ansi.StringWidth(textAreaHint) + 2; n > w {
		w = n
	}
	if w < 20 {
		w = 20
	}
	return w
}

func (ta *TextArea) clampOffset() {
	textH := ta.textHeight()
	if ta.row < ta.offset {
		ta.offset = ta.row
	}
	if ta.row >= ta.offset+textH {
		ta.offset = ta.row - textH + 1
	}
	maxOff := len(ta.lines) - textH
	if maxOff < 0 {
		maxOff = 0
	}
	if ta.offset > maxOff {
		ta.offset = maxOff
	}
	if ta.offset < 0 {
		ta.offset = 0
	}
}
