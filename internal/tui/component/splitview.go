package component

import (
	"fmt"
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

// splitChromeRows is the number of body rows a SplitView spends on its pane
// labels and the rule beneath them.
const splitChromeRows = 2

// SplitRow is one row of a SplitView: the text of its left and right panes,
// which the caller has already paired. A row with Span set draws Left across the
// full width instead, for a heading that belongs to neither pane. Line is where
// the row sits in the source the page was built from, for a caller that acts on
// a position rather than the text; 0 means it has none.
type SplitRow struct {
	Left  string
	Right string
	Span  bool
	Line  int
}

// SplitPage is one page of a SplitView: a titled body of paired rows. Pages are
// turned rather than scrolled between, so each is read on its own.
type SplitPage struct {
	Title string
	Rows  []SplitRow
}

// SplitView is a read-only popup that lays paired rows out in two panes divided
// by a vertical rule, so the two versions of a row can be compared across it.
// Both panes scroll as one on either axis, so a pair never drifts out of
// alignment. Content arrives as pages, which the view keys turn between
// (wrapping, like tabs) and which each scroll from their own top. While focused
// it scrolls with the arrow and page keys and emits DismissMsg when cancelled.
type SplitView struct {
	id           string
	title        string
	left         string
	right        string
	hint         string
	pages        []SplitPage
	page         int
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
	_ tui.Component = (*SplitView)(nil)
	_ tui.Sizeable  = (*SplitView)(nil)
	_ tui.Focusable = (*SplitView)(nil)
	_ tui.Themeable = (*SplitView)(nil)
)

// NewSplitView builds an empty side-by-side popup.
func NewSplitView(id, title string, th theme.Theme, keys keymap.KeyMap) *SplitView {
	return &SplitView{id: id, title: title, hint: "esc close", theme: th, keys: keys}
}

// Init implements tui.Component.
func (s *SplitView) Init() tea.Cmd { return nil }

// SetTitle replaces the title inlaid in the top border, so one popup can be
// reused for whatever is being compared.
func (s *SplitView) SetTitle(title string) { s.title = title }

// SetLabels names the two panes; the names head their columns above the rule.
func (s *SplitView) SetLabels(left, right string) { s.left, s.right = left, right }

// SetHint overrides the footer hint inlaid in the bottom border beside the
// scroll position. Passing "" leaves only the position.
func (s *SplitView) SetHint(hint string) { s.hint = hint }

// SetPages replaces the content and opens the first page at its top-left.
func (s *SplitView) SetPages(pages []SplitPage) {
	s.pages = pages
	s.page = 0
	s.rewind()
}

// Pages is how many pages the content is divided into.
func (s *SplitView) Pages() int { return len(s.pages) }

// Current is what the reader is looking at: the open page's title and the row at
// the top of its visible window. ok is false while there is nothing on screen.
func (s *SplitView) Current() (title string, row SplitRow, ok bool) {
	rows := s.rows()
	if len(rows) == 0 {
		return "", SplitRow{}, false
	}
	return s.pages[s.page].Title, rows[min(s.offset, len(rows)-1)], true
}

// rows are the current page's rows.
func (s *SplitView) rows() []SplitRow {
	if s.page < 0 || s.page >= len(s.pages) {
		return nil
	}
	return s.pages[s.page].Rows
}

// rewind returns to the top-left of the current page and re-measures how far it
// scrolls, since each page has its own extent.
func (s *SplitView) rewind() {
	s.offset, s.xOffset = 0, 0
	s.contentWidth = s.measureWidth()
}

// turnPage moves delta pages on, wrapping at either end.
func (s *SplitView) turnPage(delta int) {
	if len(s.pages) < 2 {
		return
	}
	s.page = (s.page + delta + len(s.pages)) % len(s.pages)
	s.rewind()
}

// SetSize implements tui.Sizeable.
func (s *SplitView) SetSize(width, height int) {
	s.width, s.height = width, height
	s.clamp()
}

// Focus implements tui.Focusable.
func (s *SplitView) Focus() { s.focused = true }

// Blur implements tui.Focusable.
func (s *SplitView) Blur() { s.focused = false }

// Focused implements tui.Focusable.
func (s *SplitView) Focused() bool { return s.focused }

// SetTheme implements tui.Themeable.
func (s *SplitView) SetTheme(th theme.Theme) { s.theme = th }

// Update scrolls both panes together while focused, and closes on cancel.
func (s *SplitView) Update(m tea.Msg) tea.Cmd {
	if !s.focused {
		return nil
	}
	km, ok := m.(tea.KeyMsg)
	if !ok {
		return nil
	}
	switch {
	case key.Matches(km, s.keys.Back):
		id := s.id
		return func() tea.Msg { return msg.DismissMsg{ID: id} }
	case key.Matches(km, s.keys.Up):
		s.offset--
	case key.Matches(km, s.keys.Down):
		s.offset++
	case key.Matches(km, s.keys.PageUp):
		s.offset -= s.pageStep()
	case key.Matches(km, s.keys.PageDown):
		s.offset += s.pageStep()
	case key.Matches(km, s.keys.Top):
		s.offset = 0
	case key.Matches(km, s.keys.Bottom):
		s.offset = len(s.rows())
	case key.Matches(km, s.keys.NextView):
		s.turnPage(1)
	case key.Matches(km, s.keys.PrevView):
		s.turnPage(-1)
	case key.Matches(km, s.keys.Left):
		s.xOffset--
	case key.Matches(km, s.keys.Right):
		s.xOffset++
	case key.Matches(km, s.keys.LineStart):
		s.xOffset = 0
	case key.Matches(km, s.keys.LineEnd):
		s.xOffset = s.contentWidth
	default:
		return nil
	}
	s.clamp()
	return nil
}

// View renders the popup as a titled box: the pane labels over a rule, then the
// visible window of the current page with a divider down the middle, and the
// page's name and scroll position inlaid in the bottom border. A frame too small
// to hold a divided row renders nothing rather than a corrupt box.
func (s *SplitView) View() string {
	if s.width < 4 || s.height < 4 {
		return ""
	}
	innerW := s.width - 2
	innerH := s.height - 2
	leftW, rightW := splitPaneWidths(innerW)
	bodyH := s.bodyHeight()
	rows := s.rows()

	border := lipgloss.NewStyle().Foreground(s.theme.Border)
	label := lipgloss.NewStyle().Foreground(s.theme.Accent).Bold(true)
	divider := border.Render(borderVertical)

	lines := make([]string, 0, splitChromeRows+bodyH)
	lines = append(lines,
		label.Render(fitLine(s.left, leftW))+divider+label.Render(fitLine(s.right, rightW)),
		border.Render(strings.Repeat(borderHorizontal, leftW)+borderCross+strings.Repeat(borderHorizontal, rightW)),
	)
	for i := 0; i < bodyH; i++ {
		var r SplitRow
		if idx := s.offset + i; idx < len(rows) {
			r = rows[idx]
		}
		// Past the end the panes are empty but still divided, so the columns read
		// as continuous down to the border.
		if r.Span {
			lines = append(lines, windowLine(r.Left, s.xOffset, innerW))
		} else {
			lines = append(lines, windowLine(r.Left, s.xOffset, leftW)+divider+windowLine(r.Right, s.xOffset, rightW))
		}
	}
	return box(strings.Join(lines, "\n"), s.title, s.footer(bodyH), innerW, innerH, s.theme, s.focused)
}

// footer is the bottom-border label: which page is open, which slice of it is on
// screen, and the hint. The page count is shown only when there is more than one
// page to turn between.
func (s *SplitView) footer(visible int) string {
	rows := s.rows()
	if len(rows) == 0 {
		return s.hint
	}
	parts := make([]string, 0, 3)
	if name := s.pageLabel(); name != "" {
		parts = append(parts, name)
	}
	parts = append(parts, fmt.Sprintf("%d-%d of %d", s.offset+1, min(s.offset+visible, len(rows)), len(rows)))
	if s.hint != "" {
		parts = append(parts, s.hint)
	}
	return strings.Join(parts, " · ")
}

// pageLabel names the open page, followed by its position once there is more
// than one.
func (s *SplitView) pageLabel() string {
	if s.page < 0 || s.page >= len(s.pages) {
		return ""
	}
	name := s.pages[s.page].Title
	if len(s.pages) < 2 {
		return name
	}
	pos := fmt.Sprintf("%d/%d", s.page+1, len(s.pages))
	if name == "" {
		return pos
	}
	return name + " (" + pos + ")"
}

// bodyHeight is how many rows fit below the labels and rule, inside the border.
func (s *SplitView) bodyHeight() int {
	return max(s.height-2-splitChromeRows, 1)
}

func (s *SplitView) pageStep() int {
	return max(s.bodyHeight()-1, 1)
}

// clamp keeps both offsets within the current page. The horizontal limit is
// measured against a single pane, since that is the width each cell is windowed
// into.
func (s *SplitView) clamp() {
	s.offset = min(max(s.offset, 0), max(len(s.rows())-s.bodyHeight(), 0))
	leftW, _ := splitPaneWidths(max(s.width-2, 0))
	s.xOffset = min(max(s.xOffset, 0), max(s.contentWidth-leftW, 0))
}

// measureWidth returns the widest tab-expanded cell on the current page — the
// horizontal extent its panes can scroll across.
func (s *SplitView) measureWidth() int {
	w := 0
	for _, r := range s.rows() {
		w = max(w, cellWidth(r.Left))
		if !r.Span {
			w = max(w, cellWidth(r.Right))
		}
	}
	return w
}

// splitPaneWidths divides innerW between the two panes, reserving one column for
// the divider between them and giving an odd remainder to the right pane.
func splitPaneWidths(innerW int) (left, right int) {
	if innerW < 1 {
		return 0, 0
	}
	left = (innerW - 1) / 2
	return left, innerW - 1 - left
}

// cellWidth is the display width of a cell once its tabs are expanded, matching
// how fitLine lays it out.
func cellWidth(s string) int {
	return ansi.StringWidth(strings.ReplaceAll(s, "\t", tabSpaces))
}
