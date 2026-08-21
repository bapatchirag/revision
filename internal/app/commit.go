package app

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/svn"
	"github.com/bapatchirag/revision/internal/tui/component"
)

// openCommit opens the commit-message editor for the current commit target: the
// selected changelist when in the Changelists view, otherwise the anonymous
// staged bucket. It refuses an empty target, and one whose commit is still
// running.
func (m *Model) openCommit() tea.Cmd {
	if m.commitPending() {
		m.showToast("a commit is already running", component.LevelWarning)
		return nil
	}
	target, label, ok := m.commitTarget()
	if !ok {
		return nil
	}
	if m.countInChangelist(target) == 0 {
		m.showToast("nothing staged in "+label+" — press space to stage files", component.LevelWarning)
		return nil
	}
	if blockers := m.uncommittable(target); len(blockers) > 0 {
		m.showToast(uncommittableText(blockers), component.LevelWarning)
		return nil
	}
	m.commitCL = target
	m.editing = true
	m.editor.Reset()
	m.editor.Focus()
	m.sizeEditor()
	return nil
}

// uncommittable returns the staged files svn will not commit, from the
// last-loaded status. A commit is one transaction: svn refuses the whole thing
// over a single conflicted or missing member, so the message the user has just
// written would be typed for nothing. Naming them before the editor opens is the
// only place this can be caught — there is no partial commit to fall back on.
func (m *Model) uncommittable(changelist string) []svn.StatusItem {
	var out []svn.StatusItem
	for _, it := range m.fileItems {
		if it.Changelist != changelist {
			continue
		}
		if it.State == svn.StateConflicted || it.State == svn.StateMissing {
			out = append(out, it)
		}
	}
	return out
}

// uncommittableText says which staged files svn will refuse a commit over, and
// what to do about each: a conflict has to be resolved, a missing file scheduled
// for deletion or brought back.
func uncommittableText(blockers []svn.StatusItem) string {
	if len(blockers) == 1 {
		return "can't commit: " + blockers[0].Path + " is " + uncommittableReason(blockers[0].State)
	}
	names := make([]string, 0, len(blockers))
	for _, it := range blockers {
		names = append(names, it.Path)
	}
	return "can't commit: svn refuses the whole changelist over " +
		fileCount(len(blockers)) + " — " + strings.Join(names, ", ")
}

// uncommittableReason names what stands between a file and a commit.
func uncommittableReason(s svn.FileState) string {
	if s == svn.StateMissing {
		return "missing — delete it or restore it first"
	}
	return "still in conflict — resolve it first"
}

// commitTarget resolves which changelist a commit would target. In the
// Changelists view it is the selected (or drilled-in) changelist, refusing the
// default/unstaged group which is not an addressable changelist; everywhere else
// it is the anonymous staged bucket.
func (m *Model) commitTarget() (cl, label string, ok bool) {
	if m.focus.Index() == panelFiles && m.filesViewIsChangelists() {
		if m.inChangelistDrill() {
			if m.drilledCL == "" {
				m.showToast("the (unstaged) group isn't a changelist — stage or name files first", component.LevelWarning)
				return "", "", false
			}
			return m.drilledCL, displayCL(m.drilledCL), true
		}
		g, sel := m.changelists.Selected()
		if !sel {
			return "", "", false
		}
		if !g.Committable() {
			m.showToast("the "+g.Label()+" group isn't a changelist — stage or name files first", component.LevelWarning)
			return "", "", false
		}
		return g.Name, g.Label(), true
	}
	return stagedChangelist, displayCL(stagedChangelist), true
}

// pathsInChangelist returns the pending files belonging to the named changelist.
func (m *Model) pathsInChangelist(name string) []string {
	var paths []string
	for _, it := range m.fileItems {
		if it.Changelist == name {
			paths = append(paths, it.Path)
		}
	}
	return paths
}

// countInChangelist returns how many pending files belong to the named
// changelist.
func (m *Model) countInChangelist(name string) int {
	return len(m.pathsInChangelist(name))
}

// submitCommit closes the editor and commits the target changelist with the
// entered message, rejecting an empty message. The files going in are marked as
// in flight until svn answers.
func (m *Model) submitCommit(message string) tea.Cmd {
	if strings.TrimSpace(message) == "" {
		m.showToast("commit message cannot be empty", component.LevelWarning)
		return nil
	}
	m.editing = false
	m.editor.Blur()
	m.loading = true
	token := m.nextPendingToken()
	m.markPending(pendingCommit, token, m.pathsInChangelist(m.commitCL))
	m.refreshChrome()
	return commitCmd(m.client, message, m.commitCL, token)
}
