package app

import (
	"fmt"
	"sort"

	"github.com/charmbracelet/lipgloss"

	"github.com/bapatchirag/revision/internal/svn"
	"github.com/bapatchirag/revision/internal/tui/theme"
)

// changelistGroup is a set of pending files that share one SVN changelist. The
// Name is the real changelist: "" for files in no changelist (the default,
// committable-by-default group), stagedChangelist for the anonymous staged
// bucket, or a user-named changelist. This is the domain model behind the
// Files panel's Changelists view.
type changelistGroup struct {
	Name  string
	Items []svn.StatusItem
}

// Committable reports whether the group maps to a real SVN changelist that can
// be committed as a unit. The default/unnamed group (Name == "") is committable
// only implicitly by SVN, never as an addressable changelist, so it returns
// false here.
func (g changelistGroup) Committable() bool { return g.Name != "" }

// Label is the human-facing name shown in the Changelists view.
func (g changelistGroup) Label() string { return displayCL(g.Name) }

// displayCL maps a changelist name to its display label: the reserved staged
// bucket shows as "(staged)", the empty/default group as "(unstaged)", and a
// user changelist as its own name.
func displayCL(name string) string {
	switch name {
	case stagedChangelist:
		return "(staged)"
	case "":
		return "(unstaged)"
	default:
		return name
	}
}

// groupChangelists buckets status items by their changelist. Named changelists
// come first (alphabetical), then the anonymous staged bucket, then the default
// unstaged group; empty buckets are omitted. This ordering keeps the actionable,
// addressable changelists at the top of the view.
func groupChangelists(items []svn.StatusItem) []changelistGroup {
	byName := map[string][]svn.StatusItem{}
	for _, it := range items {
		byName[it.Changelist] = append(byName[it.Changelist], it)
	}

	names := make([]string, 0, len(byName))
	for name := range byName {
		if name != "" && name != stagedChangelist {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	groups := make([]changelistGroup, 0, len(byName))
	for _, name := range names {
		groups = append(groups, changelistGroup{Name: name, Items: byName[name]})
	}
	if staged, ok := byName[stagedChangelist]; ok {
		groups = append(groups, changelistGroup{Name: stagedChangelist, Items: staged})
	}
	if loose, ok := byName[""]; ok {
		groups = append(groups, changelistGroup{Name: "", Items: loose})
	}
	return groups
}

// renderChangelistGroup is the domain adapter that turns a changelistGroup into
// the row the reusable List renders: a colored marker (green staged, muted
// default, accent for a named list), the label, and the file count.
func renderChangelistGroup(th theme.Theme) func(changelistGroup) string {
	return func(g changelistGroup) string {
		var color lipgloss.Color
		switch g.Name {
		case stagedChangelist:
			color = th.Success
		case "":
			color = th.Muted
		default:
			color = th.Info
		}
		marker := lipgloss.NewStyle().Foreground(color).Bold(true).Render("◆")
		label := lipgloss.NewStyle().Foreground(th.Text).Render(g.Label())
		count := lipgloss.NewStyle().Foreground(th.Muted).Render(fmt.Sprintf(" (%d)", len(g.Items)))
		return marker + " " + label + count
	}
}
