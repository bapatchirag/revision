package app

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

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
