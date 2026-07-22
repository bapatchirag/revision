// Package component is the reusable, domain-agnostic widget library. Every
// widget satisfies the contracts in internal/tui and is individually
// renderable (see cmd/gallery). Nothing here imports the SVN domain or the app
// layer — enforced by the reusability-guard test.
package component

import (
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

// fitLine pads or truncates s to exactly width display cells.
func fitLine(s string, width int) string {
	if width <= 0 {
		return ""
	}
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

	borderColor := th.Border
	if focused {
		borderColor = th.BorderFocused
	}
	bs := lipgloss.NewStyle().Foreground(borderColor)

	raw := strings.Split(content, "\n")
	side := bs.Render(borderVertical)

	lines := make([]string, 0, innerH+2)
	lines = append(lines, topBorder(title, innerW, th, bs, focused))
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
