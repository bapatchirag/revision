package component

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/bapatchirag/revision/internal/tui"
	"github.com/bapatchirag/revision/internal/tui/keymap"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

// Viewport is a scrollable, read-only text area. It renders a vertical window
// over its content and scrolls with the arrow and page keys while focused. It can
// also highlight the lines matching a search query and jump between them without
// removing any content, and — where the caller has something to do with a single
// line — carry a cursor over one of them.
type Viewport struct {
	lines        []string
	offset       int
	xOffset      int
	cursor       int
	hasCursor    bool
	gutter       int
	contentWidth int
	search       string
	matches      []int
	matchSet     map[int]bool
	current      int
	width        int
	height       int
	focused      bool
	theme        theme.Theme
	keys         keymap.KeyMap
}

var (
	_ tui.Component = (*Viewport)(nil)
	_ tui.Sizeable  = (*Viewport)(nil)
	_ tui.Focusable = (*Viewport)(nil)
	_ tui.Themeable = (*Viewport)(nil)
)

// NewViewport builds an empty viewport.
func NewViewport(th theme.Theme, keys keymap.KeyMap) *Viewport {
	return &Viewport{theme: th, keys: keys, current: -1}
}

// Init implements tui.Component.
func (v *Viewport) Init() tea.Cmd { return nil }

// SetContent replaces the viewport text and resets scrolling to the top. An
// active search is re-evaluated against the new content so its highlights stay
// valid, though no match is left selected until the next SetSearch or jump.
func (v *Viewport) SetContent(content string) {
	v.setLines(content)
	v.offset = 0
	v.xOffset = 0
	v.cursor = 0
	v.recomputeMatches()
	v.clampOffset()
	v.clampXOffset()
}

// SetContentPreservingScroll replaces the viewport text but keeps the scroll
// position, clamping it to the new content rather than jumping back to the top.
// It is for a refresh of what is already on screen, where the reader's place is
// still meaningful; SetContent remains the right call for new content.
func (v *Viewport) SetContentPreservingScroll(content string) {
	v.setLines(content)
	v.recomputeMatches()
	v.clampOffset()
	v.clampXOffset()
	v.clampCursor()
}

// SetCursorLine turns the line cursor on or off. With it on the viewport marks
// one line as the current one, moves it with the scroll keys and keeps it inside
// the window, giving the caller a line to act on; with it off the keys only
// scroll. The cursor keeps its place across a toggle, so a redraw that re-arms it
// does not send the reader back to the top.
func (v *Viewport) SetCursorLine(on bool) {
	v.hasCursor = on
	v.clampCursor()
}

// Cursor is the index of the current line, or -1 when the viewport has no
// cursor.
func (v *Viewport) Cursor() int {
	if !v.hasCursor {
		return -1
	}
	return v.cursor
}

// Offset is the index of the content line drawn at the top of the visible
// window.
func (v *Viewport) Offset() int { return v.offset }

// ClickRow moves the line cursor to row of the viewport's own area (0 is its
// first row), as the scroll keys move it. A viewport with no cursor has nothing
// to point at, and a row past the content has no line to point to.
func (v *Viewport) ClickRow(row int) tea.Cmd {
	_, innerH, _, _ := v.layout()
	i := v.offset + row
	if !v.hasCursor || row < 0 || row >= innerH || i >= len(v.lines) {
		return nil
	}
	v.cursor = i
	return nil
}

// setLines splits content into the rendered lines and re-measures the horizontal
// extent, leaving the scroll offsets to the caller.
func (v *Viewport) setLines(content string) {
	if content == "" {
		v.lines = nil
	} else {
		v.lines = strings.Split(content, "\n")
	}
	v.contentWidth = v.measureWidth()
}

// SetSize implements tui.Sizeable.
func (v *Viewport) SetSize(width, height int) {
	v.width, v.height = width, height
	v.clampOffset()
	v.clampXOffset()
}

