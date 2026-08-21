package app

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/svn"
	"github.com/bapatchirag/revision/internal/tui/component"
)

// deleteAction describes how a delete keypress should remove one file.
type deleteAction struct {
	path        string
	unversioned bool // remove from disk (untracked) vs. svn delete (versioned)
}

// requestRevert asks to discard local changes to the current selection, opening a
// confirmation modal. On a directory row it reverts every dirty file beneath it;
// on a file leaf a clean/unversioned selection has nothing to revert. A row
// already waiting on svn is left alone.
func (m *Model) requestRevert() tea.Cmd {
	if n, items, ok := m.selectedDirectory(); ok {
		return m.requestRevertDirectory(n, m.withoutPending(items))
	}
	it, ok := m.selectedFile()
	if !ok || m.isPending(it.Path) {
		return nil
	}
	if !it.State.IsDirty() {
		m.showToast("nothing to revert in "+it.Path, component.LevelWarning)
		return nil
	}
	token := m.nextPendingToken()
	m.confirmAction(revertCmd(m.client, it.Path, token), holdPending(pendingRevert, token, it.Path))
	m.openConfirm("Revert changes?", "Discard local changes to "+it.Path+"? This cannot be undone.")
	return nil
}

// requestRevertDirectory asks to discard local changes to every dirty file
// beneath the selected directory row. A directory with nothing revertable warns
// instead of opening the modal.
func (m *Model) requestRevertDirectory(n fileNode, items []svn.StatusItem) tea.Cmd {
	paths := directoryRevertPaths(n, items)
	if len(paths) == 0 {
		m.showToast("nothing to revert under "+dirLabel(n), component.LevelWarning)
		return nil
	}
	token := m.nextPendingToken()
	m.confirmAction(revertManyCmd(m.client, paths, token), holdPending(pendingRevert, token, paths...))
	m.openConfirm("Revert changes?", fmt.Sprintf(
		"Discard local changes to %d files under %s? This cannot be undone.", len(paths), dirLabel(n)))
	return nil
}

// directoryRevertPaths collects the revertable file paths beneath a directory
// row: every versioned pending change, matching the single-file revert guard.
// The row's own status item counts too, since a directory svn tracks as a change
// of its own (a scheduled add, say) renders as the directory row and has no leaf
// to select; leaving it out would revert its children and strand it.
func directoryRevertPaths(n fileNode, items []svn.StatusItem) []string {
	var paths []string
	for _, it := range items {
		if it.Path == n.Path && it.State.IsDirty() {
			paths = append(paths, it.Path)
		}
	}
	for _, it := range filesUnder(n, items) {
		if it.State.IsDirty() {
			paths = append(paths, it.Path)
		}
	}
	return paths
}

// requestDelete asks to remove the current selection, opening a confirmation
// modal. In the Diffs view it removes the highlighted patch file from disk and
// in the Rejects view the highlighted reject; on a directory row it removes
// every deletable file beneath it; on a file leaf a versioned file is scheduled
// for deletion, an unversioned one is removed from disk, and ignored files are
// left alone. A row already waiting on svn is left alone too.
func (m *Model) requestDelete() tea.Cmd {
	if m.filesViewIsDiffs() {
		return m.requestDeleteSavedDiff()
	}
	if m.filesViewIsRejects() {
		return m.requestDeleteReject()
	}
	if n, items, ok := m.selectedDirectory(); ok {
		return m.requestDeleteDirectory(n, m.withoutPending(items))
	}
	it, ok := m.selectedFile()
	if !ok || m.isPending(it.Path) {
		return nil
	}
	if it.State == svn.StateIgnored {
		m.showToast("can't delete ignored "+it.Path, component.LevelWarning)
		return nil
	}
	act := deleteAction{path: it.Path, unversioned: it.State == svn.StateUnversioned}
	message := it.Path + " will be scheduled for deletion (removed on the next commit)."
	if act.unversioned {
		message = "Permanently delete untracked " + it.Path + " from disk? This cannot be undone."
	}
	token := m.nextPendingToken()
	m.confirmAction(deleteCmd(m.client, act, token), holdPending(pendingDelete, token, act.path))
	m.openConfirm("Delete file?", message)
	return nil
}

// requestDeleteDirectory asks to remove every deletable file beneath the selected
// directory row: versioned files are scheduled for deletion and unversioned ones
// are removed from disk. Ignored files are skipped, so a directory holding only
// ignored files warns instead of opening the modal.
func (m *Model) requestDeleteDirectory(n fileNode, items []svn.StatusItem) tea.Cmd {
	acts := directoryDeleteActions(n, items)
	if len(acts) == 0 {
		m.showToast("nothing to delete under "+dirLabel(n), component.LevelWarning)
		return nil
	}
	paths := make([]string, 0, len(acts))
	for _, act := range acts {
		paths = append(paths, act.path)
	}
	token := m.nextPendingToken()
	m.confirmAction(deleteManyCmd(m.client, acts, token), holdPending(pendingDelete, token, paths...))
	m.openConfirm("Delete files?", deleteDirectoryMessage(n, acts))
	return nil
}

