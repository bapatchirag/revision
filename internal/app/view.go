package app

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/bapatchirag/revision/internal/tui/layout"
)

// View renders the full lazygit layout, floating a transient toast and, while
// active, the commit editor or a confirmation modal over it.
func (m *Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "loading…"
	}
	view := m.baseView()
	if m.aborting {
		// A fatal SSH error: show it centered and wait for the quit keypress.
		return m.overlayCenter(view, m.toast.View())
	}
	if m.showingToast {
		view = m.overlayToast(view)
	}
	switch {
	case m.updatingWC:
		view = m.overlayCenter(view, m.progress.View())
	case m.unlocking:
		view = m.overlayCenter(view, m.passEditor.View())
	case m.editing:
		view = m.overlayCenter(view, m.editor.View())
	case m.naming:
		view = m.overlayCenter(view, m.nameEditor.View())
	case m.savingDiff:
		view = m.overlayCenter(view, m.diffEditor.View())
	case m.retargeting:
		view = m.overlayCenter(view, m.pathEditor.View())
	case m.splitting:
		// The side-by-side view all but fills the screen and is read rather than
		// acted on, so the layout behind it recedes to a single dim color.
		view = m.overlayCenter(layout.Dim(view, m.theme.Muted), m.splitDiff.View())
	case m.merging:
		// Resolving is the same two-pane read, with a decision attached to it.
		view = m.overlayCenter(layout.Dim(view, m.theme.Muted), m.mergeView.View())
	case m.confirming:
		view = m.overlayCenter(view, m.modal.View())
	case m.updating:
		view = m.overlayCenter(view, m.updateMenu.View())
	case m.helping:
		view = m.overlayCenter(view, m.menu.View())
	case m.configuring:
		view = m.overlayCenter(view, m.form.View())
	}
	return view
}

// overlayCenter floats popup in the middle of the base view.
func (m *Model) overlayCenter(base, popup string) string {
	x := max((m.width-lipgloss.Width(popup))/2, 0)
	y := max((m.height-lipgloss.Height(popup))/2, 0)
	return layout.Overlay(base, popup, x, y)
}

// overlayToast floats the toast in the bottom-right corner, just above the
// status bar.
func (m *Model) overlayToast(base string) string {
	popup := m.toast.View()
	if popup == "" {
		return base
	}
	x := max(m.width-lipgloss.Width(popup)-1, 0)
	y := max(m.height-lipgloss.Height(popup)-1, 0) // 1 row for the status bar
	return layout.Overlay(base, popup, x, y)
}

// baseView renders the lazygit layout: the left column of panels beside Main,
// over the status bar. When the command log is shown it sits below Main in the
// right column.
func (m *Model) baseView() string {
	m.panels[panelFiles].SetFooter(m.filesFooter())
	m.panels[panelLog].SetFooter(m.logFooter())
	left := lipgloss.JoinVertical(lipgloss.Left,
		m.panels[panelStatus].View(),
		m.panels[panelFiles].View(),
		m.panels[panelLog].View(),
	)
	right := m.panels[panelMain].View()
	if m.showCmdLog {
		right = lipgloss.JoinVertical(lipgloss.Left, right, m.panels[panelCmdLog].View())
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	// While a filter is being typed the search bar takes the status bar's row so
	// the panel content stays fully visible above it.
	bottom := m.bar.View()
	if m.filtering {
		bottom = m.searchBar.View()
	}
	return lipgloss.JoinVertical(lipgloss.Left, body, bottom)
}