// SetGutter pins the first n display columns so they stay fixed while the body
// scrolls horizontally (n <= 0 disables the pin). It keeps a leading marker
// column — such as a unified diff's +/-/space gutter — visible no matter how far
// right the content is scrolled.
func (v *Viewport) SetGutter(n int) {
	if n < 0 {
		n = 0
	}
	v.gutter = n
}

// SetSearch sets the case-insensitive search query, highlighting every line whose
// visible text contains it and moving the current match to the first hit, which
// it scrolls into view. It returns the number of matching lines; an empty query
// clears the search.
func (v *Viewport) SetSearch(query string) int {
	v.search = query
	v.recomputeMatches()
	if len(v.matches) > 0 {
		v.current = 0
		v.scrollToCurrent()
	}
	return len(v.matches)
}

// ClearSearch removes any active search highlight.
func (v *Viewport) ClearSearch() {
	v.search = ""
	v.matches = nil
	v.matchSet = nil
	v.current = -1
}

// MatchCount returns the number of lines matching the active search.
func (v *Viewport) MatchCount() int { return len(v.matches) }

// CurrentMatch returns the 1-based position of the current match, or 0 when none
// is selected (no matches, or the content changed since the last jump).
func (v *Viewport) CurrentMatch() int {
	if v.current < 0 || v.current >= len(v.matches) {
		return 0
	}
	return v.current + 1
}

// NextMatch moves the current match forward (wrapping) and scrolls it into view.
// It reports whether a match was available to move to.
func (v *Viewport) NextMatch() bool { return v.step(1) }

// PrevMatch moves the current match backward (wrapping) and scrolls it into view.
// It reports whether a match was available to move to.
func (v *Viewport) PrevMatch() bool { return v.step(-1) }

// step advances the current match by delta (±1), wrapping around the ends. From
// no selection it starts at the first match going forward, or the last going
// backward.
func (v *Viewport) step(delta int) bool {
	n := len(v.matches)
	if n == 0 {
		return false
	}
	switch {
	case v.current < 0 && delta > 0:
		v.current = 0
	case v.current < 0:
		v.current = n - 1
	default:
		v.current = (v.current + delta + n) % n
	}
	v.scrollToCurrent()
	return true
}

// recomputeMatches rebuilds the match set for the current search over the current
// lines, keeping the search valid after the content changes. It leaves no match
// selected (current = -1) until the next SetSearch or jump.
func (v *Viewport) recomputeMatches() {
	v.matches = nil
	v.matchSet = nil
	v.current = -1
	q := strings.ToLower(strings.TrimSpace(v.search))
	if q == "" {
		return
	}
	set := make(map[int]bool)
	for i, ln := range v.lines {
		if strings.Contains(strings.ToLower(ansi.Strip(ln)), q) {
			v.matches = append(v.matches, i)
			set[i] = true
		}
	}
	v.matchSet = set
}

// scrollToCurrent centers the current match line in the visible window, taking
// the cursor onto it so the match is also the line to act on.
func (v *Viewport) scrollToCurrent() {
	if v.current < 0 || v.current >= len(v.matches) {
		return
	}
	_, innerH, _, _ := v.layout()
	if innerH < 1 {
		innerH = 1
	}
	v.offset = v.matches[v.current] - innerH/2
	v.cursor = v.matches[v.current]
	v.clampOffset()
	v.clampCursor()
}

// Focus implements tui.Focusable.
func (v *Viewport) Focus() { v.focused = true }

// Blur implements tui.Focusable.
func (v *Viewport) Blur() { v.focused = false }

// Focused implements tui.Focusable.
func (v *Viewport) Focused() bool { return v.focused }

// SetTheme implements tui.Themeable.
func (v *Viewport) SetTheme(th theme.Theme) { v.theme = th }