// directoryDeleteActions builds the delete actions for the files beneath a
// directory row: each versioned file is scheduled for deletion (svn delete) and
// each unversioned one is removed from disk, skipping ignored files the same way
// the single-file delete does. It names every file, including any a directory in
// the set already covers, so the confirmation counts what will go rather than
// how few commands it takes to shift them.
func directoryDeleteActions(n fileNode, items []svn.StatusItem) []deleteAction {
	var acts []deleteAction
	for _, it := range filesUnder(n, items) {
		if it.State == svn.StateIgnored {
			continue
		}
		acts = append(acts, deleteAction{path: it.Path, unversioned: it.State == svn.StateUnversioned})
	}
	return acts
}

// deleteDirectoryMessage composes the confirmation body for a directory delete,
// separating files merely scheduled for deletion from untracked files that would
// be permanently removed from disk.
func deleteDirectoryMessage(n fileNode, acts []deleteAction) string {
	var scheduled, disk int
	for _, act := range acts {
		if act.unversioned {
			disk++
		} else {
			scheduled++
		}
	}
	var parts []string
	if scheduled > 0 {
		parts = append(parts, fmt.Sprintf("%d files under %s will be scheduled for deletion (removed on the next commit)", scheduled, dirLabel(n)))
	}
	if disk > 0 {
		parts = append(parts, fmt.Sprintf("%d untracked files will be permanently removed from disk — this cannot be undone", disk))
	}
	return strings.Join(parts, "; ") + "."
}

// requestUpdate brings the working copy up to date with the repository's latest
// revision. It confirms first — like the update-to-revision flow — and adds a
// second confirmation when the working copy already holds conflicts svn skips.
func (m *Model) requestUpdate() tea.Cmd {
	m.confirmAction(updateCmd(m.client), nil)
	m.updateConflictPrompt = conflictUpdatePrompt(m.conflictedPaths(), "the latest revision")
	m.updateProgress = updateProgressText(m.wcRevision, "the latest revision")
	m.openConfirm("Update working copy?", "Update the working copy to the latest revision? Uncommitted changes are kept and merged.")
	return nil
}

// requestUpdateToRevision updates the working copy to the revision selected in
// the Log panel. Because this can move the working copy backwards in history, it
// asks for confirmation first; with no revision selected it warns instead.
func (m *Model) requestUpdateToRevision() tea.Cmd {
	entry, ok := m.log.Selected()
	if !ok {
		m.showToast("no revision selected", component.LevelWarning)
		return nil
	}
	m.confirmAction(updateToRevisionCmd(m.client, entry.Revision), nil)
	m.updateConflictPrompt = conflictUpdatePrompt(m.conflictedPaths(), "r"+entry.Revision)
	m.updateProgress = updateProgressText(m.wcRevision, "r"+entry.Revision)
	m.openConfirm("Update to revision?", "Update the working copy to r"+entry.Revision+"? Uncommitted changes are kept and merged.")
	return nil
}

// conflictedPaths returns the working-copy paths currently in a conflicted
// state, from the last-loaded status. svn update skips these — they stay in
// conflict while everything else moves — so they drive the extra confirmation.
func (m *Model) conflictedPaths() []string {
	var paths []string
	for _, it := range m.fileItems {
		if it.State == svn.StateConflicted {
			paths = append(paths, it.Path)
		}
	}
	return paths
}

// conflictUpdatePrompt builds the additional confirmation shown before updating
// when the working copy already holds conflicts, spelling out that svn leaves
// those files untouched and updates the rest to target (e.g. "r42" or "the
// latest revision"). It returns "" when nothing is in conflict, which suppresses
// the extra step.
func conflictUpdatePrompt(conflicts []string, target string) string {
	n := len(conflicts)
	if n == 0 {
		return ""
	}
	subject := "1 file is"
	if n > 1 {
		subject = fmt.Sprintf("%d files are", n)
	}
	return fmt.Sprintf("%s already in conflict and will be left untouched; the rest of the working copy will still update to %s. Continue?", subject, target)
}

// updateProgressText builds the message shown in the progress modal while an svn
// update runs, e.g. "Updating from r38 to r50…". target is a ready-made label:
// "r50" for a specific revision, or "the latest revision" for a HEAD update whose
// exact number svn only reports on completion.
func updateProgressText(from, target string) string {
	fromLabel := "the current revision"
	if from != "" {
		fromLabel = "r" + from
	}
	return fmt.Sprintf("Updating from %s to %s…", fromLabel, target)
}
