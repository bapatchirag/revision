package app

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// routeKey gives a key to whatever owns the keyboard — an open overlay first,
// then the global bindings — and reports whether it was consumed. An unclaimed
// key falls through to the focused panel.
func (m *Model) routeKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if m.aborting {
		// A fatal SSH error is on screen; any key quits so the user can retry.
		return tea.Quit, true
	}
	if m.updatingWC {
		// An svn update is running behind the progress modal; ignore keys so
		// they can't disturb the panels until it completes.
		return nil, true
	}
	if m.unlocking {
		// While the entered passphrase is being added, the input is locked so
		// stray keys can't queue another attempt or reach the panels beneath.
		if m.adding {
			return nil, true
		}
		return m.passEditor.Update(msg), true
	}
	if m.editing {
		return m.editor.Update(msg), true
	}
	if m.naming {
		return m.nameEditor.Update(msg), true
	}
	if m.savingDiff {
		return m.diffEditor.Update(msg), true
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
		return cmd, true
	}
	if m.switchingRepo {
		// Every edit narrows the discovered working copies to those matching what
		// has been typed. Scrolling the list writes the value too, so its own picks
		// are left alone.
		before := m.repoEditor.Value()
		cmd := m.repoEditor.Update(msg)
		if m.repoEditor.Value() != before && !m.repoEditor.ListFocused() {
			m.refreshRepoOptions()
		}
		return cmd, true
	}
	if m.splitting {
		// The side-by-side view owns the keyboard while open: it scrolls, esc
		// closes it (as a DismissMsg), and the key that opened it closes it too.
		if key.Matches(msg, m.keys.SplitDiff) {
			m.closeSplitDiff()
			return nil, true
		}
		// Editing is the one action reaching through: the overlay holds a
		// snapshot, so it steps aside rather than sit over a file being changed.
		if key.Matches(msg, m.keys.OpenEditor) {
			if cmd := m.openInEditor(); cmd != nil {
				m.closeSplitDiff()
				return cmd, true
			}
			return nil, true
		}
		return m.splitDiff.Update(msg), true
	}
	if m.merging {
		// The resolution overlay owns the keyboard while open: it decides a
		// region, scrolls, and closes on esc (as a DismissMsg) or on the key that
		// opened it.
		return m.mergeKey(msg), true
	}
	if m.filtering {
		// The filter input owns the keyboard while open. Every edit re-runs the
		// filter live so the panel updates as the user types; enter and esc are
		// returned by the search bar as Submit/Dismiss and handled by uiEvent.
		before := m.searchBar.Value()
		cmd := m.searchBar.Update(msg)
		if m.searchBar.Value() != before {
			cmd = tea.Batch(cmd, m.applyFilterLive())
		}
		return cmd, true
	}
	if m.configuring {
		if m.editingRules {
			// The rules editor sits over the settings editor and owns the keyboard
			// while open; esc closes it alone (as a DismissMsg), leaving the form up.
			return m.rulesEditor.Update(msg), true
		}
		// The settings editor live-previews the palette while its Theme field
		// changes, so scrolling that field re-themes the UI immediately. The
		// choice is only persisted on ctrl+s; esc reverts it via closeSettings.
		before := m.form.Value(themeFieldIndex)
		cmd := m.form.Update(msg)
		if after := m.form.Value(themeFieldIndex); after != before {
			m.previewTheme(after)
		}
		return cmd, true
	}
	if m.confirming {
		return m.modal.Update(msg), true
	}
	if m.updating {
		// The update prompt captures every key: ↑/↓ move, enter chooses a
		// method, esc dismisses ("don't update this time").
		return m.updateMenu.Update(msg), true
	}
	if m.helping {
		// Read-only reference: ? and esc close it, every other key is
		// swallowed.
		if key.Matches(msg, m.keys.Help) || key.Matches(msg, m.keys.Back) {
			m.closeHelp()
		}
		return nil, true
	}
	m.dismissToast()
	return m.handleKey(msg)
}