// Update scrolls the viewport while focused.
func (v *Viewport) Update(m tea.Msg) tea.Cmd {
	if !v.focused {
		return nil
	}
	km, ok := m.(tea.KeyMsg)
	if !ok {
		return nil
	}
	switch {
	case key.Matches(km, v.keys.Up):
		v.move(-1)
	case key.Matches(km, v.keys.Down):
		v.move(1)
	case key.Matches(km, v.keys.PageUp):
		v.page(-v.pageStep())
	case key.Matches(km, v.keys.PageDown):
		v.page(v.pageStep())
	case key.Matches(km, v.keys.Top):
		v.offset, v.cursor = 0, 0
	case key.Matches(km, v.keys.Bottom):
		v.offset, v.cursor = len(v.lines), len(v.lines)-1
	case key.Matches(km, v.keys.Left):
		v.xOffset -= hScrollStep
	case key.Matches(km, v.keys.Right):
		v.xOffset += hScrollStep
	case key.Matches(km, v.keys.LineStart):
		v.xOffset = 0
	case key.Matches(km, v.keys.LineEnd):
		v.xOffset = v.contentWidth
	default:
		return nil
	}
	v.clampOffset()
	v.clampCursor()
	v.clampXOffset()
	return nil
}

// move steps delta lines down the content: the cursor when there is one, which
// the window then follows only as far as it must to keep it in sight, and the
// window itself when there is not.
func (v *Viewport) move(delta int) {
	if !v.hasCursor {
		v.offset += delta
		return
	}
	v.cursor += delta
	v.clampCursor()
	_, innerH, _, _ := v.layout()
	v.offset = min(v.offset, v.cursor)
	v.offset = max(v.offset, v.cursor-max(innerH, 1)+1)
}

// page turns the window delta lines, taking the cursor along at the row it was
// on so the reader keeps their place on screen. Clamping either end brings the
// other back into the window, so the cursor never lands off it.
func (v *Viewport) page(delta int) {
	v.offset += delta
	v.cursor += delta
}

// clampCursor keeps the cursor on a line that exists.
func (v *Viewport) clampCursor() {
	v.cursor = min(max(v.cursor, 0), max(len(v.lines)-1, 0))
}

// View renders the visible window padded to width×height, drawing vertical and
// horizontal scrollbars along the right column and bottom row whenever the
// content overflows that axis. Each thumb is sized to the visible fraction and
// positioned by the scroll offset, so it tracks like a regular scrollbar.
func (v *Viewport) View() string {
	if v.width <= 0 {
		return ""
	}
	if v.height <= 0 {
		out := make([]string, 0, len(v.lines))
		for i, ln := range v.lines {
			row := v.window(ln, v.width)
			out = append(out, v.highlightMatch(i, row, v.width))
		}
		return strings.Join(out, "\n")
	}

	innerW, innerH, vBar, hBar := v.layout()

	vStart, vSize := 0, 0
	if vBar {
		vStart, vSize = scrollbarThumb(len(v.lines), innerH, v.offset, innerH)
	}

	rows := make([]string, 0, v.height)
	for i := 0; i < innerH; i++ {
		idx := v.offset + i
		line := ""
		if idx < len(v.lines) {
			line = v.lines[idx]
		}
		row := v.window(line, innerW)
		if idx < len(v.lines) {
			row = v.highlightMatch(idx, row, innerW)
			row = v.highlightCursor(idx, row, innerW)
		}
		if vBar {
			if i >= vStart && i < vStart+vSize {
				row += vScrollThumb
			} else {
				row += vScrollTrack
			}
		}
		rows = append(rows, row)
	}
	if hBar {
		rows = append(rows, v.horizontalBar(innerW, vBar))
	}
	return strings.Join(rows, "\n")
}

// highlightMatch paints a search-match line: the current match as a reverse-video
// bar (over its plain text, so it is unmistakable and always legible), and every
// other match with a subtle background bar that preserves the line's own colors.
// Non-match lines are returned unchanged.
func (v *Viewport) highlightMatch(idx int, row string, width int) string {
	if len(v.matches) == 0 || !v.matchSet[idx] {
		return row
	}
	if v.current >= 0 && v.current < len(v.matches) && v.matches[v.current] == idx {
		return lipgloss.NewStyle().Reverse(true).Render(fitLine(ansi.Strip(row), width))
	}
	return highlightLine(row, width, lipgloss.NewStyle().Background(v.theme.SelectionBg))
}

