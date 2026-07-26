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

// Form key-hint lines rendered below the fields. The variant shown depends on
// the active field's kind, so the visible affordance always matches what the
// keys do on that row.
const (
	formHint       = "ctrl+s save · ↑↓ move · esc cancel"
	formToggleHint = "space toggle · ↑↓ move · ctrl+s save · esc cancel"
	formChoiceHint = "←→ change · ↑↓ move · ctrl+s save · esc cancel"
)

// FieldKind classifies how a Form field is edited and rendered.
type FieldKind int

const (
	// FieldText is a free-form single-line string edited character by character.
	FieldText FieldKind = iota
	// FieldInt accepts digits only (an unsigned integer typed as text).
	FieldInt
	// FieldBool is an on/off toggle flipped with space or ←/→.
	FieldBool
	// FieldChoice is one of a fixed set of Options cycled with ←/→.
	FieldChoice
)

// Field is one editable row in a Form. Value always holds the current contents
// as a string — a bool as "true"/"false", an int as its decimal text, a choice
// as the selected option — so the Form stays domain-agnostic and callers map the
// strings back onto their own types. Options is consulted only for FieldChoice.
type Field struct {
	Label   string
	Kind    FieldKind
	Value   string
	Options []string
}

// Form is a focusable, multi-field editor rendered as a titled box. It edits an
// in-memory set of labeled fields — text, integer, boolean, and fixed-choice —
// and emits msg.SubmitMsg (its ID, with an empty Value) on submit (ctrl+s) and
// msg.DismissMsg on cancel (esc); the caller reads the edited fields back with
// Values. Navigation and submit/cancel keys are matched by key type, so letters
// bound to navigation elsewhere (h/j/k/l, y/n) are typed as literal text in a
// text field rather than triggering those actions.
type Form struct {
	id      string
	title   string
	fields  []Field
	cursor  int // active field index
	col     int // cursor column within the active text/int field's value
	width   int
	focused bool
	theme   theme.Theme
	keys    keymap.KeyMap
}

var (
	_ tui.Component = (*Form)(nil)
	_ tui.Sizeable  = (*Form)(nil)
	_ tui.Focusable = (*Form)(nil)
	_ tui.Themeable = (*Form)(nil)
)

// NewForm builds a form identified by id (used on emitted messages) with the
// given title and fields.
func NewForm(id, title string, fields []Field, th theme.Theme, keys keymap.KeyMap) *Form {
	f := &Form{id: id, title: title, theme: th, keys: keys}
	f.SetFields(fields)
	return f
}

// Init implements tui.Component.
func (f *Form) Init() tea.Cmd { return nil }

// SetFields replaces the form's fields, parking the cursor on the first field
// with its edit column at the end of that field's value.
func (f *Form) SetFields(fields []Field) {
	f.fields = fields
	f.cursor = 0
	f.col = f.valueLen(0)
}

// Fields returns a copy of the current fields, including any edits made so far.
func (f *Form) Fields() []Field {
	return append([]Field(nil), f.fields...)
}

// Values returns each field's current value in field order.
func (f *Form) Values() []string {
	vals := make([]string, len(f.fields))
	for i := range f.fields {
		vals[i] = f.fields[i].Value
	}
	return vals
}

// Value returns the current value of the field at i, or "" when i is out of
// range.
func (f *Form) Value(i int) string {
	if i < 0 || i >= len(f.fields) {
		return ""
	}
	return f.fields[i].Value
}

// SetSize implements tui.Sizeable; only the width is used (the height follows
// the field count).
func (f *Form) SetSize(width, _ int) { f.width = width }

// Focus implements tui.Focusable.
func (f *Form) Focus() { f.focused = true }

// Blur implements tui.Focusable.
func (f *Form) Blur() { f.focused = false }

// Focused implements tui.Focusable.
func (f *Form) Focused() bool { return f.focused }

// SetTheme implements tui.Themeable.
func (f *Form) SetTheme(th theme.Theme) { f.theme = th }

