package component

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/bapatchirag/revision/internal/tui"
	"github.com/bapatchirag/revision/internal/tui/keymap"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

// Viewport is a scrollable, read-only text area. It renders a vertical window
// over its content and scrolls with the arrow and page keys while focused.
type Viewport struct {
	lines        []string
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
	_ tui.Component = (*Viewport)(nil)
	_ tui.Sizeable  = (*Viewport)(nil)
	_ tui.Focusable = (*Viewport)(nil)
	_ tui.Themeable = (*Viewport)(nil)
)

// NewViewport builds an empty viewport.
func NewViewport(th theme.Theme, keys keymap.KeyMap) *Viewport {
	return &Viewport{theme: th, keys: keys}
}

// Init implements tui.Component.
func (v *Viewport) Init() tea.Cmd { return nil }

// SetContent replaces the viewport text and resets scrolling to the top.
func (v *Viewport) SetContent(content string) {
	if content == "" {
		v.lines = nil
	} else {
		v.lines = strings.Split(content, "\n")
	}
	v.offset = 0
	v.xOffset = 0
	v.contentWidth = v.measureWidth()
	v.clampOffset()
	v.clampXOffset()
}

// SetSize implements tui.Sizeable.
func (v *Viewport) SetSize(width, height int) {
	v.width, v.height = width, height
	v.clampOffset()
	v.clampXOffset()
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
		v.offset--
	case key.Matches(km, v.keys.Down):
		v.offset++
	case key.Matches(km, v.keys.PageUp):
		v.offset -= v.pageStep()
	case key.Matches(km, v.keys.PageDown):
		v.offset += v.pageStep()
	case key.Matches(km, v.keys.Top):
		v.offset = 0
	case key.Matches(km, v.keys.Bottom):
		v.offset = len(v.lines)
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
	v.clampXOffset()
	return nil
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
		for _, ln := range v.lines {
			out = append(out, v.window(ln, v.width))
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

// horizontalBar renders the bottom scrollbar row spanning innerW cells, adding a
// corner cell when a vertical bar shares the frame.
func (v *Viewport) horizontalBar(innerW int, corner bool) string {
	return horizontalBarRow(v.contentWidth, v.xOffset, innerW, corner)
}

// window renders the horizontal slice [xOffset, xOffset+width) of a line, padded
// to width. Tabs are expanded first so the offset counts display cells, mirroring
// fitLine's fixed-width expansion.
func (v *Viewport) window(s string, width int) string {
	return windowLine(s, v.xOffset, width)
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
