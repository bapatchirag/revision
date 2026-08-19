package app

import (
	"slices"

	tea "github.com/charmbracelet/bubbletea"

	uimsg "github.com/bapatchirag/revision/internal/tui/msg"
)

// sidePanels is what Tab cycles through. Main and the command log are left out
// of it, as in lazygit: Main is reached with 0 or a click, the command log with
// x or a click, so neither has a second way in.
var sidePanels = []int{panelStatus, panelFiles, panelLog, panelShelf}

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
	case revFilesListID:
		if m.source == sourceLog {
			m.updateMain()
		}
	case shelfListID:
		if m.source == sourceShelf {
			m.updateMain()
			return m.shelfLoadForSelection()
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
	case panelShelf:
		m.source = sourceShelf
	case panelMain, panelCmdLog:
		// Focusing Main or the command log only scrolls it; keep the current source.
	}
	// History is asked for before Main is rendered, so the first look at the Log
	// panel shows that it is loading rather than that it is empty.
	var cmd tea.Cmd
	if m.source == sourceLog {
		cmd = tea.Batch(m.ensureLogPage(), m.revisionDetailForSelection())
	}
	// The Shelf panel's height follows which panel has focus, not the terminal
	// size alone, so the layout is recomputed on every change of focus.
	m.layout()
	m.syncMainTitle()
	m.updateBar()
	m.updateMain()
	if m.source == sourceFiles {
		return m.diffLoadForSelection()
	}
	if m.source == sourceShelf {
		return tea.Batch(cmd, m.shelfLoadForSelection())
	}
	return cmd
}

// focusNextPanel and focusPrevPanel step around the side panels only, so Tab
// never lands on Main or the command log.
func (m *Model) focusNextPanel() { m.stepPanel(1) }

func (m *Model) focusPrevPanel() { m.stepPanel(-1) }

// stepPanel moves focus one place around sidePanels. Pressed on Main or the
// command log — which are outside the cycle — it returns to the side panel
// driving Main, which is where the user came from.
func (m *Model) stepPanel(step int) {
	at := slices.Index(sidePanels, m.focus.Index())
	if at < 0 {
		m.focus.Focus(m.sourcePanel())
		return
	}
	n := len(sidePanels)
	m.focus.Focus(sidePanels[((at+step)%n+n)%n])
}

// sourcePanel is the side panel currently driving Main. Focusing Main or the
// command log leaves the source alone, so it still names the last side panel to
// hold focus.
func (m *Model) sourcePanel() int {
	switch m.source {
	case sourceStatus:
		return panelStatus
	case sourceLog:
		return panelLog
	case sourceShelf:
		return panelShelf
	default:
		return panelFiles
	}
}

// syncMainTitle names the Main panel after the focused side panel: the Status
// panel makes it "About", the Files panel "Diff", and the Log panel the commit
// message — or "Diff" too, while it is showing a range of history. Focusing Main
// itself leaves the heading unchanged, so it keeps naming whichever side panel
// last drove it.
func (m *Model) syncMainTitle() {
	switch m.focus.Index() {
	case panelStatus:
		m.panels[panelMain].SetTitle("About")
	case panelFiles:
		m.panels[panelMain].SetTitle("Diff")
	case panelLog:
		if m.revDiff.set() {
			m.panels[panelMain].SetTitle("Diff")
			return
		}
		m.panels[panelMain].SetTitle("Commit message")
	case panelShelf:
		m.panels[panelMain].SetTitle("Shelved diff")
	}
}
