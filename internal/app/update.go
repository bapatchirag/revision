package app

import (
	"errors"
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/bapatchirag/revision/internal/sshagent"
	"github.com/bapatchirag/revision/internal/tui/component"
	uimsg "github.com/bapatchirag/revision/internal/tui/msg"
)

// Update handles messages, global keys, and forwards the rest to the focused
// panel.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Refresh the command log from any invocations that finished since the last
	// message; every svn command completes by delivering a message here.
	m.syncCommandLog()
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		if m.editing {
			m.sizeEditor()
		}
		if m.naming {
			m.sizeNameEditor()
		}
		if m.savingDiff {
			m.sizeDiffEditor()
		}
		if m.retargeting {
			m.sizeSourcePath()
		}
		if m.splitting {
			m.sizeSplitDiff()
		}
		if m.merging {
			m.sizeMerge()
		}
		if m.confirming {
			m.sizeModal()
		}
		if m.helping {
			m.sizeMenu()
		}
		if m.updating {
			m.sizeUpdateMenu()
		}
		if m.configuring {
			m.sizeForm()
		}
		if m.unlocking {
			m.sizeUnlock()
		}
		if m.updatingWC {
			m.sizeProgress()
		}
		return m, nil

	case statusLoadedMsg:
		if m.statusGen.stale(msg.gen) {
			return m, nil
		}
		m.loading = false
		m.err = nil
		m.fileItems = msg.items
		// The poller watches the paths svn reports, so the rows this reload added are
		// the new baseline rather than a change of their own.
		m.rebaseWatch()
		// svn has reported the real status, so anything shown ahead of it is either
		// confirmed by this or superseded; the snapshot behind it is now two steps back.
		m.dropOptimistic()
		// Re-derive what is on screen from the session: a cached diff the status
		// still stands behind stays put, so a reload no longer blanks a diff it is
		// about to fetch unchanged. Nothing else in the cache is touched — every
		// entry is re-checked when it is read, which keeps a routine reload cheap no
		// matter how much has been browsed.
		m.rederiveDiff()
		m.rebuildFileTree()
		m.focusFirstFile()
		m.rebuildChangelists()
		m.syncDrill()
		m.refreshChrome()
		// The first paint is done, so the page of history the Log panel needs can
		// be fetched now without the cost falling on startup.
		return m, tea.Batch(m.diffLoadForSelection(), m.ensureLogPage())

	case logLoadedMsg:
		// A page turn taken while this load was in flight supersedes it.
		if m.logGen.stale(msg.gen) || msg.page != m.logPage {
			return m, nil
		}
		m.logLoading = false
		if msg.err == nil {
			m.session.PutLogPage(
				logKey{anchor: m.logAnchor(msg.page), limit: m.cfg.LogLimit},
				historyPage{entries: msg.entries, more: msg.more},
			)
		}
		m.applyLogPage(historyPage{entries: msg.entries, more: msg.more}, msg.err)
		return m, m.revisionDetailForSelection()

	case headLoadedMsg:
		// The HEAD probe is the only svn command startup runs against the
		// repository, so its failure is what tells the Log panel it is unreachable.
		if msg.err != nil {
			m.logErr = msg.err
			if m.source == sourceLog {
				m.updateMain()
			}
			return m, nil
		}
		m.headRev = msg.rev
		m.updateStatus()
		return m, nil

	case revisionPendingMsg:
		// The cursor rested long enough for this revision's paths to be worth
		// asking svn for.
		if m.revGen.stale(msg.gen) {
			return m, nil
		}
		ctx, gen := m.revGen.begin(loadTimeout)
		return m, loadRevisionDetailCmd(ctx, m.client, msg.rev, gen)

	case revisionDetailMsg:
		// A revision that cannot be described leaves its metadata on screen; only
		// the changed-path list is missing, which is not worth an error for.
		if m.revGen.stale(msg.gen) || msg.err != nil {
			return m, nil
		}
		m.session.PutRevDetail(msg.rev, msg.paths)
		if m.source == sourceLog {
			m.updateMain()
		}
		return m, nil

	case diffPendingMsg:
		// The cursor rested long enough for this diff to be worth asking svn for.
		if m.diffGen.stale(msg.gen) {
			return m, nil
		}
		ctx, gen := m.diffGen.begin(loadTimeout)
		return m, loadDiffCmd(ctx, m.client, msg.key, gen)

	case diffLoadedMsg:
		if m.diffGen.stale(msg.gen) {
			return m, nil
		}
		k := diffKey{path: msg.path, dir: msg.dir}
		e := diffEntry{text: msg.diff, stamp: m.diffStamp(k)}
		if msg.err != nil {
			e.text, e.failed = "Unable to load diff: "+msg.err.Error(), true
		}
		m.session.PutDiff(k, e)
		m.applyDiff(k, e)
		if m.source == sourceFiles {
			m.updateMain()
		}
		return m, nil

	case diffSavedMsg:
		if msg.err != nil {
			m.showToast(failureText("save diff", msg.err), component.LevelError)
			return m, nil
		}
		m.showToast("diff saved to "+msg.path, component.LevelSuccess)
		// The new file belongs in the Diffs view, so re-scan the store.
		return m, m.reloadSavedDiffs()

	case savedDiffsLoadedMsg:
		if m.savedGen.stale(msg.gen) {
			return m, nil
		}
		m.savedDiffsErr = msg.err
		m.savedDiffItems = msg.files
		m.rebuildSavedDiffs()
		if m.source == sourceFiles && m.filesViewIsDiffs() {
			m.updateMain()
			return m, m.savedDiffLoadForSelection()
		}
		return m, nil

	case savedDiffReadMsg:
		if m.diffGen.stale(msg.gen) {
			return m, nil
		}
		m.savedPath = msg.path
		m.savedErr = msg.err != nil
		if msg.err != nil {
			m.savedText = "Unable to read diff: " + msg.err.Error()
		} else {
			m.savedText = msg.text
		}
		if m.source == sourceFiles {
			m.updateMain()
		}
		return m, nil

	case savedDiffDeletedMsg:
		if msg.err != nil {
			m.showToast(failureText("delete "+msg.name, msg.err), component.LevelError)
			return m, nil
		}
		m.showToast("deleted "+msg.name, component.LevelSuccess)
		if m.savedPath == msg.path {
			// Main is showing the file that just went away; drop it so the re-scan
			// reads whatever the list settles on.
			m.savedPath, m.savedText, m.savedErr = "", "", false
		}
		return m, m.reloadSavedDiffs()

	case rejectsLoadedMsg:
		if m.rejectGen.stale(msg.gen) {
			return m, nil
		}
		m.rejectsErr = msg.err
		m.rejectItems = msg.files
		m.rebuildRejects()
		if m.source == sourceFiles && m.filesViewIsRejects() {
			m.updateMain()
			return m, m.rejectLoadForSelection()
		}
		return m, nil

	case rejectReadMsg:
		if m.diffGen.stale(msg.gen) {
			return m, nil
		}
		m.rejectPath = msg.path
		m.rejectErr = msg.err != nil
		if msg.err != nil {
			m.rejectText = "Unable to read reject: " + msg.err.Error()
		} else {
			m.rejectText = msg.text
		}
		if m.source == sourceFiles {
			m.updateMain()
		}
		return m, nil

	case rejectDeletedMsg:
		if msg.err != nil {
			m.showToast(failureText("delete "+msg.name, msg.err), component.LevelError)
			return m, nil
		}
		m.showToast("deleted "+msg.name, component.LevelSuccess)
		if m.rejectPath == msg.path {
			// Main is showing the file that just went away; drop it so the re-scan
			// reads whatever the list settles on.
			m.rejectPath, m.rejectText, m.rejectErr = "", "", false
		}
		return m, m.reloadRejects()

	case patchAppliedMsg:
		if msg.err != nil {
			m.showToast(failureText("apply "+msg.name, msg.err), component.LevelError)
			return m, nil
		}
		m.showToast(patchToast(msg.name, msg.res))
		// The working copy now holds changes it did not a moment ago, so the diff on
		// screen and the status behind it are both out of date — and any hunk that
		// did not fit has just been written out as a reject.
		m.clearDiff()
		return m, tea.Batch(m.reloadStatus(), m.reloadRejectsIfShown())

	case mergeLoadedMsg:
		if msg.err != nil {
			m.showToast(failureText("read "+msg.rel, msg.err), component.LevelError)
			return m, nil
		}
		if len(msg.doc.regions) == 0 {
			m.showToast(mergeNothingToDo(msg.doc), component.LevelWarning)
			return m, nil
		}
		if msg.doc.unplaced > 0 {
			// The rest are still worth working through, so this is said in passing
			// rather than in place of the overlay.
			m.showToast(fmt.Sprintf("%d hunks no longer fit %s and are left in the reject",
				msg.doc.unplaced, msg.doc.rel), component.LevelWarning)
		}
		m.showMerge(msg.doc)
		return m, nil

	case mergeWrittenMsg:
		if msg.err != nil {
			m.showToast(failureText("resolve "+msg.rel, msg.err), component.LevelError)
			return m, nil
		}
		m.showToast(mergeDoneText(msg), component.LevelSuccess)
		// The file on disk is not the one the diff on screen was taken from, and
		// its status has just changed out from under the Files panel.
		m.clearDiff()
		if msg.kind != mergeReject {
			return m, m.reloadStatus()
		}
		if m.rejectPath == msg.aux {
			// Main is showing the reject that has just been cleared; drop it so the
			// re-scan reads whatever the list settles on.
			m.rejectPath, m.rejectText, m.rejectErr = "", "", false
		}
		return m, tea.Batch(m.reloadStatus(), m.reloadRejectsIfShown())

	case editedMsg:
		if msg.err != nil {
			m.showToast(failureText("open "+msg.name, msg.err), component.LevelError)
			return m, nil
		}
		if msg.detached {
			return m, nil
		}
		// A terminal editor has exited, so the file may have changed: re-read the
		// working copy (which reloads the diff on screen) and the local file stores.
		return m, tea.Batch(m.reloadStatus(), m.reloadSavedDiffsIfShown(), m.reloadRejectsIfShown())

	case errMsg:
		m.loading = false
		m.err = msg.err
		m.refreshChrome()
		return m, nil

	case stagedMsg:
		if msg.err != nil {
			m.showToast(failureText("stage", msg.err), component.LevelError)
			if msg.token == 0 {
				return m, nil
			}
			// Put the change back the way it was, then ask svn for the truth: a fan-out
			// over several files stops at the first failure, so some may have landed.
			m.settleOptimistic(msg.token, msg.err)
			return m, m.reloadStatus()
		}
		m.settleOptimistic(msg.token, nil)
		if msg.changelist != "" {
			m.showToast("added "+msg.path+" to "+msg.changelist, component.LevelSuccess)
		}
		// Reload status so the changelist grouping (and staged marker) refresh.
		return m, m.reloadStatus()

	case committedMsg:
		m.clearPending(msg.token)
		if msg.err != nil {
			m.loading = false
			m.showToast(failureText("commit", msg.err), component.LevelError)
			m.refreshChrome()
			return m, nil
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
		return m, tea.Batch(m.reloadStatus(), m.resetLogPaging())

	case revertedMsg:
		m.clearPending(msg.token)
		if msg.err != nil {
			m.showToast(failureText("revert", msg.err), component.LevelError)
			return m, nil
		}
		m.showToast("reverted "+msg.path, component.LevelSuccess)
		m.clearDiff()
		return m, m.reloadStatus()

	case deletedMsg:
		m.clearPending(msg.token)
		if msg.err != nil {
			m.showToast(failureText("delete", msg.err), component.LevelError)
			return m, nil
		}
		m.showToast("deleted "+msg.path, component.LevelSuccess)
		m.clearDiff()
		return m, m.reloadStatus()

	case updatedMsg:
		m.updatingWC = false
		m.updateProgress = ""
		if msg.err != nil {
			m.loading = false
			m.showToast(failureText("update", msg.err), component.LevelError)
			m.refreshChrome()
			return m, nil
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
		return m, tea.Batch(m.reloadStatus(), log)

	case workingCopyChangedMsg:
		return m, m.observeWorkingCopy(msg)

	case updateAvailableMsg:
		// Offer the update only when nothing else is on screen, so the prompt
		// never steals focus from an in-flight commit, confirmation, or menu.
		if !m.overlayActive() {
			m.openUpdate(msg.rel)
		}
		return m, nil

	case startupNoticeMsg:
		// A one-time notice surfaced at launch (e.g. config values reset during
		// reconciliation). It behaves like any toast: it clears on the next key.
		m.showToast(msg.text, component.LevelWarning)
		return m, nil

	case sshCheckedMsg:
		switch {
		case msg.err != nil:
			// The agent is unreachable or ssh-add is missing: there is nothing to
			// unlock and the key is required, so surface the error and quit.
			return m, m.abort("ssh-agent unavailable: " + msg.err.Error())
		case msg.loaded:
			return m, m.beginInitialLoad()
		default:
			m.openUnlock()
			return m, nil
		}

	case sshAddedMsg:
		if !m.unlocking {
			return m, nil
		}
		m.adding = false
		if msg.err != nil {
			if errors.Is(msg.err, sshagent.ErrAgentUnreachable) {
				return m, m.abort("ssh-agent unavailable: " + msg.err.Error())
			}
			m.passAttempts++
			if m.passAttempts >= maxPassphraseAttempts {
				return m, m.abort(fmt.Sprintf("SSH key not added after %d attempts; it is required for this working copy", m.passAttempts))
			}
			m.showToast(fmt.Sprintf("wrong passphrase (%d/%d) — try again", m.passAttempts, maxPassphraseAttempts), component.LevelError)
			m.passEditor.Reset()
			m.passEditor.Focus()
			return m, nil
		}
		m.showToast("SSH key added", component.LevelSuccess)
		m.closeUnlock()
		return m, m.beginInitialLoad()

	case sourceChangedMsg:
		return m, m.applySourceChange(msg)

	case uimsg.SelectedMsg:
		return m, m.handleSelection(msg)

	case uimsg.ActivatedMsg:
		// Enter on a changelist row drills into its files; enter on a directory in
		// the Changes tree or a drilled-in changelist tree collapses/expands it.
		switch msg.ID {
		case changelistsListID:
			return m, m.drillChangelist()
		case "files":
			return m, m.toggleCollapse()
		case changelistFilesID:
			return m, m.toggleClCollapse()
		case rejectsListID:
			return m, m.toggleRejectCollapse()
		case updateMenuID:
			return m, m.chooseUpdate(msg.Index)
		}
		return m, nil

	case uimsg.ViewSelectedMsg:
		if msg.ID == filesViewsID {
			m.updateBar()
			m.updateMain()
			switch msg.Name {
			case "Changes":
				return m, m.diffLoadForSelection()
			case savedDiffsViewName:
				// Re-scan on entry so diffs saved (or removed) elsewhere show up.
				return m, tea.Batch(m.reloadSavedDiffs(), m.savedDiffLoadForSelection())
			case rejectsViewName:
				// Re-scan on entry: a reject can appear or be cleaned up at any time.
				return m, tea.Batch(m.reloadRejects(), m.rejectLoadForSelection())
			}
		}
		return m, nil

	case uimsg.SubViewPoppedMsg:
		if msg.ID == filesViewsID {
			m.drilledCL = ""
			m.updateBar()
			m.updateMain()
		}
		return m, nil

	case uimsg.SubmitMsg:
		switch msg.ID {
		case commitEditorID:
			return m, m.submitCommit(msg.Value)
		case changelistEditorID:
			return m, m.submitChangelist(msg.Value)
		case diffNameEditorID:
			return m, m.submitDiffName(msg.Value)
		case sourcePathID:
			return m, m.submitSourcePath(msg.Value)
		case settingsFormID:
			return m, m.submitSettings()
		case passphraseEditorID:
			return m, m.submitUnlock(msg.Value)
		case searchBarID:
			m.commitFilter()
			return m, nil
		}
		return m, nil

	case uimsg.ConfirmMsg:
		if msg.ID == confirmModalID {
			m.closeConfirm()
			if prompt := m.updateConflictPrompt; prompt != "" {
				// The default update confirm was accepted, but the working copy
				// holds conflicts svn would silently skip: confirm once more,
				// spelling that out, before actually updating.
				m.updateConflictPrompt = ""
				m.openConfirm("Conflicts present — continue?", prompt)
				return m, nil
			}
			cmd := m.pending
			m.pending = nil
			// The action is on its way now, so the rows it touches read as in flight.
			m.markHeldPending()
			if m.updateProgress != "" {
				// The pending command is an svn update; show the progress modal
				// until it completes (cleared in the updatedMsg handler).
				m.showUpdating()
			}
			return m, cmd
		}
		return m, nil

	case uimsg.DismissMsg:
		switch msg.ID {
		case commitEditorID:
			m.editing = false
			m.editor.Blur()
		case changelistEditorID:
			m.naming = false
			m.nameEditor.Blur()
		case diffNameEditorID:
			m.closeDiffName()
		case sourcePathID:
			m.closeSourcePath()
		case splitDiffID:
			m.closeSplitDiff()
		case mergeViewID:
			m.closeMerge()
		case passphraseEditorID:
			// The key is required and the user declined to unlock it, so exiting is
			// the only sensible outcome; proceeding would leave a UI that cannot
			// reach the repository.
			return m, m.abort("SSH key required: passphrase entry cancelled")
		case confirmModalID:
			m.closeConfirm()
			m.pending = nil
			m.pendingHold = nil
			m.updateConflictPrompt = ""
			m.updateProgress = ""
		case updateMenuID:
			m.closeUpdate()
		case settingsFormID:
			m.closeSettings()
		case searchBarID:
			return m, m.clearFilter()
		}
		return m, nil

	case tea.KeyMsg:
		if m.aborting {
			// A fatal SSH error is on screen; any key quits so the user can retry.
			return m, tea.Quit
		}
		if m.updatingWC {
			// An svn update is running behind the progress modal; ignore keys so
			// they can't disturb the panels until it completes.
			return m, nil
		}
		if m.unlocking {
			// While the entered passphrase is being added, the input is locked so
			// stray keys can't queue another attempt or reach the panels beneath.
			if m.adding {
				return m, nil
			}
			return m, m.passEditor.Update(msg)
		}
		if m.editing {
			return m, m.editor.Update(msg)
		}
		if m.naming {
			return m, m.nameEditor.Update(msg)
		}
		if m.savingDiff {
			return m, m.diffEditor.Update(msg)
		}
		if m.retargeting {
			// Every edit re-lists the directories under the path being typed, so the
			// suggestions follow it. Scrolling the list writes the value too, so its
			// own picks are left alone.
			before := m.pathEditor.Value()
			cmd := m.pathEditor.Update(msg)
			if m.pathEditor.Value() != before && !m.pathEditor.ListFocused() {
				m.refreshSourceOptions()
			}
			return m, cmd
		}
		if m.splitting {
			// The side-by-side view owns the keyboard while open: it scrolls, esc
			// closes it (as a DismissMsg), and the key that opened it closes it too.
			if key.Matches(msg, m.keys.SplitDiff) {
				m.closeSplitDiff()
				return m, nil
			}
			// Editing is the one action reaching through: the overlay holds a
			// snapshot, so it steps aside rather than sit over a file being changed.
			if key.Matches(msg, m.keys.OpenEditor) {
				if cmd := m.openInEditor(); cmd != nil {
					m.closeSplitDiff()
					return m, cmd
				}
				return m, nil
			}
			return m, m.splitDiff.Update(msg)
		}
		if m.merging {
			// The resolution overlay owns the keyboard while open: it decides a
			// region, scrolls, and closes on esc (as a DismissMsg) or on the key that
			// opened it.
			return m, m.mergeKey(msg)
		}
		if m.filtering {
			// The filter input owns the keyboard while open. Every edit re-runs the
			// filter live so the panel updates as the user types; enter and esc are
			// returned by the search bar as Submit/Dismiss and handled above.
			before := m.searchBar.Value()
			cmd := m.searchBar.Update(msg)
			if m.searchBar.Value() != before {
				cmd = tea.Batch(cmd, m.applyFilterLive())
			}
			return m, cmd
		}
		if m.configuring {
			// The settings editor live-previews the palette while its Theme field
			// changes, so scrolling that field re-themes the UI immediately. The
			// choice is only persisted on ctrl+s; esc reverts it via closeSettings.
			before := m.form.Value(themeFieldIndex)
			cmd := m.form.Update(msg)
			if after := m.form.Value(themeFieldIndex); after != before {
				m.previewTheme(after)
			}
			return m, cmd
		}
		if m.confirming {
			return m, m.modal.Update(msg)
		}
		if m.updating {
			// The update prompt captures every key: ↑/↓ move, enter chooses a
			// method, esc dismisses ("don't update this time").
			return m, m.updateMenu.Update(msg)
		}
		if m.helping {
			// Read-only reference: ? and esc close it, every other key is
			// swallowed.
			if key.Matches(msg, m.keys.Help) || key.Matches(msg, m.keys.Back) {
				m.closeHelp()
				return m, nil
			}
			return m, nil
		}
		m.dismissToast()
		if cmd, handled := m.handleKey(msg); handled {
			return m, cmd
		}
	}

	return m, m.panels[m.focus.Index()].Update(msg)
}
