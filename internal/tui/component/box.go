// Package component is the reusable, domain-agnostic widget library. Every
// widget satisfies the contracts in internal/tui and is individually
// renderable (see cmd/gallery). Nothing here imports the SVN domain or the app
// layer — enforced by the reusability-guard test.
package component

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/bapatchirag/revision/internal/tui/theme"
)

// Rounded box-drawing runes used by the bordered components.
const (
	borderTopLeft     = "╭"
	borderTopRight    = "╮"
	borderBottomLeft  = "╰"
	borderBottomRight = "╯"
	borderHorizontal  = "─"
	borderVertical    = "│"
)

// tabSpaces is the fixed-width expansion for a tab (see fitLine).
const tabSpaces = "    "

// fitLine pads or truncates s to exactly width display cells. Tabs are expanded
// to spaces first: terminals advance a tab to a variable-width tab stop, which
// would otherwise push fixed-width content past its cell and wrap the line,
// corrupting the layout.
func fitLine(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = strings.ReplaceAll(s, "\t", tabSpaces)
	w := ansi.StringWidth(s)
	switch {
	case w > width:
		return ansi.Truncate(s, width, "")
	case w < width:
		return s + strings.Repeat(" ", width-w)
	default:
		return s
	}
}

// maxWidth returns the widest display width among the given lines.
func maxWidth(lines []string) int {
	m := 0
	for _, l := range lines {
		if w := ansi.StringWidth(l); w > m {
			m = w
		}
	}
	return m
}

// box renders content inside a rounded border with an optional title inlaid on
// the top edge. content is fit to exactly innerW×innerH cells.
func box(content, title string, innerW, innerH int, th theme.Theme, focused bool) string {
	if innerW < 0 {
		innerW = 0
	}
	if innerH < 0 {
		innerH = 0
	}
	bs := borderStyle(th, focused)
	return boxBody(content, topBorder(title, innerW, th, bs, focused), innerW, innerH, bs)
}

// boxTabbed is box for a panel that hosts tabbed views: it inlays the panel
// number, the view names (the active one highlighted) and any drill breadcrumb
// into the top border, so no content row is spent on them. While drilled into a
// titled sub-view (crumb != ""), the tabs are replaced by just that title.
func boxTabbed(content string, number int, tabs []string, active, depth int, crumb string, innerW, innerH int, th theme.Theme, focused bool) string {
	if innerW < 0 {
		innerW = 0
	}
	if innerH < 0 {
		innerH = 0
	}
	bs := borderStyle(th, focused)
	return boxBody(content, tabTopBorder(number, tabs, active, depth, crumb, innerW, th, bs, focused), innerW, innerH, bs)
}

// boxBody renders content beneath the given top border line, padding the body to
// exactly innerW×innerH cells and closing it with the bottom border.
func boxBody(content, top string, innerW, innerH int, bs lipgloss.Style) string {
	raw := strings.Split(content, "\n")
	side := bs.Render(borderVertical)

	lines := make([]string, 0, innerH+2)
	lines = append(lines, top)
	for i := 0; i < innerH; i++ {
		var body string
		if i < len(raw) {
			body = fitLine(raw[i], innerW)
		} else {
			body = strings.Repeat(" ", innerW)
		}
		lines = append(lines, side+body+side)
	}
	lines = append(lines, bs.Render(borderBottomLeft+strings.Repeat(borderHorizontal, innerW)+borderBottomRight))
	return strings.Join(lines, "\n")
}

// borderStyle is the lipgloss style for a box's border runes, brightened when
// the box is focused.
func borderStyle(th theme.Theme, focused bool) lipgloss.Style {
	c := th.Border
	if focused {
		c = th.BorderFocused
	}
	return lipgloss.NewStyle().Foreground(c)
}

// topBorder renders the top edge of a box, inlaying title after a single dash
// when there is room for it.
func topBorder(title string, innerW int, th theme.Theme, bs lipgloss.Style, focused bool) string {
	if title == "" || innerW < 4 {
		return bs.Render(borderTopLeft + strings.Repeat(borderHorizontal, innerW) + borderTopRight)
	}
	label := " " + title + " "
	if ansi.StringWidth(label) > innerW-1 {
		label = ansi.Truncate(label, innerW-1, "")
	}
	ts := lipgloss.NewStyle().Foreground(th.Accent)
	if focused {
		ts = ts.Bold(true)
	}
	dashes := innerW - 1 - ansi.StringWidth(label)
	if dashes < 0 {
		dashes = 0
	}
	return bs.Render(borderTopLeft+borderHorizontal) +
		ts.Render(label) +
		bs.Render(strings.Repeat(borderHorizontal, dashes)+borderTopRight)
}

