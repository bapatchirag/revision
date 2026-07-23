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

// promptHint / promptPickHint / promptListHint are the fixed key-hint lines
// rendered below the input: the plain variant when there is no pick list, and
// the input-focused vs. list-focused variants when one is shown.
const (
	promptHint     = "enter save · esc cancel"
	promptPickHint = "enter save · tab list · esc cancel"
	promptListHint = "↑↓ pick · tab back · esc cancel"
)

// Prompt is a focusable single-line text input rendered as a titled box, used to
// type a short value such as a name. It can optionally show a list of existing
// options beneath the input, headed by a label. Focus is split into two modes
// toggled with tab: the input field (type a value) and the option list (scroll
// with ↑/↓, which copies the highlighted option into the input). It emits
// msg.SubmitMsg with the current value on enter and msg.DismissMsg on esc.
// Editing keys are matched by key type, so letters bound to navigation elsewhere
// (h/j/k/l, y/n) are typed as literal text.
type Prompt struct {
	id          string
	title       string
	placeholder string
	value       []rune
	col         int
	optionsHead string
	options     []string
	sel         int  // highlighted option, -1 before the list is entered
	listFocused bool // true while the option list holds focus (tab toggles)
	width       int
	focused     bool
	theme       theme.Theme
	keys        keymap.KeyMap
}

var (
	_ tui.Component = (*Prompt)(nil)
	_ tui.Sizeable  = (*Prompt)(nil)
	_ tui.Focusable = (*Prompt)(nil)
	_ tui.Themeable = (*Prompt)(nil)
)

// NewPrompt builds an empty single-line input identified by id (used on emitted
// messages). placeholder is shown, muted, while the input is empty.
func NewPrompt(id, title, placeholder string, th theme.Theme, keys keymap.KeyMap) *Prompt {
	return &Prompt{id: id, title: title, placeholder: placeholder, sel: -1, theme: th, keys: keys}
}

// SetOptions sets the reference list shown beneath the input (labeled by head)
// that the user can pick from with ↑/↓ after tabbing into it. Pass an empty
// slice to hide the list.
func (p *Prompt) SetOptions(head string, options []string) {
	p.optionsHead = head
	p.options = options
	p.sel = -1
	p.listFocused = false
}

// Value returns the current input text.
func (p *Prompt) Value() string { return string(p.value) }

// SetValue replaces the input text, placing the cursor at the end.
func (p *Prompt) SetValue(s string) {
	p.value = []rune(s)
	p.col = len(p.value)
}

// Reset clears the input and any option selection, returning focus to the input
// field.
func (p *Prompt) Reset() {
	p.value = nil
	p.col = 0
	p.sel = -1
	p.listFocused = false
}

// Init implements tui.Component.
func (p *Prompt) Init() tea.Cmd { return nil }

// SetSize implements tui.Sizeable; only the width is used (the height follows the
// input and option rows).
func (p *Prompt) SetSize(width, _ int) { p.width = width }

// Focus implements tui.Focusable.
func (p *Prompt) Focus() { p.focused = true }

// Blur implements tui.Focusable.
func (p *Prompt) Blur() { p.focused = false }

// Focused implements tui.Focusable.
func (p *Prompt) Focused() bool { return p.focused }

// SetTheme implements tui.Themeable.
func (p *Prompt) SetTheme(th theme.Theme) { p.theme = th }

// Update edits the input while focused. Enter emits SubmitMsg, esc emits
// DismissMsg, and tab toggles between the input field and the option list. While
// the list is focused, ↑/↓ scroll it; otherwise editing keys are consumed here.
func (p *Prompt) Update(m tea.Msg) tea.Cmd {
	if !p.focused {
		return nil
	}
	km, ok := m.(tea.KeyMsg)
	if !ok {
		return nil
	}
	switch {
	case key.Matches(km, p.keys.Enter):
		id, val := p.id, p.Value()
		return func() tea.Msg { return msg.SubmitMsg{ID: id, Value: val} }
	case key.Matches(km, p.keys.Back):
		id := p.id
		return func() tea.Msg { return msg.DismissMsg{ID: id} }
	}
	switch km.Type {
	case tea.KeyTab, tea.KeyShiftTab:
		p.toggleList()
	case tea.KeyUp:
		if p.listFocused {
			p.move(-1)
		}
	case tea.KeyDown:
		if p.listFocused {
			p.move(1)
		}
	case tea.KeyLeft:
		if !p.listFocused && p.col > 0 {
			p.col--
		}
	case tea.KeyRight:
		if !p.listFocused && p.col < len(p.value) {
			p.col++
		}
	case tea.KeyBackspace:
		if !p.listFocused {
			p.backspace()
		}
	case tea.KeySpace:
		if !p.listFocused {
			p.insert([]rune{' '})
		}
	case tea.KeyRunes:
		if !p.listFocused {
			p.insert(km.Runes)
		}
	}
	return nil
}

