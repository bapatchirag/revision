package app

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/bapatchirag/revision/internal/svn"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

// renderStatusItem is the domain adapter that turns an svn.StatusItem into the
// display string the reusable List renders. This is the only place SVN state is
// mapped onto theme colors, keeping the List component domain-agnostic. Files in
// the anonymous staged bucket get a green dot; files in a named changelist get
// an accent dot (their changelist is shown in the Changelists view and the Main
// detail).
func renderStatusItem(th theme.Theme) func(svn.StatusItem) string {
	return func(it svn.StatusItem) string {
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
		return mark + " " + code + " " + it.Path
	}
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
