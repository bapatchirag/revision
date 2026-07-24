package app

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/bapatchirag/revision/internal/tui/theme"
)

// colorizeDiff is the domain adapter that maps the role of each unified-diff
// line onto a theme color, giving the Main viewport familiar diff syntax
// highlighting. It is the only place SVN diff structure is mapped onto colors,
// keeping the Viewport component diff-agnostic. Only styling is added: the
// Viewport's width math is ANSI-aware, so highlighted lines truncate and scroll
// exactly like plain ones. An empty diff is returned unchanged.
//
// Metadata lines ("Index:", the "===" rule and the "---"/"+++" file markers) are
// muted, a hunk header ("@@") takes the accent, added lines ("+") use the success
// color and removed lines ("-") the error color. The "---"/"+++" markers are
// matched before the single-character "-"/"+" cases so they read as headers
// rather than a giant delete/add.
//
// Tab conversion is disabled on the styles so this stays a pure styling pass;
// the Viewport remains the single owner of tab expansion (via tabSpaces), so
// colored lines align identically to plain ones.
func colorizeDiff(th theme.Theme, diff string) string {
	if diff == "" {
		return ""
	}
	var (
		meta = lipgloss.NewStyle().Foreground(th.Muted).TabWidth(lipgloss.NoTabConversion)
		hunk = lipgloss.NewStyle().Foreground(th.Accent).Bold(true).TabWidth(lipgloss.NoTabConversion)
		add  = lipgloss.NewStyle().Foreground(th.Success).TabWidth(lipgloss.NoTabConversion)
		del  = lipgloss.NewStyle().Foreground(th.Error).TabWidth(lipgloss.NoTabConversion)
	)
	lines := strings.Split(diff, "\n")
	for i, ln := range lines {
		switch {
		case strings.HasPrefix(ln, "Index:"), strings.HasPrefix(ln, "==="),
			strings.HasPrefix(ln, "---"), strings.HasPrefix(ln, "+++"):
			lines[i] = meta.Render(ln)
		case strings.HasPrefix(ln, "@@"):
			lines[i] = hunk.Render(ln)
		case strings.HasPrefix(ln, "+"):
			lines[i] = add.Render(ln)
		case strings.HasPrefix(ln, "-"):
			lines[i] = del.Render(ln)
		}
	}
	return strings.Join(lines, "\n")
}
