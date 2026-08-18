package app

import (
	tea "github.com/charmbracelet/bubbletea"

	uimsg "github.com/bapatchirag/revision/internal/tui/msg"
)

// uiEvent handles what the components report back: a selection moving, a row
// activated, a Files-panel view or drill changing, and the submit, confirm and
// dismiss of every overlay. It reports whether it owned the message.
func (m *Model) uiEvent(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case uimsg.SelectedMsg:
		return m.handleSelection(msg), true

	case uimsg.ActivatedMsg:
		// Enter on a changelist row drills into its files; enter on a directory in
		// the Changes tree or a drilled-in changelist tree collapses/expands it.
		switch msg.ID {
		case changelistsListID:
			return m.drillChangelist(), true
		case "files":
			return m.toggleCollapse(), true
		case changelistFilesID:
			return m.toggleClCollapse(), true
		case rejectsListID:
			return m.toggleRejectCollapse(), true
		case "log":
			return m.showPickedDiff(), true
		case revFilesListID:
			return m.toggleRevCollapse(), true
		case updateMenuID:
			return m.chooseUpdate(msg.Index), true
		case settingsFormID:
			// The only activatable row in the settings editor opens the rules editor.
			if msg.Index == hideRulesFieldIndex {
				return m.openHideRules(), true
			}
		}
		return nil, true

	case uimsg.ViewSelectedMsg:
		if msg.ID == filesViewsID {
			m.updateBar()
			m.updateMain()
			switch msg.Name {
			case "Changes":
				return m.diffLoadForSelection(), true
			case savedDiffsViewName:
				// Re-scan on entry so diffs saved (or removed) elsewhere show up.
				return tea.Batch(m.reloadSavedDiffs(), m.savedDiffLoadForSelection()), true
			case rejectsViewName:
				// Re-scan on entry: a reject can appear or be cleaned up at any time.
				return tea.Batch(m.reloadRejects(), m.rejectLoadForSelection()), true
			}
		}
		return nil, true

	case uimsg.SubViewPoppedMsg:
		switch msg.ID {
		case filesViewsID:
			m.drilledCL = ""
			m.updateBar()
			m.updateMain()
		case logViewsID:
			m.closeRevDiff()
		}
		return nil, true

	case uimsg.SubmitMsg:
		switch msg.ID {
		case commitEditorID:
			return m.submitCommit(msg.Value), true
		case changelistEditorID:
			return m.submitChangelist(msg.Value), true
		case diffNameEditorID:
			return m.submitDiffName(msg.Value), true
		case sourcePathID:
			return m.submitSourcePath(msg.Value), true
		case repoSwitchID:
			return m.submitRepoPath(msg.Value), true
		case settingsFormID:
			return m.submitSettings(), true
		case hideRulesEditorID:
			return m.submitHideRules(), true
		case passphraseEditorID:
			return m.submitUnlock(msg.Value), true
		case searchBarID:
			m.commitFilter()
			return nil, true
		}
		return nil, true

	case uimsg.ConfirmMsg:
		if msg.ID == confirmModalID {
			m.closeConfirm()
			if prompt := m.updateConflictPrompt; prompt != "" {
				// The default update confirm was accepted, but the working copy
				// holds conflicts svn would silently skip: confirm once more,
				// spelling that out, before actually updating.
				m.updateConflictPrompt = ""
				m.openConfirm("Conflicts present — continue?", prompt)
				return nil, true
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
			return cmd, true
		}
		return nil, true

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
		case repoSwitchID:
			m.closeRepoSwitch()
		case splitDiffID:
			m.closeSplitDiff()
		case mergeViewID:
			m.closeMerge()
		case passphraseEditorID:
			// The key is required and the user declined to unlock it, so exiting is
			// the only sensible outcome; proceeding would leave a UI that cannot
			// reach the repository.
			return m.abort("SSH key required: passphrase entry cancelled"), true
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
		case hideRulesEditorID:
			m.closeHideRules()
		case searchBarID:
			return m.clearFilter(), true
		}
		return nil, true
	}
	return nil, false
}
