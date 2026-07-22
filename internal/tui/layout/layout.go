// Package layout provides composition helpers for arranging rendered blocks:
// placement within a box and overlaying a popup on top of a background.
package layout

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Place positions content within a width×height box at the given alignment.
func Place(width, height int, hPos, vPos lipgloss.Position, content string) string {
	return lipgloss.Place(width, height, hPos, vPos, content)
}

// Center places content in the middle of a width×height box.
func Center(width, height int, content string) string {
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}

// Overlay composites fg on top of bg with fg's top-left corner at (x, y). It is
// a best-effort, cell-based overlay intended for plain (already-rendered)
// popups over a plain background; it does not attempt to preserve interleaved
// ANSI styling from the background beneath fg.
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

// overlayLine splices fg into bg starting at rune column x, padding bg with
// spaces if it is shorter than x.
func overlayLine(bg, fg string, x int) string {
	bgRunes := []rune(bg)
	fgRunes := []rune(fg)
	for len(bgRunes) < x {
		bgRunes = append(bgRunes, ' ')
	}
	out := make([]rune, 0, len(bgRunes)+len(fgRunes))
	out = append(out, bgRunes[:x]...)
	out = append(out, fgRunes...)
	if tail := x + len(fgRunes); tail < len(bgRunes) {
		out = append(out, bgRunes[tail:]...)
	}
	return string(out)
}
