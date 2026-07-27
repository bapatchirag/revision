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

// searchGlyph leads the filter line, echoing the "/" key that opens it.
const searchGlyph = "/"

// SearchBar is a slim, single-line filter input rendered inline (no border) so
// it can sit in place of the status bar while a panel is being filtered. Unlike
// Prompt it has no box, title or option list: it is just a leading glyph, a
// muted prefix that names the panel and its parameters, and the editable query.
// It emits msg.SubmitMsg with the current query on enter and msg.DismissMsg on
// esc. Editing keys are matched by key type so letters bound to navigation
// elsewhere (h/j/k/l, y/n) are typed as literal text.
type SearchBar struct {
	id      string
	prefix  string
	value   []rune
	col     int
	width   int
	focused bool
	theme   theme.Theme
	keys    keymap.KeyMap
}

var (
	_ tui.Component = (*SearchBar)(nil)
	_ tui.Sizeable  = (*SearchBar)(nil)
	_ tui.Focusable = (*SearchBar)(nil)
	_ tui.Themeable = (*SearchBar)(nil)
)

// NewSearchBar builds an empty filter input identified by id (used on emitted
// messages).
func NewSearchBar(id string, th theme.Theme, keys keymap.KeyMap) *SearchBar {
	return &SearchBar{id: id, theme: th, keys: keys}
}

// SetPrefix sets the muted label shown between the glyph and the query, used to
// name the panel being filtered and hint at its available parameters.
func (s *SearchBar) SetPrefix(prefix string) { s.prefix = prefix }

// Value returns the current query text.
func (s *SearchBar) Value() string { return string(s.value) }

// SetValue replaces the query text, placing the cursor at the end.
func (s *SearchBar) SetValue(v string) {
	s.value = []rune(v)
	s.col = len(s.value)
}

// Reset clears the query and returns the cursor to the start.
func (s *SearchBar) Reset() {
	s.value = nil
	s.col = 0
}

// Init implements tui.Component.
func (s *SearchBar) Init() tea.Cmd { return nil }

// SetSize implements tui.Sizeable; only the width is used (the height is one row).
func (s *SearchBar) SetSize(width, _ int) { s.width = width }

// Focus implements tui.Focusable.
func (s *SearchBar) Focus() { s.focused = true }

// Blur implements tui.Focusable.
func (s *SearchBar) Blur() { s.focused = false }

// Focused implements tui.Focusable.
func (s *SearchBar) Focused() bool { return s.focused }

// SetTheme implements tui.Themeable.
func (s *SearchBar) SetTheme(th theme.Theme) { s.theme = th }

// Update edits the query while focused. Enter emits SubmitMsg and esc emits
// DismissMsg; every other key edits the query in place. Enter and esc are
// matched against their dedicated bindings (not Confirm/Cancel) so that the
// "y"/"n" letters those also bind stay typable as literal text.
func (s *SearchBar) Update(m tea.Msg) tea.Cmd {
	if !s.focused {
		return nil
	}
	km, ok := m.(tea.KeyMsg)
	if !ok {
		return nil
	}
	switch {
	case key.Matches(km, s.keys.Enter):
		id, val := s.id, s.Value()
		return func() tea.Msg { return msg.SubmitMsg{ID: id, Value: val} }
	case key.Matches(km, s.keys.Back):
		id := s.id
		return func() tea.Msg { return msg.DismissMsg{ID: id} }
	}
	switch km.Type {
	case tea.KeyLeft:
		if s.col > 0 {
			s.col--
		}
	case tea.KeyRight:
		if s.col < len(s.value) {
			s.col++
		}
	case tea.KeyHome:
		s.col = 0
	case tea.KeyEnd:
		s.col = len(s.value)
	case tea.KeyBackspace:
		s.backspace()
	case tea.KeySpace:
		s.insert([]rune{' '})
	case tea.KeyRunes:
		s.insert(km.Runes)
	}
	return nil
}

// insert adds rs at the cursor.
func (s *SearchBar) insert(rs []rune) {
	if s.col > len(s.value) {
		s.col = len(s.value)
	}
	next := make([]rune, 0, len(s.value)+len(rs))
	next = append(next, s.value[:s.col]...)
	next = append(next, rs...)
	next = append(next, s.value[s.col:]...)
	s.value = next
	s.col += len(rs)
}

// backspace deletes the rune before the cursor.
func (s *SearchBar) backspace() {
	if s.col == 0 {
		return
	}
	s.value = append(s.value[:s.col-1], s.value[s.col:]...)
	s.col--
}

// View renders the one-line filter input, padded to the full width: a leading
// glyph, the muted panel/param prefix, and the query with a reverse-video cursor
// while focused.
func (s *SearchBar) View() string {
	glyph := lipgloss.NewStyle().Foreground(s.theme.Accent).Bold(true).Render(searchGlyph)
	var b strings.Builder
	b.WriteString(glyph)
	b.WriteByte(' ')
	if s.prefix != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(s.theme.Muted).Render(s.prefix))
		b.WriteByte(' ')
	}
	b.WriteString(s.queryField())
	if s.width <= 0 {
		return b.String()
	}
	return fitLine(b.String(), s.width)
}

// queryField renders the editable query with a reverse-video cursor at the
// insertion point while focused.
func (s *SearchBar) queryField() string {
	text := lipgloss.NewStyle().Foreground(s.theme.Text)
	if !s.focused {
		return text.Render(string(s.value))
	}
	col := s.col
	if col > len(s.value) {
		col = len(s.value)
	}
	cursor := lipgloss.NewStyle().Reverse(true)
	left := text.Render(string(s.value[:col]))
	if col == len(s.value) {
		return left + cursor.Render(" ")
	}
	return left + cursor.Render(string(s.value[col])) + text.Render(string(s.value[col+1:]))
}
