package app

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/bapatchirag/revision/internal/svn"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

// pickCell is the row's leading cell for the shelve picks: a tick when the row
// is held, a blank of the same width when it is not. It is always drawn, so
// picking a file cannot shift every row sideways, and it is kept apart from the
// changelist marker beside it so a picked file still shows which bucket it is in.
func pickCell(th theme.Theme, picked bool) string {
	if !picked {
		return " "
	}
	return lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Render("✓")
}

// statusRow renders a single file row — the pick cell, the changelist marker, the
// status code, and a label. It is the only place SVN state is mapped onto theme
// colors, keeping the reusable List domain-agnostic. Files in the anonymous
// staged bucket get a green dot; files in a named changelist get an accent dot
// (their changelist is shown in the Changelists view and the Main detail). The
// label is the file's basename in the tree; passing the full path yields a flat
// row.
func statusRow(th theme.Theme, it svn.StatusItem, label string, picked bool) string {
	code := lipgloss.NewStyle().
		Foreground(stateColor(th, it.State)).
		Bold(true).
		Render(it.State.Code())
	mark := " "
	switch it.Changelist {
	case "":
		// no marker: not staged, not assigned to a changelist.
	case stagedChangelist:
		mark = lipgloss.NewStyle().Foreground(th.Success).Bold(true).Render("●")
	default:
		mark = lipgloss.NewStyle().Foreground(th.Info).Bold(true).Render("●")
	}
	return pickCell(th, picked) + mark + " " + code + " " + label
}

// pendingStatusRow renders a file row whose action svn has been asked for but
// has not confirmed: the same shape as statusRow, dimmed throughout and tailed
// by an ellipsis, so it reads as in flight rather than done.
func pendingStatusRow(th theme.Theme, it svn.StatusItem, label string) string {
	mark := " "
	if it.Changelist != "" {
		mark = "●"
	}
	return " " + lipgloss.NewStyle().
		Foreground(th.Muted).
		Render(mark+" "+it.State.Code()+" "+label+" "+pendingMarker)
}

// stateColor maps an SVN working-copy state onto a theme color.
func stateColor(th theme.Theme, s svn.FileState) lipgloss.Color {
	switch s {
	case svn.StateModified:
		return th.Warning
	case svn.StateAdded, svn.StateMerged:
		return th.Success
	case svn.StateDeleted, svn.StateMissing:
		return th.Error
	case svn.StateConflicted:
		return lipgloss.Color("201") // magenta
	case svn.StateUnversioned:
		return th.Muted
	default:
		return th.Text
	}
}
