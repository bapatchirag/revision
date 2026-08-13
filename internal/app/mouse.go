package app

import (
	tea "github.com/charmbracelet/bubbletea"
)

// routeMouse gives a left click the effect of the clicked panel's number key:
// it moves focus there. Nothing else is read from the mouse — no mouse event
// ever reaches a panel — so a drag or a wheel cannot move a selection or a
// scroll position out from under the keyboard.
func (m *Model) routeMouse(msg tea.MouseMsg) tea.Cmd {
	// An overlay, the update progress modal or the filter bar owns the screen or
	// the keyboard; the panels below are not what is being pointed at.
	if m.overlayActive() || m.updatingWC || m.filtering {
		return nil
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return nil
	}
	idx, ok := m.panelAt(msg.X, msg.Y)
	if !ok || idx == m.focus.Index() {
		return nil
	}
	m.dismissToast()
	m.focus.Focus(idx)
	return m.afterFocusChange()
}

// panelAt reports which panel covers a screen cell. Cells belonging to no panel
// — the bar row, or the command log's strip while it is hidden — report false.
func (m *Model) panelAt(x, y int) (int, bool) {
	for i, r := range m.panelRects() {
		if r.contains(x, y) {
			return i, true
		}
	}
	return 0, false
}
