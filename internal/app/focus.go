package app

import (
	tea "github.com/charmbracelet/bubbletea"

	uimsg "github.com/bapatchirag/revision/internal/tui/msg"
)

// handleSelection re-renders Main when the selection that drives it changes, and
// loads the diff for a newly selected file.
func (m *Model) handleSelection(sel uimsg.SelectedMsg) tea.Cmd {
	switch sel.ID {
	case "files", changelistFilesID:
		if m.source == sourceFiles {
			m.updateMain()
			return m.diffLoadForSelection()
		}
	case changelistsListID:
		if m.source == sourceFiles {
			m.updateMain()
		}
	case savedDiffsListID:
		if m.source == sourceFiles {
			m.updateMain()
			return m.savedDiffLoadForSelection()
		}
	case rejectsListID:
		if m.source == sourceFiles {
			m.updateMain()
			return m.rejectLoadForSelection()
		}
	case "log":
		if m.source == sourceLog {
			m.updateMain()
			return m.revisionDetailForSelection()
		}
	}
	return nil
}

// afterFocusChange updates which panel drives Main, refreshes the chrome, and
// loads a diff when Main now follows the Files panel.
func (m *Model) afterFocusChange() tea.Cmd {
	switch m.focus.Index() {
	case panelStatus:
		m.source = sourceStatus
	case panelLog:
		m.source = sourceLog
	case panelFiles:
		m.source = sourceFiles
	case panelMain, panelCmdLog:
		// Focusing Main or the command log only scrolls it; keep the current source.
	}
	// History is asked for before Main is rendered, so the first look at the Log
	// panel shows that it is loading rather than that it is empty.
	var cmd tea.Cmd
	if m.source == sourceLog {
		cmd = tea.Batch(m.ensureLogPage(), m.revisionDetailForSelection())
	}
	m.syncMainTitle()
	m.updateBar()
	m.updateMain()
	if m.source == sourceFiles {
		return m.diffLoadForSelection()
	}
	return cmd
}

// focusNextPanel and focusPrevPanel cycle focus like the focus manager but skip
// the command-log panel while it is hidden, so Tab never lands on an off-screen
// panel. It is the only hideable panel and sits at the end of the ring, so a
// single extra step is always enough to pass it.
func (m *Model) focusNextPanel() {
	m.focus.Next()
	if m.focus.Index() == panelCmdLog && !m.showCmdLog {
		m.focus.Next()
	}
}

func (m *Model) focusPrevPanel() {
	m.focus.Prev()
	if m.focus.Index() == panelCmdLog && !m.showCmdLog {
		m.focus.Prev()
	}
}

// syncMainTitle names the Main panel after the focused side panel: the Status
// panel makes it "About", the Files panel "Diff", and the Log panel "Commit
// message". Focusing Main itself leaves the heading unchanged, so it keeps
// naming whichever side panel last drove it.
func (m *Model) syncMainTitle() {
	switch m.focus.Index() {
	case panelStatus:
		m.panels[panelMain].SetTitle("About")
	case panelFiles:
		m.panels[panelMain].SetTitle("Diff")
	case panelLog:
		m.panels[panelMain].SetTitle("Commit message")
	}
}
