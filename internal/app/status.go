package app

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/bapatchirag/revision/internal/svn"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

// renderStatusItem is the domain adapter that turns an svn.StatusItem into the
// display string the reusable List renders. This is the only place SVN state is
// mapped onto theme colors, keeping the List component domain-agnostic. Staged
// items (members of the staged changelist) are marked with a leading dot.
func renderStatusItem(th theme.Theme) func(svn.StatusItem) string {
	return func(it svn.StatusItem) string {
		code := lipgloss.NewStyle().
			Foreground(stateColor(th, it.State)).
			Bold(true).
			Render(it.State.Code())
		mark := " "
		if it.Changelist == stagedChangelist {
			mark = lipgloss.NewStyle().Foreground(th.Success).Bold(true).Render("●")
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
