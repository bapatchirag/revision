package app

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/svn"
	"github.com/bapatchirag/revision/internal/tui/component"
)

// selectedFile returns the file the current Files-panel view points at: the
// Changes tree selection (only when the cursor is on a file leaf, not a
// directory row), or the selection within a drilled-in changelist. At the
// Changelists overview (a group is selected), on a directory row, or in the
// Diffs and Rejects views (which list local files, not working-copy ones) there
// is no single file, so ok is false.
func (m *Model) selectedFile() (svn.StatusItem, bool) {
	if m.filesViewIsStore() {
		return svn.StatusItem{}, false
	}
	if m.filesViewIsChangelists() {
		if m.inChangelistDrill() {
			if n, ok := m.clFiles.Selected(); ok && n.Item != nil {
				return *n.Item, true
			}
		}
		return svn.StatusItem{}, false
	}
	if n, ok := m.files.Selected(); ok && n.Item != nil {
		return *n.Item, true
	}
	return svn.StatusItem{}, false
}

// rebuildFileTree re-flattens the current status items into the Changes tree,
// honoring the remembered per-directory collapse state. The cursor is put back
// on the row it was on by path, so a rebuild that reorders or drops rows keeps
// the user on the same file rather than the same row number.
func (m *Model) rebuildFileTree() {
	path := selectedNodePath(m.files)
	m.files.SetItems(m.fileTree(m.filteredStatusItems(m.fileItems), m.collapsedDirs))
	selectNodePath(m.files, path)
}

// selectedNodePath returns the path of the row a tree's cursor rests on, for
// restoring the selection across a rebuild.
func selectedNodePath[T any](l *component.List[pathRow[T]]) string {
	n, ok := l.Selected()
	if !ok {
		return ""
	}
	return n.Path
}

// selectNodePath moves a tree's cursor back onto path. A path the rebuild
// dropped leaves the cursor where the List clamped it — on the row that took its
// place — which is the nearest thing that survived.
func selectNodePath[T any](l *component.List[pathRow[T]], path string) {
	if path == "" {
		return
	}
	for i, n := range l.Items() {
		if n.Path == path {
			l.SetIndex(i)
			return
		}
	}
}

// focusFirstFile parks the Changes-tree cursor on the first file leaf the first
// time files appear, skipping the leading "/" root and directory rows so the
// panel opens on an actionable file (as it did before the tree). Later reloads
// leave the cursor where the user put it.
func (m *Model) focusFirstFile() {
	if m.filesInitialized {
		return
	}
	if idx := firstFileIndex(m.files.Items()); idx >= 0 {
		m.files.SetIndex(idx)
		m.filesInitialized = true
	}
}

// toggleCollapse expands or collapses the directory under the Changes-tree
// cursor and rebuilds the tree. It is inert on a file leaf or while the Files
// panel shows the Changelists view.
func (m *Model) toggleCollapse() tea.Cmd {
	if m.filesViewIsChangelists() {
		return nil
	}
	n, ok := m.files.Selected()
	if !ok || n.Item != nil {
		return nil
	}
	if m.collapsedDirs[n.Path] {
		delete(m.collapsedDirs, n.Path)
	} else {
		m.collapsedDirs[n.Path] = true
	}
	m.rebuildFileTree()
	m.updateMain()
	return nil
}

// rebuildClTree re-flattens the drilled-in changelist's items into its tree,
// honoring the drill's own per-directory collapse state and keeping the cursor
// on the same file across the rebuild.
func (m *Model) rebuildClTree() {
	path := selectedNodePath(m.clFiles)
	m.clFiles.SetItems(m.fileTree(m.filteredStatusItems(m.clItems), m.clCollapsedDirs))
	selectNodePath(m.clFiles, path)
}

// toggleClCollapse expands or collapses the directory under the drilled-in
// changelist tree's cursor and rebuilds it. It is inert on a file leaf.
func (m *Model) toggleClCollapse() tea.Cmd {
	n, ok := m.clFiles.Selected()
	if !ok || n.Item != nil {
		return nil
	}
	if m.clCollapsedDirs[n.Path] {
		delete(m.clCollapsedDirs, n.Path)
	} else {
		m.clCollapsedDirs[n.Path] = true
	}
	m.rebuildClTree()
	m.updateMain()
	return nil
}

// filesViewIsChangelists reports whether the Files panel's active view is the
// Changelists view.
func (m *Model) filesViewIsChangelists() bool {
	return m.filesViews.ActiveName() == "Changelists"
}

// filesViewIsDiffs reports whether the Files panel's active view is the Diffs
// browser, which lists saved patch files rather than working-copy changes.
func (m *Model) filesViewIsDiffs() bool {
	return m.filesViews.ActiveName() == savedDiffsViewName
}

// filesViewIsRejects reports whether the Files panel's active view is the
// Rejects browser, which lists the .rej files a patch left behind rather than
// working-copy changes.
func (m *Model) filesViewIsRejects() bool {
	return m.filesViews.ActiveName() == rejectsViewName
}