// Update edits the active field while focused. Submit (ctrl+s) emits SubmitMsg
// and cancel (esc) emits DismissMsg; ↑/↓ and tab/shift+tab move between fields;
// every other editing key is dispatched to the active field by its kind.
func (f *Form) Update(m tea.Msg) tea.Cmd {
	if !f.focused {
		return nil
	}
	km, ok := m.(tea.KeyMsg)
	if !ok {
		return nil
	}
	if key.Matches(km, f.keys.Submit) {
		id := f.id
		return func() tea.Msg { return msg.SubmitMsg{ID: id} }
	}
	switch km.Type {
	case tea.KeyEsc:
		id := f.id
		return func() tea.Msg { return msg.DismissMsg{ID: id} }
	case tea.KeyUp, tea.KeyShiftTab:
		f.moveField(-1)
	case tea.KeyDown, tea.KeyTab:
		f.moveField(1)
	default:
		f.editActive(km)
	}
	return nil
}

// editActive applies an editing key to the active field according to its kind: a
// bool toggles, a choice cycles, and a text/int field edits its value inline.
func (f *Form) editActive(km tea.KeyMsg) {
	if len(f.fields) == 0 {
		return
	}
	fld := &f.fields[f.cursor]
	switch fld.Kind {
	case FieldBool:
		switch km.Type {
		case tea.KeySpace, tea.KeyLeft, tea.KeyRight, tea.KeyEnter:
			f.toggleBool(fld)
		}
	case FieldChoice:
		switch km.Type {
		case tea.KeyLeft:
			f.cycleChoice(fld, -1)
		case tea.KeyRight, tea.KeySpace, tea.KeyEnter:
			f.cycleChoice(fld, 1)
		}
	default: // FieldText, FieldInt
		f.editText(fld, km)
	}
}

// editText edits a text or integer field inline: ←/→/home/end move the cursor,
// backspace deletes, enter advances to the next field, and runes are inserted
// (an integer field ignores non-digits).
func (f *Form) editText(fld *Field, km tea.KeyMsg) {
	switch km.Type {
	case tea.KeyEnter:
		f.moveField(1)
	case tea.KeyLeft:
		if f.col > 0 {
			f.col--
		}
	case tea.KeyRight:
		if f.col < f.valueLen(f.cursor) {
			f.col++
		}
	case tea.KeyHome:
		f.col = 0
	case tea.KeyEnd:
		f.col = f.valueLen(f.cursor)
	case tea.KeyBackspace:
		f.backspace(fld)
	case tea.KeySpace:
		if fld.Kind == FieldText {
			f.insert(fld, []rune{' '})
		}
	case tea.KeyRunes:
		f.insert(fld, f.acceptable(fld, km.Runes))
	}
}

// acceptable filters typed runes for a field: an integer field keeps only ASCII
// digits, while a text field accepts everything.
func (f *Form) acceptable(fld *Field, rs []rune) []rune {
	if fld.Kind != FieldInt {
		return rs
	}
	out := rs[:0:0]
	for _, r := range rs {
		if r >= '0' && r <= '9' {
			out = append(out, r)
		}
	}
	return out
}

// toggleBool flips a boolean field between "true" and "false".
func (f *Form) toggleBool(fld *Field) {
	if fld.Value == "true" {
		fld.Value = "false"
	} else {
		fld.Value = "true"
	}
}

// cycleChoice advances a choice field to the next/previous option, wrapping
// around the ends. It is a no-op when the field has no options.
func (f *Form) cycleChoice(fld *Field, delta int) {
	n := len(fld.Options)
	if n == 0 {
		return
	}
	idx := 0
	for i, o := range fld.Options {
		if o == fld.Value {
			idx = i
			break
		}
	}
	idx = (idx + delta%n + n) % n
	fld.Value = fld.Options[idx]
}

// insert adds rs at the cursor within a text/int field's value and advances the
// column past them.
func (f *Form) insert(fld *Field, rs []rune) {
	if len(rs) == 0 {
		return
	}
	runes := []rune(fld.Value)
	if f.col > len(runes) {
		f.col = len(runes)
	}
	next := make([]rune, 0, len(runes)+len(rs))
	next = append(next, runes[:f.col]...)
	next = append(next, rs...)
	next = append(next, runes[f.col:]...)
	fld.Value = string(next)
	f.col += len(rs)
}

// backspace deletes the rune before the cursor within a text/int field.
func (f *Form) backspace(fld *Field) {
	if f.col == 0 {
		return
	}
	runes := []rune(fld.Value)
	if f.col > len(runes) {
		f.col = len(runes)
	}
	fld.Value = string(runes[:f.col-1]) + string(runes[f.col:])
	f.col--
}

