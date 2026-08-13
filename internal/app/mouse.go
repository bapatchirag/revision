package app

import (
	tea "github.com/charmbracelet/bubbletea"
)

// routeMouse gives a left click the effect of the clicked panel's number key:
// it moves focus there. Inside the panel the click lands on something: one of
// the view names inlaid in its border selects that view, as [ and ] do, and a
// row moves the selection to it, as the arrow keys do. Nothing else is read from
// the mouse — no mouse event ever reaches a panel — so a drag or a wheel cannot
// move a selection or a scroll position out from under the keyboard.
func (m *Model) routeMouse(msg tea.MouseMsg) tea.Cmd {
	// An overlay, the update progress modal or the filter bar owns the screen or
	// the keyboard; the panels below are not what is being pointed at.
	if m.overlayActive() || m.updatingWC || m.filtering {
		return nil
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return nil
	}
	idx, r, ok := m.panelAt(msg.X, msg.Y)
	if !ok {
		return nil
	}
	x, y := msg.X-r.x, msg.Y-r.y
	view, onTab := m.panels[idx].ClickTab(x, y)
	row := m.panels[idx].ClickRow(x, y)
	focusing := idx != m.focus.Index()
	if !focusing && !onTab && row == nil {
		return nil
	}
	m.dismissToast()
	if focusing {
		m.focus.Focus(idx)
		return tea.Batch(m.afterFocusChange(), view, row)
	}
	return tea.Batch(view, row)
}

// panelAt reports which panel covers a screen cell, and where that panel sits so
// the caller can put the cell in the panel's own coordinates. Cells belonging to
// no panel — the bar row, or the command log's strip while it is hidden — report
// false.
func (m *Model) panelAt(x, y int) (int, rect, bool) {
	for i, r := range m.panelRects() {
		if r.contains(x, y) {
			return i, r, true
		}
	}
	return 0, rect{}, false
}
