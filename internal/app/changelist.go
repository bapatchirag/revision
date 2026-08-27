package app

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bapatchirag/revision/internal/svn"
	"github.com/bapatchirag/revision/internal/tui/component"
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
	containers := containerDirs(items)
	byName := map[string][]svn.StatusItem{}
	for _, it := range items {
		if _, structural := containers[it.Path]; structural {
			continue
		}
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

// containerDirs is the set of paths in items that are directory status entries
// with descendants in the same set (path/...). Such entries are structural: they
// duplicate a directory row already present in the tree and cannot be
// changelisted reliably, so changelist views ignore them.
//
// It answers for the whole set in one pass rather than per path. A path can only
// be a container of the ancestors written into the paths beneath it, so walking
// each path's own separators finds every container there is, where asking one
// path at a time rescans the set for each.
func containerDirs(items []svn.StatusItem) map[string]struct{} {
	have := make(map[string]struct{}, len(items))
	for _, it := range items {
		have[it.Path] = struct{}{}
	}
	containers := map[string]struct{}{}
	for _, it := range items {
		// Every ancestor, not only the parent: "a/b/c.go" makes a container of
		// "a" as much as of "a/b", and the set need not hold what lies between.
		for i := 0; i < len(it.Path); i++ {
			if it.Path[i] != '/' {
				continue
			}
			ancestor := it.Path[:i]
			if _, ok := have[ancestor]; ok {
				containers[ancestor] = struct{}{}
			}
		}
	}
	return containers
}

// renderChangelistGroup is the domain adapter that turns a changelistGroup into
// the row the reusable List renders: a pick cell, a colored marker (green
// staged, muted default, accent for a named list), the label, and the file
// count. picked reports how many of the group's files are held for shelving, so
// a partly-picked changelist reads differently from a whole one.
func renderChangelistGroup(th theme.Theme, picked func(changelistGroup) int) func(changelistGroup) string {
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
		return groupPickCell(th, picked(g), len(g.Items)) + marker + " " + label + count
	}
}

// groupPickCell marks a changelist by how much of it is held for shelving: a
// tick for all of it, a dot for some, a blank for none.
func groupPickCell(th theme.Theme, picked, total int) string {
	switch {
	case picked == 0 || total == 0:
		return " "
	case picked >= total:
		return lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Render("✓")
	default:
		return lipgloss.NewStyle().Foreground(th.Accent).Render("·")
	}
}

// changelistTarget is one file an assign-to-changelist action moves into a named
// changelist: its path plus whether it must be `svn add`ed first (an unversioned
// file being named directly, without staging it beforehand).
type changelistTarget struct {
	path string
	add  bool // svn add first (unversioned → versioned)
}

// drillChangelist expands the selected changelist into its file list as a
// drill-down sub-view, labeling the panel with the changelist and tracking which
// one is open so a status reload can keep it in sync. The files render as the
// same "/"-rooted tree as the Changes view, opening on the first file.
func (m *Model) drillChangelist() tea.Cmd {
	g, ok := m.changelists.Selected()
	if !ok {
		return nil
	}
	m.clItems = m.changelistItems(g.Name)
	m.rebuildClTree()
	if idx := firstFileIndex(m.clFiles.Items()); idx >= 0 {
		m.clFiles.SetIndex(idx)
	}
	m.drilledCL = g.Name
	cmd := m.filesViews.PushTitled(g.Label(), m.clFiles)
	m.updateBar()
	m.updateMain()
	return tea.Batch(cmd, m.diffLoadForSelection())
}

// submitChangelist closes the name prompt and assigns the selected file to the
// entered changelist, rejecting an empty or reserved name.
func (m *Model) submitChangelist(name string) tea.Cmd {
	name = strings.TrimSpace(name)
	switch name {
	case "":
		m.showToast("changelist name cannot be empty", component.LevelWarning)
		return nil
	case stagedChangelist:
		m.showToast("that changelist name is reserved", component.LevelWarning)
		return nil
	}
	m.naming = false
	m.nameEditor.Blur()
	targets := m.nameTargets
	return assignChangelistCmd(m.client, name, targets, m.applyOptimistic(changelistMutations(targets, name)))
}