// moveField moves the active-field cursor by delta (clamped into range) and
// parks the edit column at the end of the newly active field's value.
func (f *Form) moveField(delta int) {
	if len(f.fields) == 0 {
		return
	}
	f.cursor += delta
	if f.cursor < 0 {
		f.cursor = 0
	}
	if f.cursor > len(f.fields)-1 {
		f.cursor = len(f.fields) - 1
	}
	f.col = f.valueLen(f.cursor)
}

// valueLen returns the rune length of the field at i's value, or 0 when out of
// range.
func (f *Form) valueLen(i int) int {
	if i < 0 || i >= len(f.fields) {
		return 0
	}
	return len([]rune(f.fields[i].Value))
}

// View renders the form as a titled box: one row per field (label left, value
// right of a shared value column) above a key hint whose wording matches the
// active field's kind. The active row is highlighted, and an active text field
// shows a reverse-video cursor.
func (f *Form) View() string {
	innerW := f.width - 2
	if f.width <= 0 {
		innerW = f.intrinsicWidth()
	}
	if innerW < 1 {
		innerW = 1
	}
	labelW := f.labelWidth(innerW)

	sel := lipgloss.NewStyle().Foreground(f.theme.Selection).Bold(true)
	bar := lipgloss.NewStyle().Background(f.theme.SelectionBg)
	muted := lipgloss.NewStyle().Foreground(f.theme.Muted)

	rows := make([]string, 0, len(f.fields)+2)
	for i := range f.fields {
		active := i == f.cursor
		prefix := "  "
		if active && f.focused {
			prefix = sel.Render("> ")
		}
		row := prefix + f.fieldBody(&f.fields[i], active, labelW, innerW-2)
		if active {
			row = highlightLine(row, innerW, bar)
		}
		rows = append(rows, row)
	}
	rows = append(rows, fitLine("", innerW), fitLine(muted.Render(f.hint()), innerW))
	return box(strings.Join(rows, "\n"), f.title, innerW, len(rows), f.theme, f.focused)
}

// fieldBody lays a field's label in the value column's left gutter and its value
// to the right, fit to exactly width cells.
func (f *Form) fieldBody(fld *Field, active bool, labelW, width int) string {
	valW := width - labelW - 1
	if valW < 1 {
		valW = 1
	}
	label := fitLine(fld.Label, labelW)
	val := fitLine(f.valueText(fld, active, valW), valW)
	return label + " " + val
}

// valueText renders a field's value: an on/off word for a bool, the selected
// option for a choice, or the editable text (with a reverse-video cursor while
// the field is active and focused) for a text/int field.
func (f *Form) valueText(fld *Field, active bool, width int) string {
	switch fld.Kind {
	case FieldBool:
		return boolWord(fld.Value)
	case FieldChoice:
		return fld.Value
	default:
		return f.textValue(fld, active, width)
	}
}

// textValue renders a text/int field's value, drawing a reverse-video cursor at
// the edit column while the field is active and the form is focused.
func (f *Form) textValue(fld *Field, active bool, _ int) string {
	if !(active && f.focused) {
		return fld.Value
	}
	runes := []rune(fld.Value)
	col := f.col
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
	return left + cursor + right
}

// hint returns the key-hint line whose wording matches the active field's kind.
func (f *Form) hint() string {
	if len(f.fields) == 0 {
		return formHint
	}
	switch f.fields[f.cursor].Kind {
	case FieldBool:
		return formToggleHint
	case FieldChoice:
		return formChoiceHint
	default:
		return formHint
	}
}

// labelWidth is the shared width of the label gutter: the widest label, capped
// so the value column keeps at least a third of the inner width.
func (f *Form) labelWidth(innerW int) int {
	w := 0
	for i := range f.fields {
		if n := ansi.StringWidth(f.fields[i].Label); n > w {
			w = n
		}
	}
	if cap := max(innerW-2-innerW/3, 1); w > cap {
		w = cap
	}
	return w
}

// intrinsicWidth sizes the box to its content when no width has been set.
func (f *Form) intrinsicWidth() int {
	w := ansi.StringWidth(f.title) + 2
	for i := range f.fields {
		row := ansi.StringWidth(f.fields[i].Label) + ansi.StringWidth(f.fields[i].Value) + 5
		if row > w {
			w = row
		}
	}
	if n := ansi.StringWidth(formChoiceHint) + 2; n > w {
		w = n
	}
	if w < 32 {
		w = 32
	}
	return w
}

// boolWord maps a boolean field's stored value to its on/off display word.
func boolWord(v string) string {
	if v == "true" {
		return "on"
	}
	return "off"
}