// highlightCursor paints the current line's bar, marking the line the caller
// would act on. It is drawn only while focused, since the cursor moves with this
// viewport's own keys, and it gives way to the current search match, which is
// already unmistakable.
func (v *Viewport) highlightCursor(idx int, row string, width int) string {
	if !v.hasCursor || !v.focused || idx != v.cursor {
		return row
	}
	if v.current >= 0 && v.current < len(v.matches) && v.matches[v.current] == idx {
		return row
	}
	return highlightLine(row, width, lipgloss.NewStyle().Background(v.theme.SelectionBg))
}

// horizontalBar renders the bottom scrollbar row spanning innerW cells, adding a
// corner cell when a vertical bar shares the frame.
func (v *Viewport) horizontalBar(innerW int, corner bool) string {
	return horizontalBarRow(v.contentWidth, v.xOffset, innerW, corner)
}

// window renders the horizontal slice [xOffset, xOffset+width) of a line, padded
// to width, keeping the first v.gutter columns pinned. Tabs are expanded first so
// the offset counts display cells, mirroring fitLine's fixed-width expansion.
func (v *Viewport) window(s string, width int) string {
	return windowLineGutter(s, v.gutter, v.xOffset, width)
}

// hScrollStep is the column count a single Left/Right press scrolls.
const hScrollStep = 1

// Scrollbar glyphs: a heavy (slightly thick) line for the thumb over a light line
// track, using the matching orientation for each axis.
const (
	vScrollTrack = "│"
	vScrollThumb = "┃"
	hScrollTrack = "─"
	hScrollThumb = "━"
	scrollCorner = " "
)

// layout resolves the inner content dimensions and which scrollbars are shown. A
// bar appears whenever its axis overflows the content area (regardless of focus);
// since showing one bar shrinks the opposite axis (which can itself tip into
// overflow), the flags are resolved with a short fixpoint. A bar is suppressed
// when the frame is too small to spare a row or column for content.
func (v *Viewport) layout() (innerW, innerH int, vBar, hBar bool) {
	return scrollLayout(len(v.lines), v.contentWidth, v.width, v.height, 0)
}

// scrollbarThumb sizes and positions a scrollbar thumb: for a track of trackLen
// cells showing `visible` of `total` items scrolled to `offset`, it returns the
// thumb's start cell and length.
func scrollbarThumb(total, visible, offset, trackLen int) (start, size int) {
	if trackLen <= 0 {
		return 0, 0
	}
	if total <= visible || total <= 0 {
		return 0, trackLen
	}
	size = visible * trackLen / total
	if size < 1 {
		size = 1
	}
	maxStart := trackLen - size
	if maxOffset := total - visible; maxOffset > 0 {
		start = offset * maxStart / maxOffset
	}
	if start > maxStart {
		start = maxStart
	}
	if start < 0 {
		start = 0
	}
	return start, size
}

// appendVScrollbar appends a vertical scrollbar cell — a heavy thumb over a light
// track — to each row, sized and positioned for `total` items scrolled to `offset`
// across a track as tall as len(rows). Each row must already be at its final
// content width; the bar adds one cell on the right.
func appendVScrollbar(rows []string, total, offset int) {
	n := len(rows)
	start, size := scrollbarThumb(total, n, offset, n)
	for i := range rows {
		if i >= start && i < start+size {
			rows[i] += vScrollThumb
		} else {
			rows[i] += vScrollTrack
		}
	}
}

