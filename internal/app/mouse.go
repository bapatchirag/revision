package app

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// doubleClickWindow is how long after a click a second one on the same cell
// still reads as a double click.
const doubleClickWindow = 500 * time.Millisecond

// clickAt is where and when a left click landed, so the next one can be judged
// against it.
type clickAt struct {
	x, y int
	at   time.Time
}

// routeMouse gives a left click the effect of the clicked panel's number key:
// it moves focus there. Inside the panel the click lands on something: one of
// the view names inlaid in its border selects that view, as [ and ] do, and a
// row moves the selection to it, as the arrow keys do. Clicking that row a
// second time runs what it is for — see doubleClick. The wheel scrolls whatever
// the pointer rests on. Nothing else is read from the mouse.
func (m *Model) routeMouse(msg tea.MouseMsg) tea.Cmd {
	// An overlay, the update progress modal or the filter bar owns the screen or
	// the keyboard; the panels below are not what is being pointed at.
	if m.overlayActive() || m.updatingWC || m.filtering {
		return nil
	}
	idx, r, ok := m.panelAt(msg.X, msg.Y)
	if !ok {
		return nil
	}
	if dx, dy, wheel := wheelStep(msg); wheel {
		// The wheel scrolls what the pointer rests on, focus untouched: reading
		// somewhere is not working there.
		return m.panels[idx].Scroll(dx, dy)
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return nil
	}
	x, y := msg.X-r.x, msg.Y-r.y
	panel := m.panels[idx]
	view, onTab := panel.ClickTab(x, y)
	row := panel.ClickRow(x, y)
	// Only a row can be double-clicked; the border carries no action.
	second := panel.InBody(x, y) && m.doubleClicked(msg.X, msg.Y)
	focusing := idx != m.focus.Index()
	if !focusing && !onTab && !second && row == nil {
		return nil
	}
	// Dismissed before acting, since acting may raise a notice of its own.
	m.dismissToast()

	cmds := []tea.Cmd{view, row}
	// Focus first: what a double click does reads the focused panel and the
	// selection it drives.
	if focusing {
		m.focus.Focus(idx)
		cmds = append(cmds, m.afterFocusChange())
	}
	if second {
		cmds = append(cmds, m.doubleClick(idx))
	}
	return tea.Batch(cmds...)
}

// wheelStep turns a wheel event into a scroll step: dy down the content, dx
// across it. It reports false for any other mouse event. A panel whose window
// follows a selection scrolls by moving it, exactly as its arrow keys do.
func wheelStep(msg tea.MouseMsg) (dx, dy int, ok bool) {
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		dy = -1
	case tea.MouseButtonWheelDown:
		dy = 1
	case tea.MouseButtonWheelLeft:
		dx = -1
	case tea.MouseButtonWheelRight:
		dx = 1
	default:
		return 0, 0, false
	}
	// A mouse with no sideways wheel scrolls across with shift held, which
	// terminals report as a plain wheel carrying the modifier.
	if msg.Shift && dy != 0 {
		dx, dy = dy, 0
	}
	return dx, dy, true
}

// doubleClicked records a click and reports whether it completes a double click:
// a second press on the same cell within the window. The record is cleared when
// one completes, so a third press starts a fresh pair rather than firing again.
func (m *Model) doubleClicked(x, y int) bool {
	prev := m.lastClick
	now := time.Now()
	m.lastClick = clickAt{x: x, y: y, at: now}
	if prev.at.IsZero() || prev.x != x || prev.y != y || now.Sub(prev.at) > doubleClickWindow {
		return false
	}
	m.lastClick = clickAt{}
	return true
}

// doubleClick is the second click's own meaning, on top of the selection the
// first one moved: the row's action in the Files panel, the update-to-revision
// prompt on a revision, and the editor on a diff line in Main. Nothing else on
// screen has an action a row can carry.
func (m *Model) doubleClick(panel int) tea.Cmd {
	switch panel {
	case panelFiles:
		return m.activateFilesRow()
	case panelLog:
		return m.requestUpdateToRevision()
	case panelMain:
		// Main carries a line cursor only while it holds a diff, which is the only
		// thing there an editor can be opened on.
		if m.main.Cursor() >= 0 {
			return m.openInEditor()
		}
	}
	return nil
}

// activateFilesRow is what a double click means in the Files panel: the row's
// own action where it has one — a changelist opens, a directory folds — and
// otherwise the file it names goes to the editor.
func (m *Model) activateFilesRow() tea.Cmd {
	switch {
	case m.filesViewIsChangelists() && !m.inChangelistDrill():
		return m.drillChangelist()
	case m.filesViewIsRejects():
		if n, ok := m.rejects.Selected(); ok && n.Item == nil {
			return m.toggleRejectCollapse()
		}
	default:
		// The Diffs view is a flat list of patch files, so it has no directory row
		// and selectedDirectory reports none.
		if _, _, dir := m.selectedDirectory(); dir {
			if m.inChangelistDrill() {
				return m.toggleClCollapse()
			}
			return m.toggleCollapse()
		}
	}
	return m.openInEditor()
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
