// Package layout provides composition helpers for arranging rendered blocks:
// placement within a box and overlaying a popup on top of a background.
package layout

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// resetSeq closes any open SGR styling between spliced segments.
const resetSeq = "\x1b[0m"

// Place positions content within a width×height box at the given alignment.
func Place(width, height int, hPos, vPos lipgloss.Position, content string) string {
	return lipgloss.Place(width, height, hPos, vPos, content)
}

// Center places content in the middle of a width×height box.
func Center(width, height int, content string) string {
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}

// Overlay composites fg on top of bg with fg's top-left corner at (x, y). It is
// ANSI-aware: the background is sliced by visible column (preserving styling
// outside fg), so it composites correctly over a styled layout. The background
// styling directly beneath fg is replaced by fg, which is assumed to be opaque.
func Overlay(bg, fg string, x, y int) string {
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	bgLines := strings.Split(bg, "\n")
	fgLines := strings.Split(fg, "\n")
	for i, fgLine := range fgLines {
		row := y + i
		if row >= len(bgLines) {
			break
		}
		bgLines[row] = overlayLine(bgLines[row], fgLine, x)
	}
	return strings.Join(bgLines, "\n")
}

// overlayLine splices fg into bg starting at visible column x, padding bg with
// spaces when it is shorter than x. Slicing is done by display width and
// preserves ANSI styling on the exposed parts of the background; a reset is
// inserted around a styled segment so its color never bleeds into its neighbor.
func overlayLine(bg, fg string, x int) string {
	fgWidth := ansi.StringWidth(fg)
	bgWidth := ansi.StringWidth(bg)

	left := ansi.Cut(bg, 0, x)
	if lw := ansi.StringWidth(left); lw < x {
		left += strings.Repeat(" ", x-lw)
	}

	var right string
	if end := x + fgWidth; end < bgWidth {
		right = ansi.Cut(bg, end, bgWidth)
	}

	var b strings.Builder
	b.WriteString(left)
	if hasANSI(left) {
		b.WriteString(resetSeq)
	}
	b.WriteString(fg)
	if hasANSI(fg) {
		b.WriteString(resetSeq)
	}
	b.WriteString(right)
	return b.String()
}

func hasANSI(s string) bool { return strings.Contains(s, "\x1b") }