// windowLine renders the horizontal slice [xOffset, xOffset+width) of s, padded to
// width. Tabs are expanded first so the offset counts display cells, mirroring
// fitLine's fixed-width expansion.
func windowLine(s string, xOffset, width int) string {
	s = strings.ReplaceAll(s, "\t", tabSpaces)
	if xOffset > 0 {
		s = ansi.Cut(s, xOffset, xOffset+width)
	}
	return fitLine(s, width)
}

// windowLineGutter renders s into width cells like windowLine, but pins the first
// `gutter` display columns so they never scroll: the leading gutter cells are
// always drawn from the start of the line and only the remainder past them slides
// by xOffset. This keeps a fixed left column — e.g. a unified diff's +/-/space
// marker — in view while the body scrolls horizontally. With gutter <= 0 (or when
// the pane is too narrow to spare room past the gutter) it is identical to
// windowLine.
func windowLineGutter(s string, gutter, xOffset, width int) string {
	if gutter <= 0 || xOffset <= 0 || gutter >= width {
		return windowLine(s, xOffset, width)
	}
	s = strings.ReplaceAll(s, "\t", tabSpaces)
	head := ansi.Cut(s, 0, gutter)
	tail := ansi.Cut(s, gutter+xOffset, gutter+xOffset+width-gutter)
	return fitLine(head+tail, width)
}

// horizontalBarRow renders a horizontal scrollbar spanning innerW cells for
// content of contentWidth scrolled to xOffset, adding a corner cell when a
// vertical bar shares the frame.
func horizontalBarRow(contentWidth, xOffset, innerW int, corner bool) string {
	start, size := scrollbarThumb(contentWidth, innerW, xOffset, innerW)
	var b strings.Builder
	for c := 0; c < innerW; c++ {
		if c >= start && c < start+size {
			b.WriteString(hScrollThumb)
		} else {
			b.WriteString(hScrollTrack)
		}
	}
	if corner {
		b.WriteString(scrollCorner)
	}
	return b.String()
}

// scrollLayout decides which scrollbars are needed and the resulting inner
// content size. reserveTop is the number of fixed rows above the scrollable body
// (e.g. a table header); total is the count of scrollable items and contentWidth
// the widest line. Showing one bar shrinks the opposite axis, so the flags are
// resolved with a short fixpoint; a bar is suppressed when the frame is too small.
func scrollLayout(total, contentWidth, width, height, reserveTop int) (innerW, innerH int, vBar, hBar bool) {
	innerW, innerH = width, height
	if width < 1 || height < 1 {
		return innerW, innerH, false, false
	}
	for i := 0; i < 2; i++ {
		bodyRows := innerH - reserveTop
		vBar = width >= 2 && total > bodyRows
		hBar = height >= 2 && contentWidth > innerW
		innerW = width
		if vBar {
			innerW--
		}
		innerH = height
		if hBar {
			innerH--
		}
	}
	return innerW, innerH, vBar, hBar
}

func (v *Viewport) pageStep() int {
	if _, innerH, _, _ := v.layout(); innerH > 1 {
		return innerH - 1
	}
	return 1
}

func (v *Viewport) clampOffset() {
	_, innerH, _, _ := v.layout()
	maxOff := len(v.lines) - innerH
	if maxOff < 0 {
		maxOff = 0
	}
	if v.offset > maxOff {
		v.offset = maxOff
	}
	if v.offset < 0 {
		v.offset = 0
	}
}

func (v *Viewport) clampXOffset() {
	innerW, _, _, _ := v.layout()
	maxX := v.contentWidth - innerW
	if maxX < 0 {
		maxX = 0
	}
	if v.xOffset > maxX {
		v.xOffset = maxX
	}
	if v.xOffset < 0 {
		v.xOffset = 0
	}
}

// measureWidth returns the widest tab-expanded line — the horizontal extent the
// content can scroll across.
func (v *Viewport) measureWidth() int {
	m := 0
	for _, ln := range v.lines {
		if w := ansi.StringWidth(strings.ReplaceAll(ln, "\t", tabSpaces)); w > m {
			m = w
		}
	}
	return m
}
