package app

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/tui/component"
)

// mutationEvent handles the replies to the actions that change something:
// staging, committing, reverting, deleting, updating the working copy, applying
// or resolving a patch, writing a diff out and removing one, plus the exit of an
// external editor. Each reports its outcome and reloads what it invalidated. It
// reports whether it owned the message.
func (m *Model) mutationEvent(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case stagedMsg:
		if msg.err != nil {
			m.showToast(failureText("stage", msg.err), component.LevelError)
			if msg.token == 0 {
				return nil, true
			}
			// Put the change back the way it was, then ask svn for the truth: a fan-out
			// over several files stops at the first failure, so some may have landed.
			m.settleOptimistic(msg.token, msg.err)
			return m.reloadStatus(), true
		}
		m.settleOptimistic(msg.token, nil)
		if msg.changelist != "" {
			m.showToast("added "+msg.path+" to "+msg.changelist, component.LevelSuccess)
		}
		// Reload status so the changelist grouping (and staged marker) refresh.
		return m.reloadStatus(), true

	case committedMsg:
		m.clearPending(msg.token)
		if msg.err != nil {
			m.loading = false
			m.showToast(failureText("commit", msg.err), component.LevelError)
			m.refreshChrome()
			return nil, true
		}
		if msg.revision != "" {
			m.wcRevision = msg.revision
			m.showToast("committed r"+msg.revision, component.LevelSuccess)
		} else {
			m.showToast("commit complete", component.LevelSuccess)
		}
		m.clearDiff()
		m.refreshChrome()
		// A commit adds a revision at the head of history: show it.
		m.log.GoTop()
		return tea.Batch(m.reloadStatus(), m.resetLogPaging()), true

	case revertedMsg:
		m.clearPending(msg.token)
		if msg.err != nil {
			m.showToast(failureText("revert", msg.err), component.LevelError)
			return nil, true
		}
		m.showToast("reverted "+msg.path, component.LevelSuccess)
		m.clearDiff()
		return m.reloadStatus(), true

	case deletedMsg:
		m.clearPending(msg.token)
		if msg.err != nil {
			m.showToast(failureText("delete", msg.err), component.LevelError)
			return nil, true
		}
		m.showToast("deleted "+msg.path, component.LevelSuccess)
		m.clearDiff()
		return m.reloadStatus(), true

	case updatedMsg:
		m.updatingWC = false
		m.updateProgress = ""
		if msg.err != nil {
			m.loading = false
			m.showToast(failureText("update", msg.err), component.LevelError)
			m.refreshChrome()
			return nil, true
		}
		if msg.revision != "" {
			m.wcRevision = msg.revision
			m.showToast("updated to r"+msg.revision, component.LevelSuccess)
		} else {
			m.showToast("update complete", component.LevelSuccess)
		}
		m.clearDiff()
		m.updateStatus()
		m.updateBar()
		// A revision picked in the Log was on the page on screen, so stay there; a
		// plain update lands on HEAD, which is on the first page.
		log := m.reloadLogPage()
		if !msg.toRevision {
			m.log.GoTop()
			log = m.resetLogPaging()
		}
		return tea.Batch(m.reloadStatus(), log), true

	case diffSavedMsg:
		if msg.err != nil {
			m.showToast(failureText("save diff", msg.err), component.LevelError)
			return nil, true
		}
		m.showToast("diff saved to "+msg.path, component.LevelSuccess)
		// The new file belongs in the Diffs view, so re-scan the store.
		return m.reloadSavedDiffs(), true

	case savedDiffDeletedMsg:
		if msg.err != nil {
			m.showToast(failureText("delete "+msg.name, msg.err), component.LevelError)
			return nil, true
		}
		m.showToast("deleted "+msg.name, component.LevelSuccess)
		if m.savedPath == msg.path {
			// Main is showing the file that just went away; drop it so the re-scan
			// reads whatever the list settles on.
			m.savedPath, m.savedText, m.savedErr = "", "", false
		}
		return m.reloadSavedDiffs(), true

	case shelvedMsg:
		if msg.err != nil {
			if msg.entry.ID == "" {
				m.showToast(failureText("shelve", msg.err), component.LevelError)
				return nil, true
			}
			// The changes are safely on the shelf; it is the working copy that did
			// not come clean, so the reload below shows what is still in it.
			m.showToast(failureText("clear the working copy", msg.err), component.LevelError)
		} else {
			m.showToast(shelveToast(msg.entry, msg.left))
		}
		// What was picked has been taken, so the picks go with it either way: the
		// files they named are no longer in the working copy to act on.
		m.shelfPicks = nil
		m.clearDiff()
		return tea.Batch(m.reloadShelves(), m.reloadStatus()), true

	case shelveAllMsg:
		items := shelvableItems(m.fileItems)
		if len(items) == 0 {
			m.showToast("nothing to shelve", component.LevelWarning)
			return nil, true
		}
		m.openShelveName(shelveScope{label: "all changes", items: items})
		return nil, true

	case shelfRestoredMsg:
		if msg.err != nil {
			m.showToast(failureText("restore "+msg.name, msg.err), component.LevelError)
			// Part of it may still have landed before the failure, so the working copy
			// is re-read either way.
			m.clearDiff()
			return tea.Batch(m.reloadStatus(), m.reloadShelves(), m.reloadRejectsIfShown()), true
		}
		m.showToast(shelfRestoreToast(msg))
		m.clearDiff()
		if msg.dropped {
			// Main was showing the entry that has just gone; drop it so the re-scan
			// reads whatever the list settles on.
			m.shelfID, m.shelfText, m.shelfReadErr = "", "", false
		}
		return tea.Batch(m.reloadStatus(), m.reloadShelves(), m.reloadRejectsIfShown()), true

	case shelfDroppedMsg:
		if msg.err != nil {
			m.showToast(failureText("drop "+msg.name, msg.err), component.LevelError)
			return nil, true
		}
		m.showToast("dropped "+msg.name, component.LevelSuccess)
		if m.shelfID == msg.id {
			m.shelfID, m.shelfText, m.shelfReadErr = "", "", false
		}
		return m.reloadShelves(), true

	case shelfRenamedMsg:
		if msg.err != nil {
			m.showToast(failureText("rename the shelf", msg.err), component.LevelError)
			return nil, true
		}
		m.showToast("renamed to "+msg.name, component.LevelSuccess)
		return m.reloadShelves(), true

	case rejectDeletedMsg:
		if msg.err != nil {
			m.showToast(failureText("delete "+msg.name, msg.err), component.LevelError)
			return nil, true
		}
		m.showToast("deleted "+msg.name, component.LevelSuccess)
		if m.rejectPath == msg.path {
			// Main is showing the file that just went away; drop it so the re-scan
			// reads whatever the list settles on.
			m.rejectPath, m.rejectText, m.rejectErr = "", "", false
		}
		return m.reloadRejects(), true

	case patchAppliedMsg:
		if msg.err != nil {
			m.showToast(failureText("apply "+msg.name, msg.err), component.LevelError)
			return nil, true
		}
		m.showToast(patchToast(msg.name, msg.res))
		// The working copy now holds changes it did not a moment ago, so the diff on
		// screen and the status behind it are both out of date — and any hunk that
		// did not fit has just been written out as a reject.
		m.clearDiff()
		return tea.Batch(m.reloadStatus(), m.reloadRejectsIfShown()), true

	case mergeWrittenMsg:
		if msg.err != nil {
			m.showToast(failureText("resolve "+msg.rel, msg.err), component.LevelError)
			return nil, true
		}
		m.showToast(mergeDoneText(msg), component.LevelSuccess)
		// The file on disk is not the one the diff on screen was taken from, and
		// its status has just changed out from under the Files panel.
		m.clearDiff()
		if msg.kind != mergeReject {
			return m.reloadStatus(), true
		}
		if m.rejectPath == msg.aux {
			// Main is showing the reject that has just been cleared; drop it so the
			// re-scan reads whatever the list settles on.
			m.rejectPath, m.rejectText, m.rejectErr = "", "", false
		}
		return tea.Batch(m.reloadStatus(), m.reloadRejectsIfShown()), true

	case editedMsg:
		if msg.err != nil {
			m.showToast(failureText("open "+msg.name, msg.err), component.LevelError)
			return nil, true
		}
		if msg.detached {
			return nil, true
		}
		// A terminal editor has exited, so the file may have changed: re-read the
		// working copy (which reloads the diff on screen) and the local file stores.
		// Suspending for it turned mouse reporting off, so it is asked for again.
		var mouse tea.Cmd
		if m.cfg.AllowMouse {
			mouse = mouseReporting(true)
		}
		return tea.Batch(mouse, m.reloadStatus(), m.reloadSavedDiffsIfShown(), m.reloadRejectsIfShown()), true
	}
	return nil, false
}