// filesViewIsStore reports whether the active Files-panel view browses files on
// local disk — saved patches or rejects — rather than working-copy status. Those
// two views share their shape: a flat list, a file's contents in Main, and no
// selection svn knows anything about.
func (m *Model) filesViewIsStore() bool {
	return m.filesViewIsDiffs() || m.filesViewIsRejects()
}

// inChangelistDrill reports whether the Changelists view is drilled into a
// changelist's file list.
func (m *Model) inChangelistDrill() bool {
	return m.filesViewIsChangelists() && m.filesViews.Depth() > 0
}

// filesFooter returns the position/count indicator inlaid into the Files panel's
// bottom border: the 1-based position within the active view, the number of
// entries shown, and — when a filter or the hide-untracked toggle hides some —
// the full unfiltered count in brackets. It counts file leaves in the Changes
// tree and a drilled-in changelist (ignoring the synthetic root and directory
// rows), changelists in the Changelists overview, saved patch files in the Diffs
// view and rejects in the Rejects view, following whichever view is active.
func (m *Model) filesFooter() string {
	hiding := m.hideUntracked || m.filters[panelFiles] != ""
	switch {
	case m.filesViewIsDiffs():
		shown := len(m.savedDiffs.Items())
		return countLabel(m.savedDiffs.Index()+1, shown, len(m.savedDiffItems))
	case m.filesViewIsRejects():
		index, shown := fileLeafStats(m.rejects.Items(), m.rejects.Index())
		full := shown
		if m.filters[panelFiles] != "" {
			full = len(m.rejectItems)
		}
		return countLabel(index, shown, full)
	case m.inChangelistDrill():
		index, shown := fileLeafStats(m.clFiles.Items(), m.clFiles.Index())
		full := shown
		if hiding {
			full = leafCount(m.fileTree(m.clItems, m.clCollapsedDirs))
		}
		return countLabel(index, shown, full)
	case m.filesViewIsChangelists():
		shown := len(m.changelists.Items())
		full := shown
		if hiding {
			full = len(groupChangelists(m.fileItems))
		}
		return countLabel(m.changelists.Index()+1, shown, full)
	default:
		index, shown := fileLeafStats(m.files.Items(), m.files.Index())
		full := shown
		if hiding {
			full = leafCount(m.fileTree(m.fileItems, m.collapsedDirs))
		}
		return countLabel(index, shown, full)
	}
}

// countLabel formats a "position of shown" indicator for a panel's bottom
// border, appending the full count in brackets when hidden entries push it past
// the shown count. index is 1-based (0 when the cursor sits above the first
// entry, such as on the tree root); an empty, fully-unhidden view yields no
// label.
func countLabel(index, shown, full int) string {
	switch {
	case shown == 0 && full == 0:
		return ""
	case shown == 0:
		return fmt.Sprintf("0 of 0 (%d)", full)
	case full > shown:
		return fmt.Sprintf("%d of %d (%d)", index, shown, full)
	default:
		return fmt.Sprintf("%d of %d", index, shown)
	}
}

// rebuildFilesViews re-flattens every Files-panel view — the Changes tree, the
// Changelists overview, a drilled-in changelist, the saved-diffs browser and the
// rejects browser — from the current status items. It is the shared refresh used
// whenever what those views should show changes without a status reload (a
// filter edit or the untracked toggle).
func (m *Model) rebuildFilesViews() {
	m.rebuildFileTree()
	m.rebuildChangelists()
	m.rebuildSavedDiffs()
	m.rebuildRejects()
	if m.inChangelistDrill() {
		m.rebuildClTree()
	}
}

// selectedTreeNode returns the tree row under the active Files-panel cursor —
// from the Changes tree, or a drilled-in changelist tree — together with the
// item set that tree was built from (used to stage a directory's files). It
// reports ok=false at the Changelists overview, where the selection is a
// changelist group rather than a tree row, and in the Diffs and Rejects views,
// where it is a file on local disk.
func (m *Model) selectedTreeNode() (fileNode, []svn.StatusItem, bool) {
	if m.filesViewIsStore() {
		return fileNode{}, nil, false
	}
	if m.filesViewIsChangelists() {
		if m.inChangelistDrill() {
			n, ok := m.clFiles.Selected()
			return n, m.clItems, ok
		}
		return fileNode{}, nil, false
	}
	n, ok := m.files.Selected()
	return n, m.fileItems, ok
}

// selectedDirectory returns the highlighted Files-panel row when it is a
// directory row (Item == nil), along with the status items backing its view, so
// directory-level actions can fan out over filesUnder. ok is false on a file leaf
// or the Changelists overview.
func (m *Model) selectedDirectory() (fileNode, []svn.StatusItem, bool) {
	if n, items, ok := m.selectedTreeNode(); ok && n.Item == nil {
		return n, items, true
	}
	return fileNode{}, nil, false
}