// tabTopBorder renders the top edge of a tabbed panel, inlaying the panel number
// and each view name into the border with the active view highlighted, plus a
// chevron per drill-down depth. While drilled into a titled sub-view (crumb !=
// "") the tabs and chevron are replaced by just the crumb title. The middle is
// fit to exactly innerW-1 cells so the row width matches the body.
func tabTopBorder(number int, tabs []string, active, depth int, crumb string, innerW int, th theme.Theme, bs lipgloss.Style, focused bool) string {
	if innerW < 4 || len(tabs) == 0 {
		return bs.Render(borderTopLeft + strings.Repeat(borderHorizontal, innerW) + borderTopRight)
	}

	// Drilled into a titled sub-view: show just the panel number and that title.
	if depth > 0 && crumb != "" {
		return numberedTitleBorder(number, crumb, innerW, th, bs, focused)
	}

	numStyle := lipgloss.NewStyle().Foreground(th.Accent)
	activeStyle := lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	if focused {
		numStyle = numStyle.Bold(true)
	}
	mutedStyle := lipgloss.NewStyle().Foreground(th.Muted)

	var b strings.Builder
	used := 0
	dash := func(n int) {
		if n > 0 {
			b.WriteString(bs.Render(strings.Repeat(borderHorizontal, n)))
			used += n
		}
	}
	text := func(s string, st lipgloss.Style) {
		b.WriteString(st.Render(s))
		used += ansi.StringWidth(s)
	}

	text(" ", bs)
	if number > 0 {
		text("["+strconv.Itoa(number)+"]", numStyle)
	}
	for i, tab := range tabs {
		text(" ", bs)
		if i == 0 {
			dash(1)
		} else {
			dash(3)
		}
		text(" ", bs)
		if i == active {
			text(tab, activeStyle)
		} else {
			text(tab, mutedStyle)
		}
	}
	if depth > 0 {
		text(" ", bs)
		dash(1)
		text(" ", bs)
		text(strings.Repeat(">", depth), activeStyle)
	}
	text(" ", bs)

	middle := b.String()
	switch {
	case used < innerW-1:
		middle += bs.Render(strings.Repeat(borderHorizontal, innerW-1-used))
	case used > innerW-1:
		middle = ansi.Truncate(middle, innerW-1, "")
	}
	return bs.Render(borderTopLeft+borderHorizontal) + middle + bs.Render(borderTopRight)
}

// numberedTitleBorder renders a top edge carrying just the panel number and a
// single title (e.g. the changelist a drilled-in panel is showing), fit to
// innerW-1 cells. It is used in place of the tab strip while drilled in.
func numberedTitleBorder(number int, title string, innerW int, th theme.Theme, bs lipgloss.Style, focused bool) string {
	numStyle := lipgloss.NewStyle().Foreground(th.Accent)
	titleStyle := lipgloss.NewStyle().Foreground(th.Accent)
	if focused {
		numStyle = numStyle.Bold(true)
		titleStyle = titleStyle.Bold(true)
	}

	var b strings.Builder
	used := 0
	dash := func(n int) {
		if n > 0 {
			b.WriteString(bs.Render(strings.Repeat(borderHorizontal, n)))
			used += n
		}
	}
	text := func(s string, st lipgloss.Style) {
		b.WriteString(st.Render(s))
		used += ansi.StringWidth(s)
	}

	text(" ", bs)
	if number > 0 {
		text("["+strconv.Itoa(number)+"]", numStyle)
	}
	text(" ", bs)
	dash(1)
	text(" ", bs)
	text(title, titleStyle)
	text(" ", bs)

	middle := b.String()
	switch {
	case used < innerW-1:
		middle += bs.Render(strings.Repeat(borderHorizontal, innerW-1-used))
	case used > innerW-1:
		middle = ansi.Truncate(middle, innerW-1, "")
	}
	return bs.Render(borderTopLeft+borderHorizontal) + middle + bs.Render(borderTopRight)
}