// toggleList moves focus between the input field and the option list (tab). It is
// a no-op when there are no options. Entering the list highlights the first
// option (or the last one visited) and copies it into the input.
func (p *Prompt) toggleList() {
	if len(p.options) == 0 {
		return
	}
	p.listFocused = !p.listFocused
	if p.listFocused {
		if p.sel < 0 {
			p.sel = 0
		}
		p.setValueFromOption()
	}
}

// move scrolls the option selection by delta (while the list is focused) and
// copies the highlighted option into the input.
func (p *Prompt) move(delta int) {
	if len(p.options) == 0 {
		return
	}
	p.sel += delta
	if p.sel < 0 {
		p.sel = 0
	}
	if p.sel > len(p.options)-1 {
		p.sel = len(p.options) - 1
	}
	p.setValueFromOption()
}

// setValueFromOption copies the highlighted option into the input, cursor at end.
func (p *Prompt) setValueFromOption() {
	p.value = []rune(p.options[p.sel])
	p.col = len(p.value)
}

// insert adds rs at the cursor (input field only).
func (p *Prompt) insert(rs []rune) {
	if p.col > len(p.value) {
		p.col = len(p.value)
	}
	next := make([]rune, 0, len(p.value)+len(rs))
	next = append(next, p.value[:p.col]...)
	next = append(next, rs...)
	next = append(next, p.value[p.col:]...)
	p.value = next
	p.col += len(rs)
}

// backspace deletes the rune before the cursor (input field only).
func (p *Prompt) backspace() {
	if p.col == 0 {
		return
	}
	p.value = append(p.value[:p.col-1], p.value[p.col:]...)
	p.col--
}

// View renders the input (with a cursor while focused) inside a titled box, over
// an optional pick list and a key hint.
func (p *Prompt) View() string {
	innerW := p.width - 2
	if p.width <= 0 {
		innerW = p.intrinsicWidth()
	}
	if innerW < 1 {
		innerW = 1
	}

	rows := []string{p.inputRow(innerW)}
	if len(p.options) > 0 {
		head := p.optionsHead
		if head == "" {
			head = "Existing:"
		}
		rows = append(rows, fitLine("", innerW), fitLine(lipgloss.NewStyle().Foreground(p.theme.Muted).Render(head), innerW))
		sel := lipgloss.NewStyle().Foreground(p.theme.Selection).Bold(true)
		for i, opt := range p.options {
			if i == p.sel && p.listFocused {
				rows = append(rows, fitLine(sel.Render("> ")+opt, innerW))
			} else {
				rows = append(rows, fitLine("  "+opt, innerW))
			}
		}
	}
	hint := promptHint
	if len(p.options) > 0 {
		if p.listFocused {
			hint = promptListHint
		} else {
			hint = promptPickHint
		}
	}
	rows = append(rows, fitLine(lipgloss.NewStyle().Foreground(p.theme.Muted).Render(hint), innerW))
	return box(strings.Join(rows, "\n"), p.title, innerW, len(rows), p.theme, p.focused)
}

// inputRow renders the editable line: the value with a reverse-video cursor while
// the input field holds focus, the muted placeholder when empty, or the plain
// value (no cursor) while the option list holds focus.
func (p *Prompt) inputRow(innerW int) string {
	inputActive := p.focused && !p.listFocused
	if len(p.value) == 0 {
		ph := lipgloss.NewStyle().Foreground(p.theme.Muted).Render(p.placeholder)
		if !inputActive {
			return fitLine(ph, innerW)
		}
		return fitLine(lipgloss.NewStyle().Reverse(true).Render(" ")+ph, innerW)
	}
	if !inputActive {
		return fitLine(string(p.value), innerW)
	}
	col := p.col
	if col > len(p.value) {
		col = len(p.value)
	}
	left := string(p.value[:col])
	cur, right := " ", ""
	if col < len(p.value) {
		cur = string(p.value[col])
		right = string(p.value[col+1:])
	}
	cursor := lipgloss.NewStyle().Reverse(true).Render(cur)
	return fitLine(left+cursor+right, innerW)
}

// intrinsicWidth sizes the box to its content when no width has been set.
func (p *Prompt) intrinsicWidth() int {
	w := ansi.StringWidth(p.title) + 2
	if n := ansi.StringWidth(p.placeholder) + 2; n > w {
		w = n
	}
	for _, o := range p.options {
		if n := ansi.StringWidth(o) + 4; n > w {
			w = n
		}
	}
	if n := ansi.StringWidth(promptPickHint) + 2; n > w {
		w = n
	}
	if w < 24 {
		w = 24
	}
	return w
}
