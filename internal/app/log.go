package app

import (
	"strings"

	"github.com/bapatchirag/revision/internal/svn"
	"github.com/bapatchirag/revision/internal/tui/component"
)

// logLimit caps how many recent revisions the Log panel loads.
const logLimit = 50

// logColumns describes the Log table layout: compact fixed revision/author
// columns and a flexible message column that consumes the remaining width. The
// date is intentionally omitted here (it is shown in the Main detail) to keep
// the table legible in the narrow left column.
func logColumns() []component.Column {
	return []component.Column{
		{Title: "Rev", Width: 6},
		{Title: "Author", Width: 10},
		{Title: "Message", Width: 0},
	}
}

// renderLogRow is the domain adapter that turns an svn.LogEntry into the cells
// the reusable Table renders, keeping the Table component domain-agnostic.
func renderLogRow(it svn.LogEntry) []string {
	return []string{"r" + it.Revision, it.Author, firstLine(it.Message)}
}

// firstLine returns the first line of s, used to keep multi-line commit messages
// to a single table row.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