// handleKey processes global keys, returning whether the key was consumed.
func (m *Model) handleKey(k tea.KeyMsg) (tea.Cmd, bool) {
	switch {
	case key.Matches(k, m.keys.Quit):
		return tea.Quit, true
	case key.Matches(k, m.keys.Refresh):
		m.loading = true
		m.dismissToast()
		// An explicit refresh asks for fresh data: nothing may be answered from
		// the session.
		m.session.Purge()
		m.clearDiff()
		m.refreshChrome()
		return tea.Batch(m.reloadStatus(), m.reloadLogPage(), m.reloadSavedDiffsIfShown(), m.reloadRejectsIfShown()), true
	case key.Matches(k, m.keys.FocusNext):
		m.focusNextPanel()
		return m.afterFocusChange(), true
	case key.Matches(k, m.keys.FocusPrev):
		m.focusPrevPanel()
		return m.afterFocusChange(), true
	case key.Matches(k, m.keys.Settings):
		return m.openSettings(), true
	case key.Matches(k, m.keys.ChangeDir):
		return m.openSourcePath(), true
	case key.Matches(k, m.keys.SwitchRepo):
		return m.openRepoSwitch(), true
	case key.Matches(k, m.keys.Filter):
		return m.openFilter(), true
	case key.Matches(k, m.keys.ToggleCmdLog):
		return m.toggleCmdLog(), true
	case key.Matches(k, m.keys.ToggleLiveRefresh):
		return m.toggleLiveRefresh(), true
	case key.Matches(k, m.keys.SaveDiff):
		return m.saveDiff(), true
	case key.Matches(k, m.keys.SplitDiff):
		if m.focus.Index() == panelFiles {
			return m.openSplitDiff(), true
		}
		return nil, false
	case key.Matches(k, m.keys.OpenEditor):
		return m.openInEditor(), true
	case key.Matches(k, m.keys.Back):
		// esc clears the focused panel's filter when it has one; otherwise it is
		// left for the panel (e.g. to pop a changelist drill).
		if cmd, cleared := m.clearFocusedFilter(); cleared {
			return cmd, true
		}
		return nil, false
	case key.Matches(k, m.keys.Help):
		return m.openHelp(), true
	}

	switch k.String() {
	case "1":
		m.focus.Focus(panelStatus)
		return m.afterFocusChange(), true
	case "2":
		m.focus.Focus(panelFiles)
		return m.afterFocusChange(), true
	case "3":
		m.focus.Focus(panelLog)
		return m.afterFocusChange(), true
	case "4":
		// Focusing the command log reveals it first when hidden, since it cannot
		// be focused while off-screen.
		if !m.showCmdLog {
			m.showCmdLog = true
			m.layout()
		}
		m.focus.Focus(panelCmdLog)
		return m.afterFocusChange(), true
	case "0":
		m.focus.Focus(panelMain)
		return m.afterFocusChange(), true
	case " ":
		switch m.focus.Index() {
		case panelFiles:
			return m.stageSelected(), true
		case panelLog:
			return m.requestUpdateToRevision(), true
		}
		return nil, false
	case "n":
		if m.isSearchPanel(m.focus.Index()) && m.filters[m.focus.Index()] != "" {
			m.jumpMatch(m.focus.Index(), 1)
			return nil, true
		}
		if m.focus.Index() == panelFiles {
			return m.assignChangelist(), true
		}
		if m.focus.Index() == panelLog {
			return m.nextLogPage(), true
		}
		return nil, false
	case "p":
		// In the Diffs view p applies the highlighted patch; everywhere else in the
		// Files panel it means nothing, so it stays the Log panel's page-back key.
		if m.focus.Index() == panelFiles && m.filesViewIsDiffs() {
			return m.requestApplyPatch(), true
		}
		if m.focus.Index() == panelLog {
			return m.prevLogPage(), true
		}
		return nil, false
	case "N":
		if m.isSearchPanel(m.focus.Index()) && m.filters[m.focus.Index()] != "" {
			m.jumpMatch(m.focus.Index(), -1)
			return nil, true
		}
		return nil, false
	case "c":
		return m.openCommit(), true
	case "m":
		// Resolving acts on a conflicted file in the Changes tree, or on a reject in
		// the Rejects view; nowhere else has anything to resolve.
		if m.focus.Index() == panelFiles {
			return m.openMerge(), true
		}
		return nil, false
	case "r":
		if m.focus.Index() == panelFiles {
			return m.requestRevert(), true
		}
		return nil, false
	case "d":
		if m.focus.Index() == panelFiles {
			return m.requestDelete(), true
		}
		return nil, false
	case "u":
		return m.requestUpdate(), true
	case "D":
		return m.toggleDirDiff(), true
	case "U":
		return m.toggleUntracked(), true
	}
	return nil, false
}
