package app

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/bapatchirag/revision/internal/svn"
	"github.com/bapatchirag/revision/internal/tui/component"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

// logColumns describes the Log table layout: revision, author and message
// columns are all natural-width so the complete revision number and author name
// are shown in full; any overflow is revealed by the table's horizontal
// scrolling rather than being truncated. The date is intentionally omitted here
// (it is shown in the Main detail) to keep the table legible.
func logColumns() []component.Column {
	return []component.Column{
		{Title: "Rev", Width: 0},
		{Title: "Author", Width: 0},
		{Title: "Message", Width: 0},
	}
}

// renderLogRow is the domain adapter that turns an svn.LogEntry into the cells
// the reusable Table renders, keeping the Table component domain-agnostic.
//
// Two markers lead the Rev column, each in a cell of its own: a dot for a
// revision picked to be diffed, and an asterisk for the one the working copy
// sits at. Both cells are always drawn, blank when they do not apply, so picking
// a revision never shifts the column out from under the reader. While a page
// turn is in flight the rows still on screen belong to the page being left, so
// they are dimmed.
func renderLogRow(it svn.LogEntry, wcRevision string, picked, stale bool, th theme.Theme) []string {
	pick := " "
	if picked {
		pick = lipgloss.NewStyle().Foreground(th.Info).Bold(true).Render("●")
	}
	here := " "
	if wcRevision != "" && it.Revision == wcRevision {
		here = lipgloss.NewStyle().Foreground(th.Accent).Render("*")
	}
	cells := []string{pick + here + " r" + it.Revision, it.Author, firstLine(it.Message)}
	if stale {
		dim := lipgloss.NewStyle().Foreground(th.Muted)
		for i, c := range cells {
			cells[i] = dim.Render(c)
		}
	}
	return cells
}

// firstLine returns the first line of s, used to keep multi-line commit messages
// to a single table row.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// revFileNode is one visible row in the tree of files a range of history
// touched: a directory (Item == nil) or a file leaf carrying its section of the
// diff.
type revFileNode = pathRow[patchFile]

// revFilePath is the key a patched file is placed in the tree by. The diff is
// produced for the directory the display scope roots the views at, so the paths
// svn reports are already relative to it and the tree hangs off the same "/" as
// the working copy's.
func revFilePath(f patchFile) string { return f.Path }

// renderRevFileNode adapts a tree row for the reusable List, matching the
// Changes tree row for row: directories show a chevron and the segment name,
// leaves the same blank marker slot, a status code and the file name. The code
// is read from the diff itself, so it says what the range did to the file rather
// than what the working copy holds now.
func renderRevFileNode(th theme.Theme) func(revFileNode) string {
	return func(n revFileNode) string {
		indent := strings.Repeat("  ", n.Depth)
		if n.Item == nil {
			chevron := "▾"
			if n.Collapsed {
				chevron = "▸"
			}
			marker := lipgloss.NewStyle().Foreground(th.Muted).Render(chevron)
			name := lipgloss.NewStyle().Foreground(th.Info).Bold(true).Render(dirLabel(n))
			return indent + marker + " " + name
		}
		code := lipgloss.NewStyle().
			Foreground(stateColor(th, n.Item.State)).
			Bold(true).
			Render(n.Item.State.Code())
		return indent + "  " + code + " " + n.Name
	}
}