// assignChangelist opens the changelist-name prompt for the files that will move
// into a named changelist. When any files are staged (in the anonymous staged
// bucket) the whole staged set is named as a unit; otherwise it falls back to
// the single selected file. In that fallback a lone selected file already in a
// named changelist is refused (one named changelist per file — unstage it
// first), as is a state that cannot be staged. The prompt lists the existing
// named changelists to pick from.
func (m *Model) assignChangelist() tea.Cmd {
	targets := m.stagedTargets()
	if len(targets) == 0 {
		it, ok := m.selectedFile()
		if !ok || m.isPending(it.Path) {
			return nil
		}
		if isNamedChangelist(it.Changelist) {
			m.showToast(it.Path+" already in "+displayCL(it.Changelist)+" — unstage first (space)", component.LevelWarning)
			return nil
		}
		if it.State != svn.StateUnversioned && !stageable(it.State) {
			m.showToast("can't add "+it.Path+" to a changelist ("+it.State.Code()+")", component.LevelWarning)
			return nil
		}
		targets = []changelistTarget{{path: it.Path, add: it.State == svn.StateUnversioned}}
	}
	m.naming = true
	m.nameTargets = targets
	m.nameEditor.Reset()
	m.nameEditor.SetOptions("Existing changelists:", m.namedChangelists())
	m.nameEditor.Focus()
	m.sizeNameEditor()
	return nil
}

// stagedTargets collects every file currently in the anonymous staged bucket as
// changelist targets, so naming a changelist moves the whole staged set as a
// unit. Staged files are already versioned, so in practice none need an
// `svn add` first. Files already waiting on svn are left out.
func (m *Model) stagedTargets() []changelistTarget {
	var targets []changelistTarget
	for _, it := range m.fileItems {
		if it.Changelist == stagedChangelist && !m.isPending(it.Path) {
			targets = append(targets, changelistTarget{path: it.Path, add: it.State == svn.StateUnversioned})
		}
	}
	return targets
}

// syncDrill refreshes a drilled-in changelist after a status reload: it
// repopulates the file list from the rebuilt groups, or collapses the drill when
// that changelist no longer exists (e.g. its last file was unstaged).
func (m *Model) syncDrill() {
	if !m.filesViewIsChangelists() || m.filesViews.Depth() == 0 {
		return
	}
	if items := m.changelistItems(m.drilledCL); len(items) > 0 {
		m.clItems = items
		m.rebuildClTree()
		return
	}
	m.filesViews.Pop()
	m.drilledCL = ""
}

// namedChangelists returns the existing user-named changelists (excluding the
// anonymous staged/unstaged buckets), for the assign prompt to offer as options.
func (m *Model) namedChangelists() []string {
	var names []string
	for _, g := range m.changelists.Items() {
		if isNamedChangelist(g.Name) {
			names = append(names, g.Name)
		}
	}
	return names
}

// isNamedChangelist reports whether cl is a real user-named changelist, i.e. not
// the empty default group or the anonymous staged bucket.
func isNamedChangelist(cl string) bool {
	return cl != "" && cl != stagedChangelist
}

// changelistItems returns every working-copy file in the named changelist from
// the full (unfiltered) status set, so a drill snapshot stays independent of any
// Files filter currently narrowing the view.
func (m *Model) changelistItems(name string) []svn.StatusItem {
	containers := containerDirs(m.fileItems)
	var items []svn.StatusItem
	for _, it := range m.fileItems {
		if _, structural := containers[it.Path]; structural {
			continue
		}
		if it.Changelist == name {
			items = append(items, it)
		}
	}
	return items
}

// rebuildChangelists repopulates the Changelists overview from the filtered
// status items.
func (m *Model) rebuildChangelists() {
	m.changelists.SetItems(groupChangelists(m.filteredStatusItems(m.fileItems)))
}
